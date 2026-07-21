package corelifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/ownedcontainer"
	"github.com/yersonargotev/packy/internal/prompt"
)

func (facade *Facade) previewUninstall() (Plan, error) {
	state, found, err := LoadState(facade.config.State.StateFile())
	if err != nil {
		return Plan{}, err
	}
	actions := make([]plannedAction, 0, len(state.ManagedSkills)+4)
	for _, skill := range state.ManagedSkills {
		action, ok, err := previewManagedSkillRemoval(skill)
		if err != nil {
			return Plan{}, err
		}
		if ok {
			actions = append(actions, action)
		}
	}
	if found {
		actions = append(actions, plannedAction{ActionView: ActionView{Kind: ActionRemove, Path: facade.config.State.StateFile(), Description: "remove Packy state metadata"}})
	}
	codex, err := prompt.InspectCodex(facade.config.Codex.PromptFile())
	if err != nil {
		return Plan{}, err
	}
	if codex.HasPackySection {
		actions = append(actions, plannedAction{ActionView: ActionView{Kind: ActionRemoveCodexPrompt, Path: facade.config.Codex.PromptFile(), Description: "remove Codex Packy prompt markers"}})
	}
	openCode, err := opencode.Inspect(facade.config.OpenCode.ConfigFile(), facade.config.OpenCode.PromptFile())
	if err != nil {
		return Plan{}, err
	}
	if openCode.PromptExists || openCode.HasPackyInstruction {
		actions = append(actions, plannedAction{ActionView: ActionView{Kind: ActionRemoveOpenCodePrompt, Path: facade.config.OpenCode.ConfigFile(), Target: facade.config.OpenCode.PromptFile(), Description: "remove OpenCode Packy prompt reference"}})
	}

	cleanup, err := ownedcontainer.Preview(facade.authorizedContainers(state.CreatedContainers))
	if err != nil {
		return Plan{}, err
	}
	for _, record := range cleanup.Records() {
		actions = append(actions, plannedAction{ActionView: ActionView{Kind: ActionCleanup, Path: record.Path, Description: "remove Packy-created container if empty; preserve if non-empty, unmanaged, contributor-owned, or changed after preview"}})
	}
	preconditions, err := ownedcontainer.Preview(uninstallFilePreconditions(facade.config, found, codex.HasPackySection, openCode))
	if err != nil {
		return Plan{}, err
	}
	var claudePlan claudecode.ClassicPlan
	hasClaudePlan := facade.config.Claude != nil && found && !state.Legacy()
	var blockers, preserved, pending []string
	if hasClaudePlan {
		claudePlan, err = facade.config.Claude.InspectClassic(context.Background(), claudecode.ClassicRequest{Goal: claudecode.ClassicAbsent, Desired: facade.config.ClaudeDesired})
		if err != nil {
			return Plan{}, err
		}
		blockers, preserved, pending = claudePlan.Blockers(), claudePlan.Preserved(), claudePlan.PendingPrerequisites()
		for _, action := range claudePlan.Actions() {
			actions = append(actions, plannedAction{ActionView: ActionView{Kind: ActionKind("claude-" + string(action.Kind)), Path: action.Target, Description: action.Description}})
		}
	}
	outcome := OutcomeApplied
	if len(actions) == 0 {
		outcome = OutcomeConverged
	}
	if len(blockers) > 0 {
		outcome = OutcomeBlocked
	}
	if len(pending) > 0 {
		outcome = OutcomeUninstallIncomplete
	}
	return Plan{owner: facade, operation: Uninstall, actions: actions, desired: state, cleanup: cleanup, preconditions: preconditions, hasWork: len(actions) > 0, outcome: outcome, blockers: blockers, preserved: preserved, pending: pending, transition: StateTransitionView{FromSchemaVersion: state.SchemaVersion, FromStatus: state.InstallStatus}, claudePlan: claudePlan, hasClaudePlan: hasClaudePlan}, nil
}

func previewManagedSkillRemoval(skill ManagedSkill) (plannedAction, bool, error) {
	info, err := os.Lstat(skill.LinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return plannedAction{}, false, nil
		}
		return plannedAction{}, false, fmt.Errorf("inspect skill link %s: %w", skill.LinkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return plannedAction{}, false, nil
	}
	target, err := os.Readlink(skill.LinkPath)
	if err != nil {
		return plannedAction{}, false, fmt.Errorf("read skill link %s: %w", skill.LinkPath, err)
	}
	if !sameLinkTarget(skill.LinkPath, target, skill.SourcePath) {
		return plannedAction{}, false, nil
	}
	return plannedAction{ActionView: ActionView{Kind: ActionRemove, Path: skill.LinkPath, Target: target, Description: "remove managed skill " + skill.Name}}, true, nil
}

