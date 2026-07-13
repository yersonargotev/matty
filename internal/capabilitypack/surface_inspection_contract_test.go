package capabilitypack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type surfaceInspectionCall struct {
	kind        string
	prior       Pack
	desired     Pack
	ownership   []ProjectionOwnership
	resolutions []ExecutableResolution
}

// completeSurfaceContractAdapter deliberately implements every current
// inspection capability. Ticket 02 can replace those optional entry points
// with SurfaceAdapter without changing these facade-level expectations.
type completeSurfaceContractAdapter struct {
	observations   []ActivationObservation
	readiness      []ReadinessObservation
	calls          []surfaceInspectionCall
	readinessCalls int
	applied        [][]ProjectionAction
}

func (a *completeSurfaceContractAdapter) nextObservation() ActivationObservation {
	index := len(a.calls) - 1
	if index >= len(a.observations) {
		index = len(a.observations) - 1
	}
	if index < 0 {
		return ActivationObservation{}
	}
	return a.observations[index]
}

func (a *completeSurfaceContractAdapter) InspectActivation(ctx context.Context, desired Pack) (ActivationObservation, error) {
	return a.InspectActivationWithResolution(ctx, desired, nil)
}

func (a *completeSurfaceContractAdapter) InspectActivationWithResolution(_ context.Context, desired Pack, resolutions []ExecutableResolution) (ActivationObservation, error) {
	a.calls = append(a.calls, surfaceInspectionCall{kind: "desired", desired: desired, resolutions: append([]ExecutableResolution(nil), resolutions...)})
	return a.nextObservation(), nil
}

func (a *completeSurfaceContractAdapter) InspectDeactivation(_ context.Context, prior, desired Pack, resolutions []ExecutableResolution) (ActivationObservation, error) {
	a.calls = append(a.calls, surfaceInspectionCall{kind: "prior-to-desired", prior: prior, desired: desired, resolutions: append([]ExecutableResolution(nil), resolutions...)})
	return a.nextObservation(), nil
}

func (a *completeSurfaceContractAdapter) InspectReconcile(_ context.Context, desired Pack, ownership []ProjectionOwnership, resolutions []ExecutableResolution) (ActivationObservation, error) {
	a.calls = append(a.calls, surfaceInspectionCall{kind: "ownership-residual", desired: desired, ownership: cloneOwnership(ownership), resolutions: append([]ExecutableResolution(nil), resolutions...)})
	return a.nextObservation(), nil
}

func (a *completeSurfaceContractAdapter) ApplyProjections(_ context.Context, actions []ProjectionAction) *ProjectionActionError {
	a.applied = append(a.applied, append([]ProjectionAction(nil), actions...))
	return nil
}

func (a *completeSurfaceContractAdapter) InspectReadiness(_ context.Context, _ Pack, _ ActivationObservation, _ []ExecutableResolution) (ReadinessObservation, error) {
	index := a.readinessCalls
	a.readinessCalls++
	if index >= len(a.readiness) {
		index = len(a.readiness) - 1
	}
	if index < 0 {
		return ReadinessObservation{}, nil
	}
	return a.readiness[index], nil
}

func contractFacade(packs []Pack, state ActivationState, adapter *completeSurfaceContractAdapter) (Facade, *fakeActivationStore) {
	store := &fakeActivationStore{state: state}
	facade := NewFacade(
		Catalog{packs: packs},
		nil,
		WithActivation(store, map[Surface]ActivationAdapter{SurfaceCodex: adapter}),
		WithReadinessInspectors(map[Surface]ReadinessInspector{SurfaceCodex: adapter}),
	)
	return facade, store
}

func projectionObservation(pack Pack) ActivationObservation {
	projections := make([]ObservedProjection, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		id := resource.Kind + ":" + resource.ID
		projections = append(projections, ObservedProjection{
			ID: id, Exists: true, ObservedFingerprint: resource.Source, DesiredFingerprint: resource.Source,
			Action: ProjectionAction{ID: id},
		})
	}
	return ActivationObservation{Revision: "host-current", Projections: projections}
}

