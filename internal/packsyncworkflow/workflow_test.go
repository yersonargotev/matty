package packsyncworkflow

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/packsync"
)

const (
	baseA      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	baseB      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	candidateA = "1111111111111111111111111111111111111111"
	candidateB = "2222222222222222222222222222222222222222"
	headA      = "3333333333333333333333333333333333333333"
	headB      = "4444444444444444444444444444444444444444"
	treeA      = "5555555555555555555555555555555555555555"
)

func TestDispatchRequestAcceptsOnlyCanonicalManualInputs(t *testing.T) {
	valid := []DispatchRequest{
		{SchemaVersion: 1, SourceID: "mattpocock-skills", Selector: SelectorLatestStable, ClassificationMode: ClassificationAI, RequestReason: "Update stable skills."},
		{SchemaVersion: 1, SourceID: "mattpocock-skills", Selector: SelectorPrerelease, SelectorRef: "v1.2.0-beta.1", ClassificationMode: ClassificationAI, RequestReason: "Inspect prerelease."},
		{SchemaVersion: 1, SourceID: "mattpocock-skills", Selector: SelectorCommit, SelectorRef: candidateA, ClassificationMode: ClassificationHuman, RequestReason: "Human inspection."},
		{SchemaVersion: 1, SourceID: "mattpocock-skills", Selector: SelectorCommit, SelectorRef: candidateA, ClassificationMode: ClassificationHuman, RequestReason: "Supply evidence.", ExpectedPlanID: "plan-1", ExpectedBaseSHA: baseA, HumanEvidence: []byte(`{"schema_version":1}`)},
	}
	for _, request := range valid {
		if err := request.Validate(); err != nil {
			t.Fatalf("valid request %#v: %v", request, err)
		}
	}
	invalid := []DispatchRequest{
		{},
		{SchemaVersion: 1, SourceID: "../source", Selector: SelectorLatestStable, ClassificationMode: ClassificationAI, RequestReason: "bad"},
		{SchemaVersion: 1, SourceID: "source", Selector: SelectorLatestStable, SelectorRef: "main", ClassificationMode: ClassificationAI, RequestReason: "bad"},
		{SchemaVersion: 1, SourceID: "source", Selector: SelectorCommit, SelectorRef: "abcd", ClassificationMode: ClassificationAI, RequestReason: "bad"},
		{SchemaVersion: 1, SourceID: "source", Selector: SelectorPrerelease, SelectorRef: "main", ClassificationMode: ClassificationAI, RequestReason: "bad"},
		{SchemaVersion: 1, SourceID: "source", Selector: SelectorLatestStable, ClassificationMode: "fallback", RequestReason: "bad"},
		{SchemaVersion: 1, SourceID: "source", Selector: SelectorLatestStable, ClassificationMode: ClassificationAI, RequestReason: "bad", HumanEvidence: []byte(`{}`)},
	}
	for _, request := range invalid {
		if err := request.Validate(); err == nil {
			t.Fatalf("invalid request accepted: %#v", request)
		}
	}
}

func TestDispatchReasonLengthMatchesJSONSchemaUnicodeCharacters(t *testing.T) {
	request := DispatchRequest{SchemaVersion: 1, SourceID: "source", Selector: SelectorLatestStable, ClassificationMode: ClassificationAI, RequestReason: strings.Repeat("é", 500)}
	if err := request.Validate(); err != nil {
		t.Fatalf("500 Unicode characters rejected: %v", err)
	}
	request.RequestReason += "é"
	if err := request.Validate(); err == nil {
		t.Fatal("501 Unicode characters accepted")
	}
	request.RequestReason = " \t "
	if err := request.Validate(); err == nil {
		t.Fatal("whitespace-only reason accepted")
	}
}

