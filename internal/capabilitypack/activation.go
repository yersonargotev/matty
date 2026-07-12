package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

var (
	ErrInteractiveRequired = errors.New("Apply requires an interactive terminal")
	ErrApprovalMismatch    = errors.New("approval does not match the exact plan")
	ErrStalePlan           = errors.New("reconciliation plan is stale")
	ErrVerificationFailed  = errors.New("fresh verification did not match desired state")
	ErrPlanNotActionable   = errors.New("lifecycle plan is not actionable")
)

type PlanDisposition string

const (
	PlanConverged    PlanDisposition = "converged"
	PlanApplicable   PlanDisposition = "applicable"
	PlanMixed        PlanDisposition = "mixed"
	PlanBlocked      PlanDisposition = "blocked"
	PlanPendingHuman PlanDisposition = "pending-human-actions"
)

type PlanNotActionableError struct{ Disposition PlanDisposition }

func (e PlanNotActionableError) Error() string {
	return fmt.Sprintf("%s: %s", ErrPlanNotActionable, e.Disposition)
}
func (e PlanNotActionableError) Unwrap() error { return ErrPlanNotActionable }

type ConsentKind string
type Operation string
type ProjectionActionKind string
type ProjectionActionMode string

const (
	ConsentReversibleLocal        ConsentKind          = "reversible-local"
	ConsentExecutableExternal     ConsentKind          = "executable-external"
	ConsentHostFollowUp           ConsentKind          = "host-follow-up"
	ConsentDestructiveCleanup     ConsentKind          = "destructive-cleanup"
	OperationActivate             Operation            = "activate"
	OperationUpdate               Operation            = "update"
	OperationDeactivate           Operation            = "deactivate"
	OperationReconcile            Operation            = "reconcile"
	ActionSkillLink               ProjectionActionKind = "skill-link"
	ActionInstructionFile         ProjectionActionKind = "instruction-file"
	ActionOpenCodeSkillLink       ProjectionActionKind = "opencode-skill-link"
	ActionOpenCodeInstructionFile ProjectionActionKind = "opencode-instruction-file"
	ActionOpenCodeConfigReference ProjectionActionKind = "opencode-config-reference"
	ActionCodexMCPConfig          ProjectionActionKind = "codex-mcp-config"
	ActionOpenCodeMCPConfig       ProjectionActionKind = "opencode-mcp-config"
	ActionExternalCommand         ProjectionActionKind = "external-command"
	ActionHostFollowUp            ProjectionActionKind = "host-follow-up"
	ProjectionRemoveContent       ProjectionActionMode = "remove-content"
	ProjectionDeleteTarget        ProjectionActionMode = "delete-target"
)

type StalePlanError struct{ Precondition string }

func (e StalePlanError) Error() string { return fmt.Sprintf("%s: %s", ErrStalePlan, e.Precondition) }
func (e StalePlanError) Unwrap() error { return ErrStalePlan }

type ActivationRequest struct {
	PackID  string
	Surface Surface
}

type UpdateRequest struct {
	PackID  string
	Surface Surface
}

type DeactivationRequest struct {
	PackID  string
	Surface Surface
}

type ReconcileScope string

const (
	ReconcileTargeted    ReconcileScope = "targeted"
	ReconcileSurfaceWide ReconcileScope = "surface-wide"
)

type ReconcileRequest struct {
	PackID  string
	Surface Surface
}

// ExecutableResolution is the immutable fact set used to choose an external
// command. It intentionally contains no credentials or tool-owned data.
type ExecutableResolution struct {
	Tool                 string   `json:"tool"`
	Available            bool     `json:"available"`
	Path                 string   `json:"path"`
	ResolvedPath         string   `json:"resolved_path"`
	Origin               string   `json:"origin"`
	Version              string   `json:"version,omitempty"`
	AcquisitionSupported bool     `json:"acquisition_supported"`
	AcquisitionCommand   string   `json:"acquisition_command,omitempty"`
	AcquisitionArgs      []string `json:"acquisition_args,omitempty"`
	Precondition         string   `json:"precondition"`
}

// ExecutableResolver is owned by capabilitypack; the concrete Engram
// resolver is composed by the CLI at the edge of the application.
type ExecutableResolver interface {
	Resolve(context.Context, string) (ExecutableResolution, error)
}

// ExternalExecutor is the only side-effect seam for executable/external
// actions. The facade supplies exact sealed actions; it never asks the
// executor to discover or construct a command.
type ExternalExecutor interface {
	Execute(context.Context, ProjectionAction) error
}

// ProjectionAction is an adapter-produced, host-specific local projection.
// Capability-pack policy orders and approves it; only the matching adapter executes it.
type ProjectionAction struct {
	ID          string               `json:"id"`
	Description string               `json:"description"`
	Kind        ProjectionActionKind `json:"kind,omitempty"`
	Source      string               `json:"source,omitempty"`
	Target      string               `json:"target,omitempty"`
	Content     string               `json:"content,omitempty"`
	Command     string               `json:"command,omitempty"`
	Args        []string             `json:"args,omitempty"`
	Mode        ProjectionActionMode `json:"mode,omitempty"`
}

func RemovalCandidate(projection ObservedProjection, mode ProjectionActionMode, content, description string) ObservedProjection {
	projection.DesiredFingerprint = "missing"
	projection.Action.Source = ""
	projection.Action.Content = content
	projection.Action.Mode = mode
	projection.Action.Description = description
	return projection
}

type ObservedProjection struct {
	ID                  string
	Exists              bool
	ObservedFingerprint string
	DesiredFingerprint  string
	ExternallyManaged   bool
	Action              ProjectionAction
}

type ActivationObservation struct {
	Revision            string
	Projections         []ObservedProjection
	Readiness           ReadinessStatus
	PendingHumanActions []string
	RemovalCandidates   []ObservedProjection
}

type ActivationAdapter interface {
	InspectActivation(context.Context, Pack) (ActivationObservation, error)
	ApplyProjections(context.Context, []ProjectionAction) *ProjectionActionError
}

// ResolutionAwareActivationAdapter receives the already-resolved executable
// facts when a host projection embeds a global tool command. The optional
// interface keeps existing adapter fakes source-compatible while ensuring the
// production Codex/OpenCode adapters use the exact sealed executable.
type ResolutionAwareActivationAdapter interface {
	InspectActivationWithResolution(context.Context, Pack, []ExecutableResolution) (ActivationObservation, error)
}

type DeactivationAwareActivationAdapter interface {
	InspectDeactivation(context.Context, Pack, Pack, []ExecutableResolution) (ActivationObservation, error)
}

// ReconciliationAwareActivationAdapter can inspect Matty-owned projections
// that no longer occur in the complete desired pack. The facade remains
// responsible for verifying ownership before authorizing their removal.
type ReconciliationAwareActivationAdapter interface {
	InspectReconcile(context.Context, Pack, []ProjectionOwnership, []ExecutableResolution) (ActivationObservation, error)
}

func inspectDeactivation(ctx context.Context, adapter ActivationAdapter, active, desired Pack, resolutions []ExecutableResolution) (ActivationObservation, error) {
	if aware, ok := adapter.(DeactivationAwareActivationAdapter); ok {
		observation, err := aware.InspectDeactivation(ctx, active, desired, resolutions)
		if err != nil {
			return ActivationObservation{}, err
		}
		for i := range observation.RemovalCandidates {
			observation.RemovalCandidates[i].DesiredFingerprint = ""
		}
		observation.Projections = append(observation.Projections, observation.RemovalCandidates...)
		observation.RemovalCandidates = nil
		return observation, nil
	}
	return inspectActivation(ctx, adapter, desired, resolutions)
}

func inspectReconcile(ctx context.Context, adapter ActivationAdapter, desired Pack, ownership []ProjectionOwnership, resolutions []ExecutableResolution) (ActivationObservation, error) {
	if aware, ok := adapter.(ReconciliationAwareActivationAdapter); ok {
		observation, err := aware.InspectReconcile(ctx, desired, cloneOwnership(ownership), resolutions)
		if err != nil {
			return ActivationObservation{}, err
		}
		observation.Projections = append(observation.Projections, observation.RemovalCandidates...)
		observation.RemovalCandidates = nil
		return observation, nil
	}
	return inspectActivation(ctx, adapter, desired, resolutions)
}