func packResourceIDs(pack Pack) []string {
	ids := make([]string, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		ids = append(ids, resource.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestSurfaceInspectionContractPreservesLifecycleScopes(t *testing.T) {
	app := Pack{ID: "app", Version: "2", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "app", Source: "v2"}}}
	other := Pack{ID: "other", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "other", Source: "other"}}}
	activeApp := activeIntent("app", "2", 4)
	activeOther := activeIntent("other", "1", 2)

	t.Run("status is desired-only and excludes unrelated active packs", func(t *testing.T) {
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{projectionObservation(app)}}
		facade, _ := contractFacade([]Pack{app, other}, ActivationState{Intent: activeApp, Intents: []ActivationIntent{activeApp, activeOther}}, adapter)
		if _, err := facade.Status(context.Background(), StatusRequest{PackID: "app", Surface: SurfaceCodex}); err != nil {
			t.Fatal(err)
		}
		if len(adapter.calls) != 1 || adapter.calls[0].kind != "desired" || !reflect.DeepEqual(packResourceIDs(adapter.calls[0].desired), []string{"app"}) {
			t.Fatalf("status inspection = %+v", adapter.calls)
		}
	})

	t.Run("activation is desired-only", func(t *testing.T) {
		observation := projectionObservation(Pack{Resources: append(append([]Resource{}, app.Resources...), other.Resources...)})
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{observation}}
		facade, _ := contractFacade([]Pack{app, other}, ActivationState{Intent: activeOther, Intents: []ActivationIntent{activeOther}}, adapter)
		if _, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex}); err != nil {
			t.Fatal(err)
		}
		if len(adapter.calls) != 1 || adapter.calls[0].kind != "desired" || !reflect.DeepEqual(packResourceIDs(adapter.calls[0].desired), []string{"app", "other"}) {
			t.Fatalf("activation inspection = %+v", adapter.calls)
		}
	})

	t.Run("update is desired-only and uses catalog-current content", func(t *testing.T) {
		oldIntent := activeIntent("app", "1", 4)
		observation := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:app", Exists: true, ObservedFingerprint: "v1", DesiredFingerprint: "v2", Action: ProjectionAction{ID: "instruction:app", Description: "write v2"}}}}
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{observation}}
		state := ActivationState{Intent: oldIntent, Ownership: []ProjectionOwnership{{ID: "instruction:app", Contributors: []string{"app"}, Fingerprint: "v1"}}}
		facade, _ := contractFacade([]Pack{app}, state, adapter)
		plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		if len(adapter.calls) != 1 || adapter.calls[0].kind != "desired" || adapter.calls[0].desired.Resources[0].Source != "v2" ||
			!reflect.DeepEqual(phaseActionIDs(plan.Phases(), ConsentReversibleLocal), []string{"instruction:app"}) {
			t.Fatalf("update inspection/plan = calls:%+v phases:%+v", adapter.calls, plan.Phases())
		}
	})

	t.Run("deactivation compares prior to desired", func(t *testing.T) {
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{{Revision: "host-current"}}}
		facade, _ := contractFacade([]Pack{app, other}, ActivationState{Intent: activeApp, Intents: []ActivationIntent{activeApp, activeOther}}, adapter)
		if _, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex}); err != nil {
			t.Fatal(err)
		}
		if len(adapter.calls) != 1 || adapter.calls[0].kind != "prior-to-desired" ||
			!reflect.DeepEqual(packResourceIDs(adapter.calls[0].prior), []string{"app", "other"}) ||
			!reflect.DeepEqual(packResourceIDs(adapter.calls[0].desired), []string{"other"}) {
			t.Fatalf("deactivation inspection = %+v", adapter.calls)
		}
	})

	t.Run("targeted and surface-wide reconciliation preserve distinct scopes and ownership residual", func(t *testing.T) {
		ownership := []ProjectionOwnership{{ID: "instruction:obsolete", Contributors: []string{"app"}, Fingerprint: "owned"}}
		observation := ActivationObservation{Revision: "host", Projections: []ObservedProjection{
			{ID: "instruction:app", ObservedFingerprint: "missing", DesiredFingerprint: "v2", Action: ProjectionAction{ID: "instruction:app", Description: "write app"}},
			{ID: "instruction:other", ObservedFingerprint: "missing", DesiredFingerprint: "other", Action: ProjectionAction{ID: "instruction:other", Description: "write other"}},
		}}
		state := ActivationState{Intent: activeApp, Intents: []ActivationIntent{activeApp, activeOther}, Ownership: ownership}
		preview := func(request ReconcileRequest) (ReconciliationPlan, *completeSurfaceContractAdapter) {
			adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{observation}}
			facade, _ := contractFacade([]Pack{app, other}, state, adapter)
			plan, err := facade.PreviewReconcile(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			return plan, adapter
		}

		targeted, targetedAdapter := preview(ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
		surfaceWide, surfaceAdapter := preview(ReconcileRequest{Surface: SurfaceCodex})
		if targeted.ReconcileScope() != ReconcileTargeted || surfaceWide.ReconcileScope() != ReconcileSurfaceWide || targeted.ID() == surfaceWide.ID() {
			t.Fatalf("reconcile scope/identity: targeted=%s/%s surface-wide=%s/%s", targeted.ReconcileScope(), targeted.ID(), surfaceWide.ReconcileScope(), surfaceWide.ID())
		}
		if got := phaseActionIDs(targeted.Phases(), ConsentReversibleLocal); !reflect.DeepEqual(got, []string{"instruction:app"}) {
			t.Fatalf("targeted actions = %v", got)
		}
		if got := phaseActionIDs(surfaceWide.Phases(), ConsentReversibleLocal); !reflect.DeepEqual(got, []string{"instruction:app", "instruction:other"}) {
			t.Fatalf("surface-wide actions = %v", got)
		}
		for name, adapter := range map[string]*completeSurfaceContractAdapter{"targeted": targetedAdapter, "surface-wide": surfaceAdapter} {
			if len(adapter.calls) != 1 || adapter.calls[0].kind != "ownership-residual" || !reflect.DeepEqual(adapter.calls[0].ownership, ownership) || !reflect.DeepEqual(packResourceIDs(adapter.calls[0].desired), []string{"app", "other"}) {
				t.Fatalf("%s reconcile inspection = %+v", name, adapter.calls)
			}
		}
	})
}

