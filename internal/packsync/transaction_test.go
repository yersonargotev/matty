package packsync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/bundletransaction"
)

func TestInitialApplyBootstrapsTruthfulProvenanceWithoutSelectedContentChange(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy evidence\n")
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	plan := checkWith(t, repository, provider)
	selectedBefore := hashSelectedResources(t, repository, plan.ProposedLock)
	validated := 0
	engine := Engine{Source: provider, Validate: BundleValidatorFunc(func(_ context.Context, _, bundle string) error {
		validated++
		_, err := treeHash(bundle)
		return err
	})}
	request := ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}
	result, err := engine.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "applied" || !result.Changed || validated != 2 {
		t.Fatalf("result=%#v validations=%d", result, validated)
	}
	if got := hashSelectedResources(t, repository, plan.ProposedLock); got != selectedBefore {
		t.Fatalf("selected content changed: %s -> %s", selectedBefore, got)
	}
	if _, err := os.Stat(filepath.Join(repository, "skills-lock.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy evidence still present: %v", err)
	}
	production, _, present, err := readLock(filepath.Join(repository, "bundle", "sources.lock.json"))
	if err != nil || !present || lockDigest(production) != lockDigest(plan.ProposedLock) {
		t.Fatalf("production lock = %#v, present=%t, err=%v", production, present, err)
	}
	repeated := checkWith(t, repository, provider)
	if repeated.Status != "no-op" || !repeated.Authoritative || len(repeated.Changes) != 0 || len(repeated.Blockers) != 0 {
		t.Fatalf("post-Apply Check = %#v", repeated)
	}
	retry, err := engine.Apply(context.Background(), request)
	if err != nil || retry.Status != "no-op" || retry.Changed {
		t.Fatalf("repeated Apply = %#v, %v", retry, err)
	}
}

func TestApplyFaultsAndRecoverDeterministically(t *testing.T) {
	for _, test := range []struct {
		point        FaultPoint
		wantBundle   string
		wantRecovery string
	}{
		{FaultBeforeSwap, "old", ""},
		{FaultAfterFirstRename, "missing", "rolled-back"},
		{FaultAfterSecondRename, "new", "completed"},
		{FaultDuringCleanup, "new", "completed"},
	} {
		t.Run(string(test.point), func(t *testing.T) {
			repository, snapshot := tinyRepository(t)
			writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
			provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
			plan := checkWith(t, repository, provider)
			oldHash, err := treeHash(filepath.Join(repository, "bundle"))
			if err != nil {
				t.Fatal(err)
			}
			engine := Engine{Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(test.point)}
			_, applyErr := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan})
			if applyErr == nil {
				t.Fatal("faulted Apply unexpectedly succeeded")
			}
			bundle := filepath.Join(repository, "bundle")
			switch test.wantBundle {
			case "old":
				if got, err := treeHash(bundle); err != nil || got != oldHash {
					t.Fatalf("pre-swap bundle = %s, %v; want %s", got, err, oldHash)
				}
				assertNoTransactionEvidence(t, repository)
				return
			case "missing":
				if _, err := os.Stat(bundle); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("bundle exists between renames: %v", err)
				}
			case "new":
				if _, _, present, err := readLock(filepath.Join(bundle, "sources.lock.json")); err != nil || !present {
					t.Fatalf("new bundle is not installed: present=%t err=%v", present, err)
				}
			}
			recovered, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository})
			if err != nil || recovered.Status != test.wantRecovery {
				t.Fatalf("Recover = %#v, %v", recovered, err)
			}
			assertNoTransactionEvidence(t, repository)
		})
	}
}

