package packclassification

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/packsync"
)

type Mode string

const (
	ModeAI    Mode = "ai"
	ModeHuman Mode = "human"
)

var ErrModelUnavailable = errors.New("classification model unavailable after bounded retries")

type Request struct {
	SchemaVersion            int                          `json:"schema_version"`
	RequestID                string                       `json:"request_id"`
	Mode                     Mode                         `json:"mode,omitempty"`
	PlanID                   string                       `json:"plan_id"`
	BaseSHA                  string                       `json:"base_sha"`
	Candidate                packsync.Candidate           `json:"candidate"`
	PackID                   string                       `json:"pack_id"`
	CurrentVersion           string                       `json:"current_version"`
	MechanicalFloor          packsync.ClassificationLevel `json:"mechanical_floor"`
	SemanticEvidenceRequired bool                         `json:"semantic_evidence_required"`
	MechanicalReasons        []string                     `json:"mechanical_reasons"`
	Changes                  []packsync.Change            `json:"changes"`
}

func requestsForMode(plan packsync.Plan, mode Mode) ([]Request, error) {
	if err := packsync.ValidateClassificationPlan(plan); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	requests := make([]Request, 0, len(plan.AffectedPacks))
	for _, impact := range plan.AffectedPacks {
		if impact.PackID == "" || seen[impact.PackID] {
			return nil, errors.New("canonical sealed Check plan has malformed affected-pack coverage")
		}
		seen[impact.PackID] = true
		request := Request{SchemaVersion: 1, RequestID: plan.PlanID + "/" + impact.PackID, Mode: mode, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, Candidate: plan.Candidate, PackID: impact.PackID, CurrentVersion: impact.CurrentVersion, MechanicalFloor: impact.MechanicalFloor, SemanticEvidenceRequired: impact.SemanticEvidenceRequired, MechanicalReasons: append([]string(nil), impact.Reasons...)}
		for _, change := range plan.Changes {
			if change.PackID == impact.PackID {
				request.Changes = append(request.Changes, change)
			}
		}
		requests = append(requests, request)
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].PackID < requests[j].PackID })
	return requests, nil
}

type Model interface {
	Attempt(context.Context, Request) (packsync.ClassificationEvidence, error)
}

type AIService struct {
	model       Model
	maxAttempts int
}

func NewAIService(model Model, maxAttempts int) (AIService, error) {
	if model == nil {
		return AIService{}, errors.New("classification model is required")
	}
	if maxAttempts < 1 || maxAttempts > 5 {
		return AIService{}, errors.New("classification model attempts must be bounded from one through five")
	}
	return AIService{model: model, maxAttempts: maxAttempts}, nil
}

func (service AIService) Classify(ctx context.Context, plan packsync.Plan) (packsync.ClassificationEvidenceSet, error) {
	requests, err := requestsForMode(plan, ModeAI)
	if err != nil {
		return packsync.ClassificationEvidenceSet{}, err
	}
	set := packsync.ClassificationEvidenceSet{SchemaVersion: 1, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, Candidate: plan.Candidate}
	var unavailable []string
	var invalidAI []string
	for _, request := range requests {
		var evidence packsync.ClassificationEvidence
		var classifyErr error
		for attempt := 0; attempt < service.maxAttempts; attempt++ {
			if err := ctx.Err(); err != nil {
				return packsync.ClassificationEvidenceSet{}, err
			}
			evidence, classifyErr = service.model.Attempt(ctx, request)
			if classifyErr == nil {
				break
			}
		}
		if classifyErr != nil {
			unavailable = append(unavailable, request.PackID)
			continue
		}
		if evidence.PackID != request.PackID || evidence.Classifier.Type != packsync.ClassifierAI {
			invalidAI = append(invalidAI, request.PackID)
		}
		set.Evidence = append(set.Evidence, evidence)
	}
	if len(unavailable) != 0 {
		return packsync.ClassificationEvidenceSet{}, fmt.Errorf("%w: %s", ErrModelUnavailable, strings.Join(unavailable, ", "))
	}
	if len(invalidAI) != 0 {
		return packsync.ClassificationEvidenceSet{}, fmt.Errorf("AI classifier returned contradictory identity or mode for packs: %s", strings.Join(invalidAI, ", "))
	}
	if err := packsync.ValidateClassificationEvidence(plan, set); err != nil {
		return packsync.ClassificationEvidenceSet{}, fmt.Errorf("validate untrusted AI classification evidence: %w", err)
	}
	return set, nil
}

type HumanInspection struct {
	SchemaVersion int                `json:"schema_version"`
	InspectionID  string             `json:"inspection_id"`
	PlanID        string             `json:"plan_id"`
	BaseSHA       string             `json:"base_sha"`
	Candidate     packsync.Candidate `json:"candidate"`
	Requests      []Request          `json:"requests"`
}

func InspectHuman(plan packsync.Plan) (HumanInspection, error) {
	requests, err := requestsForMode(plan, ModeHuman)
	if err != nil {
		return HumanInspection{}, err
	}
	inspection := HumanInspection{SchemaVersion: 1, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, Candidate: plan.Candidate, Requests: requests}
	inspection.InspectionID, err = packsync.HumanInspectionID(plan)
	return inspection, err
}

func SupplyHumanEvidence(plan packsync.Plan, inspection HumanInspection, set packsync.ClassificationEvidenceSet) (packsync.ClassificationEvidenceSet, error) {
	wantRequests, err := requestsForMode(plan, ModeHuman)
	if err != nil {
		return packsync.ClassificationEvidenceSet{}, err
	}
	wantInspectionID, inspectionErr := packsync.HumanInspectionID(plan)
	if inspectionErr != nil || inspection.SchemaVersion != 1 || inspection.InspectionID == "" || inspection.InspectionID != wantInspectionID || inspection.PlanID != plan.PlanID || inspection.BaseSHA != plan.Preconditions.BaseCommit || !reflect.DeepEqual(inspection.Candidate, plan.Candidate) || !reflect.DeepEqual(inspection.Requests, wantRequests) {
		return packsync.ClassificationEvidenceSet{}, errors.New("human evidence requires the exact canonical inspection-first dispatch")
	}
	for _, evidence := range set.Evidence {
		if evidence.Classifier.Type != packsync.ClassifierHuman {
			return packsync.ClassificationEvidenceSet{}, errors.New("human evidence dispatch contains a non-human classifier")
		}
	}
	if set.HumanInspectionID != inspection.InspectionID {
		return packsync.ClassificationEvidenceSet{}, errors.New("human evidence is not bound to the supplied inspection")
	}
	if err := packsync.ValidateClassificationEvidence(plan, set); err != nil {
		return packsync.ClassificationEvidenceSet{}, fmt.Errorf("validate untrusted human classification evidence: %w", err)
	}
	return set, nil
}
