package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/ownedcontainer"
	"github.com/yersonargotev/matty/internal/prompt"
)

type ActionKind string

const (
	ActionWriteFile            ActionKind = "write-file"
	ActionWriteCodexPrompt     ActionKind = "write-codex-prompt"
	ActionWriteOpenCodePrompt  ActionKind = "write-opencode-prompt"
	ActionSymlink              ActionKind = "symlink"
	ActionRemove               ActionKind = "remove"
	ActionRemoveCodexPrompt    ActionKind = "remove-codex-prompt"
	ActionRemoveOpenCodePrompt ActionKind = "remove-opencode-prompt"
	ActionRun                  ActionKind = "run"
	ActionSkip                 ActionKind = "skip"
	ActionCleanup              ActionKind = "cleanup"
)

func (kind ActionKind) refreshesDuringUpdate() bool {
	return kind == ActionRun || kind == ActionWriteCodexPrompt || kind == ActionWriteOpenCodePrompt
}

func (kind ActionKind) appliesDuringUninstall() bool {
	return kind == ActionRemove || kind == ActionRemoveCodexPrompt || kind == ActionRemoveOpenCodePrompt
}

func (action PlannedAction) printDetail(w io.Writer) error {
	switch action.Kind {
	case ActionWriteOpenCodePrompt, ActionRemoveOpenCodePrompt, ActionSymlink:
		_, err := fmt.Fprintf(w, " (%s -> %s)\n", action.Path, action.Target)
		return err
	case ActionRun:
		cmd := strings.Join(append([]string{action.Command}, action.Args...), " ")
		_, err := fmt.Fprintf(w, " (%s)\n", cmd)
		return err
	case ActionWriteFile, ActionWriteCodexPrompt, ActionRemove, ActionRemoveCodexPrompt, ActionSkip, ActionCleanup:
		_, err := fmt.Fprintf(w, " (%s)\n", action.Path)
		return err
	default:
		_, err := fmt.Fprintln(w)
		return err
	}
}

// PlannedAction is a human-reportable unit of work. Issue 02 introduced the
// planning model; later issues add concrete installers behind this seam.
type PlannedAction struct {
	Kind        ActionKind
	Path        string
	Target      string
	Command     string
	Args        []string
	Description string
	skipReason  skillLinkStatus
}

type Plan struct {
	Actions []PlannedAction
	State   corelifecycle.State
	cleanup ownedcontainer.Plan
}

func BuildInstallPlan(paths Paths, checkedAt time.Time, engramInstalled bool) (Plan, error) {
	discovered, err := DiscoverManagedSkills(paths)
	if err != nil {
		return Plan{}, err
	}

	actions := []PlannedAction{
		{Kind: ActionWriteFile, Path: paths.StateFile, Description: "persist Matty state metadata"},
	}
	managed := make([]corelifecycle.ManagedSkill, 0, len(discovered))
	for _, skill := range discovered {
		status, err := plannedSkillLinkAction(skill)
		if err != nil {
			return Plan{}, err
		}
		if status.Kind != "" {
			actions = append(actions, status)
		}
		if status.Kind != ActionSkip {
			managed = append(managed, skill)
		}
	}
	if !engramInstalled {
		actions = append(actions, PlannedAction{Kind: ActionRun, Command: "brew", Args: []string{"install", engrambin.Formula}, Description: "install Engram via Homebrew"})
	}
	actions = append(actions, engramSetupActions(paths)...)
	actions = append(actions, codexPromptWriteAction(paths), openCodePromptWriteAction(paths))
	return Plan{Actions: actions, State: corelifecycle.DesiredState(classicStateConfig(paths), checkedAt, managed)}, nil
}