func TestRecoverFailsClosedForMissingManipulatedAndIncompatibleEvidence(t *testing.T) {
	repository := t.TempDir()
	engine := Engine{Validate: acceptingBundleValidator()}
	if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); !errors.Is(err, ErrRecoveryEvidence) {
		t.Fatalf("missing marker error = %v", err)
	}
	for _, marker := range []string{
		`{"schema_version":1}`,
		`{"schema_version":1,"plan_id":"bad","phase":"prepared","bundle":"/tmp/outside","backup":"/tmp/outside-backup","staged":"/tmp/outside-stage","old_sha256":"` + strings.Repeat("a", 64) + `","new_sha256":"` + strings.Repeat("b", 64) + `"}`,
	} {
		writeFile(t, recoveryMarkerPath(repository), marker+"\n")
		if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); !errors.Is(err, ErrRecoveryEvidence) {
			t.Fatalf("manipulated marker error = %v", err)
		}
		if err := os.Remove(recoveryMarkerPath(repository)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplyRejectsEverySealedFreshnessBoundary(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *fixtureSource, *Plan, *Engine)
		want   string
	}{
		{name: "plan", want: "sealed plan", mutate: func(_ *testing.T, _ string, _ *fixtureSource, plan *Plan, _ *Engine) {
			plan.Preconditions.ConfigSHA256 = strings.Repeat("f", 64)
		}},
		{name: "base", want: "repository base", mutate: func(t *testing.T, _ string, _ *fixtureSource, plan *Plan, _ *Engine) {
			plan.Preconditions.BaseCommit = strings.Repeat("a", 40)
			resealPlan(t, plan)
		}},
		{name: "candidate", want: "candidate provenance changed", mutate: func(_ *testing.T, _ string, source *fixtureSource, _ *Plan, _ *Engine) {
			source.candidate.RepositoryID++
		}},
		{name: "configuration", want: "source configuration changed", mutate: func(t *testing.T, repository string, _ *fixtureSource, _ *Plan, _ *Engine) {
			name := filepath.Join(repository, "bundle", "sources.json")
			writeFile(t, name, string(mustReadFile(t, name))+"\n")
		}},
		{name: "bundle-history-evidence", want: "bundle, history, or compatibility", mutate: func(t *testing.T, repository string, _ *fixtureSource, _ *Plan, _ *Engine) {
			name := filepath.Join(repository, "bundle", "skills", "engineering", "one", "SKILL.md")
			writeFile(t, name, "drift\n")
		}},
		{name: "production-lock", want: "production provenance lock changed", mutate: func(t *testing.T, repository string, _ *fixtureSource, _ *Plan, _ *Engine) {
			writeFile(t, filepath.Join(repository, "bundle", "sources.lock.json"), "{}\n")
		}},
		{name: "Matty-owned-suite", want: "fresh Matty-owned validation", mutate: func(_ *testing.T, _ string, _ *fixtureSource, _ *Plan, engine *Engine) {
			engine.Validate = BundleValidatorFunc(func(context.Context, string, string) error { return errors.New("suite rejected hostile content") })
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, snapshot := tinyRepository(t)
			writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
			provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
			plan := checkWith(t, repository, provider)
			engine := Engine{Source: provider, Validate: acceptingBundleValidator()}
			test.mutate(t, repository, provider, &plan, &engine)
			_, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Apply error = %v, want %q", err, test.want)
			}
			if _, markerErr := os.Stat(recoveryMarkerPath(repository)); !errors.Is(markerErr, os.ErrNotExist) {
				t.Fatalf("stale Apply published recovery state: %v", markerErr)
			}
		})
	}
}

func TestRecoverRetainsEvidenceForIncompleteBackupAndAmbiguousSiblings(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string, recoveryMarker)
	}{
		{name: "incomplete-backup", mutate: func(t *testing.T, _ string, marker recoveryMarker) {
			name := filepath.Join(marker.Backup, "skills", "engineering", "one", "SKILL.md")
			writeFile(t, name, "tampered\n")
		}},
		{name: "incomplete-staging", mutate: func(t *testing.T, _ string, marker recoveryMarker) {
			name := filepath.Join(marker.Staged, "skills", "engineering", "one", "SKILL.md")
			writeFile(t, name, "tampered\n")
		}},
		{name: "ambiguous-sibling", mutate: func(t *testing.T, repository string, _ recoveryMarker) {
			if err := os.Mkdir(filepath.Join(repository, ".matty-bundle-unexpected.backup"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, snapshot := tinyRepository(t)
			writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
			provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
			plan := checkWith(t, repository, provider)
			engine := Engine{Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(FaultAfterFirstRename)}
			if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil {
				t.Fatal("faulted Apply unexpectedly succeeded")
			}
			marker, err := readRecoveryMarker(recoveryMarkerPath(repository))
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, repository, marker)
			if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); !errors.Is(err, ErrRecoveryEvidence) {
				t.Fatalf("Recover error = %v", err)
			}
			if _, err := os.Stat(recoveryMarkerPath(repository)); err != nil {
				t.Fatalf("recovery marker was not retained: %v", err)
			}
		})
	}
}