func TestSurfaceInspectionContractPreservesOwnershipAndDestructiveConsent(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "catalog"}}}
	intent := activeIntent("app", "1", 4)
	ownedState := ActivationState{Intent: intent, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "owned"}}}
	removal := ObservedProjection{ID: "instruction:guide", Exists: true, ObservedFingerprint: "owned", Action: ProjectionAction{ID: "instruction:guide", Description: "delete guide", Mode: ProjectionDeleteTarget}}

	t.Run("deactivation protects ownership and requires destructive approval", func(t *testing.T) {
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{{Revision: "host", RemovalCandidates: []ObservedProjection{removal}}}}
		facade, store := contractFacade([]Pack{pack}, ownedState, adapter)
		plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		phases := plan.Phases()
		if !plan.Applicable() || len(plan.Blockers()) != 0 || len(plan.PendingHumanActions()) != 0 || len(phases) != 1 || phases[0].Kind != ConsentDestructiveCleanup || !phases[0].ApprovalRequired || !reflect.DeepEqual(phaseActionIDs(phases, ConsentDestructiveCleanup), []string{"instruction:guide"}) {
			t.Fatalf("owned deactivation plan = applicable:%v blockers:%+v pending:%v phases:%+v", plan.Applicable(), plan.Blockers(), plan.PendingHumanActions(), phases)
		}
		if len(adapter.applied) != 0 || len(store.saves) != 0 {
			t.Fatalf("deactivation preview caused effects: applied=%d saves=%d", len(adapter.applied), len(store.saves))
		}
	})

	t.Run("deactivation preserves unowned content as pending human action", func(t *testing.T) {
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{{Revision: "host", RemovalCandidates: []ObservedProjection{removal}}}}
		facade, store := contractFacade([]Pack{pack}, ActivationState{Intent: intent}, adapter)
		plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Phases()) != 0 || len(plan.PendingHumanActions()) != 1 || plan.PendingHumanActions()[0] != "preserved instruction:guide because it is drifted, ambiguous, unmanaged, or ownership no longer matches" || len(adapter.applied) != 0 || len(store.saves) != 0 {
			t.Fatalf("unowned deactivation plan = pending:%v phases:%+v applied:%d saves:%d", plan.PendingHumanActions(), plan.Phases(), len(adapter.applied), len(store.saves))
		}
	})

	t.Run("reconciliation requires destructive approval for verified ownership residual", func(t *testing.T) {
		candidate := RemovalCandidate(removal, ProjectionDeleteTarget, "", "delete obsolete guide")
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{{Revision: "host", RemovalCandidates: []ObservedProjection{candidate}}}}
		facade, store := contractFacade([]Pack{pack}, ownedState, adapter)
		plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "app", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		phases := plan.Phases()
		if plan.ReconcileScope() != ReconcileTargeted || len(phases) != 1 || phases[0].Kind != ConsentDestructiveCleanup || !phases[0].ApprovalRequired || !reflect.DeepEqual(phaseActionIDs(phases, ConsentDestructiveCleanup), []string{"instruction:guide"}) {
			t.Fatalf("owned reconcile plan = scope:%s phases:%+v", plan.ReconcileScope(), phases)
		}
		if len(adapter.applied) != 0 || len(store.saves) != 0 {
			t.Fatalf("reconcile preview caused effects: applied=%d saves=%d", len(adapter.applied), len(store.saves))
		}
	})
}

