package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/addyacceptance"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func TestAddyRegistrationTracerProvesExactEndToEndAdmission(t *testing.T) {
	fixture := addyacceptance.Canonical()
	if got := addyResourceCounts(fixture); got != [5]int{24, 4, 8, 7, 1} {
		t.Fatalf("canonical Addy inventory = %v", got)
	}
	if got := addySurfaceBindingCounts(fixture); got != [2]int{36, 36} {
		t.Fatalf("canonical Addy surface bindings = %v", got)
	}

	base := t.TempDir()
	copyTreeForTest(t, filepath.Join(repositoryRootForTest(t), "bundle"), filepath.Join(base, "bundle"))
	removeConfiguredAddyForTracer(t, filepath.Join(base, "bundle"), fixture)
	if err := os.RemoveAll(filepath.Join(base, "bundle", "packs", "addy")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "bundle", "packs", "addy"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestValue := map[string]any{}
	rawManifest, err := json.Marshal(fixture.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawManifest, &manifestValue); err != nil {
		t.Fatal(err)
	}
	delete(manifestValue, "surfaces")
	manifest, err := json.MarshalIndent(manifestValue, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "bundle", "packs", "addy", "pack.json"), append(manifest, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	registration := packsync.SourceConfig{ID: "addy", Provider: "github", Repository: fixture.Provenance.Repository, Selector: packsync.Selector{Mode: packsync.SelectorStableRelease}}
	for _, resource := range fixture.Manifest.Resources {
		registration.Resources = append(registration.Resources, packsync.Binding{PackID: "addy", Kind: resource.Kind, ResourceID: resource.ID, UpstreamPath: resource.Source})
	}
	registrationDigest, err := packsyncworkflow.CanonicalRegistrationSHA256(registration)
	if err != nil {
		t.Fatal(err)
	}

	gitForTest(t, base, "init", "-q")
	gitForTest(t, base, "config", "user.name", "fixture")
	gitForTest(t, base, "config", "user.email", "fixture@example.com")
	gitForTest(t, base, "add", ".")
	gitForTest(t, base, "commit", "-qm", "base")
	baseSHA := strings.TrimSpace(gitForTest(t, base, "rev-parse", "HEAD"))

	source := &exactAddySource{candidate: exactAddyCandidate(fixture)}
	validator := &exactAddyValidator{}
	fakeGitHub := &fakeGitHubCommands{sourceID: "addy", baseHead: baseSHA}
	oldSourceFactory, oldValidatorFactory, oldGatewayFactory := workflowSourceFactory, workflowValidatorFactory, workflowGatewayFactory
	workflowSourceFactory = func() packsync.Source { return source }
	workflowValidatorFactory = func() phaseValidator { return validator }
	workflowGatewayFactory = func(repositoryRoot string, plan packsync.Plan) *githubGateway {
		runAddy := func(ctx context.Context, directory, name string, args ...string) (string, error) {
			output, err := fakeGitHub.run(ctx, directory, name, args...)
			return strings.ReplaceAll(output, "sync/mattpocock-skills", "sync/addy"), err
		}
		return &githubGateway{repositoryRoot: repositoryRoot, repository: "owner/repo", plan: plan, retry: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Nanosecond, Sleeper: noWaitSleeper{}}, run: runAddy}
	}
	t.Cleanup(func() {
		workflowSourceFactory, workflowValidatorFactory, workflowGatewayFactory = oldSourceFactory, oldValidatorFactory, oldGatewayFactory
	})
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_ACTOR", "maintainer")
	t.Setenv("GITHUB_RUN_ID", "88")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")

	first := packsyncworkflow.DispatchRequest{SchemaVersion: 2, Operation: packsyncworkflow.OperationRegister, SourceID: "addy", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationHuman, RequestReason: "register exact Addy 0.6.4", Registration: &registration, RegistrationSHA256: registrationDigest}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	if err := writeCanonical(requestPath, first); err != nil {
		t.Fatal(err)
	}
	artifacts := t.TempDir()
	inspectDir := filepath.Join(artifacts, "inspect")
	if err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", base, "--request", requestPath, "--output", inspectDir}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var plan packsync.Plan
	readJSONForTest(t, filepath.Join(inspectDir, "plan.json"), &plan)
	if plan.Status != "review-required" || plan.Registration == nil || plan.RegistrationSHA256 != registrationDigest || len(plan.AffectedPacks) != 1 || plan.AffectedPacks[0].PackID != "addy" {
		t.Fatalf("registration inspection = %#v", plan)
	}

	evidence := exactHumanEvidence(t, plan)
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Selector, second.SelectorRef = packsyncworkflow.SelectorCommit, plan.Candidate.Commit
	second.ExpectedPlanID, second.ExpectedBaseSHA = plan.PlanID, plan.Preconditions.BaseCommit
	second.HumanEvidence = evidenceJSON
	secondRequest := filepath.Join(t.TempDir(), "request.json")
	if err := writeCanonical(secondRequest, second); err != nil {
		t.Fatal(err)
	}
	classificationDir := filepath.Join(artifacts, "classification")
	if err := run(context.Background(), []string{"--phase", "classify", "--repository-root", base, "--request", secondRequest, "--plan", filepath.Join(inspectDir, "plan.json"), "--output", classificationDir}, io.Discard); err != nil {
		t.Fatal(err)
	}

	validateRepo, publishRepo := filepath.Join(t.TempDir(), "validate"), filepath.Join(t.TempDir(), "publish")
	gitForTest(t, filepath.Dir(validateRepo), "clone", "-q", base, validateRepo)
	gitForTest(t, filepath.Dir(publishRepo), "clone", "-q", base, publishRepo)
	validationDir := filepath.Join(artifacts, "validation")
	if err := run(context.Background(), []string{"--phase", "validate", "--repository-root", validateRepo, "--request", secondRequest, "--plan", filepath.Join(inspectDir, "plan.json"), "--evidence", filepath.Join(classificationDir, "classification.json"), "--output", validationDir}, io.Discard); err != nil {
		t.Fatal(err)
	}
	publicationDir := filepath.Join(artifacts, "publication")
	if err := run(context.Background(), []string{"--phase", "publish", "--repository-root", publishRepo, "--request", secondRequest, "--plan", filepath.Join(inspectDir, "plan.json"), "--evidence", filepath.Join(classificationDir, "classification.json"), "--validation", filepath.Join(validationDir, "validation.json"), "--output", publicationDir}, io.Discard); err != nil {
		t.Fatal(err)
	}

	var validation packsyncworkflow.ValidationArtifact
	var publication packsyncworkflow.PublicationArtifact
	readJSONForTest(t, filepath.Join(validationDir, "validation.json"), &validation)
	readJSONForTest(t, filepath.Join(publicationDir, "publication.json"), &publication)
	if source.acquisitions < 3 || source.executions != 0 || validator.bundleCalls < 2 || validator.suiteCalls < 2 || validator.commands < 2 {
		t.Fatalf("reacquisition/gates = acquisitions:%d executions:%d bundle:%d suite:%d commands:%d", source.acquisitions, source.executions, validator.bundleCalls, validator.suiteCalls, validator.commands)
	}
	if validation.ResultTreeSHA == "" || publication.ResultTreeSHA != validation.ResultTreeSHA || publication.BranchName != "sync/addy" || publication.PRNumber != 7 || !publication.DecisionReady || publication.AutoMerge || publication.UpstreamContentExecuted || fakeGitHub.createCalls != 1 || fakeGitHub.pushCalls != 1 {
		t.Fatalf("registration publication = validation:%#v publication:%#v", validation, publication)
	}
	assertSecretFreeArtifacts(t, artifacts)

	t.Run("failure-before-validation-gate-does-not-write", func(t *testing.T) {
		failedRepo := filepath.Join(t.TempDir(), "failed")
		gitForTest(t, filepath.Dir(failedRepo), "clone", "-q", base, failedRepo)
		bad := evidence
		bad.Evidence = nil
		badPath := filepath.Join(t.TempDir(), "bad.json")
		if err := writeCanonical(badPath, bad); err != nil {
			t.Fatal(err)
		}
		beforePush, beforeCreate := fakeGitHub.pushCalls, fakeGitHub.createCalls
		err := run(context.Background(), []string{"--phase", "publish", "--repository-root", failedRepo, "--request", secondRequest, "--plan", filepath.Join(inspectDir, "plan.json"), "--evidence", badPath, "--validation", filepath.Join(validationDir, "validation.json"), "--output", filepath.Join(t.TempDir(), "failure")}, io.Discard)
		if err == nil || fakeGitHub.pushCalls != beforePush || fakeGitHub.createCalls != beforeCreate {
			t.Fatalf("pre-gate failure wrote GitHub state: pushes=%d creates=%d err=%v", fakeGitHub.pushCalls-beforePush, fakeGitHub.createCalls-beforeCreate, err)
		}
	})
}

