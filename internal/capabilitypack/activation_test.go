package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeActivationAdapter struct {
	observations []ActivationObservation
	inspectCalls int
	actions      []ProjectionAction
	events       *[]string
	applyErr     error
}

func TestPlanDispositionDefinesLifecycleSemantics(t *testing.T) {
	action := PlanPhase{Kind: ConsentReversibleLocal, ApprovalRequired: true, Actions: []ProjectionAction{{ID: "instruction:guide"}}}
	blocker := PlanBlocker{Kind: BlockerOwnership, Subject: "instruction:guide"}
	for _, tc := range []struct {
		name string
		plan ReconciliationPlan
		want PlanDisposition
	}{
		{name: "legitimately converged no-op", plan: ReconciliationPlan{noOp: true}, want: PlanConverged},
		{name: "applicable actions", plan: ReconciliationPlan{phases: []PlanPhase{action}}, want: PlanApplicable},
		{name: "mixed actions and protected content", plan: ReconciliationPlan{phases: []PlanPhase{action}, blockers: []PlanBlocker{blocker}}, want: PlanMixed},
		{name: "fully blocked", plan: ReconciliationPlan{blockers: []PlanBlocker{blocker}}, want: PlanBlocked},
		{name: "pending human actions only", plan: ReconciliationPlan{pendingHumanActions: []string{"authenticate"}}, want: PlanPendingHuman},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.plan.Disposition(); got != tc.want {
				t.Fatalf("disposition=%s want=%s", got, tc.want)
			}
		})
	}
}

func (f *fakeActivationAdapter) InspectActivation(context.Context, Pack) (ActivationObservation, error) {
	f.inspectCalls++
	if f.inspectCalls > len(f.observations) {
		return f.observations[len(f.observations)-1], nil
	}
	return f.observations[f.inspectCalls-1], nil
}

func (f *fakeActivationAdapter) ApplyProjections(_ context.Context, actions []ProjectionAction) *ProjectionActionError {
	if f.events != nil {
		*f.events = append(*f.events, "effects")
	}
	f.actions = append(f.actions, actions...)
	if f.applyErr == nil {
		return nil
	}
	var actionErr ProjectionActionError
	if errors.As(f.applyErr, &actionErr) {
		return &actionErr
	}
	return &ProjectionActionError{ID: actions[0].ID, Err: f.applyErr}
}

type fakeActivationStore struct {
	state  ActivationState
	events *[]string
	saves  []ActivationState
}

func (f *fakeActivationStore) Load(context.Context, Surface) (ActivationState, error) {
	return cloneActivationState(f.state), nil
}
func (f *fakeActivationStore) Save(_ context.Context, _ Surface, expectedRevision int, state ActivationState) error {
	if f.state.Intent.Revision != expectedRevision {
		return ErrStalePlan
	}
	if f.events != nil {
		*f.events = append(*f.events, "persist")
	}
	f.state = cloneActivationState(state)
	f.saves = append(f.saves, cloneActivationState(state))
	return nil
}

func activationFixture(observations ...ActivationObservation) (Facade, *fakeActivationAdapter, *fakeActivationStore) {
	return activationFixtureForSurface(SurfaceCodex, observations...)
}

func activationFixtureForSurface(surface Surface, observations ...ActivationObservation) (Facade, *fakeActivationAdapter, *fakeActivationStore) {
	pack := Pack{ID: "matty", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}, Resources: []Resource{{Kind: "skill", ID: "ask-matt", Source: "/bundle/skills/ask-matt"}, {Kind: "instruction", ID: "matty-guidance", Source: "/bundle/instructions/matty-guidance.md"}}}
	adapter := &fakeActivationAdapter{observations: observations}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, map[Surface]SurfaceInspector{surface: fakeSurfaceInspectorPtr()}, WithActivation(store, map[Surface]ActivationAdapter{surface: adapter}))
	return facade, adapter, store
}

func fakeSurfaceInspectorPtr() SurfaceInspector { return &fakeSurfaceInspector{} }