func TestRetryPolicyRetriesOnlyTransientFailuresWithBackoffAndRetryAfter(t *testing.T) {
	sleeper := &recordingSleeper{}
	policy := RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second, Sleeper: sleeper}
	attempts := 0
	err := policy.Do(context.Background(), func() error {
		attempts++
		switch attempts {
		case 1:
			return Failure{Kind: FailureTransient, Err: errors.New("network")}
		case 2:
			return Failure{Kind: FailureTransient, RetryAfter: 7 * time.Second, Err: errors.New("rate limited")}
		default:
			return nil
		}
	})
	if err != nil || attempts != 3 || !reflect.DeepEqual(sleeper.delays, []time.Duration{time.Second, 7 * time.Second}) {
		t.Fatalf("retry = attempts %d delays %v err %v", attempts, sleeper.delays, err)
	}

	for _, kind := range []FailureKind{FailureProvenance, FailureIntegrity, FailureClassification, FailureValidation, FailureOwnership, FailureDivergence} {
		attempts = 0
		sleeper.delays = nil
		err = policy.Do(context.Background(), func() error {
			attempts++
			return Failure{Kind: kind, Err: errors.New("blocked")}
		})
		if err == nil || attempts != 1 || len(sleeper.delays) != 0 {
			t.Fatalf("kind %s retried: attempts %d delays %v err %v", kind, attempts, sleeper.delays, err)
		}
	}
}

func TestPublicationLifecycleCreatesUpdatesAndNoOpsOnlyForPristineOwnership(t *testing.T) {
	proposal := validProposal()
	created, err := EvaluatePublication(proposal, PublicationState{BaseSHA: baseA, ProvenanceCurrent: true})
	if err != nil || created.Action != PublicationCreate || created.Branch != "sync/mattpocock-skills" {
		t.Fatalf("create = %#v, %v", created, err)
	}

	pristine := pristineState()
	updated := proposal
	updated.CandidateSHA = candidateB
	updated.PlanID = "plan-2"
	updated.ResultTreeSHA = headB
	updated.HeadSHA = headB
	pristine.CandidateRelation = CandidateAdvancing
	decision, err := EvaluatePublication(updated, pristine)
	if err != nil || decision.Action != PublicationUpdate || decision.PRNumber != 7 {
		t.Fatalf("update = %#v, %v", decision, err)
	}

	decision, err = EvaluatePublication(proposal, pristine)
	if err != nil || decision.Action != PublicationNoop {
		t.Fatalf("no-op = %#v, %v", decision, err)
	}
}

func TestPublicationFailsClosedBeforeWritingReviewerOrStaleState(t *testing.T) {
	proposal := validProposal()
	cases := map[string]func(*PublicationState){
		"base advanced":       func(s *PublicationState) { s.BaseSHA = baseB },
		"candidate regressed": func(s *PublicationState) { s.CandidateRelation = CandidateRegressive },
		"branch divergent":    func(s *PublicationState) { s.Branch.Diverged = true },
		"metadata edited":     func(s *PublicationState) { s.PR.MetadataHash = "edited" },
		"human commits":       func(s *PublicationState) { s.Branch.HumanCommits = true },
		"ownership ambiguous": func(s *PublicationState) { s.Branch.Owner = "" },
		"PR closed":           func(s *PublicationState) { s.PR.Open = false },
		"unexpected identity": func(s *PublicationState) { s.PR.HeadBranch = "other" },
		"stale plan":          func(s *PublicationState) { s.Record.PlanID = "other" },
		"moved provenance":    func(s *PublicationState) { s.ProvenanceCurrent = false },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			state := pristineState()
			mutate(&state)
			decision, err := EvaluatePublication(proposal, state)
			if err == nil || decision.Action != PublicationBlock || len(decision.Blockers) == 0 {
				t.Fatalf("decision = %#v, %v", decision, err)
			}
			if state.Writes != 0 {
				t.Fatalf("evaluation mutated publication state: %#v", state)
			}
		})
	}
}

