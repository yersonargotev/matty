package capabilitypack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	OperationActivate             Operation            = "activate"
	ActionSkillLink               ProjectionActionKind = "skill-link"
	ActionInstructionFile         ProjectionActionKind = "instruction-file"
	ActionOpenCodeSkillLink       ProjectionActionKind = "opencode-skill-link"
	ActionOpenCodeInstructionFile ProjectionActionKind = "opencode-instruction-file"
	ActionOpenCodeConfigReference ProjectionActionKind = "opencode-config-reference"
)

type StalePlanError struct{ Precondition string }

func (e StalePlanError) Error() string { return fmt.Sprintf("%s: %s", ErrStalePlan, e.Precondition) }
func (e StalePlanError) Unwrap() error { return ErrStalePlan }

type ActivationRequest struct {
	PackID  string
	Surface Surface
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
}

type ObservedProjection struct {
	ID                  string
	Exists              bool
	ObservedFingerprint string
	DesiredFingerprint  string
	Action              ProjectionAction
}

type ActivationObservation struct {
	Revision    string
	Projections []ObservedProjection
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
	PlanID  string   `json:"plan_id"`
	Actions []string `json:"actions"`
}

type ActivationState struct {
	SchemaVersion int                   `json:"schema_version"`
	Intent        ActivationIntent      `json:"intent"`
	Journal       *ApplyingJournal      `json:"applying_journal,omitempty"`
	Ownership     []ProjectionOwnership `json:"ownership,omitempty"`
}

type ActivationStore interface {
	Load(context.Context, Surface) (ActivationState, error)
	Save(context.Context, Surface, int, ActivationState) error
}

type activationDependencies struct {
	store    ActivationStore
	adapters map[Surface]ActivationAdapter
}

type FacadeOption func(*Facade)

func WithActivation(store ActivationStore, adapters map[Surface]ActivationAdapter) FacadeOption {
	return func(f *Facade) { f.activation = &activationDependencies{store: store, adapters: adapters} }
}

type PlanPhase struct {
	Kind    ConsentKind
	Digest  string
	Actions []ProjectionAction
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
	}
	return result
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
	Verified    bool
	PlanID      string
	Projections int
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
			if projection.Exists && !ownedAtFingerprint(state.Ownership, projection.ID, projection.ObservedFingerprint) {
				return ReconciliationPlan{}, fmt.Errorf("projection %q is unmanaged or drifted; preserving existing %s content", projection.ID, request.Surface)
			}
			actions = append(actions, projection.Action)
		}
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	noOp := state.Intent.Active && state.Intent.PackID == pack.ID && state.Intent.Surface == request.Surface && state.Intent.Version == pack.Version && ownershipMatches(state.Ownership, observation.Projections) && len(actions) == 0
	plan := ReconciliationPlan{pack: pack, operation: OperationActivate, surface: request.Surface, intentRevision: state.Intent.Revision, observationFingerprint: observationDigest(observation), noOp: noOp}
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
		plan.phases = []PlanPhase{{Kind: ConsentReversibleLocal, Actions: append([]ProjectionAction(nil), actions...)}}
	}
	if !noOp && len(actions) == 0 {
		return ReconciliationPlan{}, fmt.Errorf("existing %s projections are not verified Matty ownership", request.Surface)
	}
	plan.seal()
	return plan, nil
}

func (f Facade) Approve(plan ReconciliationPlan, kind ConsentKind) ApprovalReceipt {
	for _, phase := range plan.phases {
		if phase.Kind == kind {
			return ApprovalReceipt{planDigest: plan.digest, phaseDigest: phase.Digest, kind: kind}
		}
	}
	return ApprovalReceipt{}
}

func (f Facade) Apply(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	if request.Plan.noOp {
		return ApplyResult{Verified: true, PlanID: request.Plan.id}, nil
	}
	if !request.Interactive {
		return ApplyResult{}, ErrInteractiveRequired
	}
	if !request.Plan.validSeal() {
		return ApplyResult{}, ErrApprovalMismatch
	}
	for _, phase := range request.Plan.phases {
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

	actions := flattenActions(request.Plan.phases)
	state.SchemaVersion = 1
	state.Intent = ActivationIntent{PackID: pack.ID, Surface: request.Plan.surface, Version: pack.Version, Active: true, Revision: state.Intent.Revision + 1}
	state.Journal = &ApplyingJournal{PlanID: request.Plan.id}
	for _, action := range actions {
		state.Journal.Actions = append(state.Journal.Actions, action.ID)
	}
	state.Ownership = nil
	if err := f.activation.store.Save(ctx, request.Plan.surface, request.Plan.intentRevision, state); err != nil {
		return ApplyResult{}, err
	}
	if err := adapter.ApplyProjections(ctx, actions); err != nil {
		return ApplyResult{}, err
	}
	verified, err := adapter.InspectActivation(ctx, pack)
	if err != nil {
		return ApplyResult{}, err
	}
	if !verificationMatches(request.Plan.desired, verified.Projections) {
		return ApplyResult{}, ErrVerificationFailed
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
	return ApplyResult{Verified: true, PlanID: request.Plan.id, Projections: len(state.Ownership)}, nil
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
	if pack.ID != "matty" {
		return Pack{}, nil, ActivationState{}, fmt.Errorf("activation currently supports only capability pack %q", "matty")
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
			Kind    ConsentKind
			Actions []ProjectionAction
		}{p.phases[i].Kind, p.phases[i].Actions})
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
		NoOp            bool
	}{p.pack.ID, p.pack.Version, p.operation, p.surface, p.intentRevision, p.observationFingerprint, p.phases, p.desired, p.portable, p.noOp}
}
func digestJSON(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func observationDigest(o ActivationObservation) string {
	normalized := ActivationObservation{Revision: o.Revision, Projections: append([]ObservedProjection(nil), o.Projections...)}
	sort.Slice(normalized.Projections, func(i, j int) bool { return normalized.Projections[i].ID < normalized.Projections[j].ID })
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
func ownershipMatches(owners []ProjectionOwnership, projections []ObservedProjection) bool {
	if len(owners) != len(projections) {
		return false
	}
	byID := map[string]ProjectionOwnership{}
	for _, owner := range owners {
		byID[owner.ID] = owner
	}
	for _, projection := range projections {
		owner, ok := byID[projection.ID]
		if !ok || owner.Fingerprint != projection.DesiredFingerprint || len(owner.Contributors) != 1 || owner.Contributors[0] != "matty" {
			return false
		}
	}
	return true
}
func ownedAtFingerprint(owners []ProjectionOwnership, id, fingerprint string) bool {
	for _, owner := range owners {
		if owner.ID == id && owner.Fingerprint == fingerprint && len(owner.Contributors) == 1 && owner.Contributors[0] == "matty" {
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
		state.Journal = &journal
	}
	return state
}