func BuildUpdatePlan(paths Paths, checkedAt time.Time) (Plan, error) {
	plan, err := BuildInstallPlan(paths, checkedAt, true)
	if err != nil {
		return Plan{}, err
	}
	actions := make([]PlannedAction, 0, len(plan.Actions)+2)
	actions = append(actions, PlannedAction{Kind: ActionRun, Command: "brew", Args: []string{"update"}, Description: "refresh Homebrew formula metadata"})
	actions = append(actions, PlannedAction{Kind: ActionRun, Command: "brew", Args: []string{"upgrade", "engram"}, Description: "update Engram via Homebrew"})
	for _, action := range plan.Actions {
		if action.Kind.refreshesDuringUpdate() {
			continue
		}
		actions = append(actions, action)
	}
	actions = append(actions, engramSetupActions(paths)...)
	actions = append(actions, codexPromptWriteAction(paths), openCodePromptWriteAction(paths))
	plan.Actions = actions
	return plan, nil
}

func codexPromptWriteAction(paths Paths) PlannedAction {
	return PlannedAction{Kind: ActionWriteCodexPrompt, Path: paths.CodexPromptFile, Description: "write Codex Matty prompt markers"}
}

func openCodePromptWriteAction(paths Paths) PlannedAction {
	return PlannedAction{Kind: ActionWriteOpenCodePrompt, Path: paths.OpenCodeConfigFile, Target: paths.OpenCodePromptFile, Description: "write OpenCode Matty prompt reference"}
}

func engramSetupActions(paths Paths) []PlannedAction {
	engram := engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv)
	return []PlannedAction{
		{Kind: ActionRun, Command: engram, Args: []string{"setup", "codex"}, Description: "delegate Codex Engram setup through Homebrew binary"},
		{Kind: ActionRun, Command: engram, Args: []string{"setup", "opencode"}, Description: "delegate OpenCode Engram setup through Homebrew binary"},
	}
}

func plannedSkillLinkAction(skill corelifecycle.ManagedSkill) (PlannedAction, error) {
	link, err := inspectSkillLink(skill)
	if err != nil {
		return PlannedAction{}, err
	}
	behavior, ok := skillLinkBehaviors[link.status]
	if !ok {
		return PlannedAction{}, fmt.Errorf("inspect skill link %s: unknown status %s", skill.LinkPath, link.status)
	}
	return behavior.plannedAction(skill, link), nil
}

type skillLinkStatus string

const (
	skillLinkMissing          skillLinkStatus = "missing"
	skillLinkManaged          skillLinkStatus = "managed"
	skillLinkUnmanagedPath    skillLinkStatus = "unmanaged-path"
	skillLinkUnmanagedSymlink skillLinkStatus = "unmanaged-symlink"
)

type skillLinkInspection struct {
	status skillLinkStatus
	target string
}

type skillLinkDoctorProblem struct {
	missing bool
	detail  string
}

type skillLinkBehavior struct {
	plannedAction func(corelifecycle.ManagedSkill, skillLinkInspection) PlannedAction
	doctorProblem func(corelifecycle.ManagedSkill, skillLinkInspection) (skillLinkDoctorProblem, bool)
}

var skillLinkBehaviors = map[skillLinkStatus]skillLinkBehavior{
	skillLinkMissing: {
		plannedAction: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSymlink, Path: skill.LinkPath, Target: skill.SourcePath, Description: "link managed skill " + skill.Name}
		},
		doctorProblem: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{missing: true, detail: skill.Name}, true
		},
	},
	skillLinkManaged: {
		plannedAction: func(corelifecycle.ManagedSkill, skillLinkInspection) PlannedAction { return PlannedAction{} },
		doctorProblem: func(corelifecycle.ManagedSkill, skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{}, false
		},
	},
	skillLinkUnmanagedPath: {
		plannedAction: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSkip, Path: skill.LinkPath, Target: skill.SourcePath, Description: "preserve unmanaged path for skill " + skill.Name, skipReason: skillLinkUnmanagedPath}
		},
		doctorProblem: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{detail: skill.Name + " is not a symlink"}, true
		},
	},
	skillLinkUnmanagedSymlink: {
		plannedAction: func(skill corelifecycle.ManagedSkill, link skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSkip, Path: skill.LinkPath, Target: link.target, Description: "preserve unmanaged symlink for skill " + skill.Name, skipReason: skillLinkUnmanagedSymlink}
		},
		doctorProblem: func(skill corelifecycle.ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{detail: skill.Name}, true
		},
	},
}

