package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

type phaseValidator interface {
	packsync.BundleValidator
	packsyncworkflow.Validator
}

var (
	workflowValidatorFactory = func() phaseValidator { return commandValidator{} }
	workflowGatewayFactory   = newGitHubGateway
)

func publish(ctx context.Context, option options, output io.Writer) error {
	if option.requestPath == "" || option.planPath == "" || option.evidencePath == "" || option.validationPath == "" || option.outputDir == "" {
		return errors.New("publish requires request, plan, evidence, validation proof, and output paths")
	}
	var dispatch packsyncworkflow.DispatchRequest
	var plan packsync.Plan
	var evidence packsync.ClassificationEvidenceSet
	if err := readJSON(option.requestPath, &dispatch); err != nil {
		return err
	}
	if err := dispatch.Validate(); err != nil {
		return err
	}
	if err := readJSON(option.planPath, &plan); err != nil {
		return err
	}
	if err := readJSON(option.evidencePath, &evidence); err != nil {
		return err
	}
	if plan.SourceID != dispatch.SourceID || plan.Candidate.Commit == "" || plan.Preconditions.BaseCommit == "" {
		return errors.New("dispatch and sealed plan identity contradict")
	}
	if err := validateWorkflowEvidence(plan, evidence); err != nil {
		return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureClassification, Err: err}
	}
	var validation packsyncworkflow.ValidationArtifact
	if err := readJSON(option.validationPath, &validation); err != nil {
		return err
	}
	if err := validation.Validate(); err != nil || validation.SourceID != dispatch.SourceID || validation.PlanID != plan.PlanID || validation.BaseSHA != plan.Preconditions.BaseCommit || validation.CandidateSHA != plan.Candidate.Commit {
		return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureValidation, Err: errors.New("sandbox validation proof is missing, invalid, or stale")}
	}
	acquisition, err := os.MkdirTemp("", "packy-pack-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(acquisition)
	validator := workflowValidatorFactory()
	engine := packsync.Engine{Source: workflowSourceFactory(), Validate: validator}
	apply := packsync.ApplyRequest{CheckRequest: packsync.CheckRequest{RepositoryRoot: option.repositoryRoot, SourceID: dispatch.SourceID, AcquisitionDir: acquisition, Registration: plan.Registration}, Plan: plan, ClassificationEvidence: evidence}
	if plan.Status == "no-op" {
		return writeNoopArtifact(option.outputDir, dispatch.SourceID, plan)
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	if err := stageAll(ctx, option.repositoryRoot); err != nil {
		return err
	}
	baseStatus, err := command(ctx, option.repositoryRoot, "git", "status", "--porcelain")
	if err != nil || strings.TrimSpace(baseStatus) != "" {
		return errors.New("publish sandbox must begin from the exact clean base")
	}
	github := workflowGatewayFactory(option.repositoryRoot, plan)
	builder := &publicationBuilder{dispatch: dispatch, plan: plan, evidence: evidence, evidencePath: option.evidencePath, github: github}
	publisher := packsyncworkflow.Publisher{Applier: engine, Validator: validator, Builder: builder, Diff: gitDiffVerifier{}, Provenance: engine, GitHub: github}
	result, err := publisher.Run(ctx, packsyncworkflow.PublishRequest{RepositoryRoot: option.repositoryRoot, Apply: apply})
	if err != nil {
		return err
	}
	builder.brief.PullRequest = result.PullRequest.Number
	builder.brief.HeadSHA = result.PullRequest.HeadSHA
	builder.brief.ResultTreeSHA = result.Proposal.ResultTreeSHA
	builder.brief.Validation = result.Readiness.Gates
	builder.brief.DecisionReady = result.Readiness.DecisionReady
	builder.brief.Blockers = nil
	builder.brief.InvalidationConditions = result.Proposal.InvalidationConditions
	if err := writeCanonical(filepath.Join(option.outputDir, "proposal-brief.json"), builder.brief); err != nil {
		return err
	}
	markdown, err := builder.brief.Markdown()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(option.outputDir, "proposal-brief.md"), []byte(markdown), 0o600); err != nil {
		return err
	}
	artifact := packsyncworkflow.PublicationArtifact{SchemaVersion: 2, SourceID: dispatch.SourceID, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, CandidateSHA: plan.Candidate.Commit, ArtifactProvenance: artifactProvenance(plan), ResultTreeSHA: result.Proposal.ResultTreeSHA, HeadSHA: result.Proposal.HeadSHA, ProvenanceSHA256: builder.provenance, BranchName: result.Decision.Branch, PRNumber: result.PullRequest.Number, PRStateSHA256: result.PullRequest.MetadataHash, ManagedTitle: result.Proposal.ManagedTitle, ManagedMetadataHash: result.PullRequest.MetadataHash, Validation: result.Readiness.Gates, DecisionReady: result.Readiness.DecisionReady, AutoMerge: false, ManualMergeRequired: true, UpstreamContentExecuted: false, InvalidationConditions: result.Proposal.InvalidationConditions}
	if err := artifact.Validate(); err != nil {
		return err
	}
	if err := writeCanonical(filepath.Join(option.outputDir, "publication.json"), artifact); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, filepath.Join(option.outputDir, "publication.json"))
	return err
}

