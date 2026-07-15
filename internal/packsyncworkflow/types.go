// Package packsyncworkflow owns the manual synchronization operation around
// packsync. It owns dispatch, retry, publication, artifact, and readiness
// policy, but never candidate admission, compatibility floors, plans, Apply,
// or Recover.
package packsyncworkflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Selector string

const (
	SelectorLatestStable Selector = "latest-stable"
	SelectorPrerelease   Selector = "prerelease"
	SelectorCommit       Selector = "commit"
)

type ClassificationMode string

const (
	ClassificationAI    ClassificationMode = "ai"
	ClassificationHuman ClassificationMode = "human"
)

type HumanDispatchPhase string

const (
	HumanInspect  HumanDispatchPhase = "inspection"
	HumanEvidence HumanDispatchPhase = "evidence"
)

type DispatchRequest struct {
	SchemaVersion      int                `json:"schema_version"`
	SourceID           string             `json:"source_id"`
	Selector           Selector           `json:"selector"`
	SelectorRef        string             `json:"selector_ref,omitempty"`
	ClassificationMode ClassificationMode `json:"classification_mode"`
	RequestReason      string             `json:"request_reason"`
	RetryOfRun         string             `json:"retry_of_run,omitempty"`
	ExpectedPlanID     string             `json:"expected_plan_id,omitempty"`
	ExpectedBaseSHA    string             `json:"expected_base_sha,omitempty"`
	HumanEvidence      json.RawMessage    `json:"human_evidence,omitempty"`
}

type ValidationArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	SourceID      string `json:"source_id"`
	PlanID        string `json:"plan_id"`
	BaseSHA       string `json:"base_sha"`
	CandidateSHA  string `json:"candidate_sha"`
	MattySuite    bool   `json:"matty_suite"`
	Apply         bool   `json:"apply"`
	UpstreamBytes bool   `json:"contains_upstream_bytes"`
}

func (artifact ValidationArtifact) Validate() error {
	if artifact.SchemaVersion != 1 || !sourceIDPattern.MatchString(artifact.SourceID) || artifact.PlanID == "" || requireFullSHA("base", artifact.BaseSHA) != nil || requireFullSHA("candidate", artifact.CandidateSHA) != nil || !artifact.MattySuite || !artifact.Apply || artifact.UpstreamBytes {
		return errors.New("sandbox validation proof is incomplete or contradictory")
	}
	return nil
}

var (
	sourceIDPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	fullSHAPattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	prereleasePattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z][0-9A-Za-z.-]*$`)
	runIDPattern      = regexp.MustCompile(`^[0-9]+$`)
)

func (request DispatchRequest) Validate() error {
	if request.SchemaVersion != 1 || !sourceIDPattern.MatchString(request.SourceID) || strings.TrimSpace(request.RequestReason) == "" || utf8.RuneCountInString(request.RequestReason) > 500 {
		return errors.New("dispatch schema, source id, and request reason are required")
	}
	switch request.Selector {
	case SelectorLatestStable:
		if request.SelectorRef != "" {
			return errors.New("latest-stable forbids a selector ref")
		}
	case SelectorPrerelease:
		if !prereleasePattern.MatchString(request.SelectorRef) {
			return errors.New("prerelease selector requires one exact published prerelease tag")
		}
	case SelectorCommit:
		if !fullSHAPattern.MatchString(request.SelectorRef) {
			return errors.New("commit selector requires one full lowercase commit SHA")
		}
	default:
		return errors.New("selector is not supported")
	}
	if request.ClassificationMode != ClassificationAI && request.ClassificationMode != ClassificationHuman {
		return errors.New("classification mode must be explicitly ai or human")
	}
	if request.RetryOfRun != "" && !runIDPattern.MatchString(request.RetryOfRun) {
		return errors.New("retry_of_run must identify one prior numeric workflow run")
	}
	hasHumanBinding := request.ExpectedPlanID != "" || request.ExpectedBaseSHA != "" || len(request.HumanEvidence) != 0
	if request.ClassificationMode == ClassificationAI && hasHumanBinding {
		return errors.New("AI dispatch contradicts human evidence binding")
	}
	if hasHumanBinding {
		if request.ClassificationMode != ClassificationHuman || request.Selector != SelectorCommit || request.ExpectedPlanID == "" || !fullSHAPattern.MatchString(request.ExpectedBaseSHA) || len(request.HumanEvidence) == 0 || !json.Valid(request.HumanEvidence) {
			return errors.New("human evidence dispatch requires exact commit, plan, base, and canonical evidence")
		}
	}
	return nil
}

func (request DispatchRequest) HumanPhase() (HumanDispatchPhase, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	if request.ClassificationMode != ClassificationHuman {
		return "", errors.New("human phase requires explicit human classification")
	}
	if len(request.HumanEvidence) == 0 {
		return HumanInspect, nil
	}
	return HumanEvidence, nil
}

type FailureArtifact struct {
	SchemaVersion         int      `json:"schema_version"`
	State                 string   `json:"state"`
	SourceID              string   `json:"source_id"`
	PlanID                string   `json:"plan_id,omitempty"`
	BaseSHA               string   `json:"base_sha,omitempty"`
	CandidateSHA          string   `json:"candidate_sha,omitempty"`
	Blockers              []string `json:"blockers"`
	Recovery              []string `json:"recovery"`
	RunURL                string   `json:"run_url,omitempty"`
	ContainsSecrets       bool     `json:"contains_secrets"`
	ContainsUpstreamBytes bool     `json:"contains_upstream_bytes"`
}

func (artifact FailureArtifact) CanonicalJSON() ([]byte, error) {
	if artifact.SchemaVersion != 1 || artifact.State == "" || artifact.SourceID == "" || len(artifact.Blockers) == 0 || len(artifact.Recovery) == 0 {
		return nil, errors.New("operational artifact is incomplete")
	}
	if artifact.ContainsSecrets || artifact.ContainsUpstreamBytes {
		return nil, errors.New("operational artifacts must not contain secrets or upstream bytes")
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err := json.Indent(&compact, data, "", "  "); err != nil {
		return nil, err
	}
	compact.WriteByte('\n')
	return compact.Bytes(), nil
}

func requireFullSHA(name, value string) error {
	if !fullSHAPattern.MatchString(value) {
		return fmt.Errorf("%s must be one full lowercase SHA", name)
	}
	return nil
}