func inspectSkillLink(skill corelifecycle.ManagedSkill) (skillLinkInspection, error) {
	info, err := os.Lstat(skill.LinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skillLinkInspection{status: skillLinkMissing}, nil
		}
		return skillLinkInspection{}, fmt.Errorf("inspect skill link %s: %w", skill.LinkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return skillLinkInspection{status: skillLinkUnmanagedPath}, nil
	}
	target, err := os.Readlink(skill.LinkPath)
	if err != nil {
		return skillLinkInspection{}, fmt.Errorf("read skill link %s: %w", skill.LinkPath, err)
	}
	if sameSymlinkTarget(skill.LinkPath, target, skill.SourcePath) {
		return skillLinkInspection{status: skillLinkManaged, target: target}, nil
	}
	return skillLinkInspection{status: skillLinkUnmanagedSymlink, target: target}, nil
}

func sameSymlinkTarget(linkPath, gotTarget, wantTarget string) bool {
	if gotTarget == wantTarget {
		return true
	}
	if !filepath.IsAbs(gotTarget) {
		gotTarget = filepath.Join(filepath.Dir(linkPath), gotTarget)
	}
	gotAbs, gotErr := filepath.Abs(gotTarget)
	wantAbs, wantErr := filepath.Abs(wantTarget)
	return gotErr == nil && wantErr == nil && gotAbs == wantAbs
}

func BuildUninstallPlan(paths Paths, state corelifecycle.State) Plan {
	actions := make([]PlannedAction, 0, len(state.ManagedSkills)+1)
	for _, skill := range state.ManagedSkills {
		link, err := inspectSkillLink(skill)
		if err == nil && link.status == skillLinkManaged {
			actions = append(actions, PlannedAction{Kind: ActionRemove, Path: skill.LinkPath, Target: link.target, Description: "remove managed skill " + skill.Name})
		}
	}
	actions = append(actions, PlannedAction{Kind: ActionRemove, Path: paths.StateFile, Description: "remove Matty state metadata"})
	actions = append(actions, PlannedAction{Kind: ActionRemoveCodexPrompt, Path: paths.CodexPromptFile, Description: "remove Codex Matty prompt markers"})
	actions = append(actions, PlannedAction{Kind: ActionRemoveOpenCodePrompt, Path: paths.OpenCodeConfigFile, Target: paths.OpenCodePromptFile, Description: "remove OpenCode Matty prompt reference"})
	cleanup, _ := ownedcontainer.Preview(authorizedContainers(paths, state.CreatedContainers))
	for _, record := range cleanup.Records() {
		actions = append(actions, PlannedAction{Kind: ActionCleanup, Path: record.Path, Description: "remove Matty-created container if empty; preserve if non-empty, unmanaged, contributor-owned, or changed after preview"})
	}
	return Plan{Actions: actions, State: state, cleanup: cleanup}
}

