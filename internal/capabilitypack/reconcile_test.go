package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func reconcileFixture(packs []Pack, state ActivationState, observations ...ActivationObservation) (Facade, *fakeActivationAdapter, *fakeActivationStore) {
	adapter := &fakeActivationAdapter{observations: observations}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: packs}, nil, WithActivation(store, map[Surface]ActivationAdapter{SurfaceCodex: adapter}))
	return facade, adapter, store
}

func activeIntent(packID, version string, revision int) ActivationIntent {
	return ActivationIntent{PackID: packID, Surface: SurfaceCodex, Version: version, Active: true, Revision: revision}
}

func TestTargetedReconcileRepairsActivePackInsideCompleteSurfaceDesiredStateWithoutChangingIntent(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}, {Kind: "instruction", ID: "app", Source: "app"}}},
		{ID: "other", Version: "2", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}, {Kind: "instruction", ID: "other", Source: "other"}}},
	}
	intents := []ActivationIntent{activeIntent("app", "1", 7), activeIntent("other", "2", 3)}
	state := ActivationState{Intent: intents[0], Intents: intents, Ownership: []ProjectionOwnership{
		{ID: "instruction:shared", Contributors: []string{"app", "other"}, Fingerprint: "same"},
		{ID: "instruction:app", Contributors: []string{"app"}, Fingerprint: "old"},
		{ID: "instruction:other", Contributors: []string{"other"}, Fingerprint: "other"},
	}}
	preview := ActivationObservation{Revision: "host", Projections: []ObservedProjection{
		{ID: "instruction:shared", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:shared"}},
		{ID: "instruction:app", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:app", Description: "repair app"}},
		{ID: "instruction:other", Exists: true, ObservedFingerprint: "other", DesiredFingerprint: "other", Action: ProjectionAction{ID: "instruction:other"}},
	}}
	verified := preview
	verified.Projections = append([]ObservedProjection(nil), preview.Projections...)
	verified.Projections[1].ObservedFingerprint = "new"
	facade, adapter, store := reconcileFixture(packs, state, preview, preview, verified)

	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Operation() != OperationReconcile || plan.ReconcileScope() != ReconcileTargeted || !reflect.DeepEqual(plan.Contributors()["instruction:shared"], []string{"app", "other"}) {
		t.Fatalf("plan operation/contributors = %s %+v", plan.Operation(), plan.Contributors())
	}
	if phases := plan.Phases(); len(phases) != 1 || phases[0].Kind != ConsentReversibleLocal || len(phases[0].Actions) != 1 || phases[0].Actions[0].ID != "instruction:app" {
		t.Fatalf("targeted phases = %+v", phases)
	}
	before := cloneActivationState(store.state)
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err != nil || !result.Verified {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !reflect.DeepEqual(store.state.Intent, before.Intent) || !reflect.DeepEqual(store.state.Intents, before.Intents) {
		t.Fatalf("reconcile changed intent: before=%+v after=%+v", before, store.state)
	}
	if len(adapter.actions) != 1 || adapter.actions[0].ID != "instruction:app" {
		t.Fatalf("actions = %+v", adapter.actions)
	}
}

func TestSurfaceWideReconcileUsesOnlyActiveSurfaceIntentsAndAllContributorSets(t *testing.T) {
	packs := []Pack{
		{ID: "one", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
		{ID: "two", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}, {Kind: "instruction", ID: "two", Source: "two"}}},
		{ID: "inactive", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "inactive", Source: "inactive"}}},
	}
	intents := []ActivationIntent{activeIntent("one", "1", 2), activeIntent("two", "1", 4), {PackID: "inactive", Surface: SurfaceCodex, Version: "1", Active: false, Revision: 8}}
	state := ActivationState{Intent: intents[0], Intents: intents}
	obs := ActivationObservation{Revision: "host", Projections: []ObservedProjection{
		{ID: "instruction:shared", ObservedFingerprint: "missing", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:shared"}},
		{ID: "instruction:two", ObservedFingerprint: "missing", DesiredFingerprint: "two", Action: ProjectionAction{ID: "instruction:two"}},
	}}
	facade, _, _ := reconcileFixture(packs, state, obs)
	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Contributors()["instruction:shared"], []string{"one", "two"}) {
		t.Fatalf("contributors = %+v", plan.Contributors())
	}
	if plan.ReconcileScope() != ReconcileSurfaceWide {
		t.Fatalf("scope = %q", plan.ReconcileScope())
	}
	if _, present := plan.Contributors()["instruction:inactive"]; present || len(plan.Phases()) != 1 || len(plan.Phases()[0].Actions) != 2 {
		t.Fatalf("surface-wide plan includes inactive intent: contributors=%+v phases=%+v", plan.Contributors(), plan.Phases())
	}
}

