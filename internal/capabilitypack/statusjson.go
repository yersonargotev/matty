package capabilitypack

import "sort"

const StatusSchemaVersion = 1

type JSONOptionalBool struct {
	State string `json:"state"`
	Value *bool  `json:"value"`
}

type JSONIntent struct {
	State    string `json:"state"`
	Active   *bool  `json:"active"`
	Revision *int   `json:"revision"`
}

type JSONAttempt struct {
	Outcome string `json:"outcome"`
	PlanID  string `json:"plan_id"`
}

type JSONReadiness struct {
	Configured JSONOptionalBool `json:"configured"`
	Authorized JSONOptionalBool `json:"authorized"`
	Usable     JSONOptionalBool `json:"usable"`
}

type JSONProjectionSummary struct {
	Verified  int `json:"verified"`
	Missing   int `json:"missing"`
	Drifted   int `json:"drifted"`
	Ambiguous int `json:"ambiguous"`
	Unmanaged int `json:"unmanaged"`
}

type JSONStatusEntry struct {
	Pack                string                `json:"pack"`
	PackVersion         string                `json:"pack_version"`
	Surface             Surface               `json:"surface"`
	Intent              JSONIntent            `json:"intent"`
	LatestAttempt       *JSONAttempt          `json:"latest_attempt"`
	Projections         JSONProjectionSummary `json:"projection_summary"`
	Readiness           JSONReadiness         `json:"readiness"`
	Blockers            []string              `json:"blockers"`
	Evidence            []string              `json:"evidence"`
	PendingHumanActions []string              `json:"pending_human_actions"`
}

type JSONStatusReport struct {
	SchemaVersion int               `json:"schema_version"`
	Report        string            `json:"report"`
	Entries       []JSONStatusEntry `json:"entries"`
}

func (report StatusReport) JSONReport(targeted bool) JSONStatusReport {
	kind := "pack-status-overview"
	if targeted {
		kind = "pack-status"
	}
	entries := make([]JSONStatusEntry, 0, len(report.Entries))
	for _, entry := range report.Entries {
		intent := JSONIntent{State: "absent"}
		if entry.IntentPresent {
			active, revision := entry.Intent.Active, entry.Intent.Revision
			intent = JSONIntent{State: "known", Active: &active, Revision: &revision}
		}
		var attempt *JSONAttempt
		if entry.LatestAttempt != nil {
			outcome := entry.LatestAttempt.Outcome
			switch AttemptOutcome(outcome) {
			case AttemptApplying, AttemptVerified, AttemptRecoveryRequired:
			default:
				outcome = "unknown"
			}
			attempt = &JSONAttempt{Outcome: outcome, PlanID: entry.LatestAttempt.PlanID}
		}
		entries = append(entries, JSONStatusEntry{
			Pack: entry.Pack.ID, PackVersion: entry.Pack.Version, Surface: entry.Surface,
			Intent: intent, LatestAttempt: attempt, Projections: JSONProjectionSummary{Verified: entry.Projections.Verified, Missing: entry.Projections.Missing, Drifted: entry.Projections.Drifted, Ambiguous: entry.Projections.Ambiguous, Unmanaged: entry.Projections.Unmanaged},
			Readiness: JSONReadiness{optionalBool(entry.ReadinessObserved.Configured, entry.Readiness.Configured), optionalBool(entry.ReadinessObserved.Authorization, entry.Readiness.Authorized), optionalBool(entry.ReadinessObserved.Usability, entry.Readiness.Usable)},
			Blockers:  sortedCopy(entry.Blockers), Evidence: sortedCopy(entry.Evidence), PendingHumanActions: sortedCopy(entry.PendingHumanActions),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Pack != entries[j].Pack {
			return entries[i].Pack < entries[j].Pack
		}
		return entries[i].Surface < entries[j].Surface
	})
	return JSONStatusReport{SchemaVersion: StatusSchemaVersion, Report: kind, Entries: entries}
}

func optionalBool(observed, value bool) JSONOptionalBool {
	if !observed {
		return JSONOptionalBool{State: "unknown", Value: nil}
	}
	v := value
	return JSONOptionalBool{State: "known", Value: &v}
}

func sortedCopy(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}