func phaseActionIDs(phases []PlanPhase, kind ConsentKind) []string {
	var ids []string
	for _, phase := range phases {
		if phase.Kind != kind {
			continue
		}
		for _, action := range phase.Actions {
			ids = append(ids, action.ID)
		}
	}
	return ids
}

func TestSurfaceInspectionContractPreservesPlanPolicyAndInspectionPurity(t *testing.T) {
	sandbox := t.TempDir()
	home, configHome := filepath.Join(sandbox, "home"), filepath.Join(sandbox, "config")
	for _, path := range []string{home, configHome} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "catalog"}, {Kind: "instruction", ID: "shared", Source: "catalog-shared"}}}
	observation := ActivationObservation{
		Revision: "host-7",
		Projections: []ObservedProjection{
			{ID: "instruction:shared", Exists: true, ObservedFingerprint: "operator-edit", DesiredFingerprint: "catalog-shared", Action: ProjectionAction{ID: "instruction:shared", Description: "write shared"}},
			{ID: "instruction:guide", ObservedFingerprint: "missing", DesiredFingerprint: "catalog", Action: ProjectionAction{ID: "instruction:guide", Description: "write guide"}},
		},
		Readiness:           ReadinessStatus{Authorized: true, Usable: true},
		PendingHumanActions: []string{"reload host"},
	}
	adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{observation}}
	facade, store := contractFacade([]Pack{pack}, ActivationState{}, adapter)
	resolver := &fakeExecutableResolver{}
	executor := &fakeExternalExecutor{}
	WithExternalEffects(resolver, executor)(&facade)
	before := cloneActivationState(store.state)

	first, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	second, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID() == "" || first.ID() != second.ID() || first.Digest() != second.Digest() {
		t.Fatalf("unstable plan identity: first=%s/%s second=%s/%s", first.ID(), first.Digest(), second.ID(), second.Digest())
	}
	phases := first.Phases()
	if len(phases) != 2 || phases[0].Kind != ConsentReversibleLocal || !phases[0].ApprovalRequired ||
		len(phases[0].Actions) != 1 || phases[0].Actions[0].ID != "instruction:guide" ||
		phases[1].Kind != ConsentHostFollowUp || phases[1].ApprovalRequired || phases[1].Actions[0].Description != "reload host" {
		t.Fatalf("consent phases/actions = %+v", phases)
	}
	if first.Applicable() || len(first.Blockers()) != 1 || first.Blockers()[0].Kind != BlockerOwnership ||
		!reflect.DeepEqual(first.PendingHumanActions(), []string{"reload host"}) || first.Readiness() != (ReadinessStatus{}) {
		t.Fatalf("policy contract = applicable:%v blockers:%+v pending:%v readiness:%+v", first.Applicable(), first.Blockers(), first.PendingHumanActions(), first.Readiness())
	}
	if len(adapter.calls) != 2 || len(adapter.applied) != 0 || len(store.saves) != 0 || resolver.calls != 0 || len(executor.actions) != 0 || !reflect.DeepEqual(store.state, before) {
		t.Fatalf("inspection caused effects: calls=%d apply=%d saves=%d resolver=%d commands=%d state=%+v", len(adapter.calls), len(adapter.applied), len(store.saves), resolver.calls, len(executor.actions), store.state)
	}
	for _, path := range []string{home, configHome} {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("inspection wrote sandboxed path %s: %+v", path, entries)
		}
	}
}

