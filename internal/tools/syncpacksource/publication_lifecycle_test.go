package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/packsync"
	"github.com/yersonargotev/matty/internal/packsyncworkflow"
)

const (
	baseA      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	baseB      = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	candidateA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	headA      = "cccccccccccccccccccccccccccccccccccccccc"
)

func TestGitHubGatewayPristineCreateFinalizesOnlyAfterExactReobservation(t *testing.T) {
	fake := &fakeGitHubCommands{}
	gateway := lifecycleGateway(t, fake)
	proposal := lifecycleProposal()
	prepared, err := gateway.Prepare(proposal)
	if err != nil {
		t.Fatal(err)
	}
	first, err := gateway.Observe(context.Background(), proposal.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	first.ProvenanceCurrent = true
	decision, err := packsyncworkflow.EvaluatePublication(prepared, first)
	if err != nil || decision.Action != packsyncworkflow.PublicationCreate {
		t.Fatalf("create decision = %#v, %v", decision, err)
	}
	returned, err := gateway.Publish(context.Background(), prepared, decision)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := gateway.Observe(context.Background(), proposal.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if !draft.PR.Draft || draft.PR.MetadataHash != prepared.ManagedMetadataHash || fake.readyCalls != 0 {
		t.Fatalf("first publication was not protected draft: %#v", draft.PR)
	}
	finalHash, err := gateway.Finalize(context.Background(), prepared, decision, draft.PR)
	if err != nil {
		t.Fatal(err)
	}
	final, err := gateway.Observe(context.Background(), proposal.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if returned.Number != 7 || final.PR.Draft || final.PR.MetadataHash != finalHash || fake.readyCalls != 1 || fake.createCalls != 1 {
		t.Fatalf("final state = %#v create=%d ready=%d", final.PR, fake.createCalls, fake.readyCalls)
	}
	identity := packsyncworkflow.ReadinessIdentity{PlanID: prepared.PlanID, BaseSHA: prepared.BaseSHA, HeadSHA: final.PR.HeadSHA, CandidateSHA: prepared.CandidateSHA, ProvenanceSHA256: prepared.ProvenanceSHA256, PRNumber: final.PR.Number, PRStateSHA256: final.PR.MetadataHash}
	if ready, err := packsyncworkflow.MarkDecisionReady(identity, prepared.Validation, final.PR.Draft, final.PR.AutoMerge); err != nil || !ready.DecisionReady {
		t.Fatalf("readiness = %#v, %v", ready, err)
	}
}

func TestGitHubGatewayNeverOverwritesEditedMetadata(t *testing.T) {
	fake := &fakeGitHubCommands{branchHead: headA, pr: &fakePR{number: 7, open: true, head: headA, title: "managed", body: "reviewer edit", draft: false}}
	gateway := lifecycleGateway(t, fake)
	gateway.title = "managed"
	state, err := gateway.Observe(context.Background(), "mattpocock-skills")
	if err != nil {
		t.Fatal(err)
	}
	before := fake.editCalls
	err = gateway.editPRWithReobserve(context.Background(), "mattpocock-skills", 7, strings.Repeat("0", 64), "replacement", strings.Repeat("1", 64))
	if err == nil || fake.editCalls != before || state.PR.MetadataHash == strings.Repeat("0", 64) {
		t.Fatalf("reviewer metadata was overwritten: state=%#v edits=%d err=%v", state.PR, fake.editCalls, err)
	}
}

func TestGitHubGatewayObservationFailureNeverBecomesBranchAbsence(t *testing.T) {
	fake := &fakeGitHubCommands{lsRemoteErr: errors.New("HTTP 503 Retry-After: 1")}
	gateway := lifecycleGateway(t, fake)
	if _, err := gateway.Observe(context.Background(), "mattpocock-skills"); err == nil {
		t.Fatal("failed branch observation was treated as authoritative absence")
	}
	if fake.pushCalls != 0 || fake.createCalls != 0 {
		t.Fatalf("observation failure wrote state: pushes=%d creates=%d", fake.pushCalls, fake.createCalls)
	}
}

func TestGitHubGatewayCompareFailureRemainsTransient(t *testing.T) {
	oldCandidate := strings.Repeat("d", 40)
	prefix, title := "managed evidence", "managed"
	hash := packsyncworkflow.ManagedMetadataHash(title, prefix)
	record := packsyncworkflow.PublicationRecord{PlanID: "old-plan", BaseSHA: baseA, CandidateSHA: oldCandidate, HeadSHA: headA, ProvenanceSHA256: strings.Repeat("5", 64), MetadataHash: hash}
	body, err := packsyncworkflow.ManagedBody(prefix, record)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeGitHubCommands{branchHead: headA, compareErr: errors.New("HTTP 503 Retry-After: 1"), pr: &fakePR{number: 7, open: true, head: headA, title: title, body: body}}
	gateway := lifecycleGateway(t, fake)
	_, err = gateway.Observe(context.Background(), "mattpocock-skills")
	var failure packsyncworkflow.Failure
	if !errors.As(err, &failure) || failure.Kind != packsyncworkflow.FailureTransient {
		t.Fatalf("compare failure became regression: %T %v", err, err)
	}
}

func TestGitHubGatewayFailedUndraftKeepsBlockedMetadata(t *testing.T) {
	fake := &fakeGitHubCommands{readyErr: errors.New("HTTP 503")}
	gateway := lifecycleGateway(t, fake)
	proposal, err := gateway.Prepare(lifecycleProposal())
	if err != nil {
		t.Fatal(err)
	}
	state, err := gateway.Observe(context.Background(), proposal.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	state.ProvenanceCurrent = true
	decision, err := packsyncworkflow.EvaluatePublication(proposal, state)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := gateway.Publish(context.Background(), proposal, decision)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := gateway.Observe(context.Background(), proposal.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	before := fake.pr.body
	if _, err := gateway.Finalize(context.Background(), proposal, decision, draft.PR); err == nil {
		t.Fatal("failed undraft finalized readiness")
	}
	if pr.Number != 7 || fake.readyCalls != 3 || !fake.pr.draft || fake.pr.body != before || !strings.Contains(fake.pr.body, `"decision_ready": false`) {
		t.Fatalf("failed undraft changed blocked metadata: %#v", fake.pr)
	}
}

func TestGitHubGatewayFailedFirstPRCreationReportsExactOrphanRecovery(t *testing.T) {
	fake := &fakeGitHubCommands{createErr: errors.New("HTTP 503")}
	gateway := lifecycleGateway(t, fake)
	proposal, err := gateway.Prepare(lifecycleProposal())
	if err != nil {
		t.Fatal(err)
	}
	state, err := gateway.Observe(context.Background(), proposal.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	state.ProvenanceCurrent = true
	decision, err := packsyncworkflow.EvaluatePublication(proposal, state)
	if err != nil {
		t.Fatal(err)
	}
	_, err = gateway.Publish(context.Background(), proposal, decision)
	artifact := packsyncworkflow.NewFailureArtifact(packsyncworkflow.FailureArtifactContext{SourceID: proposal.SourceID}, err)
	if err == nil || !strings.Contains(artifact.Blockers[0], "stable branch was pushed") || !strings.Contains(artifact.Recovery[0], "delete only that exact orphan branch") {
		t.Fatalf("orphan recovery = %#v err=%v", artifact, err)
	}
}

func TestGitHubGatewayPristineNoopPerformsNoWrites(t *testing.T) {
	proposal := lifecycleProposal()
	pr := managedFakePR(t, proposal, proposal.ManagedTitle, "existing exact evidence", false)
	fake := &fakeGitHubCommands{branchHead: headA, pr: pr}
	gateway := lifecycleGateway(t, fake)
	prepared, err := gateway.Prepare(proposal)
	if err != nil {
		t.Fatal(err)
	}
	state, err := gateway.Observe(context.Background(), proposal.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	state.ProvenanceCurrent = true
	decision, err := packsyncworkflow.EvaluatePublication(prepared, state)
	if err != nil || decision.Action != packsyncworkflow.PublicationNoop || fake.pushCalls != 0 || fake.editCalls != 0 || fake.createCalls != 0 {
		t.Fatalf("no-op crossed write boundary: decision=%#v pushes=%d edits=%d creates=%d err=%v", decision, fake.pushCalls, fake.editCalls, fake.createCalls, err)
	}
}

func TestGitHubGatewayPristineUpdateUsesStableBranchAndSamePR(t *testing.T) {
	proposal := lifecycleProposal()
	old := proposal
	old.PlanID = "old-plan"
	old.CandidateSHA = strings.Repeat("d", 40)
	old.ProvenanceSHA256 = strings.Repeat("8", 64)
	pr := managedFakePR(t, old, "old managed", "old evidence", false)
	fake := &fakeGitHubCommands{branchHead: headA, pr: pr}
	gateway := lifecycleGateway(t, fake)
	prepared, err := gateway.Prepare(proposal)
	if err != nil {
		t.Fatal(err)
	}
	state, err := gateway.Observe(context.Background(), proposal.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	state.ProvenanceCurrent = true
	decision, err := packsyncworkflow.EvaluatePublication(prepared, state)
	if err != nil || decision.Action != packsyncworkflow.PublicationUpdate || decision.PRNumber != 7 {
		t.Fatalf("update decision = %#v, %v", decision, err)
	}
	if _, err := gateway.Publish(context.Background(), prepared, decision); err != nil {
		t.Fatal(err)
	}
	if fake.createCalls != 0 || fake.pushCalls != 1 || fake.editCalls != 1 || fake.pr.number != 7 {
		t.Fatalf("update did not preserve one branch/PR: %#v", fake)
	}
}

func TestGitHubGatewayLifecycleBlockersNeverWrite(t *testing.T) {
	proposal := lifecycleProposal()
	for name, configure := range map[string]func(*fakeGitHubCommands){
		"base advanced": func(fake *fakeGitHubCommands) { fake.baseHead = baseB },
		"closed PR":     func(fake *fakeGitHubCommands) { fake.pr.open = false },
		"human commits": func(fake *fakeGitHubCommands) {
			fake.branchHead, fake.pr.head = baseB, baseB
		},
		"spoofed bot commit author": func(fake *fakeGitHubCommands) { fake.pr.author = "reviewer" },
		"human commit identity":     func(fake *fakeGitHubCommands) { fake.commitOwner = "reviewer" },
		"regressive candidate": func(fake *fakeGitHubCommands) {
			fake.pr = managedFakePR(t, packsyncworkflow.Proposal{SourceID: proposal.SourceID, PlanID: "newer", BaseSHA: baseA, CandidateSHA: strings.Repeat("d", 40), ResultTreeSHA: headA, ProvenanceSHA256: strings.Repeat("8", 64)}, "old managed", "old evidence", false)
			fake.compareStatus = "behind"
		},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeGitHubCommands{branchHead: headA, pr: managedFakePR(t, proposal, "managed", "evidence", false)}
			configure(fake)
			gateway := lifecycleGateway(t, fake)
			prepared, err := gateway.Prepare(proposal)
			if err != nil {
				t.Fatal(err)
			}
			state, err := gateway.Observe(context.Background(), proposal.SourceID)
			if err != nil {
				t.Fatal(err)
			}
			state.ProvenanceCurrent = true
			if _, err := packsyncworkflow.EvaluatePublication(prepared, state); err == nil {
				t.Fatal("unsafe lifecycle state was admitted")
			}
			if fake.pushCalls != 0 || fake.editCalls != 0 || fake.createCalls != 0 {
				t.Fatalf("blocker wrote state: %#v", fake)
			}
		})
	}
}

func managedFakePR(t *testing.T, proposal packsyncworkflow.Proposal, title, prefix string, draft bool) *fakePR {
	t.Helper()
	hash := packsyncworkflow.ManagedMetadataHash(title, prefix)
	record := packsyncworkflow.NewPublicationRecord(proposal, proposal.ResultTreeSHA, hash)
	body, err := packsyncworkflow.ManagedBody(prefix, record)
	if err != nil {
		t.Fatal(err)
	}
	return &fakePR{number: 7, open: true, head: proposal.ResultTreeSHA, title: title, body: body, draft: draft}
}

func lifecycleGateway(t *testing.T, fake *fakeGitHubCommands) *githubGateway {
	t.Helper()
	return &githubGateway{repositoryRoot: t.TempDir(), repository: "owner/repo", plan: packsync.Plan{Candidate: packsync.Candidate{Repository: "owner/upstream", Commit: candidateA}}, retry: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Nanosecond, Sleeper: noWaitSleeper{}}, run: fake.run, brief: lifecycleBrief()}
}

func lifecycleProposal() packsyncworkflow.Proposal {
	return packsyncworkflow.Proposal{SourceID: "mattpocock-skills", PlanID: "plan-1", BaseSHA: baseA, CandidateSHA: candidateA, ResultTreeSHA: headA, ProvenanceSHA256: strings.Repeat("5", 64), ManagedTitle: "sync(mattpocock-skills): candidate", Validation: packsyncworkflow.ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, MattySuite: true}, InvalidationConditions: packsyncworkflow.DecisionReadyInvalidationConditions()}
}

func lifecycleBrief() packsyncworkflow.ReviewBrief {
	return packsyncworkflow.ReviewBrief{SchemaVersion: 1, Actor: "maintainer", RunID: "1", RunAttempt: "1", RunURL: "https://github.com/owner/repo/actions/runs/1", Request: packsyncworkflow.DispatchRequest{SchemaVersion: 1, SourceID: "mattpocock-skills", Selector: packsyncworkflow.SelectorCommit, SelectorRef: candidateA, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "test"}, Candidate: packsync.Candidate{Commit: candidateA}, PlanID: "plan-1", BaseSHA: baseA, HeadSHA: headA, Branch: "sync/mattpocock-skills", SelectedResources: []packsync.ResourceEvidence{{SHA256: strings.Repeat("4", 64)}}, PreviousSnapshotSHA256: strings.Repeat("3", 64), ProposedSnapshotSHA256: strings.Repeat("5", 64), ApplyStatus: "applied", ManualMergeRequired: true}
}

type noWaitSleeper struct{}

func (noWaitSleeper) Sleep(context.Context, time.Duration) error { return nil }

type fakePR struct {
	number int
	open   bool
	head   string
	title  string
	body   string
	draft  bool
	author string
}

type fakeGitHubCommands struct {
	baseHead      string
	branchHead    string
	pr            *fakePR
	lsRemoteErr   error
	compareErr    error
	compareStatus string
	readyErr      error
	createErr     error
	commitOwner   string
	pushCalls     int
	createCalls   int
	editCalls     int
	readyCalls    int
}

func (fake *fakeGitHubCommands) run(_ context.Context, directory string, name string, args ...string) (string, error) {
	joined := name + " " + strings.Join(args, " ")
	switch {
	case strings.HasPrefix(joined, "gh api repos/owner/repo/git/ref/heads/main"):
		if fake.baseHead != "" {
			return fake.baseHead + "\n", nil
		}
		return baseA + "\n", nil
	case strings.HasPrefix(joined, "git ls-remote"):
		if fake.lsRemoteErr != nil {
			return "", fake.lsRemoteErr
		}
		if fake.branchHead == "" {
			return "", nil
		}
		return fake.branchHead + "\trefs/heads/sync/mattpocock-skills\n", nil
	case strings.HasPrefix(joined, "gh pr list"):
		if fake.pr == nil {
			return "[]", nil
		}
		state := "CLOSED"
		if fake.pr.open {
			state = "OPEN"
		}
		author := fake.pr.author
		if author == "" {
			author = "app/github-actions"
		}
		data, _ := json.Marshal([]map[string]any{{"number": fake.pr.number, "state": state, "baseRefName": "main", "headRefName": "sync/mattpocock-skills", "headRefOid": fake.pr.head, "isDraft": fake.pr.draft, "autoMergeRequest": nil, "title": fake.pr.title, "body": fake.pr.body, "author": map[string]string{"login": author}}})
		return string(data), nil
	case strings.Contains(joined, "/compare/"):
		if fake.compareErr != nil {
			return "", fake.compareErr
		}
		if fake.compareStatus != "" {
			return fake.compareStatus + "\n", nil
		}
		return "ahead\n", nil
	case strings.Contains(joined, "/commits/"):
		owner := fake.commitOwner
		if owner == "" {
			owner = packsyncworkflow.AutomationOwner
		}
		record, _ := packsyncworkflow.ParsePublicationRecord(fake.pr.body)
		message := fmt.Sprintf("sync(mattpocock-skills): %s [%s]", record.CandidateSHA[:12], record.PlanID)
		data, _ := json.Marshal(map[string]any{"author": owner, "committer": owner, "message": message, "parents": []string{record.BaseSHA}})
		return string(data), nil
	case strings.HasPrefix(joined, "git push"):
		fake.pushCalls++
		fake.branchHead = headA
		if output, err := exec.Command("git", "-C", directory, "rev-parse", "HEAD").Output(); err == nil {
			fake.branchHead = strings.TrimSpace(string(output))
		}
		if fake.pr != nil {
			fake.pr.head = headA
		}
		return "", nil
	case strings.HasPrefix(joined, "gh pr create"):
		fake.createCalls++
		if fake.createErr != nil {
			return "", fake.createErr
		}
		fake.pr = &fakePR{number: 7, open: true, head: fake.branchHead, title: argumentAfter(args, "--title"), body: argumentAfter(args, "--body"), draft: true}
		return "https://github.com/owner/repo/pull/7", nil
	case strings.HasPrefix(joined, "gh pr edit"):
		if fake.pr == nil {
			return "", errors.New("missing PR")
		}
		fake.editCalls++
		fake.pr.title, fake.pr.body = argumentAfter(args, "--title"), argumentAfter(args, "--body")
		return "", nil
	case strings.HasPrefix(joined, "gh pr ready"):
		fake.readyCalls++
		if fake.readyErr != nil {
			return "", fake.readyErr
		}
		fake.pr.draft = false
		return "", nil
	default:
		return "", fmt.Errorf("unexpected command: %s", joined)
	}
}

func argumentAfter(args []string, name string) string {
	for index := range args {
		if args[index] == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}