func TestOperationalArtifactIsCanonicalAndRejectsSensitiveOrUpstreamPayloads(t *testing.T) {
	artifact := FailureArtifact{SchemaVersion: 1, State: "blocked", SourceID: "source", PlanID: "plan", BaseSHA: baseA, CandidateSHA: candidateA, Blockers: []string{"branch ownership is ambiguous"}, Recovery: []string{"Restore the managed metadata or close the PR, then dispatch again."}, ContainsSecrets: false, ContainsUpstreamBytes: false}
	data, err := artifact.CanonicalJSON()
	if err != nil || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("canonical artifact = %q, %v", data, err)
	}
	if strings.Contains(string(data), "token") || strings.Contains(string(data), "upstream_payload") {
		t.Fatalf("artifact leaks forbidden fields: %s", data)
	}
	artifact.ContainsSecrets = true
	if _, err := artifact.CanonicalJSON(); err == nil {
		t.Fatal("secret-bearing artifact accepted")
	}
	artifact.ContainsSecrets = false
	artifact.ContainsUpstreamBytes = true
	if _, err := artifact.CanonicalJSON(); err == nil {
		t.Fatal("upstream-byte-bearing artifact accepted")
	}
}

func TestFailureArtifactNeverSerializesRawBoundaryErrors(t *testing.T) {
	artifact := NewFailureArtifact(FailureArtifactContext{SourceID: "source"}, Failure{Kind: FailureValidation, Err: errors.New("Bearer secret-token upstream payload bytes")})
	data, err := artifact.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-token", "upstream payload", "Bearer"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("operational artifact leaked %q: %s", forbidden, data)
		}
	}
}

func TestPublicationFailureArtifactPreservesExactSafeRecovery(t *testing.T) {
	state := pristineState()
	state.PR.MetadataHash = strings.Repeat("9", 64)
	decision, err := EvaluatePublication(validProposal(), state)
	if err == nil || len(decision.Blockers) != 1 {
		t.Fatalf("edited metadata = %#v, %v", decision, err)
	}
	artifact := NewFailureArtifact(FailureArtifactContext{SourceID: "mattpocock-skills"}, err)
	if artifact.Blockers[0] != "managed pull request metadata was edited" || !strings.Contains(artifact.Recovery[0], "must not overwrite") {
		t.Fatalf("artifact lost exact safe recovery: %#v", artifact)
	}
}

func TestValidationArtifactBindsSandboxProofWithoutUpstreamBytes(t *testing.T) {
	artifact := ValidationArtifact{SchemaVersion: 1, SourceID: "mattpocock-skills", PlanID: "plan-1", BaseSHA: baseA, CandidateSHA: candidateA, MattySuite: true, Apply: true}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("valid proof: %v", err)
	}
	artifact.UpstreamBytes = true
	if err := artifact.Validate(); err == nil {
		t.Fatal("validation proof accepted upstream bytes")
	}
}

func TestDecisionReadinessBindsExactIdentityAndInvalidatesOnAnyLaterChange(t *testing.T) {
	identity := ReadinessIdentity{PlanID: "plan-1", BaseSHA: baseA, HeadSHA: headA, CandidateSHA: candidateA, ProvenanceSHA256: strings.Repeat("5", 64), PRNumber: 7, PRStateSHA256: strings.Repeat("6", 64)}
	gates := ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, MattySuite: true}
	ready, err := MarkDecisionReady(identity, gates, false, false)
	if err != nil || !ready.DecisionReady || ready.AutoMerge || !ready.ManualMergeRequired {
		t.Fatalf("readiness = %#v, %v", ready, err)
	}
	mutations := []func(*ReadinessIdentity){
		func(v *ReadinessIdentity) { v.BaseSHA = baseB },
		func(v *ReadinessIdentity) { v.HeadSHA = headB },
		func(v *ReadinessIdentity) { v.CandidateSHA = candidateB },
		func(v *ReadinessIdentity) { v.ProvenanceSHA256 = strings.Repeat("7", 64) },
		func(v *ReadinessIdentity) { v.PRStateSHA256 = strings.Repeat("8", 64) },
	}
	for _, mutate := range mutations {
		observed := identity
		mutate(&observed)
		if !ready.InvalidatedBy(observed) {
			t.Fatalf("readiness survived identity change: %#v", observed)
		}
	}
}

