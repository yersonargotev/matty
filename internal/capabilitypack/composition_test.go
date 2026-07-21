package capabilitypack

import (
	"context"
	"testing"
)

func TestCompositionRejectsImplicitSharingAndStaleAliases(t *testing.T) {
	binding := func(sharing string) []Binding {
		return []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "review", Invocation: "$review", Mode: "native", Sharing: sharing}}
	}
	shared := Resource{Kind: "skill", ID: "review", Source: "content/review", Bindings: binding("exclusive")}
	packs := []Pack{
		{ID: "active", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{shared}},
		{ID: "requested", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{shared}},
	}
	active := ActivationIntent{PackID: "active", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Aliases: []SurfaceAlias{{Kind: "command", ID: "removed", Name: "addy-removed"}}}
	facade := NewFacade(Catalog{packs: packs})
	result, err := facade.compose(packs[1], ActivationState{Intent: active}, SurfaceCodex, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.blockers) != 2 || result.blockers[0].Kind != BlockerAlias || result.blockers[1].Kind != BlockerSharing {
		t.Fatalf("blockers = %+v", result.blockers)
	}
}

func TestCompositionAcceptsIdenticalExplicitSharedContributions(t *testing.T) {
	resource := Resource{Kind: "skill", ID: "review", Source: "content/review", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "review", Invocation: "$review", Mode: "native", Sharing: "shared"}}}
	packs := []Pack{{ID: "active", Version: "1", Resources: []Resource{resource}}, {ID: "requested", Version: "1", Resources: []Resource{resource}}}
	active := ActivationIntent{PackID: "active", Surface: SurfaceCodex, Version: "1", Active: true, Aliases: []SurfaceAlias{}}
	result, err := NewFacade(Catalog{packs: packs}).compose(packs[1], ActivationState{Intent: active}, SurfaceCodex, true)
	if err != nil || len(result.blockers) != 0 || len(result.contributorSet("skill:review")) != 2 {
		t.Fatalf("composition = %+v, err = %v", result, err)
	}
}

func TestClaudeCommandAndSkillShareNamespaceAndRequireAlias(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", manifestVersion: manifestSchemaV3, Surfaces: []Surface{SurfaceClaude}, Resources: []Resource{
		{Kind: "command", ID: "review-command", Bindings: []Binding{{Surface: SurfaceClaude, Projection: "skill", Name: "review", Invocation: "/review", Mode: "native", Sharing: "exclusive"}}},
		{Kind: "skill", ID: "review-skill", Bindings: []Binding{{Surface: SurfaceClaude, Projection: "skill", Name: "review", Invocation: "/review", Mode: "native", Sharing: "exclusive"}}},
	}}
	result, err := NewFacade(Catalog{packs: []Pack{pack}}).compose(pack, ActivationState{}, SurfaceClaude, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.blockers) != 1 || result.blockers[0].Kind != BlockerAlias || result.blockers[0].Subject != "personal-skill:review" {
		t.Fatalf("blockers = %#v", result.blockers)
	}

	intent := ActivationIntent{PackID: "app", Surface: SurfaceClaude, Version: "1", Active: true, Aliases: []SurfaceAlias{{Kind: "command", ID: "review-command", Name: "review-command"}}}
	resolved, err := NewFacade(Catalog{packs: []Pack{pack}}).compose(pack, ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}, SurfaceClaude, true)
	if err != nil || len(resolved.blockers) != 0 {
		t.Fatalf("aliased composition = %#v err=%v", resolved, err)
	}
}

func TestCompositionAppliesOnlyExplicitSurfaceAliasToHostBinding(t *testing.T) {
	resource := Resource{Kind: "command", ID: "review", Bindings: []Binding{
		{Surface: SurfaceCodex, Projection: "skill", Name: "review", Invocation: "$review", Mode: "degraded", Degradation: "codex-command-as-workflow-skill", Sharing: "exclusive"},
		{Surface: SurfaceOpenCode, Projection: "command", Name: "review", Invocation: "/review", Mode: "native", Sharing: "exclusive"},
	}}
	pack := Pack{ID: "addy", Version: "1", Resources: []Resource{resource}}
	intent := ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1", Active: true, Aliases: []SurfaceAlias{{Kind: "command", ID: "review", Name: "addy-review"}}}
	result, err := NewFacade(Catalog{packs: []Pack{pack}}).compose(pack, ActivationState{Intent: intent}, SurfaceCodex, true)
	if err != nil || len(result.blockers) != 0 {
		t.Fatalf("composition = %+v, err = %v", result, err)
	}
	bindings := result.combinedPack().Resources[0].Bindings
	if bindings[0].Name != "addy-review" || bindings[0].Invocation != "$addy-review" || bindings[1].Name != "review" || bindings[1].Invocation != "/review" {
		t.Fatalf("bindings = %+v", bindings)
	}
}