func TestSurfaceInspectionContractPreservesPreflightVerificationRecoveryAndReadiness(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "catalog"}}}
	pending := ActivationObservation{Revision: "host-1", Projections: []ObservedProjection{{ID: "instruction:guide", ObservedFingerprint: "missing", DesiredFingerprint: "catalog", Action: ProjectionAction{ID: "instruction:guide", Description: "write guide"}}}}
	verified := ActivationObservation{Revision: "host-2", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "catalog", DesiredFingerprint: "catalog", Action: ProjectionAction{ID: "instruction:guide", Description: "write guide"}}}}

	t.Run("stale preflight executes no effects", func(t *testing.T) {
		changed := pending
		changed.Revision = "host-changed"
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{pending, changed}}
		facade, store := contractFacade([]Pack{pack}, ActivationState{}, adapter)
		plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
		if !errors.Is(err, ErrStalePlan) || len(adapter.calls) != 2 || len(adapter.applied) != 0 || len(store.saves) != 0 {
			t.Fatalf("stale preflight err=%v calls=%d applied=%d saves=%d", err, len(adapter.calls), len(adapter.applied), len(store.saves))
		}
	})

	t.Run("apply verifies freshly and returns normalized readiness", func(t *testing.T) {
		adapter := &completeSurfaceContractAdapter{
			observations: []ActivationObservation{pending, pending, verified},
			readiness:    []ReadinessObservation{{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, PendingHumanActions: []string{"reload host"}}},
		}
		facade, store := contractFacade([]Pack{pack}, ActivationState{}, adapter)
		plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Verified || result.PlanID != plan.ID() || result.Readiness != (ReadinessStatus{Configured: true, Authorized: true}) || !reflect.DeepEqual(result.PendingHumanActions, []string{"reload host"}) {
			t.Fatalf("apply result = %+v", result)
		}
		if len(adapter.calls) != 3 || len(adapter.applied) != 1 || len(adapter.applied[0]) != 1 || adapter.applied[0][0].ID != "instruction:guide" || adapter.readinessCalls != 1 {
			t.Fatalf("inspection/application calls = inspect:%+v applied:%+v readiness:%d", adapter.calls, adapter.applied, adapter.readinessCalls)
		}
		if store.state.Journal != nil || len(store.state.Ownership) != 1 || store.state.Ownership[0].Fingerprint != "catalog" {
			t.Fatalf("verified state = %+v", store.state)
		}
	})

	t.Run("recovery remains bound to historical attempt", func(t *testing.T) {
		history := ApplyingJournal{PlanID: "plan-old", PlanDigest: "digest-old", Operation: OperationActivate, Surface: SurfaceCodex, PackID: "app", Outcome: AttemptRecoveryRequired, FailedAction: "reversible-local", FailureDetail: "interrupted"}
		state := ActivationState{Intent: activeIntent("app", "1", 6), Journal: &history, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "catalog"}}}
		adapter := &completeSurfaceContractAdapter{observations: []ActivationObservation{verified}}
		facade, store := contractFacade([]Pack{pack}, state, adapter)
		plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Recovery() || plan.ID() == history.PlanID || !reflect.DeepEqual(plan.HistoricalAttempt(), &history) || len(plan.Phases()) != 1 || plan.Phases()[0].Kind != ConsentReversibleLocal {
			t.Fatalf("recovery plan = %+v historical=%+v", plan, plan.HistoricalAttempt())
		}
		result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
		if err != nil || !result.Verified || result.PlanID != plan.ID() {
			t.Fatalf("recovery result=%+v err=%v", result, err)
		}
		if len(adapter.calls) != 3 || len(adapter.applied) != 0 || store.state.Journal != nil || len(store.state.History) != 1 || store.state.History[0].PlanID != history.PlanID || len(store.state.LastAttempts) != 1 || store.state.LastAttempts[0].Outcome != AttemptVerified {
			t.Fatalf("recovery verification: inspect=%d applied=%d state=%+v", len(adapter.calls), len(adapter.applied), store.state)
		}
	})

	t.Run("status observes readiness freshly without mutation", func(t *testing.T) {
		state := ActivationState{Intent: activeIntent("app", "1", 3), Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "catalog"}}}
		adapter := &completeSurfaceContractAdapter{
			observations: []ActivationObservation{verified},
			readiness:    []ReadinessObservation{{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true, Evidence: []string{"runtime loaded"}}},
		}
		facade, store := contractFacade([]Pack{pack}, state, adapter)
		report, err := facade.Status(context.Background(), StatusRequest{PackID: "app", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		entry := report.Entries[0]
		if entry.Readiness != (ReadinessStatus{Configured: true, Authorized: true, Usable: true}) || entry.Projections.Verified != 1 || !reflect.DeepEqual(entry.Evidence, []string{"instruction:guide: verified observed=catalog desired=catalog target=", "runtime loaded"}) {
			t.Fatalf("status entry = %+v", entry)
		}
		if len(adapter.calls) != 1 || adapter.readinessCalls != 1 || len(adapter.applied) != 0 || len(store.saves) != 0 {
			t.Fatalf("status mutation = inspect:%d readiness:%d applied:%d saves:%d", len(adapter.calls), adapter.readinessCalls, len(adapter.applied), len(store.saves))
		}
	})
}
