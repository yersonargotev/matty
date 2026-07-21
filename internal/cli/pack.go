package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/skillbundle"
	"github.com/yersonargotev/packy/internal/workstation"
)

func newPackCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Discover and manage capability packs",
		Long: `Discover and manage opt-in capability packs independently on Claude Code, Codex, and OpenCode.

Lifecycle commands preview an immutable plan before interactive Apply. Approvals
are requested separately for each consent kind. A verified Apply can succeed while
login, trust, permissions, reload, or runtime loading remain pending; use targeted
status with --require usable as the separate automation gate.

After a stale plan or recovery-required attempt, repeat the original lifecycle
verb to inspect fresh state and receive a new Preview. Packy never retries it
automatically.`,
		Example: `  packy pack list
  packy pack show matty
  packy pack show engram --json
  packy pack status
  packy pack status engram --surface claude
  packy pack status engram --surface claude --require usable --json
  packy pack status engram --surface codex
  packy pack status engram --surface codex --require usable
  packy pack activate matty --surface codex --dry-run
  packy pack activate engram --surface claude --dry-run --json
  packy pack activate matty --surface codex
  packy pack update matty --surface codex
  packy pack reconcile matty --surface codex
  packy pack reconcile --surface codex
  packy pack deactivate matty --surface codex`,
	}
	cmd.AddCommand(newPackListCommand(opts, workstationResolver), newPackShowCommand(opts, workstationResolver), newPackStatusCommand(opts, workstationResolver), newPackActivateCommand(opts, workstationResolver), newPackUpdateCommand(opts, workstationResolver), newPackDeactivateCommand(opts, workstationResolver), newPackReconcileCommand(opts, workstationResolver))
	return cmd
}

func newPackReconcileCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var aliasValues []string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "reconcile [pack]", Short: "Repair active capability packs on one CLI surface", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(aliasValues) > 0 && len(args) == 0 {
				return fmt.Errorf("--alias is valid only for targeted reconcile of one pack")
			}
			aliases, err := parseSurfaceAliases(aliasValues)
			if err != nil {
				return err
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			packID := ""
			if len(args) == 1 {
				packID = args[0]
			}
			plan, err := facade.PreviewReconcile(cmd.Context(), capabilitypack.ReconcileRequest{PackID: packID, Surface: capabilitypack.Surface(surface), Aliases: aliases})
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			if err := renderActivationPlanOutput(cmd, plan, dryRun, jsonOutput); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	cmd.Flags().StringArrayVar(&aliasValues, "alias", nil, "Set a surface-local alias (<kind>:<logical-id>=<host-name>); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON events")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func newPackDeactivateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var jsonOutput bool
	cmd := &cobra.Command{Use: "deactivate <pack>", Short: "Deactivate a capability pack on one CLI surface", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		facade, err := activationFacade(opts, workstationResolver)
		if err != nil {
			return err
		}
		plan, err := facade.PreviewDeactivate(cmd.Context(), capabilitypack.DeactivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface)})
		if err != nil {
			return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
		}
		if err := renderActivationPlanOutput(cmd, plan, dryRun, jsonOutput); err != nil {
			return err
		}
		return applyPackPlan(cmd, opts, facade, plan, dryRun, jsonOutput)
	}}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON events")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func newPackUpdateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var aliasValues []string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "update <pack>", Short: "Update an active capability pack to the catalog-current version", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			aliases, err := parseSurfaceAliases(aliasValues)
			if err != nil {
				return err
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			plan, err := facade.PreviewUpdate(cmd.Context(), capabilitypack.UpdateRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), Aliases: aliases})
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			if err := renderActivationPlanOutput(cmd, plan, dryRun, jsonOutput); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	cmd.Flags().StringArrayVar(&aliasValues, "alias", nil, "Set a surface-local alias (<kind>:<logical-id>=<host-name>); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON events")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func applyPackPlan(cmd *cobra.Command, opts Options, facade capabilitypack.Facade, plan capabilitypack.ReconciliationPlan, dryRun, jsonOutput bool) error {
	if !plan.Applicable() {
		return lifecycleFailure(cmd, jsonOutput, "blocked", capabilitypack.PlanNotActionableError{Disposition: plan.Disposition()}, &plan)
	}
	if dryRun || plan.NoOp() {
		return nil
	}
	interactive := opts.Terminal.Interactive(cmd.InOrStdin())
	if !interactive {
		_, err := facade.Apply(cmd.Context(), capabilitypack.ApplyRequest{Plan: plan, Interactive: false})
		if err != nil {
			return lifecycleFailure(cmd, jsonOutput, "apply-noninteractive", err, &plan)
		}
		return nil
	}
	var receipts []capabilitypack.ApprovalReceipt
	for _, phase := range plan.Phases() {
		if !phase.ApprovalRequired {
			continue
		}
		approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Approve %s phase for exact plan %s?", phase.Kind, plan.ID()))
		if err != nil {
			return lifecycleFailure(cmd, jsonOutput, "approval", err, &plan)
		}
		if !approved {
			operation := string(plan.Operation())
			if plan.Operation() == capabilitypack.OperationActivate {
				operation = "activation"
			}
			return lifecycleFailure(cmd, jsonOutput, "approval", fmt.Errorf("%s cancelled; plan %s was not approved", operation, plan.ID()), &plan)
		}
		receipts = append(receipts, facade.Approve(plan, phase.Kind))
	}
	result, err := facade.Apply(cmd.Context(), capabilitypack.ApplyRequest{Plan: plan, Approvals: receipts, Interactive: true})
	if err != nil {
		return lifecycleFailure(cmd, jsonOutput, "apply", err, &plan)
	}
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(capabilitypack.JSONApplyResultFor(plan, result))
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Verified plan %s: %d %s projections owned by %s\n", result.PlanID, result.Projections, surfaceName(plan.Surface()), plan.Pack().ID); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Readiness: configured=%s, authorized=%s, usable=%s\n", readinessValue(result.ReadinessObserved.Configured, result.Readiness.Configured), readinessValue(result.ReadinessObserved.Authorization, result.Readiness.Authorized), readinessValue(result.ReadinessObserved.Usability, result.Readiness.Usable)); err != nil {
		return err
	}
	if len(result.PendingHumanActions) > 0 {
		if _, err = fmt.Fprintln(cmd.OutOrStdout(), "Pending human actions:"); err != nil {
			return err
		}
		for _, action := range result.PendingHumanActions {
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", action); err != nil {
				return err
			}
		}
	}
	return nil
}

func newPackActivateCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var dryRun bool
	var aliasValues []string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "activate <pack>", Short: "Activate a capability pack on one CLI surface", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			aliases, err := parseSurfaceAliases(aliasValues)
			if err != nil {
				return err
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			plan, err := facade.Preview(cmd.Context(), capabilitypack.ActivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface), Aliases: aliases})
			if err != nil {
				return lifecycleFailure(cmd, jsonOutput, "preview", err, nil)
			}
			if err := renderActivationPlanOutput(cmd, plan, dryRun, jsonOutput); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	cmd.Flags().StringArrayVar(&aliasValues, "alias", nil, "Set a surface-local alias (<kind>:<logical-id>=<host-name>); repeatable")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON events")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func lifecycleFailure(cmd *cobra.Command, jsonOutput bool, stage string, err error, plan *capabilitypack.ReconciliationPlan) error {
	if jsonOutput {
		var approval *bool
		var actions *int
		if stage == "preview" || stage == "blocked" {
			no, zero := false, 0
			approval, actions = &no, &zero
		} else if stage == "approval" {
			yes, zero := true, 0
			approval, actions = &yes, &zero
		} else if stage == "apply-noninteractive" {
			no := false
			approval = &no
		} else if stage == "apply" {
			yes := true
			approval = &yes
		}
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(capabilitypack.JSONFailureFor(stage, err, plan, approval, actions))
	}
	return err
}

func surfaceName(surface capabilitypack.Surface) string {
	if surface == capabilitypack.SurfaceClaude {
		return "Claude Code"
	}
	if surface == capabilitypack.SurfaceOpenCode {
		return "OpenCode"
	}
	return "Codex"
}