func pendingObservation(fingerprint string) ActivationObservation {
	return ActivationObservation{Revision: "host-1", Projections: []ObservedProjection{{ID: "skill:ask-matt", DesiredFingerprint: "skill-new", ObservedFingerprint: fingerprint, Action: ProjectionAction{ID: "skill:ask-matt", Description: "link ask-matt skill"}}, {ID: "instruction:matty-guidance", DesiredFingerprint: "instruction-new", ObservedFingerprint: fingerprint, Action: ProjectionAction{ID: "instruction:matty-guidance", Description: "write Matty guidance"}}}}
}

func TestActivationPreviewIsPureAndProducesStableImmutablePlan(t *testing.T) {
	facade, adapter, store := activationFixture(pendingObservation("missing"))

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.NoOp() || plan.ID() == "" || plan.Digest() == "" {
		t.Fatalf("invalid plan: %+v", plan)
	}
	if got := plan.Phases(); len(got) != 1 || got[0].Kind != ConsentReversibleLocal || len(got[0].Actions) != 2 {
		t.Fatalf("phases = %+v", got)
	}
	if adapter.inspectCalls != 1 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("preview side effects: inspect=%d actions=%v saves=%v", adapter.inspectCalls, adapter.actions, store.saves)
	}
	mutated := plan.Phases()
	mutated[0].Actions[0].Description = "tampered"
	if plan.Phases()[0].Actions[0].Description == "tampered" {
		t.Fatal("plan exposed mutable action storage")
	}
}

func TestApplyRejectsNonInteractiveBeforeStateOrEffects(t *testing.T) {
	facade, adapter, store := activationFixture(pendingObservation("missing"))
	plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	receipt := facade.Approve(plan, ConsentReversibleLocal)

	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{receipt}, Interactive: false})
	if !errors.Is(err, ErrInteractiveRequired) {
		t.Fatalf("error = %v", err)
	}
	if adapter.inspectCalls != 1 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("non-interactive caused effects: %+v %+v", adapter.actions, store.saves)
	}
}

func TestApprovalIsBoundToExactPlan(t *testing.T) {
	facade, adapter, store := activationFixture(pendingObservation("missing"), pendingObservation("missing"))
	plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	otherFacade, _, _ := activationFixture(ActivationObservation{Revision: "host-other", Projections: pendingObservation("missing").Projections})
	other, _ := otherFacade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})

	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{otherFacade.Approve(other, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("error = %v", err)
	}
	if adapter.inspectCalls != 1 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("mismatched approval caused effects")
	}
}

func TestStalePlanExecutesZeroActions(t *testing.T) {
	facade, adapter, store := activationFixture(pendingObservation("missing"), ActivationObservation{Revision: "host-2", Projections: pendingObservation("missing").Projections})
	plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})

	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) {
		t.Fatalf("error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("stale plan caused effects")
	}
}

func TestApplyPersistsIntentAndJournalBeforeEffectsThenRecordsVerifiedOwnership(t *testing.T) {
	events := []string{}
	verified := pendingObservation("missing")
	verified.Revision = "host-2"
	for i := range verified.Projections {
		verified.Projections[i].ObservedFingerprint = verified.Projections[i].DesiredFingerprint
	}
	facade, adapter, store := activationFixture(pendingObservation("missing"), pendingObservation("missing"), verified)
	adapter.events, store.events = &events, &events
	plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})

	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || !reflect.DeepEqual(events[:2], []string{"persist", "effects"}) {
		t.Fatalf("result/events = %+v %v", result, events)
	}
	if len(store.saves) != 2 || store.saves[0].Journal == nil || store.saves[0].Intent.Revision != 1 || len(store.saves[0].Ownership) != 0 {
		t.Fatalf("pre-effect state = %+v", store.saves[0])
	}
	if store.saves[1].Journal != nil || len(store.saves[1].Ownership) != 2 {
		t.Fatalf("verified state = %+v", store.saves[1])
	}
	for _, owner := range store.saves[1].Ownership {
		if !reflect.DeepEqual(owner.Contributors, []string{"matty"}) || owner.Fingerprint == "" {
			t.Fatalf("ownership = %+v", owner)
		}
	}
}

func TestVerificationFailureDoesNotInventOwnership(t *testing.T) {
	facade, _, store := activationFixture(pendingObservation("missing"), pendingObservation("missing"), pendingObservation("missing"))
	plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})

	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("error = %v", err)
	}
	if len(store.state.Ownership) != 0 || store.state.Journal == nil {
		t.Fatalf("failure invented ownership or cleared journal: %+v", store.state)
	}
}