func validateSandbox(ctx context.Context, option options, output io.Writer) error {
	if option.requestPath == "" || option.planPath == "" || option.evidencePath == "" || option.outputDir == "" {
		return errors.New("validate requires request, plan, evidence, and output paths")
	}
	var dispatch packsyncworkflow.DispatchRequest
	var plan packsync.Plan
	var evidence packsync.ClassificationEvidenceSet
	if err := readJSON(option.requestPath, &dispatch); err != nil {
		return err
	}
	if err := readJSON(option.planPath, &plan); err != nil {
		return err
	}
	if err := readJSON(option.evidencePath, &evidence); err != nil {
		return err
	}
	if err := dispatch.Validate(); err != nil || plan.SourceID != dispatch.SourceID || plan.Preconditions.BaseCommit == "" || plan.Candidate.Commit == "" {
		return errors.New("validation inputs contradict the sealed dispatch")
	}
	if err := validateWorkflowEvidence(plan, evidence); err != nil {
		return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureClassification, Err: err}
	}
	acquisition, err := os.MkdirTemp("", "packy-pack-validate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(acquisition)
	validator := workflowValidatorFactory()
	engine := packsync.Engine{Source: workflowSourceFactory(), Validate: validator}
	apply := packsync.ApplyRequest{CheckRequest: packsync.CheckRequest{RepositoryRoot: option.repositoryRoot, SourceID: dispatch.SourceID, AcquisitionDir: acquisition, Registration: plan.Registration}, Plan: plan, ClassificationEvidence: evidence}
	if _, err := engine.Apply(ctx, apply); err != nil {
		return err
	}
	var validationErr error
	if applied, ok := validator.(packsyncworkflow.AppliedValidator); ok {
		validationErr = applied.ValidateApplied(ctx, option.repositoryRoot)
	} else {
		validationErr = validator.Validate(ctx, option.repositoryRoot)
	}
	if validationErr != nil {
		return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureValidation, Err: validationErr}
	}
	if err := stageAll(ctx, option.repositoryRoot); err != nil {
		return err
	}
	resultTree, err := command(ctx, option.repositoryRoot, "git", "write-tree")
	if err != nil {
		return err
	}
	artifact := packsyncworkflow.ValidationArtifact{SchemaVersion: 2, SourceID: dispatch.SourceID, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, CandidateSHA: plan.Candidate.Commit, ArtifactProvenance: artifactProvenance(plan), ResultTreeSHA: strings.TrimSpace(resultTree), PackySuite: true, Apply: true, UpstreamBytes: false}
	if err := artifact.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	if err := writeCanonical(filepath.Join(option.outputDir, "validation.json"), artifact); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, filepath.Join(option.outputDir, "validation.json"))
	return err
}

func validateWorkflowEvidence(plan packsync.Plan, evidence packsync.ClassificationEvidenceSet) error {
	if len(plan.AffectedPacks) > 0 {
		return packsync.ValidateClassificationEvidence(plan, evidence)
	}
	if len(evidence.Evidence) != 0 || evidence.PlanID != "" || evidence.BaseSHA != "" || evidence.SchemaVersion != 0 {
		return errors.New("classification evidence contradicts a plan without affected packs")
	}
	return nil
}

type publicationBuilder struct {
	dispatch     packsyncworkflow.DispatchRequest
	plan         packsync.Plan
	evidence     packsync.ClassificationEvidenceSet
	evidencePath string
	github       *githubGateway
	proposal     packsyncworkflow.Proposal
	brief        packsyncworkflow.ReviewBrief
	provenance   string
}

type gitDiffVerifier struct{}

func (gitDiffVerifier) Seal(ctx context.Context, root string) (string, error) {
	if err := stageAll(ctx, root); err != nil {
		return "", err
	}
	tree, err := command(ctx, root, "git", "write-tree")
	return strings.TrimSpace(tree), err
}

func (gitDiffVerifier) VerifyWorkspace(ctx context.Context, root, seal string) error {
	observed, err := (gitDiffVerifier{}).Seal(ctx, root)
	if err != nil {
		return err
	}
	if observed != seal {
		return errors.New("workspace tree changed after Apply was sealed")
	}
	return nil
}

func (gitDiffVerifier) VerifyCommit(ctx context.Context, root, seal, head string) error {
	observed, err := command(ctx, root, "git", "rev-parse", head+"^{tree}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(observed) != seal {
		return errors.New("commit tree differs from sealed Apply tree")
	}
	return nil
}

