package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/matty/internal/opencode"
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
	case ActionWriteFile, ActionWriteCodexPrompt, ActionRemove, ActionRemoveCodexPrompt, ActionSkip:
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
}

type Plan struct {
	Actions []PlannedAction
	State   State
}

func BuildInstallPlan(paths Paths, checkedAt time.Time, engramInstalled bool) (Plan, error) {
	discovered, err := DiscoverManagedSkills(paths)
	if err != nil {
		return Plan{}, err
	}

	actions := []PlannedAction{
		{Kind: ActionWriteFile, Path: paths.StateFile, Description: "persist Matty state metadata"},
	}
	managed := make([]ManagedSkill, 0, len(discovered))
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
		actions = append(actions, PlannedAction{Kind: ActionRun, Command: "brew", Args: []string{"install", "gentleman-programming/tap/engram"}, Description: "install Engram via Homebrew"})
	}
	actions = append(actions, engramSetupActions()...)
	actions = append(actions, codexPromptWriteAction(paths), openCodePromptWriteAction(paths))
	return Plan{Actions: actions, State: DesiredState(paths, checkedAt, managed)}, nil
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
	actions = append(actions, engramSetupActions()...)
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

func engramSetupActions() []PlannedAction {
	return []PlannedAction{
		{Kind: ActionRun, Command: "engram", Args: []string{"setup", "codex"}, Description: "delegate Codex Engram setup"},
		{Kind: ActionRun, Command: "engram", Args: []string{"setup", "opencode"}, Description: "delegate OpenCode Engram setup"},
	}
}

func plannedSkillLinkAction(skill ManagedSkill) (PlannedAction, error) {
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
	plannedAction func(ManagedSkill, skillLinkInspection) PlannedAction
	doctorProblem func(ManagedSkill, skillLinkInspection) (skillLinkDoctorProblem, bool)
}

var skillLinkBehaviors = map[skillLinkStatus]skillLinkBehavior{
	skillLinkMissing: {
		plannedAction: func(skill ManagedSkill, _ skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSymlink, Path: skill.LinkPath, Target: skill.SourcePath, Description: "link managed skill " + skill.Name}
		},
		doctorProblem: func(skill ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{missing: true, detail: skill.Name}, true
		},
	},
	skillLinkManaged: {
		plannedAction: func(ManagedSkill, skillLinkInspection) PlannedAction { return PlannedAction{} },
		doctorProblem: func(ManagedSkill, skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{}, false
		},
	},
	skillLinkUnmanagedPath: {
		plannedAction: func(skill ManagedSkill, _ skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSkip, Path: skill.LinkPath, Target: skill.SourcePath, Description: "preserve unmanaged path for skill " + skill.Name}
		},
		doctorProblem: func(skill ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{detail: skill.Name + " is not a symlink"}, true
		},
	},
	skillLinkUnmanagedSymlink: {
		plannedAction: func(skill ManagedSkill, link skillLinkInspection) PlannedAction {
			return PlannedAction{Kind: ActionSkip, Path: skill.LinkPath, Target: link.target, Description: "preserve unmanaged symlink for skill " + skill.Name}
		},
		doctorProblem: func(skill ManagedSkill, _ skillLinkInspection) (skillLinkDoctorProblem, bool) {
			return skillLinkDoctorProblem{detail: skill.Name}, true
		},
	},
}

func inspectSkillLink(skill ManagedSkill) (skillLinkInspection, error) {
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

func BuildUninstallPlan(paths Paths, state State) Plan {
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
	return Plan{Actions: actions, State: state}
}

func UninstallPlanHasWork(paths Paths, state State) bool {
	if pathExists(paths.StateFile) {
		return true
	}
	for _, skill := range state.ManagedSkills {
		link, err := inspectSkillLink(skill)
		if err == nil && link.status == skillLinkManaged {
			return true
		}
	}
	if fileContains(paths.CodexPromptFile, "<!-- matty:skills-router -->") {
		return true
	}
	if pathExists(paths.OpenCodePromptFile) || fileContains(paths.OpenCodeConfigFile, paths.OpenCodePromptFile) {
		return true
	}
	return false
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), needle)
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

func ApplyInstallPlan(ctx context.Context, paths Paths, plan Plan, runner Runner) ([]string, error) {
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
			if err := runner.Run(ctx, action.Command, action.Args...); err != nil {
				return nil, actionRunError(action, err)
			}
		}
	}
	if err := SaveState(paths.StateFile, plan.State); err != nil {
		return nil, err
	}
	return warnings, nil
}

func actionRunError(action PlannedAction, err error) error {
	cmd := strings.Join(append([]string{action.Command}, action.Args...), " ")
	switch {
	case action.Command == "brew" && len(action.Args) > 0 && action.Args[0] == "install":
		return fmt.Errorf("run %s: failed to install Engram via Homebrew; ensure Homebrew is installed and retry: %w", cmd, err)
	case action.Command == "brew" && len(action.Args) > 0 && (action.Args[0] == "update" || action.Args[0] == "upgrade"):
		return fmt.Errorf("run %s: failed to update Engram via Homebrew; ensure Homebrew is installed and retry: %w", cmd, err)
	case action.Command == "engram" && len(action.Args) >= 2 && action.Args[0] == "setup":
		return fmt.Errorf("run %s: failed to configure Engram for %s; install Engram and retry matty install or matty update: %w", cmd, action.Args[1], err)
	default:
		return fmt.Errorf("run %s: %w", cmd, err)
	}
}

func ApplyUninstallPlan(_ context.Context, paths Paths, plan Plan) error {
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
	return nil
}
