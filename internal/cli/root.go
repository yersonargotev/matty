package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/matty/internal/bootstrap"
	"github.com/yersonargotev/matty/internal/capabilitypack"
	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/skillbundle"
	mattyversion "github.com/yersonargotev/matty/internal/version"
)

// Options carries injectable process boundaries for tests and future command
// implementations. The zero value uses the real OS environment and runner.
type Options struct {
	Env                 Env
	Runner              Runner
	Clock               func() time.Time
	Terminal            Terminal
	ReadinessInspectors map[capabilitypack.Surface]capabilitypack.ReadinessInspector
	EngramFacts         engrambin.Facts
	DoctorReportBuilder func(Paths, Runner) DoctorReport
}

func (o Options) withDefaults() Options {
	if o.Env == nil {
		o.Env = osEnv{}
	}
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.Clock == nil {
		o.Clock = time.Now
	}
	o.EngramFacts = o.EngramFacts.WithDefaults()
	if o.DoctorReportBuilder == nil {
		facts := o.EngramFacts
		o.DoctorReportBuilder = func(paths Paths, runner Runner) DoctorReport {
			return buildDoctorReport(paths, runner, facts)
		}
	}
	if o.Terminal == nil {
		o.Terminal = processTerminal{}
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
		Version:       mattyversion.Value,
	}

	root.AddCommand(
		newPackCommand(opts),
		newInitCommand(opts),
		newInstallCommand(opts),
		newDoctorCommand(opts),
		newUpdateCommand(opts),
		newUninstallCommand(opts),
	)

	return root
}

func newInitCommand(opts Options) *cobra.Command {
	var (
		homeFlag      string
		sourceRoot    string
		repositoryURL string
		repositoryRef string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Matty's package-installed source checkout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home := strings.TrimSpace(homeFlag)
			if home == "" {
				home = opts.Env.Getenv("HOME")
			}
			if home == "" {
				return fmt.Errorf("HOME is required")
			}
			configHome := ""
			if strings.TrimSpace(homeFlag) == "" {
				configHome = opts.Env.Getenv("XDG_CONFIG_HOME")
			}
			if configHome == "" || !filepath.IsAbs(configHome) {
				configHome = filepath.Join(home, ".config")
			}

			root := strings.TrimSpace(sourceRoot)
			if root == "" {
				root = DefaultInstalledSourceRoot(home)
			}
			root, err := filepath.Abs(root)
			if err != nil {
				return fmt.Errorf("resolve installed source root: %w", err)
			}

			result, err := bootstrap.EnsureInstalledSource(bootstrap.BootstrapOptions{
				SourceRoot:    root,
				RepositoryURL: repositoryURL,
				RepositoryRef: defaultInitRepositoryRef(repositoryRef, mattyversion.Value),
				HomeDir:       home,
				ConfigHome:    configHome,
				ReportProgress: func(message string) error {
					_, err := fmt.Fprintf(cmd.OutOrStdout(), "matty init: %s\n", message)
					return err
				},
			})
			if err != nil {
				return err
			}
			switch {
			case result.Cloned:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty init: initialized Installed Source at %s\n", result.SourceRoot)
			case result.Updated:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty init: updated Installed Source at %s\n", result.SourceRoot)
			default:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty init: Installed Source already initialized at %s\n", result.SourceRoot)
			}
			return err
		},
	}

	cmd.Flags().StringVar(&homeFlag, "home", "", "home directory used to resolve the default Installed Source")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "Installed Source root (default ~/.local/share/matty)")
	cmd.Flags().StringVar(&repositoryURL, "repository-url", bootstrap.DefaultRepositoryURL, "Matty Source of Truth Git URL")
	cmd.Flags().StringVar(&repositoryRef, "repository-ref", "", "optional Matty Source of Truth Git ref to clone or check out")
	return cmd
}

