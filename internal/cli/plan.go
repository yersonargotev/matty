package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ActionKind string

const (
	ActionWriteFile ActionKind = "write-file"
	ActionSymlink   ActionKind = "symlink"
	ActionRemove    ActionKind = "remove"
	ActionRun       ActionKind = "run"
	ActionSkip      ActionKind = "skip"
)

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
		if action.Kind == ActionRun {
			continue
		}
		actions = append(actions, action)
	}
	actions = append(actions, engramSetupActions()...)
	plan.Actions = actions
	return plan, nil
}

func engramSetupActions() []PlannedAction {
	return []PlannedAction{
		{Kind: ActionRun, Command: "engram", Args: []string{"setup", "codex"}, Description: "delegate Codex Engram setup"},
		{Kind: ActionRun, Command: "engram", Args: []string{"setup", "opencode"}, Description: "delegate OpenCode Engram setup"},
	}
}

func plannedSkillLinkAction(skill ManagedSkill) (PlannedAction, error) {
	info, err := os.Lstat(skill.LinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return PlannedAction{Kind: ActionSymlink, Path: skill.LinkPath, Target: skill.SourcePath, Description: "link managed skill " + skill.Name}, nil
		}
		return PlannedAction{}, fmt.Errorf("inspect skill link %s: %w", skill.LinkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return PlannedAction{Kind: ActionSkip, Path: skill.LinkPath, Target: skill.SourcePath, Description: "preserve unmanaged path for skill " + skill.Name}, nil
	}
	target, err := os.Readlink(skill.LinkPath)
	if err != nil {
		return PlannedAction{}, fmt.Errorf("read skill link %s: %w", skill.LinkPath, err)
	}
	if sameSymlinkTarget(skill.LinkPath, target, skill.SourcePath) {
		return PlannedAction{}, nil
	}
	return PlannedAction{Kind: ActionSkip, Path: skill.LinkPath, Target: target, Description: "preserve unmanaged symlink for skill " + skill.Name}, nil
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
		info, err := os.Lstat(skill.LinkPath)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(skill.LinkPath)
		if err != nil {
			continue
		}
		if sameSymlinkTarget(skill.LinkPath, target, skill.SourcePath) {
			actions = append(actions, PlannedAction{Kind: ActionRemove, Path: skill.LinkPath, Target: skill.SourcePath, Description: "remove managed skill " + skill.Name})
		}
	}
	actions = append(actions, PlannedAction{Kind: ActionRemove, Path: paths.StateFile, Description: "remove Matty state metadata"})
	return Plan{Actions: actions, State: state}
}

func PrintPlan(w io.Writer, plan Plan) error {
	for _, action := range plan.Actions {
		if _, err := fmt.Fprintf(w, "- %s: %s", action.Kind, action.Description); err != nil {
			return err
		}
		switch action.Kind {
		case ActionWriteFile, ActionRemove:
			_, err := fmt.Fprintf(w, " (%s)\n", action.Path)
			if err != nil {
				return err
			}
		case ActionSymlink:
			_, err := fmt.Fprintf(w, " (%s -> %s)\n", action.Path, action.Target)
			if err != nil {
				return err
			}
		case ActionSkip:
			_, err := fmt.Fprintf(w, " (%s)\n", action.Path)
			if err != nil {
				return err
			}
		case ActionRun:
			cmd := strings.Join(append([]string{action.Command}, action.Args...), " ")
			_, err := fmt.Fprintf(w, " (%s)\n", cmd)
			if err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

func ApplyInstallPlan(ctx context.Context, paths Paths, plan Plan, runner Runner) error {
	if err := os.MkdirAll(paths.MattyDir, 0o700); err != nil {
		return fmt.Errorf("create Matty config directory %s: %w", paths.MattyDir, err)
	}
	if err := os.MkdirAll(paths.AgentSkillsDir, 0o700); err != nil {
		return fmt.Errorf("create agent skills directory %s: %w", paths.AgentSkillsDir, err)
	}
	for _, action := range plan.Actions {
		switch action.Kind {
		case ActionSymlink:
			if err := os.Symlink(action.Target, action.Path); err != nil {
				return fmt.Errorf("create skill symlink %s -> %s: %w", action.Path, action.Target, err)
			}
		case ActionRun:
			if err := runner.Run(ctx, action.Command, action.Args...); err != nil {
				return actionRunError(action, err)
			}
		}
	}
	return SaveState(paths.StateFile, plan.State)
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
		if action.Kind != ActionRemove {
			continue
		}
		if action.Path == paths.StateFile {
			if err := os.Remove(action.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove Matty state %s: %w", action.Path, err)
			}
			continue
		}
		if err := os.Remove(action.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove skill symlink %s: %w", action.Path, err)
		}
	}
	return nil
}
