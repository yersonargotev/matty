package governancedrift

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var (
	now  = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	shaA = strings.Repeat("a", 40)
	shaB = strings.Repeat("b", 40)
)

func value(t *testing.T, raw string) SanitizedValue {
	t.Helper()
	v, err := NewSanitizedValue([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func fixture(t *testing.T) (Contract, Observation) {
	t.Helper()
	contract := Contract{SchemaVersion: 1, Controls: []Control{
		{ID: "branch-protection", Boundaries: []Boundary{BoundaryPromotion}, Expected: value(t, `{"required":true,"actors":["owner"]}`)},
		{ID: "release-environment", Boundaries: []Boundary{BoundaryPromotion, BoundaryPublication}, Expected: value(t, `{"reviewers":1}`)},
	}}
	observation := Observation{SchemaVersion: 1, Identity: EvidenceIdentity{
		Repository: "yersonargotev/packy", Ref: "refs/heads/main", CommitSHA: shaA, WorkflowSHA: shaB, CollectedAt: now,
	}, Controls: []ObservedControl{
		{ID: "branch-protection", State: ObservationObserved, Actual: value(t, `{"actors":["owner"],"required":true}`)},
		{ID: "release-environment", State: ObservationObserved, Actual: value(t, `{"reviewers":1}`)},
	}}
	return contract, observation
}

func TestEvaluateStates(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Observation)
		want    EvaluationState
		control string
	}{
		{"clean", func(*Observation) {}, StateClean, ""},
		{"confirmed drift", func(o *Observation) { o.Controls[0].Actual = value(t, `{"required":false}`) }, StateConfirmedDrift, "branch-protection"},
		{"unclassifiable", func(o *Observation) {
			o.Controls[1] = ObservedControl{ID: "release-environment", State: ObservationUnclassifiable, Detail: "API shape changed"}
		}, StateUnclassifiableDrift, "release-environment"},
		{"collection failure", func(o *Observation) { o.Controls = o.Controls[:1] }, StateCollectionFailure, "release-environment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, o := fixture(t)
			tt.mutate(&o)
			got, err := Evaluate(c, o)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tt.want {
				t.Fatalf("state=%q want %q", got.State, tt.want)
			}
			if tt.control != "" && (len(got.Findings) != 1 || got.Findings[0].ControlID != tt.control) {
				t.Fatalf("findings=%+v", got.Findings)
			}
		})
	}
}

func TestGateAffectedAndUnaffectedPaths(t *testing.T) {
	c, o := fixture(t)
	o.Controls[0].Actual = value(t, `{"required":false}`)
	e, err := Evaluate(c, o)
	if err != nil {
		t.Fatal(err)
	}
	base := GateRequest{Repository: o.Identity.Repository, Ref: o.Identity.Ref, CommitSHA: shaA, WorkflowSHA: shaB, Now: now.Add(time.Minute), MaxAge: time.Hour, Evaluations: []Evaluation{e}}
	base.Boundary = BoundaryPromotion
	if got := Gate(base); got.Allowed {
		t.Fatalf("affected promotion allowed: %+v", got)
	}
	base.Boundary = BoundaryPublication
	if got := Gate(base); !got.Allowed {
		t.Fatalf("unaffected publication blocked: %+v", got)
	}
}

func TestGateFailsClosedOnStaleOrMissingEvidence(t *testing.T) {
	c, o := fixture(t)
	e, err := Evaluate(c, o)
	if err != nil {
		t.Fatal(err)
	}
	request := GateRequest{Boundary: BoundaryPublication, Repository: o.Identity.Repository, Ref: o.Identity.Ref, CommitSHA: shaA, WorkflowSHA: shaB, Now: now.Add(2 * time.Hour), MaxAge: time.Hour, Evaluations: []Evaluation{e}}
	if got := Gate(request); got.Allowed || !strings.Contains(strings.Join(got.Reasons, ","), "stale") {
		t.Fatalf("stale gate=%+v", got)
	}
	request.Evaluations = nil
	if got := Gate(request); got.Allowed || !strings.Contains(strings.Join(got.Reasons, ","), "missing") {
		t.Fatalf("missing gate=%+v", got)
	}
}