type ActivationIntent struct {
	PackID   string  `json:"pack_id"`
	Surface  Surface `json:"surface"`
	Version  string  `json:"version"`
	Active   bool    `json:"active"`
	Revision int     `json:"revision"`
}

type ProjectionOwnership struct {
	ID           string   `json:"id"`
	Contributors []string `json:"contributors"`
	Fingerprint  string   `json:"fingerprint"`
}

type ApplyingJournal struct {
	PlanID        string         `json:"plan_id"`
	PlanDigest    string         `json:"plan_digest,omitempty"`
	Operation     Operation      `json:"operation,omitempty"`
	Surface       Surface        `json:"surface,omitempty"`
	PackID        string         `json:"pack_id,omitempty"`
	Outcome       AttemptOutcome `json:"outcome,omitempty"`
	Actions       []string       `json:"actions"`
	Completed     []string       `json:"completed,omitempty"`
	FailedAction  string         `json:"failed_action,omitempty"`
	FailureDetail string         `json:"failure_detail,omitempty"`
}

type AttemptOutcome string

const (
	AttemptApplying         AttemptOutcome = "applying"
	AttemptVerified         AttemptOutcome = "verified"
	AttemptRecoveryRequired AttemptOutcome = "recovery-required"
)

type ProjectionActionError struct {
	ID  string
	Err error
}

func (e ProjectionActionError) Error() string {
	return fmt.Sprintf("apply projection %s: %v", e.ID, e.Err)
}
func (e ProjectionActionError) Unwrap() error { return e.Err }

func (j ApplyingJournal) NotStarted() []string {
	completed := map[string]bool{}
	for _, id := range j.Completed {
		completed[id] = true
	}
	result := make([]string, 0, len(j.Actions))
	for _, id := range j.Actions {
		if !completed[id] && id != j.FailedAction {
			result = append(result, id)
		}
	}
	return result
}

func (j *ApplyingJournal) recordFailure(action string, err error) {
	j.FailedAction = action
	j.Outcome = AttemptRecoveryRequired
	j.FailureDetail = err.Error()
}

func requiredFailedActionID(err error, phase string) string {
	var actionErr *ProjectionActionError
	if errors.As(err, &actionErr) && actionErr.ID != "" {
		return actionErr.ID
	}
	panic("activation adapter violated its action-specific error contract: " + phase)
}

type ExternalEffect struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
}

type ActivationState struct {
	SchemaVersion int                   `json:"schema_version"`
	Intent        ActivationIntent      `json:"intent"`
	Intents       []ActivationIntent    `json:"intents,omitempty"`
	Journal       *ApplyingJournal      `json:"applying_journal,omitempty"`
	LastAttempts  []ApplyingJournal     `json:"last_attempts,omitempty"`
	History       []ApplyingJournal     `json:"attempt_history,omitempty"`
	Ownership     []ProjectionOwnership `json:"ownership,omitempty"`
	External      []ExternalEffect      `json:"external_effects,omitempty"`
}

type ActivationStore interface {
	Load(context.Context, Surface) (ActivationState, error)
	Save(context.Context, Surface, int, ActivationState) error
}

type activationDependencies struct {
	store    ActivationStore
	adapters map[Surface]ActivationAdapter
	resolver ExecutableResolver
	executor ExternalExecutor
}

type FacadeOption func(*Facade)

func WithActivation(store ActivationStore, adapters map[Surface]ActivationAdapter) FacadeOption {
	return func(f *Facade) {
		var resolver ExecutableResolver
		var executor ExternalExecutor
		if f.activation != nil {
			resolver = f.activation.resolver
			executor = f.activation.executor
		}
		f.activation = &activationDependencies{store: store, adapters: adapters, resolver: resolver, executor: executor}
	}
}

func WithExternalEffects(resolver ExecutableResolver, executor ExternalExecutor) FacadeOption {
	return func(f *Facade) {
		if f.activation == nil {
			f.activation = &activationDependencies{}
		}
		f.activation.resolver = resolver
		f.activation.executor = executor
	}
}

type PlanPhase struct {
	Kind             ConsentKind
	Digest           string
	ApprovalRequired bool
	Actions          []ProjectionAction
}

type ReconciliationPlan struct {
	id                     string
	digest                 string
	pack                   Pack
	operation              Operation
	surface                Surface
	intentRevision         int
	oldVersion             string
	observationFingerprint string
	phases                 []PlanPhase
	desired                []projectionExpectation
	portable               []PortableOutcome
	resolutions            []ExecutableResolution
	readiness              ReadinessStatus
	pendingHumanActions    []string
	noOp                   bool
	activations            []PlannedActivation
	contributors           map[string][]string
	retained               []RetainedProjection
	blockers               []PlanBlocker
	compositionFacts       []Pack
	intentFacts            []ActivationIntent
	ownershipFacts         []ProjectionOwnership
	activeDependents       []ActiveDependent
	beforeCompositionFacts []Pack
	removedContributors    map[string]string
	reconcileScope         ReconcileScope
	recovery               bool
	historicalAttempt      *ApplyingJournal
}

type RetainedProjection struct {
	ID           string
	Contributors []string
}

type projectionExpectation struct {
	ID, Fingerprint   string
	ExternallyManaged bool
}
type PortableOutcome struct{ Kind, ID string }

func (p ReconciliationPlan) ID() string                     { return p.id }
func (p ReconciliationPlan) Digest() string                 { return p.digest }
func (p ReconciliationPlan) Pack() Pack                     { return clonePack(p.pack) }
func (p ReconciliationPlan) Surface() Surface               { return p.surface }
func (p ReconciliationPlan) Operation() Operation           { return p.operation }
func (p ReconciliationPlan) ReconcileScope() ReconcileScope { return p.reconcileScope }
func (p ReconciliationPlan) OldVersion() string             { return p.oldVersion }
func (p ReconciliationPlan) IntentRevision() int            { return p.intentRevision }
func (p ReconciliationPlan) NoOp() bool                     { return p.noOp }
func (p ReconciliationPlan) Applicable() bool {
	return p.Disposition() == PlanApplicable || p.Disposition() == PlanConverged || p.Disposition() == PlanPendingHuman
}
func (p ReconciliationPlan) Disposition() PlanDisposition {
	actions := false
	for _, phase := range p.phases {
		if phase.ApprovalRequired && len(phase.Actions) > 0 {
			actions = true
			break
		}
	}
	if len(p.blockers) > 0 {
		if actions {
			return PlanMixed
		}
		return PlanBlocked
	}
	if actions {
		return PlanApplicable
	}
	if len(p.pendingHumanActions) > 0 {
		return PlanPendingHuman
	}
	return PlanConverged
}
func (p ReconciliationPlan) Activations() []PlannedActivation {
	result := append([]PlannedActivation(nil), p.activations...)
	for i := range result {
		result[i].Pack = clonePack(result[i].Pack)
	}
	return result
}
func (p ReconciliationPlan) Blockers() []PlanBlocker {
	return append([]PlanBlocker(nil), p.blockers...)
}
func (p ReconciliationPlan) RetainedProjections() []RetainedProjection {
	result := append([]RetainedProjection(nil), p.retained...)
	for i := range result {
		result[i].Contributors = append([]string(nil), result[i].Contributors...)
	}
	return result
}
func (p ReconciliationPlan) Contributors() map[string][]string {
	result := make(map[string][]string, len(p.contributors))
	for id, contributors := range p.contributors {
		result[id] = append([]string(nil), contributors...)
	}
	return result
}
func (p ReconciliationPlan) RemovedContributors() map[string]string {
	result := make(map[string]string, len(p.removedContributors))
	for id, contributor := range p.removedContributors {
		result[id] = contributor
	}
	return result
}
func (p ReconciliationPlan) PortableOutcomes() []PortableOutcome {
	return append([]PortableOutcome(nil), p.portable...)
}
func (p ReconciliationPlan) Phases() []PlanPhase {
	result := make([]PlanPhase, len(p.phases))
	for i, phase := range p.phases {
		result[i] = phase
		result[i].Actions = append([]ProjectionAction(nil), phase.Actions...)
		for j := range result[i].Actions {
			result[i].Actions[j].Args = append([]string(nil), result[i].Actions[j].Args...)
		}
	}
	return result
}