func TestApplyAndRecoverHoldSharedLockForEveryMutationAndRepairPhase(t *testing.T) {
	for _, point := range []FaultPoint{FaultBeforeSwap, FaultAfterFirstRename, FaultAfterSecondRename, FaultDuringCleanup} {
		t.Run(string(point), func(t *testing.T) {
			repository, snapshot := tinyRepository(t)
			writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
			provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
			plan := checkWith(t, repository, provider)
			engine := Engine{Source: provider, Validate: acceptingBundleValidator(), Fault: func(observed FaultPoint) error {
				if observed != point {
					return nil
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()
				if guard, err := bundletransaction.Acquire(ctx, repository); err == nil {
					guard.Release()
					return errors.New("mutation phase did not hold the shared lock")
				}
				return errors.New("injected while locked")
			}}
			if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil || !strings.Contains(err.Error(), "injected while locked") {
				t.Fatalf("Apply error = %v", err)
			}
			if point == FaultBeforeSwap {
				return
			}
			engine.Fault = nil
			engine.Validate = BundleValidatorFunc(func(ctx context.Context, _, _ string) error {
				wait, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
				if guard, err := bundletransaction.Acquire(wait, repository); err == nil {
					guard.Release()
					return errors.New("Recover validation did not hold the shared lock")
				}
				return nil
			})
			if _, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoverFinishesCleanupIdempotentlyAfterEffectsCompleted(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	plan := checkWith(t, repository, provider)
	engine := Engine{Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(FaultDuringCleanup)}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil {
		t.Fatal("cleanup fault unexpectedly succeeded")
	}
	marker, err := readRecoveryMarker(recoveryMarkerPath(repository))
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanupCommitted(marker); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository})
	if err != nil || result.Status != "completed" {
		t.Fatalf("Recover = %#v, %v", result, err)
	}
	assertNoTransactionEvidence(t, repository)
}

func TestStagedSuiteFailureLeavesRepositoryUntouched(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	plan := checkWith(t, repository, provider)
	before, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	validations := 0
	engine := Engine{Source: provider, Validate: BundleValidatorFunc(func(context.Context, string, string) error {
		validations++
		if validations == 2 {
			return errors.New("staged suite failed")
		}
		return nil
	})}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil || !strings.Contains(err.Error(), "staged suite failed") {
		t.Fatalf("Apply error = %v", err)
	}
	after, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil || before != after {
		t.Fatalf("pre-swap failure changed bundle: %s -> %s, %v", before, after, err)
	}
	if _, err := os.Stat(filepath.Join(repository, "skills-lock.json")); err != nil {
		t.Fatalf("pre-swap failure removed legacy evidence: %v", err)
	}
	assertNoTransactionEvidence(t, repository)
}

func TestApplyMaterializesCompleteAuthoritativeCandidateBundle(t *testing.T) {
	repository, oldSnapshot := tinyRepository(t)
	initializeFixtureGit(t, repository)
	oldCandidate := acceptedCandidate()
	bootstrapSource := &fixtureSource{root: oldSnapshot, candidate: oldCandidate}
	bootstrap := checkWith(t, repository, bootstrapSource)
	engine := Engine{Source: bootstrapSource, Validate: acceptingBundleValidator()}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}

	newSnapshot := t.TempDir()
	writeFile(t, filepath.Join(newSnapshot, "skills", "engineering", "one", "SKILL.md"), "updated\n")
	newCandidate := advancedCandidate(oldCandidate)
	source := &multiReleaseSource{root: newSnapshot, candidates: map[string]Candidate{oldCandidate.Release.Tag: oldCandidate, newCandidate.Release.Tag: newCandidate}}
	plan := checkWith(t, repository, source)
	if plan.Status != "review-required" || !plan.Authoritative || plan.Counts.Modified != 1 {
		t.Fatalf("authoritative update plan = %#v", plan)
	}
	engine.Source = source
	evidence := classificationEvidenceForPlan(t, plan, ClassifierAI, "fixture-model", LevelPatch)
	result, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan, ClassificationEvidence: evidence})
	if err != nil || result.Status != "applied" {
		t.Fatalf("Apply = %#v, %v", result, err)
	}
	updated := mustReadFile(t, filepath.Join(repository, "bundle", "skills", "engineering", "one", "SKILL.md"))
	if string(updated) != "updated\n" {
		t.Fatalf("selected candidate was not materialized: %q", updated)
	}
	manifest := mustReadFile(t, filepath.Join(repository, "bundle", "packs", "matty", "pack.json"))
	if !strings.Contains(string(manifest), `"version": "1.0.1"`) {
		t.Fatalf("classified exact version was not materialized: %s", manifest)
	}
	lock, _, present, err := readLock(filepath.Join(repository, "bundle", "sources.lock.json"))
	if err != nil || !present || lock.Candidate.Commit != newCandidate.Commit {
		t.Fatalf("updated lock = %#v, present=%t, err=%v", lock, present, err)
	}
	repeated := checkWith(t, repository, source)
	if repeated.Status != "no-op" || len(repeated.Changes) != 0 || len(repeated.Blockers) != 0 {
		t.Fatalf("repeated Check = %#v", repeated)
	}
	retryRequest := ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan, ClassificationEvidence: evidence}
	if retry, err := engine.Apply(context.Background(), retryRequest); err != nil || retry.Status != "no-op" || retry.Changed {
		t.Fatalf("idempotent classified Apply = %#v, %v", retry, err)
	}
	writeFile(t, filepath.Join(repository, "bundle", "packs", "matty", "pack.json"), strings.Replace(string(manifest), `"version": "1.0.1"`, `"version": "9.9.9"`, 1))
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan, ClassificationEvidence: evidence}); err == nil || !strings.Contains(err.Error(), "classified pack versions") {
		t.Fatalf("tampered classified version error = %v", err)
	}
}

