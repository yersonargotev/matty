// Package governancedrift evaluates read-only governance observations and makes
// fail-closed gate and issue-lifecycle decisions. It deliberately has no write
// or self-correction capability.
package governancedrift

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	ContractSchemaVersion    = 1
	ObservationSchemaVersion = 1
)

type Boundary string

const (
	BoundaryPromotion   Boundary = "promotion"
	BoundaryPublication Boundary = "publication"
)

type SanitizedValue string

func NewSanitizedValue(raw []byte) (SanitizedValue, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode sanitized JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", errors.New("sanitized JSON must contain exactly one value")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode sanitized JSON: %w", err)
	}
	return SanitizedValue(canonical), nil
}

func (v SanitizedValue) MarshalJSON() ([]byte, error) {
	if _, err := NewSanitizedValue([]byte(v)); err != nil {
		return nil, err
	}
	return []byte(v), nil
}

func (v *SanitizedValue) UnmarshalJSON(raw []byte) error {
	canonical, err := NewSanitizedValue(raw)
	if err != nil {
		return err
	}
	*v = canonical
	return nil
}

type Contract struct {
	SchemaVersion int       `json:"schema_version"`
	Controls      []Control `json:"controls"`
}

type Control struct {
	ID         string         `json:"id"`
	Boundaries []Boundary     `json:"boundaries"`
	Expected   SanitizedValue `json:"expected"`
}

type EvidenceIdentity struct {
	Repository  string    `json:"repository"`
	Ref         string    `json:"ref"`
	CommitSHA   string    `json:"commit_sha"`
	WorkflowSHA string    `json:"workflow_sha"`
	CollectedAt time.Time `json:"collected_at"`
}

type Observation struct {
	SchemaVersion int               `json:"schema_version"`
	Identity      EvidenceIdentity  `json:"identity"`
	Controls      []ObservedControl `json:"controls"`
}

type ObservationState string

const (
	ObservationObserved          ObservationState = "observed"
	ObservationUnclassifiable    ObservationState = "unclassifiable"
	ObservationCollectionFailure ObservationState = "collection-failure"
)

type ObservedControl struct {
	ID     string           `json:"id"`
	State  ObservationState `json:"state"`
	Actual SanitizedValue   `json:"actual,omitempty"`
	Detail string           `json:"detail,omitempty"`
}

type EvaluationState string

const (
	StateClean               EvaluationState = "clean"
	StateConfirmedDrift      EvaluationState = "confirmed-drift"
	StateUnclassifiableDrift EvaluationState = "unclassifiable-drift"
	StateCollectionFailure   EvaluationState = "collection-failure"
)

type Finding struct {
	ControlID  string          `json:"control_id"`
	State      EvaluationState `json:"state"`
	Boundaries []Boundary      `json:"boundaries"`
	Detail     string          `json:"detail,omitempty"`
}

type Evaluation struct {
	Identity EvidenceIdentity `json:"identity"`
	State    EvaluationState  `json:"state"`
	Findings []Finding        `json:"findings"`
}

func Evaluate(contract Contract, observation Observation) (Evaluation, error) {
	if err := validateContract(contract); err != nil {
		return Evaluation{}, err
	}
	if err := validateObservation(observation); err != nil {
		return Evaluation{}, err
	}
	seen := make(map[string]ObservedControl, len(observation.Controls))
	for _, item := range observation.Controls {
		seen[item.ID] = item
	}
	known := make(map[string]bool, len(contract.Controls))
	for _, control := range contract.Controls {
		known[control.ID] = true
	}
	for id := range seen {
		if !known[id] {
			return Evaluation{}, fmt.Errorf("observation contains unknown control %q", id)
		}
	}
	result := Evaluation{Identity: observation.Identity, State: StateClean, Findings: []Finding{}}
	for _, control := range contract.Controls {
		item, ok := seen[control.ID]
		finding := Finding{ControlID: control.ID, Boundaries: append([]Boundary(nil), control.Boundaries...)}
		switch {
		case !ok || item.State == ObservationCollectionFailure:
			finding.State = StateCollectionFailure
			if ok {
				finding.Detail = item.Detail
			} else {
				finding.Detail = "control was not collected"
			}
		case item.State == ObservationUnclassifiable:
			finding.State, finding.Detail = StateUnclassifiableDrift, item.Detail
		case item.Actual != control.Expected:
			finding.State, finding.Detail = StateConfirmedDrift, item.Detail
		default:
			continue
		}
		result.Findings = append(result.Findings, finding)
		result.State = worse(result.State, finding.State)
	}
	for i := range result.Findings {
		sort.Slice(result.Findings[i].Boundaries, func(a, b int) bool {
			return result.Findings[i].Boundaries[a] < result.Findings[i].Boundaries[b]
		})
	}
	sort.Slice(result.Findings, func(i, j int) bool {
		return result.Findings[i].ControlID < result.Findings[j].ControlID
	})
	return result, nil
}

