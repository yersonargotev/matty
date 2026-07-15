package packsync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type ClassifierType string

const (
	ClassifierAI    ClassifierType = "ai"
	ClassifierHuman ClassifierType = "human"
)

type ClassifierIdentity struct {
	Type ClassifierType `json:"type"`
	ID   string         `json:"id"`
}

type ClassificationEvidence struct {
	PackID          string              `json:"pack_id"`
	Classifier      ClassifierIdentity  `json:"classifier"`
	Rationale       string              `json:"rationale"`
	CurrentVersion  string              `json:"current_version"`
	ProposedVersion string              `json:"proposed_version"`
	ChangedAspects  []string            `json:"changed_aspects"`
	MechanicalFloor ClassificationLevel `json:"mechanical_floor"`
	FinalLevel      ClassificationLevel `json:"final_level"`
	Migration       string              `json:"migration,omitempty"`
	RequiredActions []string            `json:"required_actions,omitempty"`
}

type ClassificationEvidenceSet struct {
	SchemaVersion     int                      `json:"schema_version"`
	PlanID            string                   `json:"plan_id"`
	BaseSHA           string                   `json:"base_sha"`
	Candidate         Candidate                `json:"candidate"`
	HumanInspectionID string                   `json:"human_inspection_id,omitempty"`
	Evidence          []ClassificationEvidence `json:"evidence"`
}

func ValidateClassificationEvidence(plan Plan, set ClassificationEvidenceSet) error {
	if !plan.VerifySeal() || plan.SchemaVersion != 1 || plan.Status != "review-required" || !plan.Authoritative || len(plan.Blockers) != 0 {
		return errors.New("classification requires a canonical sealed Check plan")
	}
	if len(set.Evidence) != len(plan.AffectedPacks) {
		return errors.New("classification requires complete evidence coverage for every affected pack")
	}
	if set.SchemaVersion != 1 || set.PlanID != plan.PlanID {
		return errors.New("classification evidence plan identity is stale or malformed")
	}
	if set.BaseSHA == "" || set.BaseSHA != plan.Preconditions.BaseCommit {
		return errors.New("classification evidence base SHA is stale or malformed")
	}
	if !reflect.DeepEqual(set.Candidate, plan.Candidate) {
		return errors.New("classification evidence candidate is stale or contradictory")
	}
	impacts := make(map[string]PackImpact, len(plan.AffectedPacks))
	for _, impact := range plan.AffectedPacks {
		if impact.PackID == "" || impacts[impact.PackID].PackID != "" {
			return errors.New("canonical plan contains malformed affected-pack coverage")
		}
		impacts[impact.PackID] = impact
	}
	seen := make(map[string]bool, len(set.Evidence))
	var classifierType ClassifierType
	for index, evidence := range set.Evidence {
		if evidence.PackID != plan.AffectedPacks[index].PackID {
			return errors.New("classification evidence is not in canonical affected-pack order")
		}
		impact, ok := impacts[evidence.PackID]
		if !ok || seen[evidence.PackID] {
			return errors.New("classification requires complete evidence coverage without duplicate or unrelated packs")
		}
		seen[evidence.PackID] = true
		if err := validatePackClassification(impact, evidence); err != nil {
			return fmt.Errorf("pack %s: %w", evidence.PackID, err)
		}
		if classifierType == "" {
			classifierType = evidence.Classifier.Type
		} else if classifierType != evidence.Classifier.Type {
			return errors.New("classification evidence set contradicts one explicit classifier mode")
		}
	}
	if classifierType == ClassifierHuman {
		inspectionID, err := HumanInspectionID(plan)
		if err != nil || set.HumanInspectionID != inspectionID {
			return errors.New("human classification evidence is not bound to the canonical inspection-first dispatch")
		}
	} else if set.HumanInspectionID != "" {
		return errors.New("AI classification evidence contradicts a human inspection binding")
	}
	return nil
}

