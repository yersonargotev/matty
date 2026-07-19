package capabilitypack

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/packy/internal/addyacceptance"
)

func TestCompleteAddyAliasesRemainSurfaceLocalAndSharedRemovalRetainsContributor(t *testing.T) {
	catalog := completeAddyCatalog(t)
	pack, _ := catalog.Show("addy")
	alias := SurfaceAlias{Kind: "command", ID: "build", Name: "addy-build"}
	composed, err := NewFacade(catalog).compose(pack, ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: pack.Version, Active: true, Aliases: []SurfaceAlias{alias}}}, SurfaceCodex, true)
	if err != nil || len(composed.blockers) != 0 {
		t.Fatalf("aliased complete composition = %+v err=%v", composed.blockers, err)
	}
	for _, resource := range composed.combinedPack().Resources {
		if resource.Kind != "command" || resource.ID != "build" {
			continue
		}
		if resource.Bindings[0].Name != "addy-build" || resource.Bindings[0].Invocation != "$addy-build" || resource.Bindings[1].Name != "build" || resource.Bindings[1].Invocation != "/build" {
			t.Fatalf("alias leaked across surfaces: %+v", resource.Bindings)
		}
	}

	var shared Resource
	for _, resource := range pack.Resources {
		if resource.Kind == "skill" && resource.ID == "using-agent-skills" {
			shared = resource
			for i := range shared.Bindings {
				shared.Bindings[i].Sharing = "shared"
			}
		}
	}
	other := Pack{ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{shared}}
	for i := range pack.Resources {
		if pack.Resources[i].Kind == "skill" && pack.Resources[i].ID == "using-agent-skills" {
			pack.Resources[i] = shared
		}
	}
	catalog = Catalog{packs: []Pack{pack, other}}
	state := ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 4}, Intents: []ActivationIntent{{PackID: "addy", Surface: SurfaceCodex, Version: pack.Version, Active: true}, {PackID: "other", Surface: SurfaceCodex, Version: other.Version, Active: true}}, Ownership: []ProjectionOwnership{{ID: "skill:using-agent-skills", Contributors: []string{"addy", "other"}, Fingerprint: "same"}}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "skill:using-agent-skills", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "skill:using-agent-skills"}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	store := &fakeActivationStore{state: state}
	plan, err := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter})).PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if retained := plan.RetainedProjections(); len(retained) != 1 || retained[0].ID != "skill:using-agent-skills" || !reflect.DeepEqual(retained[0].Contributors, []string{"other"}) {
		t.Fatalf("shared Addy projection was not retained: %+v", retained)
	}
	if len(plan.Phases()) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("shared retention attempted removal: phases=%+v actions=%+v", plan.Phases(), adapter.actions)
	}
}

func TestCompleteAddyCollisionBlocksUntilExactSurfaceAliasReplans(t *testing.T) {
	catalog := completeAddyCatalog(t)
	adapter := &fakeSurfaceAdapter{inspect: func(transition SurfaceTransition) SurfaceInspection {
		inspection := completeAddyObservation(transition.Desired, SurfaceCodex, "missing")
		inspection.OccupiedNames = []OccupiedName{{Namespace: "skill", Name: "build", OwnerType: "unmanaged", Fingerprint: "operator"}}
		for i := range inspection.Projections {
			if inspection.Projections[i].ID == "skill:build" {
				inspection.Projections[i].Exists = true
				inspection.Projections[i].ObservedFingerprint = "operator"
			}
		}
		return inspection
	}}
	store := &fakeActivationStore{}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	blocked, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Applicable() || len(blocked.Blockers()) != 1 {
		t.Fatalf("unaliased collision was not an exact blocker: %+v", blocked.JSONReport(true))
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: blocked, Interactive: true}); !errors.Is(err, ErrPlanNotActionable) {
		t.Fatalf("blocked collision Apply error = %v", err)
	}
	alias := SurfaceAlias{Kind: "command", ID: "build", Name: "addy-build"}
	replanned, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex, Aliases: []SurfaceAlias{alias}})
	if err != nil {
		t.Fatal(err)
	}
	if !replanned.Applicable() || len(replanned.Blockers()) != 0 || replanned.Aliases()[0] != alias {
		t.Fatalf("exact alias did not produce a fresh applicable plan: %+v", replanned.JSONReport(true))
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("collision/alias previews crossed a mutation boundary")
	}
}

func TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp(t *testing.T) {
	catalog := completeAddyCatalog(t)
	pack, err := catalog.Show("addy")
	if err != nil {
		t.Fatal(err)
	}
	pending := completeAddyObservation(pack, SurfaceCodex, "missing")
	verified := completeAddyObservation(pack, SurfaceCodex, "desired")
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, verified, verified}}
	store := &fakeActivationStore{}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Phases()) != 1 || plan.Phases()[0].Kind != ConsentReversibleLocal || len(plan.Phases()[0].Actions) != 36 {
		t.Fatalf("complete Addy phase = %+v", plan.Phases())
	}
	wrong := facade.Approve(plan, ConsentDestructiveCleanup)
	if _, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{wrong}, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("wrong typed receipt error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("rejected receipt crossed an effect boundary")
	}

	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Projections != 36 || len(store.state.Ownership) != 36 {
		t.Fatalf("verified complete Apply = actions %d ownership %d", result.Projections, len(store.state.Ownership))
	}
	noOp, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !noOp.NoOp() || len(noOp.Phases()) != 0 {
		t.Fatalf("exact candidate was not a no-op: %+v", noOp.JSONReport(true))
	}
}

func TestCompleteAddyCohortStalePreflightAndAtomicFailureRequireFreshRecovery(t *testing.T) {
	catalog := completeAddyCatalog(t)
	pack, _ := catalog.Show("addy")
	pending := completeAddyObservation(pack, SurfaceOpenCode, "missing")
	changed := completeAddyObservation(pack, SurfaceOpenCode, "changed")
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, changed}}
	store := &fakeActivationStore{}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}))
	plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceOpenCode})
	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("stale complete cohort crossed boundary: err=%v actions=%d saves=%d", err, len(adapter.actions), len(store.saves))
	}
}

func TestCompleteAddyAtomicAdapterFailureRecordsAttemptAndRequiresFreshRecoveryPlan(t *testing.T) {
	catalog := completeAddyCatalog(t)
	pack, _ := catalog.Show("addy")
	pending := completeAddyObservation(pack, SurfaceCodex, "missing")
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, pending}, applyErr: errors.New("atomic adapter interruption")}
	store := &fakeActivationStore{}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex})
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true}); err == nil {
		t.Fatal("adapter interruption unexpectedly succeeded")
	}
	if store.state.Journal == nil || !store.state.Intent.Active {
		t.Fatalf("failed attempt state = %+v", store.state)
	}
	recovery, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !recovery.Recovery() || recovery.ID() == plan.ID() || recovery.HistoricalAttempt() == nil {
		t.Fatalf("fresh recovery plan = %+v", recovery.JSONReport(true))
	}
}

func TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct(t *testing.T) {
	catalog := completeAddyCatalog(t)
	pack, _ := catalog.Show("addy")
	projection := completeAddyObservation(pack, SurfaceCodex, "desired")
	projection.PendingHumanActions = []string{"authenticate Codex host"}
	projection.Readiness = ReadinessObservation{AuthorizationObserved: true, Authorized: true}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{projection}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 3}}}
	for _, observed := range projection.Projections {
		store.state.Ownership = append(store.state.Ownership, ProjectionOwnership{ID: observed.ID, Fingerprint: observed.DesiredFingerprint, Contributors: []string{"addy"}})
	}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	report, err := facade.Status(context.Background(), StatusRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if !entry.Readiness.Configured || !entry.Readiness.Authorized || entry.Readiness.Usable || entry.ReadinessObserved.Usability {
		t.Fatalf("unknown usability collapsed: %+v observed=%+v", entry.Readiness, entry.ReadinessObserved)
	}
	json := report.JSONReport(true).Entries[0]
	if json.Readiness.Usable.State != "unknown" || json.Readiness.Usable.Value != nil {
		t.Fatalf("JSON unknown usability collapsed: %+v", json.Readiness)
	}
	if !reflect.DeepEqual(entry.PendingHumanActions, []string{"authenticate Codex host"}) || len(entry.Contract.OptionalModes) != 4 || len(entry.Contract.Exclusions) != 2 {
		t.Fatalf("pending/optional/excluded facts mixed: %+v", entry)
	}
}