func validateContract(contract Contract) error {
	if contract.SchemaVersion != ContractSchemaVersion {
		return fmt.Errorf("unsupported contract schema version %d", contract.SchemaVersion)
	}
	if len(contract.Controls) == 0 {
		return errors.New("contract has no controls")
	}
	ids := map[string]bool{}
	for _, c := range contract.Controls {
		if strings.TrimSpace(c.ID) == "" || ids[c.ID] {
			return fmt.Errorf("invalid or duplicate control %q", c.ID)
		}
		ids[c.ID] = true
		if _, err := NewSanitizedValue([]byte(c.Expected)); err != nil {
			return fmt.Errorf("control %s expected value: %w", c.ID, err)
		}
		if len(c.Boundaries) == 0 {
			return fmt.Errorf("control %s has no affected boundary", c.ID)
		}
		boundaries := map[Boundary]bool{}
		for _, b := range c.Boundaries {
			if !validBoundary(b) || boundaries[b] {
				return fmt.Errorf("control %s has invalid boundary %q", c.ID, b)
			}
			boundaries[b] = true
		}
	}
	return nil
}

func validateObservation(observation Observation) error {
	if observation.SchemaVersion != ObservationSchemaVersion {
		return fmt.Errorf("unsupported observation schema version %d", observation.SchemaVersion)
	}
	if err := observation.Identity.validate(); err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, c := range observation.Controls {
		if strings.TrimSpace(c.ID) == "" || ids[c.ID] {
			return fmt.Errorf("invalid or duplicate observed control %q", c.ID)
		}
		ids[c.ID] = true
		switch c.State {
		case ObservationObserved:
			if _, err := NewSanitizedValue([]byte(c.Actual)); err != nil {
				return fmt.Errorf("control %s actual value: %w", c.ID, err)
			}
		case ObservationUnclassifiable, ObservationCollectionFailure:
			if strings.TrimSpace(c.Detail) == "" {
				return fmt.Errorf("control %s state %s requires detail", c.ID, c.State)
			}
		default:
			return fmt.Errorf("control %s has invalid observation state %q", c.ID, c.State)
		}
	}
	return nil
}

