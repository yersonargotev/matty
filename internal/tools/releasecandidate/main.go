// Command releasecandidate is the private filesystem adapter for Packy's pure
// release candidate and publication-state domain.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/release"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type stringList []string

var errDestinationExists = errors.New("destination already exists")

func (v *stringList) String() string     { return strings.Join(*v, ",") }
func (v *stringList) Set(s string) error { *v = append(*v, s); return nil }

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("command is required: create, verify-provenance, or verify-state")
	}
	switch args[0] {
	case "create":
		return runCreate(args[1:], stdout)
	case "verify-provenance":
		return runVerifyProvenance(args[1:], stdout)
	case "verify-state":
		return runVerifyState(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runCreate(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("releasecandidate create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var version, repository, ref, commit, workflow, workflowSHA, notesPath, distDir, outputDir string
	var permissions stringList
	flags.StringVar(&version, "version", "", "release version")
	flags.StringVar(&repository, "repository", "", "repository")
	flags.StringVar(&ref, "ref", "", "source ref")
	flags.StringVar(&commit, "commit", "", "source commit")
	flags.StringVar(&workflow, "workflow", "", "workflow path")
	flags.StringVar(&workflowSHA, "workflow-sha", "", "workflow SHA-256")
	flags.StringVar(&notesPath, "release-notes", "", "release notes path")
	flags.StringVar(&distDir, "dist", "", "retained artifacts directory")
	flags.StringVar(&outputDir, "output-dir", "", "metadata output directory")
	flags.Var(&permissions, "permission", "effective permission name=read|write (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if notesPath == "" || distDir == "" || outputDir == "" {
		return errors.New("release-notes, dist, and output-dir are required")
	}
	canonicalNotes, canonicalDist, canonicalOutput, err := validateCreatePaths(notesPath, distDir, outputDir)
	if err != nil {
		return err
	}
	parsedPermissions, err := parsePermissions(permissions)
	if err != nil {
		return err
	}
	notes, err := readRegularFile(canonicalNotes)
	if err != nil {
		return fmt.Errorf("read release notes: %w", err)
	}
	subjects, contents, err := observeDist(canonicalDist)
	if err != nil {
		return err
	}
	candidate, err := release.NewCandidate(release.Observation{
		Version: version, Repository: repository, Ref: ref, Commit: commit, Workflow: workflow,
		WorkflowSHA: workflowSHA, ReleaseNotesSHA256: digest(notes), Permissions: parsedPermissions,
		Subjects: subjects, SHA256SUMS: contents[release.ChecksumsName], SBOM: contents[release.SBOMName],
	})
	if err != nil {
		return fmt.Errorf("create candidate: %w", err)
	}
	candidateJSON, err := canonicalJSON(candidate)
	if err != nil {
		return err
	}
	provenanceJSON, err := canonicalJSON(release.ProvenanceFor(candidate))
	if err != nil {
		return err
	}
	if err := writePairAtomic(canonicalOutput, candidateJSON, provenanceJSON); err != nil {
		return err
	}
	_, err = stdout.Write(candidateJSON)
	return err
}

func runVerifyProvenance(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("releasecandidate verify-provenance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var candidatePath, provenancePath string
	flags.StringVar(&candidatePath, "candidate", "", "candidate JSON")
	flags.StringVar(&provenancePath, "provenance", "", "provenance JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	candidate, provenance, err := readEvidence(candidatePath, provenancePath)
	if err != nil {
		return err
	}
	if err := release.VerifyProvenance(candidate, provenance); err != nil {
		return err
	}
	_, err = io.WriteString(stdout, "{\"verified\":true}\n")
	return err
}

type releaseState struct {
	CandidateID        string             `json:"candidate_id"`
	Provenance         release.Provenance `json:"provenance"`
	Version            string             `json:"version"`
	Repository         string             `json:"repository"`
	Ref                string             `json:"ref"`
	TargetCommit       string             `json:"target_commit"`
	Workflow           string             `json:"workflow"`
	WorkflowSHA        string             `json:"workflow_sha"`
	ReleaseNotesSHA256 string             `json:"release_notes_sha256"`
	Draft              bool               `json:"draft"`
	Assets             []serverAsset      `json:"assets"`
}
type serverAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type stateOutput struct {
	Decision release.Lifecycle `json:"decision"`
	Missing  []release.Subject `json:"missing_assets"`
}

func runVerifyState(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("releasecandidate verify-state", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var candidatePath, provenancePath, statePath, mode string
	flags.StringVar(&candidatePath, "candidate", "", "candidate JSON")
	flags.StringVar(&provenancePath, "provenance", "", "provenance JSON")
	flags.StringVar(&statePath, "state", "", "normalized release state JSON")
	flags.StringVar(&mode, "mode", "", "draft or published")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	candidate, provenance, err := readEvidence(candidatePath, provenancePath)
	if err != nil {
		return err
	}
	if err := release.VerifyProvenance(candidate, provenance); err != nil {
		return fmt.Errorf("verify provenance: %w", err)
	}
	var state releaseState
	if err := strictReadJSON(statePath, &state, stateSchema); err != nil {
		return fmt.Errorf("read release state: %w", err)
	}
	assets := make([]release.Subject, len(state.Assets))
	for i, asset := range state.Assets {
		if !strings.HasPrefix(asset.Digest, "sha256:") {
			return fmt.Errorf("asset %q digest must use sha256: prefix", asset.Name)
		}
		assets[i] = release.Subject{Name: asset.Name, SHA256: strings.TrimPrefix(asset.Digest, "sha256:")}
	}
	observed := release.Release{Version: state.Version, CandidateID: state.CandidateID, Provenance: state.Provenance,
		Repository: state.Repository, Ref: state.Ref, TargetCommit: state.TargetCommit, Workflow: state.Workflow,
		WorkflowSHA: state.WorkflowSHA, ReleaseNotesSHA256: state.ReleaseNotesSHA256, Draft: state.Draft, Assets: assets}
	var decision release.LifecycleDecision
	switch mode {
	case "draft":
		decision, err = release.VerifyDraftPreparation(candidate, []release.Release{observed})
	case "published":
		err = release.VerifyPublishedContinuation(candidate, []release.Release{observed})
		decision.Lifecycle = release.ContinuePublished
	default:
		return errors.New("mode must be draft or published")
	}
	if err != nil {
		return err
	}
	if decision.Missing == nil {
		decision.Missing = []release.Subject{}
	}
	data, err := canonicalJSON(stateOutput{Decision: decision.Lifecycle, Missing: decision.Missing})
	if err != nil {
		return err
	}
	_, err = stdout.Write(data)
	return err
}

func readEvidence(candidatePath, provenancePath string) (release.Candidate, release.Provenance, error) {
	if candidatePath == "" || provenancePath == "" {
		return release.Candidate{}, release.Provenance{}, errors.New("candidate and provenance are required")
	}
	var candidate release.Candidate
	if err := strictReadJSON(candidatePath, &candidate, candidateSchema); err != nil {
		return candidate, release.Provenance{}, fmt.Errorf("read candidate: %w", err)
	}
	var provenance release.Provenance
	if err := strictReadJSON(provenancePath, &provenance, provenanceSchema); err != nil {
		return candidate, provenance, fmt.Errorf("read provenance: %w", err)
	}
	if err := release.VerifyProvenance(candidate, provenance); err != nil {
		return candidate, provenance, fmt.Errorf("verify provenance: %w", err)
	}
	return candidate, provenance, nil
}

func parsePermissions(values []string) ([]release.Permission, error) {
	result := make([]release.Permission, 0, len(values))
	for _, value := range values {
		name, access, ok := strings.Cut(value, "=")
		if !ok || name == "" || access == "" {
			return nil, fmt.Errorf("invalid permission %q; want name=access", value)
		}
		result = append(result, release.Permission{Name: name, Access: access})
	}
	return result, nil
}
func observeDist(dir string) ([]release.Subject, map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read dist: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil, errors.New("dist is empty")
	}
	subjects := make([]release.Subject, 0, len(entries))
	contents := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			return nil, nil, fmt.Errorf("dist contains hidden entry %q", entry.Name())
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("dist entry %q is not a regular file", entry.Name())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		contents[entry.Name()] = data
		subjects = append(subjects, release.Subject{Name: entry.Name(), SHA256: digest(data)})
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].Name < subjects[j].Name })
	return subjects, contents, nil
}
func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("path is not a regular file")
	}
	return os.ReadFile(path)
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func overlaps(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	rel1, _ := filepath.Rel(aa, bb)
	rel2, _ := filepath.Rel(bb, aa)
	return rel1 == "." || (rel1 != ".." && !strings.HasPrefix(rel1, ".."+string(filepath.Separator))) || (rel2 != ".." && !strings.HasPrefix(rel2, ".."+string(filepath.Separator)))
}
func validateCreatePaths(notesPath, distPath, outputPath string) (string, string, string, error) {
	notes, err := resolveExistingRoot(notesPath, false)
	if err != nil {
		return "", "", "", fmt.Errorf("release-notes: %w", err)
	}
	dist, err := resolveExistingRoot(distPath, true)
	if err != nil {
		return "", "", "", fmt.Errorf("dist: %w", err)
	}
	if _, err := os.Lstat(outputPath); err == nil {
		return "", "", "", errors.New("output-dir already exists")
	} else if !os.IsNotExist(err) {
		return "", "", "", err
	}
	parent, err := resolveExistingRoot(filepath.Dir(outputPath), true)
	if err != nil {
		return "", "", "", fmt.Errorf("output-dir parent: %w", err)
	}
	output := filepath.Join(parent, filepath.Base(filepath.Clean(outputPath)))
	if overlaps(dist, output) || overlaps(notes, output) || overlaps(dist, notes) {
		return "", "", "", errors.New("release-notes, dist, and output-dir must not overlap")
	}
	return notes, dist, output, nil
}
func resolveExistingRoot(path string, directory bool) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("symlink roots are forbidden")
	}
	if directory && !info.IsDir() {
		return "", errors.New("path is not a directory")
	}
	if !directory && !info.Mode().IsRegular() {
		return "", errors.New("path is not a regular file")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	return resolved, nil
}
func canonicalJSON(value any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func writePairAtomic(dir string, candidate, provenance []byte) error {
	if _, err := os.Lstat(dir); err == nil {
		return errors.New("output-dir already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	parent, base := filepath.Dir(dir), filepath.Base(dir)
	stage, err := os.MkdirTemp(parent, "."+base+".staging-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for name, data := range map[string][]byte{"candidate.json": candidate, "provenance.json": provenance} {
		if err := os.WriteFile(filepath.Join(stage, name), data, 0o600); err != nil {
			return err
		}
	}
	if err := renameNoReplace(stage, dir); err != nil {
		if errors.Is(err, errDestinationExists) {
			return errors.New("refusing to overwrite existing output-dir")
		}
		return fmt.Errorf("publish metadata directory: %w", err)
	}
	return nil
}

// jsonShape is a small exact-key/duplicate schema for adapter inputs.
type jsonShape struct {
	keys       map[string]jsonShape
	array      *jsonShape
	scalarType string
}

var stringScalar = jsonShape{scalarType: "string"}
var boolScalar = jsonShape{scalarType: "bool"}
var permissionShape = jsonShape{keys: map[string]jsonShape{"name": stringScalar, "access": stringScalar}}
var subjectShape = jsonShape{keys: map[string]jsonShape{"name": stringScalar, "sha256": stringScalar}}
var candidateSchema = jsonShape{keys: map[string]jsonShape{"id": stringScalar, "version": stringScalar, "repository": stringScalar, "ref": stringScalar, "commit": stringScalar, "workflow": stringScalar, "workflow_sha": stringScalar, "release_notes_sha256": stringScalar, "permissions": {array: &permissionShape}, "subjects": {array: &subjectShape}}}
var provenanceSchema = jsonShape{keys: map[string]jsonShape{"candidate_id": stringScalar, "version": stringScalar, "repository": stringScalar, "ref": stringScalar, "commit": stringScalar, "workflow": stringScalar, "workflow_sha": stringScalar, "release_notes_sha256": stringScalar, "permissions": {array: &permissionShape}, "subjects": {array: &subjectShape}}}
var assetShape = jsonShape{keys: map[string]jsonShape{"name": stringScalar, "digest": stringScalar}}
var stateSchema = jsonShape{keys: map[string]jsonShape{"candidate_id": stringScalar, "provenance": provenanceSchema, "version": stringScalar, "repository": stringScalar, "ref": stringScalar, "target_commit": stringScalar, "workflow": stringScalar, "workflow_sha": stringScalar, "release_notes_sha256": stringScalar, "draft": boolScalar, "assets": {array: &assetShape}}}

func strictReadJSON(path string, out any, schema jsonShape) error {
	data, err := readRegularFile(path)
	if err != nil {
		return err
	}
	if err := validateJSON(bytes.NewReader(data), schema); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	return nil
}
func validateJSON(r io.Reader, schema jsonShape) error {
	dec := json.NewDecoder(r)
	if err := walkJSON(dec, schema); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	return nil
}
func walkJSON(dec *json.Decoder, s jsonShape) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if s.keys != nil {
		d, ok := tok.(json.Delim)
		if !ok || d != '{' {
			return errors.New("expected object")
		}
		seen := map[string]bool{}
		for dec.More() {
			kt, _ := dec.Token()
			key, ok := kt.(string)
			if !ok {
				return errors.New("invalid object key")
			}
			child, ok := s.keys[key]
			if !ok {
				return fmt.Errorf("unknown or incorrectly cased field %q", key)
			}
			if seen[key] {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[key] = true
			if err := walkJSON(dec, child); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		if err == nil && len(seen) != len(s.keys) {
			return errors.New("object is missing required fields")
		}
		return err
	}
	if s.array != nil {
		d, ok := tok.(json.Delim)
		if !ok || d != '[' {
			return errors.New("expected array")
		}
		for dec.More() {
			if err := walkJSON(dec, *s.array); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	}
	if _, ok := tok.(json.Delim); ok || tok == nil {
		return errors.New("expected non-null scalar")
	}
	if s.scalarType == "string" {
		if _, ok := tok.(string); !ok {
			return errors.New("expected string")
		}
	}
	if s.scalarType == "bool" {
		if _, ok := tok.(bool); !ok {
			return errors.New("expected boolean")
		}
	}
	return nil
}
