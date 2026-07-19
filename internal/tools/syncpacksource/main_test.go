package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsync/githubsource"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func TestRendererUsesTheEngineCanonicalHumanAndJSONPlans(t *testing.T) {
	plan := packsync.Plan{SchemaVersion: 1, PlanID: "pack-sync-test", Status: "blocked", SourceID: "source", Blockers: []string{"blocked"}, Changes: []packsync.Change{}, Discoveries: []string{}}
	for _, test := range []struct {
		format string
		want   func() []byte
	}{
		{format: "human", want: func() []byte { return []byte(plan.Human()) }},
		{format: "json", want: func() []byte { data, _ := plan.CanonicalJSON(); return data }},
	} {
		var output bytes.Buffer
		if err := render(&output, plan, test.format); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(output.Bytes(), test.want()) {
			t.Fatalf("%s renderer diverged from engine canonical plan", test.format)
		}
	}
}

func TestRetryAfterAcceptsSecondsAndHTTPDates(t *testing.T) {
	if got := retryAfter("7"); got != 7*time.Second {
		t.Fatalf("Retry-After seconds = %v", got)
	}
	future := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
	if got := retryAfter(future); got < 8*time.Second || got > 11*time.Second {
		t.Fatalf("Retry-After date = %v", got)
	}
}

func TestValidationSubprocessEnvironmentDropsCredentials(t *testing.T) {
	got := withoutCredentials([]string{"PATH=/bin", "GH_TOKEN=secret", "GITHUB_TOKEN=secret", "SSH_AUTH_SOCK=/tmp/agent", "SAFE=value"})
	want := []string{"PATH=/bin", "SAFE=value"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered environment = %#v", got)
	}
}