func TestGateFailsClosedOnMalformedEvaluation(t *testing.T) {
	_, o := fixture(t)
	base := GateRequest{Boundary: BoundaryPromotion, Repository: o.Identity.Repository, Ref: o.Identity.Ref, CommitSHA: shaA, WorkflowSHA: shaB, Now: now.Add(time.Minute), MaxAge: time.Hour}
	tests := []Evaluation{
		{Identity: o.Identity, State: "forged"},
		{Identity: o.Identity, State: StateClean, Findings: []Finding{{ControlID: "x", State: StateConfirmedDrift, Boundaries: []Boundary{BoundaryPromotion}}}},
		{Identity: o.Identity, State: StateConfirmedDrift},
		{Identity: o.Identity, State: StateConfirmedDrift, Findings: []Finding{{ControlID: "x", State: StateConfirmedDrift, Boundaries: []Boundary{"unknown"}}}},
	}
	for _, evaluation := range tests {
		base.Evaluations = []Evaluation{evaluation}
		if got := Gate(base); got.Allowed || !strings.Contains(strings.Join(got.Reasons, ","), "invalid") && !strings.Contains(strings.Join(got.Reasons, ","), "no findings") {
			t.Fatalf("malformed evaluation allowed: %+v => %+v", evaluation, got)
		}
	}
}

func TestIssueLifecycleDeduplicatesAndResolves(t *testing.T) {
	c, o := fixture(t)
	o.Controls[0].Actual = value(t, `false`)
	e, err := Evaluate(c, o)
	if err != nil {
		t.Fatal(err)
	}
	create, err := DecideIssue(IssueRequest{CanonicalKey: "governance-drift", Evaluation: e})
	if err != nil {
		t.Fatal(err)
	}
	if create.Action != IssueCreate {
		t.Fatalf("create=%+v", create)
	}
	existing := []ExistingIssue{{Number: 9, CanonicalKey: "governance-drift", Open: true, EvidenceDigest: create.EvidenceDigest}, {Number: 4, CanonicalKey: "governance-drift", Open: true, EvidenceDigest: create.EvidenceDigest}}
	dedupe, err := DecideIssue(IssueRequest{CanonicalKey: "governance-drift", Evaluation: e, Existing: existing})
	if err != nil {
		t.Fatal(err)
	}
	if dedupe.Action != IssueDeduplicate || dedupe.PrimaryNumber != 4 || len(dedupe.CloseNumbers) != 1 || dedupe.CloseNumbers[0] != 9 {
		t.Fatalf("dedupe=%+v", dedupe)
	}
	_, clean := fixture(t)
	cleanEval, err := Evaluate(c, clean)
	if err != nil {
		t.Fatal(err)
	}
	resolve, err := DecideIssue(IssueRequest{CanonicalKey: "governance-drift", Evaluation: cleanEval, Existing: existing})
	if err != nil {
		t.Fatal(err)
	}
	if resolve.Action != IssueResolve || len(resolve.CloseNumbers) != 2 {
		t.Fatalf("resolve=%+v", resolve)
	}
}

func TestCanonicalJSONAndValidation(t *testing.T) {
	a := value(t, `{"z":2,"a":1}`)
	b := value(t, " { \"a\": 1, \"z\": 2 } ")
	if a != b {
		t.Fatalf("canonical values differ: %q %q", a, b)
	}
	raw, err := json.Marshal(struct {
		Value SanitizedValue `json:"value"`
	}{a})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"value":{"a":1,"z":2}}` {
		t.Fatalf("json=%s", raw)
	}
	c, o := fixture(t)
	o.Identity.CommitSHA = "abc"
	if _, err := Evaluate(c, o); err == nil {
		t.Fatal("short SHA accepted")
	}
	o.Identity.CommitSHA = shaA
	o.SchemaVersion = 2
	if _, err := Evaluate(c, o); err == nil {
		t.Fatal("unknown schema accepted")
	}
}
