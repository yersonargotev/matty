package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrInteractiveRequired = errors.New("Apply requires an interactive terminal")
	ErrApprovalMismatch    = errors.New("approval does not match the exact plan")
	ErrStalePlan           = errors.New("reconciliation plan is stale")
	ErrVerificationFailed  = errors.New("fresh verification did not match desired state")
)

type ConsentKind string
type Operation string
type ProjectionActionKind string

const (
	ConsentReversibleLocal        ConsentKind          = "reversible-local"
	ConsentExecutableExternal     ConsentKind          = "executable-external"
	ConsentHostFollowUp           ConsentKind          = "host-follow-up"
	OperationActivate             Operation            = "activate"
	ActionSkillLink               ProjectionActionKind = "skill-link"
	ActionInstructionFile         ProjectionActionKind = "instruction-file"
	ActionOpenCodeSkillLink       ProjectionActionKind = "opencode-skill-link"
	ActionOpenCodeInstructionFile ProjectionActionKind = "opencode-instruction-file"
	ActionOpenCodeConfigReference ProjectionActionKind = "opencode-config-reference"
	ActionCodexMCPConfig          ProjectionActionKind = "codex-mcp-config"
	ActionOpenCodeMCPConfig       ProjectionActionKind = "opencode-mcp-config"
	ActionExternalCommand         ProjectionActionKind = "external-command"
	ActionHostFollowUp            ProjectionActionKind = "host-follow-up"
)

type StalePlanError struct{ Precondition string }

func (e StalePlanError) Error() string { return fmt.Sprintf("%s: %s", ErrStalePlan, e.Precondition) }
func (e StalePlanError) Unwrap() error { return ErrStalePlan }

type ActivationRequest struct {
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
}

type ObservedProjection struct {
	ID                  string
	Exists              bool
	ObservedFingerprint string
	DesiredFingerprint  string
	Action              ProjectionAction
}

type ActivationObservation struct {
	Revision            string
	Projections         []ObservedProjection
	Readiness           ReadinessStatus
	PendingHumanActions []string
}

type ActivationAdapter interface {
	InspectActivation(context.Context, Pack) (ActivationObservation, error)
	ApplyProjections(context.Context, []ProjectionAction) error
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
	PlanID        string   `json:"plan_id"`
	Actions       []string `json:"actions"`
	Completed     []string `json:"completed,omitempty"`
	FailedAction  string   `json:"failed_action,omitempty"`
	FailureDetail string   `json:"failure_detail,omitempty"`
}

type ExternalEffect struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
}

type ActivationState struct {
	SchemaVersion int                   `json:"schema_version"`
	Intent        ActivationIntent      `json:"intent"`
	Journal       *ApplyingJournal      `json:"applying_journal,omitempty"`
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
	observationFingerprint string
	phases                 []PlanPhase
	desired                []projectionExpectation
	portable               []PortableOutcome
	resolutions            []ExecutableResolution
	readiness              ReadinessStatus
	pendingHumanActions    []string
	noOp                   bool
}

type projectionExpectation struct{ ID, Fingerprint string }
type PortableOutcome struct{ Kind, ID string }

