package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/packsync"
	"github.com/yersonargotev/matty/internal/packsync/githubsource"
	"github.com/yersonargotev/matty/internal/packsyncworkflow"
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

func TestPublicSourceRetriesOnlyTransientFailuresAndRespectsRetryAfter(t *testing.T) {
	underlying := &sourceFailureFixture{failures: []error{githubsource.HTTPError{Operation: "test", StatusCode: http.StatusTooManyRequests, Status: "429", RetryAfter: "4"}, githubsource.HTTPError{Operation: "test", StatusCode: http.StatusServiceUnavailable, Status: "503"}}}
	sleeper := &sourceSleeper{}
	source := retryingSource{source: underlying, policy: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, Sleeper: sleeper}}
	if _, err := source.Releases(context.Background(), packsync.SourceConfig{}); err != nil {
		t.Fatal(err)
	}
	if underlying.calls != 3 || !reflect.DeepEqual(sleeper.delays, []time.Duration{4 * time.Second, 2 * time.Second}) {
		t.Fatalf("calls=%d delays=%v", underlying.calls, sleeper.delays)
	}

	underlying = &sourceFailureFixture{failures: []error{errors.New("moved provenance")}}
	source = retryingSource{source: underlying, policy: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, Sleeper: sleeper}}
	if _, err := source.Releases(context.Background(), packsync.SourceConfig{}); err == nil || underlying.calls != 1 {
		t.Fatalf("integrity blocker retried: calls=%d err=%v", underlying.calls, err)
	}
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
	t.Setenv("MATTY_SOURCE_ID", "source")
	t.Setenv("MATTY_SELECTOR", "commit")
	t.Setenv("MATTY_SELECTOR_REF", strings.Repeat("a", 40))
	t.Setenv("MATTY_CLASSIFICATION_MODE", "ai")
	t.Setenv("MATTY_REQUEST_REASON", "fixture")
	request, check, err := inspectRequest(options{repositoryRoot: t.TempDir()})
	if err != nil || request.SourceID != "source" || check.Selector == nil || check.Selector.Mode != packsync.SelectorCommit {
		t.Fatalf("normalized request = %#v, %#v, %v", request, check, err)
	}
}

func TestInspectBoundaryEmitsCanonicalNoopArtifact(t *testing.T) {
	output := t.TempDir()
	plan := packsync.Plan{SchemaVersion: 1, PlanID: "plan-noop", Status: "no-op", SourceID: "source", Candidate: packsync.Candidate{Commit: strings.Repeat("a", 40)}, Preconditions: packsync.Preconditions{BaseCommit: strings.Repeat("b", 40)}}
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