func UninstallPlanHasWork(paths Paths, state corelifecycle.State) bool {
	if pathExists(paths.StateFile) {
		return true
	}
	for _, skill := range state.ManagedSkills {
		link, err := inspectSkillLink(skill)
		if err == nil && link.status == skillLinkManaged {
			return true
		}
	}
	codex, _ := prompt.InspectCodex(paths.CodexPromptFile)
	if codex.HasMattySection {
		return true
	}
	opencodeConfig, _ := opencode.Inspect(paths.OpenCodeConfigFile, paths.OpenCodePromptFile)
	return opencodeConfig.PromptExists || opencodeConfig.HasMattyInstruction
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func PrintPlan(w io.Writer, plan Plan) error {
	for _, action := range plan.Actions {
		if _, err := fmt.Fprintf(w, "- %s: %s", action.Kind, action.Description); err != nil {
			return err
		}
		if err := action.printDetail(w); err != nil {
			return err
		}
	}
	return nil
}

type unmanagedSymlinkSummary struct {
	count   int
	example PlannedAction
}

func unmanagedSymlinkSkipSummary(plan Plan) (unmanagedSymlinkSummary, bool) {
	var summary unmanagedSymlinkSummary
	skipped := 0
	for _, action := range plan.Actions {
		if action.Kind != ActionSkip {
			continue
		}
		skipped++
		if action.skipReason == skillLinkUnmanagedSymlink {
			if summary.count == 0 {
				summary.example = action
			}
			summary.count++
		}
	}
	if summary.count == 0 {
		return unmanagedSymlinkSummary{}, false
	}
	expectedSkillLinks := len(plan.State.ManagedSkills) + skipped
	if !isMostExpectedSkillLinks(summary.count, expectedSkillLinks) {
		return unmanagedSymlinkSummary{}, false
	}
	return summary, true
}

func isMostExpectedSkillLinks(count, expectedSkillLinks int) bool {
	return expectedSkillLinks > 0 && count*2 > expectedSkillLinks
}

func unmanagedSymlinkRecoveryWarning(plan Plan) (string, bool) {
	summary, ok := unmanagedSymlinkSkipSummary(plan)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("skipped %d unmanaged skill symlinks; setup may be incomplete. Example: %s -> %s. %s", summary.count, summary.example.Path, summary.example.Target, unmanagedSymlinkRecoveryAdvice()), true
}

func unmanagedSymlinkRecoveryAdvice() string {
	return "Safe recovery: verify these are stale Matty-created links, remove them, then run matty install; Matty will not overwrite arbitrary files or links."
}

// persistClassicState remains injectable while the CLI owns operation
// sequencing. The install-facade slice moves this seam into corelifecycle.
var persistClassicState = corelifecycle.SaveState

func ApplyInstallPlan(ctx context.Context, paths Paths, plan Plan, runner Runner) ([]string, error) {
	previous, previousFound, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil {
		return nil, err
	}
	anchor, err := provisionStateAnchor(paths)
	if err != nil {
		return nil, err
	}
	recovery := recoveryState(plan.State, previous, previousFound, anchor)
	if err := persistClassicState(paths.StateFile, recovery); err != nil {
		if cleanupErr := cleanupUnrecordedContainers(anchor); cleanupErr != nil {
			return nil, fmt.Errorf("%w; clean up unrecorded Matty containers: %v", err, cleanupErr)
		}
		return nil, err
	}
	created, provisionErr := ownedcontainer.Provision(effectContainerRecords(paths))
	recovery.CreatedContainers = ownedcontainer.Merge(recovery.CreatedContainers, created)
	if err := persistClassicState(paths.StateFile, recovery); err != nil {
		if cleanupErr := cleanupUnrecordedContainers(created); cleanupErr != nil {
			return nil, fmt.Errorf("%w; clean up unrecorded Matty containers: %v", err, cleanupErr)
		}
		return nil, err
	}
	if provisionErr != nil {
		return nil, provisionErr
	}
	if err := os.MkdirAll(paths.MattyDir, 0o700); err != nil {
		return nil, fmt.Errorf("create Matty config directory %s: %w", paths.MattyDir, err)
	}
	if err := os.MkdirAll(paths.AgentSkillsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create agent skills directory %s: %w", paths.AgentSkillsDir, err)
	}
	var warnings []string
	for _, action := range plan.Actions {
		switch action.Kind {
		case ActionSymlink:
			if err := os.Symlink(action.Target, action.Path); err != nil {
				return nil, fmt.Errorf("create skill symlink %s -> %s: %w", action.Path, action.Target, err)
			}
			recovery.ManagedSkills = append(recovery.ManagedSkills, managedSkillForAction(plan.State.ManagedSkills, action))
			if err := persistClassicState(paths.StateFile, recovery); err != nil {
				if removeErr := os.Remove(action.Path); removeErr != nil {
					return nil, fmt.Errorf("%w; roll back unrecorded skill symlink %s: %v", err, action.Path, removeErr)
				}
				return nil, err
			}
		case ActionWriteCodexPrompt:
			result, err := prompt.WriteCodex(action.Path)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, result.Warnings...)
		case ActionWriteOpenCodePrompt:
			result, err := opencode.Write(action.Path, action.Target)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, result.Warnings...)
		case ActionRun:
			if isEngramSetupAction(action) {
				canonical := engrambin.DiscoverHomebrew(paths.HomebrewPrefixEnv)
				if canonical == nil {
					return nil, missingCanonicalEngramSetupError(action, engrambin.HomebrewCandidatePaths(engrambin.HomebrewPrefixes(paths.HomebrewPrefixEnv)))
				}
				action.Command = canonical.Path
			}
			if err := runner.Run(ctx, action.Command, action.Args...); err != nil {
				return nil, actionRunError(action, err)
			}
		}
	}
	if previous.RecoveryRequired() {
		plan.State.ManagedSkills = append([]corelifecycle.ManagedSkill(nil), recovery.ManagedSkills...)
	}
	plan.State.CreatedContainers = append([]ownedcontainer.Record(nil), recovery.CreatedContainers...)
	plan.State.InstallStatus = corelifecycle.InstallConfirmed
	if err := persistClassicState(paths.StateFile, plan.State); err != nil {
		return nil, err
	}
	return warnings, nil
}

