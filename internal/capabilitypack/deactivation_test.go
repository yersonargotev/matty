package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func deactivationFixture(packs []Pack, state ActivationState, observations ...SurfaceInspection) (Facade, *fakeSurfaceAdapter, *fakeActivationStore) {
	adapter := &fakeSurfaceAdapter{observations: observations}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	return facade, adapter, store
}

func deletionObservation(revision, observed string, exists bool) SurfaceInspection {
	return SurfaceInspection{Revision: revision, Projections: []ObservedProjection{{
		ID:                  "instruction:guide",
		Exists:              exists,
		ObservedFingerprint: observed,
		Action:              ProjectionAction{ID: "instruction:guide", Description: "delete guide"},
	}}}
}

func TestDeactivatePersistsInactiveIntentBeforeVerifiedLastContributorDeletion(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "verified"}}}
	deleted := deletionObservation("host-2", "", false)
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host-1", "verified", true), deletionObservation("host-1", "verified", true), deleted)
	events := []string{}
	adapter.events, store.events = &events, &events

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Operation() != OperationDeactivate || plan.OldVersion() != "1.0.0" || plan.IntentRevision() != 4 {
		t.Fatalf("deactivation facts = operation %s version %q revision %d", plan.Operation(), plan.OldVersion(), plan.IntentRevision())
	}
	if phases := plan.Phases(); len(phases) != 1 || phases[0].Kind != ConsentDestructiveCleanup || len(phases[0].Actions) != 1 {
		t.Fatalf("phases = %+v", phases)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("local approval authorized deletion: %v", err)
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || !reflect.DeepEqual(events[:2], []string{"persist", "effects"}) {
		t.Fatalf("result/events = %+v %v", result, events)
	}
	if store.saves[0].Intent.Active || store.saves[0].Journal == nil || store.state.Journal != nil || len(store.state.Ownership) != 0 {
		t.Fatalf("deactivation state = first %+v final %+v", store.saves[0], store.state)
	}
}

func TestDeactivateRejectsActiveDependentWithoutCascade(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:app"}},
		{ID: "dependent", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"cap:app"}}},
	}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}, Intents: []ActivationIntent{
		{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2},
		{PackID: "dependent", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3},
	}}
	facade, adapter, store := deactivationFixture(packs, state, SurfaceInspection{Revision: "host"})

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) != 1 || len(plan.Phases()) != 0 {
		t.Fatalf("blocked plan = applicable %v blockers %+v phases %+v", plan.Applicable(), plan.Blockers(), plan.Phases())
	}
	detail := strings.ToLower(plan.Blockers()[0].Detail)
	for _, want := range []string{"app", "dependent", "cap:app", "cascade"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("blocker detail %q does not mention %q", detail, want)
		}
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("blocked deactivation caused effects")
	}
}

func TestDeactivateRetainsSharedProjectionAndResultingContributorsWithoutRewrite(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
		{ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
	}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 5}, Intents: []ActivationIntent{
		{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 5},
		{PackID: "other", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1},
	}, Ownership: []ProjectionOwnership{{ID: "instruction:shared", Contributors: []string{"app", "other"}, Fingerprint: "same"}}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:shared"}}}}
	facade, adapter, _ := deactivationFixture(packs, state, observation)

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if retained := plan.RetainedProjections(); len(retained) != 1 || retained[0].ID != "instruction:shared" || !reflect.DeepEqual(retained[0].Contributors, []string{"other"}) {
		t.Fatalf("retained = %+v", retained)
	}
	if got := plan.Contributors()["instruction:shared"]; !reflect.DeepEqual(got, []string{"other"}) {
		t.Fatalf("result contributors = %v", got)
	}
	if len(plan.Phases()) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("shared projection was rewritten: phases=%+v actions=%+v", plan.Phases(), adapter.actions)
	}
}