func (p ReconciliationPlan) Resolutions() []ExecutableResolution {
	result := append([]ExecutableResolution(nil), p.resolutions...)
	for i := range result {
		result[i].AcquisitionArgs = append([]string(nil), result[i].AcquisitionArgs...)
	}
	return result
}

func (p ReconciliationPlan) PendingHumanActions() []string {
	return append([]string(nil), p.pendingHumanActions...)
}

func (p ReconciliationPlan) Readiness() ReadinessStatus { return p.readiness }
func (p ReconciliationPlan) Recovery() bool             { return p.recovery }
func (p ReconciliationPlan) HistoricalAttempt() *ApplyingJournal {
	if p.historicalAttempt == nil {
		return nil
	}
	copy := cloneJournal(*p.historicalAttempt)
	return &copy
}

type ApprovalReceipt struct {
	planDigest, phaseDigest string
	kind                    ConsentKind
}

type ApplyRequest struct {
	Plan        ReconciliationPlan
	Approvals   []ApprovalReceipt
	Interactive bool
}

type ApplyResult struct {
	Verified            bool
	PlanID              string
	Projections         int
	Readiness           ReadinessStatus
	PendingHumanActions []string
}

func (f Facade) Preview(ctx context.Context, request ActivationRequest) (ReconciliationPlan, error) {
	return f.preview(ctx, request, OperationActivate, "")
}

func (f Facade) PreviewUpdate(ctx context.Context, request UpdateRequest) (ReconciliationPlan, error) {
	activation := ActivationRequest{PackID: request.PackID, Surface: request.Surface}
	_, _, state, err := f.activationInputs(ctx, activation)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	intent, ok := intentForPack(state, request.PackID, request.Surface)
	if !ok || !intent.Active {
		return ReconciliationPlan{}, fmt.Errorf("capability pack %q is not active on %s", request.PackID, request.Surface)
	}
	return f.preview(ctx, activation, OperationUpdate, intent.Version)
}

func (f Facade) PreviewDeactivate(ctx context.Context, request DeactivationRequest) (ReconciliationPlan, error) {
	activation := ActivationRequest{PackID: request.PackID, Surface: request.Surface}
	requested, adapter, state, err := f.activationInputs(ctx, activation)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	intent, active := intentForPack(state, request.PackID, request.Surface)
	recovery := recoveryAttempt(state, OperationDeactivate, request.PackID, request.Surface)
	oldVersion := requested.Version
	if active && intent.Version != "" {
		oldVersion = intent.Version
	}
	before := f.compose(requested, state, request.Surface)
	target, dependents := f.composeWithout(requested, state, request.Surface)
	combined := target.combinedPack()
	resolutions, err := f.resolveExecutables(ctx, before.combinedPack())
	if err != nil {
		return ReconciliationPlan{}, err
	}
	observation, err := inspectDeactivation(ctx, adapter, before.combinedPack(), combined, resolutions)
	if err != nil {
		return ReconciliationPlan{}, fmt.Errorf("inspect deactivation of pack %q on %s: %w", requested.ID, request.Surface, err)
	}
	plan := ReconciliationPlan{pack: requested, operation: OperationDeactivate, surface: request.Surface, intentRevision: state.Intent.Revision, oldVersion: oldVersion, observationFingerprint: observationDigest(observation), resolutions: resolutions, contributors: target.contributors, compositionFacts: target.packs, beforeCompositionFacts: before.packs, intentFacts: target.intentFacts, ownershipFacts: cloneOwnership(state.Ownership), activeDependents: dependents, removedContributors: map[string]string{}}
	for id, contributors := range before.contributors {
		for _, contributor := range contributors {
			if contributor == requested.ID {
				plan.removedContributors[id] = contributor
			}
		}
	}
	for _, dependent := range dependents {
		plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerActiveDependent, Subject: requested.ID, Detail: fmt.Sprintf("cannot deactivate requested pack %s: active pack %s still requires capability/dependency %s; no automatic cascade will occur", requested.ID, dependent.PackID, dependent.Dependency)})
	}
	plan.blockers = append(plan.blockers, target.blockers...)
	sortBlockers(plan.blockers)
	for _, resource := range combined.Resources {
		plan.portable = append(plan.portable, PortableOutcome{Kind: resource.Kind, ID: resource.ID})
	}
	for _, projection := range observation.Projections {
		contributors := target.contributorSet(projection.ID)
		if projection.DesiredFingerprint != "" {
			plan.desired = append(plan.desired, projectionExpectation{ID: projection.ID, Fingerprint: projection.DesiredFingerprint, ExternallyManaged: projection.ExternallyManaged})
			if projection.Exists && projection.ObservedFingerprint == projection.DesiredFingerprint {
				plan.retained = append(plan.retained, RetainedProjection{ID: projection.ID, Contributors: contributors})
			} else {
				detail := fmt.Sprintf("preserved shared projection %s because it is missing, drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID)
				plan.pendingHumanActions = append(plan.pendingHumanActions, detail)
				plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: projection.ID, Detail: detail})
			}
			continue
		}
		owner, owned := ownershipByID(state.Ownership, projection.ID)
		if (active && intent.Active || recovery) && projection.Exists && owned && len(owner.Contributors) == 1 && owner.Contributors[0] == requested.ID && owner.Fingerprint == projection.ObservedFingerprint {
			plan.phases = appendPhaseAction(plan.phases, ConsentDestructiveCleanup, projection.Action)
			continue
		}
		if projection.Exists {
			plan.pendingHumanActions = append(plan.pendingHumanActions, fmt.Sprintf("preserved %s because it is drifted, ambiguous, unmanaged, or ownership no longer matches", projection.ID))
		}
	}
	if (!active || !intent.Active) && !recovery {
		plan.noOp = len(plan.phases) == 0 && len(plan.pendingHumanActions) == 0 && !hasContributor(state.Ownership, requested.ID)
		if !plan.noOp {
			plan.blockers = append(plan.blockers, PlanBlocker{Kind: BlockerOwnership, Subject: requested.ID, Detail: fmt.Sprintf("inactive pack %s has partial, drifted, or residual state; preserved it without starting general reconcile", requested.ID)})
		}
	}
	sortBlockers(plan.blockers)
	if len(plan.blockers) > 0 {
		plan.noOp = false
	}
	sort.Slice(plan.retained, func(i, j int) bool { return plan.retained[i].ID < plan.retained[j].ID })
	sort.Strings(plan.pendingHumanActions)
	plan.attachRecovery(state, recovery)
	plan.requireRecoveryApproval()
	plan.seal()
	return plan, nil
}

func hasContributor(values []ProjectionOwnership, packID string) bool {
	for _, value := range values {
		for _, contributor := range value.Contributors {
			if contributor == packID {
				return true
			}
		}
	}
	return false
}