func (builder *publicationBuilder) Build(ctx context.Context, repositoryRoot string, applyResult packsync.ApplyResult) (packsyncworkflow.Proposal, error) {
	if err := stageAll(ctx, repositoryRoot); err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	provenance, err := fileHash(filepath.Join(repositoryRoot, "bundle", "sources", builder.dispatch.SourceID+".lock.json"))
	if err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	head, err := prepareCommit(ctx, repositoryRoot, builder.dispatch.SourceID, builder.plan)
	if err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	traces := []packsyncworkflow.ClassifierTrace{}
	_ = readJSON(filepath.Join(filepath.Dir(builder.evidencePath), "classifier-trace.json"), &traces)
	brief := packsyncworkflow.ReviewBrief{SchemaVersion: 1, Actor: os.Getenv("GITHUB_ACTOR"), RunID: os.Getenv("GITHUB_RUN_ID"), RunAttempt: os.Getenv("GITHUB_RUN_ATTEMPT"), RunURL: actionsRunURL(), Request: builder.dispatch, Candidate: builder.plan.Candidate, PlanID: builder.plan.PlanID, BaseSHA: builder.plan.Preconditions.BaseCommit, HeadSHA: head, Branch: "sync/" + builder.dispatch.SourceID, Changes: builder.plan.Changes, Discoveries: builder.plan.Discoveries, SelectedResources: builder.plan.ProposedLock.Resources, PreviousSnapshotSHA256: builder.plan.PreviousSnapshotSHA256, ProposedSnapshotSHA256: builder.plan.ProposedLock.Snapshot, Classification: builder.evidence.Evidence, ClassifierTrace: traces, ApplyStatus: applyResult.Status, UpstreamContentExecuted: false, DecisionReady: false, AutoMerge: false, ManualMergeRequired: true, Recovery: []string{"Review the canonical evidence and diff, then merge manually only while readiness remains valid."}}
	title := fmt.Sprintf("sync(%s): %s", builder.dispatch.SourceID, builder.plan.Candidate.Commit[:12])
	proposal := packsyncworkflow.Proposal{SourceID: builder.dispatch.SourceID, PlanID: builder.plan.PlanID, BaseSHA: builder.plan.Preconditions.BaseCommit, CandidateSHA: builder.plan.Candidate.Commit, HeadSHA: head, ProvenanceSHA256: provenance, ManagedTitle: title}
	builder.proposal, builder.brief, builder.provenance = proposal, brief, provenance
	builder.github.title, builder.github.brief = title, brief
	return proposal, nil
}

type commandValidator struct {
	run func(*exec.Cmd) ([]byte, error)
}

const stagedValidationEnvironment = "PACKY_VALIDATION_STAGED=1"