func parseSurfaceAliases(values []string) ([]capabilitypack.SurfaceAlias, error) {
	if values == nil {
		return nil, nil
	}
	aliases := make([]capabilitypack.SurfaceAlias, 0, len(values))
	for _, value := range values {
		if strings.Count(value, "=") != 1 {
			return nil, fmt.Errorf("invalid --alias %q: expected <kind>:<logical-id>=<host-name>", value)
		}
		identity, name, _ := strings.Cut(value, "=")
		kind, id, ok := strings.Cut(identity, ":")
		if !ok || kind == "" || id == "" || name == "" {
			return nil, fmt.Errorf("invalid --alias %q: expected <kind>:<logical-id>=<host-name>", value)
		}
		aliases = append(aliases, capabilitypack.SurfaceAlias{Kind: kind, ID: id, Name: name})
	}
	return aliases, nil
}

func readinessValue(observed, value bool) string {
	if !observed {
		return "unknown"
	}
	return yesNo(value)
}

func activationFacade(opts Options, workstationResolver *workstation.Resolver) (capabilitypack.Facade, error) {
	composition, err := resolvePackComposition(opts, workstationResolver)
	if err != nil {
		return capabilitypack.Facade{}, err
	}
	codexAdapter := codex.NewSurfaceAdapterWithConfig(composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile())
	openCodeAdapter := opencode.NewSurfaceAdapter(composition.bundleRoot, composition.skills.Root(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
	store := capabilitypack.NewFileActivationStore(composition.state.File())
	claudeLayout := composition.claude
	claudeExecutable, _ := opts.ClaudeLookPath("claude")
	claudePacks := make(map[string]capabilitypack.Pack)
	for _, pack := range composition.catalog.List() {
		if slices.Contains(pack.Surfaces, capabilitypack.SurfaceClaude) {
			claudePacks[pack.ID] = pack
		}
	}
	ownership := claudecode.NewCapabilityPackOwnershipProvider(store, claudePacks, claudeLayout, composition.bundleRoot)
	var claudeAdapter *claudecode.SurfaceAdapter
	if opts.ClaudeAuthorization != nil {
		claudeAdapter = claudecode.NewSurfaceAdapterWithAuthorization(composition.bundleRoot, claudeLayout, filepath.Dir(composition.state.File()), claudeExecutable, opts.ClaudeRunner, ownership, opts.ClaudeAuthorization)
	} else {
		claudeAdapter = claudecode.NewSurfaceAdapter(composition.bundleRoot, claudeLayout, filepath.Dir(composition.state.File()), claudeExecutable, opts.ClaudeRunner, ownership)
	}
	if opts.ClaudeRuntimeEvidence != nil {
		claudeAdapter = claudeAdapter.WithRuntimeEvidence(opts.ClaudeRuntimeEvidence)
	}
	adapters := opts.SurfaceAdapters
	if adapters == nil {
		adapters = map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
			capabilitypack.SurfaceCodex:    codexAdapter,
			capabilitypack.SurfaceOpenCode: openCodeAdapter,
			capabilitypack.SurfaceClaude:   claudeAdapter,
		}
	}
	return capabilitypack.NewFacade(composition.catalog,
		capabilitypack.WithActivation(store, adapters),
		capabilitypack.WithExternalEffects(
			composition.engram,
			runnerExternalExecutor{runner: opts.Runner},
		),
	), nil
}

type outputRunner interface {
	RunOutput(context.Context, string, ...string) (string, string, int, error)
}

type claudeRunner struct{ runner Runner }

func (r claudeRunner) Run(ctx context.Context, command claudecode.Command) claudecode.Result {
	if runner, ok := r.runner.(outputRunner); ok {
		stdout, stderr, exitCode, err := runner.RunOutput(ctx, command.Executable, command.Args...)
		return claudecode.Result{Stdout: stdout, Stderr: stderr, ExitCode: exitCode, Err: err}
	}
	err := r.runner.Run(ctx, command.Executable, command.Args...)
	return claudecode.Result{Err: err}
}

func renderActivationPlan(cmd *cobra.Command, plan capabilitypack.ReconciliationPlan, dryRun bool) error {
	prefix := "Activation plan"
	if plan.Operation() == capabilitypack.OperationUpdate {
		prefix = "Update plan"
	} else if plan.Operation() == capabilitypack.OperationDeactivate {
		prefix = "Deactivation plan"
	} else if plan.Operation() == capabilitypack.OperationReconcile {
		prefix = "Reconcile plan"
	}
	if dryRun {
		prefix = strings.TrimSuffix(prefix, " plan") + " dry-run plan"
	}
	packLabel := plan.Pack().ID
	if plan.Operation() != capabilitypack.OperationUpdate && plan.Operation() != capabilitypack.OperationDeactivate && plan.Operation() != capabilitypack.OperationReconcile {
		packLabel += " " + plan.Pack().Version
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\nDigest: %s\nPack: %s\nSurface: %s\n", prefix, plan.ID(), plan.Digest(), packLabel, plan.Surface()); err != nil {
		return err
	}
	if history := plan.HistoricalAttempt(); history != nil {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Recovery: fresh %s Preview toward the already-approved intent; historical plan %s is not replayed.\nHistorical outcome: %s\nHistorical digest: %s\nCompleted: %s\nFailed: %s — %s\nNot started: %s\nTo recover, repeat `packy pack %s %s --surface %s`; a new Preview and approvals are required.\n", plan.Operation(), history.PlanID, history.Outcome, history.PlanDigest, joinFacts(history.Completed), history.FailedAction, history.FailureDetail, joinFacts(history.NotStarted()), plan.Operation(), plan.Pack().ID, plan.Surface()); err != nil {
			return err
		}
	}
	if plan.Operation() == capabilitypack.OperationUpdate {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Version: %s -> %s (catalog-current)\nIntent revision: %d\n", plan.OldVersion(), plan.Pack().Version, plan.IntentRevision()); err != nil {
			return err
		}
	} else if plan.Operation() == capabilitypack.OperationDeactivate {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Active version: %s\nIntent revision: %d\n", plan.OldVersion(), plan.IntentRevision()); err != nil {
			return err
		}
	} else if plan.Operation() == capabilitypack.OperationReconcile {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Scope: %s\nIntent revision: %d (unchanged)\n", plan.ReconcileScope(), plan.IntentRevision()); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Version: %s\nIntent revision: %d\n", plan.Pack().Version, plan.IntentRevision()); err != nil {
		return err
	}
	for _, activation := range plan.Activations() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Activation: %s %s %s\n", activation.Role, activation.Pack.ID, activation.Pack.Version); err != nil {
			return err
		}
	}
	for _, alias := range plan.Aliases() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Alias: %s:%s=%s\n", alias.Kind, alias.ID, alias.Name); err != nil {
			return err
		}
	}
	if err := renderPackContract(cmd, plan.LifecycleContract()); err != nil {
		return err
	}
	readiness, observed := plan.Readiness(), plan.ReadinessObserved()
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Expected readiness: configured=%s, authorized=%s, usable=%s\nObserved evidence: %s\nPending evidence: %s\n", readinessValue(observed.Configured, readiness.Configured), readinessValue(observed.Authorization, readiness.Authorized), readinessValue(observed.Usability, readiness.Usable), renderPendingAction(plan.Evidence()), renderPendingAction(plan.PendingEvidence())); err != nil {
		return err
	}
	structured := plan.JSONReport(dryRun)
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Contract diff: added=%s changed=%s removed=%s retained=%s\n", joinFacts(structured.ContractDiff.Added), joinFacts(structured.ContractDiff.Changed), joinFacts(structured.ContractDiff.Removed), joinFacts(structured.ContractDiff.Retained)); err != nil {
		return err
	}
	for _, migration := range structured.Migrations {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Migration: %s\n", migration); err != nil {
			return err
		}
	}
	disposition := plan.Disposition()
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Plan disposition: %s\n", disposition); err != nil {
		return err
	}
	if disposition == capabilitypack.PlanBlocked || disposition == capabilitypack.PlanMixed {
		operation := "activation"
		if plan.Operation() == capabilitypack.OperationUpdate {
			operation = "update"
		} else if plan.Operation() == capabilitypack.OperationDeactivate {
			operation = "deactivation"
		} else if plan.Operation() == capabilitypack.OperationReconcile {
			operation = "reconcile"
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Cannot apply %s: %d blockers\nPreserved or blocked projections:\n", operation, len(plan.Blockers())); err != nil {
			return err
		}
		for _, blocker := range plan.Blockers() {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Blocker: %s %s — %s\n", blocker.Kind, blocker.Subject, blocker.Detail); err != nil {
				return err
			}
		}
		if disposition == capabilitypack.PlanBlocked {
			return nil
		}
	}
	if disposition == capabilitypack.PlanMixed {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Applicable actions (not applied while required blockers remain):"); err != nil {
			return err
		}
	}
	if plan.NoOp() {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Already converged: no approval or Apply required.")
		return err
	}
	contributors := plan.Contributors()
	removed := plan.RemovedContributors()
	removedKeys := make([]string, 0, len(removed))
	for id := range removed {
		removedKeys = append(removedKeys, id)
	}
	sort.Strings(removedKeys)
	for _, id := range removedKeys {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Contributor removed: %s <- %s\n", id, removed[id]); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(contributors))
	for id := range contributors {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Contributors: %s <- %s\n", id, strings.Join(contributors[id], ", ")); err != nil {
			return err
		}
	}
	for _, retained := range plan.RetainedProjections() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Retained shared projection: %s <- %s (no rewrite)\n", retained.ID, strings.Join(retained.Contributors, ", ")); err != nil {
			return err
		}
	}
	for _, resolution := range plan.Resolutions() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Requirement: %s available=%s path=%s origin=%s\n", resolution.Tool, yesNo(resolution.Available), resolution.Path, resolution.Origin); err != nil {
			return err
		}
	}
	for _, phase := range plan.Phases() {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Phase: %s (%s)\n", phase.Kind, phase.Digest); err != nil {
			return err
		}
		for _, action := range phase.Actions {
			description := action.Description
			if action.Kind == capabilitypack.ActionExternalCommand {
				description = "run: " + strings.Join(append([]string{action.Command}, action.Args...), " ") + " (" + action.Description + ")"
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", description); err != nil {
				return err
			}
		}
	}
	if pending := plan.PendingHumanActions(); len(pending) > 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Pending human actions:"); err != nil {
			return err
		}
		for _, action := range pending {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", action); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderActivationPlanOutput(cmd *cobra.Command, plan capabilitypack.ReconciliationPlan, dryRun, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan.JSONReport(dryRun))
	}
	return renderActivationPlan(cmd, plan, dryRun)
}

