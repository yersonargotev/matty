package packsyncworkflow

import (
	"context"
	"errors"
	"reflect"

	"github.com/yersonargotev/packy/internal/packsync"
)

// Applier is the existing packsync transaction seam. Implementations must be
// packsync.Engine or a behaviorally equivalent test fake; workflow code does
// not materialize candidate bytes or pack versions itself.
type Applier interface {
	Apply(context.Context, packsync.ApplyRequest) (packsync.ApplyResult, error)
	RecoverPending(context.Context, string) (packsync.ApplyResult, bool, error)
}

// Validator runs the complete Packy-owned validation authority against the
// sandbox checkout after Apply and before publication credentials are used.
type Validator interface {
	Validate(context.Context, string) error
}

// AppliedValidator optionally distinguishes validation of a repository whose
// sealed plan has already been applied from ordinary repository validation.
type AppliedValidator interface {
	ValidateApplied(context.Context, string) error
}

type ProposalBuilder interface {
	Build(context.Context, string, packsync.ApplyResult) (Proposal, error)
}

type DiffVerifier interface {
	Seal(context.Context, string) (string, error)
	VerifyWorkspace(context.Context, string, string) error
	VerifyCommit(context.Context, string, string, string) error
}

type ProvenanceVerifier interface {
	RevalidateCandidate(context.Context, packsync.Plan) error
}

// PublicationGateway is the narrow GitHub edge. Observe must return detached
// state; Publish is called only after two identical fail-closed observations.
type PublicationGateway interface {
	Prepare(Proposal) (Proposal, error)
	Observe(context.Context, string) (PublicationState, error)
	Publish(context.Context, Proposal, PublicationDecision) (PRState, error)
	Finalize(context.Context, Proposal, PublicationDecision, PRState) (string, error)
}

type PublishRequest struct {
	RepositoryRoot string
	Apply          packsync.ApplyRequest
}

type PublishResult struct {
	Apply       packsync.ApplyResult
	Proposal    Proposal
	Decision    PublicationDecision
	PullRequest PRState
	Readiness   Readiness
}

type Publisher struct {
	Applier    Applier
	Validator  Validator
	Builder    ProposalBuilder
	Diff       DiffVerifier
	Provenance ProvenanceVerifier
	GitHub     PublicationGateway
}

