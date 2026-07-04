package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

const version = "0.0.0-dev"

// Options carries injectable process boundaries for tests and future command
// implementations. The zero value uses the real OS environment and runner.
type Options struct {
	Env    Env
	Runner Runner
}

func (o Options) withDefaults() Options {
	if o.Env == nil {
		o.Env = osEnv{}
	}
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	return o
}

// NewRootCommand constructs the Matty CLI command tree.
func NewRootCommand(opts Options) *cobra.Command {
	opts = opts.withDefaults()

	root := &cobra.Command{
		Use:           "matty",
		Short:         "Install and configure the Matty AI coding workflow",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.AddCommand(
		newInstallCommand(opts),
		newDoctorCommand(opts),
		newUpdateCommand(opts),
		newUninstallCommand(opts),
	)

	return root
}

func newInstallCommand(opts Options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Matty-managed global workflow configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			if _, _, err := LoadState(paths.StateFile); err != nil {
				return err
			}

			plan, err := BuildInstallPlan(paths, time.Now())
			if err != nil {
				return err
			}
			if dryRun {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "matty install dry-run: planned actions"); err != nil {
					return err
				}
				return PrintPlan(cmd.OutOrStdout(), plan)
			}
			if err := ApplyInstallPlan(cmd.Context(), paths, plan); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty install: synced %d managed skills and wrote state %s\n", len(plan.State.ManagedSkills), paths.StateFile)
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Matty-managed changes without writing files")
	return cmd
}

func newDoctorCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check Matty setup without changing files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			_, found, err := LoadState(paths.StateFile)
			if err != nil {
				return err
			}
			stateStatus := "missing"
			if found {
				stateStatus = "present"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "HOME=%s\nCONFIG_HOME=%s\nMATTY_STATE=%s\nMATTY_STATE_STATUS=%s\nAGENT_SKILLS=%s\n", paths.HomeDir, paths.ConfigHome, paths.StateFile, stateStatus, paths.AgentSkillsDir)
			return err
		},
	}
}

func newUpdateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Refresh Matty-managed tools and configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			if _, _, err := LoadState(paths.StateFile); err != nil {
				return err
			}
			plan, err := BuildInstallPlan(paths, time.Now())
			if err != nil {
				return err
			}
			if err := ApplyInstallPlan(cmd.Context(), paths, plan); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty update: synced %d managed skills and wrote state %s\n", len(plan.State.ManagedSkills), paths.StateFile)
			return err
		},
	}
}

func newUninstallCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove only Matty-managed artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			state, found, err := LoadState(paths.StateFile)
			if err != nil {
				return err
			}
			if !found {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty uninstall: no Matty state found at %s\n", paths.StateFile)
				return err
			}
			plan := BuildUninstallPlan(paths, state)
			if err := ApplyUninstallPlan(cmd.Context(), paths, plan); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty uninstall: removed managed skill symlinks and state %s\n", paths.StateFile)
			return err
		},
	}
}