func TestApplyRejectsAffectedPlanWithoutCompleteClassificationEvidence(t *testing.T) {
	repository, oldSnapshot := tinyRepository(t)
	initializeFixtureGit(t, repository)
	bootstrapSource := &fixtureSource{root: oldSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, bootstrapSource)
	engine := Engine{Source: bootstrapSource, Validate: acceptingBundleValidator()}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}

	newSnapshot := t.TempDir()
	writeFile(t, filepath.Join(newSnapshot, "skills", "engineering", "one", "SKILL.md"), "updated\n")
	newCandidate := advancedCandidate(acceptedCandidate())
	source := &multiReleaseSource{root: newSnapshot, candidates: map[string]Candidate{acceptedCandidate().Release.Tag: acceptedCandidate(), newCandidate.Release.Tag: newCandidate}}
	plan := checkWith(t, repository, source)
	before, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	engine.Source = source
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil || !strings.Contains(err.Error(), "complete evidence coverage") {
		t.Fatalf("Apply error = %v", err)
	}
	after, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil || after != before {
		t.Fatalf("rejected classification changed bundle: %s -> %s, %v", before, after, err)
	}
}

func TestApplyRemovesObsoleteDestinationWhenManifestMovesBinding(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, provider)
	engine := Engine{Source: provider, Validate: acceptingBundleValidator()}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}
	oldDestination := filepath.Join(repository, "bundle", "skills", "engineering", "one")
	newDestination := filepath.Join(repository, "bundle", "skills", "engineering", "moved")
	copyTree(t, oldDestination, newDestination)
	manifest := filepath.Join(repository, "bundle", "packs", "matty", "pack.json")
	writeFile(t, manifest, strings.Replace(string(mustReadFile(t, manifest)), "skills/engineering/one", "skills/engineering/moved", 1))

	plan := checkWith(t, repository, provider)
	if plan.Status != "review-required" || !plan.Authoritative || plan.Counts.Moved != 1 {
		t.Fatalf("destination move plan = %#v", plan)
	}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDestination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete selected destination remains: %v", err)
	}
	if got := string(mustReadFile(t, filepath.Join(newDestination, "SKILL.md"))); got != "same\n" {
		t.Fatalf("new selected destination = %q", got)
	}
	repeated := checkWith(t, repository, provider)
	if repeated.Status != "no-op" || len(repeated.Changes) != 0 || len(repeated.Blockers) != 0 {
		t.Fatalf("repeated Check = %#v", repeated)
	}
}

