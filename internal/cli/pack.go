package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/matty/internal/capabilitypack"
	"github.com/yersonargotev/matty/internal/codex"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/skillbundle"
)

func newPackCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Discover and manage capability packs",
		Long: `Discover and manage opt-in capability packs independently on Codex and OpenCode.

Lifecycle commands preview an immutable plan before interactive Apply. Approvals
are requested separately for each consent kind. A verified Apply can succeed while
login, trust, permissions, reload, or runtime loading remain pending; use targeted
status with --require usable as the separate automation gate.

After a stale plan or recovery-required attempt, repeat the original lifecycle
verb to inspect fresh state and receive a new Preview. Matty never retries it
automatically.`,
		Example: `  matty pack list
  matty pack show matty
  matty pack status
  matty pack status engram --surface codex
  matty pack status engram --surface codex --require usable
  matty pack activate matty --surface codex --dry-run
  matty pack activate matty --surface codex
  matty pack update matty --surface codex
  matty pack reconcile matty --surface codex
  matty pack reconcile --surface codex
  matty pack deactivate matty --surface codex`,
	}
	cmd.AddCommand(newPackListCommand(opts), newPackShowCommand(opts), newPackStatusCommand(opts), newPackActivateCommand(opts), newPackUpdateCommand(opts), newPackDeactivateCommand(opts), newPackReconcileCommand(opts))
	return cmd
}

