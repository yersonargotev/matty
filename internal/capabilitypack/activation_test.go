package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeSurfaceAdapter struct {
	observations []SurfaceInspection
	readiness    []ReadinessObservation
	calls        []surfaceInspectionCall
	applied      [][]ProjectionAction
	inspectCalls int
	actions      []ProjectionAction
	events       *[]string
	applyErr     error
	inspect      func(SurfaceTransition) SurfaceInspection
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

func TestAdapterExecutableProjectionUsesPlanBoundExternalConsent(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceClaude}}
	pending := SurfaceInspection{Revision: "one", Projections: []ObservedProjection{{ID: "mcp_server:memory", ObservedFingerprint: "missing", DesiredFingerprint: "exact", Action: ProjectionAction{ID: "mcp_server:memory", Kind: "claude-user-mcp", Consent: ConsentExecutableExternal}}}}
	verified := pending
	verified.Revision = "two"
	verified.Projections = append([]ObservedProjection(nil), pending.Projections...)
	verified.Projections[0].Exists = true
	verified.Projections[0].ObservedFingerprint = "exact"
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, pending, verified}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceClaude: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceClaude})
	if err != nil {
		t.Fatal(err)
	}
	if phases := plan.Phases(); len(phases) != 1 || phases[0].Kind != ConsentExecutableExternal || len(phases[0].Actions) != 1 {
		t.Fatalf("phases = %#v", phases)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("local consent authorized executable adapter action: %v", err)
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if err != nil || !result.Verified || len(adapter.applied) != 1 || adapter.applied[0][0].ID != "mcp_server:memory" {
		t.Fatalf("result=%#v err=%v applied=%#v", result, err, adapter.applied)
	}
}

func (f *fakeSurfaceAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	f.inspectCalls++
	kind := "desired"
	if transition.Prior.ID != "" {
		kind = "prior-to-desired"
	} else if transition.ResidualOwnership != nil {
		kind = "ownership-residual"
	}
	f.calls = append(f.calls, surfaceInspectionCall{kind: kind, prior: transition.Prior, desired: transition.Desired, ownership: cloneOwnership(transition.ResidualOwnership), resolutions: cloneResolutions(transition.ResolvedExecutables)})
	var result SurfaceInspection
	if f.inspect != nil {
		result = f.inspect(transition)
	} else if len(f.observations) == 0 {
		result = SurfaceInspection{}
	} else if f.inspectCalls > len(f.observations) {
		result = f.observations[len(f.observations)-1]
	} else {
		result = f.observations[f.inspectCalls-1]
	}
	readinessIndex := f.inspectCalls - 1
	if readinessIndex >= len(f.readiness) {
		readinessIndex = len(f.readiness) - 1
	}
	if readinessIndex >= 0 {
		result.Readiness = f.readiness[readinessIndex]
	}
	for i := range result.Projections {
		projection := &result.Projections[i]
		if projection.Action.ID == "" {
			projection.Action.ID = projection.ID
		}
		if projection.Action.Mode == ProjectionDeleteTarget || projection.Action.Mode == ProjectionRemoveContent || ((transition.Prior.ID != "" || transition.ResidualOwnership != nil) && (projection.DesiredFingerprint == "" || projection.DesiredFingerprint == "missing")) {
			projection.Goal = ProjectionAbsent
			projection.DesiredFingerprint = ""
			if projection.Action.Mode == "" {
				projection.Action.Mode = ProjectionDeleteTarget
			}
		} else if projection.Goal == "" {
			projection.Goal = ProjectionPresent
		}
	}
	return result, nil
}

