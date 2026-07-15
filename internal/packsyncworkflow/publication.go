package packsyncworkflow

import (
	"errors"
	"fmt"
	"strings"
)

const AutomationOwner = "github-actions[bot]"

type CandidateRelation string

const (
	CandidateSame       CandidateRelation = "same"
	CandidateAdvancing  CandidateRelation = "advancing"
	CandidateRegressive CandidateRelation = "regressive"
)

type ValidationGates struct {
	Provenance     bool `json:"provenance"`
	Classification bool `json:"classification"`
	Reacquisition  bool `json:"reacquisition"`
	Apply          bool `json:"apply"`
	Diff           bool `json:"diff"`
	Ownership      bool `json:"ownership"`
	MattySuite     bool `json:"matty_suite"`
}

func (gates ValidationGates) Complete() bool {
	return gates.Provenance && gates.Classification && gates.Reacquisition && gates.Apply && gates.Diff && gates.Ownership && gates.MattySuite
}

type Proposal struct {
	SourceID               string
	PlanID                 string
	BaseSHA                string
	CandidateSHA           string
	ResultTreeSHA          string
	ProvenanceSHA256       string
	ManagedTitle           string
	ManagedMetadataHash    string
	Validation             ValidationGates
	InvalidationConditions []string
}

func DecisionReadyInvalidationConditions() []string {
	return []string{"base_changed", "candidate_changed", "provenance_changed", "head_changed", "pr_state_changed"}
}

type BranchState struct {
	Exists              bool
	Name                string
	HeadSHA             string
	Owner               string
	ManagedMetadataHash string
	HumanCommits        bool
	Diverged            bool
}

type PRState struct {
	Exists       bool
	Number       int
	Open         bool
	BaseBranch   string
	HeadBranch   string
	HeadSHA      string
	MetadataHash string
	Owner        string
	Draft        bool
	AutoMerge    bool
}

type PublicationRecord struct {
	PlanID           string `json:"plan_id"`
	BaseSHA          string `json:"base_sha"`
	CandidateSHA     string `json:"candidate_sha"`
	HeadSHA          string `json:"head_sha"`
	ProvenanceSHA256 string `json:"provenance_sha256"`
	MetadataHash     string `json:"metadata_hash"`
}

type PublicationState struct {
	BaseSHA           string
	ProvenanceCurrent bool
	CandidateRelation CandidateRelation
	Branch            BranchState
	PR                PRState
	Record            PublicationRecord
	Writes            int
}

type PublicationAction string

const (
	PublicationCreate PublicationAction = "create"
	PublicationUpdate PublicationAction = "update"
	PublicationNoop   PublicationAction = "no-op"
	PublicationBlock  PublicationAction = "blocked"
)

type PublicationDecision struct {
	Action   PublicationAction
	Branch   string
	PRNumber int
	Blockers []string
}

func EvaluatePublication(proposal Proposal, state PublicationState) (PublicationDecision, error) {
	branch := "sync/" + proposal.SourceID
	decision := PublicationDecision{Action: PublicationBlock, Branch: branch, PRNumber: state.PR.Number}
	if err := validateProposal(proposal); err != nil {
		decision.Blockers = []string{err.Error()}
		return decision, err
	}
	if state.BaseSHA != proposal.BaseSHA {
		return blocked(decision, "repository base advanced; dispatch a fresh Check")
	}
	if !state.ProvenanceCurrent {
		return blocked(decision, "candidate provenance moved after inspection")
	}
	if state.CandidateRelation == CandidateRegressive {
		return blocked(decision, "candidate would regress the currently proposed source")
	}
	if !state.Branch.Exists && !state.PR.Exists {
		decision.Action = PublicationCreate
		return decision, nil
	}
	if !state.Branch.Exists || !state.PR.Exists {
		return blocked(decision, "stable branch or pull request ownership is absent or ambiguous")
	}
	if !state.PR.Open {
		return blocked(decision, "the owned source pull request is closed")
	}
	if state.Branch.Name != branch || state.PR.HeadBranch != branch || state.PR.BaseBranch != "main" {
		return blocked(decision, "branch or pull request identity is unexpected")
	}
	if state.Branch.Owner != AutomationOwner || state.PR.Owner != AutomationOwner {
		return blocked(decision, "automation ownership is absent or ambiguous")
	}
	if state.Branch.HumanCommits {
		return blocked(decision, "source branch contains human commits")
	}
	if state.Branch.Diverged || state.Branch.HeadSHA != state.PR.HeadSHA || state.Branch.HeadSHA != state.Record.HeadSHA {
		return blocked(decision, "source branch or pull request diverged from the last publication")
	}
	if state.PR.MetadataHash != state.Record.MetadataHash || state.Branch.ManagedMetadataHash != state.Record.MetadataHash {
		return blocked(decision, "managed pull request metadata was edited")
	}
	if state.Record.BaseSHA != proposal.BaseSHA || state.Record.ProvenanceSHA256 == "" {
		return blocked(decision, "publication record base or provenance is stale")
	}
	exact := state.Record.PlanID == proposal.PlanID && state.Record.CandidateSHA == proposal.CandidateSHA && state.Record.HeadSHA == proposal.ResultTreeSHA && state.Record.ProvenanceSHA256 == proposal.ProvenanceSHA256
	if exact {
		decision.Action = PublicationNoop
		return decision, nil
	}
	if state.Record.CandidateSHA == proposal.CandidateSHA {
		return blocked(decision, "plan or result identity is stale for the same candidate")
	}
	if state.CandidateRelation != CandidateAdvancing {
		return blocked(decision, "candidate advancement is not proven")
	}
	decision.Action = PublicationUpdate
	return decision, nil
}