func renderPackContract(cmd *cobra.Command, contract capabilitypack.LifecycleContract) error {
	if contract.CompatibilityObserved {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Compatibility: %s\n", contract.Compatibility); err != nil {
			return err
		}
	}
	for _, binding := range contract.Bindings {
		mode := binding.Mode
		if binding.Degradation != "" {
			mode += " (" + binding.Degradation + ")"
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Binding: %s:%s -> %s [%s]\n", binding.Kind, binding.ID, binding.Invocation, mode); err != nil {
			return err
		}
	}
	for _, exclusion := range contract.Exclusions {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Exclusion: %s — %s\n", exclusion.ID, exclusion.Reason); err != nil {
			return err
		}
	}
	for _, mode := range contract.OptionalModes {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Optional mode: %s — %s\n", mode.ID, strings.Join(mode.Authorities, ", ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Invocation-time prompt authority: %s\n%s\n", joinFacts(contract.PromptAuthorities), contract.AuthorityDisclosure); err != nil {
		return err
	}
	return nil
}

func joinFacts(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

type runnerExternalExecutor struct{ runner Runner }

func (e runnerExternalExecutor) Execute(ctx context.Context, action capabilitypack.ProjectionAction) error {
	return e.runner.Run(ctx, action.Command, action.Args...)
}

func newPackStatusCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var surface string
	var require string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "status [pack]", Short: "Inspect capability pack status", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packID := ""
			if len(args) == 1 {
				packID = args[0]
			}
			if require != "" && (require != "usable" || packID == "" || surface == "") {
				return fmt.Errorf("--require usable is valid only for status of one pack and surface")
			}
			facade, err := activationFacade(opts, workstationResolver)
			if err != nil {
				return err
			}
			report, err := facade.Status(cmd.Context(), capabilitypack.StatusRequest{PackID: packID, Surface: capabilitypack.Surface(surface)})
			if err != nil {
				return err
			}
			if jsonOutput {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(report.JSONReport(packID != "")); err != nil {
					return err
				}
			} else if packID == "" {
				return renderPackStatusOverview(cmd, report)
			} else if err := renderPackStatusDetail(cmd, report.Entries[0]); err != nil {
				return err
			}
			if require == "usable" && !report.Entries[0].Readiness.Usable {
				return fmt.Errorf("pack %q on %s is not freshly observed usable", packID, surface)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (claude, codex, or opencode)")
	cmd.Flags().StringVar(&require, "require", "", "Require a readiness dimension (usable)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func renderPackStatusOverview(cmd *cobra.Command, report capabilitypack.StatusReport) error {
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "PACK\tSURFACE\tINTENT\tATTEMPT\tCONFIGURED\tAUTHORIZED\tUSABLE\tACTION")
	for _, entry := range report.Entries {
		configured := readinessValue(entry.ReadinessObserved.Configured, entry.Readiness.Configured)
		authorized := readinessValue(entry.ReadinessObserved.Authorization, entry.Readiness.Authorized)
		usable := readinessValue(entry.ReadinessObserved.Usability, entry.Readiness.Usable)
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", entry.Pack.ID, entry.Surface, renderIntent(entry.Intent), renderAttempt(entry.LatestAttempt), configured, authorized, usable, renderStatusAction(entry))
	}
	return writer.Flush()
}

func renderPackStatusDetail(cmd *cobra.Command, entry capabilitypack.StatusEntry) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s\nIntent: %s\nUpdate available: %s\nLatest attempt: %s\nReadiness: configured=%s, authorized=%s, usable=%s\nProjections: %d verified; %d drifted; %d ambiguous; %d missing; %d unmanaged\nBlockers: %s\nPending human actions: %s\nEvidence: %s\n", entry.Pack.ID, entry.Pack.Version, entry.Surface, renderIntent(entry.Intent), renderUpdateAvailability(entry), renderAttempt(entry.LatestAttempt), readinessValue(entry.ReadinessObserved.Configured, entry.Readiness.Configured), readinessValue(entry.ReadinessObserved.Authorization, entry.Readiness.Authorized), readinessValue(entry.ReadinessObserved.Usability, entry.Readiness.Usable), entry.Projections.Verified, entry.Projections.Drifted, entry.Projections.Ambiguous, entry.Projections.Missing, entry.Projections.Unmanaged, renderPendingAction(entry.Blockers), renderPendingAction(entry.PendingHumanActions), renderPendingAction(entry.Evidence)); err != nil {
		return err
	}
	for _, projection := range entry.ProjectionDetails {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Projection: %s owner=%s health=%s contributors=%s\n", projection.ID, projection.Owner, projection.Health, joinFacts(projection.Contributors)); err != nil {
			return err
		}
	}
	return renderPackContract(cmd, entry.Contract)
}