func newPackReconcileCommand(opts Options) *cobra.Command {
	var surface string
	var dryRun bool
	cmd := &cobra.Command{
		Use: "reconcile [pack]", Short: "Repair active capability packs on one CLI surface", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			facade, err := activationFacade(opts)
			if err != nil {
				return err
			}
			packID := ""
			if len(args) == 1 {
				packID = args[0]
			}
			plan, err := facade.PreviewReconcile(cmd.Context(), capabilitypack.ReconcileRequest{PackID: packID, Surface: capabilitypack.Surface(surface)})
			if err != nil {
				return err
			}
			if err := renderActivationPlan(cmd, plan, dryRun); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (codex or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func newPackDeactivateCommand(opts Options) *cobra.Command {
	var surface string
	var dryRun bool
	cmd := &cobra.Command{Use: "deactivate <pack>", Short: "Deactivate a capability pack on one CLI surface", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		facade, err := activationFacade(opts)
		if err != nil {
			return err
		}
		plan, err := facade.PreviewDeactivate(cmd.Context(), capabilitypack.DeactivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface)})
		if err != nil {
			return err
		}
		if err := renderActivationPlan(cmd, plan, dryRun); err != nil {
			return err
		}
		return applyPackPlan(cmd, opts, facade, plan, dryRun)
	}}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (codex or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func newPackUpdateCommand(opts Options) *cobra.Command {
	var surface string
	var dryRun bool
	cmd := &cobra.Command{
		Use: "update <pack>", Short: "Update an active capability pack to the catalog-current version", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			facade, err := activationFacade(opts)
			if err != nil {
				return err
			}
			plan, err := facade.PreviewUpdate(cmd.Context(), capabilitypack.UpdateRequest{PackID: args[0], Surface: capabilitypack.Surface(surface)})
			if err != nil {
				return err
			}
			if err := renderActivationPlan(cmd, plan, dryRun); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (codex or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func applyPackPlan(cmd *cobra.Command, opts Options, facade capabilitypack.Facade, plan capabilitypack.ReconciliationPlan, dryRun bool) error {
	if !plan.Applicable() {
		return capabilitypack.PlanNotActionableError{Disposition: plan.Disposition()}
	}
	if dryRun || plan.NoOp() {
		return nil
	}
	interactive := opts.Terminal.Interactive(cmd.InOrStdin())
	if !interactive {
		_, err := facade.Apply(cmd.Context(), capabilitypack.ApplyRequest{Plan: plan, Interactive: false})
		return err
	}
	var receipts []capabilitypack.ApprovalReceipt
	for _, phase := range plan.Phases() {
		if !phase.ApprovalRequired {
			continue
		}
		approved, err := opts.Terminal.Approve(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Approve %s phase for exact plan %s?", phase.Kind, plan.ID()))
		if err != nil {
			return err
		}
		if !approved {
			operation := string(plan.Operation())
			if plan.Operation() == capabilitypack.OperationActivate {
				operation = "activation"
			}
			return fmt.Errorf("%s cancelled; plan %s was not approved", operation, plan.ID())
		}
		receipts = append(receipts, facade.Approve(plan, phase.Kind))
	}
	result, err := facade.Apply(cmd.Context(), capabilitypack.ApplyRequest{Plan: plan, Approvals: receipts, Interactive: true})
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Verified plan %s: %d %s projections owned by %s\n", result.PlanID, result.Projections, surfaceName(plan.Surface()), plan.Pack().ID); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Readiness: configured=%s, authorized=%s, usable=%s\n", yesNo(result.Readiness.Configured), yesNo(result.Readiness.Authorized), yesNo(result.Readiness.Usable)); err != nil {
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

func newPackActivateCommand(opts Options) *cobra.Command {
	var surface string
	var dryRun bool
	cmd := &cobra.Command{
		Use: "activate <pack>", Short: "Activate a capability pack on one CLI surface", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			facade, err := activationFacade(opts)
			if err != nil {
				return err
			}
			plan, err := facade.Preview(cmd.Context(), capabilitypack.ActivationRequest{PackID: args[0], Surface: capabilitypack.Surface(surface)})
			if err != nil {
				return err
			}
			if err := renderActivationPlan(cmd, plan, dryRun); err != nil {
				return err
			}
			return applyPackPlan(cmd, opts, facade, plan, dryRun)
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (codex or opencode)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview the immutable plan without approval or mutation")
	_ = cmd.MarkFlagRequired("surface")
	return cmd
}

func surfaceName(surface capabilitypack.Surface) string {
	if surface == capabilitypack.SurfaceOpenCode {
		return "OpenCode"
	}
	return "Codex"
}

func activationFacade(opts Options) (capabilitypack.Facade, error) {
	catalog, err := discoverPackCatalog(opts)
	if err != nil {
		return capabilitypack.Facade{}, err
	}
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		return capabilitypack.Facade{}, err
	}
	codexAdapter := codex.NewSurfaceAdapterWithConfig(paths.BundleSourceRoot, paths.AgentSkillsDir, paths.CodexPromptFile, paths.CodexConfigFile)
	openCodeAdapter := opencode.NewSurfaceAdapter(paths.BundleSourceRoot, paths.AgentSkillsDir, paths.OpenCodeConfigFile, paths.OpenCodePromptFile)
	adapters := opts.SurfaceAdapters
	if adapters == nil {
		adapters = map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
			capabilitypack.SurfaceCodex:    codexAdapter,
			capabilitypack.SurfaceOpenCode: openCodeAdapter,
		}
	}
	return capabilitypack.NewFacade(catalog,
		capabilitypack.WithActivation(capabilitypack.NewFileActivationStore(paths.PackStateFile), adapters),
		capabilitypack.WithExternalEffects(
			engrambin.NewResolver(paths.HomebrewPrefixEnv, opts.Runner.LookPath),
			runnerExternalExecutor{runner: opts.Runner},
		),
	), nil
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
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Recovery: fresh %s Preview toward the already-approved intent; historical plan %s is not replayed.\nHistorical outcome: %s\nHistorical digest: %s\nCompleted: %s\nFailed: %s — %s\nNot started: %s\nTo recover, repeat `matty pack %s %s --surface %s`; a new Preview and approvals are required.\n", plan.Operation(), history.PlanID, history.Outcome, history.PlanDigest, joinFacts(history.Completed), history.FailedAction, history.FailureDetail, joinFacts(history.NotStarted()), plan.Operation(), plan.Pack().ID, plan.Surface()); err != nil {
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

func newPackStatusCommand(opts Options) *cobra.Command {
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
			facade, err := activationFacade(opts)
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
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (codex or opencode)")
	cmd.Flags().StringVar(&require, "require", "", "Require a readiness dimension (usable)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func renderPackStatusOverview(cmd *cobra.Command, report capabilitypack.StatusReport) error {
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "PACK\tSURFACE\tINTENT\tATTEMPT\tCONFIGURED\tAUTHORIZED\tUSABLE\tACTION")
	for _, entry := range report.Entries {
		configured, authorized, usable := yesNo(entry.Readiness.Configured), yesNo(entry.Readiness.Authorized), yesNo(entry.Readiness.Usable)
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", entry.Pack.ID, entry.Surface, renderIntent(entry.Intent), renderAttempt(entry.LatestAttempt), configured, authorized, usable, renderPendingAction(entry.PendingHumanActions))
	}
	return writer.Flush()
}

func renderPackStatusDetail(cmd *cobra.Command, entry capabilitypack.StatusEntry) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s\nIntent: %s\nLatest attempt: %s\nReadiness: configured=%s, authorized=%s, usable=%s\nProjections: %d verified; %d drifted; %d ambiguous; %d missing; %d unmanaged\nBlockers: %s\nPending human actions: %s\nEvidence: %s\n", entry.Pack.ID, entry.Pack.Version, entry.Surface, renderIntent(entry.Intent), renderAttempt(entry.LatestAttempt), yesNo(entry.Readiness.Configured), yesNo(entry.Readiness.Authorized), yesNo(entry.Readiness.Usable), entry.Projections.Verified, entry.Projections.Drifted, entry.Projections.Ambiguous, entry.Projections.Missing, entry.Projections.Unmanaged, renderPendingAction(entry.Blockers), renderPendingAction(entry.PendingHumanActions), renderPendingAction(entry.Evidence))
	return err
}

func renderIntent(intent capabilitypack.IntentStatus) string {
	if !intent.Active {
		return "inactive"
	}
	return fmt.Sprintf("active at revision %d", intent.Revision)
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

func newPackListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List available capability packs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			catalog, err := discoverPackCatalog(opts)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "PACK\tVERSION\tDESCRIPTION\tAVAILABLE ON")
			for _, pack := range catalog.List() {
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

func newPackShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use: "show <pack>", Short: "Show a capability pack", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, err := discoverPackCatalog(opts)
			if err != nil {
				return err
			}
			pack, err := catalog.Show(args[0])
			if err != nil {
				return err
			}
			counts := pack.ResourceCounts()
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\nDescription: %s\nSupported CLI surfaces: %s\nProvides capabilities: %s\nRequires capabilities: %s\nRequires global tools: %s\nConflicts with capabilities: %s\nResources: %d skill, %d instruction, %d mcp_server, %d lifecycle\n",
				pack.ID, pack.Version, pack.Description, joinSurfaces(pack.Surfaces), joinOrNone(pack.Provides), joinOrNone(pack.Requires.Capabilities), joinOrNone(pack.Requires.Tools), joinOrNone(pack.Conflicts), counts.Skills, counts.Instructions, counts.MCPServers, counts.Lifecycles)
			return nil
		},
	}
}

func discoverPackCatalog(opts Options) (capabilitypack.Catalog, error) {
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		return capabilitypack.Catalog{}, err
	}
	if err := skillbundle.ValidateSource(paths.SkillSourceRoot, paths.SkillSourceMissingHint); err != nil {
		return capabilitypack.Catalog{}, err
	}
	return capabilitypack.Discover(paths.BundleSourceRoot)
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