func defaultInitRepositoryRef(explicitRef, currentVersion string) string {
	if strings.TrimSpace(explicitRef) != "" {
		return explicitRef
	}
	if strings.HasPrefix(currentVersion, "v") {
		return currentVersion
	}
	return ""
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
			lifecycle := corelifecycle.NewFacade(classicLifecycleConfig(paths, mattyversion.Value), opts.Runner, opts.Clock)
			plan, err := lifecycle.Preview(corelifecycle.Install)
			if err != nil {
				return err
			}
			if err := printSkillSourceReport(cmd.OutOrStdout(), paths); err != nil {
				return err
			}
			if dryRun {
				return printLifecycleDryRunPlan(cmd.OutOrStdout(), "matty install", plan)
			}
			result, err := lifecycle.Apply(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if err := printWarnings(cmd.OutOrStdout(), result.Warnings()); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty install: synced %d managed skills and wrote state %s\n", result.ManagedSkillCount(), result.StateFile())
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Matty-managed changes without writing files")
	return cmd
}

func newDoctorCommand(opts Options) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check Matty setup without changing files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			report := opts.DoctorReportBuilder(paths, opts.Runner)
			if jsonOutput {
				if err := RenderDoctorJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else if err := RenderDoctorHuman(cmd.OutOrStdout(), report); err != nil {
				return err
			}
			return report.HealthError()
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func newUpdateCommand(opts Options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Refresh Matty-managed tools and configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			lifecycle := corelifecycle.NewFacade(classicLifecycleConfig(paths, mattyversion.Value), opts.Runner, opts.Clock)
			plan, err := lifecycle.Preview(corelifecycle.Update)
			if err != nil {
				return err
			}
			if err := printSkillSourceReport(cmd.OutOrStdout(), paths); err != nil {
				return err
			}
			if dryRun {
				return printLifecycleDryRunPlan(cmd.OutOrStdout(), "matty update", plan)
			}
			result, err := lifecycle.Apply(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if err := printWarnings(cmd.OutOrStdout(), result.Warnings()); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty update: synced %d managed skills and wrote state %s\n", result.ManagedSkillCount(), result.StateFile())
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Matty-managed update changes without writing files or running commands")
	return cmd
}

func printSkillSourceReport(out io.Writer, paths Paths) error {
	switch paths.SkillSourceOrigin {
	case SkillSourceOriginOverride:
		if _, err := fmt.Fprintf(out, "Skill source: explicit override (MATTY_SKILLS_SOURCE=%s)\n", paths.SkillSourceRoot); err != nil {
			return err
		}
	case SkillSourceOriginRepo:
		if _, err := fmt.Fprintf(out, "Skill source: repo checkout (%s)\n", paths.SkillSourceRoot); err != nil {
			return err
		}
		installedSkillSource := skillbundle.SourceRoot(paths.InstalledSourceRoot)
		if skillbundle.SourceRootExists(installedSkillSource) {
			if _, err := fmt.Fprintf(out, "warning: installed source also exists at %s; repo checkout source may create a development-mode install. For package-installed setup, run matty install outside the repo or set MATTY_SKILLS_SOURCE explicitly.\n", installedSkillSource); err != nil {
				return err
			}
		}
	case SkillSourceOriginInstalled:
		if _, err := fmt.Fprintf(out, "Skill source: installed source (%s)\n", paths.SkillSourceRoot); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(out, "Skill source: %s\n", paths.SkillSourceRoot); err != nil {
			return err
		}
	}
	return nil
}

func printDryRunPlan(out io.Writer, command string, plan Plan) error {
	if _, err := fmt.Fprintf(out, "%s dry-run: planned actions\n", command); err != nil {
		return err
	}
	return PrintPlan(out, plan)
}

func printLifecycleDryRunPlan(out io.Writer, command string, plan corelifecycle.Plan) error {
	if _, err := fmt.Fprintf(out, "%s dry-run: planned actions\n", command); err != nil {
		return err
	}
	for _, action := range plan.Actions() {
		if _, err := fmt.Fprintf(out, "- %s: %s", action.Kind, action.Description); err != nil {
			return err
		}
		switch action.Kind {
		case corelifecycle.ActionWriteOpenCodePrompt, corelifecycle.ActionSymlink:
			if _, err := fmt.Fprintf(out, " (%s -> %s)\n", action.Path, action.Target); err != nil {
				return err
			}
		case corelifecycle.ActionRun:
			if _, err := fmt.Fprintf(out, " (%s)\n", strings.Join(append([]string{action.Command}, action.Args...), " ")); err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintf(out, " (%s)\n", action.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func printWarnings(out io.Writer, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(out, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func newUninstallCommand(opts Options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove only Matty-managed artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			state, _, err := corelifecycle.LoadState(paths.StateFile)
			if err != nil {
				return err
			}
			plan := BuildUninstallPlan(paths, state)
			hasWork := UninstallPlanHasWork(paths, state)
			if dryRun {
				return printDryRunPlan(cmd.OutOrStdout(), "matty uninstall", plan)
			}
			if !hasWork {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "matty uninstall: no Matty-managed artifacts found")
				return err
			}
			if err := ApplyUninstallPlan(cmd.Context(), paths, plan); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty uninstall: removed Matty-managed artifacts and state %s\n", paths.StateFile)
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Matty-managed removals without deleting files")
	return cmd
}
