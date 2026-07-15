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
