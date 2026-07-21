package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/corelifecycle"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/setuphealth"
	"github.com/yersonargotev/packy/internal/skillbundle"
	packyversion "github.com/yersonargotev/packy/internal/version"
	"github.com/yersonargotev/packy/internal/workstation"
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
	ClaudeRunner        claudecode.Runner
	ClaudeLookPath      claudecode.LookPath
}

func (o Options) withDefaults() Options {
	if o.Env == nil {
		o.Env = osEnv{}
	}
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.ClaudeRunner == nil {
		o.ClaudeRunner = execClaudeRunner{}
	}
	if o.ClaudeLookPath == nil {
		o.ClaudeLookPath = claudecode.LookPath(o.Runner.LookPath)
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

// NewRootCommand constructs the Packy CLI command tree.
func NewRootCommand(opts Options) *cobra.Command {
	opts = opts.withDefaults()
	workstationResolver := newWorkstationResolver(opts)

	root := &cobra.Command{
		Use:           "packy",
		Short:         "Install and configure the Packy AI coding workflow",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       packyversion.Value,
	}

	root.AddCommand(
		newVersionCommand(),
		newPackCommand(opts, workstationResolver),
		newInitCommand(opts, workstationResolver),
		newInstallCommand(opts, workstationResolver),
		newDoctorCommand(opts, workstationResolver),
		newUpdateCommand(opts, workstationResolver),
		newUninstallCommand(opts, workstationResolver),
	)

	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Packy version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "packy version %s\n", packyversion.Value)
			return err
		},
	}
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
		Short: "Initialize Packy's package-installed source checkout",
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
				RepositoryRef:   defaultInitRepositoryRef(repositoryRef, packyversion.Value),
				HomeDir:         snapshot.Home(),
				ConfigHome:      snapshot.ConfigurationHome(),
				ReportProgress: func(message string) error {
					_, err := fmt.Fprintf(cmd.OutOrStdout(), "packy init: %s\n", message)
					return err
				},
			})
			if err != nil {
				return err
			}
			switch {
			case result.Cloned:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy init: initialized Installed Source at %s\n", installedSource.Root())
			case result.Updated:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy init: updated Installed Source at %s\n", installedSource.Root())
			default:
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "packy init: Installed Source already initialized at %s\n", installedSource.Root())
			}
			return err
		},
	}

	cmd.Flags().StringVar(&homeFlag, "home", "", "home directory used to resolve the default Installed Source")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "Installed Source root (default ~/.local/share/packy)")
	cmd.Flags().StringVar(&repositoryURL, "repository-url", bootstrap.DefaultRepositoryURL, "Packy Source of Truth Git URL")
	cmd.Flags().StringVar(&repositoryRef, "repository-ref", "", "optional Packy Source of Truth Git ref to clone or check out")
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
		Short: "Install Packy-managed global workflow configuration",
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
				if err := printLifecycleDryRunPlan(cmd.OutOrStdout(), "packy install", plan); err != nil {
					return err
				}
				return classicLifecycleOutcomeError(plan.Outcome())
			}
			result, err := lifecycle.Apply(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if err := printWarnings(cmd.OutOrStdout(), result.Warnings()); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "packy install: synced %d managed skills and wrote state %s (outcome: %s)\n", result.ManagedSkillCount(), result.StateFile(), result.Outcome()); err != nil {
				return err
			}
			return classicLifecycleOutcomeError(result.Outcome())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Packy-managed changes without writing files")
	return cmd
}

func newDoctorCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check Packy setup without changing files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				report setuphealth.Report
				err    error
			)
			if opts.SetupHealthDiagnose != nil {
				report, err = opts.SetupHealthDiagnose()
			} else {
				report, err = diagnoseSetupHealth(cmd.Context(), opts, workstationResolver)
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

func diagnoseSetupHealth(ctx context.Context, opts Options, resolver *workstation.Resolver) (setuphealth.Report, error) {
	snapshot, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		return setuphealth.Report{}, err
	}
	sources, err := resolveInvocationSources(opts, snapshot)
	if err != nil {
		return setuphealth.Report{}, err
	}

	state := corelifecycle.NewLayout(snapshot.PackyHome())
	skills := skillbundle.NewGlobalLayout(snapshot.Home())
	codexLayout := codex.NewCanonicalLayout(snapshot.Home())
	openCodeLayout := opencode.NewCanonicalLayout(snapshot.ConfigurationHome())
	engramLayout := engrambin.NewSetupLayout(snapshot.Home(), snapshot.HomebrewPrefix())
	lifecycleObservation := corelifecycle.ObserveSetup(state, skills, sources.skills)
	claudeExecutable, err := opts.ClaudeLookPath("claude")
	if err != nil {
		claudeExecutable = ""
	}
	claudeObservation := claudecode.ObserveSetup(
		ctx, claudecode.NewCanonicalLayout(snapshot.Home()), claudeExecutable, opts.ClaudeRunner,
		lifecycleObservation.State().ClaudeOwnershipSnapshot(),
	)

	return setuphealth.Diagnose(
		snapshot.Home(),
		snapshot.ConfigurationHome(),
		lifecycleObservation,
		engrambin.ObserveSetup(engramLayout, snapshot.ExecutableSearchPath(), opts.Runner.LookPath, opts.EngramFacts),
		codex.ObserveSetup(codexLayout),
		opencode.ObserveSetup(openCodeLayout),
		claudeObservation,
	), nil
}

func newUpdateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Refresh Packy-managed tools and configuration",
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
				if err := printLifecycleDryRunPlan(cmd.OutOrStdout(), "packy update", plan); err != nil {
					return err
				}
				return classicLifecycleOutcomeError(plan.Outcome())
			}
			result, err := lifecycle.Apply(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if err := printWarnings(cmd.OutOrStdout(), result.Warnings()); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "packy update: synced %d managed skills and wrote state %s (outcome: %s)\n", result.ManagedSkillCount(), result.StateFile(), result.Outcome()); err != nil {
				return err
			}
			return classicLifecycleOutcomeError(result.Outcome())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Packy-managed update changes without writing files or running commands")
	return cmd
}

func printSkillSourceReport(out io.Writer, source skillbundle.Source, installedSource bootstrap.InstalledSource) error {
	switch source.Origin {
	case skillbundle.SourceOriginOverride:
		if _, err := fmt.Fprintf(out, "Skill source: explicit override (PACKY_SKILLS_SOURCE=%s)\n", source.Root); err != nil {
			return err
		}
	case skillbundle.SourceOriginRepository:
		if _, err := fmt.Fprintf(out, "Skill source: repo checkout (%s)\n", source.Root); err != nil {
			return err
		}
		installedSkillSource := skillbundle.InstalledSourceRoot(installedSource)
		if skillbundle.SourceRootExists(installedSkillSource) {
			if _, err := fmt.Fprintf(out, "warning: installed source also exists at %s; repo checkout source may create a development-mode install. For package-installed setup, run packy install outside the repo or set PACKY_SKILLS_SOURCE explicitly.\n", installedSkillSource); err != nil {
				return err
			}
		}
	case skillbundle.SourceOriginInstalled:
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
	if _, err := fmt.Fprintf(out, "%s dry-run: planned actions\nOutcome: %s\nDesired surfaces: %s\n", command, plan.Outcome(), strings.Join(plan.DesiredSurfaces(), ", ")); err != nil {
		return err
	}
	transition := plan.StateTransition()
	if _, err := fmt.Fprintf(out, "State transition: schema %d -> %d; status %s -> %s\n", transition.FromSchemaVersion, transition.ToSchemaVersion, transition.FromStatus, transition.ToStatus); err != nil {
		return err
	}
	for _, value := range plan.PendingPrerequisites() {
		if _, err := fmt.Fprintf(out, "Pending prerequisite: %s\n", value); err != nil {
			return err
		}
	}
	for _, value := range plan.Preserved() {
		if _, err := fmt.Fprintf(out, "Preserved: %s\n", value); err != nil {
			return err
		}
	}
	for _, value := range plan.Blockers() {
		if _, err := fmt.Fprintf(out, "Blocker: %s\n", value); err != nil {
			return err
		}
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
		Short: "Remove only Packy-managed artifacts",
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
				if err := printLifecycleDryRunPlan(cmd.OutOrStdout(), "packy uninstall", plan); err != nil {
					return err
				}
				return classicLifecycleOutcomeError(plan.Outcome())
			}
			result, err := lifecycle.Apply(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if !result.HasWork() {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "packy uninstall: no Packy-managed artifacts found")
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "packy uninstall: %s; processed Packy-managed artifacts for state %s\n", result.Outcome(), result.StateFile()); err != nil {
				return err
			}
			return classicLifecycleOutcomeError(result.Outcome())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Packy-managed removals without deleting files")
	return cmd
}

var ErrClassicLifecycleIncomplete = errors.New("classic lifecycle did not converge")

func classicLifecycleOutcomeError(outcome corelifecycle.Outcome) error {
	switch outcome {
	case corelifecycle.OutcomeConverged, corelifecycle.OutcomeApplied, corelifecycle.OutcomeAppliedWithPendingPrerequisite:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrClassicLifecycleIncomplete, outcome)
	}
}