// Run completes sandbox Apply and the full validation gate before observing
// or writing GitHub state. GitHub state is then observed twice so a last-moment
// reviewer edit, base movement, or ownership change fails before Publish.
func (publisher Publisher) Run(ctx context.Context, request PublishRequest) (PublishResult, error) {
	if publisher.Applier == nil || publisher.Validator == nil || publisher.Builder == nil || publisher.Diff == nil || publisher.Provenance == nil || publisher.GitHub == nil || request.RepositoryRoot == "" {
		return PublishResult{}, errors.New("publish requires Apply, validation, proposal construction, provenance revalidation, GitHub, and a sandbox repository")
	}
	recovered, pending, err := publisher.Applier.RecoverPending(ctx, request.RepositoryRoot)
	if err != nil {
		return PublishResult{}, err
	}
	var result packsync.ApplyResult
	if pending && recovered.Status == "completed" {
		if recovered.PlanID != request.Apply.Plan.PlanID {
			return PublishResult{}, Failure{Kind: FailureIntegrity, Err: errors.New("recovered transaction belongs to a different sealed plan")}
		}
		result = recovered
	} else {
		result, err = publisher.Applier.Apply(ctx, request.Apply)
		if err != nil {
			return PublishResult{}, err
		}
	}
	diffSeal, err := publisher.Diff.Seal(ctx, request.RepositoryRoot)
	if err != nil {
		return PublishResult{}, Failure{Kind: FailureIntegrity, Err: err}
	}
	var validationErr error
	if validator, ok := publisher.Validator.(AppliedValidator); ok {
		validationErr = validator.ValidateApplied(ctx, request.RepositoryRoot)
	} else {
		validationErr = publisher.Validator.Validate(ctx, request.RepositoryRoot)
	}
	if validationErr != nil {
		return PublishResult{}, Failure{Kind: FailureValidation, Err: validationErr}
	}
	if err := publisher.Diff.VerifyWorkspace(ctx, request.RepositoryRoot, diffSeal); err != nil {
		return PublishResult{}, Failure{Kind: FailureValidation, Err: errors.New("validation changed the sealed Apply diff")}
	}
	proposal, err := publisher.Builder.Build(ctx, request.RepositoryRoot, result)
	if err != nil {
		return PublishResult{}, err
	}
	proposal.ResultTreeSHA = diffSeal
	if err := publisher.Diff.VerifyCommit(ctx, request.RepositoryRoot, diffSeal, proposal.HeadSHA); err != nil {
		return PublishResult{}, Failure{Kind: FailureIntegrity, Err: errors.New("publication commit does not match the sealed Apply diff")}
	}
	// These gates are derived here from the completed owner-controlled sequence,
	// never asserted by the workflow adapter. Ownership is admitted only if the
	// fail-closed state evaluation below succeeds.
	proposal.Validation = ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, PackySuite: true}
	proposal.InvalidationConditions = DecisionReadyInvalidationConditions()
	proposal, err = publisher.GitHub.Prepare(proposal)
	if err != nil {
		return PublishResult{}, err
	}
	if err := validateProposal(proposal); err != nil {
		return PublishResult{}, err
	}
	if err := publisher.Provenance.RevalidateCandidate(ctx, request.Apply.Plan); err != nil {
		return PublishResult{}, Failure{Kind: FailureProvenance, Err: errors.New("candidate provenance changed after sandbox validation")}
	}
	first, err := publisher.GitHub.Observe(ctx, proposal.SourceID)
	if err != nil {
		return PublishResult{}, err
	}
	first.ProvenanceCurrent = true
	decision, err := EvaluatePublication(proposal, first)
	if err != nil {
		return PublishResult{Apply: result, Decision: decision}, err
	}
	if err := publisher.Provenance.RevalidateCandidate(ctx, request.Apply.Plan); err != nil {
		return PublishResult{}, Failure{Kind: FailureProvenance, Err: errors.New("candidate provenance changed during final publication revalidation")}
	}
	second, err := publisher.GitHub.Observe(ctx, proposal.SourceID)
	if err != nil {
		return PublishResult{}, err
	}
	second.ProvenanceCurrent = true
	if !reflect.DeepEqual(first, second) {
		return PublishResult{Apply: result, Decision: PublicationDecision{Action: PublicationBlock, Branch: "sync/" + proposal.SourceID, Blockers: []string{"publication state changed during final revalidation"}}}, Failure{Kind: FailureDivergence, Err: errors.New("publication state changed during final revalidation")}
	}
	if decision.Action == PublicationNoop {
		ready, err := readinessFor(proposal, second.PR)
		return PublishResult{Apply: result, Proposal: proposal, Decision: decision, PullRequest: second.PR, Readiness: ready}, err
	}
	pr, err := publisher.GitHub.Publish(ctx, proposal, decision)
	if err != nil {
		return PublishResult{}, err
	}
	published, err := publisher.GitHub.Observe(ctx, proposal.SourceID)
	if err != nil {
		return PublishResult{}, err
	}
	published.ProvenanceCurrent = true
	if err := validatePublishedState(proposal, decision, pr, published); err != nil {
		return PublishResult{}, err
	}
	if err := publisher.Provenance.RevalidateCandidate(ctx, request.Apply.Plan); err != nil {
		return PublishResult{}, Failure{Kind: FailureProvenance, Blocker: "candidate provenance moved after the draft publication", Recovery: "Leave the draft blocked and start a fresh Check; never finalize stale provenance.", Err: errors.New("candidate provenance changed before finalization")}
	}
	finalHash, err := publisher.GitHub.Finalize(ctx, proposal, decision, published.PR)
	if err != nil {
		return PublishResult{}, err
	}
	if err := publisher.Provenance.RevalidateCandidate(ctx, request.Apply.Plan); err != nil {
		return PublishResult{}, Failure{Kind: FailureProvenance, Blocker: "candidate provenance moved before decision readiness", Recovery: "Treat the pull request as not ready and start a fresh Check for the moved identity.", Err: errors.New("candidate provenance changed before readiness")}
	}
	final, err := publisher.GitHub.Observe(ctx, proposal.SourceID)
	if err != nil {
		return PublishResult{}, err
	}
	final.ProvenanceCurrent = true
	exactRecord := final.Record.PlanID == proposal.PlanID && final.Record.BaseSHA == proposal.BaseSHA && final.Record.CandidateSHA == proposal.CandidateSHA && final.Record.HeadSHA == proposal.HeadSHA && final.Record.ResultTreeSHA == proposal.ResultTreeSHA && final.Record.ProvenanceSHA256 == proposal.ProvenanceSHA256 && final.Record.MetadataHash == finalHash
	if final.BaseSHA != proposal.BaseSHA || final.CandidateRelation == CandidateRegressive || !final.Branch.Exists || final.Branch.Name != "sync/"+proposal.SourceID || final.Branch.HeadSHA != proposal.HeadSHA || final.Branch.Owner != AutomationOwner || final.Branch.Diverged || final.Branch.HumanCommits || !final.PR.Exists || !final.PR.Open || final.PR.Number != published.PR.Number || final.PR.BaseBranch != "main" || final.PR.HeadBranch != "sync/"+proposal.SourceID || final.PR.HeadSHA != proposal.HeadSHA || final.PR.MetadataHash != finalHash || final.PR.Owner != AutomationOwner || final.PR.Draft || final.PR.AutoMerge || !exactRecord {
		return PublishResult{}, Failure{Kind: FailureOwnership, Blocker: "final pull request identity changed before readiness could be established", Recovery: "Preserve the observed pull request for manual review; do not overwrite it or open a competitor.", Err: errors.New("final pull request identity failed closed")}
	}
	ready, err := readinessFor(proposal, final.PR)
	if err != nil {
		return PublishResult{}, err
	}
	return PublishResult{Apply: result, Proposal: proposal, Decision: decision, PullRequest: final.PR, Readiness: ready}, nil
}