func (f Facade) preview(ctx context.Context, request ActivationRequest, operation Operation, oldVersion string) (ReconciliationPlan, error) {
	requested, adapter, state, err := f.activationInputs(ctx, request)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	composition := f.compose(requested, state, request.Surface)
	pack := composition.combinedPack()
	resolutions, err := f.resolveExecutables(ctx, pack)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	var observation ActivationObservation
	if operation == OperationReconcile {
		observation, err = inspectReconcile(ctx, adapter, pack, state.Ownership, resolutions)
	} else {
		observation, err = inspectActivation(ctx, adapter, pack, resolutions)
	}
	if err != nil {
		return ReconciliationPlan{}, fmt.Errorf("inspect activation of pack %q on %s: %w", pack.ID, request.Surface, err)
	}

	actions := make([]ProjectionAction, 0, len(observation.Projections))
	destructiveActions := make([]ProjectionAction, 0)
	for _, projection := range observation.Projections {
		if projection.ID == "" || projection.DesiredFingerprint == "" || projection.Action.ID != projection.ID {
			return ReconciliationPlan{}, fmt.Errorf("inspect activation of pack %q on %s: adapter returned an invalid projection", pack.ID, request.Surface)
		}
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			if projection.ExternallyManaged {
				continue
			}
			owned := ownedAtComposition(state.Ownership, projection.ID, projection.ObservedFingerprint, composition)
			managedDrift := operation == OperationReconcile && projection.Exists && repairEligible(state.Ownership, projection, composition)
			if operation == OperationReconcile && (projection.Action.Mode == ProjectionDeleteTarget || projection.Action.Mode == ProjectionRemoveContent) {
				owner, ok := ownershipByID(state.Ownership, projection.ID)
				owned = ok && owner.Fingerprint == projection.ObservedFingerprint
			}
			if projection.Exists && !owned && !managedDrift {
				composition.blockers = append(composition.blockers, PlanBlocker{BlockerOwnership, projection.ID, fmt.Sprintf("projection is unmanaged or drifted; preserving existing %s content", request.Surface)})
				continue
			}
			if managedDrift {
				projection.Action.Description = "restore drifted Matty-managed projection " + projection.ID + " to catalog-current content: " + projection.Action.Description
			}
			if operation == OperationReconcile && (projection.Action.Mode == ProjectionDeleteTarget || projection.Action.Mode == ProjectionRemoveContent) {
				destructiveActions = append(destructiveActions, projection.Action)
			} else {
				actions = append(actions, projection.Action)
			}
		}
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	sort.Slice(destructiveActions, func(i, j int) bool { return destructiveActions[i].ID < destructiveActions[j].ID })
	externalActions, externalBlockers := f.externalPlan(pack, request.Surface, state, resolutions)
	composition.blockers = append(composition.blockers, externalBlockers...)
	sortBlockers(composition.blockers)
	noOp := compositionActive(state, composition.packs, request.Surface) && ownershipMatchesContributors(state.Ownership, observation.Projections, composition) && len(actions) == 0 && len(externalActions) == 0
	readiness := observation.Readiness
	readiness.Configured = noOp
	if !readiness.Configured {
		readiness.Authorized = false
		readiness.Usable = false
	} else if !readiness.Authorized {
		readiness.Usable = false
	}
	pendingHumanActions := append([]string(nil), observation.PendingHumanActions...)
	sort.Strings(pendingHumanActions)
	plan := ReconciliationPlan{pack: requested, operation: operation, surface: request.Surface, intentRevision: state.Intent.Revision, oldVersion: oldVersion, observationFingerprint: observationDigest(observation), resolutions: resolutions, readiness: readiness, pendingHumanActions: pendingHumanActions, noOp: noOp, activations: composition.activations, contributors: composition.contributors, blockers: composition.blockers, compositionFacts: composition.packs, intentFacts: composition.intentFacts, ownershipFacts: cloneOwnership(state.Ownership)}
	recovery := recoveryAttempt(state, operation, request.PackID, request.Surface)
	plan.attachRecovery(state, recovery)
	for _, resource := range pack.Resources {
		plan.portable = append(plan.portable, PortableOutcome{Kind: resource.Kind, ID: resource.ID})
	}
	sort.Slice(plan.portable, func(i, j int) bool {
		if plan.portable[i].Kind == plan.portable[j].Kind {
			return plan.portable[i].ID < plan.portable[j].ID
		}
		return plan.portable[i].Kind < plan.portable[j].Kind
	})
	for _, projection := range observation.Projections {
		plan.desired = append(plan.desired, projectionExpectation{projection.ID, projection.DesiredFingerprint, projection.ExternallyManaged})
		contributors := composition.contributorSet(projection.ID)
		if projection.ObservedFingerprint == projection.DesiredFingerprint && len(contributors) > 1 {
			plan.retained = append(plan.retained, RetainedProjection{ID: projection.ID, Contributors: contributors})
		}
	}
	sort.Slice(plan.desired, func(i, j int) bool { return plan.desired[i].ID < plan.desired[j].ID })
	sort.Slice(plan.retained, func(i, j int) bool { return plan.retained[i].ID < plan.retained[j].ID })
	if len(actions) > 0 {
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentReversibleLocal, ApprovalRequired: true, Actions: append([]ProjectionAction(nil), actions...)})
	}
	if len(externalActions) > 0 {
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentExecutableExternal, ApprovalRequired: true, Actions: append([]ProjectionAction(nil), externalActions...)})
	}
	if len(destructiveActions) > 0 {
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentDestructiveCleanup, ApprovalRequired: true, Actions: append([]ProjectionAction(nil), destructiveActions...)})
	}
	if len(pendingHumanActions) > 0 {
		hostActions := make([]ProjectionAction, 0, len(pendingHumanActions))
		for i, action := range pendingHumanActions {
			hostActions = append(hostActions, ProjectionAction{ID: fmt.Sprintf("host-follow-up:%s:%d", request.Surface, i), Kind: ActionHostFollowUp, Description: action})
		}
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentHostFollowUp, Actions: hostActions})
	}
	if len(plan.blockers) > 0 {
		plan.noOp = false
	}
	plan.requireRecoveryApproval()
	plan.seal()
	return plan, nil
}

func (f Facade) Approve(plan ReconciliationPlan, kind ConsentKind) ApprovalReceipt {
	for _, phase := range plan.phases {
		if phase.Kind == kind && phase.ApprovalRequired {
			return ApprovalReceipt{planDigest: plan.digest, phaseDigest: phase.Digest, kind: kind}
		}
	}
	return ApprovalReceipt{}
}

