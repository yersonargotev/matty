package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/matty/internal/bootstrap"
	"github.com/yersonargotev/matty/internal/capabilitypack"
	"github.com/yersonargotev/matty/internal/codex"
	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/setuphealth"
	"github.com/yersonargotev/matty/internal/skillbundle"
	mattyversion "github.com/yersonargotev/matty/internal/version"
	"github.com/yersonargotev/matty/internal/workstation"
)

// Options carries injectable process boundaries for tests and future command
// implementations. The zero value uses the real OS environment and runner.
type Options struct {
	Env                 Env
	Getwd               func() (string, error)
	Runner              Runner
	Clock               func() time.Time
	Terminal            Terminal
	SurfaceAdapters     map[capabilitypack.Surface]capabilitypack.SurfaceAdapter
	EngramFacts         engrambin.Facts
	SetupHealthDiagnose func() (setuphealth.Report, error)
}

func (o Options) withDefaults() Options {
	if o.Env == nil {
		o.Env = osEnv{}
	}
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.Getwd == nil {
		o.Getwd = os.Getwd
	}
	if o.Clock == nil {
		o.Clock = time.Now
	}
	o.EngramFacts = o.EngramFacts.WithDefaults()
	if o.Terminal == nil {
		o.Terminal = processTerminal{}
	}
	return o
}

// NewRootCommand constructs the Matty CLI command tree.
func NewRootCommand(opts Options) *cobra.Command {
	opts = opts.withDefaults()
	workstationResolver := newWorkstationResolver(opts)

	root := &cobra.Command{
		Use:           "matty",
		Short:         "Install and configure the Matty AI coding workflow",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       mattyversion.Value,
	}

	root.AddCommand(
		newPackCommand(opts, workstationResolver),
		newInitCommand(opts, workstationResolver),
		newInstallCommand(opts, workstationResolver),
		newDoctorCommand(opts, workstationResolver),
		newUpdateCommand(opts, workstationResolver),
		newUninstallCommand(opts, workstationResolver),
	)

	return root
}

func newInitCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
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
			snapshot, err := workstationResolver.Resolve(workstation.Options{Home: strings.TrimSpace(homeFlag)})
			if err != nil {
				return err
			}
			installedSource, err := bootstrap.ResolveInstalledSource(snapshot, sourceRoot)
			if err != nil {
				return err
			}

			result, err := bootstrap.EnsureInstalledSource(bootstrap.BootstrapOptions{
				InstalledSource: installedSource,
				RepositoryURL:   repositoryURL,
				RepositoryRef:   defaultInitRepositoryRef(repositoryRef, mattyversion.Value),
				HomeDir:         snapshot.Home(),
				ConfigHome:      snapshot.ConfigurationHome(),
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

func newWorkstationResolver(opts Options) *workstation.Resolver {
	return workstation.NewResolver(func() (workstation.Inputs, error) {
		cwd, err := opts.Getwd()
		return workstation.Inputs{
			Home:                 opts.Env.Getenv("HOME"),
			ConfigurationHome:    opts.Env.Getenv("XDG_CONFIG_HOME"),
			ExecutableSearchPath: opts.Env.Getenv("PATH"),
			HomebrewPrefix:       opts.Env.Getenv("HOMEBREW_PREFIX"),
			CurrentDirectory:     cwd,
			CurrentDirectoryErr:  err,
		}, nil
	})
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

func newInstallCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Matty-managed global workflow configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			composition, err := resolveClassicLifecycle(opts, workstationResolver)
			if err != nil {
				return err
			}
			lifecycle := corelifecycle.NewFacade(composition.config, opts.Runner, opts.Clock)
			plan, err := lifecycle.Preview(corelifecycle.Install)
			if err != nil {
				return err
			}
			if err := printSkillSourceReport(cmd.OutOrStdout(), composition.skillSource, composition.installedSource); err != nil {
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

func newDoctorCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check Matty setup without changing files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				report setuphealth.Report
				err    error
			)
			if opts.SetupHealthDiagnose != nil {
				report, err = opts.SetupHealthDiagnose()
			} else {
				report, err = diagnoseSetupHealth(opts, workstationResolver)
			}
			if err != nil {
				return err
			}
			if jsonOutput {
				if err := renderSetupHealthJSON(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			} else if err := renderSetupHealthHuman(cmd.OutOrStdout(), report); err != nil {
				return err
			}
			return setupHealthError(report)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func diagnoseSetupHealth(opts Options, resolver *workstation.Resolver) (setuphealth.Report, error) {
	snapshot, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		return setuphealth.Report{}, err
	}
	installedSource, err := bootstrap.ResolveInstalledSource(snapshot, "")
	if err != nil {
		return setuphealth.Report{}, err
	}
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		return setuphealth.Report{}, fmt.Errorf("resolve skill source root: %w", err)
	}
	source, err := skillbundle.ResolveSource(skillbundle.SourceOptions{
		ExplicitRoot:    opts.Env.Getenv("MATTY_SKILLS_SOURCE"),
		RepositoryStart: currentDirectory,
		InstalledRoot:   installedSource.Root(),
	})
	if err != nil {
		return setuphealth.Report{}, err
	}

	state := corelifecycle.NewLayout(snapshot.MattyHome())
	skills := skillbundle.NewGlobalLayout(snapshot.Home())
	codexLayout := codex.NewCanonicalLayout(snapshot.Home())
	openCodeLayout := opencode.NewCanonicalLayout(snapshot.ConfigurationHome())
	engramLayout := engrambin.NewSetupLayout(snapshot.Home(), snapshot.HomebrewPrefix())

	return setuphealth.Diagnose(
		snapshot.Home(),
		snapshot.ConfigurationHome(),
		corelifecycle.ObserveSetup(state, skills, source),
		engrambin.ObserveSetup(engramLayout, snapshot.ExecutableSearchPath(), opts.Runner.LookPath, opts.EngramFacts),
		codex.ObserveSetup(codexLayout),
		opencode.ObserveSetup(openCodeLayout),
	), nil
}

func newUpdateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Refresh Matty-managed tools and configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			composition, err := resolveClassicLifecycle(opts, workstationResolver)
			if err != nil {
				return err
			}
			lifecycle := corelifecycle.NewFacade(composition.config, opts.Runner, opts.Clock)
			plan, err := lifecycle.Preview(corelifecycle.Update)
			if err != nil {
				return err
			}
			if err := printSkillSourceReport(cmd.OutOrStdout(), composition.skillSource, composition.installedSource); err != nil {
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

func printSkillSourceReport(out io.Writer, source skillbundle.Source, installedSource bootstrap.InstalledSource) error {
	switch source.Origin {
	case SkillSourceOriginOverride:
		if _, err := fmt.Fprintf(out, "Skill source: explicit override (MATTY_SKILLS_SOURCE=%s)\n", source.Root); err != nil {
			return err
		}
	case SkillSourceOriginRepo:
		if _, err := fmt.Fprintf(out, "Skill source: repo checkout (%s)\n", source.Root); err != nil {
			return err
		}
		installedSkillSource := skillbundle.SourceRoot(installedSource.Root())
		if skillbundle.SourceRootExists(installedSkillSource) {
			if _, err := fmt.Fprintf(out, "warning: installed source also exists at %s; repo checkout source may create a development-mode install. For package-installed setup, run matty install outside the repo or set MATTY_SKILLS_SOURCE explicitly.\n", installedSkillSource); err != nil {
				return err
			}
		}
	case SkillSourceOriginInstalled:
		if _, err := fmt.Fprintf(out, "Skill source: installed source (%s)\n", source.Root); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(out, "Skill source: %s\n", source.Root); err != nil {
			return err
		}
	}
	return nil
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
		case corelifecycle.ActionWriteOpenCodePrompt, corelifecycle.ActionRemoveOpenCodePrompt, corelifecycle.ActionSymlink:
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

func newUninstallCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove only Matty-managed artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			composition, err := resolveClassicLifecycle(opts, workstationResolver)
			if err != nil {
				return err
			}
			lifecycle := corelifecycle.NewFacade(composition.config, opts.Runner, opts.Clock)
			plan, err := lifecycle.Preview(corelifecycle.Uninstall)
			if err != nil {
				return err
			}
			if dryRun {
				return printLifecycleDryRunPlan(cmd.OutOrStdout(), "matty uninstall", plan)
			}
			result, err := lifecycle.Apply(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if !result.HasWork() {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "matty uninstall: no Matty-managed artifacts found")
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "matty uninstall: removed Matty-managed artifacts and state %s\n", result.StateFile())
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Matty-managed removals without deleting files")
	return cmd
}

type classicLifecycleComposition struct {
	config          corelifecycle.FacadeConfig
	skillSource     skillbundle.Source
	installedSource bootstrap.InstalledSource
}

func resolveClassicLifecycle(opts Options, resolver *workstation.Resolver) (classicLifecycleComposition, error) {
	snapshot, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		return classicLifecycleComposition{}, err
	}
	installedSource, err := bootstrap.ResolveInstalledSource(snapshot, "")
	if err != nil {
		return classicLifecycleComposition{}, err
	}
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		return classicLifecycleComposition{}, fmt.Errorf("resolve skill source root: %w", err)
	}
	source, err := skillbundle.ResolveSource(skillbundle.SourceOptions{
		ExplicitRoot:    opts.Env.Getenv("MATTY_SKILLS_SOURCE"),
		RepositoryStart: currentDirectory,
		InstalledRoot:   installedSource.Root(),
	})
	if err != nil {
		return classicLifecycleComposition{}, err
	}
	return classicLifecycleComposition{
		config: corelifecycle.FacadeConfig{
			MattyHome:       snapshot.MattyHome(),
			Skills:          skillbundle.NewGlobalLayout(snapshot.Home()),
			SkillSource:     source,
			Codex:           codex.NewCanonicalLayout(snapshot.Home()),
			OpenCode:        opencode.NewCanonicalLayout(snapshot.ConfigurationHome()),
			Engram:          engrambin.NewTopology(snapshot.HomebrewPrefix()),
			InstalledSource: installedSource,
			RunningVersion:  mattyversion.Value,
		},
		skillSource:     source,
		installedSource: installedSource,
	}, nil
}
