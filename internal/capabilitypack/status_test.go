package capabilitypack

import (
	"context"
	"reflect"
	"testing"
)

func resourceStatusInspection(transition SurfaceTransition) SurfaceInspection {
	pack := transition.Desired
	projections := make([]ObservedProjection, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		projection := ObservedProjection{
			Goal:               ProjectionPresent,
			ID:                 resource.Kind + ":" + resource.ID,
			Exists:             true,
			DesiredFingerprint: "healthy",
			Action:             ProjectionAction{ID: resource.Kind + ":" + resource.ID},
		}
		if resource.ID == "engram-memory" {
			projection.ObservedFingerprint = "drifted"
			projection.ExternallyManaged = true
		} else {
			projection.ObservedFingerprint = "healthy"
		}
		projections = append(projections, projection)
	}
	return SurfaceInspection{Projections: projections, Readiness: ReadinessObservation{AuthorizationObserved: true, Authorized: pack.ID == "matty", UsabilityObserved: true, Usable: pack.ID == "matty"}}
}

func TestStatusIsolatesReadinessForTwoActivePacksOnOneSurface(t *testing.T) {
	matty := Pack{ID: "matty", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "matty-guidance"}}}
	engram := Pack{ID: "engram", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "external_setup", ID: "engram-memory"}}}
	store := &fakeActivationStore{state: ActivationState{
		Intents: []ActivationIntent{
			{PackID: "matty", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 1},
			{PackID: "engram", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 2},
		},
		Ownership: []ProjectionOwnership{{ID: "instruction:matty-guidance", Fingerprint: "healthy", Contributors: []string{"matty"}}},
	}}
	facade := NewFacade(
		Catalog{packs: []Pack{engram, matty}},
		WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{inspect: resourceStatusInspection}}),
	)

	directed, err := facade.Status(context.Background(), StatusRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if got := directed.Entries[0]; got.Readiness != (ReadinessStatus{Configured: true, Authorized: true, Usable: true}) || len(got.ProjectionDetails) != 1 || got.ProjectionDetails[0].ID != "instruction:matty-guidance" {
		t.Fatalf("directed Matty status includes unrelated evidence: %+v", got)
	}
	directed, err = facade.Status(context.Background(), StatusRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if got := directed.Entries[0]; got.Readiness != (ReadinessStatus{}) || got.Projections.Drifted != 1 || len(got.Blockers) == 0 {
		t.Fatalf("directed Engram status did not retain its drift and blockers: %+v", got)
	}

	overview, err := facade.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Entries) != 2 {
		t.Fatalf("overview entries=%d want=2", len(overview.Entries))
	}
	engramEntry, mattyEntry := overview.Entries[0], overview.Entries[1]
	if engramEntry.Pack.ID != "engram" || engramEntry.Readiness != (ReadinessStatus{}) || engramEntry.Projections.Drifted != 1 || len(engramEntry.Blockers) == 0 {
		t.Fatalf("Engram status did not retain its drift and blockers: %+v", engramEntry)
	}
	if mattyEntry.Pack.ID != "matty" || mattyEntry.Readiness != (ReadinessStatus{Configured: true, Authorized: true, Usable: true}) || mattyEntry.Projections.Verified != 1 || len(mattyEntry.Blockers) != 0 {
		t.Fatalf("Matty status was degraded by Engram: %+v", mattyEntry)
	}
}

func TestSurfaceWideStatusRetainsSharedProjectionConflicts(t *testing.T) {
	shared := Resource{Kind: "instruction", ID: "shared-guidance"}
	matty := Pack{ID: "matty", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{shared}}
	engram := Pack{ID: "engram", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{shared}}
	store := &fakeActivationStore{state: ActivationState{
		Intents: []ActivationIntent{
			{PackID: "matty", Surface: SurfaceCodex, Version: "1", Active: true},
			{PackID: "engram", Surface: SurfaceCodex, Version: "1", Active: true},
		},
		Ownership: []ProjectionOwnership{{ID: "instruction:shared-guidance", Fingerprint: "healthy", Contributors: []string{"matty"}}},
	}}
	facade := NewFacade(Catalog{packs: []Pack{engram, matty}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{inspect: resourceStatusInspection}}))

	report, err := facade.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range report.Entries {
		if entry.Projections.Ambiguous != 1 || len(entry.Blockers) == 0 || !reflect.DeepEqual(entry.ProjectionDetails[0].Contributors, []string{"engram", "matty"}) {
			t.Fatalf("shared conflict hidden for %s: %+v", entry.Pack.ID, entry)
		}
	}
}