func validateProposal(proposal Proposal) error {
	if !sourceIDPattern.MatchString(proposal.SourceID) || proposal.PlanID == "" || proposal.ManagedTitle == "" || !proposal.Validation.Complete() || len(proposal.InvalidationConditions) == 0 {
		return errors.New("proposal is incomplete or validation is not complete")
	}
	for name, value := range map[string]string{"base": proposal.BaseSHA, "candidate": proposal.CandidateSHA, "result tree": proposal.ResultTreeSHA} {
		if err := requireFullSHA(name, value); err != nil {
			return err
		}
	}
	if len(proposal.ProvenanceSHA256) != 64 || len(proposal.ManagedMetadataHash) != 64 {
		return errors.New("proposal provenance and managed metadata hashes are required")
	}
	return nil
}

func blocked(decision PublicationDecision, message string) (PublicationDecision, error) {
	decision.Blockers = []string{message}
	kind := FailureOwnership
	if strings.Contains(message, "provenance") || strings.Contains(message, "candidate") {
		kind = FailureProvenance
	} else if strings.Contains(message, "diverg") || strings.Contains(message, "base") || strings.Contains(message, "plan") || strings.Contains(message, "human commit") {
		kind = FailureDivergence
	}
	return decision, Failure{Kind: kind, Blocker: message, Recovery: recoveryForPublicationBlocker(message), Err: fmt.Errorf("publication blocked: %s", message)}
}

func recoveryForPublicationBlocker(message string) string {
	switch message {
	case "repository base advanced; dispatch a fresh Check", "plan or result identity is stale for the same candidate", "publication record base or provenance is stale":
		return "Start a fresh dispatch so Check seals the current base, candidate, plan, and provenance."
	case "candidate would regress the currently proposed source":
		return "Select a candidate that advances the current automation-owned proposal; never rewrite it backward."
	case "the owned source pull request is closed":
		return "A maintainer must decide whether to reopen the exact owned pull request; automation must not open a competitor."
	case "managed pull request metadata was edited":
		return "Review and preserve the human metadata edit; automation must not overwrite it."
	case "source branch contains human commits", "source branch or pull request diverged from the last publication":
		return "Review the branch manually and restore an unambiguous automation-owned state; automation must not force-push reviewer work."
	default:
		return "Restore one unambiguous automation-owned stable branch and pull request, then start a fresh Check."
	}
}

type ReadinessIdentity struct {
	PlanID           string `json:"plan_id"`
	BaseSHA          string `json:"base_sha"`
	HeadSHA          string `json:"head_sha"`
	CandidateSHA     string `json:"candidate_sha"`
	ProvenanceSHA256 string `json:"provenance_sha256"`
	PRNumber         int    `json:"pr_number"`
	PRStateSHA256    string `json:"pr_state_sha256"`
}

type Readiness struct {
	Identity            ReadinessIdentity `json:"identity"`
	Gates               ValidationGates   `json:"gates"`
	DecisionReady       bool              `json:"decision_ready"`
	AutoMerge           bool              `json:"auto_merge"`
	ManualMergeRequired bool              `json:"manual_merge_required"`
}

func MarkDecisionReady(identity ReadinessIdentity, gates ValidationGates, draft, autoMerge bool) (Readiness, error) {
	ready := Readiness{Identity: identity, Gates: gates, AutoMerge: autoMerge, ManualMergeRequired: true}
	if identity.PlanID == "" || identity.PRNumber < 1 || requireFullSHA("base", identity.BaseSHA) != nil || requireFullSHA("head", identity.HeadSHA) != nil || requireFullSHA("candidate", identity.CandidateSHA) != nil || len(identity.ProvenanceSHA256) != 64 || len(identity.PRStateSHA256) != 64 {
		return ready, errors.New("readiness identity is incomplete")
	}
	if !gates.Complete() || draft || autoMerge {
		return ready, errors.New("decision readiness requires every gate, a non-draft PR, and disabled auto-merge")
	}
	ready.DecisionReady = true
	return ready, nil
}

func (readiness Readiness) InvalidatedBy(observed ReadinessIdentity) bool {
	return !readiness.DecisionReady || readiness.Identity != observed
}
