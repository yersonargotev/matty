package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func updateFixture(packs []Pack, state ActivationState, observations ...ActivationObservation) (Facade, *fakeActivationAdapter, *fakeActivationStore) {
	adapter := &fakeActivationAdapter{observations: observations}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: packs}, nil, WithActivation(store, map[Surface]ActivationAdapter{SurfaceCodex: adapter}))
	return facade, adapter, store
}

func TestUpdatePlansCatalogCurrentAndPersistsTargetBeforeEffects(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	pending := ActivationObservation{Revision: "host-1", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide", Description: "write v2"}}}}
	verified := pending
	verified.Revision = "host-2"
	verified.Projections = append([]ObservedProjection(nil), pending.Projections...)
	verified.Projections[0].ObservedFingerprint = "new"
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, pending, pending, verified)
	events := []string{}
	adapter.events, store.events = &events, &events

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Operation() != OperationUpdate || plan.OldVersion() != "1.0.0" || plan.Pack().Version != "2.0.0" || plan.IntentRevision() != 4 {
		t.Fatalf("update facts = operation %s, %s -> %s, revision %d", plan.Operation(), plan.OldVersion(), plan.Pack().Version, plan.IntentRevision())
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events[:2], []string{"persist", "effects"}) || store.saves[0].Intent.Version != "2.0.0" || store.saves[0].Journal == nil {
		t.Fatalf("ordering/state = %v %+v", events, store.saves[0])
	}
	if store.state.Journal != nil || store.state.Ownership[0].Fingerprint != "new" {
		t.Fatalf("final state = %+v", store.state)
	}
}

func TestUpdateIncludesNewDependencyAndRetainsUnchangedSharedProjection(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"dep"}}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
		{ID: "dep", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"dep"}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
	}
	obs := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:shared"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}, Ownership: []ProjectionOwnership{{ID: "instruction:shared", Contributors: []string{"app"}, Fingerprint: "same"}}}
	facade, adapter, _ := updateFixture(packs, state, obs)

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	activations := plan.Activations()
	if len(activations) != 2 || activations[1].Pack.ID != "dep" || activations[1].Role != ActivationRequired {
		t.Fatalf("activations = %+v", activations)
	}
	retained := plan.RetainedProjections()
	if len(retained) != 1 || !reflect.DeepEqual(retained[0].Contributors, []string{"app", "dep"}) || len(plan.Phases()) != 0 || plan.NoOp() {
		t.Fatalf("retained/plan = %+v phases=%+v noop=%v", retained, plan.Phases(), plan.NoOp())
	}
	if len(adapter.actions) != 0 {
		t.Fatal("unchanged shared projection was rewritten")
	}
}

func TestCatalogCurrentUpdateIsNoOpOnlyWhenConverged(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	converged := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "2.0.0", Active: true, Revision: 7}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "same"}}}
	facade, _, store := updateFixture([]Pack{pack}, state, converged)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() {
		t.Fatalf("plan noop=%v err=%v", plan.NoOp(), err)
	}
	if len(store.saves) != 0 {
		t.Fatal("no-op persisted state")
	}
}

func TestUpdateRejectsStaleCatalogAndExactPlanApproval(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	obs := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, obs)
	plan, _ := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	facade.catalog.packs[0].Version = "3.0.0"

	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("stale update err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}

func TestUpdateBlocksIncompatibleNewContribution(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "new"}}}, {ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "other"}}}}
	obs := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:shared"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true}, {PackID: "other", Surface: SurfaceCodex, Version: "1.0.0", Active: true}}}
	facade, adapter, store := updateFixture(packs, state, obs)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) == 0 || len(plan.Phases()) != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("blocked plan = applicable %v blockers %+v", plan.Applicable(), plan.Blockers())
	}
}

func TestUpdateBlocksCapabilityConflictIntroducedByCatalogCurrent(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Conflicts: []string{"cap:other"}},
		{ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:other"}},
	}
	obs := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "combined", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "combined"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true}, {PackID: "other", Surface: SurfaceCodex, Version: "1.0.0", Active: true}}}
	facade, _, _ := updateFixture(packs, state, obs)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) != 1 || plan.Blockers()[0].Kind != BlockerCapabilityConflict {
		t.Fatalf("blockers = %+v", plan.Blockers())
	}
}

func TestCatalogCurrentDriftPlansSafeRepair(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	drift := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "drift", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "2.0.0", Active: true, Revision: 7}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "drift"}}}
	facade, _, _ := updateFixture([]Pack{pack}, state, drift)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || plan.NoOp() || !plan.Applicable() || len(phaseActions(plan.phases, ConsentReversibleLocal)) != 1 {
		t.Fatalf("drift plan noop=%v applicable=%v phases=%+v err=%v", plan.NoOp(), plan.Applicable(), plan.Phases(), err)
	}
}

func TestUpdateRejectsStaleIntentOwnershipAndHostFactsWithZeroEffects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*fakeActivationStore)
		obs    []ActivationObservation
	}{
		{name: "intent", mutate: func(s *fakeActivationStore) { s.state.Intent.Revision++ }},
		{name: "ownership", mutate: func(s *fakeActivationStore) { s.state.Ownership[0].Contributors = []string{"changed"} }},
		{name: "host", obs: []ActivationObservation{
			{Revision: "changed", Projections: []ObservedProjection{
				{ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}},
			}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
			preview := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}}}}
			observations := append([]ActivationObservation{preview}, tc.obs...)
			state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
			facade, adapter, store := updateFixture([]Pack{pack}, state, observations...)
			plan, _ := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
			if tc.mutate != nil {
				tc.mutate(store)
			}
			_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
			if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
				t.Fatalf("err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
			}
		})
	}
}

func TestUpdateExternalPhasesUseTypedApprovalsAndStopAtBarrier(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{missingEngramResolution()}}
	executor := &fakeExternalExecutor{failID: "external:engram:setup:codex", failErr: errors.New("setup failed")}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"), engramObservation("ready"))
	facade.catalog.packs[0].Version = "2.0.0"
	store.state = ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	phases := plan.Phases()
	if len(phases) != 3 || phases[0].Kind != ConsentReversibleLocal || phases[1].Kind != ConsentExecutableExternal || phases[2].Kind != ConsentHostFollowUp {
		t.Fatalf("phases = %+v", phases)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrApprovalMismatch) || len(store.saves) != 0 {
		t.Fatalf("local-only approval err=%v saves=%d", err, len(store.saves))
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if err == nil || len(executor.actions) != 2 || len(adapter.actions) == 0 || store.state.Journal == nil || store.state.Journal.FailedAction != "external:engram:setup:codex" {
		t.Fatalf("barrier err=%v external=%+v state=%+v", err, executor.actions, store.state)
	}
}

func TestUpdateRejectsChangedDependencyClosureWithZeroEffects(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"dep"}}}, {ID: "dep", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"dep"}}}
	obs := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "combined", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "combined"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}
	facade, adapter, store := updateFixture(packs, state, obs)
	plan, _ := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	facade.catalog.packs[0].Requires.Capabilities = nil
	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("dependency stale err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}

func TestUpdateRejectsChangedExecutableResolutionWithZeroEffects(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/v1/engram"), availableEngramResolution("/v2/engram")}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"))
	facade.catalog.packs[0].Version = "2.0.0"
	store.state = ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}
	plan, _ := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "engram", Surface: SurfaceCodex})
	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 || len(executor.actions) != 0 {
		t.Fatalf("executable stale err=%v saves=%d local=%d external=%d", err, len(store.saves), len(adapter.actions), len(executor.actions))
	}
}
