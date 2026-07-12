package capabilitypack

import (
	"context"
	"reflect"
	"testing"
)

type fakeReadinessInspector struct {
	observations []ReadinessObservation
	calls        int
}

func TestExternallyManagedProjectionIsVerifiedWithoutMattyOwnership(t *testing.T) {
	projection := ObservedProjection{ID: "external_setup:engram:codex:mcp", Exists: true, ObservedFingerprint: "ready", DesiredFingerprint: "ready", ExternallyManaged: true}
	details, summary := deriveProjectionStatus("engram", []ObservedProjection{projection}, nil, composition{})
	if len(details) != 1 || details[0].Health != ProjectionVerified || summary.Verified != 1 || summary.Unmanaged != 0 {
		t.Fatalf("details=%+v summary=%+v", details, summary)
	}
}

func (f *fakeReadinessInspector) InspectReadiness(context.Context, Pack, ActivationObservation, []ExecutableResolution) (ReadinessObservation, error) {
	i := f.calls
	f.calls++
	if i >= len(f.observations) {
		i = len(f.observations) - 1
	}
	return f.observations[i], nil
}

type fakeSurfaceInspector struct {
	calls []string
}

func TestStatusDerivesReadinessFreshlyAndNormalizesInconsistentEvidence(t *testing.T) {
	pack := Pack{ID: "engram", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "memory"}}}
	projection := ObservedProjection{ID: "instruction:memory", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:memory", Target: "/codex/AGENTS.md"}}
	adapter := &fakeActivationAdapter{observations: []ActivationObservation{{Projections: []ObservedProjection{projection}}, {Projections: []ObservedProjection{projection}}, {Projections: []ObservedProjection{projection}}}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:memory", Fingerprint: "same", Contributors: []string{"engram"}}}}}
	readiness := &fakeReadinessInspector{observations: []ReadinessObservation{
		{AuthorizationObserved: true, PendingHumanActions: []string{"login to Codex"}},
		{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, PendingHumanActions: []string{"reload Codex"}},
		{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true, Evidence: []string{"runtime loaded memory capability"}},
	}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, nil, WithActivation(store, map[Surface]ActivationAdapter{SurfaceCodex: adapter}), WithReadinessInspectors(map[Surface]ReadinessInspector{SurfaceCodex: readiness}))
	wants := []ReadinessStatus{{Configured: true}, {Configured: true, Authorized: true}, {Configured: true, Authorized: true, Usable: true}}
	for i, want := range wants {
		report, err := facade.Status(context.Background(), StatusRequest{PackID: "engram", Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		if report.Entries[0].Readiness != want {
			t.Fatalf("call %d readiness=%+v want=%+v", i, report.Entries[0].Readiness, want)
		}
	}
	if adapter.inspectCalls != 3 || readiness.calls != 3 || len(store.saves) != 0 {
		t.Fatalf("status was not fresh/pure: adapter=%d readiness=%d saves=%d", adapter.inspectCalls, readiness.calls, len(store.saves))
	}
}

func TestStatusRejectsOwnershipProblemsAndDoesNotInventRecoveryReadiness(t *testing.T) {
	pack := Pack{ID: "engram", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "memory"}}}
	projection := ObservedProjection{ID: "instruction:memory", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:memory"}}
	adapter := &fakeActivationAdapter{observations: []ActivationObservation{{Projections: []ObservedProjection{projection}}}}
	journal := &ApplyingJournal{PlanID: "failed", PackID: "engram", Surface: SurfaceCodex, Outcome: AttemptRecoveryRequired}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Active: true, Revision: 2}, Journal: journal, Ownership: []ProjectionOwnership{{ID: "instruction:memory", Fingerprint: "same", Contributors: []string{"wrong"}}}}}
	ready := &fakeReadinessInspector{observations: []ReadinessObservation{{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, nil, WithActivation(store, map[Surface]ActivationAdapter{SurfaceCodex: adapter}), WithReadinessInspectors(map[Surface]ReadinessInspector{SurfaceCodex: ready}))
	report, err := facade.Status(context.Background(), StatusRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if entry.Readiness != (ReadinessStatus{}) || entry.Projections.Ambiguous != 1 || entry.LatestAttempt == nil || entry.LatestAttempt.Outcome != "recovery-required" {
		t.Fatalf("entry=%+v", entry)
	}
}

func TestStatusConfiguredDoesNotDependOnIntentAndLatestAttemptIsPairSpecific(t *testing.T) {
	pack := Pack{ID: "matty", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
	projection := ObservedProjection{ID: "instruction:guide", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}
	adapter := &fakeActivationAdapter{observations: []ActivationObservation{{Projections: []ObservedProjection{projection}}}}
	store := &fakeActivationStore{state: ActivationState{Ownership: []ProjectionOwnership{{ID: "instruction:guide", Fingerprint: "same", Contributors: []string{"matty"}}}, LastAttempts: []ApplyingJournal{{PlanID: "other", PackID: "engram", Surface: SurfaceCodex, Outcome: AttemptVerified}, {PlanID: "matty-plan", PackID: "matty", Surface: SurfaceCodex, Outcome: AttemptVerified}}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, nil, WithActivation(store, map[Surface]ActivationAdapter{SurfaceCodex: adapter}))
	report, err := facade.Status(context.Background(), StatusRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if !entry.Readiness.Configured || entry.Intent.Active || entry.LatestAttempt == nil || entry.LatestAttempt.PlanID != "matty-plan" {
		t.Fatalf("entry=%+v", entry)
	}
}

func (f *fakeSurfaceInspector) Inspect(_ context.Context, pack Pack) (SurfaceObservation, error) {
	f.calls = append(f.calls, pack.ID)
	return SurfaceObservation{Inspected: true}, nil
}

func TestStatusInspectsEveryPackSurfaceAndReportsInactiveBaseline(t *testing.T) {
	catalog := Catalog{packs: []Pack{
		{ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
		{ID: "matty", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
	}}
	codex := &fakeSurfaceInspector{}
	opencode := &fakeSurfaceInspector{}
	facade := NewFacade(catalog, map[Surface]SurfaceInspector{SurfaceCodex: codex, SurfaceOpenCode: opencode})

	report, err := facade.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 4 {
		t.Fatalf("entries = %d, want 4", len(report.Entries))
	}
	for _, entry := range report.Entries {
		if entry.Intent.Active || entry.Intent.Revision != 0 || entry.LatestAttempt != nil {
			t.Fatalf("invented lifecycle state: %+v", entry)
		}
		if entry.Readiness.Configured || entry.Readiness.Authorized || entry.Readiness.Usable {
			t.Fatalf("invented readiness: %+v", entry.Readiness)
		}
		if entry.Projections != (ProjectionSummary{}) || len(entry.PendingHumanActions) != 0 {
			t.Fatalf("invented ownership or pending actions: %+v", entry)
		}
		if !entry.Observation.Inspected {
			t.Fatalf("missing fresh adapter observation: %+v", entry)
		}
	}
	if !reflect.DeepEqual(codex.calls, []string{"engram", "matty"}) || !reflect.DeepEqual(opencode.calls, []string{"engram", "matty"}) {
		t.Fatalf("inspection calls: codex=%v opencode=%v", codex.calls, opencode.calls)
	}
}

func TestStatusTargetsOnePackAndSurface(t *testing.T) {
	catalog := Catalog{packs: []Pack{{ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}}}}
	codex := &fakeSurfaceInspector{}
	opencode := &fakeSurfaceInspector{}
	facade := NewFacade(catalog, map[Surface]SurfaceInspector{SurfaceCodex: codex, SurfaceOpenCode: opencode})

	report, err := facade.Status(context.Background(), StatusRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 1 || report.Entries[0].Pack.ID != "engram" || report.Entries[0].Surface != SurfaceCodex {
		t.Fatalf("report = %+v", report)
	}
	if !reflect.DeepEqual(codex.calls, []string{"engram"}) || len(opencode.calls) != 0 {
		t.Fatalf("inspection calls: codex=%v opencode=%v", codex.calls, opencode.calls)
	}
}
