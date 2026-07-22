// Package release models the immutable evidence required to publish a Packy
// release. It deliberately contains no GitHub or filesystem behavior.
package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	versionPattern = regexp.MustCompile(`^v0\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Permission is one effective GitHub Actions permission and its access level.
type Permission struct {
	Name   string `json:"name"`
	Access string `json:"access"`
}

// Subject is one release artifact bound by its SHA-256 digest.
type Subject struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// Authorization is the exact workflow authority allowed to create a candidate.
type Authorization struct {
	Repository  string
	Ref         string
	Workflow    string
	WorkflowSHA string
	Permissions []Permission
}

// Candidate is an immutable release identity. ID is derived from every other
// field and is stable regardless of input slice ordering.
type Candidate struct {
	ID          string       `json:"id"`
	Version     string       `json:"version"`
	Repository  string       `json:"repository"`
	Ref         string       `json:"ref"`
	Commit      string       `json:"commit"`
	Workflow    string       `json:"workflow"`
	WorkflowSHA string       `json:"workflow_sha"`
	Permissions []Permission `json:"permissions"`
	Subjects    []Subject    `json:"subjects"`
}

// NewCandidate validates exact authority and returns a canonical immutable
// candidate. Authority and observed values must match, including permissions.
func NewCandidate(version, repository, ref, commit, workflow, workflowSHA string, permissions []Permission, subjects []Subject, authorized Authorization) (Candidate, error) {
	if !versionPattern.MatchString(version) {
		return Candidate{}, errors.New("release version must have form v0.x.y")
	}
	if repository == "" || repository != authorized.Repository {
		return Candidate{}, errors.New("repository is not authorized")
	}
	if ref == "" || ref != authorized.Ref {
		return Candidate{}, errors.New("ref is not authorized")
	}
	if !commitPattern.MatchString(commit) {
		return Candidate{}, errors.New("commit must be one full lowercase 40-character SHA")
	}
	if workflow == "" || workflow != authorized.Workflow {
		return Candidate{}, errors.New("workflow is not authorized")
	}
	if !digestPattern.MatchString(workflowSHA) || workflowSHA != authorized.WorkflowSHA {
		return Candidate{}, errors.New("workflow SHA-256 digest is not authorized")
	}
	canonicalPermissions, err := canonicalizePermissions(permissions)
	if err != nil {
		return Candidate{}, err
	}
	expectedPermissions, err := canonicalizePermissions(authorized.Permissions)
	if err != nil {
		return Candidate{}, fmt.Errorf("invalid authorization: %w", err)
	}
	if !equalPermissions(canonicalPermissions, expectedPermissions) {
		return Candidate{}, errors.New("effective permissions do not match authorization")
	}
	canonicalSubjects, err := canonicalizeSubjects(subjects)
	if err != nil {
		return Candidate{}, err
	}
	candidate := Candidate{Version: version, Repository: repository, Ref: ref, Commit: commit, Workflow: workflow, WorkflowSHA: workflowSHA, Permissions: canonicalPermissions, Subjects: canonicalSubjects}
	candidate.ID = identityDigest(candidate)
	return candidate, nil
}

func canonicalizePermissions(values []Permission) ([]Permission, error) {
	if len(values) == 0 {
		return nil, errors.New("effective permissions are required")
	}
	result := append([]Permission(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	for i, permission := range result {
		if permission.Name == "" || (permission.Access != "read" && permission.Access != "write" && permission.Access != "none") {
			return nil, errors.New("effective permission has an invalid name or access")
		}
		if i > 0 && permission.Name == result[i-1].Name {
			return nil, fmt.Errorf("duplicate effective permission %q", permission.Name)
		}
	}
	return result, nil
}

func canonicalizeSubjects(values []Subject) ([]Subject, error) {
	if len(values) == 0 {
		return nil, errors.New("artifact subjects are required")
	}
	result := append([]Subject(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	for i, subject := range result {
		if subject.Name == "" || subject.Name == "." || subject.Name == ".." || strings.ContainsAny(subject.Name, `/\\`) || !digestPattern.MatchString(subject.SHA256) {
			return nil, errors.New("artifact subject has an invalid name or SHA-256 digest")
		}
		if i > 0 && subject.Name == result[i-1].Name {
			return nil, fmt.Errorf("duplicate artifact subject %q", subject.Name)
		}
	}
	return result, nil
}

func identityDigest(candidate Candidate) string {
	candidate.ID = ""
	data, _ := json.Marshal(candidate)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func equalPermissions(a, b []Permission) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalSubjects(a, b []Subject) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Provenance is offline-verifiable evidence for exactly one candidate.
type Provenance struct {
	CandidateID string       `json:"candidate_id"`
	Version     string       `json:"version"`
	Repository  string       `json:"repository"`
	Ref         string       `json:"ref"`
	Commit      string       `json:"commit"`
	Workflow    string       `json:"workflow"`
	WorkflowSHA string       `json:"workflow_sha"`
	Permissions []Permission `json:"permissions"`
	Subjects    []Subject    `json:"subjects"`
}

// ProvenanceFor creates a detached copy of a candidate's evidence.
func ProvenanceFor(candidate Candidate) Provenance {
	return Provenance{CandidateID: candidate.ID, Version: candidate.Version, Repository: candidate.Repository, Ref: candidate.Ref, Commit: candidate.Commit, Workflow: candidate.Workflow, WorkflowSHA: candidate.WorkflowSHA, Permissions: append([]Permission(nil), candidate.Permissions...), Subjects: append([]Subject(nil), candidate.Subjects...)}
}

// VerifyProvenance fails closed unless evidence binds every candidate field and
// exactly the same artifact subject set.
func VerifyProvenance(candidate Candidate, provenance Provenance) error {
	permissions, err := canonicalizePermissions(provenance.Permissions)
	if err != nil {
		return fmt.Errorf("invalid provenance permissions: %w", err)
	}
	subjects, err := canonicalizeSubjects(provenance.Subjects)
	if err != nil {
		return fmt.Errorf("invalid provenance subjects: %w", err)
	}
	if candidate.ID == "" || candidate.ID != identityDigest(candidate) {
		return errors.New("candidate identity is stale")
	}
	if provenance.CandidateID != candidate.ID || provenance.Version != candidate.Version || provenance.Repository != candidate.Repository || provenance.Ref != candidate.Ref || provenance.Commit != candidate.Commit || provenance.Workflow != candidate.Workflow || provenance.WorkflowSHA != candidate.WorkflowSHA || !equalPermissions(permissions, candidate.Permissions) || !equalSubjects(subjects, candidate.Subjects) {
		return errors.New("provenance does not bind the exact candidate")
	}
	return nil
}

// Release is an observed GitHub release projected into the pure domain model.
type Release struct {
	Version     string
	CandidateID string
	Draft       bool
	Assets      []Subject
}

// Lifecycle is the only safe next action for an exact observed release.
type Lifecycle string

const (
	ResumeDraft       Lifecycle = "resume-draft"
	PublishDraft      Lifecycle = "publish-draft"
	ContinuePublished Lifecycle = "continue-published"
)

// LifecycleDecision describes a read-only verification result.
type LifecycleDecision struct {
	Lifecycle Lifecycle
	Missing   []Subject
}

// VerifyLifecycle verifies existing state without mutating candidates,
// releases, or their slices. An absent release is a resumable draft missing all
// assets; adapters may then create it.
func VerifyLifecycle(candidate Candidate, releases []Release) (LifecycleDecision, error) {
	if candidate.ID == "" || candidate.ID != identityDigest(candidate) {
		return LifecycleDecision{}, errors.New("candidate identity is stale")
	}
	var matching []Release
	for _, release := range releases {
		if release.Version == candidate.Version {
			matching = append(matching, release)
		}
	}
	if len(matching) > 1 {
		return LifecycleDecision{}, errors.New("release state is ambiguous or duplicated")
	}
	if len(matching) == 0 {
		return LifecycleDecision{Lifecycle: ResumeDraft, Missing: append([]Subject(nil), candidate.Subjects...)}, nil
	}
	release := matching[0]
	if release.CandidateID != candidate.ID {
		return LifecycleDecision{}, errors.New("same-version release has a divergent candidate identity")
	}
	assets, err := canonicalizeOptionalSubjects(release.Assets)
	if err != nil {
		return LifecycleDecision{}, err
	}
	missing, err := compareAssets(candidate.Subjects, assets)
	if err != nil {
		return LifecycleDecision{}, err
	}
	if release.Draft {
		if len(missing) > 0 {
			return LifecycleDecision{Lifecycle: ResumeDraft, Missing: missing}, nil
		}
		return LifecycleDecision{Lifecycle: PublishDraft}, nil
	}
	if len(missing) > 0 {
		return LifecycleDecision{}, errors.New("published release is incomplete")
	}
	return LifecycleDecision{Lifecycle: ContinuePublished}, nil
}

// VerifyDraftPreparation admits only states from which a draft can safely be
// created, resumed, or published. An already-published release is deliberately
// rejected so callers cannot accidentally treat continuation as preparation.
func VerifyDraftPreparation(candidate Candidate, releases []Release) (LifecycleDecision, error) {
	decision, err := VerifyLifecycle(candidate, releases)
	if err != nil {
		return LifecycleDecision{}, err
	}
	if decision.Lifecycle == ContinuePublished {
		return LifecycleDecision{}, errors.New("release is already published; draft preparation is forbidden")
	}
	return decision, nil
}

// VerifyPublishedContinuation admits only a complete, exact published release
// for read-back and downstream Homebrew continuation.
func VerifyPublishedContinuation(candidate Candidate, releases []Release) error {
	decision, err := VerifyLifecycle(candidate, releases)
	if err != nil {
		return err
	}
	if decision.Lifecycle != ContinuePublished {
		return errors.New("an exact complete published release is required for continuation")
	}
	return nil
}

func canonicalizeOptionalSubjects(values []Subject) ([]Subject, error) {
	if len(values) == 0 {
		return []Subject{}, nil
	}
	return canonicalizeSubjects(values)
}

func compareAssets(expected, actual []Subject) ([]Subject, error) {
	byName := make(map[string]string, len(actual))
	for _, subject := range actual {
		byName[subject.Name] = subject.SHA256
	}
	for _, subject := range actual {
		found := false
		for _, want := range expected {
			if want.Name == subject.Name {
				found = true
				if want.SHA256 != subject.SHA256 {
					return nil, fmt.Errorf("asset %q has a stale or mismatched digest", subject.Name)
				}
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unexpected asset %q", subject.Name)
		}
	}
	var missing []Subject
	for _, want := range expected {
		if _, ok := byName[want.Name]; !ok {
			missing = append(missing, want)
		}
	}
	return missing, nil
}