func TestTargetedReconcileDoesNotRepairUnrelatedActivePack(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "app"}}},
		{ID: "other", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "other"}}},
	}
	intents := []ActivationIntent{activeIntent("app", "1", 2), activeIntent("other", "1", 3)}
	state := ActivationState{Intent: intents[0], Intents: intents, Ownership: []ProjectionOwnership{{ID: "instruction:app", Contributors: []string{"app"}, Fingerprint: "same"}, {ID: "instruction:other", Contributors: []string{"other"}, Fingerprint: "old"}}}
	obs := ActivationObservation{Projections: []ObservedProjection{
		{ID: "instruction:app", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:app"}},
		{ID: "instruction:other", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:other"}},
	}}
	facade, _, _ := reconcileFixture(packs, state, obs)
	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() || len(plan.Phases()) != 0 {
		t.Fatalf("targeted unrelated drift: noop=%v phases=%+v err=%v", plan.NoOp(), plan.Phases(), err)
	}
}

func TestTargetedReconcileDoesNotDeleteUnrelatedObsoleteOwnership(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}}, {ID: "other", Version: "1", Surfaces: []Surface{SurfaceCodex}}}
	intents := []ActivationIntent{activeIntent("app", "1", 2), activeIntent("other", "1", 3)}
	state := ActivationState{Intent: intents[0], Intents: intents, Ownership: []ProjectionOwnership{{ID: "instruction:obsolete", Contributors: []string{"other"}, Fingerprint: "owned"}}}
	obs := ActivationObservation{Projections: []ObservedProjection{{ID: "instruction:obsolete", Exists: true, ObservedFingerprint: "owned", DesiredFingerprint: "missing", Action: ProjectionAction{ID: "instruction:obsolete", Mode: ProjectionDeleteTarget}}}}
	facade, _, _ := reconcileFixture(packs, state, obs)
	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() || len(plan.Phases()) != 0 {
		t.Fatalf("unrelated cleanup: noop=%v phases=%+v blockers=%+v pending=%+v err=%v", plan.NoOp(), plan.Phases(), plan.Blockers(), plan.PendingHumanActions(), err)
	}
}

func TestTargetedReconcileRejectsInactivePackWithoutEffects(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: false, Revision: 5}}
	facade, adapter, store := reconcileFixture([]Pack{pack}, state, ActivationObservation{Revision: "host"})
	_, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not active") || adapter.inspectCalls != 0 || len(store.saves) != 0 {
		t.Fatalf("inactive reconcile err=%v inspect=%d saves=%d", err, adapter.inspectCalls, len(store.saves))
	}
}

func TestDriftFreeReconcileIsNoOpWithoutApprovalApplyOrIntentMutation(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
	state := ActivationState{Intent: activeIntent("app", "1", 9), Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "same"}}}
	obs := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}}}
	facade, adapter, store := reconcileFixture([]Pack{pack}, state, obs)
	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() || len(plan.Phases()) != 0 || adapter.inspectCalls != 1 || len(store.saves) != 0 {
		t.Fatalf("noop=%v phases=%+v inspect=%d saves=%d err=%v", plan.NoOp(), plan.Phases(), adapter.inspectCalls, len(store.saves), err)
	}
}

func TestReconcilePreservesAmbiguousOrUnmanagedDriftAsHumanAction(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
	for _, tc := range []struct {
		name      string
		ownership []ProjectionOwnership
	}{
		{name: "ambiguous ownership", ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "owned"}}},
		{name: "unmanaged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := ActivationState{Intent: activeIntent("app", "1", 1), Ownership: tc.ownership}
			obs := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "user-content", DesiredFingerprint: "desired", Action: ProjectionAction{ID: "instruction:guide", Description: "overwrite guide"}}}}
			facade, adapter, store := reconcileFixture([]Pack{pack}, state, obs)
			plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Phases()) != 0 || (len(plan.PendingHumanActions()) == 0 && len(plan.Blockers()) == 0) {
				t.Fatalf("unsafe drift was actionable: phases=%+v pending=%v blockers=%+v", plan.Phases(), plan.PendingHumanActions(), plan.Blockers())
			}
			if len(adapter.actions) != 0 || len(store.saves) != 0 {
				t.Fatal("preserved drift caused effects")
			}
		})
	}
}

