package packsync

import (
	"reflect"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestValidateClassificationEvidenceCoversEveryAffectedPackAndFailsClosed(t *testing.T) {
	plan := classificationPlan(t)
	valid := validClassificationEvidence(plan)
	if err := ValidateClassificationEvidence(plan, valid); err != nil {
		t.Fatalf("valid evidence = %v", err)
	}
	if valid.Evidence[0].FinalLevel != LevelMinor || valid.Evidence[0].MechanicalFloor != LevelNone {
		t.Fatal("classifier elevation above the mechanical floor was not represented")
	}

	tests := []struct {
		name   string
		want   string
		mutate func(*ClassificationEvidenceSet)
	}{
		{name: "missing evidence", want: "complete evidence coverage", mutate: func(set *ClassificationEvidenceSet) { set.Evidence = nil }},
		{name: "incomplete multi-pack coverage", want: "complete evidence coverage", mutate: func(set *ClassificationEvidenceSet) { set.Evidence = set.Evidence[:1] }},
		{name: "malformed classifier", want: "classifier", mutate: func(set *ClassificationEvidenceSet) { set.Evidence[0].Classifier.Type = "bot" }},
		{name: "malformed aspects", want: "changed aspects", mutate: func(set *ClassificationEvidenceSet) { set.Evidence[0].ChangedAspects = []string{""} }},
		{name: "contradictory current version", want: "current version", mutate: func(set *ClassificationEvidenceSet) { set.Evidence[0].CurrentVersion = "9.9.9" }},
		{name: "contradictory floor", want: "mechanical floor", mutate: func(set *ClassificationEvidenceSet) { set.Evidence[0].MechanicalFloor = LevelPatch }},
		{name: "below floor", want: "below mechanical floor", mutate: func(set *ClassificationEvidenceSet) {
			set.Evidence[1].FinalLevel = LevelMinor
			set.Evidence[1].ProposedVersion = "2.1.0"
		}},
		{name: "arbitrary version", want: "exact next", mutate: func(set *ClassificationEvidenceSet) { set.Evidence[0].ProposedVersion = "1.9.0" }},
		{name: "major without migration", want: "major classification", mutate: func(set *ClassificationEvidenceSet) { set.Evidence[1].Migration = "" }},
		{name: "major without actions", want: "major classification", mutate: func(set *ClassificationEvidenceSet) { set.Evidence[1].RequiredActions = nil }},
		{name: "stale plan", want: "plan identity", mutate: func(set *ClassificationEvidenceSet) { set.PlanID = "other" }},
		{name: "stale base", want: "base SHA", mutate: func(set *ClassificationEvidenceSet) { set.BaseSHA = strings.Repeat("b", 40) }},
		{name: "stale candidate", want: "candidate", mutate: func(set *ClassificationEvidenceSet) { set.Candidate.Commit = strings.Repeat("c", 40) }},
		{name: "duplicate pack", want: "canonical affected-pack order", mutate: func(set *ClassificationEvidenceSet) { set.Evidence[1].PackID = set.Evidence[0].PackID }},
		{name: "non-canonical pack order", want: "canonical affected-pack order", mutate: func(set *ClassificationEvidenceSet) {
			set.Evidence[0], set.Evidence[1] = set.Evidence[1], set.Evidence[0]
		}},
		{name: "human evidence without inspection", want: "inspection-first", mutate: func(set *ClassificationEvidenceSet) {
			for index := range set.Evidence {
				set.Evidence[index].Classifier = ClassifierIdentity{Type: ClassifierHuman, ID: "maintainer"}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := cloneClassificationEvidenceSet(valid)
			test.mutate(&set)
			if err := ValidateClassificationEvidence(plan, set); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateClassificationEvidenceRejectsTamperedCanonicalPlan(t *testing.T) {
	plan := classificationPlan(t)
	set := validClassificationEvidence(plan)
	plan.AffectedPacks[0].MechanicalFloor = LevelMajor
	if err := ValidateClassificationEvidence(plan, set); err == nil || !strings.Contains(err.Error(), "canonical sealed Check plan") {
		t.Fatalf("error = %v", err)
	}
}

func classificationPlan(t *testing.T) Plan {
	t.Helper()
	plan := Plan{
		SchemaVersion: 1,
		Status:        "review-required",
		Authoritative: true,
		Candidate:     acceptedCandidate(),
		Preconditions: Preconditions{BaseCommit: strings.Repeat("a", 40)},
		AffectedPacks: []PackImpact{
			{PackID: "alpha", CurrentVersion: "1.2.3", MechanicalFloor: LevelNone, SemanticEvidenceRequired: true, Reasons: []string{"upstream-owned content changed"}},
			{PackID: "beta", CurrentVersion: "2.0.0", MechanicalFloor: LevelMajor, Reasons: []string{"selected resource removed"}},
		},
	}
	resealPlan(t, &plan)
	return plan
}

func validClassificationEvidence(plan Plan) ClassificationEvidenceSet {
	return ClassificationEvidenceSet{
		SchemaVersion: 1,
		PlanID:        plan.PlanID,
		BaseSHA:       plan.Preconditions.BaseCommit,
		Candidate:     plan.Candidate,
		Evidence: []ClassificationEvidence{
			{PackID: "alpha", Classifier: ClassifierIdentity{Type: ClassifierAI, ID: "fixture-model"}, Rationale: "New behavior is backward compatible.", CurrentVersion: "1.2.3", ProposedVersion: "1.3.0", ChangedAspects: []string{"skill guidance"}, MechanicalFloor: LevelNone, FinalLevel: LevelMinor},
			{PackID: "beta", Classifier: ClassifierIdentity{Type: ClassifierAI, ID: "fixture-model"}, Rationale: "A selected capability was removed.", CurrentVersion: "2.0.0", ProposedVersion: "3.0.0", ChangedAspects: []string{"available skills"}, MechanicalFloor: LevelMajor, FinalLevel: LevelMajor, Migration: "Move callers to alpha.", RequiredActions: []string{"Update active pack references."}},
		},
	}
}

func cloneClassificationEvidenceSet(set ClassificationEvidenceSet) ClassificationEvidenceSet {
	clone := set
	clone.Evidence = append([]ClassificationEvidence(nil), set.Evidence...)
	for i := range clone.Evidence {
		clone.Evidence[i].ChangedAspects = append([]string(nil), set.Evidence[i].ChangedAspects...)
		clone.Evidence[i].RequiredActions = append([]string(nil), set.Evidence[i].RequiredActions...)
	}
	if !reflect.DeepEqual(clone, set) {
		panic("classification evidence clone changed values")
	}
	return clone
}

func classificationEvidenceForPlan(t *testing.T, plan Plan, classifierType ClassifierType, classifierID string, level ClassificationLevel) ClassificationEvidenceSet {
	t.Helper()
	set := ClassificationEvidenceSet{SchemaVersion: 1, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, Candidate: plan.Candidate}
	for _, impact := range plan.AffectedPacks {
		final := level
		if classificationRank(impact.MechanicalFloor) > classificationRank(final) {
			final = impact.MechanicalFloor
		}
		version, err := nextVersion(impact.CurrentVersion, final)
		if err != nil {
			t.Fatal(err)
		}
		evidence := ClassificationEvidence{PackID: impact.PackID, Classifier: ClassifierIdentity{Type: classifierType, ID: classifierID}, Rationale: "Fixture classification rationale.", CurrentVersion: impact.CurrentVersion, ProposedVersion: version, ChangedAspects: []string{"observable fixture behavior"}, MechanicalFloor: impact.MechanicalFloor, FinalLevel: final}
		if final == LevelMajor {
			evidence.Migration = "Follow the fixture migration."
			evidence.RequiredActions = []string{"Complete the fixture action."}
		}
		set.Evidence = append(set.Evidence, evidence)
	}
	return set
}

func initializeFixtureGit(t *testing.T, root string) {
	t.Helper()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("."); err != nil {
		t.Fatal(err)
	}
	_, err = worktree.Commit("fixture base", &git.CommitOptions{Author: &object.Signature{Name: "Matty test", Email: "test@matty.invalid", When: time.Unix(1, 0).UTC()}})
	if err != nil {
		t.Fatal(err)
	}
}