func TestCompleteAddyExactOwnershipRemovalBlocksDriftWithoutEffects(t *testing.T) {
	catalog := completeAddyCatalog(t)
	pack, _ := catalog.Show("addy")
	state := ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 5}, Ownership: []ProjectionOwnership{{ID: "skill:using-agent-skills", Fingerprint: "sealed", Contributors: []string{"addy"}}}}
	drift := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "skill:using-agent-skills", Exists: true, ObservedFingerprint: "operator-drift", DesiredFingerprint: "", Action: ProjectionAction{ID: "skill:using-agent-skills", Mode: ProjectionDeleteTarget}}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{drift}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.PendingHumanActions()) == 0 || len(plan.Phases()) != 0 {
		t.Fatalf("drifted removal was not blocked: %+v", plan.JSONReport(true))
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 || len(store.state.Ownership) != 1 || store.state.Ownership[0].Fingerprint != "sealed" || !store.state.Intent.Active {
		t.Fatalf("drifted exact-ownership removal preview was not zero-mutation: actions=%v state=%+v", adapter.actions, store.state)
	}
}

func TestCompleteAddyDualSurfaceFailurePreservesAuthorizedOtherSurface(t *testing.T) {
	catalog := completeAddyCatalog(t)
	pack, _ := catalog.Show("addy")
	codexPending := completeAddyObservation(pack, SurfaceCodex, "missing")
	codexVerified := completeAddyObservation(pack, SurfaceCodex, "desired")
	openPending := completeAddyObservation(pack, SurfaceOpenCode, "missing")
	openChanged := completeAddyObservation(pack, SurfaceOpenCode, "changed")
	codexAdapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{codexPending, codexPending, codexVerified}}
	openAdapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{openPending, openChanged}}
	store := &surfaceStateStore{states: map[Surface]ActivationState{}}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: codexAdapter, SurfaceOpenCode: openAdapter}))

	codexPlan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: codexPlan, Approvals: []ApprovalReceipt{facade.Approve(codexPlan, ConsentReversibleLocal)}, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	codexBefore := cloneActivationState(store.states[SurfaceCodex])
	openPlan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: openPlan, Approvals: []ApprovalReceipt{facade.Approve(openPlan, ConsentReversibleLocal)}, Interactive: true}); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("OpenCode stale failure = %v", err)
	}
	if !reflect.DeepEqual(store.states[SurfaceCodex], codexBefore) || len(codexAdapter.actions) != 36 {
		t.Fatal("OpenCode failure changed authorized Codex state")
	}
	if len(openAdapter.actions) != 0 || store.states[SurfaceOpenCode].Intent.Active {
		t.Fatal("failed OpenCode surface crossed its write boundary")
	}
}

type surfaceStateStore struct{ states map[Surface]ActivationState }

func (s *surfaceStateStore) Load(_ context.Context, surface Surface) (ActivationState, error) {
	return cloneActivationState(s.states[surface]), nil
}

func (s *surfaceStateStore) Save(_ context.Context, surface Surface, expectedRevision int, state ActivationState) error {
	if s.states[surface].Intent.Revision != expectedRevision {
		return ErrStalePlan
	}
	s.states[surface] = cloneActivationState(state)
	return nil
}

func completeAddyCatalog(t *testing.T) Catalog {
	t.Helper()
	bundle := filepath.Join(t.TempDir(), "bundle")
	if err := addyacceptance.WriteSnapshot(bundle); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(addyacceptance.Canonical().Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(manifest, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, "surfaces")
	manifest, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bundle, "packs", "addy", "pack.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := discoverCatalog(bundle, []catalogEntry{{ID: "addy", Description: "Addy acceptance cohort", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}}})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func completeAddyObservation(pack Pack, surface Surface, observed string) SurfaceInspection {
	inspection := SurfaceInspection{Revision: "host-stable"}
	for _, resource := range pack.Resources {
		for _, binding := range resource.Bindings {
			if binding.Surface != surface {
				continue
			}
			fingerprint := "desired"
			inspection.Projections = append(inspection.Projections, ObservedProjection{ID: binding.Projection + ":" + binding.Name, Exists: observed == fingerprint, ObservedFingerprint: observed, DesiredFingerprint: fingerprint, Action: ProjectionAction{ID: binding.Projection + ":" + binding.Name, Description: "project " + resource.Kind + ":" + resource.ID}})
		}
	}
	return inspection
}