func TestCommandValidatorRunsStagedBundleFromCopiedRepositoryWithSandboxedConfiguration(t *testing.T) {
	repository := t.TempDir()
	staged := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(filepath.Join(repository, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repository, "bundle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "bundle", "identity"), []byte("production\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "identity"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `#!/usr/bin/env bash
set -euo pipefail
test "$(cat bundle/identity)" = "staged"
test "${PACKY_VALIDATION_STAGED:-}" = "1"
test "${HOME}" != "${OPERATOR_HOME}"
test "${XDG_CONFIG_HOME}" != "${OPERATOR_XDG}"
test -z "${GITHUB_TOKEN:-}"
mkdir -p "${HOME}" "${XDG_CONFIG_HOME}"
printf validated > "${HOME}/proof"
printf validated > "${XDG_CONFIG_HOME}/proof"
`
	if err := os.WriteFile(filepath.Join(repository, "scripts", "validate-packy.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	operatorHome := filepath.Join(t.TempDir(), "operator-home")
	operatorXDG := filepath.Join(t.TempDir(), "operator-xdg")
	t.Setenv("HOME", operatorHome)
	t.Setenv("XDG_CONFIG_HOME", operatorXDG)
	t.Setenv("OPERATOR_HOME", operatorHome)
	t.Setenv("OPERATOR_XDG", operatorXDG)
	t.Setenv("GITHUB_TOKEN", "must-not-reach-validator")
	t.Setenv("PACKY_VALIDATION_STAGED", "hostile-inherited-value")

	if err := (commandValidator{}).ValidateBundle(context.Background(), repository, staged); err != nil {
		t.Fatal(err)
	}
	productionScript := strings.Replace(script, `= "staged"`, `= "production"`, 1)
	if err := os.WriteFile(filepath.Join(repository, "scripts", "validate-packy.sh"), []byte(productionScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (commandValidator{}).ValidateBundle(context.Background(), repository, filepath.Join(repository, "bundle")); err != nil {
		t.Fatalf("production bundle validation did not use a disposable checkout: %v", err)
	}
	for _, path := range []string{operatorHome, operatorXDG} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("staged validation wrote operator configuration path %s: %v", path, err)
		}
	}
}

func TestPublicSourceRetriesRateLimit403AndContinuesAfterSuccess(t *testing.T) {
	underlying := &sourceFailureFixture{failures: []error{githubsource.HTTPError{Operation: "test", StatusCode: http.StatusForbidden, Status: "403 Forbidden", RetryAfter: "4", RateLimitRemaining: "0"}}}
	sleeper := &sourceSleeper{}
	source := retryingSource{source: underlying, policy: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, Sleeper: sleeper}}
	if _, err := source.Releases(context.Background(), packsync.SourceConfig{}); err != nil {
		t.Fatal(err)
	}
	if underlying.calls != 2 || !reflect.DeepEqual(sleeper.delays, []time.Duration{4 * time.Second}) {
		t.Fatalf("calls=%d delays=%v", underlying.calls, sleeper.delays)
	}
}

func TestPublicSourceRetries429And503WithCanonicalDelays(t *testing.T) {
	underlying := &sourceFailureFixture{failures: []error{githubsource.HTTPError{Operation: "test", StatusCode: http.StatusTooManyRequests, Status: "429", RetryAfter: "4"}, githubsource.HTTPError{Operation: "test", StatusCode: http.StatusServiceUnavailable, Status: "503"}}}
	sleeper := &sourceSleeper{}
	source := retryingSource{source: underlying, policy: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, Sleeper: sleeper}}
	if _, err := source.Releases(context.Background(), packsync.SourceConfig{}); err != nil {
		t.Fatal(err)
	}
	if underlying.calls != 3 || !reflect.DeepEqual(sleeper.delays, []time.Duration{4 * time.Second, 2 * time.Second}) {
		t.Fatalf("calls=%d delays=%v", underlying.calls, sleeper.delays)
	}
}

func TestPublicSourceDoesNotRetryNonTransientFailures(t *testing.T) {
	sleeper := &sourceSleeper{}
	underlying := &sourceFailureFixture{failures: []error{errors.New("moved provenance")}}
	source := retryingSource{source: underlying, policy: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, Sleeper: sleeper}}
	if _, err := source.Releases(context.Background(), packsync.SourceConfig{}); err == nil || underlying.calls != 1 {
		t.Fatalf("integrity blocker retried: calls=%d err=%v", underlying.calls, err)
	}
}

func TestInspectBoundaryReportsNonRateLimit403AsSecretFreeAccessFailure(t *testing.T) {
	repository := t.TempDir()
	copyTreeForTest(t, filepath.Join(repositoryRootForTest(t), "bundle"), filepath.Join(repository, "bundle"))
	underlying := &sourceFailureFixture{failures: []error{githubsource.HTTPError{Operation: "read GitHub API", StatusCode: http.StatusForbidden, Status: "403 Forbidden", RateLimitRemaining: "4999"}}}
	oldFactory := workflowSourceFactory
	workflowSourceFactory = func() packsync.Source { return newRetryingSource(underlying) }
	t.Cleanup(func() { workflowSourceFactory = oldFactory })

	t.Setenv("GITHUB_TOKEN", "inspect-secret-token")
	t.Setenv("PACKY_SOURCE_ID", "mattpocock-skills")
	t.Setenv("PACKY_SELECTOR", "latest-stable")
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", "access failure fixture")
	output := t.TempDir()
	err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", repository, "--output", output}, io.Discard)
	var failure packsyncworkflow.Failure
	if !errors.As(err, &failure) || failure.Kind != packsyncworkflow.FailureAccess || underlying.calls != 1 {
		t.Fatalf("Inspect failure = %#v, calls=%d, err=%v", failure, underlying.calls, err)
	}
	data, readErr := os.ReadFile(filepath.Join(output, "operational-artifact.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	artifactText := string(data)
	for _, forbidden := range []string{"inspect-secret-token", "Authorization", "response body bytes", "provenance"} {
		if strings.Contains(artifactText, forbidden) {
			t.Fatalf("access artifact contains %q: %s", forbidden, artifactText)
		}
	}
	if !strings.Contains(artifactText, "denied access") || !strings.Contains(artifactText, "contents: read") {
		t.Fatalf("access artifact is not actionable: %s", artifactText)
	}
	if _, err := os.Stat(filepath.Join(output, "plan.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("access failure unexpectedly produced a plan: %v", err)
	}
}

func TestWorkflowAuthenticationIsLimitedToGitHubAPIOrigin(t *testing.T) {
	seen := map[string]string{}
	client := newAuthenticatedGitHubHTTPClient("job-scoped-token", roundTripFunc(func(request *http.Request) (*http.Response, error) {
		seen[request.URL.Host] = request.Header.Get("Authorization")
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header), Request: request}, nil
	}))
	for _, endpoint := range []string{"https://api.github.com/repos/o/r", "https://example.com/redirected-archive"} {
		request, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer attacker-controlled")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	}
	if seen["api.github.com"] != "Bearer job-scoped-token" || seen["example.com"] != "" {
		t.Fatalf("authorization by origin = %#v", seen)
	}
}

func TestInspectBoundaryRetriesRateLimit403AndContinuesSuccessfully(t *testing.T) {
	repository, snapshot, lock := prepareInspectFixture(t)
	stable := &sandboxSource{root: snapshot, oldRoot: snapshot, oldCandidate: lock.Candidate, candidate: lock.Candidate}
	flaky := &releasesFailureSource{source: stable, failures: []error{githubsource.HTTPError{Operation: "read GitHub API", StatusCode: http.StatusForbidden, Status: "403 Forbidden", RetryAfter: "4", RateLimitRemaining: "0"}}}
	sleeper := &sourceSleeper{}
	oldFactory := workflowSourceFactory
	workflowSourceFactory = func() packsync.Source {
		return retryingSource{source: flaky, policy: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, Sleeper: sleeper}}
	}
	t.Cleanup(func() { workflowSourceFactory = oldFactory })

	setInspectEnvironment(t, "rate-limit retry fixture")
	output := t.TempDir()
	if err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", repository, "--output", output}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var plan packsync.Plan
	readJSONForTest(t, filepath.Join(output, "plan.json"), &plan)
	if plan.Status != "no-op" || flaky.calls != 2 || !reflect.DeepEqual(sleeper.delays, []time.Duration{4 * time.Second}) {
		t.Fatalf("Inspect continuation = status:%s calls:%d delays:%v", plan.Status, flaky.calls, sleeper.delays)
	}
}

func TestInspectBoundaryKeepsGenuinelyMovedTagAsProvenanceFailure(t *testing.T) {
	repository, snapshot, lock := prepareInspectFixture(t)
	moved := lock.Candidate
	if len(moved.TagObjects) == 0 {
		t.Fatal("fixture candidate has no tag object")
	}
	moved.TagRefSHA = strings.Repeat("d", 40)
	moved.TagObjects = append([]packsync.TagObject(nil), moved.TagObjects...)
	moved.TagObjects[0].SHA = moved.TagRefSHA
	source := &sandboxSource{root: snapshot, oldRoot: snapshot, oldCandidate: lock.Candidate, candidate: moved}
	oldFactory := workflowSourceFactory
	workflowSourceFactory = func() packsync.Source { return source }
	t.Cleanup(func() { workflowSourceFactory = oldFactory })

	setInspectEnvironment(t, "moved tag fixture")
	output := t.TempDir()
	err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", repository, "--output", output}, io.Discard)
	var failure packsyncworkflow.Failure
	if !errors.As(err, &failure) || failure.Kind != packsyncworkflow.FailureProvenance {
		t.Fatalf("moved-tag failure = %#v, %v", failure, err)
	}
	var plan packsync.Plan
	readJSONForTest(t, filepath.Join(output, "plan.json"), &plan)
	if plan.Status != "blocked" || !strings.Contains(strings.Join(plan.Blockers, " "), "tag ref moved") {
		t.Fatalf("moved-tag plan = %#v", plan)
	}
}

func prepareInspectFixture(t *testing.T) (string, string, packsync.Lock) {
	t.Helper()
	root := repositoryRootForTest(t)
	repository := t.TempDir()
	copyTreeForTest(t, filepath.Join(root, "bundle"), filepath.Join(repository, "bundle"))
	snapshot := t.TempDir()
	var config struct {
		Sources []packsync.SourceConfig `json:"sources"`
	}
	readJSONForTest(t, filepath.Join(repository, "bundle", "sources.json"), &config)
	for _, binding := range config.Sources[0].Resources {
		copyTreeForTest(t, filepath.Join(repository, "bundle", filepath.FromSlash(binding.UpstreamPath)), filepath.Join(snapshot, filepath.FromSlash(binding.UpstreamPath)))
	}
	var lock packsync.Lock
	readJSONForTest(t, filepath.Join(repository, "bundle", "sources/mattpocock-skills.lock.json"), &lock)
	gitForTest(t, repository, "init", "-q")
	gitForTest(t, repository, "config", "user.name", "fixture")
	gitForTest(t, repository, "config", "user.email", "fixture@example.com")
	gitForTest(t, repository, "add", ".")
	gitForTest(t, repository, "commit", "-qm", "base")
	return repository, snapshot, lock
}

func setInspectEnvironment(t *testing.T, reason string) {
	t.Helper()
	t.Setenv("PACKY_SOURCE_ID", "mattpocock-skills")
	t.Setenv("PACKY_SELECTOR", "latest-stable")
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", reason)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestWorkflowAcceptsOnlyEmptyEvidenceWhenNoPackIsAffected(t *testing.T) {
	plan := packsync.Plan{}
	if err := validateWorkflowEvidence(plan, packsync.ClassificationEvidenceSet{}); err != nil {
		t.Fatal(err)
	}
	if err := validateWorkflowEvidence(plan, packsync.ClassificationEvidenceSet{SchemaVersion: 1}); err == nil {
		t.Fatal("classification evidence accepted for unaffected plan")
	}
}

func TestInspectNormalizesWorkflowEnvironmentThroughCanonicalDispatch(t *testing.T) {
	t.Setenv("PACKY_SOURCE_ID", "source")
	t.Setenv("PACKY_SELECTOR", "commit")
	t.Setenv("PACKY_SELECTOR_REF", strings.Repeat("a", 40))
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", "fixture")
	request := packsyncworkflow.DispatchRequest{SchemaVersion: 2, Operation: packsyncworkflow.OperationSynchronize, SourceID: "source", Selector: packsyncworkflow.SelectorCommit, SelectorRef: strings.Repeat("a", 40), ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "fixture"}
	digest, err := request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PACKY_REQUEST_DIGEST", digest)
	got, check, err := inspectRequest(options{repositoryRoot: t.TempDir()})
	if err != nil || got.SourceID != "source" || check.Selector == nil || check.Selector.Mode != packsync.SelectorCommit {
		t.Fatalf("normalized request = %#v, %#v, %v", got, check, err)
	}
}

func TestInspectCarriesStrictRegistrationIntoCheck(t *testing.T) {
	registration := packsync.SourceConfig{ID: "addy", Provider: "github", Repository: "addyosmani/agent-skills", Selector: packsync.Selector{Mode: packsync.SelectorStableRelease}, Resources: []packsync.Binding{{PackID: "addy", Kind: "skill", ResourceID: "fixture", UpstreamPath: "skills/fixture"}}}
	digest, err := packsyncworkflow.CanonicalRegistrationSHA256(registration)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(registration)
	t.Setenv("PACKY_SOURCE_ID", "addy")
	t.Setenv("PACKY_OPERATION", "register")
	t.Setenv("PACKY_REGISTRATION_JSON", string(data))
	t.Setenv("PACKY_REGISTRATION_SHA256", digest)
	t.Setenv("PACKY_SELECTOR", "latest-stable")
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", "admit Addy")
	request := packsyncworkflow.DispatchRequest{SchemaVersion: 2, Operation: packsyncworkflow.OperationRegister, SourceID: "addy", Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "admit Addy", Registration: &registration, RegistrationSHA256: digest}
	requestDigest, _ := request.Digest()
	t.Setenv("PACKY_REQUEST_DIGEST", requestDigest)

	got, check, err := inspectRequest(options{repositoryRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got.Operation != packsyncworkflow.OperationRegister || check.Registration == nil || !reflect.DeepEqual(*check.Registration, registration) {
		t.Fatalf("registration dispatch lost authority: request=%#v check=%#v", got, check)
	}
}

func TestInspectDoesNotAdoptLegacyMattyWorkflowEnvironment(t *testing.T) {
	t.Setenv("PACKY_SOURCE_ID", "")
	t.Setenv("MATTY_SOURCE_ID", "legacy-source")
	t.Setenv("MATTY_SELECTOR", "commit")
	t.Setenv("MATTY_SELECTOR_REF", strings.Repeat("a", 40))

	got, check, err := inspectRequest(options{repositoryRoot: t.TempDir(), sourceID: "explicit-source"})
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceID != "explicit-source" || check.SourceID != "explicit-source" {
		t.Fatalf("legacy Matty environment was adopted: request=%#v check=%#v", got, check)
	}
}

func TestInspectRejectsMismatchedWorkflowRequestDigest(t *testing.T) {
	t.Setenv("PACKY_SOURCE_ID", "source")
	t.Setenv("PACKY_SELECTOR", "latest-stable")
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", "fixture")
	t.Setenv("PACKY_REQUEST_DIGEST", strings.Repeat("0", 64))
	if _, _, err := inspectRequest(options{repositoryRoot: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "request digest") {
		t.Fatalf("mismatched request digest error = %v", err)
	}
}

func TestInvalidDispatchStillEmitsCanonicalFailureArtifact(t *testing.T) {
	output := t.TempDir()
	t.Setenv("PACKY_SOURCE_ID", "../source")
	t.Setenv("PACKY_SELECTOR", "latest-stable")
	t.Setenv("PACKY_CLASSIFICATION_MODE", "ai")
	t.Setenv("PACKY_REQUEST_REASON", "invalid source fixture")
	if err := run(context.Background(), []string{"--phase", "inspect", "--output", output}, io.Discard); err == nil {
		t.Fatal("invalid dispatch was admitted")
	}
	data, err := os.ReadFile(filepath.Join(output, "operational-artifact.json"))
	if err != nil {
		t.Fatal(err)
	}
	var artifact packsyncworkflow.FailureArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.SourceID != "unknown" {
		t.Fatalf("failure source = %q", artifact.SourceID)
	}
	if _, err := artifact.CanonicalJSON(); err != nil {
		t.Fatalf("failure artifact is not canonical: %v", err)
	}
}

func TestInspectBoundaryEmitsCanonicalNoopArtifact(t *testing.T) {
	output := t.TempDir()
	plan := packsync.Plan{SchemaVersion: 1, PlanID: "plan-noop", Status: "no-op", SourceID: "source", Candidate: packsync.Candidate{Commit: strings.Repeat("a", 40)}, SourceLockSHA256: strings.Repeat("c", 64), LockSetSHA256: strings.Repeat("d", 64), Preconditions: packsync.Preconditions{BaseCommit: strings.Repeat("b", 40), ConfigSHA256: strings.Repeat("e", 64), ManifestsSHA256: strings.Repeat("f", 64)}}
	if err := writeNoopArtifact(output, plan.SourceID, plan); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, "no-op.json"))
	if err != nil {
		t.Fatal(err)
	}
	var artifact map[string]any
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact["state"] != "no-op" || artifact["base_sha"] != plan.Preconditions.BaseCommit || artifact["candidate_sha"] != plan.Candidate.Commit || artifact["contains_secrets"] != false || artifact["contains_upstream_bytes"] != false {
		t.Fatalf("no-op artifact = %#v", artifact)
	}
}

func TestHumanEvidenceDispatchRechecksTheOriginalReleaseSelector(t *testing.T) {
	candidate := packsync.Candidate{Commit: strings.Repeat("a", 40), Release: &packsync.Release{Tag: "v1.2.0", Prerelease: false}}
	evidence, _ := json.Marshal(packsync.ClassificationEvidenceSet{Candidate: candidate})
	request := packsyncworkflow.DispatchRequest{SchemaVersion: 1, SourceID: "source", Selector: packsyncworkflow.SelectorCommit, SelectorRef: candidate.Commit, ClassificationMode: packsyncworkflow.ClassificationHuman, RequestReason: "evidence", ExpectedPlanID: "plan", ExpectedBaseSHA: strings.Repeat("b", 40), HumanEvidence: evidence}
	check, err := checkRequestForDispatch(t.TempDir(), request)
	if err != nil || check.Selector == nil || check.Selector.Mode != packsync.SelectorStableRelease {
		t.Fatalf("human evidence Check = %#v, %v", check, err)
	}
}

func TestHumanEvidenceRegistrationRetainsTheSealedSourceProposal(t *testing.T) {
	candidate := packsync.Candidate{Commit: strings.Repeat("a", 40), Release: &packsync.Release{Tag: "v1.2.0", Prerelease: false}}
	evidence, _ := json.Marshal(packsync.ClassificationEvidenceSet{Candidate: candidate})
	registration := packsync.SourceConfig{ID: "addy", Provider: "github", Repository: "addyosmani/agent-skills", Selector: packsync.Selector{Mode: packsync.SelectorStableRelease}, Resources: []packsync.Binding{}}
	digest, _ := packsyncworkflow.CanonicalRegistrationSHA256(registration)
	request := packsyncworkflow.DispatchRequest{SchemaVersion: 2, Operation: packsyncworkflow.OperationRegister, SourceID: "addy", Selector: packsyncworkflow.SelectorCommit, SelectorRef: candidate.Commit, ClassificationMode: packsyncworkflow.ClassificationHuman, RequestReason: "evidence", ExpectedPlanID: "plan", ExpectedBaseSHA: strings.Repeat("b", 40), HumanEvidence: evidence, Registration: &registration, RegistrationSHA256: digest}
	check, err := checkRequestForDispatch(t.TempDir(), request)
	if err != nil || check.Registration == nil || !reflect.DeepEqual(*check.Registration, registration) {
		t.Fatalf("human registration Check = %#v, %v", check, err)
	}
}

func TestClassificationBoundaryProducesExactSafeArtifact(t *testing.T) {
	artifact := packsyncworkflow.NewFailureArtifact(packsyncworkflow.FailureArtifactContext{SourceID: "source"}, classificationFailure(errors.New("Bearer secret model payload")))
	if !strings.Contains(artifact.Blockers[0], "Classification evidence") || !strings.Contains(artifact.Recovery[0], "classifier mode") {
		t.Fatalf("classification artifact = %#v", artifact)
	}
	data, _ := json.Marshal(artifact)
	if strings.Contains(string(data), "secret model payload") {
		t.Fatal("classification artifact serialized raw model error")
	}
}

type sourceFailureFixture struct {
	failures []error
	calls    int
}

type releasesFailureSource struct {
	source   packsync.Source
	failures []error
	calls    int
}

func (source *releasesFailureSource) Releases(ctx context.Context, config packsync.SourceConfig) ([]packsync.Release, error) {
	source.calls++
	if len(source.failures) != 0 {
		err := source.failures[0]
		source.failures = source.failures[1:]
		return nil, err
	}
	return source.source.Releases(ctx, config)
}

func (source *releasesFailureSource) ResolveRelease(ctx context.Context, config packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	return source.source.ResolveRelease(ctx, config, release)
}

func (source *releasesFailureSource) ResolveCommit(ctx context.Context, config packsync.SourceConfig, commit string) (packsync.Candidate, error) {
	return source.source.ResolveCommit(ctx, config, commit)
}

func (source *releasesFailureSource) WithSnapshot(ctx context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	return source.source.WithSnapshot(ctx, candidate, temporaryRoot, visit)
}

func (source *sourceFailureFixture) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	source.calls++
	if len(source.failures) == 0 {
		return nil, nil
	}
	err := source.failures[0]
	source.failures = source.failures[1:]
	return nil, err
}

func (*sourceFailureFixture) ResolveRelease(context.Context, packsync.SourceConfig, packsync.Release) (packsync.Candidate, error) {
	return packsync.Candidate{}, nil
}
func (*sourceFailureFixture) ResolveCommit(context.Context, packsync.SourceConfig, string) (packsync.Candidate, error) {
	return packsync.Candidate{}, nil
}
func (*sourceFailureFixture) WithSnapshot(context.Context, packsync.Candidate, string, func(string) error) error {
	return nil
}

type sourceSleeper struct{ delays []time.Duration }

func (sleeper *sourceSleeper) Sleep(_ context.Context, delay time.Duration) error {
	sleeper.delays = append(sleeper.delays, delay)
	return nil
}