func (i EvidenceIdentity) validate() error {
	if strings.TrimSpace(i.Repository) == "" || strings.TrimSpace(i.Ref) == "" {
		return errors.New("evidence repository and ref are required")
	}
	if !fullSHA(i.CommitSHA) || !fullSHA(i.WorkflowSHA) {
		return errors.New("evidence commit_sha and workflow_sha must be 40 or 64 lowercase hexadecimal characters")
	}
	if i.CollectedAt.IsZero() {
		return errors.New("evidence collection time is required")
	}
	return nil
}
func fullSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && s == strings.ToLower(s)
}
func validBoundary(b Boundary) bool { return b == BoundaryPromotion || b == BoundaryPublication }
func worse(a, b EvaluationState) EvaluationState {
	rank := map[EvaluationState]int{StateClean: 0, StateConfirmedDrift: 1, StateUnclassifiableDrift: 2, StateCollectionFailure: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

type GateRequest struct {
	Boundary    Boundary
	Repository  string
	Ref         string
	CommitSHA   string
	WorkflowSHA string
	Now         time.Time
	MaxAge      time.Duration
	Evaluations []Evaluation
}
type GateDecision struct {
	Allowed bool     `json:"allowed"`
	Reasons []string `json:"reasons"`
}

func Gate(request GateRequest) GateDecision {
	reasons := []string{}
	if !validBoundary(request.Boundary) || request.Now.IsZero() || request.MaxAge <= 0 || !fullSHA(request.CommitSHA) || !fullSHA(request.WorkflowSHA) {
		return GateDecision{Reasons: []string{"invalid gate request"}}
	}
	matched := false
	for _, evaluation := range request.Evaluations {
		i := evaluation.Identity
		if i.Repository != request.Repository || i.Ref != request.Ref || i.CommitSHA != request.CommitSHA || i.WorkflowSHA != request.WorkflowSHA {
			continue
		}
		matched = true
		if err := validateEvaluation(evaluation); err != nil {
			reasons = append(reasons, "matching evidence is invalid: "+err.Error())
			continue
		}
		if i.CollectedAt.After(request.Now) || request.Now.Sub(i.CollectedAt) > request.MaxAge {
			reasons = append(reasons, "matching evidence is stale")
			continue
		}
		for _, f := range evaluation.Findings {
			if affects(f.Boundaries, request.Boundary) {
				reasons = append(reasons, fmt.Sprintf("%s: %s", f.ControlID, f.State))
			}
		}
	}
	if !matched {
		reasons = append(reasons, "matching evidence is missing")
	}
	sort.Strings(reasons)
	return GateDecision{Allowed: len(reasons) == 0, Reasons: reasons}
}

func validateEvaluation(evaluation Evaluation) error {
	if err := evaluation.Identity.validate(); err != nil {
		return err
	}
	switch evaluation.State {
	case StateClean:
		if len(evaluation.Findings) != 0 {
			return errors.New("clean evidence contains findings")
		}
		return nil
	case StateConfirmedDrift, StateUnclassifiableDrift, StateCollectionFailure:
	default:
		return fmt.Errorf("unknown evaluation state %q", evaluation.State)
	}
	if len(evaluation.Findings) == 0 {
		return errors.New("non-clean evidence has no findings")
	}
	overall := StateClean
	controls := map[string]bool{}
	for _, finding := range evaluation.Findings {
		if strings.TrimSpace(finding.ControlID) == "" || controls[finding.ControlID] {
			return fmt.Errorf("invalid or duplicate finding control %q", finding.ControlID)
		}
		controls[finding.ControlID] = true
		switch finding.State {
		case StateConfirmedDrift, StateUnclassifiableDrift, StateCollectionFailure:
		default:
			return fmt.Errorf("finding %s has invalid state %q", finding.ControlID, finding.State)
		}
		if len(finding.Boundaries) == 0 {
			return fmt.Errorf("finding %s has no affected boundary", finding.ControlID)
		}
		seen := map[Boundary]bool{}
		for _, boundary := range finding.Boundaries {
			if !validBoundary(boundary) || seen[boundary] {
				return fmt.Errorf("finding %s has invalid boundary %q", finding.ControlID, boundary)
			}
			seen[boundary] = true
		}
		overall = worse(overall, finding.State)
	}
	if overall != evaluation.State {
		return fmt.Errorf("evaluation state %q does not summarize findings (%q)", evaluation.State, overall)
	}
	return nil
}

func affects(boundaries []Boundary, target Boundary) bool {
	for _, b := range boundaries {
		if b == target {
			return true
		}
	}
	return false
}

type ExistingIssue struct {
	Number         int    `json:"number"`
	CanonicalKey   string `json:"canonical_key"`
	Open           bool   `json:"open"`
	EvidenceDigest string `json:"evidence_digest"`
}
type IssueRequest struct {
	CanonicalKey string
	Evaluation   Evaluation
	Existing     []ExistingIssue
}
type IssueAction string

const (
	IssueNoop        IssueAction = "noop"
	IssueCreate      IssueAction = "create"
	IssueUpdate      IssueAction = "update"
	IssueDeduplicate IssueAction = "deduplicate"
	IssueResolve     IssueAction = "resolve"
)

type IssueDecision struct {
	Action         IssueAction `json:"action"`
	PrimaryNumber  int         `json:"primary_number,omitempty"`
	CloseNumbers   []int       `json:"close_numbers"`
	EvidenceDigest string      `json:"evidence_digest"`
}

func DecideIssue(request IssueRequest) (IssueDecision, error) {
	if strings.TrimSpace(request.CanonicalKey) == "" {
		return IssueDecision{}, errors.New("canonical issue key is required")
	}
	digest, err := evaluationDigest(request.Evaluation)
	if err != nil {
		return IssueDecision{}, err
	}
	open := []ExistingIssue{}
	for _, issue := range request.Existing {
		if issue.Open && issue.CanonicalKey == request.CanonicalKey {
			open = append(open, issue)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Number < open[j].Number })
	decision := IssueDecision{Action: IssueNoop, CloseNumbers: []int{}, EvidenceDigest: digest}
	if request.Evaluation.State == StateClean {
		if len(open) > 0 {
			decision.Action = IssueResolve
			for _, i := range open {
				decision.CloseNumbers = append(decision.CloseNumbers, i.Number)
			}
		}
		return decision, nil
	}
	if len(open) == 0 {
		decision.Action = IssueCreate
		return decision, nil
	}
	decision.PrimaryNumber = open[0].Number
	for _, i := range open[1:] {
		decision.CloseNumbers = append(decision.CloseNumbers, i.Number)
	}
	if len(decision.CloseNumbers) > 0 {
		decision.Action = IssueDeduplicate
	} else if open[0].EvidenceDigest != digest {
		decision.Action = IssueUpdate
	}
	return decision, nil
}
func evaluationDigest(e Evaluation) (string, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
