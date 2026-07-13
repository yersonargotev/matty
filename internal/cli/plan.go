package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/matty/internal/corelifecycle"
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

func buildDoctorExpectedSkillPlan(paths Paths) (Plan, error) {
	discovered, err := DiscoverManagedSkills(paths)
	if err != nil {
		return Plan{}, err
	}

	var actions []PlannedAction
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
	return Plan{Actions: actions, State: corelifecycle.State{ManagedSkills: managed}}, nil
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

func unmanagedSymlinkRecoveryAdvice() string {
	return "Safe recovery: verify these are stale Matty-created links, remove them, then run matty install; Matty will not overwrite arbitrary files or links."
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
