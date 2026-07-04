package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type ActionKind string

const (
	ActionWriteFile ActionKind = "write-file"
	ActionSymlink   ActionKind = "symlink"
	ActionRun       ActionKind = "run"
)

// PlannedAction is a human-reportable unit of work. Issue 02 introduces the
// planning model before later issues fill in the concrete installers.
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

func BuildInstallPlan(paths Paths, checkedAt time.Time) Plan {
	state := DesiredState(paths, checkedAt)
	actions := []PlannedAction{
		{Kind: ActionWriteFile, Path: paths.StateFile, Description: "persist Matty state metadata"},
	}
	for _, skill := range state.ManagedSkills {
		actions = append(actions, PlannedAction{Kind: ActionSymlink, Path: skill.LinkPath, Target: skill.SourcePath, Description: "link managed skill " + skill.Name})
	}
	actions = append(actions,
		PlannedAction{Kind: ActionRun, Command: "brew", Args: []string{"install", "engram"}, Description: "install or verify Engram"},
		PlannedAction{Kind: ActionRun, Command: "engram", Args: []string{"setup", "codex"}, Description: "delegate Codex Engram setup"},
		PlannedAction{Kind: ActionRun, Command: "engram", Args: []string{"setup", "opencode"}, Description: "delegate OpenCode Engram setup"},
	)
	return Plan{Actions: actions, State: state}
}

func PrintPlan(w io.Writer, plan Plan) error {
	for _, action := range plan.Actions {
		if _, err := fmt.Fprintf(w, "- %s: %s", action.Kind, action.Description); err != nil {
			return err
		}
		switch action.Kind {
		case ActionWriteFile:
			_, err := fmt.Fprintf(w, " (%s)\n", action.Path)
			if err != nil {
				return err
			}
		case ActionSymlink:
			_, err := fmt.Fprintf(w, " (%s -> %s)\n", action.Path, action.Target)
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

func ApplyStateOnlyPlan(_ context.Context, paths Paths, plan Plan) error {
	if err := os.MkdirAll(paths.MattyDir, 0o700); err != nil {
		return fmt.Errorf("create Matty config directory %s: %w", paths.MattyDir, err)
	}
	return SaveState(paths.StateFile, plan.State)
}