func (validator commandValidator) ValidateBundle(ctx context.Context, repositoryRoot, bundleRoot string) error {
	sandbox, err := os.MkdirTemp("", "packy-staged-validation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(sandbox)
	checkout := filepath.Join(sandbox, "repo")
	if err := copyForValidation(repositoryRoot, checkout, bundleRoot); err != nil {
		return err
	}
	return validator.validate(ctx, checkout, true)
}

func (validator commandValidator) Validate(ctx context.Context, repositoryRoot string) error {
	return validator.validate(ctx, repositoryRoot, false)
}

func (validator commandValidator) ValidateApplied(ctx context.Context, repositoryRoot string) error {
	return validator.validate(ctx, repositoryRoot, true)
}

func (validator commandValidator) validate(ctx context.Context, repositoryRoot string, staged bool) error {
	home, err := os.MkdirTemp("", "packy-validation-home-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	cmd := exec.CommandContext(ctx, "bash", "./scripts/validate-packy.sh")
	cmd.Dir = repositoryRoot
	environment := withoutStagedValidationMarker(withoutCredentials(os.Environ()))
	environment = append(environment, "HOME="+filepath.Join(home, "home"), "XDG_CONFIG_HOME="+filepath.Join(home, "xdg"))
	if staged {
		environment = append(environment, stagedValidationEnvironment)
	}
	cmd.Env = environment
	run := validator.run
	if run == nil {
		run = func(cmd *exec.Cmd) ([]byte, error) { return cmd.CombinedOutput() }
	}
	output, err := run(cmd)
	if err != nil {
		return fmt.Errorf("Packy-owned validation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func withoutStagedValidationMarker(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, variable := range environment {
		if !strings.HasPrefix(variable, "PACKY_VALIDATION_STAGED=") {
			filtered = append(filtered, variable)
		}
	}
	return filtered
}

func copyForValidation(repositoryRoot, checkout, bundleRoot string) error {
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(repositoryRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" || entry.Name() == ".codegraph" || entry.Name() == ".scratch" || entry.Name() == "bundle" || isPackyTransactionArtifact(entry.Name()) {
			continue
		}
		if err := copyPath(filepath.Join(repositoryRoot, entry.Name()), filepath.Join(checkout, entry.Name())); err != nil {
			return err
		}
	}
	return copyPath(bundleRoot, filepath.Join(checkout, "bundle"))
}

func isPackyTransactionArtifact(name string) bool {
	return name == ".packy-bundle-recovery.json" ||
		strings.HasPrefix(name, ".packy-bundle-validation-") ||
		strings.HasPrefix(name, ".packy-bundle-") && (strings.HasSuffix(name, ".staged") || strings.HasSuffix(name, ".backup"))
}

func copyPath(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return errors.New("validation copy encountered a non-regular path")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

type githubGateway struct {
	repositoryRoot string
	repository     string
	plan           packsync.Plan
	title          string
	bodyPrefix     string
	brief          packsyncworkflow.ReviewBrief
	last           packsyncworkflow.PublicationState
	retry          packsyncworkflow.RetryPolicy
	run            func(context.Context, string, string, ...string) (string, error)
}

func newGitHubGateway(repositoryRoot string, plan packsync.Plan) *githubGateway {
	return &githubGateway{repositoryRoot: repositoryRoot, repository: os.Getenv("GITHUB_REPOSITORY"), plan: plan, retry: packsyncworkflow.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Second}, run: command}
}

func (gateway *githubGateway) Prepare(proposal packsyncworkflow.Proposal) (packsyncworkflow.Proposal, error) {
	gateway.brief.ResultTreeSHA = proposal.ResultTreeSHA
	gateway.brief.Validation = proposal.Validation
	gateway.brief.DecisionReady = false
	gateway.brief.Blockers = []string{"Publication remains blocked until the exact post-write pull request identity is reobserved."}
	gateway.brief.InvalidationConditions = proposal.InvalidationConditions
	prefix, err := gateway.brief.Markdown()
	if err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	proposal.ManagedMetadataHash = packsyncworkflow.ManagedMetadataHash(proposal.ManagedTitle, prefix)
	gateway.title, gateway.bodyPrefix = proposal.ManagedTitle, prefix
	gateway.brief.Validation = proposal.Validation
	gateway.brief.InvalidationConditions = proposal.InvalidationConditions
	return proposal, nil
}

func (gateway *githubGateway) Observe(ctx context.Context, sourceID string) (packsyncworkflow.PublicationState, error) {
	if gateway.repository == "" {
		return packsyncworkflow.PublicationState{}, errors.New("GITHUB_REPOSITORY is required for publication")
	}
	branch := "sync/" + sourceID
	base, err := gateway.retryCommand(ctx, "gh", "api", "repos/"+gateway.repository+"/git/ref/heads/main", "--jq", ".object.sha")
	if err != nil {
		return packsyncworkflow.PublicationState{}, err
	}
	state := packsyncworkflow.PublicationState{BaseSHA: strings.TrimSpace(base)}
	remote, err := gateway.retryCommand(ctx, "git", "ls-remote", "origin", "refs/heads/"+branch)
	if err != nil {
		return state, err
	}
	fields := strings.Fields(remote)
	if len(fields) > 0 {
		state.Branch.Exists, state.Branch.Name, state.Branch.HeadSHA = true, branch, fields[0]
	}
	prs, err := gateway.pullRequests(ctx, branch)
	if err != nil {
		return state, err
	}
	if len(prs) > 1 {
		state.Branch.Owner = ""
		state.PR.Exists = true
		return state, nil
	}
	if len(prs) == 1 {
		pr := prs[0]
		state.PR = packsyncworkflow.PRState{Exists: true, Number: pr.Number, Open: pr.State == "OPEN", BaseBranch: pr.BaseRefName, HeadBranch: pr.HeadRefName, HeadSHA: pr.HeadRefOID, MetadataHash: packsyncworkflow.ManagedMetadataHash(pr.Title, pr.Body), Owner: pr.Author.Login, Draft: pr.IsDraft, AutoMerge: pr.AutoMergeRequest != nil}
		lastEditor, editorErr := gateway.lastPREditor(ctx, pr.Number)
		if editorErr != nil {
			return state, editorErr
		}
		state.PR.LastEditor = lastEditor
		record, ok := packsyncworkflow.ParsePublicationRecord(pr.Body)
		if ok {
			state.Record = record
			state.Branch.ManagedMetadataHash = record.MetadataHash
		}
	}
	if state.PR.Exists {
		state.PR.Owner = normalizedAutomationLogin(state.PR.Owner)
	}
	commitOwned := false
	if state.Branch.Exists && state.Record.HeadSHA == state.Branch.HeadSHA {
		identityJSON, identityErr := gateway.retryCommand(ctx, "gh", "api", "repos/"+gateway.repository+"/commits/"+state.Branch.HeadSHA, "--jq", "{author:.author.login,committer:.committer.login,message:.commit.message,parents:[.parents[].sha]}")
		if identityErr != nil {
			return state, identityErr
		}
		var identity struct {
			Author    string   `json:"author"`
			Committer string   `json:"committer"`
			Message   string   `json:"message"`
			Parents   []string `json:"parents"`
		}
		if json.Unmarshal([]byte(identityJSON), &identity) != nil {
			return state, packsyncworkflow.Failure{Kind: packsyncworkflow.FailureIntegrity, Err: errors.New("commit ownership identity is malformed")}
		}
		expectedMessage := ""
		if len(state.Record.CandidateSHA) == 40 {
			expectedMessage = fmt.Sprintf("sync(%s): %s [%s]", sourceID, state.Record.CandidateSHA[:12], state.Record.PlanID)
		}
		commitOwned = identity.Author == packsyncworkflow.AutomationOwner && identity.Committer == packsyncworkflow.AutomationOwner && identity.Message == expectedMessage && len(identity.Parents) == 1 && identity.Parents[0] == state.Record.BaseSHA
	}
	if state.Branch.Exists && commitOwned && state.PR.Owner == packsyncworkflow.AutomationOwner && (state.PR.LastEditor == "" || state.PR.LastEditor == packsyncworkflow.AutomationOwner) && state.Record.HeadSHA != "" && state.Record.HeadSHA == state.Branch.HeadSHA && state.PR.HeadSHA == state.Branch.HeadSHA && state.Record.MetadataHash == state.PR.MetadataHash {
		state.Branch.Owner = packsyncworkflow.AutomationOwner
	}
	if state.Record.HeadSHA != "" && state.Branch.HeadSHA != state.Record.HeadSHA {
		state.Branch.Diverged = true
		state.Branch.HumanCommits = true
	}
	switch {
	case state.Record.CandidateSHA == "" || state.Record.CandidateSHA == gateway.plan.Candidate.Commit:
		state.CandidateRelation = packsyncworkflow.CandidateSame
	default:
		status, compareErr := gateway.retryCommand(ctx, "gh", "api", "repos/"+gateway.plan.Candidate.Repository+"/compare/"+state.Record.CandidateSHA+"..."+gateway.plan.Candidate.Commit, "--jq", ".status")
		if compareErr != nil {
			return state, compareErr
		}
		if strings.TrimSpace(status) == "ahead" {
			state.CandidateRelation = packsyncworkflow.CandidateAdvancing
		} else {
			state.CandidateRelation = packsyncworkflow.CandidateRegressive
		}
	}
	gateway.last = state
	return state, nil
}

func (gateway *githubGateway) Publish(ctx context.Context, proposal packsyncworkflow.Proposal, decision packsyncworkflow.PublicationDecision) (packsyncworkflow.PRState, error) {
	expectedState := gateway.last
	branch := decision.Branch
	head := proposal.HeadSHA
	expected := gateway.last.Branch.HeadSHA
	lease := "--force-with-lease=refs/heads/" + branch + ":" + expected
	if err := gateway.pushOnce(ctx, lease, branch, head); err != nil {
		return packsyncworkflow.PRState{}, err
	}
	record := packsyncworkflow.NewPublicationRecord(proposal, head, proposal.ManagedMetadataHash)
	body, err := packsyncworkflow.ManagedBody(gateway.bodyPrefix, record)
	if err != nil {
		return packsyncworkflow.PRState{}, err
	}
	var number int
	if decision.Action == packsyncworkflow.PublicationCreate {
		number, err = gateway.createPRWithRetry(ctx, proposal, body, record)
		if err != nil {
			kind := packsyncworkflow.FailureIntegrity
			var failure packsyncworkflow.Failure
			if errors.As(err, &failure) {
				if failure.Blocker != "" && failure.Recovery != "" {
					return packsyncworkflow.PRState{}, err
				}
				kind = failure.Kind
			}
			return packsyncworkflow.PRState{}, packsyncworkflow.Failure{Kind: kind, Blocker: "The automation-owned stable branch was pushed but first pull-request creation did not complete.", Recovery: fmt.Sprintf("Verify refs/heads/%s still equals %s and has no pull request, delete only that exact orphan branch, then start a fresh Check.", branch, head), Err: err}
		}
	} else {
		number = decision.PRNumber
		beforePR := expectedState.PR
		beforePR.HeadSHA = proposal.HeadSHA
		if err := gateway.editPRWithReobserve(ctx, proposal, beforePR, expectedState.Record, record, body); err != nil {
			return packsyncworkflow.PRState{}, err
		}
	}
	return packsyncworkflow.PRState{Number: number}, nil
}

func (gateway *githubGateway) Finalize(ctx context.Context, proposal packsyncworkflow.Proposal, decision packsyncworkflow.PublicationDecision, observed packsyncworkflow.PRState) (string, error) {
	if decision.Action == packsyncworkflow.PublicationCreate {
		beforeReady := gateway.last.PR
		afterReady := beforeReady
		afterReady.Draft = false
		readyRecord := gateway.last.Record
		if err := gateway.retry.Do(ctx, func() error {
			current, observeErr := gateway.observeMutationOnce(ctx, proposal)
			if observeErr != nil {
				return packsyncworkflow.ClassifyNetworkFailure(observeErr)
			}
			if current.matchesPR(proposal, afterReady, readyRecord) {
				return nil
			}
			if !current.matchesPR(proposal, beforeReady, readyRecord) {
				return publicationCASFailure("draft pull request state changed before the ready transition")
			}
			_, readyErr := gateway.run(ctx, gateway.repositoryRoot, "gh", "pr", "ready", fmt.Sprint(observed.Number), "--repo", gateway.repository)
			if readyErr == nil {
				return nil
			}
			after, observeErr := gateway.observeMutationOnce(ctx, proposal)
			if observeErr == nil && after.matchesPR(proposal, afterReady, readyRecord) {
				return nil
			}
			if observeErr == nil && !after.matchesPR(proposal, beforeReady, readyRecord) {
				return publicationCASFailure("draft pull request state changed during an ambiguous ready transition")
			}
			return packsyncworkflow.ClassifyNetworkFailure(readyErr)
		}); err != nil {
			return "", err
		}
	}
	gateway.brief.PullRequest = observed.Number
	gateway.brief.HeadSHA = observed.HeadSHA
	gateway.brief.Validation = proposal.Validation
	gateway.brief.Blockers = nil
	gateway.brief.DecisionReady = true
	gateway.brief.InvalidationConditions = proposal.InvalidationConditions
	finalPrefix, err := gateway.brief.Markdown()
	if err != nil {
		return "", err
	}
	finalHash := packsyncworkflow.ManagedMetadataHash(gateway.title, finalPrefix)
	record := packsyncworkflow.NewPublicationRecord(proposal, proposal.HeadSHA, finalHash)
	body, err := packsyncworkflow.ManagedBody(finalPrefix, record)
	if err != nil {
		return "", err
	}
	beforePR := gateway.last.PR
	beforePR.Number = observed.Number
	beforePR.HeadSHA = proposal.HeadSHA
	beforePR.MetadataHash = observed.MetadataHash
	if decision.Action == packsyncworkflow.PublicationCreate {
		beforePR.Draft = false
	}
	if err := gateway.editPRWithReobserve(ctx, proposal, beforePR, gateway.last.Record, record, body); err != nil {
		return "", err
	}
	return finalHash, nil
}

func (gateway *githubGateway) editPRWithReobserve(ctx context.Context, proposal packsyncworkflow.Proposal, beforePR packsyncworkflow.PRState, beforeRecord, targetRecord packsyncworkflow.PublicationRecord, body string) error {
	return gateway.retry.Do(ctx, func() error {
		state, err := gateway.observeMutationBeforeEdit(ctx, proposal, beforePR, beforeRecord)
		if err != nil {
			return packsyncworkflow.ClassifyNetworkFailure(err)
		}
		if state.matchesPR(proposal, beforePR, targetRecord) {
			return nil
		}
		if !state.matchesPR(proposal, beforePR, beforeRecord) {
			return publicationCASFailure("branch or pull request state changed before an automation edit")
		}
		_, editErr := gateway.run(ctx, gateway.repositoryRoot, "gh", "pr", "edit", fmt.Sprint(beforePR.Number), "--repo", gateway.repository, "--title", gateway.title, "--body", body)
		if editErr == nil {
			return nil
		}
		after, observeErr := gateway.observeMutationOnce(ctx, proposal)
		if observeErr == nil && after.matchesPR(proposal, beforePR, targetRecord) {
			return nil
		}
		if observeErr == nil && !after.matchesPR(proposal, beforePR, beforeRecord) {
			return publicationCASFailure("branch or pull request state changed during an ambiguous edit")
		}
		return packsyncworkflow.ClassifyNetworkFailure(editErr)
	})
}

func (gateway *githubGateway) observeMutationBeforeEdit(ctx context.Context, proposal packsyncworkflow.Proposal, expected packsyncworkflow.PRState, record packsyncworkflow.PublicationRecord) (mutationObservation, error) {
	var state mutationObservation
	for observation := 0; observation < gateway.retry.MaxAttempts; observation++ {
		var err error
		state, err = gateway.observeMutationOnce(ctx, proposal)
		if err != nil || !state.matchesPRHeadProjectionLag(proposal, expected, record) {
			return state, err
		}
	}
	return state, nil
}

type ghPR struct {
	Number           int    `json:"number"`
	State            string `json:"state"`
	BaseRefName      string `json:"baseRefName"`
	HeadRefName      string `json:"headRefName"`
	HeadRefOID       string `json:"headRefOid"`
	IsDraft          bool   `json:"isDraft"`
	AutoMergeRequest any    `json:"autoMergeRequest"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
}

type commitIdentity struct {
	Author    string   `json:"author"`
	Committer string   `json:"committer"`
	Message   string   `json:"message"`
	Parents   []string `json:"parents"`
}

type mutationObservation struct {
	BaseSHA    string
	BranchHead string
	PRs        []ghPR
	Commit     commitIdentity
	LastEditor string
}

func (gateway *githubGateway) observeMutationOnce(ctx context.Context, proposal packsyncworkflow.Proposal) (mutationObservation, error) {
	branch := "sync/" + proposal.SourceID
	base, err := gateway.run(ctx, gateway.repositoryRoot, "gh", "api", "repos/"+gateway.repository+"/git/ref/heads/main", "--jq", ".object.sha")
	if err != nil {
		return mutationObservation{}, err
	}
	remote, err := gateway.run(ctx, gateway.repositoryRoot, "git", "ls-remote", "origin", "refs/heads/"+branch)
	if err != nil {
		return mutationObservation{}, err
	}
	fields := strings.Fields(remote)
	if len(fields) != 2 || fields[1] != "refs/heads/"+branch {
		return mutationObservation{}, errors.New("stable publication branch is absent or ambiguous")
	}
	prs, err := gateway.pullRequestsOnce(ctx, branch)
	if err != nil {
		return mutationObservation{}, err
	}
	lastEditor := ""
	if len(prs) == 1 {
		lastEditor, err = gateway.lastPREditorOnce(ctx, prs[0].Number)
		if err != nil {
			return mutationObservation{}, err
		}
	}
	identityJSON, err := gateway.run(ctx, gateway.repositoryRoot, "gh", "api", "repos/"+gateway.repository+"/commits/"+fields[0], "--jq", "{author:.author.login,committer:.committer.login,message:.commit.message,parents:[.parents[].sha]}")
	if err != nil {
		return mutationObservation{}, err
	}
	var identity commitIdentity
	if err := json.Unmarshal([]byte(identityJSON), &identity); err != nil {
		return mutationObservation{}, errors.New("commit ownership identity is malformed")
	}
	return mutationObservation{BaseSHA: strings.TrimSpace(base), BranchHead: fields[0], PRs: prs, Commit: identity, LastEditor: lastEditor}, nil
}

func (state mutationObservation) matchesCommon(proposal packsyncworkflow.Proposal) bool {
	expectedMessage := fmt.Sprintf("sync(%s): %s [%s]", proposal.SourceID, proposal.CandidateSHA[:12], proposal.PlanID)
	return state.BaseSHA == proposal.BaseSHA && state.BranchHead == proposal.HeadSHA && state.Commit.Author == packsyncworkflow.AutomationOwner && state.Commit.Committer == packsyncworkflow.AutomationOwner && state.Commit.Message == expectedMessage && len(state.Commit.Parents) == 1 && state.Commit.Parents[0] == proposal.BaseSHA
}

func (state mutationObservation) matchesPR(proposal packsyncworkflow.Proposal, expected packsyncworkflow.PRState, record packsyncworkflow.PublicationRecord) bool {
	if !state.matchesCommon(proposal) || len(state.PRs) != 1 {
		return false
	}
	pr := state.PRs[0]
	owner := normalizedAutomationLogin(pr.Author.Login)
	parsed, ok := packsyncworkflow.ParsePublicationRecord(pr.Body)
	return ok && parsed == record && pr.Number == expected.Number && pr.State == "OPEN" && expected.Open && pr.BaseRefName == expected.BaseBranch && pr.HeadRefName == expected.HeadBranch && pr.HeadRefOID == proposal.HeadSHA && pr.IsDraft == expected.Draft && (pr.AutoMergeRequest != nil) == expected.AutoMerge && owner == packsyncworkflow.AutomationOwner && (state.LastEditor == "" || state.LastEditor == packsyncworkflow.AutomationOwner) && packsyncworkflow.ManagedMetadataHash(pr.Title, pr.Body) == record.MetadataHash
}

func (state mutationObservation) matchesPRHeadProjectionLag(proposal packsyncworkflow.Proposal, expected packsyncworkflow.PRState, record packsyncworkflow.PublicationRecord) bool {
	if len(state.PRs) != 1 || record.HeadSHA == "" || record.HeadSHA == proposal.HeadSHA || state.PRs[0].HeadRefOID != record.HeadSHA {
		return false
	}
	adjusted := state
	adjusted.PRs = append([]ghPR(nil), state.PRs...)
	adjusted.PRs[0].HeadRefOID = proposal.HeadSHA
	return adjusted.matchesPR(proposal, expected, record)
}

func (state mutationObservation) matchesCreated(proposal packsyncworkflow.Proposal, record packsyncworkflow.PublicationRecord) (int, bool) {
	if len(state.PRs) != 1 {
		return 0, false
	}
	pr := state.PRs[0]
	expected := packsyncworkflow.PRState{Number: pr.Number, Open: true, BaseBranch: "main", HeadBranch: "sync/" + proposal.SourceID, HeadSHA: proposal.HeadSHA, Owner: packsyncworkflow.AutomationOwner, Draft: true}
	return pr.Number, pr.Number > 0 && state.matchesPR(proposal, expected, record)
}

func publicationCASFailure(message string) error {
	return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureOwnership, Blocker: message, Recovery: "Preserve the observed branch and pull request for manual inspection; start a fresh Check before any automation write.", Err: errors.New("publication compare-and-swap failed")}
}

func (gateway *githubGateway) pullRequests(ctx context.Context, branch string) ([]ghPR, error) {
	var prs []ghPR
	err := gateway.retry.Do(ctx, func() error {
		var err error
		prs, err = gateway.pullRequestsOnce(ctx, branch)
		if err == nil {
			return nil
		}
		return packsyncworkflow.ClassifyNetworkFailure(err)
	})
	return prs, err
}

func (gateway *githubGateway) lastPREditor(ctx context.Context, number int) (editor string, err error) {
	err = gateway.retry.Do(ctx, func() error {
		editor, err = gateway.lastPREditorOnce(ctx, number)
		if err == nil {
			return nil
		}
		return packsyncworkflow.ClassifyNetworkFailure(err)
	})
	return editor, err
}

func (gateway *githubGateway) lastPREditorOnce(ctx context.Context, number int) (string, error) {
	owner, name, ok := strings.Cut(gateway.repository, "/")
	if !ok || owner == "" || name == "" {
		return "", errors.New("publication repository identity is invalid")
	}
	query := `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){userContentEdits(last:1){nodes{editor{login}}}}}}`
	output, err := gateway.run(ctx, gateway.repositoryRoot, "gh", "api", "graphql", "-f", "query="+query, "-F", "owner="+owner, "-F", "name="+name, "-F", fmt.Sprintf("number=%d", number), "--jq", `.data.repository.pullRequest.userContentEdits.nodes | if length == 0 then {present:false,editor:""} else {present:true,editor:(.[0].editor.login // "")} end`)
	if err != nil {
		return "", err
	}
	var edit struct {
		Present bool   `json:"present"`
		Editor  string `json:"editor"`
	}
	if err := json.Unmarshal([]byte(output), &edit); err != nil {
		return "", errors.New("pull-request edit identity is malformed")
	}
	if edit.Present && strings.TrimSpace(edit.Editor) == "" {
		return "unavailable-edit-actor", nil
	}
	return normalizedAutomationLogin(strings.TrimSpace(edit.Editor)), nil
}

func normalizedAutomationLogin(login string) string {
	switch login {
	case "app/github-actions", "github-actions", packsyncworkflow.AutomationOwner:
		return packsyncworkflow.AutomationOwner
	default:
		return login
	}
}

func (gateway *githubGateway) pullRequestsOnce(ctx context.Context, branch string) ([]ghPR, error) {
	data, err := gateway.run(ctx, gateway.repositoryRoot, "gh", "pr", "list", "--repo", gateway.repository, "--state", "all", "--head", branch, "--json", "number,state,baseRefName,headRefName,headRefOid,isDraft,autoMergeRequest,title,body,author")
	if err != nil {
		return nil, err
	}
	var prs []ghPR
	if err := json.Unmarshal([]byte(data), &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

func (gateway *githubGateway) retryCommand(ctx context.Context, name string, args ...string) (output string, err error) {
	err = gateway.retry.Do(ctx, func() error {
		output, err = gateway.run(ctx, gateway.repositoryRoot, name, args...)
		if err == nil {
			return nil
		}
		return packsyncworkflow.ClassifyNetworkFailure(err)
	})
	return output, err
}

func (gateway *githubGateway) pushOnce(ctx context.Context, lease, branch, localHead string) error {
	_, err := gateway.run(ctx, gateway.repositoryRoot, "git", "push", lease, "origin", "HEAD:refs/heads/"+branch)
	if err == nil {
		return nil
	}
	remote, observeErr := gateway.run(ctx, gateway.repositoryRoot, "git", "ls-remote", "origin", "refs/heads/"+branch)
	if observeErr == nil {
		fields := strings.Fields(remote)
		if len(fields) > 0 && fields[0] == localHead {
			return nil
		}
		if (len(fields) > 0 && fields[0] != gateway.last.Branch.HeadSHA) || (len(fields) == 0 && gateway.last.Branch.HeadSHA != "") {
			return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureDivergence, Err: errors.New("source branch changed during ambiguous push")}
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "rejected") || strings.Contains(strings.ToLower(err.Error()), "stale info") {
		return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureDivergence, Err: errors.New("force-with-lease rejected changed source branch")}
	}
	return packsyncworkflow.ClassifyNetworkFailure(err)
}

func (gateway *githubGateway) createPRWithRetry(ctx context.Context, proposal packsyncworkflow.Proposal, body string, record packsyncworkflow.PublicationRecord) (number int, err error) {
	branch := "sync/" + proposal.SourceID
	err = gateway.retry.Do(ctx, func() error {
		before, observeErr := gateway.observeMutationOnce(ctx, proposal)
		if observeErr != nil {
			return packsyncworkflow.ClassifyNetworkFailure(observeErr)
		}
		if createdNumber, ok := before.matchesCreated(proposal, record); ok {
			number = createdNumber
			return nil
		}
		if !before.matchesCommon(proposal) || len(before.PRs) != 0 {
			if len(before.PRs) != 0 {
				return competingPRFailure(before.PRs)
			}
			return publicationCASFailure("stable branch identity changed before pull-request creation")
		}
		_, createErr := gateway.run(ctx, gateway.repositoryRoot, "gh", "pr", "create", "--repo", gateway.repository, "--base", "main", "--head", branch, "--title", gateway.title, "--body", body, "--draft")
		after, observeErr := gateway.observeMutationOnce(ctx, proposal)
		if observeErr != nil {
			return packsyncworkflow.ClassifyNetworkFailure(observeErr)
		}
		if createdNumber, ok := after.matchesCreated(proposal, record); ok {
			number = createdNumber
			return nil
		}
		if !after.matchesCommon(proposal) || len(after.PRs) != 0 {
			return publicationCASFailure("branch or pull request state changed during ambiguous creation")
		}
		if createErr == nil {
			return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureIntegrity, Err: errors.New("pull-request creation returned success without exact created state")}
		}
		return packsyncworkflow.ClassifyNetworkFailure(createErr)
	})
	return number, err
}

func competingPRFailure(prs []ghPR) error {
	if len(prs) == 1 && prs[0].State != "OPEN" {
		return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureOwnership, Blocker: "the stable source branch already has a closed pull request", Recovery: "A maintainer must decide whether to reopen that exact pull request; automation must not create a competitor.", Err: errors.New("closed pull request blocks first publication")}
	}
	return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureOwnership, Blocker: "the stable source branch has an unexpected or ambiguous pull request", Recovery: "Preserve every observed pull request and resolve ownership manually; automation must not create a competitor.", Err: errors.New("ambiguous PR creation state")}
}

