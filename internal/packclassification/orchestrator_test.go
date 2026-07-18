package packclassification

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yersonargotev/packy/internal/packsync"
)

func TestRequestsEmitExactlyOneRequestForEveryAffectedPackFromSealedCheck(t *testing.T) {
	fixture := newClassificationFixture(t)
	inspection, err := InspectHuman(fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	requests := inspection.Requests
	if len(requests) != 2 || requests[0].PackID != "alpha" || requests[1].PackID != "beta" || requests[0].RequestID == requests[1].RequestID {
		t.Fatalf("requests = %#v", requests)
	}
	for _, request := range requests {
		if request.PlanID != fixture.plan.PlanID || request.BaseSHA != fixture.plan.Preconditions.BaseCommit || !reflect.DeepEqual(request.Candidate, fixture.plan.Candidate) {
			t.Fatalf("request is not bound to canonical Check: %#v", request)
		}
	}
	tampered := fixture.plan
	tampered.AffectedPacks[0].MechanicalFloor = packsync.LevelMajor
	if _, err := InspectHuman(tampered); err == nil || !strings.Contains(err.Error(), "canonical sealed Check plan") {
		t.Fatalf("tampered plan error = %v", err)
	}
}

func TestAIClassificationUsesStructuredEvidenceAndFailsClosedAfterBoundedRetries(t *testing.T) {
	fixture := newClassificationFixture(t)
	model := &fixtureModel{}
	service, err := NewAIService(model, 3)
	if err != nil {
		t.Fatal(err)
	}
	set, err := service.Classify(context.Background(), fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 2 || len(set.Evidence) != 2 {
		t.Fatalf("model requests=%d evidence=%d", len(model.requests), len(set.Evidence))
	}
	if err := packsync.ValidateClassificationEvidence(fixture.plan, set); err != nil {
		t.Fatalf("structured AI evidence = %v", err)
	}

	unavailable := &fixtureModel{err: errors.New("model offline")}
	service, err = NewAIService(unavailable, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Classify(context.Background(), fixture.plan); !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("unavailable model error = %v", err)
	}
	if len(unavailable.requests) != 4 {
		t.Fatalf("retry calls = %d, want two attempts for each of two packs", len(unavailable.requests))
	}
	for _, request := range unavailable.requests {
		if request.Mode != ModeAI {
			t.Fatalf("implicit human fallback request = %#v", request)
		}
	}
}

func TestHumanClassificationRequiresInspectionThenBoundEvidenceDispatch(t *testing.T) {
	fixture := newClassificationFixture(t)
	set := fixtureEvidence(fixture.plan, packsync.ClassifierHuman, "maintainer")
	if _, err := SupplyHumanEvidence(fixture.plan, HumanInspection{}, set); err == nil || !strings.Contains(err.Error(), "inspection") {
		t.Fatalf("evidence-first error = %v", err)
	}
	inspection, err := InspectHuman(fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	set.HumanInspectionID = inspection.InspectionID
	validated, err := SupplyHumanEvidence(fixture.plan, inspection, set)
	if err != nil || !reflect.DeepEqual(validated, set) {
		t.Fatalf("second dispatch = %#v, %v", validated, err)
	}
	inspection.Requests[0].PackID = "other"
	if _, err := SupplyHumanEvidence(fixture.plan, inspection, set); err == nil || !strings.Contains(err.Error(), "inspection") {
		t.Fatalf("stale inspection error = %v", err)
	}
}

func TestClassifiedFixtureAppliesInSandboxAndRunsPackyOwnedValidation(t *testing.T) {
	fixture := newClassificationFixture(t)
	model := &fixtureModel{}
	service, err := NewAIService(model, 1)
	if err != nil {
		t.Fatal(err)
	}
	set, err := service.Classify(context.Background(), fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	var validated []string
	engine := packsync.Engine{Source: fixture.source, Validate: packsync.BundleValidatorFunc(func(_ context.Context, _, bundle string) error {
		for _, pack := range []string{"alpha", "beta"} {
			data, err := os.ReadFile(filepath.Join(bundle, "packs", pack, "pack.json"))
			if err != nil {
				return err
			}
			validated = append(validated, pack+":"+string(data))
		}
		return nil
	})}
	result, err := engine.Apply(context.Background(), packsync.ApplyRequest{CheckRequest: packsync.CheckRequest{RepositoryRoot: fixture.repository, SourceID: "fixture", AcquisitionDir: t.TempDir()}, Plan: fixture.plan, ClassificationEvidence: set})
	if err != nil || result.Status != "applied" {
		t.Fatalf("Apply = %#v, %v", result, err)
	}
	if len(validated) != 4 {
		t.Fatalf("Packy-owned validation calls = %d, want current and staged bundle for two packs", len(validated))
	}
	for _, pack := range []string{"alpha", "beta"} {
		data, err := os.ReadFile(filepath.Join(fixture.repository, "bundle", "packs", pack, "pack.json"))
		if err != nil || !strings.Contains(string(data), `"version": "1.0.1"`) {
			t.Fatalf("%s manifest = %s, %v", pack, data, err)
		}
	}
	repeated := fixture.check(t)
	if repeated.Status != "no-op" || len(repeated.AffectedPacks) != 0 || len(repeated.Changes) != 0 {
		t.Fatalf("post-Apply Check = %#v", repeated)
	}
	retry, err := engine.Apply(context.Background(), packsync.ApplyRequest{CheckRequest: packsync.CheckRequest{RepositoryRoot: fixture.repository, SourceID: "fixture", AcquisitionDir: t.TempDir()}, Plan: fixture.plan, ClassificationEvidence: set})
	if err != nil || retry.Status != "no-op" || retry.Changed {
		t.Fatalf("idempotent Apply = %#v, %v", retry, err)
	}
}

type classificationFixture struct {
	repository string
	source     *fixtureSource
	plan       packsync.Plan
}

func newClassificationFixture(t *testing.T) classificationFixture {
	t.Helper()
	repository := t.TempDir()
	write(t, filepath.Join(repository, "bundle", "sources.json"), sourceConfig(strings.Repeat("a", 40)))
	for _, pack := range []string{"alpha", "beta"} {
		write(t, filepath.Join(repository, "bundle", "packs", pack, "pack.json"), `{"schema_version":1,"id":"`+pack+`","version":"1.0.0","resources":[{"kind":"skill","id":"main","source":"skills/`+pack+`"}]}`)
		write(t, filepath.Join(repository, "bundle", "skills", pack, "SKILL.md"), "old "+pack+"\n")
	}
	initializeGit(t, repository)
	oldSnapshot := t.TempDir()
	for _, pack := range []string{"alpha", "beta"} {
		write(t, filepath.Join(oldSnapshot, "skills", pack, "SKILL.md"), "old "+pack+"\n")
	}
	old := fixtureCandidate(strings.Repeat("a", 40), strings.Repeat("1", 40))
	bootstrapSource := &fixtureSource{snapshots: map[string]string{old.Commit: oldSnapshot}, candidates: map[string]packsync.Candidate{old.Commit: old}}
	bootstrap := check(t, repository, bootstrapSource)
	engine := packsync.Engine{Source: bootstrapSource, Validate: packsync.BundleValidatorFunc(func(context.Context, string, string) error { return nil })}
	if _, err := engine.Apply(context.Background(), packsync.ApplyRequest{CheckRequest: packsync.CheckRequest{RepositoryRoot: repository, SourceID: "fixture", AcquisitionDir: t.TempDir()}, Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}

	newCommit := strings.Repeat("b", 40)
	write(t, filepath.Join(repository, "bundle", "sources.json"), sourceConfig(newCommit))
	newSnapshot := t.TempDir()
	for _, pack := range []string{"alpha", "beta"} {
		write(t, filepath.Join(newSnapshot, "skills", pack, "SKILL.md"), "new "+pack+"\n")
	}
	next := fixtureCandidate(newCommit, strings.Repeat("2", 40))
	source := &fixtureSource{snapshots: map[string]string{old.Commit: oldSnapshot, next.Commit: newSnapshot}, candidates: map[string]packsync.Candidate{old.Commit: old, next.Commit: next}}
	plan := check(t, repository, source)
	if plan.Status != "review-required" || len(plan.AffectedPacks) != 2 {
		t.Fatalf("classification fixture plan = %#v", plan)
	}
	return classificationFixture{repository: repository, source: source, plan: plan}
}

func (fixture classificationFixture) check(t *testing.T) packsync.Plan {
	t.Helper()
	return check(t, fixture.repository, fixture.source)
}

type fixtureSource struct {
	snapshots  map[string]string
	candidates map[string]packsync.Candidate
}

func (source *fixtureSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return nil, nil
}

func (source *fixtureSource) ResolveRelease(context.Context, packsync.SourceConfig, packsync.Release) (packsync.Candidate, error) {
	return packsync.Candidate{}, errors.New("fixture uses exact commits")
}

func (source *fixtureSource) ResolveCommit(_ context.Context, _ packsync.SourceConfig, commit string) (packsync.Candidate, error) {
	candidate, ok := source.candidates[commit]
	if !ok {
		return packsync.Candidate{}, errors.New("unknown fixture commit")
	}
	return candidate, nil
}

func (source *fixtureSource) WithSnapshot(_ context.Context, candidate packsync.Candidate, temporaryRoot string, visit func(string) error) error {
	root := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTree(source.snapshots[candidate.Commit], root); err != nil {
		return err
	}
	err := visit(root)
	cleanup := os.RemoveAll(root)
	if err != nil {
		return err
	}
	return cleanup
}

type fixtureModel struct {
	err      error
	requests []Request
}

func (model *fixtureModel) Attempt(_ context.Context, request Request) (packsync.ClassificationEvidence, error) {
	model.requests = append(model.requests, request)
	if model.err != nil {
		return packsync.ClassificationEvidence{}, model.err
	}
	return evidenceForRequest(request, packsync.ClassifierAI, "fixture-model"), nil
}

func fixtureEvidence(plan packsync.Plan, classifierType packsync.ClassifierType, classifierID string) packsync.ClassificationEvidenceSet {
	mode := ModeAI
	if classifierType == packsync.ClassifierHuman {
		mode = ModeHuman
	}
	requests, _ := requestsForMode(plan, mode)
	set := packsync.ClassificationEvidenceSet{SchemaVersion: 1, PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, Candidate: plan.Candidate}
	for _, request := range requests {
		set.Evidence = append(set.Evidence, evidenceForRequest(request, classifierType, classifierID))
	}
	return set
}

func evidenceForRequest(request Request, classifierType packsync.ClassifierType, classifierID string) packsync.ClassificationEvidence {
	level := packsync.LevelPatch
	if request.MechanicalFloor == packsync.LevelMinor || request.MechanicalFloor == packsync.LevelMajor {
		level = request.MechanicalFloor
	}
	evidence := packsync.ClassificationEvidence{PackID: request.PackID, Classifier: packsync.ClassifierIdentity{Type: classifierType, ID: classifierID}, Rationale: "Fixture behavior remains compatible.", CurrentVersion: request.CurrentVersion, ProposedVersion: "1.0.1", ChangedAspects: []string{"fixture skill behavior"}, MechanicalFloor: request.MechanicalFloor, FinalLevel: level}
	if level == packsync.LevelMajor {
		evidence.ProposedVersion = "2.0.0"
		evidence.Migration = "Use the replacement fixture capability."
		evidence.RequiredActions = []string{"Update fixture consumers."}
	}
	return evidence
}

func check(t *testing.T, repository string, source packsync.Source) packsync.Plan {
	t.Helper()
	plan, err := (packsync.Engine{Source: source}).Check(context.Background(), packsync.CheckRequest{RepositoryRoot: repository, SourceID: "fixture", AcquisitionDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func fixtureCandidate(commit, tree string) packsync.Candidate {
	return packsync.Candidate{Repository: "owner/repo", RepositoryID: 1, RepositoryNodeID: "repo-node", RepositoryHTML: "https://github.com/owner/repo", RepositoryClone: "https://github.com/owner/repo.git", RepositoryAPI: "https://api.github.com/repos/owner/repo", Visibility: "public", Owner: "owner", OwnerID: 2, OwnerNodeID: "owner-node", Public: true, Commit: commit, CommitNodeID: "commit-node-" + commit[:1], Tree: tree, Parents: []string{strings.Repeat("f", 40)}, CommitVerify: packsync.Verification{Reason: "unsigned"}}
}

func sourceConfig(commit string) string {
	return `{"schema_version":1,"sources":[{"id":"fixture","provider":"github","repository":"owner/repo","selector":{"mode":"commit","ref":"` + commit + `"},"resources":[{"pack_id":"alpha","kind":"skill","resource_id":"main","upstream_path":"skills/alpha"},{"pack_id":"beta","kind":"skill","resource_id":"main","upstream_path":"skills/beta"}]}]}`
}

func write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initializeGit(t *testing.T, root string) {
	t.Helper()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, _ := repository.Worktree()
	if _, err := worktree.Add("."); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "Packy test", Email: "test@packy.invalid", When: time.Unix(1, 0).UTC()}}); err != nil {
		t.Fatal(err)
	}
}

func copyTree(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(source, path)
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