func uninstallFilePreconditions(config facadeConfig, stateFound, codexOwned bool, openCode opencode.Inspection) []ownedcontainer.Record {
	var records []ownedcontainer.Record
	if stateFound {
		records = append(records, ownedcontainer.Record{Path: config.State.StateFile(), Kind: ownedcontainer.File})
	}
	if codexOwned {
		records = append(records, ownedcontainer.Record{Path: config.Codex.PromptFile(), Kind: ownedcontainer.File})
	}
	if openCode.ConfigExists && openCode.HasPackyInstruction {
		records = append(records, ownedcontainer.Record{Path: config.OpenCode.ConfigFile(), Kind: ownedcontainer.File})
	}
	if openCode.PromptExists {
		records = append(records, ownedcontainer.Record{Path: config.OpenCode.PromptFile(), Kind: ownedcontainer.File})
	}
	return records
}

func (facade *Facade) applyUninstall(plan Plan) (Result, error) {
	if err := plan.preconditions.Verify(); err != nil {
		return Result{}, err
	}
	if err := plan.cleanup.Verify(); err != nil {
		return Result{}, err
	}
	if !plan.hasWork {
		return Result{stateFile: facade.config.State.StateFile(), outcome: OutcomeConverged}, nil
	}
	if plan.hasClaudePlan {
		result, err := facade.config.Claude.ApplyClassic(context.Background(), plan.claudePlan)
		if err != nil {
			state := plan.desired
			state.InstallStatus = InstallRecoveryRequired
			state.LatestAttempt = &LatestAttempt{Operation: Uninstall, Outcome: AttemptRecoveryRequired, CompletedEffects: result.Completed, FailedEffect: result.Failed, NotStartedEffects: result.NotStarted}
			_ = SaveState(facade.config.State.StateFile(), state)
			return Result{outcome: OutcomeRecoveryRequired, completedEffects: result.Completed, failedEffect: result.Failed, notStartedEffects: result.NotStarted}, err
		}
		if len(plan.pending) > 0 || len(plan.blockers) > 0 {
			state := plan.desired
			state.InstallStatus = InstallUninstallIncomplete
			state.LatestAttempt = &LatestAttempt{Operation: Uninstall, Outcome: AttemptUninstallIncomplete, CompletedEffects: result.Completed, NotStartedEffects: result.NotStarted}
			if err := SaveState(facade.config.State.StateFile(), state); err != nil {
				return Result{}, err
			}
		}
	}
	for _, action := range plan.actions {
		if len(plan.pending) > 0 || len(plan.blockers) > 0 {
			if action.Kind == ActionRemove && action.Path == facade.config.State.StateFile() {
				continue
			}
		}
		if strings.HasPrefix(string(action.Kind), "claude-") {
			continue
		}
		switch action.Kind {
		case ActionRemove:
			if action.Path == facade.config.State.StateFile() {
				if err := os.Remove(action.Path); err != nil && !os.IsNotExist(err) {
					return Result{}, fmt.Errorf("remove Packy state %s: %w", action.Path, err)
				}
				continue
			}
			current, ok, err := previewManagedSkillRemoval(ManagedSkill{SourcePath: action.Target, LinkPath: action.Path})
			if err != nil {
				return Result{}, err
			}
			if !ok || current.Target != action.Target {
				continue
			}
			if err := os.Remove(action.Path); err != nil && !os.IsNotExist(err) {
				return Result{}, fmt.Errorf("remove skill symlink %s: %w", action.Path, err)
			}
		case ActionRemoveCodexPrompt:
			if err := prompt.RemoveCodex(action.Path); err != nil {
				return Result{}, err
			}
		case ActionRemoveOpenCodePrompt:
			if err := opencode.Remove(action.Path, action.Target); err != nil {
				return Result{}, err
			}
		}
	}
	if _, err := plan.cleanup.Cleanup(); err != nil {
		return Result{}, err
	}
	if len(plan.pending) > 0 || len(plan.blockers) > 0 {
		return Result{stateFile: facade.config.State.StateFile(), hasWork: true, outcome: OutcomeUninstallIncomplete, completedEffects: actionEffectIDs(plan.actions)}, nil
	}
	return Result{stateFile: facade.config.State.StateFile(), hasWork: true, outcome: OutcomeApplied, completedEffects: actionEffectIDs(plan.actions)}, nil
}

func (facade *Facade) authorizedContainers(records []ownedcontainer.Record) []ownedcontainer.Record {
	allowed := make(map[string]struct{})
	for _, record := range facade.containerRecords() {
		allowed[filepath.Clean(record.Path)] = struct{}{}
	}
	authorized := make([]ownedcontainer.Record, 0, len(records))
	for _, record := range records {
		if _, ok := allowed[filepath.Clean(record.Path)]; ok {
			authorized = append(authorized, record)
		}
	}
	return authorized
}