func fileHash(name string) (string, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return packsyncworkflow.HashText(string(data)), nil
}

func stageAll(ctx context.Context, root string) error {
	_, err := command(ctx, root, "git", "add", "-A")
	return err
}

func prepareCommit(ctx context.Context, root, sourceID string, plan packsync.Plan) (string, error) {
	if _, err := command(ctx, root, "git", "config", "user.name", packsyncworkflow.AutomationOwner); err != nil {
		return "", err
	}
	if _, err := command(ctx, root, "git", "config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"); err != nil {
		return "", err
	}
	if _, err := command(ctx, root, "git", "commit", "-m", fmt.Sprintf("sync(%s): %s [%s]", sourceID, plan.Candidate.Commit[:12], plan.PlanID)); err != nil {
		return "", err
	}
	head, err := command(ctx, root, "git", "rev-parse", "HEAD")
	return strings.TrimSpace(head), err
}

func command(ctx context.Context, directory, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func actionsRunURL() string {
	server, repository, runID := os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID")
	if server == "" || repository == "" || runID == "" {
		return ""
	}
	return strings.TrimSuffix(server, "/") + "/" + repository + "/actions/runs/" + runID
}

func withoutCredentials(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, item := range environment {
		name, _, _ := strings.Cut(item, "=")
		switch name {
		case "GH_TOKEN", "GITHUB_TOKEN", "HOMEBREW_TAP_TOKEN", "SSH_AUTH_SOCK":
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