func TestRecoverResumesAfterItsOwnRollbackAndCleanupEffects(t *testing.T) {
	t.Run("rollback rename already completed", func(t *testing.T) {
		repository, snapshot := tinyRepository(t)
		writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
		provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
		plan := checkWith(t, repository, provider)
		engine := Engine{Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(FaultAfterFirstRename)}
		if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil {
			t.Fatal("faulted Apply unexpectedly succeeded")
		}
		marker, err := readRecoveryMarker(recoveryMarkerPath(repository))
		if err != nil {
			t.Fatal(err)
		}
		marker.Phase = "rolling-back"
		if err := writeRecoveryMarker(recoveryMarkerPath(repository), &marker); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(marker.Backup, marker.Bundle); err != nil {
			t.Fatal(err)
		}
		result, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository})
		if err != nil || result.Status != "rolled-back" {
			t.Fatalf("Recover = %#v, %v", result, err)
		}
		assertNoTransactionEvidence(t, repository)
	})

	t.Run("new bundle cleanup already completed", func(t *testing.T) {
		repository, snapshot := tinyRepository(t)
		writeFile(t, filepath.Join(repository, "skills-lock.json"), "legacy\n")
		provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
		plan := checkWith(t, repository, provider)
		engine := Engine{Source: provider, Validate: acceptingBundleValidator(), Fault: failOnce(FaultAfterSecondRename)}
		if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: plan}); err == nil {
			t.Fatal("faulted Apply unexpectedly succeeded")
		}
		marker, err := readRecoveryMarker(recoveryMarkerPath(repository))
		if err != nil {
			t.Fatal(err)
		}
		if err := cleanupCommitted(marker); err != nil {
			t.Fatal(err)
		}
		result, err := engine.Recover(context.Background(), RecoverRequest{RepositoryRoot: repository})
		if err != nil || result.Status != "completed" {
			t.Fatalf("Recover = %#v, %v", result, err)
		}
		assertNoTransactionEvidence(t, repository)
	})
}

func acceptingBundleValidator() BundleValidator {
	return BundleValidatorFunc(func(context.Context, string, string) error { return nil })
}

func resealPlan(t *testing.T, plan *Plan) {
	t.Helper()
	plan.PlanID = ""
	id, err := seal(*plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanID = id
}

func advancedCandidate(previous Candidate) Candidate {
	next := previous
	release := *previous.Release
	release.ID++
	release.NodeID = "new-release-node"
	release.Tag = "v1.2.0"
	release.Name = "v1.2.0"
	release.CreatedAt = release.CreatedAt.Add(time.Hour)
	release.PublishedAt = release.PublishedAt.Add(time.Hour)
	next.Release = &release
	next.TagRefName = "refs/tags/" + release.Tag
	next.TagRefSHA = strings.Repeat("c", 40)
	next.TagObjects = append([]TagObject(nil), previous.TagObjects...)
	next.TagObjects[0].SHA = next.TagRefSHA
	next.TagObjects[0].Name = release.Tag
	next.Commit = strings.Repeat("d", 40)
	next.CommitNodeID = "new-commit-node"
	next.Tree = strings.Repeat("e", 40)
	next.Parents = []string{previous.Commit}
	next.TagObjects[0].TargetSHA = next.Commit
	return next
}

func failOnce(point FaultPoint) FaultInjector {
	fired := false
	return func(observed FaultPoint) error {
		if observed == point && !fired {
			fired = true
			return errors.New("injected " + string(point))
		}
		return nil
	}
}

func newCheckRequest(t *testing.T, repository string) CheckRequest {
	t.Helper()
	return CheckRequest{RepositoryRoot: repository, AcquisitionDir: t.TempDir()}
}

func hashSelectedResources(t *testing.T, repository string, lock Lock) string {
	t.Helper()
	var evidence []FileEvidence
	for _, resource := range lock.Resources {
		files, err := inventory(filepath.Join(repository, filepath.FromSlash(resource.VendoredPath)))
		if err != nil {
			t.Fatal(err)
		}
		evidence = append(evidence, FileEvidence{Path: bindingKey(resource.Binding), Size: int64(len(files)), Mode: 0o600, SHA256: resourceHash(files)})
	}
	return resourceHash(evidence)
}

func assertNoTransactionEvidence(t *testing.T, repository string) {
	t.Helper()
	if _, err := os.Stat(recoveryMarkerPath(repository)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery marker remains: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(repository, ".matty-bundle-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("transaction siblings remain: %v, %v", matches, err)
	}
}