func TestMarkdownBriefRendersTheSameCanonicalEvidenceWithoutUpstreamBytes(t *testing.T) {
	request := DispatchRequest{SchemaVersion: 1, SourceID: "source", Selector: SelectorCommit, SelectorRef: candidateA, ClassificationMode: ClassificationAI, RequestReason: "fixture"}
	brief := ReviewBrief{SchemaVersion: 1, Actor: "maintainer", RunID: "1", RunAttempt: "1", RunURL: "https://github.com/owner/repo/actions/runs/1", Request: request, Candidate: packsync.Candidate{Commit: candidateA}, PlanID: "plan", BaseSHA: baseA, HeadSHA: headA, ResultTreeSHA: treeA, Branch: "sync/source", SelectedResources: []packsync.ResourceEvidence{{SHA256: strings.Repeat("4", 64)}}, PreviousSnapshotSHA256: strings.Repeat("3", 64), ProposedSnapshotSHA256: strings.Repeat("5", 64), ApplyStatus: "applied", Validation: ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, MattySuite: true}, DecisionReady: true, ManualMergeRequired: true, InvalidationConditions: []string{"base_changed", "candidate_changed", "provenance_changed", "head_changed", "pr_state_changed"}}
	canonical, err := brief.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := brief.Markdown()
	if err != nil || !strings.Contains(markdown, strings.TrimSpace(string(canonical))) {
		t.Fatalf("Markdown did not render canonical JSON: %v\n%s", err, markdown)
	}
}

func TestManagedPublicationRecordDetectsEditedMetadata(t *testing.T) {
	record := PublicationRecord{PlanID: "plan", BaseSHA: baseA, CandidateSHA: candidateA, HeadSHA: headA, ResultTreeSHA: headA, MetadataHash: strings.Repeat("5", 64)}
	body, err := ManagedBody("canonical brief", record)
	if err != nil {
		t.Fatal(err)
	}
	parsed, ok := ParsePublicationRecord(body)
	if !ok || parsed != record {
		t.Fatalf("parsed record = %#v, %v", parsed, ok)
	}
	want := ManagedMetadataHash("title", body)
	if got := ManagedMetadataHash("title", "edited\n"+body); got == want {
		t.Fatal("edited managed body retained metadata identity")
	}
}

func TestHumanClassificationRemainsInspectionFirstEvidenceSecond(t *testing.T) {
	first := DispatchRequest{SchemaVersion: 1, SourceID: "source", Selector: SelectorLatestStable, ClassificationMode: ClassificationHuman, RequestReason: "inspect"}
	if phase, err := first.HumanPhase(); err != nil || phase != HumanInspect {
		t.Fatalf("first phase = %q, %v", phase, err)
	}
	second := DispatchRequest{SchemaVersion: 1, SourceID: "source", Selector: SelectorCommit, SelectorRef: candidateA, ClassificationMode: ClassificationHuman, RequestReason: "classify", ExpectedPlanID: "plan", ExpectedBaseSHA: baseA, HumanEvidence: []byte(`{"schema_version":1}`)}
	if phase, err := second.HumanPhase(); err != nil || phase != HumanEvidence {
		t.Fatalf("second phase = %q, %v", phase, err)
	}
}

func TestConcurrencySupersedesOnlyPendingAndPromotesThroughFreshCheck(t *testing.T) {
	state := ConcurrencyState{}
	if state.Admit("active") != AdmissionActive || state.Admit("pending-1") != AdmissionPending || state.Admit("pending-2") != AdmissionSuperseded {
		t.Fatalf("admission state = %#v", state)
	}
	if state.Active.Identity != "active" || state.Pending.Identity != "pending-2" {
		t.Fatalf("new request canceled active or failed to replace pending: %#v", state)
	}
	promoted := state.CompleteActive()
	if promoted == nil || promoted.Identity != "pending-2" || !promoted.NeedsFreshCheck || state.Pending != nil {
		t.Fatalf("promoted request did not require fresh Check: %#v, %#v", promoted, state)
	}
}