func (f *fakeSurfaceAdapter) ApplyProjections(_ context.Context, actions []ProjectionAction) *ProjectionActionError {
	if f.events != nil {
		*f.events = append(*f.events, "effects")
	}
	f.actions = append(f.actions, actions...)
	f.applied = append(f.applied, append([]ProjectionAction(nil), actions...))
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

func activationFixture(observations ...SurfaceInspection) (Facade, *fakeSurfaceAdapter, *fakeActivationStore) {
	return activationFixtureForSurface(SurfaceCodex, observations...)
}

func activationFixtureForSurface(surface Surface, observations ...SurfaceInspection) (Facade, *fakeSurfaceAdapter, *fakeActivationStore) {
	pack := Pack{ID: "matty", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}, Resources: []Resource{{Kind: "skill", ID: "ask-matt", Source: "/bundle/skills/ask-matt"}, {Kind: "instruction", ID: "matty-guidance", Source: "/bundle/instructions/matty-guidance.md"}}}
	adapter := &fakeSurfaceAdapter{observations: observations}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{surface: adapter}))
	return facade, adapter, store
}

func pendingObservation(fingerprint string) SurfaceInspection {
	return SurfaceInspection{Revision: "host-1", Projections: []ObservedProjection{{ID: "skill:ask-matt", DesiredFingerprint: "skill-new", ObservedFingerprint: fingerprint, Action: ProjectionAction{ID: "skill:ask-matt", Description: "link ask-matt skill"}}, {ID: "instruction:matty-guidance", DesiredFingerprint: "instruction-new", ObservedFingerprint: fingerprint, Action: ProjectionAction{ID: "instruction:matty-guidance", Description: "write Matty guidance"}}}}
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
	otherFacade, _, _ := activationFixture(SurfaceInspection{Revision: "host-other", Projections: pendingObservation("missing").Projections})
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
	facade, adapter, store := activationFixture(pendingObservation("missing"), SurfaceInspection{Revision: "host-2", Projections: pendingObservation("missing").Projections})
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

func TestMattyApplyReportsFreshReadinessWithoutInventingRuntimeUsability(t *testing.T) {
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode} {
		t.Run(string(surface), func(t *testing.T) {
			verified := pendingObservation("missing")
			for i := range verified.Projections {
				verified.Projections[i].ObservedFingerprint = verified.Projections[i].DesiredFingerprint
			}
			verified.Readiness = ReadinessObservation{
				AuthorizationObserved: true,
				Authorized:            true,
				PendingHumanActions:   []string{"reload host and verify the capability in a new runtime session"},
			}
			facade, _, _ := activationFixtureForSurface(surface, pendingObservation("missing"), pendingObservation("missing"), verified)

			plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: surface})
			if err != nil {
				t.Fatal(err)
			}
			result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
			if err != nil {
				t.Fatal(err)
			}
			if !result.Verified || result.Readiness != (ReadinessStatus{Configured: true, Authorized: true}) {
				t.Fatalf("readiness = %+v", result.Readiness)
			}
			if !reflect.DeepEqual(result.PendingHumanActions, []string{"reload host and verify the capability in a new runtime session"}) {
				t.Fatalf("pending actions = %v", result.PendingHumanActions)
			}
		})
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
	obs.Readiness = ReadinessObservation{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true}
	facade, adapter, store := activationFixture(obs)
	store.state = ActivationState{Intent: ActivationIntent{PackID: "matty", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 7}, Ownership: []ProjectionOwnership{{ID: "skill:ask-matt", Contributors: []string{"matty"}, Fingerprint: "skill-new"}, {ID: "instruction:matty-guidance", Contributors: []string{"matty"}, Fingerprint: "instruction-new"}}}

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NoOp() {
		t.Fatalf("plan is not no-op: %+v", plan)
	}
	if plan.Readiness() != (ReadinessStatus{Configured: true, Authorized: true, Usable: true}) || plan.ReadinessObserved() != (ReadinessObservationStatus{Configured: true, Authorization: true, Usability: true}) {
		t.Fatalf("preview expected readiness = %+v observed=%+v", plan.Readiness(), plan.ReadinessObserved())
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
	facade, adapter, store := activationFixtureForSurface(SurfaceOpenCode, pendingObservation("missing"), SurfaceInspection{Revision: "changed", Projections: pendingObservation("missing").Projections})
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