func (p ReconciliationPlan) ID() string       { return p.id }
func (p ReconciliationPlan) Digest() string   { return p.digest }
func (p ReconciliationPlan) Pack() Pack       { return clonePack(p.pack) }
func (p ReconciliationPlan) Surface() Surface { return p.surface }
func (p ReconciliationPlan) NoOp() bool       { return p.noOp }
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
	pack, adapter, state, err := f.activationInputs(ctx, request)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	observation, err := adapter.InspectActivation(ctx, pack)
	if err != nil {
		return ReconciliationPlan{}, fmt.Errorf("inspect activation of pack %q on %s: %w", pack.ID, request.Surface, err)
	}

	actions := make([]ProjectionAction, 0, len(observation.Projections))
	for _, projection := range observation.Projections {
		if projection.ID == "" || projection.DesiredFingerprint == "" || projection.Action.ID != projection.ID {
			return ReconciliationPlan{}, fmt.Errorf("inspect activation of pack %q on %s: adapter returned an invalid projection", pack.ID, request.Surface)
		}
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			if projection.Exists && !ownedAtFingerprint(state.Ownership, projection.ID, projection.ObservedFingerprint, pack.ID) {
				return ReconciliationPlan{}, fmt.Errorf("projection %q is unmanaged or drifted; preserving existing %s content", projection.ID, request.Surface)
			}
			actions = append(actions, projection.Action)
		}
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	externalActions, resolutions, err := f.externalPlan(ctx, pack, request.Surface, state)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	noOp := state.Intent.Active && state.Intent.PackID == pack.ID && state.Intent.Surface == request.Surface && state.Intent.Version == pack.Version && ownershipMatches(state.Ownership, observation.Projections, pack.ID) && len(actions) == 0 && len(externalActions) == 0
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
	plan := ReconciliationPlan{pack: pack, operation: OperationActivate, surface: request.Surface, intentRevision: state.Intent.Revision, observationFingerprint: observationDigest(observation), resolutions: resolutions, readiness: readiness, pendingHumanActions: pendingHumanActions, noOp: noOp}
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
		plan.desired = append(plan.desired, projectionExpectation{projection.ID, projection.DesiredFingerprint})
	}
	sort.Slice(plan.desired, func(i, j int) bool { return plan.desired[i].ID < plan.desired[j].ID })
	if len(actions) > 0 {
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentReversibleLocal, ApprovalRequired: true, Actions: append([]ProjectionAction(nil), actions...)})
	}
	if len(externalActions) > 0 {
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentExecutableExternal, ApprovalRequired: true, Actions: append([]ProjectionAction(nil), externalActions...)})
	}
	if len(pendingHumanActions) > 0 {
		hostActions := make([]ProjectionAction, 0, len(pendingHumanActions))
		for i, action := range pendingHumanActions {
			hostActions = append(hostActions, ProjectionAction{ID: fmt.Sprintf("host-follow-up:%s:%d", request.Surface, i), Kind: ActionHostFollowUp, Description: action})
		}
		plan.phases = append(plan.phases, PlanPhase{Kind: ConsentHostFollowUp, Actions: hostActions})
	}
	if !noOp && len(actions) == 0 {
		return ReconciliationPlan{}, fmt.Errorf("existing %s projections are not verified Matty ownership", request.Surface)
	}
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
	if request.Plan.noOp {
		return ApplyResult{Verified: true, PlanID: request.Plan.id, Readiness: request.Plan.readiness, PendingHumanActions: request.Plan.PendingHumanActions()}, nil
	}
	if !request.Interactive {
		return ApplyResult{}, ErrInteractiveRequired
	}
	if !request.Plan.validSeal() {
		return ApplyResult{}, ErrApprovalMismatch
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
	pack, adapter, state, err := f.activationInputs(ctx, ActivationRequest{PackID: request.Plan.pack.ID, Surface: request.Plan.surface})
	if err != nil {
		return ApplyResult{}, err
	}
	observation, err := adapter.InspectActivation(ctx, pack)
	if err != nil {
		return ApplyResult{}, err
	}
	if state.Intent.Revision != request.Plan.intentRevision {
		return ApplyResult{}, StalePlanError{Precondition: fmt.Sprintf("activation intent revision changed from %d to %d; rerun activation to preview a fresh plan", request.Plan.intentRevision, state.Intent.Revision)}
	}
	if observationDigest(observation) != request.Plan.observationFingerprint {
		return ApplyResult{}, StalePlanError{Precondition: fmt.Sprintf("%s projections changed after Preview; rerun activation to preview a fresh plan", request.Plan.surface)}
	}
	if f.activation.resolver != nil || len(request.Plan.resolutions) > 0 {
		resolutions, err := f.resolveExecutables(ctx, pack)
		if err != nil {
			return ApplyResult{}, err
		}
		if !sameResolutions(request.Plan.resolutions, resolutions) {
			return ApplyResult{}, StalePlanError{Precondition: "Engram executable resolution changed after Preview; rerun activation to preview a fresh plan"}
		}
	}
	if hasPhaseActions(request.Plan.phases, ConsentExecutableExternal) && f.activation.executor == nil {
		return ApplyResult{}, fmt.Errorf("external effects are not configured")
	}

	actions := flattenActions(request.Plan.phases)
	state.SchemaVersion = 1
	state.Intent = ActivationIntent{PackID: pack.ID, Surface: request.Plan.surface, Version: pack.Version, Active: true, Revision: state.Intent.Revision + 1}
	state.Journal = &ApplyingJournal{PlanID: request.Plan.id}
	for _, action := range actions {
		if action.Kind != ActionHostFollowUp {
			state.Journal.Actions = append(state.Journal.Actions, action.ID)
		}
	}
	state.Ownership = nil
	if err := f.activation.store.Save(ctx, request.Plan.surface, request.Plan.intentRevision, state); err != nil {
		return ApplyResult{}, err
	}
	localActions := phaseActions(request.Plan.phases, ConsentReversibleLocal)
	if len(localActions) > 0 {
		if err := adapter.ApplyProjections(ctx, localActions); err != nil {
			return ApplyResult{}, err
		}
	}
	verified, err := adapter.InspectActivation(ctx, pack)
	if err != nil {
		return ApplyResult{}, err
	}
	if !verificationMatches(request.Plan.desired, verified.Projections) {
		return ApplyResult{}, ErrVerificationFailed
	}
	externalActions := phaseActions(request.Plan.phases, ConsentExecutableExternal)
	if len(externalActions) > 0 {
		for _, action := range localActions {
			state.Journal.Completed = appendCompleted(state.Journal.Completed, action.ID)
		}
		if err := f.activation.store.Save(ctx, request.Plan.surface, state.Intent.Revision, state); err != nil {
			return ApplyResult{}, fmt.Errorf("persist verified local recovery facts: %w", err)
		}
	}
	for _, action := range externalActions {
		if err := f.activation.executor.Execute(ctx, action); err != nil {
			state.Journal.FailedAction = action.ID
			state.Journal.FailureDetail = err.Error()
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
	if len(externalActions) > 0 {
		verified, err = adapter.InspectActivation(ctx, pack)
		if err != nil {
			return ApplyResult{}, err
		}
		if !verificationMatches(request.Plan.desired, verified.Projections) {
			return ApplyResult{}, ErrVerificationFailed
		}
	}
	state.Journal = nil
	state.Ownership = make([]ProjectionOwnership, 0, len(verified.Projections))
	for _, projection := range verified.Projections {
		state.Ownership = append(state.Ownership, ProjectionOwnership{ID: projection.ID, Contributors: []string{pack.ID}, Fingerprint: projection.DesiredFingerprint})
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
	if pack.ID != "matty" && pack.ID != "engram" {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("activation currently supports only capability packs %q and %q", "matty", "engram")
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
		Observation     string
		Phases          []PlanPhase
		Desired         []projectionExpectation
		Portable        []PortableOutcome
		Resolutions     []ExecutableResolution
		Readiness       ReadinessStatus
		Pending         []string
		NoOp            bool
	}{p.pack.ID, p.pack.Version, p.operation, p.surface, p.intentRevision, p.observationFingerprint, p.phases, p.desired, p.portable, p.resolutions, p.readiness, p.pendingHumanActions, p.noOp}
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
func cloneActivationState(state ActivationState) ActivationState {
	state.Ownership = append([]ProjectionOwnership(nil), state.Ownership...)
	for i := range state.Ownership {
		state.Ownership[i].Contributors = append([]string(nil), state.Ownership[i].Contributors...)
	}
	if state.Journal != nil {
		journal := *state.Journal
		journal.Actions = append([]string(nil), journal.Actions...)
		journal.Completed = append([]string(nil), journal.Completed...)
		state.Journal = &journal
	}
	state.External = append([]ExternalEffect(nil), state.External...)
	return state
}

func (f Facade) externalPlan(ctx context.Context, pack Pack, surface Surface, state ActivationState) ([]ProjectionAction, []ExecutableResolution, error) {
	if pack.ID != "engram" {
		return nil, nil, nil
	}
	resolutions, err := f.resolveExecutables(ctx, pack)
	if err != nil {
		return nil, nil, err
	}
	var actions []ProjectionAction
	for _, resolution := range resolutions {
		if !resolution.Available {
			if !resolution.AcquisitionSupported || strings.TrimSpace(resolution.AcquisitionCommand) == "" {
				return nil, nil, fmt.Errorf("unsatisfied global tool requirement %q: no supported acquisition action is available", resolution.Tool)
			}
			acquisition := ProjectionAction{ID: "external:" + resolution.Tool + ":acquire", Kind: ActionExternalCommand, Command: resolution.AcquisitionCommand, Args: append([]string(nil), resolution.AcquisitionArgs...), Description: fmt.Sprintf("acquire global tool %s via %s %s", resolution.Tool, resolution.AcquisitionCommand, strings.Join(resolution.AcquisitionArgs, " "))}
			if !externalEffectCompleted(state.External, acquisition) {
				actions = append(actions, acquisition)
			}
		}
		if strings.TrimSpace(resolution.Path) == "" {
			return nil, nil, fmt.Errorf("resolved global tool %q has no executable path", resolution.Tool)
		}
		setup := ProjectionAction{ID: "external:" + resolution.Tool + ":setup:" + string(surface), Kind: ActionExternalCommand, Command: resolution.Path, Args: []string{"setup", string(surface)}, Description: fmt.Sprintf("run %s setup %s", resolution.Path, surface)}
		if !externalEffectCompleted(state.External, setup) {
			actions = append(actions, setup)
		}
	}
	return actions, resolutions, nil
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