func (f Facade) Apply(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	if !request.Plan.Applicable() {
		return ApplyResult{}, PlanNotActionableError{Disposition: request.Plan.Disposition()}
	}
	if !request.Plan.validSeal() {
		return ApplyResult{}, ErrApprovalMismatch
	}
	if request.Plan.noOp {
		if _, err := f.preflightPlan(ctx, request.Plan); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Verified: true, PlanID: request.Plan.id, Readiness: request.Plan.readiness, PendingHumanActions: request.Plan.PendingHumanActions()}, nil
	}
	if !request.Interactive {
		return ApplyResult{}, ErrInteractiveRequired
	}
	for _, phase := range request.Plan.phases {
		if !phase.ApprovalRequired {
			continue
		}
		approved := false
		for _, receipt := range request.Approvals {
			if receipt.planDigest == request.Plan.digest && receipt.phaseDigest == phase.Digest && receipt.kind == phase.Kind {
				approved = true
				break
			}
		}
		if !approved {
			return ApplyResult{}, ErrApprovalMismatch
		}
	}
	preflight, err := f.preflightPlan(ctx, request.Plan)
	if err != nil {
		return ApplyResult{}, err
	}
	pack, adapter, state := preflight.pack, preflight.adapter, preflight.state
	currentComposition, combined, resolutions := preflight.composition, preflight.combined, preflight.resolutions
	if hasPhaseActions(request.Plan.phases, ConsentExecutableExternal) && f.activation.executor == nil {
		return ApplyResult{}, fmt.Errorf("external effects are not configured")
	}

	actions := flattenActions(request.Plan.phases)
	state.SchemaVersion = 1
	if request.Plan.operation != OperationReconcile && !request.Plan.recovery {
		previousIntents := activeIntents(state)
		activeTarget := request.Plan.operation != OperationDeactivate
		state.Intent = ActivationIntent{PackID: pack.ID, Surface: request.Plan.surface, Version: pack.Version, Active: activeTarget, Revision: state.Intent.Revision + 1}
		byID := map[string]ActivationIntent{}
		for _, intent := range previousIntents {
			byID[intent.PackID] = intent
		}
		for _, activation := range request.Plan.activations {
			byID[activation.Pack.ID] = ActivationIntent{PackID: activation.Pack.ID, Surface: request.Plan.surface, Version: activation.Pack.Version, Active: true, Revision: state.Intent.Revision}
		}
		if request.Plan.operation == OperationDeactivate {
			byID[pack.ID] = state.Intent
		}
		state.Intents = nil
		for _, intent := range byID {
			state.Intents = append(state.Intents, intent)
		}
		sort.Slice(state.Intents, func(i, j int) bool { return state.Intents[i].PackID < state.Intents[j].PackID })
	}
	if request.Plan.recovery && state.Journal != nil {
		state.History = append(state.History, cloneJournal(*request.Plan.historicalAttempt))
	}
	state.Journal = &ApplyingJournal{PlanID: request.Plan.id, PlanDigest: request.Plan.digest, Operation: request.Plan.operation, Surface: request.Plan.surface, PackID: request.Plan.pack.ID, Outcome: AttemptApplying}
	for _, action := range actions {
		if action.Kind != ActionHostFollowUp {
			state.Journal.Actions = append(state.Journal.Actions, action.ID)
		}
	}
	if err := f.activation.store.Save(ctx, request.Plan.surface, request.Plan.intentRevision, state); err != nil {
		return ApplyResult{}, err
	}
	localActions := phaseActions(request.Plan.phases, ConsentReversibleLocal)
	if len(localActions) > 0 {
		if err := adapter.ApplyProjections(ctx, localActions); err != nil {
			state.Journal.recordFailure(requiredFailedActionID(err, "reversible-local"), err)
			if saveErr := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("apply reversible local projections: %v; could not persist recovery facts: %w", err, saveErr)
			}
			return ApplyResult{}, err
		}
	}
	destructiveActions := phaseActions(request.Plan.phases, ConsentDestructiveCleanup)
	var verified ActivationObservation
	if request.Plan.operation == OperationReconcile {
		verified, err = inspectReconcile(ctx, adapter, combined, state.Ownership, resolutions)
	} else {
		verified, err = inspectActivation(ctx, adapter, combined, resolutions)
	}
	if err != nil {
		state.Journal.recordFailure("verify-reversible-local", err)
		if saveErr := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); saveErr != nil {
			return ApplyResult{}, fmt.Errorf("verify reversible local projections: %v; could not persist recovery facts: %w", err, saveErr)
		}
		return ApplyResult{}, err
	}
	verificationDesired := withoutExternallyManagedExpectations(request.Plan.desired)
	if len(destructiveActions) > 0 {
		verificationDesired = withoutActionExpectations(verificationDesired, destructiveActions)
	}
	verifiedMatches := verificationMatches(verificationDesired, verified.Projections)
	if len(verificationDesired) != len(request.Plan.desired) {
		verifiedMatches = verificationMatchesSubset(verificationDesired, verified.Projections)
	}
	if request.Plan.operation == OperationReconcile && request.Plan.reconcileScope == ReconcileTargeted {
		verifiedMatches = verificationMatchesSubset(verificationDesired, verified.Projections)
	}
	if request.Plan.operation == OperationDeactivate && len(destructiveActions) > 0 {
		verifiedMatches = verificationMatchesSubset(verificationDesired, verified.Projections)
	} else if request.Plan.operation == OperationDeactivate {
		verifiedMatches = verificationMatchesDeactivation(request.Plan.desired, verified.Projections)
	}
	if !verifiedMatches {
		state.Journal.recordFailure("verify-reversible-local", errors.New(verificationMismatch(request.Plan.desired, verified.Projections)))
		if saveErr := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); saveErr != nil {
			return ApplyResult{}, fmt.Errorf("%w: %s; could not persist recovery facts: %v", ErrVerificationFailed, state.Journal.FailureDetail, saveErr)
		}
		return ApplyResult{}, fmt.Errorf("%w: %s", ErrVerificationFailed, verificationMismatch(request.Plan.desired, verified.Projections))
	}
	externalActions := phaseActions(request.Plan.phases, ConsentExecutableExternal)
	for _, action := range localActions {
		state.Journal.Completed = appendCompleted(state.Journal.Completed, action.ID)
	}
	if len(externalActions) > 0 {
		if err := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); err != nil {
			return ApplyResult{}, fmt.Errorf("persist verified local recovery facts: %w", err)
		}
	}
	if len(externalActions) == 0 && len(destructiveActions) > 0 {
		if err := adapter.ApplyProjections(ctx, destructiveActions); err != nil {
			state.Journal.recordFailure(requiredFailedActionID(err, "destructive-cleanup"), err)
			_ = f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state)
			return ApplyResult{}, err
		}
		for _, action := range destructiveActions {
			state.Journal.Completed = appendCompleted(state.Journal.Completed, action.ID)
		}
		if err := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); err != nil {
			return ApplyResult{}, fmt.Errorf("destructive actions completed but recovery facts could not be persisted: %w", err)
		}
	}
	for _, action := range externalActions {
		if err := f.activation.executor.Execute(ctx, action); err != nil {
			state.Journal.recordFailure(action.ID, err)
			if saveErr := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("external action %s failed: %v; could not persist recovery facts: %w", action.ID, err, saveErr)
			}
			return ApplyResult{}, fmt.Errorf("external action %s failed; later actions stopped and recovery is required: %w", action.ID, err)
		}
		state.Journal.Completed = append(state.Journal.Completed, action.ID)
		state.External = recordExternalEffect(state.External, action)
		if err := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); err != nil {
			return ApplyResult{}, fmt.Errorf("external action %s completed but recovery facts could not be persisted: %w", action.ID, err)
		}
	}
	if len(externalActions) > 0 && len(destructiveActions) > 0 {
		if err := adapter.ApplyProjections(ctx, destructiveActions); err != nil {
			state.Journal.recordFailure(requiredFailedActionID(err, "destructive-cleanup"), err)
			_ = f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state)
			return ApplyResult{}, err
		}
		for _, action := range destructiveActions {
			state.Journal.Completed = appendCompleted(state.Journal.Completed, action.ID)
		}
		if err := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); err != nil {
			return ApplyResult{}, fmt.Errorf("destructive actions completed but recovery facts could not be persisted: %w", err)
		}
	}
	if len(externalActions) > 0 || len(destructiveActions) > 0 {
		if request.Plan.operation == OperationReconcile {
			verified, err = inspectReconcile(ctx, adapter, combined, state.Ownership, resolutions)
		} else {
			verified, err = inspectActivation(ctx, adapter, combined, resolutions)
		}
		if err != nil {
			state.Journal.recordFailure("verify-after-external", err)
			if saveErr := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("verify after external effects: %v; could not persist recovery facts: %w", err, saveErr)
			}
			return ApplyResult{}, err
		}
		matches := verificationMatches(request.Plan.desired, verified.Projections)
		if request.Plan.operation == OperationReconcile && request.Plan.reconcileScope == ReconcileTargeted {
			matches = verificationMatchesSubset(request.Plan.desired, verified.Projections)
		}
		if request.Plan.operation == OperationDeactivate {
			matches = verificationMatchesDeactivation(request.Plan.desired, verified.Projections)
		}
		if !matches {
			state.Journal.recordFailure("verify-after-external", errors.New(verificationMismatch(request.Plan.desired, verified.Projections)))
			if saveErr := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); saveErr != nil {
				return ApplyResult{}, fmt.Errorf("%w: %s; could not persist recovery facts: %v", ErrVerificationFailed, state.Journal.FailureDetail, saveErr)
			}
			return ApplyResult{}, fmt.Errorf("%w: %s", ErrVerificationFailed, verificationMismatch(request.Plan.desired, verified.Projections))
		}
	}
	verifiedAttempt := cloneJournal(*state.Journal)
	verifiedAttempt.Outcome = AttemptVerified
	state.LastAttempts = recordLatestAttempt(state.LastAttempts, verifiedAttempt)
	state.Journal = nil
	previousOwnership := cloneOwnership(state.Ownership)
	state.Ownership = make([]ProjectionOwnership, 0, len(verified.Projections))
	if request.Plan.operation == OperationReconcile && request.Plan.reconcileScope == ReconcileTargeted {
		desiredIDs := map[string]bool{}
		for _, expectation := range request.Plan.desired {
			desiredIDs[expectation.ID] = true
		}
		for _, owner := range previousOwnership {
			if !desiredIDs[owner.ID] {
				state.Ownership = append(state.Ownership, owner)
			}
		}
	}
	for _, projection := range verified.Projections {
		if projection.ExternallyManaged || projection.DesiredFingerprint == "" || hasPhaseActionID(request.Plan.phases, ConsentDestructiveCleanup, projection.ID) || (request.Plan.operation == OperationReconcile && request.Plan.reconcileScope == ReconcileTargeted && !hasExpectation(request.Plan.desired, projection.ID)) {
			continue
		}
		state.Ownership = append(state.Ownership, ProjectionOwnership{ID: projection.ID, Contributors: currentComposition.contributorSet(projection.ID), Fingerprint: projection.DesiredFingerprint})
	}
	sort.Slice(state.Ownership, func(i, j int) bool { return state.Ownership[i].ID < state.Ownership[j].ID })
	if err := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); err != nil {
		return ApplyResult{}, err
	}
	readiness := verified.Readiness
	readiness.Configured = true
	if !readiness.Authorized {
		readiness.Usable = false
	}
	return ApplyResult{Verified: true, PlanID: request.Plan.id, Projections: len(state.Ownership), Readiness: readiness, PendingHumanActions: append([]string(nil), verified.PendingHumanActions...)}, nil
}