func renderStatusAction(entry capabilitypack.StatusEntry) string {
	if entry.UpdateAvailable {
		return "update to " + entry.Pack.Version
	}
	return renderPendingAction(entry.PendingHumanActions)
}

func renderUpdateAvailability(entry capabilitypack.StatusEntry) string {
	if !entry.UpdateAvailable {
		return "no"
	}
	return fmt.Sprintf("yes (%s -> %s)", entry.Intent.Version, entry.Pack.Version)
}

func renderIntent(intent capabilitypack.IntentStatus) string {
	if !intent.Active {
		return "inactive"
	}
	if intent.Version == "" {
		return fmt.Sprintf("active at revision %d", intent.Revision)
	}
	return fmt.Sprintf("active at version %s, revision %d", intent.Version, intent.Revision)
}

func renderAttempt(attempt *capabilitypack.AttemptStatus) string {
	if attempt == nil {
		return "none"
	}
	if attempt.PlanID == "" {
		return attempt.Outcome
	}
	return fmt.Sprintf("%s (%s)", attempt.Outcome, attempt.PlanID)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func renderPendingAction(actions []string) string {
	if len(actions) == 0 {
		return "none"
	}
	return strings.Join(actions, "; ")
}

func newPackListCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List available capability packs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			catalog, err := discoverPackCatalog(opts, workstationResolver)
			if err != nil {
				return err
			}
			packs, err := catalog.ListCurrent()
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "PACK\tVERSION\tDESCRIPTION\tAVAILABLE ON")
			for _, pack := range packs {
				surfaces := make([]string, len(pack.Surfaces))
				for i, surface := range pack.Surfaces {
					surfaces[i] = string(surface)
				}
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", pack.ID, pack.Version, pack.Description, strings.Join(surfaces, ", "))
			}
			return writer.Flush()
		},
	}
}