func provisionStateAnchor(paths Paths) ([]ownedcontainer.Record, error) {
	var created []ownedcontainer.Record
	if _, err := os.Lstat(paths.MattyDir); os.IsNotExist(err) {
		if err := os.Mkdir(paths.MattyDir, 0o700); err != nil {
			return nil, fmt.Errorf("create Matty config directory %s: %w", paths.MattyDir, err)
		}
		created = append(created, ownedcontainer.Record{Path: paths.MattyDir, Kind: ownedcontainer.Directory})
	} else if err != nil {
		return nil, fmt.Errorf("inspect Matty config directory %s: %w", paths.MattyDir, err)
	}
	if _, err := os.Lstat(paths.StateFile); os.IsNotExist(err) {
		created = append(created, ownedcontainer.Record{Path: paths.StateFile, Kind: ownedcontainer.File})
	} else if err != nil {
		return nil, fmt.Errorf("inspect Matty state %s: %w", paths.StateFile, err)
	}
	return created, nil
}

func effectContainerRecords(paths Paths) []ownedcontainer.Record {
	records := containerRecords(paths)
	out := make([]ownedcontainer.Record, 0, len(records)-2)
	for _, record := range records {
		if record.Path != paths.MattyDir && record.Path != paths.StateFile {
			out = append(out, record)
		}
	}
	return out
}

func cleanupUnrecordedContainers(created []ownedcontainer.Record) error {
	cleanup, err := ownedcontainer.Preview(created)
	if err != nil {
		return err
	}
	_, err = cleanup.Cleanup()
	return err
}

func recoveryState(desired, previous corelifecycle.State, previousFound bool, created []ownedcontainer.Record) corelifecycle.State {
	recovery := desired
	recovery.InstallStatus = corelifecycle.InstallRecoveryRequired
	recovery.ManagedSkills = nil
	if previousFound {
		for _, skill := range previous.ManagedSkills {
			link, err := inspectSkillLink(skill)
			if err == nil && link.status == skillLinkManaged {
				recovery.ManagedSkills = append(recovery.ManagedSkills, skill)
			}
		}
	}
	recovery.CreatedContainers = ownedcontainer.Merge(previous.CreatedContainers, created)
	return recovery
}

