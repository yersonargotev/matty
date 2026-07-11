package capabilitypack

import (
	"context"
	"reflect"
	"testing"
)

type fakeSurfaceInspector struct {
	calls []string
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