func TestPublishRunsApplyAndCompleteValidationBeforeGitHubAndRevalidatesBeforeWrite(t *testing.T) {
	events := []string{}
	applier := fakeApplier{events: &events}
	validator := fakeValidator{events: &events}
	github := &fakePublicationGateway{events: &events, states: []PublicationState{{BaseSHA: baseA, ProvenanceCurrent: true}, {BaseSHA: baseA, ProvenanceCurrent: true}, publishedState(true, strings.Repeat("6", 64)), publishedState(false, strings.Repeat("7", 64))}}
	result, err := (Publisher{Applier: applier, Validator: validator, Builder: fakeProposalBuilder{events: &events}, Diff: fakeDiff{}, Provenance: fakeProvenance{events: &events}, GitHub: github}).Run(context.Background(), PublishRequest{RepositoryRoot: t.TempDir()})
	if err != nil || result.Decision.Action != PublicationCreate {
		t.Fatalf("publish = %#v, %v", result, err)
	}
	want := []string{"apply", "validate", "build", "prepare", "provenance", "observe", "provenance", "observe", "publish", "observe", "provenance", "finalize", "provenance", "observe"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}

	events = nil
	github = &fakePublicationGateway{events: &events, states: []PublicationState{{BaseSHA: baseA, ProvenanceCurrent: true}, {BaseSHA: baseB, ProvenanceCurrent: true}}}
	_, err = (Publisher{Applier: fakeApplier{events: &events}, Validator: fakeValidator{events: &events}, Builder: fakeProposalBuilder{events: &events}, Diff: fakeDiff{}, Provenance: fakeProvenance{events: &events}, GitHub: github}).Run(context.Background(), PublishRequest{RepositoryRoot: t.TempDir()})
	if err == nil || contains(events, "publish") {
		t.Fatalf("changed final state wrote: events %v err %v", events, err)
	}
}

func TestMovedProvenanceAfterDraftNeverFinalizesReadiness(t *testing.T) {
	events := []string{}
	github := &fakePublicationGateway{events: &events, states: []PublicationState{{BaseSHA: baseA}, {BaseSHA: baseA}, publishedState(true, strings.Repeat("6", 64))}}
	provenance := &sequenceProvenance{events: &events, failAt: 3}
	_, err := (Publisher{Applier: fakeApplier{events: &events}, Validator: fakeValidator{events: &events}, Builder: fakeProposalBuilder{events: &events}, Diff: fakeDiff{}, Provenance: provenance, GitHub: github}).Run(context.Background(), PublishRequest{RepositoryRoot: t.TempDir()})
	if err == nil || contains(events, "finalize") {
		t.Fatalf("moved provenance finalized readiness: events=%v err=%v", events, err)
	}
}

func TestFailedApplyOrValidationNeverObservesOrWritesGitHub(t *testing.T) {
	for name, test := range map[string]struct {
		applier   fakeApplier
		validator fakeValidator
	}{
		"apply":      {applier: fakeApplier{err: errors.New("provenance")}},
		"validation": {validator: fakeValidator{err: errors.New("suite")}},
	} {
		t.Run(name, func(t *testing.T) {
			events := []string{}
			applier := test.applier
			validator := test.validator
			applier.events = &events
			validator.events = &events
			github := &fakePublicationGateway{events: &events}
			_, err := (Publisher{Applier: applier, Validator: validator, Builder: fakeProposalBuilder{events: &events}, Diff: fakeDiff{}, Provenance: fakeProvenance{events: &events}, GitHub: github}).Run(context.Background(), PublishRequest{RepositoryRoot: t.TempDir()})
			if err == nil || github.publishCalls != 0 || contains(events, "observe") {
				t.Fatalf("pre-publication failure crossed GitHub boundary: %v, events %v", err, events)
			}
		})
	}
}