func managedSkillForAction(skills []corelifecycle.ManagedSkill, action PlannedAction) corelifecycle.ManagedSkill {
	for _, skill := range skills {
		if skill.LinkPath == action.Path && skill.SourcePath == action.Target {
			return skill
		}
	}
	panic("install plan symlink has no matching managed skill")
}

func isEngramSetupAction(action PlannedAction) bool {
	return filepath.Base(action.Command) == "engram" && len(action.Args) >= 2 && action.Args[0] == "setup"
}

func missingCanonicalEngramSetupError(action PlannedAction, candidates []string) error {
	return fmt.Errorf("run %s: canonical Homebrew Engram was not found at any expected Homebrew path (%s); run brew install %s or set HOMEBREW_PREFIX to the active Homebrew prefix, then retry matty install or matty update", strings.Join(append([]string{action.Command}, action.Args...), " "), strings.Join(candidates, ", "), engrambin.Formula)
}

func actionRunError(action PlannedAction, err error) error {
	cmd := strings.Join(append([]string{action.Command}, action.Args...), " ")
	switch {
	case action.Command == "brew" && len(action.Args) > 0 && action.Args[0] == "install":
		return fmt.Errorf("run %s: failed to install Engram via Homebrew; ensure Homebrew is installed and retry: %w", cmd, err)
	case action.Command == "brew" && len(action.Args) > 0 && (action.Args[0] == "update" || action.Args[0] == "upgrade"):
		return fmt.Errorf("run %s: failed to update Engram via Homebrew; ensure Homebrew is installed and retry: %w", cmd, err)
	case isEngramSetupAction(action):
		return fmt.Errorf("run %s: failed to configure Engram for %s through the Homebrew-managed binary; run brew install %s or brew upgrade engram, then retry matty install or matty update: %w", cmd, action.Args[1], engrambin.Formula, err)
	default:
		return fmt.Errorf("run %s: %w", cmd, err)
	}
}

func ApplyUninstallPlan(_ context.Context, paths Paths, plan Plan) error {
	if err := plan.cleanup.Verify(); err != nil {
		return err
	}
	for _, action := range plan.Actions {
		if !action.Kind.appliesDuringUninstall() {
			continue
		}
		if action.Path == paths.StateFile {
			if err := os.Remove(action.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove Matty state %s: %w", action.Path, err)
			}
			continue
		}
		if action.Kind == ActionRemoveCodexPrompt {
			if err := prompt.RemoveCodex(action.Path); err != nil {
				return err
			}
			continue
		}
		if action.Kind == ActionRemoveOpenCodePrompt {
			if err := opencode.Remove(action.Path, action.Target); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(action.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove skill symlink %s: %w", action.Path, err)
		}
	}
	if _, err := plan.cleanup.Cleanup(); err != nil {
		return err
	}
	return nil
}

func containerRecords(paths Paths) []ownedcontainer.Record {
	return []ownedcontainer.Record{
		{Path: paths.MattyDir, Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(paths.AgentSkillsDir), Kind: ownedcontainer.Directory},
		{Path: paths.AgentSkillsDir, Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(paths.CodexPromptFile), Kind: ownedcontainer.Directory},
		{Path: paths.ConfigHome, Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(paths.OpenCodeConfigFile), Kind: ownedcontainer.Directory},
		{Path: paths.StateFile, Kind: ownedcontainer.File},
		{Path: paths.CodexPromptFile, Kind: ownedcontainer.File},
		{Path: paths.OpenCodeConfigFile, Kind: ownedcontainer.File},
		{Path: paths.OpenCodePromptFile, Kind: ownedcontainer.File},
	}
}

func authorizedContainers(paths Paths, records []ownedcontainer.Record) []ownedcontainer.Record {
	allowed := make(map[string]struct{})
	for _, record := range containerRecords(paths) {
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