func removeConfiguredAddyForTracer(t *testing.T, bundleRoot string, fixture addyacceptance.Fixture) {
	t.Helper()
	configPath := filepath.Join(bundleRoot, "sources.json")
	var config packsync.Config
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	sources := config.Sources[:0]
	for _, source := range config.Sources {
		if source.ID != "addy" {
			sources = append(sources, source)
		}
	}
	config.Sources = sources
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(bundleRoot, "sources", "addy.lock.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, resource := range fixture.Manifest.Resources {
		if err := os.RemoveAll(filepath.Join(bundleRoot, filepath.FromSlash(resource.Source))); err != nil {
			t.Fatal(err)
		}
	}
}

type exactAddySource struct {
	candidate    packsync.Candidate
	acquisitions int
	executions   int
}

func (source *exactAddySource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return []packsync.Release{*source.candidate.Release}, nil
}
func (source *exactAddySource) ResolveRelease(_ context.Context, _ packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	candidate := source.candidate
	candidate.Release = &release
	return candidate, nil
}
func (source *exactAddySource) ResolveCommit(_ context.Context, _ packsync.SourceConfig, sha string) (packsync.Candidate, error) {
	if sha != source.candidate.Commit {
		return packsync.Candidate{}, errors.New("unexpected Addy commit")
	}
	return source.candidate, nil
}
func (source *exactAddySource) WithSnapshot(_ context.Context, _ packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	source.acquisitions++
	snapshot := filepath.Join(temporaryRoot, "exact-addy")
	if err := addyacceptance.WriteExactAcquisition(snapshot); err != nil {
		return err
	}
	err := visit(snapshot)
	cleanup := os.RemoveAll(snapshot)
	if err != nil {
		return err
	}
	return cleanup
}