func TestReconcileDeletesObsoleteProjectionOnlyWithVerifiedOwnershipAndDestructiveApproval(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}}
	state := ActivationState{Intent: activeIntent("app", "1", 3), Ownership: []ProjectionOwnership{{ID: "instruction:obsolete", Contributors: []string{"app"}, Fingerprint: "owned"}}}
	owned := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{
		ID: "instruction:obsolete", Exists: true, ObservedFingerprint: "owned", DesiredFingerprint: "missing",
		Action: ProjectionAction{ID: "instruction:obsolete", Description: "delete obsolete instruction", Mode: ProjectionDeleteTarget},
	}}}
	deleted := ActivationObservation{Revision: "host-2", Projections: []ObservedProjection{{ID: "instruction:obsolete", Exists: false, DesiredFingerprint: "missing"}}}
	facade, adapter, store := reconcileFixture([]Pack{pack}, state, owned, owned, deleted)
	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if phases := plan.Phases(); len(phases) != 1 || phases[0].Kind != ConsentDestructiveCleanup || len(phases[0].Actions) != 1 {
		t.Fatalf("destructive phases = %+v", phases)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrApprovalMismatch) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("local approval authorized cleanup: err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}

func TestReconcileApprovalKindsCannotAuthorizeOtherPhases(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{missingEngramResolution()}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"))
	store.state = ActivationState{Intent: activeIntent("engram", "1.0.0", 6)}
	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if phases := plan.Phases(); len(phases) < 2 || phases[0].Kind != ConsentReversibleLocal || phases[1].Kind != ConsentExecutableExternal {
		t.Fatalf("phases = %+v", phases)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrApprovalMismatch) || len(store.saves) != 0 || len(adapter.actions) != 0 || len(executor.actions) != 0 {
		t.Fatalf("local approval authorized other phase: err=%v saves=%d local=%d external=%d", err, len(store.saves), len(adapter.actions), len(executor.actions))
	}
}

func TestReconcileRejectsRepresentativeStaleFactsWithZeroEffectsAndUnchangedIntent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Facade, *fakeActivationStore)
		obs    []ActivationObservation
		want   string
	}{
		{name: "intent revision", mutate: func(_ *Facade, store *fakeActivationStore) { store.state.Intent.Revision++ }, want: "intent"},
		{name: "ownership", mutate: func(_ *Facade, store *fakeActivationStore) { store.state.Ownership[0].Fingerprint = "changed" }, want: "ownership"},
		{name: "catalog manifest", mutate: func(facade *Facade, _ *fakeActivationStore) { facade.catalog.packs[0].Resources[0].Source = "changed" }, want: "catalog"},
		{name: "host observation", obs: []ActivationObservation{{Revision: "host-2", Projections: []ObservedProjection{{ID: "instruction:guide", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}}}}}, want: "projections"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
			state := ActivationState{Intent: activeIntent("app", "1", 4), Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
			preview := ActivationObservation{Revision: "host-1", Projections: []ObservedProjection{{ID: "instruction:guide", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}}}}
			facade, adapter, store := reconcileFixture([]Pack{pack}, state, append([]ActivationObservation{preview}, tc.obs...)...)
			plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			before := cloneActivationState(store.state)
			if tc.mutate != nil {
				tc.mutate(&facade, store)
				before = cloneActivationState(store.state)
			}
			_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
			var stale StalePlanError
			if !errors.As(err, &stale) || !strings.Contains(strings.ToLower(stale.Precondition), tc.want) {
				t.Fatalf("stale error = %v, want precondition containing %q", err, tc.want)
			}
			if len(store.saves) != 0 || len(adapter.actions) != 0 || !reflect.DeepEqual(store.state.Intent, before.Intent) || !reflect.DeepEqual(store.state.Ownership, before.Ownership) {
				t.Fatalf("stale reconcile caused effects: state=%+v saves=%d actions=%d", store.state, len(store.saves), len(adapter.actions))
			}
		})
	}
}
