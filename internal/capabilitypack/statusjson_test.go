package capabilitypack

import (
	"reflect"
	"testing"
)

func TestStatusJSONDistinguishesObservedFalseFromUnknownAndSorts(t *testing.T) {
	knownFalse := StatusEntry{Pack: Pack{ID: "z", Version: "1"}, Surface: SurfaceCodex, IntentPresent: true, Intent: IntentStatus{Revision: 2}, ReadinessObserved: ReadinessObservationStatus{Configured: true, Authorization: true}, Blockers: []string{"z", "a"}, Evidence: nil, PendingHumanActions: []string{"reload", "login"}}
	unknown := StatusEntry{Pack: Pack{ID: "a", Version: "1"}, Surface: SurfaceOpenCode, ReadinessObserved: ReadinessObservationStatus{Configured: true}}
	report := (StatusReport{Entries: []StatusEntry{knownFalse, unknown}}).JSONReport(false)
	if report.Entries[0].Pack != "a" || report.Entries[1].Pack != "z" {
		t.Fatalf("entries not sorted: %#v", report.Entries)
	}
	entry := report.Entries[1]
	if entry.Intent.State != "known" || entry.Intent.Active == nil || *entry.Intent.Active || entry.Readiness.Authorized.State != "known" || entry.Readiness.Authorized.Value == nil || *entry.Readiness.Authorized.Value {
		t.Fatalf("observed false lost: %#v", entry)
	}
	if entry.Readiness.Usable.State != "unknown" || entry.Readiness.Usable.Value != nil {
		t.Fatalf("unknown lost: %#v", entry.Readiness.Usable)
	}
	if !reflect.DeepEqual(entry.Blockers, []string{"a", "z"}) || !reflect.DeepEqual(entry.PendingHumanActions, []string{"login", "reload"}) || entry.Evidence == nil {
		t.Fatalf("arrays not deterministic/non-null: %#v", entry)
	}
}

func TestStatusJSONNormalizesUnknownAttemptOutcome(t *testing.T) {
	report := (StatusReport{Entries: []StatusEntry{{Pack: Pack{ID: "matty"}, Surface: SurfaceCodex, LatestAttempt: &AttemptStatus{Outcome: "future-value", PlanID: "plan-1"}}}}).JSONReport(true)
	if got := report.Entries[0].LatestAttempt; got == nil || got.Outcome != "unknown" || got.PlanID != "plan-1" {
		t.Fatalf("attempt = %#v", got)
	}
}