func validatePublishedState(proposal Proposal, decision PublicationDecision, returned PRState, state PublicationState) error {
	if !state.Branch.Exists || !state.PR.Exists || !state.PR.Open || state.PR.Number != returned.Number || state.PR.HeadSHA != proposal.HeadSHA || state.Branch.HeadSHA != proposal.HeadSHA || state.PR.MetadataHash != proposal.ManagedMetadataHash || state.Record.MetadataHash != proposal.ManagedMetadataHash || state.PR.Owner != AutomationOwner || state.Branch.Owner != AutomationOwner || state.PR.AutoMerge {
		return Failure{Kind: FailureOwnership, Blocker: "draft publication state did not match the exact automation-owned proposal", Recovery: "Inspect the stable branch and pull request manually; never overwrite changed reviewer state or open a competitor.", Err: errors.New("published state failed exact reobservation")}
	}
	if decision.Action == PublicationCreate && !state.PR.Draft {
		return Failure{Kind: FailureOwnership, Blocker: "the first pull request was not created as a protected draft", Recovery: "Keep the pull request blocked for manual inspection; automation must not declare it ready.", Err: errors.New("new pull request was not draft")}
	}
	return nil
}

func readinessFor(proposal Proposal, pr PRState) (Readiness, error) {
	identity := ReadinessIdentity{PlanID: proposal.PlanID, BaseSHA: proposal.BaseSHA, HeadSHA: pr.HeadSHA, CandidateSHA: proposal.CandidateSHA, ProvenanceSHA256: proposal.ProvenanceSHA256, PRNumber: pr.Number, PRStateSHA256: pr.MetadataHash}
	return MarkDecisionReady(identity, proposal.Validation, pr.Draft, pr.AutoMerge)
}
