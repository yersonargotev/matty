package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/packsync"
	"github.com/yersonargotev/matty/internal/packsyncworkflow"
)

func TestSandboxTracerRunsInspectClassifyValidatePublishWithoutExternalWrites(t *testing.T) {
	root := repositoryRootForTest(t)
	base := t.TempDir()
	copyTreeForTest(t, filepath.Join(root, "bundle"), filepath.Join(base, "bundle"))
	snapshot := t.TempDir()
	var config struct {
		Sources []packsync.SourceConfig `json:"sources"`
	}
	readJSONForTest(t, filepath.Join(base, "bundle", "sources.json"), &config)
	for _, binding := range config.Sources[0].Resources {
		copyTreeForTest(t, filepath.Join(base, "bundle", filepath.FromSlash(binding.UpstreamPath)), filepath.Join(snapshot, filepath.FromSlash(binding.UpstreamPath)))
	}
	copyTreeForTest(t, filepath.Join(root, "internal", "packsync", "testdata", "real-upstream"), snapshot)
	oldSnapshot := t.TempDir()
	copyTreeForTest(t, snapshot, oldSnapshot)
	if err := os.MkdirAll(filepath.Join(snapshot, "skills", "in-progress", "sandbox-discovery"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "skills", "in-progress", "sandbox-discovery", "SKILL.md"), []byte("sandbox discovery\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var lock packsync.Lock
	readJSONForTest(t, filepath.Join(base, "bundle", "sources.lock.json"), &lock)
	candidate := lock.Candidate
	candidate.Commit = candidateA
	candidate.Tree = strings.Repeat("f", 40)
	candidate.CommitNodeID = "sandbox-candidate"
	if candidate.Release == nil {
		t.Fatal("fixture lock has no release")
	}
	release := *candidate.Release
	release.ID++
	release.Tag, release.Name = "v9.9.9", "v9.9.9"
	release.CreatedAt = release.CreatedAt.Add(time.Hour)
	release.PublishedAt = release.PublishedAt.Add(time.Hour)
	candidate.Release = &release
	candidate.TagRefName = "refs/tags/" + release.Tag
	candidate.TagRefSHA = strings.Repeat("e", 40)
	candidate.TagObjects = []packsync.TagObject{{SHA: candidate.TagRefSHA, Name: release.Tag, TargetSHA: candidate.Commit, TargetType: "commit", Verification: packsync.Verification{Reason: "unsigned"}}}
	source := &sandboxSource{root: snapshot, oldRoot: oldSnapshot, oldCandidate: lock.Candidate, candidate: candidate}

	gitForTest(t, base, "init", "-q")
	gitForTest(t, base, "config", "user.name", "fixture")
	gitForTest(t, base, "config", "user.email", "fixture@example.com")
	gitForTest(t, base, "add", ".")
	gitForTest(t, base, "commit", "-qm", "base")
	baseSHA := strings.TrimSpace(gitForTest(t, base, "rev-parse", "HEAD"))
	validateRepo, publishRepo := filepath.Join(t.TempDir(), "validate"), filepath.Join(t.TempDir(), "publish")
	gitForTest(t, filepath.Dir(validateRepo), "clone", "-q", base, validateRepo)
	gitForTest(t, filepath.Dir(publishRepo), "clone", "-q", base, publishRepo)

	oldSourceFactory, oldValidatorFactory, oldGatewayFactory := workflowSourceFactory, workflowValidatorFactory, workflowGatewayFactory
	workflowSourceFactory = func() packsync.Source { return source }
	validator := &sandboxValidator{}
	workflowValidatorFactory = func() phaseValidator { return validator }
	fakeGitHub := &fakeGitHubCommands{baseHead: baseSHA}
	workflowGatewayFactory = func(repositoryRoot string, plan packsync.Plan) *githubGateway {
		return &githubGateway{repositoryRoot: repositoryRoot, repository: "owner/repo", plan: plan, retry: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Nanosecond, Sleeper: noWaitSleeper{}}, run: fakeGitHub.run}
	}
	t.Cleanup(func() {
		workflowSourceFactory, workflowValidatorFactory, workflowGatewayFactory = oldSourceFactory, oldValidatorFactory, oldGatewayFactory
	})
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_ACTOR", "maintainer")
	t.Setenv("GITHUB_RUN_ID", "37")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("MATTY_SOURCE_ID", "mattpocock-skills")
	t.Setenv("MATTY_SELECTOR", "latest-stable")
	t.Setenv("MATTY_SELECTOR_REF", "")
	t.Setenv("MATTY_CLASSIFICATION_MODE", "ai")
	t.Setenv("MATTY_REQUEST_REASON", "sandbox tracer")
	t.Setenv("MATTY_EXPECTED_PLAN_ID", "")
	t.Setenv("MATTY_EXPECTED_BASE_SHA", "")
	t.Setenv("MATTY_HUMAN_EVIDENCE_JSON", "")

	artifacts := t.TempDir()
	inspect := filepath.Join(artifacts, "inspect")
	if err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", base, "--output", inspect}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var plan packsync.Plan
	readJSONForTest(t, filepath.Join(inspect, "plan.json"), &plan)
	if plan.Status != "review-required" || len(plan.AffectedPacks) != 0 || len(plan.Discoveries) == 0 {
		t.Fatalf("inspection did not isolate the unselected discovery: %#v", plan)
	}
	classification := filepath.Join(artifacts, "classification")
	if err := run(context.Background(), []string{"--phase", "classify", "--repository-root", base, "--request", filepath.Join(inspect, "request.json"), "--plan", filepath.Join(inspect, "plan.json"), "--output", classification}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	validation := filepath.Join(artifacts, "validation")
	if err := run(context.Background(), []string{"--phase", "validate", "--repository-root", validateRepo, "--request", filepath.Join(inspect, "request.json"), "--plan", filepath.Join(inspect, "plan.json"), "--evidence", filepath.Join(classification, "classification.json"), "--output", validation}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	publication := filepath.Join(artifacts, "publication")
	if err := run(context.Background(), []string{"--phase", "publish", "--repository-root", publishRepo, "--request", filepath.Join(inspect, "request.json"), "--plan", filepath.Join(inspect, "plan.json"), "--evidence", filepath.Join(classification, "classification.json"), "--validation", filepath.Join(validation, "validation.json"), "--output", publication}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if validator.bundleCalls < 2 || validator.suiteCalls < 2 || fakeGitHub.createCalls != 1 || fakeGitHub.pushCalls != 1 {
		t.Fatalf("tracer gates/writes = bundle:%d suite:%d create:%d push:%d", validator.bundleCalls, validator.suiteCalls, fakeGitHub.createCalls, fakeGitHub.pushCalls)
	}
	var result map[string]any
	readJSONForTest(t, filepath.Join(publication, "publication.json"), &result)
	if result["decision_ready"] != true || result["auto_merge"] != false {
		t.Fatalf("publication result = %#v", result)
	}
}

type sandboxSource struct {
	root         string
	oldRoot      string
	oldCandidate packsync.Candidate
	candidate    packsync.Candidate
}

func (source *sandboxSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return []packsync.Release{*source.oldCandidate.Release, *source.candidate.Release}, nil
}
func (source *sandboxSource) ResolveRelease(_ context.Context, _ packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	candidate := source.oldCandidate
	if release.Tag == source.candidate.Release.Tag {
		candidate = source.candidate
	}
	candidate.Release = &release
	return candidate, nil
}
func (source *sandboxSource) ResolveCommit(_ context.Context, _ packsync.SourceConfig, sha string) (packsync.Candidate, error) {
	candidate := source.candidate
	candidate.Commit = sha
	candidate.Release = nil
	return candidate, nil
}
func (source *sandboxSource) snapshotRoot(candidate packsync.Candidate) string {
	if candidate.Commit == source.oldCandidate.Commit {
		return source.oldRoot
	}
	return source.root
}

func (source *sandboxSource) WithSnapshot(_ context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	target := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeErrorForTest(source.snapshotRoot(candidate), target); err != nil {
		return err
	}
	err := visit(target)
	cleanup := os.RemoveAll(target)
	if err != nil {
		return err
	}
	return cleanup
}

type sandboxValidator struct{ bundleCalls, suiteCalls int }

func (validator *sandboxValidator) ValidateBundle(context.Context, string, string) error {
	validator.bundleCalls++
	return nil
}
func (validator *sandboxValidator) Validate(context.Context, string) error {
	validator.suiteCalls++
	return nil
}

func repositoryRootForTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func readJSONForTest(t *testing.T, name string, out any) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
func copyTreeForTest(t *testing.T, source, target string) {
	t.Helper()
	if err := copyTreeErrorForTest(source, target); err != nil {
		t.Fatal(err)
	}
}
func copyTreeErrorForTest(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, info.Mode().Perm())
	})
}
func gitForTest(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}