func newPackShowCommand(opts Options, workstationResolver *workstation.Resolver) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "show <pack>", Short: "Show a capability pack", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, err := discoverPackCatalog(opts, workstationResolver)
			if err != nil {
				return err
			}
			pack, err := catalog.Show(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return renderPackShowJSON(cmd.OutOrStdout(), pack)
			}
			counts := pack.ResourceCounts()
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\nDescription: %s\nSupported CLI surfaces: %s\nProvides capabilities: %s\nRequires capabilities: %s\nRequires global tools: %s\nConflicts with capabilities: %s\nResources: %d skill, %d instruction, %d mcp_server, %d lifecycle\n",
				pack.ID, pack.Version, pack.Description, joinSurfaces(pack.Surfaces), joinOrNone(pack.Provides), joinOrNone(pack.Requires.Capabilities), joinOrNone(pack.Requires.Tools), joinOrNone(pack.Conflicts), counts.Skills, counts.Instructions, counts.MCPServers, counts.Lifecycles)
			for _, surface := range pack.Surfaces {
				contract := capabilitypack.LifecycleContractFor(pack, surface, nil)
				if !contract.CompatibilityObserved {
					continue
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Surface contract: %s\n", surface); err != nil {
					return err
				}
				if err := renderPackContract(cmd, contract); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

type packComposition struct {
	catalog    capabilitypack.Catalog
	state      capabilitypack.StateLayout
	skills     skillbundle.GlobalLayout
	bundleRoot string
	codex      codex.CanonicalLayout
	openCode   opencode.CanonicalLayout
	claude     claudecode.CanonicalLayout
	engram     engrambin.Resolver
}

func resolvePackComposition(opts Options, workstationResolver *workstation.Resolver) (packComposition, error) {
	snapshot, err := workstationResolver.Resolve(workstation.Options{})
	if err != nil {
		return packComposition{}, err
	}
	sources, err := resolveInvocationSources(opts, snapshot)
	if err != nil {
		return packComposition{}, err
	}
	if err := skillbundle.ValidateSource(sources.skills.Root, sources.skills.MissingHint); err != nil {
		return packComposition{}, err
	}
	bundleRoot := skillbundle.BundleRoot(sources.skills.Root)
	catalog, err := capabilitypack.DiscoverForDurableIntents(bundleRoot)
	if err != nil {
		return packComposition{}, err
	}
	return packComposition{
		catalog:    catalog,
		state:      capabilitypack.NewStateLayout(snapshot.PackyHome()),
		skills:     skillbundle.NewGlobalLayout(snapshot.Home()),
		bundleRoot: bundleRoot,
		codex:      codex.NewCanonicalLayout(snapshot.Home()),
		openCode:   opencode.NewCanonicalLayout(snapshot.ConfigurationHome()),
		claude:     claudecode.NewCanonicalLayout(snapshot.Home()),
		engram:     engrambin.NewResolver(snapshot.HomebrewPrefix(), opts.Runner.LookPath),
	}, nil
}

func discoverPackCatalog(opts Options, workstationResolver *workstation.Resolver) (capabilitypack.Catalog, error) {
	composition, err := resolvePackComposition(opts, workstationResolver)
	if err != nil {
		return capabilitypack.Catalog{}, err
	}
	return composition.catalog, nil
}

func joinSurfaces(values []capabilitypack.Surface) string {
	items := make([]string, len(values))
	for i, value := range values {
		items[i] = string(value)
	}
	return strings.Join(items, ", ")
}
func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