func TestExternallyManagedProjectionIsVerifiedWithoutMattyOwnership(t *testing.T) {
	projection := ObservedProjection{Goal: ProjectionPresent, ID: "external_setup:engram:codex:mcp", Exists: true, ObservedFingerprint: "ready", DesiredFingerprint: "ready", ExternallyManaged: true}
	details, summary := deriveProjectionStatus("engram", []ObservedProjection{projection}, nil, composition{})
	if len(details) != 1 || details[0].Health != ProjectionVerified || summary.Verified != 1 || summary.Unmanaged != 0 {
		t.Fatalf("details=%+v summary=%+v", details, summary)
	}
}

func TestStatusDerivesReadinessFreshlyAndNormalizesInconsistentEvidence(t *testing.T) {
	pack := Pack{ID: "engram", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "memory"}}}
	projection := ObservedProjection{Goal: ProjectionPresent, ID: "instruction:memory", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:memory", Target: "/codex/AGENTS.md"}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Projections: []ObservedProjection{projection}}, {Projections: []ObservedProjection{projection}}, {Projections: []ObservedProjection{projection}}}}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:memory", Fingerprint: "same", Contributors: []string{"engram"}}}}}
	readiness := []ReadinessObservation{
		{AuthorizationObserved: true, PendingHumanActions: []string{"login to Codex"}},
		{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, PendingHumanActions: []string{"reload Codex"}},
		{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true, Evidence: []string{"runtime loaded memory capability"}},
	}
	for i := range adapter.observations {
		adapter.observations[i].Readiness = readiness[i]
	}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
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
	if adapter.inspectCalls != 3 || len(store.saves) != 0 {
		t.Fatalf("status was not fresh/pure: adapter=%d saves=%d", adapter.inspectCalls, len(store.saves))
	}
}

func TestStatusRejectsOwnershipProblemsAndDoesNotInventRecoveryReadiness(t *testing.T) {
	pack := Pack{ID: "engram", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "memory"}}}
	projection := ObservedProjection{Goal: ProjectionPresent, ID: "instruction:memory", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:memory"}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Projections: []ObservedProjection{projection}}}}
	journal := &ApplyingJournal{PlanID: "failed", PackID: "engram", Surface: SurfaceCodex, Outcome: AttemptRecoveryRequired}
	store := &fakeActivationStore{state: ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Active: true, Revision: 2}, Journal: journal, Ownership: []ProjectionOwnership{{ID: "instruction:memory", Fingerprint: "same", Contributors: []string{"wrong"}}}}}
	adapter.observations[0].Readiness = ReadinessObservation{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
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
	projection := ObservedProjection{Goal: ProjectionPresent, ID: "instruction:guide", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Projections: []ObservedProjection{projection}}}}
	store := &fakeActivationStore{state: ActivationState{Ownership: []ProjectionOwnership{{ID: "instruction:guide", Fingerprint: "same", Contributors: []string{"matty"}}}, LastAttempts: []ApplyingJournal{{PlanID: "other", PackID: "engram", Surface: SurfaceCodex, Outcome: AttemptVerified}, {PlanID: "matty-plan", PackID: "matty", Surface: SurfaceCodex, Outcome: AttemptVerified}}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	report, err := facade.Status(context.Background(), StatusRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if !entry.Readiness.Configured || entry.Intent.Active || entry.LatestAttempt == nil || entry.LatestAttempt.PlanID != "matty-plan" {
		t.Fatalf("entry=%+v", entry)
	}
}

func TestStatusInspectsEveryPackSurfaceAndReportsInactiveBaseline(t *testing.T) {
	catalog := Catalog{packs: []Pack{
		{ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
		{ID: "matty", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
	}}
	codex := &fakeSurfaceAdapter{}
	opencode := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: codex, SurfaceOpenCode: opencode}))

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
	}
	if !reflect.DeepEqual(desiredPackIDs(codex.calls), []string{"engram", "matty"}) || !reflect.DeepEqual(desiredPackIDs(opencode.calls), []string{"engram", "matty"}) {
		t.Fatalf("inspection calls: codex=%v opencode=%v", codex.calls, opencode.calls)
	}
}

func TestStatusTargetsOnePackAndSurface(t *testing.T) {
	catalog := Catalog{packs: []Pack{{ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}}}}
	codex := &fakeSurfaceAdapter{}
	opencode := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: codex, SurfaceOpenCode: opencode}))

	report, err := facade.Status(context.Background(), StatusRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 1 || report.Entries[0].Pack.ID != "engram" || report.Entries[0].Surface != SurfaceCodex {
		t.Fatalf("report = %+v", report)
	}
	if !reflect.DeepEqual(desiredPackIDs(codex.calls), []string{"engram"}) || len(opencode.calls) != 0 {
		t.Fatalf("inspection calls: codex=%v opencode=%v", codex.calls, opencode.calls)
	}
}

func desiredPackIDs(calls []surfaceInspectionCall) []string {
	ids := make([]string, len(calls))
	for i, call := range calls {
		ids[i] = call.desired.ID
	}
	return ids
}