type classicLifecycleComposition struct {
	config          corelifecycle.FacadeConfig
	skillSource     skillbundle.Source
	installedSource bootstrap.InstalledSource
}

type invocationSources struct {
	installed bootstrap.InstalledSource
	skills    skillbundle.Source
}

func resolveInvocationSources(opts Options, snapshot workstation.Snapshot) (invocationSources, error) {
	installed, err := bootstrap.ResolveInstalledSource(snapshot, "")
	if err != nil {
		return invocationSources{}, err
	}
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		return invocationSources{}, fmt.Errorf("resolve skill source root: %w", err)
	}
	skills, err := skillbundle.ResolveSource(skillbundle.SourceOptions{
		ExplicitRoot:    opts.Env.Getenv("PACKY_SKILLS_SOURCE"),
		RepositoryStart: currentDirectory,
		InstalledSource: installed,
	})
	if err != nil {
		return invocationSources{}, err
	}
	return invocationSources{installed: installed, skills: skills}, nil
}

func resolveClassicLifecycle(opts Options, resolver *workstation.Resolver) (classicLifecycleComposition, error) {
	snapshot, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		return classicLifecycleComposition{}, err
	}
	sources, err := resolveInvocationSources(opts, snapshot)
	if err != nil {
		return classicLifecycleComposition{}, err
	}
	claudeExecutable, err := opts.ClaudeLookPath("claude")
	if err != nil {
		claudeExecutable = ""
	}
	stateLayout := corelifecycle.NewLayout(snapshot.PackyHome())
	claudeAdapter := claudecode.NewSurfaceAdapter(
		"", claudecode.NewCanonicalLayout(snapshot.Home()), snapshot.PackyHome(),
		claudeExecutable, opts.ClaudeRunner,
		claudecode.OwnershipSnapshotFunc(func(_ context.Context) (claudecode.OwnershipSnapshot, error) {
			return corelifecycle.ObserveClaudeOwnershipSnapshot(stateLayout.StateFile())
		}),
	)
	return classicLifecycleComposition{
		config: corelifecycle.FacadeConfig{
			PackyHome:       snapshot.PackyHome(),
			Skills:          skillbundle.NewGlobalLayout(snapshot.Home()),
			SkillSource:     sources.skills,
			Codex:           codex.NewCanonicalLayout(snapshot.Home()),
			OpenCode:        opencode.NewCanonicalLayout(snapshot.ConfigurationHome()),
			Engram:          engrambin.NewTopology(snapshot.HomebrewPrefix()),
			InstalledSource: sources.installed,
			RunningVersion:  packyversion.Value,
			Claude:          claudeAdapter,
		},
		skillSource:     sources.skills,
		installedSource: sources.installed,
	}, nil
}