func TestPreviewIncludesInactiveTransitiveRequirementsInCanonicalComposition(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"cap:b"}}, Resources: []Resource{{Kind: "instruction", ID: "app", Source: "app"}}},
		{ID: "b", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:b"}, Requires: Requirements{Capabilities: []string{"cap:c"}}, Resources: []Resource{{Kind: "instruction", ID: "b", Source: "b"}}},
		{ID: "c", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:c"}, Resources: []Resource{{Kind: "instruction", ID: "c", Source: "c"}}},
	}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:all", ObservedFingerprint: "missing", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:all", Description: "combined"}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{obs}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Activations()
	if len(got) != 3 || got[0].Pack.ID != "app" || got[0].Role != ActivationRequested || got[1].Pack.ID != "b" || got[1].Role != ActivationRequired || got[2].Pack.ID != "c" {
		t.Fatalf("activations=%+v", got)
	}
	if !plan.Applicable() {
		t.Fatalf("blockers=%+v", plan.Blockers())
	}
}

func TestPreviewAggregatesCompositionAndOwnershipBlockersWithoutApplicableActions(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:x"}, Conflicts: []string{"cap:y"}, Requires: Requirements{Capabilities: []string{"missing"}}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "one"}}},
		{ID: "active", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:y"}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "two"}}},
	}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "user", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:shared"}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{obs}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "active", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) < 4 || len(plan.Phases()) != 0 {
		t.Fatalf("plan applicable=%v blockers=%+v phases=%+v", plan.Applicable(), plan.Blockers(), plan.Phases())
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("blocked Preview mutated state")
	}
	if _, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err == nil {
		t.Fatal("blocked Apply succeeded")
	}
}

func TestApplyRecordsCompleteContributorsOnlyAfterFreshVerification(t *testing.T) {
	packs := []Pack{
		{ID: "active", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
		{ID: "requested", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
	}
	pending := SurfaceInspection{Revision: "host-1", Projections: []ObservedProjection{{ID: "instruction:shared", ObservedFingerprint: "missing", DesiredFingerprint: "desired", Action: ProjectionAction{ID: "instruction:shared", Description: "write shared"}}}}
	verified := SurfaceInspection{Revision: "host-2", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "desired", DesiredFingerprint: "desired", Action: ProjectionAction{ID: "instruction:shared"}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, verified}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "active", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}}}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "requested", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	receipt := facade.Approve(plan, ConsentReversibleLocal)
	if _, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{receipt}, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	got := store.state.Ownership
	if len(got) != 1 || len(got[0].Contributors) != 2 || got[0].Contributors[0] != "active" || got[0].Contributors[1] != "requested" {
		t.Fatalf("ownership=%+v", got)
	}
	if len(store.state.Intents) != 2 {
		t.Fatalf("intents=%+v", store.state.Intents)
	}
}

func TestApplyRejectsChangedDependencyCatalogBeforePersistenceOrActions(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"cap:dep"}}}, {ID: "dep", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:dep"}}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:combined", ObservedFingerprint: "missing", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:combined"}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	facade.catalog.packs[1].Version = "2.0.0"
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err == nil {
		t.Fatal("changed catalog was applied")
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("stale apply effects saves=%d actions=%d", len(store.saves), len(adapter.actions))
	}
}

func TestPreviewTraversesRequirementsOfAlreadyActiveDependencies(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"b"}}}, {ID: "b", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"b"}, Requires: Requirements{Capabilities: []string{"c"}}}, {ID: "c", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"c"}}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "combined", ObservedFingerprint: "missing", DesiredFingerprint: "new", Action: ProjectionAction{ID: "combined"}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "b", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3}}}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Activations()
	if len(got) != 2 || got[0].Pack.ID != "app" || got[1].Pack.ID != "c" || got[1].Role != ActivationRequired {
		t.Fatalf("activations=%+v", got)
	}
}

func TestNoOpApplyStillRejectsFreshHostChanges(t *testing.T) {
	pack := Pack{ID: "matty", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	converged := SurfaceInspection{Revision: "host-1", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}}}
	changed := SurfaceInspection{Revision: "host-2", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "changed", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{converged, changed}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "matty", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"matty"}, Fingerprint: "same"}}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() {
		t.Fatalf("no-op preview=%v err=%v", plan.NoOp(), err)
	}
	if _, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan}); err == nil {
		t.Fatal("stale no-op Apply succeeded")
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("stale no-op Apply had effects")
	}
}