func TestDeactivatePreservesAndBlocksDriftedSharedProjection(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}}, {ID: "other", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 3}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: true}, {PackID: "other", Surface: SurfaceCodex, Version: "1", Active: true}}, Ownership: []ProjectionOwnership{{ID: "instruction:shared", Contributors: []string{"app", "other"}, Fingerprint: "verified"}}}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "user-drift", DesiredFingerprint: "verified", Action: ProjectionAction{ID: "instruction:shared"}}}}
	facade, adapter, store := deactivationFixture(packs, state, obs)
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.PendingHumanActions()) == 0 || len(plan.Phases()) != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("plan=%+v pending=%v", plan.Blockers(), plan.PendingHumanActions())
	}
}

func TestDeactivatePreservesDriftedAndUnmanagedLastContributorTargets(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	for _, tc := range []struct {
		name      string
		ownership []ProjectionOwnership
	}{
		{name: "drifted", ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "verified"}}},
		{name: "unmanaged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1}, Ownership: tc.ownership}
			facade, adapter, _ := deactivationFixture([]Pack{pack}, state, deletionObservation("host", "user-content", true))
			plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Phases()) != 0 || len(plan.PendingHumanActions()) == 0 || len(adapter.actions) != 0 {
				t.Fatalf("unsafe target was not preserved: phases=%+v pending=%v actions=%v", plan.Phases(), plan.PendingHumanActions(), adapter.actions)
			}
		})
	}
}

func TestDeactivateRejectsStaleHostFactWithZeroEffects(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "verified"}}}
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host-1", "verified", true), deletionObservation("host-2", "verified", true))
	plan, _ := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})

	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("stale deactivation err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}

func TestDeactivateRejectsChangedIntentOwnershipCatalogAndDependentsWithZeroEffects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Facade, *fakeActivationStore)
	}{
		{"intent", func(_ *Facade, s *fakeActivationStore) { s.state.Intent.Revision++ }},
		{"ownership", func(_ *Facade, s *fakeActivationStore) { s.state.Ownership[0].Fingerprint = "changed" }},
		{"catalog", func(f *Facade, _ *fakeActivationStore) { f.catalog.packs[0].Version = "2" }},
		{"active-dependents", func(f *Facade, s *fakeActivationStore) {
			f.catalog.packs = append(f.catalog.packs, Pack{ID: "dependent", Version: "1", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"cap:app"}}})
			s.state.Intents = append(s.state.Intents, ActivationIntent{PackID: "dependent", Surface: SurfaceCodex, Version: "1", Active: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:app"}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
			state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 2}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "verified"}}}
			obs := deletionObservation("host", "verified", true)
			facade, adapter, store := deactivationFixture([]Pack{pack}, state, obs, obs)
			plan, _ := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
			tc.mutate(&facade, store)
			_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
			if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
				t.Fatalf("err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
			}
		})
	}
}

func TestDeactivateAlreadyInactiveConvergedIsNoOp(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 8}}
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, SurfaceInspection{Revision: "host"})

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() || len(plan.Phases()) != 0 {
		t.Fatalf("inactive plan noop=%v phases=%+v err=%v", plan.NoOp(), plan.Phases(), err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("inactive no-op caused effects")
	}
}

func TestDeactivateInactiveConvergedPackIsNoOpWithUnrelatedSurfaceOwnership(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}}, {ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "other"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 8}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 8}, {PackID: "other", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}, Ownership: []ProjectionOwnership{{ID: "instruction:other", Contributors: []string{"other"}, Fingerprint: "same"}}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:other", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:other"}}}}
	facade, _, store := deactivationFixture(packs, state, observation)
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() || len(store.saves) != 0 {
		t.Fatalf("plan noop=%v saves=%d err=%v", plan.NoOp(), len(store.saves), err)
	}
}

func TestDeactivateInactivePartialStateIsReportOnlyWithoutApplyOrEffects(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: false, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "verified"}}}
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host", "user-drift", true))
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || plan.NoOp() || len(plan.PendingHumanActions()) == 0 || len(plan.Phases()) != 0 {
		t.Fatalf("applicable=%v noop=%v blockers=%v pending=%v", plan.Applicable(), plan.NoOp(), plan.Blockers(), plan.PendingHumanActions())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err == nil {
		t.Fatal("blocked inactive partial state applied")
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("inactive partial preview/apply caused effects")
	}
}