func HumanInspectionID(plan Plan) (string, error) {
	if !plan.VerifySeal() || plan.SchemaVersion != 1 || plan.Status != "review-required" || !plan.Authoritative || len(plan.Blockers) != 0 || len(plan.AffectedPacks) == 0 {
		return "", errors.New("human inspection requires a canonical sealed Check plan")
	}
	digest := sha256.Sum256([]byte(plan.PlanID + "\x00human-inspection"))
	return "pack-classification-inspection-" + hex.EncodeToString(digest[:]), nil
}

func validateApplyClassification(plan Plan, set ClassificationEvidenceSet) error {
	if len(plan.AffectedPacks) == 0 {
		if len(set.Evidence) != 0 || set.PlanID != "" || set.BaseSHA != "" || set.SchemaVersion != 0 {
			return errors.New("classification evidence was supplied for a plan with no affected packs")
		}
		return nil
	}
	return ValidateClassificationEvidence(plan, set)
}

func validatePackClassification(impact PackImpact, evidence ClassificationEvidence) error {
	if (evidence.Classifier.Type != ClassifierAI && evidence.Classifier.Type != ClassifierHuman) || strings.TrimSpace(evidence.Classifier.ID) == "" {
		return errors.New("classifier type and identity are malformed")
	}
	rationale := strings.TrimSpace(evidence.Rationale)
	if rationale == "" || len(rationale) > 500 {
		return errors.New("classifier rationale is missing or not concise")
	}
	if evidence.CurrentVersion != impact.CurrentVersion {
		return errors.New("current version contradicts the sealed plan")
	}
	if evidence.MechanicalFloor != impact.MechanicalFloor {
		return errors.New("mechanical floor contradicts the engine-owned floor")
	}
	if !validFinalLevel(evidence.FinalLevel) {
		return errors.New("final classification level is malformed")
	}
	if classificationRank(evidence.FinalLevel) < classificationRank(impact.MechanicalFloor) {
		return fmt.Errorf("final classification %s is below mechanical floor %s", evidence.FinalLevel, impact.MechanicalFloor)
	}
	want, err := nextVersion(impact.CurrentVersion, evidence.FinalLevel)
	if err != nil || evidence.ProposedVersion != want {
		return fmt.Errorf("proposed version must be the exact next %s version %s", evidence.FinalLevel, want)
	}
	if err := validateNonemptyUnique("changed aspects", evidence.ChangedAspects); err != nil {
		return err
	}
	if evidence.FinalLevel == LevelMajor {
		if strings.TrimSpace(evidence.Migration) == "" || validateNonemptyUnique("required actions", evidence.RequiredActions) != nil {
			return errors.New("major classification requires migration and mandatory actions")
		}
	} else if strings.TrimSpace(evidence.Migration) != "" || len(evidence.RequiredActions) != 0 {
		return errors.New("migration or mandatory actions contradict a non-major classification")
	}
	return nil
}

func validFinalLevel(level ClassificationLevel) bool {
	return level == LevelPatch || level == LevelMinor || level == LevelMajor
}

func validateNonemptyUnique(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s are missing", name)
	}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return fmt.Errorf("%s are malformed or duplicated", name)
		}
		seen[value] = true
	}
	return nil
}

func nextVersion(current string, level ClassificationLevel) (string, error) {
	parts := strings.Split(current, ".")
	if len(parts) != 3 {
		return "", errors.New("current pack version is not a three-part SemVer")
	}
	values := make([]int, 3)
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return "", errors.New("current pack version is not canonical SemVer")
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return "", errors.New("current pack version is not canonical SemVer")
		}
		values[i] = value
	}
	switch level {
	case LevelPatch:
		values[2]++
	case LevelMinor:
		values[1]++
		values[2] = 0
	case LevelMajor:
		values[0]++
		values[1], values[2] = 0, 0
	default:
		return "", fmt.Errorf("invalid classification level %q", level)
	}
	return fmt.Sprintf("%d.%d.%d", values[0], values[1], values[2]), nil
}