func TestValidatorOrBuilderCannotChangeSealedApplyDiff(t *testing.T) {
	for name, diff := range map[string]fakeDiff{
		"validator mutation": {workspaceErr: errors.New("changed")},
		"commit mutation":    {commitErr: errors.New("changed")},
	} {
		t.Run(name, func(t *testing.T) {
			events := []string{}
			github := &fakePublicationGateway{events: &events}
			_, err := (Publisher{Applier: fakeApplier{events: &events}, Validator: fakeValidator{events: &events}, Builder: fakeProposalBuilder{events: &events}, Diff: diff, Provenance: fakeProvenance{events: &events}, GitHub: github}).Run(context.Background(), PublishRequest{RepositoryRoot: t.TempDir()})
			if err == nil || github.publishCalls != 0 || contains(events, "observe") {
				t.Fatalf("changed diff crossed publication boundary: events=%v err=%v", events, err)
			}
		})
	}
}

func TestPublisherKeepsValidatedTreeDistinctFromCommitHead(t *testing.T) {
	events := []string{}
	diff := &recordingDiff{seal: treeA, commitErr: errors.New("stop after recording")}
	github := &fakePublicationGateway{events: &events}
	_, err := (Publisher{Applier: fakeApplier{events: &events}, Validator: fakeValidator{events: &events}, Builder: fakeProposalBuilder{events: &events}, Diff: diff, Provenance: fakeProvenance{events: &events}, GitHub: github}).Run(context.Background(), PublishRequest{RepositoryRoot: t.TempDir()})
	if err == nil || diff.verifiedTree != treeA || diff.verifiedHead != headA || github.publishCalls != 0 {
		t.Fatalf("tree/head verification = tree:%s head:%s writes:%d err:%v", diff.verifiedTree, diff.verifiedHead, github.publishCalls, err)
	}
}

type recordingSleeper struct{ delays []time.Duration }

func (s *recordingSleeper) Sleep(_ context.Context, delay time.Duration) error {
	s.delays = append(s.delays, delay)
	return nil
}

type fakeApplier struct {
	events *[]string
	err    error
}

func (fake fakeApplier) Apply(context.Context, packsync.ApplyRequest) (packsync.ApplyResult, error) {
	*fake.events = append(*fake.events, "apply")
	return packsync.ApplyResult{Status: "applied", Changed: true}, fake.err
}

func (fake fakeApplier) RecoverPending(context.Context, string) (packsync.ApplyResult, bool, error) {
	return packsync.ApplyResult{}, false, nil
}

type fakeValidator struct {
	events *[]string
	err    error
}

type fakeProposalBuilder struct{ events *[]string }

func (fake fakeProposalBuilder) Build(context.Context, string, packsync.ApplyResult) (Proposal, error) {
	*fake.events = append(*fake.events, "build")
	return validProposal(), nil
}

type fakeDiff struct {
	workspaceErr error
	commitErr    error
}

func (fakeDiff) Seal(context.Context, string) (string, error)               { return headA, nil }
func (fake fakeDiff) VerifyWorkspace(context.Context, string, string) error { return fake.workspaceErr }
func (fake fakeDiff) VerifyCommit(context.Context, string, string, string) error {
	return fake.commitErr
}

type recordingDiff struct {
	seal         string
	commitErr    error
	verifiedTree string
	verifiedHead string
}

func (fake *recordingDiff) Seal(context.Context, string) (string, error)     { return fake.seal, nil }
func (*recordingDiff) VerifyWorkspace(context.Context, string, string) error { return nil }
func (fake *recordingDiff) VerifyCommit(_ context.Context, _ string, tree, head string) error {
	fake.verifiedTree, fake.verifiedHead = tree, head
	return fake.commitErr
}

type fakeProvenance struct {
	events *[]string
	err    error
}

type sequenceProvenance struct {
	events *[]string
	calls  int
	failAt int
}