func verificationMatchesSubset(desired []projectionExpectation, observed []ObservedProjection) bool {
	byID := make(map[string]ObservedProjection, len(observed))
	for _, projection := range observed {
		byID[projection.ID] = projection
	}
	for _, expectation := range desired {
		projection, ok := byID[expectation.ID]
		if !ok || projection.ObservedFingerprint != expectation.Fingerprint {
			return false
		}
	}
	return true
}

func withoutActionExpectations(values []projectionExpectation, actions []ProjectionAction) []projectionExpectation {
	ids := map[string]bool{}
	for _, action := range actions {
		ids[action.ID] = true
	}
	result := make([]projectionExpectation, 0, len(values))
	for _, value := range values {
		if !ids[value.ID] {
			result = append(result, value)
		}
	}
	return result
}

func hasPhaseActionID(phases []PlanPhase, kind ConsentKind, id string) bool {
	for _, action := range phaseActions(phases, kind) {
		if action.ID == id {
			return true
		}
	}
	return false
}

func hasExpectation(values []projectionExpectation, id string) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}

type planPreflight struct {
	pack        Pack
	adapter     ActivationAdapter
	state       ActivationState
	composition composition
	combined    Pack
	resolutions []ExecutableResolution
}

func (f Facade) preflightPlan(ctx context.Context, plan ReconciliationPlan) (planPreflight, error) {
	freshCatalog, err := f.catalog.refreshed()
	if err != nil {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("catalog or manifest changed after Preview: %v; rerun %s to preview a fresh plan", err, plan.operation)}
	}
	f.catalog = freshCatalog
	pack, adapter, state, err := f.activationInputs(ctx, ActivationRequest{PackID: plan.pack.ID, Surface: plan.surface})
	if err != nil {
		return planPreflight{}, err
	}
	if plan.operation == OperationReconcile && state.Intent.Revision != plan.intentRevision {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("activation intent revision changed from %d to %d; rerun %s to preview a fresh plan", plan.intentRevision, state.Intent.Revision, plan.operation)}
	}
	if plan.recovery {
		currentHistory := normalizedRecoveryJournal(state.Journal)
		if currentHistory == nil || digestJSON(currentHistory) != digestJSON(plan.historicalAttempt) {
			return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("recovery attempt history changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
		}
	}
	current := f.compose(pack, state, plan.surface)
	if plan.operation == OperationDeactivate {
		before := current
		target, dependents := f.composeWithout(pack, state, plan.surface)
		if digestJSON(before.packs) != digestJSON(plan.beforeCompositionFacts) || digestJSON(dependents) != digestJSON(plan.activeDependents) {
			return planPreflight{}, StalePlanError{Precondition: "dependency closure or active dependents changed after Preview; rerun deactivate to preview a fresh plan"}
		}
		current = target
	}
	if plan.operation == OperationReconcile && digestJSON(current.intentFacts) != digestJSON(plan.intentFacts) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("active intents or intent revisions changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	planned := composition{packs: plan.compositionFacts, activations: plan.activations, contributors: plan.contributors, blockers: plan.blockers, intentFacts: plan.intentFacts}
	if current.identityDigest() != planned.identityDigest() {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("dependency or catalog composition changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	if digestJSON(cloneOwnership(state.Ownership)) != digestJSON(plan.ownershipFacts) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("projection ownership changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	combined := current.combinedPack()
	resolutionPack := combined
	if plan.operation == OperationDeactivate {
		resolutionPack = composition{requested: pack, packs: plan.beforeCompositionFacts}.combinedPack()
	}
	resolutions, err := f.resolveExecutables(ctx, resolutionPack)
	if err != nil {
		return planPreflight{}, err
	}
	if !sameResolutions(plan.resolutions, resolutions) {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("executable resolution changed after Preview; rerun %s to preview a fresh plan", plan.operation)}
	}
	var observation ActivationObservation
	if plan.operation == OperationDeactivate {
		before := composition{requested: pack, packs: plan.beforeCompositionFacts}.combinedPack()
		observation, err = inspectDeactivation(ctx, adapter, before, combined, resolutions)
	} else if plan.operation == OperationReconcile {
		observation, err = inspectReconcile(ctx, adapter, combined, state.Ownership, resolutions)
	} else {
		observation, err = inspectActivation(ctx, adapter, combined, resolutions)
	}
	if err != nil {
		return planPreflight{}, err
	}
	if state.Intent.Revision != plan.intentRevision {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("activation intent revision changed from %d to %d; rerun %s to preview a fresh plan", plan.intentRevision, state.Intent.Revision, plan.operation)}
	}
	if observationDigest(observation) != plan.observationFingerprint {
		return planPreflight{}, StalePlanError{Precondition: fmt.Sprintf("%s projections changed after Preview; rerun %s to preview a fresh plan", plan.surface, plan.operation)}
	}
	return planPreflight{pack: pack, adapter: adapter, state: state, composition: current, combined: combined, resolutions: resolutions}, nil
}

func appendCompleted(completed []string, id string) []string {
	for _, existing := range completed {
		if existing == id {
			return completed
		}
	}
	return append(completed, id)
}

func (f Facade) activationInputs(ctx context.Context, request ActivationRequest) (Pack, ActivationAdapter, ActivationState, error) {
	if f.activation == nil || f.activation.store == nil {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("activation is not configured")
	}
	if request.Surface != SurfaceCodex && request.Surface != SurfaceOpenCode {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("activation does not support CLI surface %q", request.Surface)
	}
	pack, err := f.catalog.Show(request.PackID)
	if err != nil {
		return Pack{}, nil, ActivationState{}, err
	}
	adapter := f.activation.adapters[request.Surface]
	if adapter == nil {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("no activation adapter configured for CLI surface %q", request.Surface)
	}
	state, err := f.activation.store.Load(ctx, request.Surface)
	return pack, adapter, state, err
}

func (p *ReconciliationPlan) seal() {
	for i := range p.phases {
		p.phases[i].Digest = digestJSON(struct {
			Kind             ConsentKind
			ApprovalRequired bool
			Actions          []ProjectionAction
		}{p.phases[i].Kind, p.phases[i].ApprovalRequired, p.phases[i].Actions})
	}
	p.digest = digestJSON(p.sealPayload())
	p.id = "plan-" + p.digest[:12]
}

func recoveryAttempt(state ActivationState, operation Operation, packID string, surface Surface) bool {
	journal := state.Journal
	if journal == nil || (journal.Outcome != AttemptRecoveryRequired && journal.Outcome != AttemptApplying) || journal.Operation != operation || journal.PackID != packID || journal.Surface != surface {
		return false
	}
	intent, ok := intentForPack(state, packID, surface)
	switch operation {
	case OperationActivate, OperationUpdate:
		return ok && intent.Active
	case OperationDeactivate:
		return !ok || !intent.Active
	default:
		return false
	}
}

func (p *ReconciliationPlan) attachRecovery(state ActivationState, recovery bool) {
	if !recovery || state.Journal == nil {
		return
	}
	p.recovery = true
	p.historicalAttempt = normalizedRecoveryJournal(state.Journal)
}

func normalizedRecoveryJournal(value *ApplyingJournal) *ApplyingJournal {
	if value == nil {
		return nil
	}
	journal := cloneJournal(*value)
	if journal.Outcome == AttemptApplying {
		journal.Outcome = AttemptRecoveryRequired
		if journal.FailedAction == "" {
			journal.FailedAction = "interrupted"
		}
		if journal.FailureDetail == "" {
			journal.FailureDetail = "attempt was interrupted before a terminal outcome was durably recorded"
		}
	}
	return &journal
}

func (p *ReconciliationPlan) requireRecoveryApproval() {
	if !p.recovery || len(p.blockers) > 0 {
		return
	}
	p.noOp = false
	for _, phase := range p.phases {
		if phase.ApprovalRequired {
			return
		}
	}
	p.phases = append([]PlanPhase{{Kind: ConsentReversibleLocal, ApprovalRequired: true}}, p.phases...)
}
func (p ReconciliationPlan) validSeal() bool {
	copy := p
	copy.seal()
	return copy.digest == p.digest && copy.id == p.id
}
func (p ReconciliationPlan) sealPayload() any {
	return struct {
		PackID, Version string
		Operation       Operation
		Surface         Surface
		IntentRevision  int
		OldVersion      string
		Observation     string
		Phases          []PlanPhase
		Desired         []projectionExpectation
		Portable        []PortableOutcome
		Resolutions     []ExecutableResolution
		Readiness       ReadinessStatus
		Pending         []string
		NoOp            bool
		Activations     []PlannedActivation
		Contributors    map[string][]string
		Retained        []RetainedProjection
		Blockers        []PlanBlocker
		Composition     []Pack
		IntentFacts     []ActivationIntent
		OwnershipFacts  []ProjectionOwnership
		Dependents      []ActiveDependent
		Before          []Pack
		Removed         map[string]string
		ReconcileScope  ReconcileScope
		Recovery        bool
		Historical      *ApplyingJournal
	}{p.pack.ID, p.pack.Version, p.operation, p.surface, p.intentRevision, p.oldVersion, p.observationFingerprint, p.phases, p.desired, p.portable, p.resolutions, p.readiness, p.pendingHumanActions, p.noOp, p.activations, p.contributors, p.retained, p.blockers, p.compositionFacts, p.intentFacts, p.ownershipFacts, p.activeDependents, p.beforeCompositionFacts, p.removedContributors, p.reconcileScope, p.recovery, p.historicalAttempt}
}

func ownershipByID(values []ProjectionOwnership, id string) (ProjectionOwnership, bool) {
	for _, value := range values {
		if value.ID == id {
			return value, true
		}
	}
	return ProjectionOwnership{}, false
}

func appendPhaseAction(phases []PlanPhase, kind ConsentKind, action ProjectionAction) []PlanPhase {
	for i := range phases {
		if phases[i].Kind == kind {
			phases[i].Actions = append(phases[i].Actions, action)
			return phases
		}
	}
	return append(phases, PlanPhase{Kind: kind, ApprovalRequired: true, Actions: []ProjectionAction{action}})
}

func cloneOwnership(values []ProjectionOwnership) []ProjectionOwnership {
	result := append([]ProjectionOwnership(nil), values...)
	for i := range result {
		result[i].Contributors = append([]string(nil), result[i].Contributors...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func intentForPack(state ActivationState, packID string, surface Surface) (ActivationIntent, bool) {
	for _, intent := range activeIntents(state) {
		if intent.PackID == packID && intent.Surface == surface {
			return intent, true
		}
	}
	return ActivationIntent{}, false
}

func compositionActive(state ActivationState, packs []Pack, surface Surface) bool {
	active := map[string]ActivationIntent{}
	for _, intent := range activeIntents(state) {
		active[intent.PackID] = intent
	}
	for _, pack := range packs {
		intent, ok := active[pack.ID]
		if !ok || !intent.Active || intent.Surface != surface || intent.Version != pack.Version {
			return false
		}
	}
	return len(packs) > 0
}

func ownershipMatchesContributors(owners []ProjectionOwnership, projections []ObservedProjection, c composition) bool {
	managedCount := 0
	for _, projection := range projections {
		if !projection.ExternallyManaged {
			managedCount++
		}
	}
	if len(owners) != managedCount {
		return false
	}
	byID := map[string]ProjectionOwnership{}
	for _, owner := range owners {
		byID[owner.ID] = owner
	}
	for _, projection := range projections {
		if projection.ExternallyManaged {
			if projection.ObservedFingerprint != projection.DesiredFingerprint {
				return false
			}
			continue
		}
		owner, ok := byID[projection.ID]
		if !ok || owner.Fingerprint != projection.DesiredFingerprint || digestJSON(owner.Contributors) != digestJSON(c.contributorSet(projection.ID)) {
			return false
		}
	}
	return true
}

func withoutExternallyManagedExpectations(values []projectionExpectation) []projectionExpectation {
	result := make([]projectionExpectation, 0, len(values))
	for _, value := range values {
		if !value.ExternallyManaged {
			result = append(result, value)
		}
	}
	return result
}
func digestJSON(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func observationDigest(o ActivationObservation) string {
	normalized := ActivationObservation{Revision: o.Revision, Projections: append([]ObservedProjection(nil), o.Projections...), Readiness: o.Readiness, PendingHumanActions: append([]string(nil), o.PendingHumanActions...)}
	sort.Slice(normalized.Projections, func(i, j int) bool { return normalized.Projections[i].ID < normalized.Projections[j].ID })
	sort.Strings(normalized.PendingHumanActions)
	return digestJSON(normalized)
}
func flattenActions(phases []PlanPhase) []ProjectionAction {
	var actions []ProjectionAction
	for _, phase := range phases {
		actions = append(actions, phase.Actions...)
	}
	return actions
}
func verificationMatches(expected []projectionExpectation, values []ObservedProjection) bool {
	if len(values) != len(expected) || len(values) == 0 {
		return false
	}
	byID := map[string]ObservedProjection{}
	for _, value := range values {
		byID[value.ID] = value
	}
	for _, want := range expected {
		value, ok := byID[want.ID]
		if !ok || value.DesiredFingerprint != want.Fingerprint || value.ObservedFingerprint != want.Fingerprint {
			return false
		}
	}
	return true
}

func verificationMatchesDeactivation(expected []projectionExpectation, values []ObservedProjection) bool {
	byID := map[string]ObservedProjection{}
	for _, value := range values {
		byID[value.ID] = value
	}
	for _, want := range expected {
		value, ok := byID[want.ID]
		if !ok || value.ObservedFingerprint != want.Fingerprint || value.DesiredFingerprint != want.Fingerprint {
			return false
		}
		delete(byID, want.ID)
	}
	for _, value := range byID {
		if value.Exists {
			return false
		}
	}
	return true
}

func verificationMismatch(expected []projectionExpectation, values []ObservedProjection) string {
	want := map[string]string{}
	for _, projection := range expected {
		want[projection.ID] = projection.Fingerprint
	}
	got := map[string]string{}
	for _, projection := range values {
		got[projection.ID] = projection.ObservedFingerprint
	}
	var details []string
	ids := make([]string, 0, len(want)+len(got))
	seen := map[string]bool{}
	for id := range want {
		seen[id] = true
		ids = append(ids, id)
	}
	for id := range got {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if want[id] != got[id] {
			details = append(details, fmt.Sprintf("%s expected %s observed %s", id, want[id], got[id]))
		}
	}
	return fmt.Sprintf("expected %d projections, observed %d; %s", len(expected), len(values), strings.Join(details, "; "))
}
func ownershipMatches(owners []ProjectionOwnership, projections []ObservedProjection, packID string) bool {
	if len(owners) != len(projections) {
		return false
	}
	byID := map[string]ProjectionOwnership{}
	for _, owner := range owners {
		byID[owner.ID] = owner
	}
	for _, projection := range projections {
		owner, ok := byID[projection.ID]
		if !ok || owner.Fingerprint != projection.DesiredFingerprint || len(owner.Contributors) != 1 || owner.Contributors[0] != packID {
			return false
		}
	}
	return true
}
func ownedAtFingerprint(owners []ProjectionOwnership, id, fingerprint, packID string) bool {
	for _, owner := range owners {
		if owner.ID == id && owner.Fingerprint == fingerprint && len(owner.Contributors) == 1 && owner.Contributors[0] == packID {
			return true
		}
	}
	return false
}
func ownedAtComposition(owners []ProjectionOwnership, id, fingerprint string, c composition) bool {
	for _, owner := range owners {
		if owner.ID == id && owner.Fingerprint == fingerprint && digestJSON(owner.Contributors) == digestJSON(c.contributorSet(id)) {
			return true
		}
	}
	return false
}

func repairEligible(owners []ProjectionOwnership, projection ObservedProjection, c composition) bool {
	if projection.Action.Mode == ProjectionDeleteTarget || projection.Action.Mode == ProjectionRemoveContent {
		return false
	}
	var matched []ProjectionOwnership
	for _, owner := range owners {
		if owner.ID == projection.ID {
			matched = append(matched, owner)
		}
	}
	if len(matched) != 1 {
		return false
	}
	owner := matched[0]
	return owner.Fingerprint == projection.DesiredFingerprint && digestJSON(owner.Contributors) == digestJSON(c.contributorSet(projection.ID))
}
func cloneActivationState(state ActivationState) ActivationState {
	state.Ownership = append([]ProjectionOwnership(nil), state.Ownership...)
	state.Intents = append([]ActivationIntent(nil), state.Intents...)
	for i := range state.Ownership {
		state.Ownership[i].Contributors = append([]string(nil), state.Ownership[i].Contributors...)
	}
	if state.Journal != nil {
		journal := cloneJournal(*state.Journal)
		state.Journal = &journal
	}
	state.LastAttempts = append([]ApplyingJournal(nil), state.LastAttempts...)
	for i := range state.LastAttempts {
		state.LastAttempts[i] = cloneJournal(state.LastAttempts[i])
	}
	state.History = append([]ApplyingJournal(nil), state.History...)
	for i := range state.History {
		state.History[i] = cloneJournal(state.History[i])
	}
	state.External = append([]ExternalEffect(nil), state.External...)
	return state
}

func cloneJournal(journal ApplyingJournal) ApplyingJournal {
	journal.Actions = append([]string(nil), journal.Actions...)
	journal.Completed = append([]string(nil), journal.Completed...)
	return journal
}

func recordLatestAttempt(attempts []ApplyingJournal, attempt ApplyingJournal) []ApplyingJournal {
	result := append([]ApplyingJournal(nil), attempts...)
	for i := range result {
		if result[i].PackID == attempt.PackID && result[i].Surface == attempt.Surface {
			result[i] = cloneJournal(attempt)
			return result
		}
	}
	return append(result, cloneJournal(attempt))
}

func (f Facade) externalPlan(pack Pack, surface Surface, state ActivationState, resolutions []ExecutableResolution) ([]ProjectionAction, []PlanBlocker) {
	var actions []ProjectionAction
	var blockers []PlanBlocker
	for _, resolution := range resolutions {
		if !resolution.Available {
			if !resolution.AcquisitionSupported || strings.TrimSpace(resolution.AcquisitionCommand) == "" {
				blockers = append(blockers, PlanBlocker{BlockerGlobalRequirement, resolution.Tool, "no supported acquisition action is available; configure a supported acquisition or install it before retrying"})
				continue
			}
			acquisition := ProjectionAction{ID: "external:" + resolution.Tool + ":acquire", Kind: ActionExternalCommand, Command: resolution.AcquisitionCommand, Args: append([]string(nil), resolution.AcquisitionArgs...), Description: fmt.Sprintf("acquire global tool %s via %s %s", resolution.Tool, resolution.AcquisitionCommand, strings.Join(resolution.AcquisitionArgs, " "))}
			if !externalEffectCompleted(state.External, acquisition) {
				actions = append(actions, acquisition)
			}
		}
		if strings.TrimSpace(resolution.Path) == "" {
			blockers = append(blockers, PlanBlocker{BlockerGlobalRequirement, resolution.Tool, "resolved tool has no executable path"})
			continue
		}
		setup := ProjectionAction{ID: "external:" + resolution.Tool + ":setup:" + string(surface), Kind: ActionExternalCommand, Command: resolution.Path, Args: []string{"setup", string(surface)}, Description: fmt.Sprintf("run %s setup %s", resolution.Path, surface)}
		if !externalEffectCompleted(state.External, setup) || externalVerificationNeedsRetry(state, setup, surface) {
			actions = append(actions, setup)
		}
	}
	sortBlockers(blockers)
	return actions, blockers
}

func externalVerificationNeedsRetry(state ActivationState, setup ProjectionAction, surface Surface) bool {
	if state.Journal == nil || state.Journal.Outcome != AttemptRecoveryRequired || state.Journal.FailedAction != "verify-after-external" || !slices.Contains(state.Journal.Completed, setup.ID) {
		return false
	}
	return state.Journal.Surface == surface
}

func inspectActivation(ctx context.Context, adapter ActivationAdapter, pack Pack, resolutions []ExecutableResolution) (ActivationObservation, error) {
	var observation ActivationObservation
	var err error
	if resolved, ok := adapter.(ResolutionAwareActivationAdapter); ok {
		observation, err = resolved.InspectActivationWithResolution(ctx, pack, resolutions)
	} else {
		observation, err = adapter.InspectActivation(ctx, pack)
	}
	if err != nil {
		return ActivationObservation{}, err
	}
	if pack.ID == "matty" {
		observation.Readiness.Authorized = true
		observation.Readiness.Usable = true
	}
	return observation, nil
}

func (f Facade) resolveExecutables(ctx context.Context, pack Pack) ([]ExecutableResolution, error) {
	if len(pack.Requires.Tools) == 0 {
		return nil, nil
	}
	if f.activation == nil || f.activation.resolver == nil {
		return nil, fmt.Errorf("pack %q requires an executable resolver", pack.ID)
	}
	result := make([]ExecutableResolution, 0, len(pack.Requires.Tools))
	for _, tool := range pack.Requires.Tools {
		resolution, err := f.activation.resolver.Resolve(ctx, tool)
		if err != nil {
			return nil, fmt.Errorf("resolve required executable %q: %w", tool, err)
		}
		resolution.Tool = tool
		resolution.AcquisitionArgs = append([]string(nil), resolution.AcquisitionArgs...)
		if resolution.Precondition == "" {
			resolution.Precondition = resolutionFingerprint(resolution)
		}
		result = append(result, resolution)
	}
	return result, nil
}

func ResolvedExecutablePath(command string, resolutions []ExecutableResolution) string {
	for _, resolution := range resolutions {
		if resolution.Tool == command && resolution.Path != "" {
			return resolution.Path
		}
	}
	return command
}

func resolutionFingerprint(resolution ExecutableResolution) string {
	return digestJSON(struct {
		Tool, Path, ResolvedPath, Origin, Version, Precondition string
		Available, AcquisitionSupported                         bool
		AcquisitionCommand                                      string
		AcquisitionArgs                                         []string
	}{resolution.Tool, resolution.Path, resolution.ResolvedPath, resolution.Origin, resolution.Version, "", resolution.Available, resolution.AcquisitionSupported, resolution.AcquisitionCommand, resolution.AcquisitionArgs})
}

func sameResolutions(want, got []ExecutableResolution) bool {
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if resolutionFingerprint(want[i]) != resolutionFingerprint(got[i]) || want[i].Precondition != got[i].Precondition {
			return false
		}
	}
	return true
}

func externalEffectFingerprint(action ProjectionAction) string {
	return digestJSON(struct {
		ID, Kind, Command, Description string
		Args                           []string
	}{action.ID, string(action.Kind), action.Command, action.Description, action.Args})
}

func externalEffectCompleted(effects []ExternalEffect, action ProjectionAction) bool {
	want := externalEffectFingerprint(action)
	for _, effect := range effects {
		if effect.ID == action.ID && effect.Fingerprint == want {
			return true
		}
	}
	return false
}

func recordExternalEffect(effects []ExternalEffect, action ProjectionAction) []ExternalEffect {
	result := append([]ExternalEffect(nil), effects...)
	want := externalEffectFingerprint(action)
	for i := range result {
		if result[i].ID == action.ID {
			result[i].Fingerprint = want
			return result
		}
	}
	return append(result, ExternalEffect{ID: action.ID, Fingerprint: want})
}

func phaseActions(phases []PlanPhase, kind ConsentKind) []ProjectionAction {
	var actions []ProjectionAction
	for _, phase := range phases {
		if phase.Kind == kind {
			for _, action := range phase.Actions {
				action.Args = append([]string(nil), action.Args...)
				actions = append(actions, action)
			}
		}
	}
	return actions
}

func hasPhaseActions(phases []PlanPhase, kind ConsentKind) bool {
	return len(phaseActions(phases, kind)) > 0
}
