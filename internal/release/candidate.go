// Package release models the immutable evidence required to publish a Packy
// release. It deliberately contains no GitHub or filesystem behavior.
package release

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const (
	PackyRepository      = "yersonargotev/packy"
	PackyMainRef         = "refs/heads/main"
	PackyReleaseWorkflow = ".github/workflows/release.yml"
	ChecksumsName        = "SHA256SUMS"
	SBOMName             = "sbom.spdx.json"
)

var (
	versionPattern     = regexp.MustCompile(`^v0\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	commitPattern      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	allowedPermissions = map[string]string{
		"actions": "read", "attestations": "write", "contents": "write", "id-token": "write",
	}
)

type Permission struct {
	Name   string `json:"name"`
	Access string `json:"access"`
}

type Subject struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// Observation is the complete, caller-observed input to candidate validation.
// SHA256SUMS and SBOM are bytes so their contents can be verified offline.
type Observation struct {
	Version            string
	Repository         string
	Ref                string
	Commit             string
	Workflow           string
	WorkflowSHA        string
	ReleaseNotesSHA256 string
	Permissions        []Permission
	Subjects           []Subject
	SHA256SUMS         []byte
	SBOM               []byte
}

type Candidate struct {
	ID                 string       `json:"id"`
	Version            string       `json:"version"`
	Repository         string       `json:"repository"`
	Ref                string       `json:"ref"`
	Commit             string       `json:"commit"`
	Workflow           string       `json:"workflow"`
	WorkflowSHA        string       `json:"workflow_sha"`
	ReleaseNotesSHA256 string       `json:"release_notes_sha256"`
	Permissions        []Permission `json:"permissions"`
	Subjects           []Subject    `json:"subjects"`
}

// NewCandidate enforces Packy's fixed release policy and validates all retained
// metadata artifacts before deriving a canonical identity.
func NewCandidate(observed Observation) (Candidate, error) {
	if !versionPattern.MatchString(observed.Version) {
		return Candidate{}, errors.New("release version must have form v0.x.y")
	}
	if observed.Repository != PackyRepository {
		return Candidate{}, errors.New("repository is not Packy's authorized repository")
	}
	if observed.Ref != PackyMainRef {
		return Candidate{}, errors.New("release ref must be Packy's protected main ref")
	}
	if !commitPattern.MatchString(observed.Commit) {
		return Candidate{}, errors.New("commit must be one full lowercase 40-character SHA")
	}
	if observed.Workflow != PackyReleaseWorkflow {
		return Candidate{}, errors.New("workflow is not Packy's release workflow")
	}
	if !digestPattern.MatchString(observed.WorkflowSHA) {
		return Candidate{}, errors.New("workflow SHA-256 digest is invalid")
	}
	if !digestPattern.MatchString(observed.ReleaseNotesSHA256) {
		return Candidate{}, errors.New("release notes SHA-256 digest is invalid")
	}
	permissions, err := canonicalizePermissions(observed.Permissions)
	if err != nil {
		return Candidate{}, err
	}
	subjects, err := canonicalizeSubjects(observed.Subjects)
	if err != nil {
		return Candidate{}, err
	}
	if err := verifyMetadataSubjects(subjects, observed.SHA256SUMS, observed.SBOM, observed.Version); err != nil {
		return Candidate{}, err
	}
	candidate := Candidate{
		Version: observed.Version, Repository: observed.Repository, Ref: observed.Ref,
		Commit: observed.Commit, Workflow: observed.Workflow, WorkflowSHA: observed.WorkflowSHA,
		ReleaseNotesSHA256: observed.ReleaseNotesSHA256, Permissions: permissions, Subjects: subjects,
	}
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
		ceiling, ok := allowedPermissions[permission.Name]
		if !ok || permission.Access == "none" || (permission.Access != "read" && permission.Access != "write") {
			return nil, fmt.Errorf("effective permission %q is not allowed", permission.Name)
		}
		if ceiling == "read" && permission.Access != "read" {
			return nil, fmt.Errorf("effective permission %q exceeds Packy's policy", permission.Name)
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
		if subject.Name == "" || subject.Name == "." || subject.Name == ".." ||
			strings.ContainsAny(subject.Name, `/\`) || !digestPattern.MatchString(subject.SHA256) {
			return nil, errors.New("artifact subject has an invalid name or SHA-256 digest")
		}
		if i > 0 && subject.Name == result[i-1].Name {
			return nil, fmt.Errorf("duplicate artifact subject %q", subject.Name)
		}
	}
	return result, nil
}

func verifyMetadataSubjects(subjects []Subject, checksums, sbom []byte, version string) error {
	byName := make(map[string]string, len(subjects))
	for _, subject := range subjects {
		byName[subject.Name] = subject.SHA256
	}
	for name, content := range map[string][]byte{ChecksumsName: checksums, SBOMName: sbom} {
		want, ok := byName[name]
		if !ok {
			return fmt.Errorf("required artifact subject %q is missing", name)
		}
		got := sha256.Sum256(content)
		if hex.EncodeToString(got[:]) != want {
			return fmt.Errorf("%s content does not match its subject digest", name)
		}
	}
	if err := VerifySHA256SUMS(checksums, subjects); err != nil {
		return err
	}
	if err := VerifySPDXSBOM(sbom, version, subjects); err != nil {
		return err
	}
	return nil
}

// VerifySHA256SUMS verifies a strict two-space SHA256SUMS manifest. It must
// contain every retained subject except the manifest itself exactly once.
func VerifySHA256SUMS(content []byte, subjects []Subject) error {
	if len(content) == 0 || content[len(content)-1] != '\n' {
		return errors.New("SHA256SUMS must be non-empty and newline-terminated")
	}
	expected := make(map[string]string, len(subjects)-1)
	for _, subject := range subjects {
		if subject.Name != ChecksumsName {
			expected[subject.Name] = subject.SHA256
		}
	}
	seen := make(map[string]bool, len(expected))
	lastName := ""
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			return errors.New("SHA256SUMS contains a blank line")
		}
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || !digestPattern.MatchString(parts[0]) || parts[1] == "" || strings.ContainsAny(parts[1], `/\`) {
			return errors.New("SHA256SUMS contains a malformed entry")
		}
		want, ok := expected[parts[1]]
		if !ok {
			return fmt.Errorf("SHA256SUMS contains unexpected subject %q", parts[1])
		}
		if seen[parts[1]] {
			return fmt.Errorf("SHA256SUMS duplicates subject %q", parts[1])
		}
		if lastName != "" && parts[1] <= lastName {
			return errors.New("SHA256SUMS subjects are not in canonical name order")
		}
		if want != parts[0] {
			return fmt.Errorf("SHA256SUMS digest mismatch for %q", parts[1])
		}
		seen[parts[1]] = true
		lastName = parts[1]
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read SHA256SUMS: %w", err)
	}
	for name := range expected {
		if !seen[name] {
			return fmt.Errorf("SHA256SUMS is missing subject %q", name)
		}
	}
	return nil
}

type spdxDocument struct {
	SPDXVersion       string `json:"spdxVersion"`
	Name              string `json:"name"`
	DocumentNamespace string `json:"documentNamespace"`
	Files             []struct {
		FileName  string `json:"fileName"`
		Checksums []struct {
			Algorithm string `json:"algorithm"`
			Value     string `json:"checksumValue"`
		} `json:"checksums"`
	} `json:"files"`
}

// VerifySPDXSBOM validates a minimal SPDX 2.3 inventory. It must describe every
// non-metadata retained subject exactly once with the exact SHA-256.
func VerifySPDXSBOM(content []byte, version string, subjects []Subject) error {
	var document spdxDocument
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("decode SPDX SBOM: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("SPDX SBOM contains trailing JSON")
	}
	if document.SPDXVersion != "SPDX-2.3" || document.Name != "packy-"+version ||
		document.DocumentNamespace == "" || !strings.Contains(document.DocumentNamespace, version) {
		return errors.New("SPDX SBOM has stale or mismatched document identity")
	}
	expected := make(map[string]string)
	for _, subject := range subjects {
		if subject.Name != ChecksumsName && subject.Name != SBOMName {
			expected[subject.Name] = subject.SHA256
		}
	}
	seen := make(map[string]bool, len(expected))
	for _, file := range document.Files {
		want, ok := expected[file.FileName]
		if !ok {
			return fmt.Errorf("SPDX SBOM contains unexpected subject %q", file.FileName)
		}
		if seen[file.FileName] {
			return fmt.Errorf("SPDX SBOM duplicates subject %q", file.FileName)
		}
		var got string
		for _, checksum := range file.Checksums {
			if checksum.Algorithm == "SHA256" {
				if got != "" {
					return fmt.Errorf("SPDX SBOM duplicates SHA256 for %q", file.FileName)
				}
				got = checksum.Value
			}
		}
		if got == "" || got != want {
			return fmt.Errorf("SPDX SBOM digest mismatch for %q", file.FileName)
		}
		seen[file.FileName] = true
	}
	for name := range expected {
		if !seen[name] {
			return fmt.Errorf("SPDX SBOM is missing subject %q", name)
		}
	}
	return nil
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

type Provenance struct {
	CandidateID        string       `json:"candidate_id"`
	Version            string       `json:"version"`
	Repository         string       `json:"repository"`
	Ref                string       `json:"ref"`
	Commit             string       `json:"commit"`
	Workflow           string       `json:"workflow"`
	WorkflowSHA        string       `json:"workflow_sha"`
	ReleaseNotesSHA256 string       `json:"release_notes_sha256"`
	Permissions        []Permission `json:"permissions"`
	Subjects           []Subject    `json:"subjects"`
}

func ProvenanceFor(candidate Candidate) Provenance {
	return Provenance{
		CandidateID: candidate.ID, Version: candidate.Version, Repository: candidate.Repository,
		Ref: candidate.Ref, Commit: candidate.Commit, Workflow: candidate.Workflow,
		WorkflowSHA: candidate.WorkflowSHA, ReleaseNotesSHA256: candidate.ReleaseNotesSHA256,
		Permissions: append([]Permission(nil), candidate.Permissions...),
		Subjects:    append([]Subject(nil), candidate.Subjects...),
	}
}

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
	if provenance.CandidateID != candidate.ID || provenance.Version != candidate.Version ||
		provenance.Repository != candidate.Repository || provenance.Ref != candidate.Ref ||
		provenance.Commit != candidate.Commit || provenance.Workflow != candidate.Workflow ||
		provenance.WorkflowSHA != candidate.WorkflowSHA ||
		provenance.ReleaseNotesSHA256 != candidate.ReleaseNotesSHA256 ||
		!equalPermissions(permissions, candidate.Permissions) || !equalSubjects(subjects, candidate.Subjects) {
		return errors.New("provenance does not bind the exact candidate")
	}
	return nil
}

// Release is a complete read-only projection of the GitHub release metadata.
type Release struct {
	Version               string
	CandidateID           string
	ProvenanceCandidateID string
	Repository            string
	Ref                   string
	TargetCommit          string
	Workflow              string
	WorkflowSHA           string
	ReleaseNotesSHA256    string
	Draft                 bool
	Assets                []Subject
}

type Lifecycle string

const (
	ResumeDraft       Lifecycle = "resume-draft"
	PublishDraft      Lifecycle = "publish-draft"
	ContinuePublished Lifecycle = "continue-published"
)

type LifecycleDecision struct {
	Lifecycle Lifecycle
	Missing   []Subject
}

func VerifyLifecycle(candidate Candidate, releases []Release) (LifecycleDecision, error) {
	if candidate.ID == "" || candidate.ID != identityDigest(candidate) {
		return LifecycleDecision{}, errors.New("candidate identity is stale")
	}
	var matching []Release
	for _, observed := range releases {
		if observed.Version == candidate.Version {
			matching = append(matching, observed)
		}
	}
	if len(matching) > 1 {
		return LifecycleDecision{}, errors.New("release state is ambiguous or duplicated")
	}
	if len(matching) == 0 {
		return LifecycleDecision{Lifecycle: ResumeDraft, Missing: append([]Subject(nil), candidate.Subjects...)}, nil
	}
	observed := matching[0]
	if observed.CandidateID != candidate.ID || observed.ProvenanceCandidateID != candidate.ID || observed.Repository != candidate.Repository ||
		observed.Ref != candidate.Ref || observed.TargetCommit != candidate.Commit ||
		observed.Workflow != candidate.Workflow || observed.WorkflowSHA != candidate.WorkflowSHA ||
		observed.ReleaseNotesSHA256 != candidate.ReleaseNotesSHA256 {
		return LifecycleDecision{}, errors.New("same-version release has divergent identity metadata")
	}
	assets, err := canonicalizeOptionalSubjects(observed.Assets)
	if err != nil {
		return LifecycleDecision{}, err
	}
	missing, err := compareAssets(candidate.Subjects, assets)
	if err != nil {
		return LifecycleDecision{}, err
	}
	if observed.Draft {
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