func TestAlreadyConvergedActivationIsNoOpWithoutApprovalOrApply(t *testing.T) {
	obs := pendingObservation("missing")
	for i := range obs.Projections {
		obs.Projections[i].ObservedFingerprint = obs.Projections[i].DesiredFingerprint
	}
	facade, adapter, store := activationFixture(obs)
	store.state = ActivationState{Intent: ActivationIntent{PackID: "matty", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 7}, Ownership: []ProjectionOwnership{{ID: "skill:ask-matt", Contributors: []string{"matty"}, Fingerprint: "skill-new"}, {ID: "instruction:matty-guidance", Contributors: []string{"matty"}, Fingerprint: "instruction-new"}}}

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NoOp() {
		t.Fatalf("plan is not no-op: %+v", plan)
	}
	if adapter.inspectCalls != 1 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("no-op caused apply/save")
	}
}

func TestOpenCodeActivationUsesExactApprovedPlanAndRecordsOwnershipAfterVerification(t *testing.T) {
	events := []string{}
	verified := pendingObservation("missing")
	verified.Revision = "host-2"
	for i := range verified.Projections {
		verified.Projections[i].ObservedFingerprint = verified.Projections[i].DesiredFingerprint
	}
	facade, adapter, store := activationFixtureForSurface(SurfaceOpenCode, pendingObservation("missing"), pendingObservation("missing"), verified)
	adapter.events, store.events = &events, &events
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Surface() != SurfaceOpenCode {
		t.Fatalf("surface = %s", plan.Surface())
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || !reflect.DeepEqual(events[:2], []string{"persist", "effects"}) {
		t.Fatalf("result/events = %+v %v", result, events)
	}
	if store.saves[0].Intent.Surface != SurfaceOpenCode || store.saves[0].Journal == nil || len(store.saves[0].Ownership) != 0 {
		t.Fatalf("pre-effect state = %+v", store.saves[0])
	}
	if store.saves[1].Journal != nil || len(store.saves[1].Ownership) != 2 {
		t.Fatalf("verified state = %+v", store.saves[1])
	}
}

func TestApprovalForCodexPlanCannotApproveOpenCodePlan(t *testing.T) {
	codexFacade, _, _ := activationFixtureForSurface(SurfaceCodex, pendingObservation("missing"))
	openCodeFacade, adapter, store := activationFixtureForSurface(SurfaceOpenCode, pendingObservation("missing"))
	codexPlan, _ := codexFacade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	openCodePlan, _ := openCodeFacade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceOpenCode})

	_, err := openCodeFacade.Apply(context.Background(), ApplyRequest{Plan: openCodePlan, Approvals: []ApprovalReceipt{codexFacade.Approve(codexPlan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("error = %v", err)
	}
	if adapter.inspectCalls != 1 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("cross-surface approval caused effects")
	}
}

func TestOpenCodeStalePlanExecutesZeroActions(t *testing.T) {
	facade, adapter, store := activationFixtureForSurface(SurfaceOpenCode, pendingObservation("missing"), ActivationObservation{Revision: "changed", Projections: pendingObservation("missing").Projections})
	plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceOpenCode})
	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) {
		t.Fatalf("error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("stale OpenCode plan caused effects")
	}
}

func TestSurfacesSharePortableOutcomesWhileKeepingDistinctPlanIdentity(t *testing.T) {
	codexFacade, _, _ := activationFixtureForSurface(SurfaceCodex, pendingObservation("missing"))
	openCodeFacade, _, _ := activationFixtureForSurface(SurfaceOpenCode, pendingObservation("missing"))
	codexPlan, err := codexFacade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	openCodePlan, err := openCodeFacade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(codexPlan.PortableOutcomes(), openCodePlan.PortableOutcomes()) {
		t.Fatalf("portable outcomes differ: codex=%v opencode=%v", codexPlan.PortableOutcomes(), openCodePlan.PortableOutcomes())
	}
	if codexPlan.Digest() == openCodePlan.Digest() {
		t.Fatal("host-specific surfaces did not produce distinct sealed plans")
	}
}