type exactAddyValidator struct {
	bundleCalls, suiteCalls int
	commands                int
}

func (validator *exactAddyValidator) ValidateBundle(ctx context.Context, repositoryRoot, bundleRoot string) error {
	validator.bundleCalls++
	if err := validateExactAddyResult(bundleRoot); err != nil {
		return err
	}
	return (commandValidator{run: validator.captureValidationCommand}).ValidateBundle(ctx, repositoryRoot, bundleRoot)
}
func (validator *exactAddyValidator) Validate(ctx context.Context, repositoryRoot string) error {
	validator.suiteCalls++
	if err := validateExactAddyResult(filepath.Join(repositoryRoot, "bundle")); err != nil {
		return err
	}
	return (commandValidator{run: validator.captureValidationCommand}).Validate(ctx, repositoryRoot)
}
func (validator *exactAddyValidator) captureValidationCommand(cmd *exec.Cmd) ([]byte, error) {
	validator.commands++
	if cmd.Dir == "" || len(cmd.Args) != 2 || cmd.Args[0] != "bash" || cmd.Args[1] != "./scripts/validate-packy.sh" {
		return nil, errors.New("validation did not invoke Packy's full authority")
	}
	return nil, nil
}
func validateExactAddyResult(bundleRoot string) error {
	var manifest addyacceptance.Manifest
	data, err := os.ReadFile(filepath.Join(bundleRoot, "packs", "addy", "pack.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.ID != "addy" || manifest.Version != addyacceptance.PackVersion || addyResourceCounts(addyacceptance.Fixture{Manifest: manifest}) != [5]int{24, 4, 8, 7, 1} {
		return errors.New("Packy suite rejected incomplete Addy result")
	}
	return nil
}

func exactHumanEvidence(t *testing.T, plan packsync.Plan) packsync.ClassificationEvidenceSet {
	t.Helper()
	inspectionID, err := packsync.HumanInspectionID(plan)
	if err != nil {
		t.Fatal(err)
	}
	set := packsync.ClassificationEvidenceSet{SchemaVersion: 1, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, Candidate: plan.Candidate, HumanInspectionID: inspectionID}
	for _, impact := range plan.AffectedPacks {
		evidence := packsync.ClassificationEvidence{PackID: impact.PackID, Classifier: packsync.ClassifierIdentity{Type: packsync.ClassifierHuman, ID: "maintainer"}, Rationale: "Exact Addy 0.6.4 inventory is admitted as the complete 1.0.0 introduction.", CurrentVersion: impact.CurrentVersion, ProposedVersion: addyacceptance.PackVersion, ChangedAspects: []string{"complete Addy workflow pack introduction"}, MechanicalFloor: impact.MechanicalFloor, FinalLevel: packsync.LevelMajor, Migration: "Introduce the previously absent Addy pack.", RequiredActions: []string{"Review both projected surfaces before activation."}}
		set.Evidence = append(set.Evidence, evidence)
	}
	return set
}

func exactAddyCandidate(fixture addyacceptance.Fixture) packsync.Candidate {
	verifiedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	signature, payload := strings.Repeat("a", 64), strings.Repeat("b", 64)
	return packsync.Candidate{Repository: fixture.Provenance.Repository, RepositoryID: fixture.Provenance.RepositoryID, RepositoryNodeID: "R_kgDONPcQgA", RepositoryHTML: "https://github.com/" + fixture.Provenance.Repository, RepositoryClone: "https://github.com/" + fixture.Provenance.Repository + ".git", RepositoryAPI: "https://api.github.com/repos/" + fixture.Provenance.Repository, Visibility: "public", Owner: "addyosmani", OwnerID: fixture.Provenance.OwnerID, OwnerNodeID: "MDQ6VXNlcjExMDk1Mw==", Public: true, Release: &packsync.Release{ID: 1, NodeID: "RE_addy064", Tag: fixture.Provenance.Release, Name: fixture.Provenance.Release, Target: "main", CreatedAt: verifiedAt, PublishedAt: verifiedAt, Author: packsync.Actor{Login: "addyosmani", ID: fixture.Provenance.OwnerID, NodeID: "MDQ6VXNlcjExMDk1Mw=="}}, TagRefName: "refs/tags/" + fixture.Provenance.Release, TagRefType: "tag", TagRefSHA: fixture.Provenance.TagSHA, TagObjects: []packsync.TagObject{{SHA: fixture.Provenance.TagSHA, Name: fixture.Provenance.Release, TargetSHA: fixture.Provenance.Commit, TargetType: "commit", Verification: packsync.Verification{Reason: fixture.Provenance.TagVerification.Reason}}}, Commit: fixture.Provenance.Commit, CommitNodeID: "C_addy064", Tree: fixture.Provenance.Tree, Parents: append([]string(nil), fixture.Provenance.CommitParents...), CommitVerify: packsync.Verification{Verified: true, Reason: fixture.Provenance.CommitVerification.Reason, VerifiedAt: &verifiedAt, SignatureSHA256: &signature, PayloadSHA256: &payload}, ArchiveSHA256: fixture.Provenance.ArchiveSHA256}
}

func addyResourceCounts(fixture addyacceptance.Fixture) [5]int {
	var counts [5]int
	for _, resource := range fixture.Manifest.Resources {
		switch resource.Kind {
		case "skill":
			counts[0]++
		case "agent":
			counts[1]++
		case "command":
			counts[2]++
		case "asset":
			counts[3]++
		case "notice":
			counts[4]++
		}
	}
	return counts
}

func addySurfaceBindingCounts(fixture addyacceptance.Fixture) [2]int {
	var counts [2]int
	for _, resource := range fixture.Manifest.Resources {
		for _, binding := range resource.Bindings {
			switch binding.Surface {
			case "codex":
				counts[0]++
			case "opencode":
				counts[1]++
			}
		}
	}
	return counts
}

func assertSecretFreeArtifacts(t *testing.T, root string) {
	t.Helper()
	forbidden := [][]byte{[]byte("GITHUB_TOKEN"), []byte("Authorization: Bearer"), addyacceptance.ExactArchive()}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, needle := range forbidden {
			if len(needle) != 0 && bytes.Contains(data, needle) {
				return errors.New("artifact contains secret or upstream archive bytes")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