func (fake *sequenceProvenance) RevalidateCandidate(context.Context, packsync.Plan) error {
	*fake.events = append(*fake.events, "provenance")
	fake.calls++
	if fake.calls == fake.failAt {
		return errors.New("moved")
	}
	return nil
}

func (fake fakeProvenance) RevalidateCandidate(context.Context, packsync.Plan) error {
	*fake.events = append(*fake.events, "provenance")
	return fake.err
}

func (fake fakeValidator) Validate(context.Context, string) error {
	*fake.events = append(*fake.events, "validate")
	return fake.err
}

type fakePublicationGateway struct {
	events       *[]string
	states       []PublicationState
	publishCalls int
}

func (fake *fakePublicationGateway) Prepare(proposal Proposal) (Proposal, error) {
	*fake.events = append(*fake.events, "prepare")
	proposal.ManagedMetadataHash = strings.Repeat("6", 64)
	return proposal, nil
}

func (fake *fakePublicationGateway) Observe(context.Context, string) (PublicationState, error) {
	*fake.events = append(*fake.events, "observe")
	if len(fake.states) == 0 {
		return PublicationState{}, errors.New("unexpected observation")
	}
	state := fake.states[0]
	fake.states = fake.states[1:]
	return state, nil
}

func (fake *fakePublicationGateway) Publish(_ context.Context, _ Proposal, _ PublicationDecision) (PRState, error) {
	*fake.events = append(*fake.events, "publish")
	fake.publishCalls++
	return PRState{Number: 7}, nil
}

func (fake *fakePublicationGateway) Finalize(_ context.Context, _ Proposal, _ PublicationDecision, _ PRState) (string, error) {
	*fake.events = append(*fake.events, "finalize")
	return strings.Repeat("7", 64), nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func validProposal() Proposal {
	return Proposal{SourceID: "mattpocock-skills", PlanID: "plan-1", BaseSHA: baseA, CandidateSHA: candidateA, ResultTreeSHA: headA, HeadSHA: headA, ProvenanceSHA256: strings.Repeat("5", 64), ManagedTitle: "sync(mattpocock-skills): candidate", ManagedMetadataHash: strings.Repeat("6", 64), Validation: ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, MattySuite: true}, InvalidationConditions: DecisionReadyInvalidationConditions()}
}

func pristineState() PublicationState {
	return PublicationState{BaseSHA: baseA, ProvenanceCurrent: true, CandidateRelation: CandidateSame, Branch: BranchState{Exists: true, Name: "sync/mattpocock-skills", HeadSHA: headA, Owner: AutomationOwner, ManagedMetadataHash: strings.Repeat("6", 64)}, PR: PRState{Exists: true, Number: 7, Open: true, BaseBranch: "main", HeadBranch: "sync/mattpocock-skills", HeadSHA: headA, MetadataHash: strings.Repeat("6", 64), Owner: AutomationOwner}, Record: PublicationRecord{PlanID: "plan-1", BaseSHA: baseA, CandidateSHA: candidateA, HeadSHA: headA, ResultTreeSHA: headA, ProvenanceSHA256: strings.Repeat("5", 64), MetadataHash: strings.Repeat("6", 64)}}
}

func publishedState(draft bool, metadataHash string) PublicationState {
	return PublicationState{BaseSHA: baseA, ProvenanceCurrent: true, CandidateRelation: CandidateSame, Branch: BranchState{Exists: true, Name: "sync/mattpocock-skills", HeadSHA: headA, Owner: AutomationOwner, ManagedMetadataHash: metadataHash}, PR: PRState{Exists: true, Number: 7, Open: true, BaseBranch: "main", HeadBranch: "sync/mattpocock-skills", HeadSHA: headA, MetadataHash: metadataHash, Owner: AutomationOwner, Draft: draft}, Record: PublicationRecord{PlanID: "plan-1", BaseSHA: baseA, CandidateSHA: candidateA, HeadSHA: headA, ResultTreeSHA: headA, ProvenanceSHA256: strings.Repeat("5", 64), MetadataHash: metadataHash}}
}
