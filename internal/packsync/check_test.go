package packsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

var acceptedDiscoveries = []string{
	"skills/deprecated/design-an-interface",
	"skills/deprecated/qa",
	"skills/deprecated/request-refactor-plan",
	"skills/deprecated/ubiquitous-language",
	"skills/in-progress/claude-handoff",
	"skills/in-progress/wizard",
	"skills/in-progress/writing-beats",
	"skills/in-progress/writing-fragments",
	"skills/in-progress/writing-shape",
	"skills/misc/git-guardrails-claude-code",
	"skills/misc/migrate-to-shoehorn",
	"skills/misc/scaffold-exercises",
	"skills/misc/setup-pre-commit",
	"skills/personal/edit-article",
	"skills/personal/obsidian-vault",
}

func TestCheckAcquiresCandidateOutsideLockThenLocksLocalObservation(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	provider := &gatedSnapshotSource{fixtureSource: fixtureSource{root: snapshot, candidate: acceptedCandidate()}, acquired: make(chan struct{}), proceed: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		acquisition, err := os.MkdirTemp("", "check-lock-test-")
		if err != nil {
			done <- err
			return
		}
		defer os.RemoveAll(acquisition)
		_, err = (Engine{allowBootstrap: true, Source: provider}).Check(context.Background(), CheckRequest{RepositoryRoot: repository, AcquisitionDir: acquisition})
		done <- err
	}()
	<-provider.acquired
	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatalf("candidate acquisition held repository lock: %v", err)
	}
	close(provider.proceed)
	select {
	case err := <-done:
		t.Fatalf("local observation completed outside the repository lock: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCheckSealsAbsentSourceRegistrationWithoutPersistingIt(t *testing.T) {
	repository, existingSnapshot := tinyRepository(t)
	bootstrapSource := &fixtureSource{root: existingSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, bootstrapSource)
	if _, err := (Engine{allowBootstrap: true, Source: bootstrapSource, Validate: acceptingBundleValidator()}).Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}
	legacyPackID := "ma" + "tty"
	writeFile(t, filepath.Join(repository, "bundle", "packs", legacyPackID, "pack.json"), fmt.Sprintf(`{"schema_version":1,"id":%q,"version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/engineering/one"},{"kind":"skill","id":"two","source":"skills/engineering/two"}]}`, legacyPackID))
	snapshot := t.TempDir()
	writeFile(t, filepath.Join(snapshot, "skills", "engineering", "two", "SKILL.md"), "new\n")
	registration := SourceConfig{ID: "addy", Provider: "github", Repository: "addyosmani/agent-skills", Selector: Selector{Mode: SelectorStableRelease}, Resources: []Binding{{PackID: legacyPackID, Kind: "skill", ResourceID: "two", UpstreamPath: "skills/engineering/two"}}}
	before := mustReadFile(t, filepath.Join(repository, "bundle", "sources.json"))
	plan, err := (Engine{Source: &fixtureSource{root: snapshot, candidate: acceptedCandidateFor("addyosmani/agent-skills")}}).Check(context.Background(), CheckRequest{RepositoryRoot: repository, SourceID: "addy", Registration: &registration, AcquisitionDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Authoritative || plan.Status != "review-required" || plan.Registration == nil || plan.RegistrationSHA256 == "" || !plan.VerifySeal() {
		t.Fatalf("registration plan = %#v", plan)
	}
	if len(plan.AffectedPacks) != 1 || plan.AffectedPacks[0].CurrentVersion != "1.0.0" || plan.AffectedPacks[0].MechanicalFloor != LevelNone {
		t.Fatalf("registration into an existing Pack classification = %#v", plan.AffectedPacks)
	}
	if got := mustReadFile(t, filepath.Join(repository, "bundle", "sources.json")); !reflect.DeepEqual(got, before) {
		t.Fatal("Check persisted proposed registration")
	}
}

func TestCheckRejectsRegistrationWithExistingSourceOrBindingOwner(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	bootstrapSource := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, bootstrapSource)
	if _, err := (Engine{allowBootstrap: true, Source: bootstrapSource, Validate: acceptingBundleValidator()}).Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}
	base := SourceConfig{ID: "addy", Provider: "github", Repository: "addyosmani/agent-skills", Selector: Selector{Mode: SelectorStableRelease}, Resources: []Binding{}}
	for _, test := range []struct {
		name         string
		registration SourceConfig
	}{
		{name: "source", registration: SourceConfig{ID: "mattpocock-skills", Provider: "github", Repository: "mattpocock/skills", Selector: Selector{Mode: SelectorStableRelease}, Resources: []Binding{}}},
		{name: "binding", registration: func() SourceConfig {
			value := base
			value.Resources = []Binding{{PackID: "ma" + "tty", Kind: "skill", ResourceID: "one", UpstreamPath: "other"}}
			return value
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Engine{Source: &fixtureSource{root: snapshot, candidate: acceptedCandidate()}}).Check(context.Background(), CheckRequest{RepositoryRoot: repository, SourceID: test.registration.ID, Registration: &test.registration, AcquisitionDir: t.TempDir()})
			if err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
}

func TestCheckValidatesAcceptedMattyMajorMigrationWithoutWritingRepository(t *testing.T) {
	sourceRepository := repositoryRoot(t)
	snapshot := realSnapshot(t, sourceRepository, true)
	repository := bootstrapRepository(t, sourceRepository)
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	before, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	plan := checkWith(t, repository, provider)
	after, err := treeHash(filepath.Join(repository, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("Check modified repository bundle state")
	}
	wantCounts := Counts{Resources: 23, Files: 45, Discoveries: 15}
	if !reflect.DeepEqual(plan.Counts, wantCounts) {
		t.Fatalf("counts = %#v, want %#v", plan.Counts, wantCounts)
	}
	if plan.Status != "blocked" || plan.Authoritative || !plan.VerifySeal() {
		t.Fatalf("plan status=%s authoritative=%t sealed=%t", plan.Status, plan.Authoritative, plan.VerifySeal())
	}
	if !reflect.DeepEqual(plan.Discoveries, acceptedDiscoveries) {
		t.Fatalf("discoveries = %#v", plan.Discoveries)
	}
	if len(plan.Blockers) != 1 || !strings.Contains(plan.Blockers[0], "production provenance lock is absent") {
		t.Fatalf("migration blockers = %#v", plan.Blockers)
	}
	if !plan.LegacyEvidence {
		t.Fatal("legacy root lock should be reported only as evidence")
	}
	jsonPlan, err := plan.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonPlan), plan.PlanID) || !strings.Contains(plan.Human(), plan.PlanID) {
		t.Fatal("canonical JSON and human plan must carry the same plan_id")
	}
	repeated := checkWith(t, repository, provider)
	if repeated.PlanID != plan.PlanID || repeated.Human() != plan.Human() {
		t.Fatal("identical inputs did not produce an identical canonical plan")
	}
}

func TestCheckSealsOneClassificationImpactPerAffectedPack(t *testing.T) {
	repository, oldSnapshot := tinyRepository(t)
	bootstrapSource := &fixtureSource{root: oldSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, bootstrapSource)
	engine := Engine{allowBootstrap: true, Source: bootstrapSource, Validate: acceptingBundleValidator()}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: newCheckRequest(t, repository), Plan: bootstrap}); err != nil {
		t.Fatal(err)
	}

	newSnapshot := t.TempDir()
	writeFile(t, filepath.Join(newSnapshot, "skills", "engineering", "one", "SKILL.md"), "updated\n")
	newCandidate := advancedCandidate(acceptedCandidate())
	source := &multiReleaseSource{root: newSnapshot, candidates: map[string]Candidate{
		acceptedCandidate().Release.Tag: acceptedCandidate(),
		newCandidate.Release.Tag:        newCandidate,
	}}
	plan := checkWith(t, repository, source)
	want := []PackImpact{{
		PackID:                   "matty",
		CurrentVersion:           "1.0.0",
		MechanicalFloor:          LevelNone,
		SemanticEvidenceRequired: true,
		Reasons:                  []string{"upstream-owned content changed"},
	}}
	if !reflect.DeepEqual(plan.AffectedPacks, want) {
		t.Fatalf("affected packs = %#v, want %#v", plan.AffectedPacks, want)
	}
	if !plan.VerifySeal() {
		t.Fatal("classification impacts are not part of the canonical plan seal")
	}
}

func TestCheckFailsClosedWhenMajorMigrationEvidenceIsMissing(t *testing.T) {
	repository := repositoryRoot(t)
	snapshot := realSnapshot(t, repository, true)
	copyRoot := t.TempDir()
	copyTree(t, filepath.Join(repository, "bundle"), filepath.Join(copyRoot, "bundle"))
	writeFile(t, filepath.Join(copyRoot, "skills-lock.json"), "{}\n")
	removeProductionLock(t, copyRoot)
	if err := os.Remove(filepath.Join(copyRoot, "bundle", "compatibility", "matty", "1.0.0-to-2.0.0.json")); err != nil {
		t.Fatal(err)
	}

	plan := checkWith(t, copyRoot, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
	assertBlocker(t, plan, "compatibility evidence is missing")
}

func TestCheckFailsClosedWhenMajorMigrationEvidenceIsIncomplete(t *testing.T) {
	repository := repositoryRoot(t)
	snapshot := realSnapshot(t, repository, true)
	copyRoot := t.TempDir()
	copyTree(t, filepath.Join(repository, "bundle"), filepath.Join(copyRoot, "bundle"))
	writeFile(t, filepath.Join(copyRoot, "skills-lock.json"), "{}\n")
	removeProductionLock(t, copyRoot)
	mutateCompatibilityEvidence(t, copyRoot, func(evidence map[string]any) {
		migration := evidence["migration"].(map[string]any)
		files := migration["divergent_files"].([]any)
		migration["divergent_files"] = files[:len(files)-1]
	})

	plan := checkWith(t, copyRoot, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
	assertBlocker(t, plan, "divergent-file mapping")
}

func TestCheckFailsClosedWhenAcceptedMigrationHistoryIsMissing(t *testing.T) {
	repository := repositoryRoot(t)
	snapshot := realSnapshot(t, repository, true)
	copyRoot := t.TempDir()
	copyTree(t, filepath.Join(repository, "bundle"), filepath.Join(copyRoot, "bundle"))
	writeFile(t, filepath.Join(copyRoot, "skills-lock.json"), "{}\n")
	removeProductionLock(t, copyRoot)
	if err := os.RemoveAll(filepath.Join(copyRoot, "bundle", "history", "matty", "1.0.0")); err != nil {
		t.Fatal(err)
	}

	plan := checkWith(t, copyRoot, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
	assertBlocker(t, plan, "accepted compatibility history is missing")
}

func TestCheckRejectsCoordinatedReplacementEvidenceAndInstructionDrift(t *testing.T) {
	repository := repositoryRoot(t)
	snapshot := realSnapshot(t, repository, true)
	copyRoot := t.TempDir()
	copyTree(t, filepath.Join(repository, "bundle"), filepath.Join(copyRoot, "bundle"))
	writeFile(t, filepath.Join(copyRoot, "skills-lock.json"), "{}\n")
	removeProductionLock(t, copyRoot)
	mutateCompatibilityEvidence(t, copyRoot, func(evidence map[string]any) {
		rule := evidence["migration"].(map[string]any)["replacement_rules"].([]any)[0].(map[string]any)
		rule["semantics"] = "Use arbitrary replacement vocabulary."
	})
	instruction := filepath.Join(copyRoot, "bundle", "instructions", "matty-workflow-conventions.md")
	writeFile(t, instruction, strings.Replace(string(mustReadFile(t, instruction)), "Use **Specs and tickets** as the workflow vocabulary.", "Use arbitrary replacement vocabulary.", 1))

	plan := checkWith(t, copyRoot, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
	assertBlocker(t, plan, "trusted digest")
}

func TestCheckFailsClosedWhenMajorMigrationEvidenceOrReplacementSemanticsDrift(t *testing.T) {
	repository := repositoryRoot(t)
	snapshot := realSnapshot(t, repository, true)
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{name: "classification", want: "human major classification", mutate: func(t *testing.T, root string) {
			mutateCompatibilityEvidence(t, root, func(evidence map[string]any) { evidence["classification"].(map[string]any)["level"] = "minor" })
		}},
		{name: "replacement semantics", want: "replacement semantics", mutate: func(t *testing.T, root string) {
			mutateCompatibilityEvidence(t, root, func(evidence map[string]any) {
				evidence["migration"].(map[string]any)["replacement_rules"].([]any)[0].(map[string]any)["semantics"] = ""
			})
		}},
		{name: "instruction bytes", want: "replacement semantics", mutate: func(t *testing.T, root string) {
			name := filepath.Join(root, "bundle", "instructions", "matty-workflow-conventions.md")
			writeFile(t, name, strings.Replace(string(mustReadFile(t, name)), "Specs and tickets", "PRDs and issues", 2))
		}},
		{name: "historical artifact hash", want: "historical-artifact hash", mutate: func(t *testing.T, root string) {
			name := filepath.Join(root, "bundle", "history", "matty", "1.0.0", "artifact.json")
			writeFile(t, name, string(mustReadFile(t, name))+" ")
		}},
		{name: "selection evidence", want: "selection evidence", mutate: func(t *testing.T, root string) {
			mutateCompatibilityEvidence(t, root, func(evidence map[string]any) {
				selection := evidence["selection"].(map[string]any)
				bindings := selection["bindings"].([]any)
				selection["bindings"] = bindings[:len(bindings)-1]
			})
		}},
		{name: "upstream provenance claim", want: "must not claim upstream provenance", mutate: func(t *testing.T, root string) {
			mutateCompatibilityEvidence(t, root, func(evidence map[string]any) { evidence["claims_upstream_provenance"] = true })
		}},
		{name: "source lock replacement", want: "replace the source lock", mutate: func(t *testing.T, root string) {
			mutateCompatibilityEvidence(t, root, func(evidence map[string]any) { evidence["replaces_source_lock"] = true })
		}},
		{name: "configured selection", want: "selection evidence", mutate: func(t *testing.T, root string) {
			name := filepath.Join(root, "bundle", "sources.json")
			var config map[string]any
			if err := json.Unmarshal(mustReadFile(t, name), &config); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, rawSource := range config["sources"].([]any) {
				source := rawSource.(map[string]any)
				if source["id"] != "mattpocock-skills" {
					continue
				}
				resources := source["resources"].([]any)
				if len(resources) == 0 {
					t.Fatal("target source has no resources")
				}
				source["resources"] = resources[:len(resources)-1]
				found = true
				break
			}
			if !found {
				t.Fatal("target source is missing")
			}
			writeJSON(t, name, config)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyRoot := t.TempDir()
			copyTree(t, filepath.Join(repository, "bundle"), filepath.Join(copyRoot, "bundle"))
			writeFile(t, filepath.Join(copyRoot, "skills-lock.json"), "{}\n")
			removeProductionLock(t, copyRoot)
			test.mutate(t, copyRoot)
			plan := checkWith(t, copyRoot, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
			assertBlocker(t, plan, test.want)
		})
	}
}

func TestCheckFailsClosedWhenRestoredSelectedByteDrifts(t *testing.T) {
	repository := repositoryRoot(t)
	snapshot := realSnapshot(t, repository, true)
	copyRoot := t.TempDir()
	copyTree(t, filepath.Join(repository, "bundle"), filepath.Join(copyRoot, "bundle"))
	writeFile(t, filepath.Join(copyRoot, "skills-lock.json"), "{}\n")
	removeProductionLock(t, copyRoot)
	name := filepath.Join(copyRoot, "bundle", "skills", "engineering", "wayfinder", "SKILL.md")
	writeFile(t, name, string(mustReadFile(t, name))+"drift\n")

	plan := checkWith(t, copyRoot, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
	assertBlocker(t, plan, "bootstrap selected bytes differ from the exact candidate")
}

func TestByteIdenticalBootstrapIsSealedAndOneByteInvalidatesIt(t *testing.T) {
	sourceRepository := repositoryRoot(t)
	repository := bootstrapRepository(t, sourceRepository)
	identicalRoot := realSnapshot(t, sourceRepository, false)
	identical := checkWith(t, repository, &fixtureSource{root: identicalRoot, candidate: acceptedCandidate()})
	if identical.Counts.Modified != 0 || !identical.VerifySeal() || identical.PlanID == "" {
		t.Fatalf("byte-identical bootstrap plan = %#v", identical)
	}
	changedRoot := t.TempDir()
	copyTree(t, identicalRoot, changedRoot)
	name := filepath.Join(changedRoot, "skills", "engineering", "ask-matt", "SKILL.md")
	file, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	changed := checkWith(t, repository, &fixtureSource{root: changedRoot, candidate: acceptedCandidate()})
	if changed.Counts.Modified != 1 || changed.PlanID == identical.PlanID || !changed.VerifySeal() {
		t.Fatalf("one-byte invalidation modified=%d same-id=%t sealed=%t", changed.Counts.Modified, changed.PlanID == identical.PlanID, changed.VerifySeal())
	}
}

func TestPlanIDIsIndependentOfAbsoluteCheckoutPath(t *testing.T) {
	firstRoot, snapshot := tinyRepository(t)
	secondRoot := t.TempDir()
	copyTree(t, firstRoot, secondRoot)
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	first := checkWith(t, firstRoot, provider)
	second := checkWith(t, secondRoot, provider)
	if first.PlanID != second.PlanID {
		t.Fatalf("byte-identical checkouts produced %s and %s", first.PlanID, second.PlanID)
	}
}

func TestCheckFailsClosedForMovedIdentityTagMovementLossAndDrift(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, provider)
	writeJSON(t, filepath.Join(repository, "bundle", "sources/mattpocock-skills.lock.json"), bootstrap.ProposedLock)
	authoritative := checkWith(t, repository, provider)
	if authoritative.Status != "no-op" || !authoritative.Authoritative || len(authoritative.Blockers) != 0 {
		t.Fatalf("authoritative baseline = %#v", authoritative)
	}

	t.Run("moved identity", func(t *testing.T) {
		moved := *provider
		moved.candidate.RepositoryID++
		plan := checkWith(t, repository, &moved)
		assertBlocker(t, plan, "numeric identity moved")
	})
	t.Run("moved tag", func(t *testing.T) {
		moved := *provider
		moved.candidate.TagRefSHA = strings.Repeat("b", 40)
		plan := checkWith(t, repository, &moved)
		assertBlocker(t, plan, "tag ref moved")
	})
	t.Run("recreated release identity", func(t *testing.T) {
		recreated := *provider
		release := *provider.candidate.Release
		release.ID++
		recreated.candidate.Release = &release
		plan := checkWith(t, repository, &recreated)
		assertBlocker(t, plan, "release identity or publication evidence changed")
	})
	t.Run("selected resource loss", func(t *testing.T) {
		missing := t.TempDir()
		plan := checkWith(t, repository, &fixtureSource{root: missing, candidate: acceptedCandidate()})
		assertBlocker(t, plan, "selected resource missing")
	})
	t.Run("local drift", func(t *testing.T) {
		copyRoot := t.TempDir()
		copyTree(t, repository, copyRoot)
		name := filepath.Join(copyRoot, "bundle", "skills", "engineering", "one", "SKILL.md")
		if err := os.WriteFile(name, []byte("drift\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		plan := checkWith(t, copyRoot, provider)
		assertBlocker(t, plan, "local selected-resource drift")
	})
	t.Run("malformed resolved provenance", func(t *testing.T) {
		malformed := *provider
		malformed.candidate.TagObjects = append([]TagObject(nil), provider.candidate.TagObjects...)
		malformed.candidate.Commit = strings.Repeat("z", 40)
		malformed.candidate.Tree = "x"
		malformed.candidate.TagObjects[0].TargetType = "blob"
		plan := checkWith(t, repository, &malformed)
		assertBlocker(t, plan, "complete immutable commit and tree")
		assertBlocker(t, plan, "incomplete or ambiguous")
	})
	t.Run("malformed authoritative lock provenance", func(t *testing.T) {
		copyRoot := t.TempDir()
		copyTree(t, repository, copyRoot)
		lock := bootstrap.ProposedLock
		lock.Candidate.Commit = ""
		lock.Candidate.Tree = ""
		lock.Resources = append(lock.Resources, lock.Resources[0])
		lock.Snapshot = snapshotHash(lock.Resources)
		writeJSON(t, filepath.Join(copyRoot, "bundle", "sources/mattpocock-skills.lock.json"), lock)
		_, err := (Engine{allowBootstrap: true, Source: provider}).Check(context.Background(), newCheckRequest(t, copyRoot))
		if err == nil || !strings.Contains(err.Error(), "duplicate source-lock resource") {
			t.Fatalf("malformed lock error = %v", err)
		}
	})
}

func TestNewReleaseWithIdenticalBytesProducesProvenanceUpdate(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, provider)
	writeJSON(t, filepath.Join(repository, "bundle", "sources/mattpocock-skills.lock.json"), bootstrap.ProposedLock)
	newer := *provider
	release := *provider.candidate.Release
	release.ID++
	release.NodeID = "new-release-node"
	release.Tag = "v1.2.0"
	release.Name = "v1.2.0"
	release.CreatedAt = release.CreatedAt.Add(time.Hour)
	release.PublishedAt = release.PublishedAt.Add(time.Hour)
	newer.candidate.Release = &release
	newer.candidate.TagRefName = "refs/tags/v1.2.0"
	newer.candidate.TagRefSHA = strings.Repeat("c", 40)
	newer.candidate.TagObjects = append([]TagObject(nil), provider.candidate.TagObjects...)
	newer.candidate.TagObjects[0].SHA = strings.Repeat("c", 40)
	newer.candidate.TagObjects[0].Name = "v1.2.0"
	history := &multiReleaseSource{root: snapshot, candidates: map[string]Candidate{provider.candidate.Release.Tag: provider.candidate, newer.candidate.Release.Tag: newer.candidate}}
	plan := checkWith(t, repository, history)
	if plan.Status != "review-required" {
		t.Fatalf("identical-byte new release status = %s, blockers=%#v", plan.Status, plan.Blockers)
	}
	assertChange(t, plan, "provenance-updated")
}

func TestCheckReResolvesMovedLockedTagBeforeSelectingNewerRelease(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	locked := acceptedCandidate()
	bootstrap := checkWith(t, repository, &fixtureSource{root: snapshot, candidate: locked})
	writeJSON(t, filepath.Join(repository, "bundle", "sources/mattpocock-skills.lock.json"), bootstrap.ProposedLock)

	newer := locked
	newRelease := *locked.Release
	newRelease.ID++
	newRelease.NodeID = "new-release-node"
	newRelease.Tag = "v1.2.0"
	newRelease.Name = "v1.2.0"
	newRelease.CreatedAt = newRelease.CreatedAt.Add(time.Hour)
	newRelease.PublishedAt = newRelease.PublishedAt.Add(time.Hour)
	newer.Release = &newRelease
	newer.TagRefName = "refs/tags/v1.2.0"
	newer.TagRefSHA = strings.Repeat("c", 40)
	newer.TagObjects = append([]TagObject(nil), locked.TagObjects...)
	newer.TagObjects[0].SHA = newer.TagRefSHA
	newer.TagObjects[0].Name = newRelease.Tag

	movedLocked := locked
	movedLocked.TagObjects = append([]TagObject(nil), locked.TagObjects...)
	movedLocked.TagRefSHA = strings.Repeat("d", 40)
	movedLocked.TagObjects[0].SHA = movedLocked.TagRefSHA
	provider := &multiReleaseSource{root: snapshot, candidates: map[string]Candidate{locked.Release.Tag: movedLocked, newer.Release.Tag: newer}}
	plan := checkWith(t, repository, provider)
	assertBlocker(t, plan, "currently locked release/tag/commit provenance changed")
}

func TestAuthoritativeDiffReportsResourceAddRemoveAndMove(t *testing.T) {
	baseRepository, baseSnapshot := tinyRepository(t)
	provider := &fixtureSource{root: baseSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, baseRepository, provider)
	writeJSON(t, filepath.Join(baseRepository, "bundle", "sources/mattpocock-skills.lock.json"), bootstrap.ProposedLock)

	t.Run("move", func(t *testing.T) {
		repository := t.TempDir()
		copyTree(t, baseRepository, repository)
		config := filepath.Join(repository, "bundle", "sources.json")
		data, err := os.ReadFile(config)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, config, strings.Replace(string(data), "skills/engineering/one", "skills/engineering/moved", 1))
		snapshot := t.TempDir()
		writeFile(t, filepath.Join(snapshot, "skills", "engineering", "moved", "SKILL.md"), "same\n")
		plan := checkWith(t, repository, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
		assertChange(t, plan, "resource-moved")
	})

	t.Run("remove", func(t *testing.T) {
		repository := t.TempDir()
		copyTree(t, baseRepository, repository)
		writeFile(t, filepath.Join(repository, "bundle", "sources.json"), `{"schema_version":1,"sources":[{"id":"mattpocock-skills","provider":"github","repository":"mattpocock/skills","selector":{"mode":"stable-release"},"resources":[]}]}`)
		plan := checkWith(t, repository, &fixtureSource{root: t.TempDir(), candidate: acceptedCandidate()})
		assertChange(t, plan, "resource-removed")
	})

	t.Run("add", func(t *testing.T) {
		repository := t.TempDir()
		copyTree(t, baseRepository, repository)
		writeFile(t, filepath.Join(repository, "bundle", "sources.json"), `{"schema_version":1,"sources":[{"id":"mattpocock-skills","provider":"github","repository":"mattpocock/skills","selector":{"mode":"stable-release"},"resources":[{"pack_id":"matty","kind":"skill","resource_id":"one","upstream_path":"skills/engineering/one"},{"pack_id":"matty","kind":"skill","resource_id":"two","upstream_path":"skills/engineering/two"}]}]}`)
		writeFile(t, filepath.Join(repository, "bundle", "packs", "matty", "pack.json"), `{"schema_version":1,"id":"matty","version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/engineering/one"},{"kind":"skill","id":"two","source":"skills/engineering/two"}]}`)
		writeFile(t, filepath.Join(repository, "bundle", "skills", "engineering", "two", "SKILL.md"), "two\n")
		snapshot := t.TempDir()
		copyTree(t, baseSnapshot, snapshot)
		writeFile(t, filepath.Join(snapshot, "skills", "engineering", "two", "SKILL.md"), "two\n")
		plan := checkWith(t, repository, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
		assertChange(t, plan, "resource-added")
	})
}

func TestSelectorsRejectFloatingAndResolveDeterministically(t *testing.T) {
	now := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	provider := &selectorSource{fixtureSource: fixtureSource{candidate: acceptedCandidate()}, releases: []Release{
		{ID: 1, Tag: "v1", PublishedAt: now},
		{ID: 3, Tag: "v3", PublishedAt: now, Draft: true},
		{ID: 2, Tag: "v2", PublishedAt: now},
		{ID: 4, Tag: "v4-beta", PublishedAt: now.Add(time.Hour), Prerelease: true},
	}}
	engine := Engine{allowBootstrap: true, Source: provider}
	source := SourceConfig{Repository: "mattpocock/skills"}
	candidate, err := engine.resolve(context.Background(), source, Selector{Mode: "stable-release"})
	if err != nil || candidate.Release == nil || candidate.Release.Tag != "v2" {
		t.Fatalf("newest deterministic stable = %#v, %v", candidate.Release, err)
	}
	candidate, err = engine.resolve(context.Background(), source, Selector{Mode: "prerelease", Ref: "v4-beta"})
	if err != nil || candidate.Release == nil || candidate.Release.Tag != "v4-beta" {
		t.Fatalf("exact prerelease = %#v, %v", candidate.Release, err)
	}
	for _, selector := range []Selector{{Mode: "branch", Ref: "main"}, {Mode: "commit", Ref: "abc"}, {Mode: "stable-release", Ref: "v1"}} {
		if err := validateSelector(selector); err == nil {
			t.Fatalf("selector %#v unexpectedly accepted", selector)
		}
	}
}

func TestRecursiveAnnotatedTagChainAllowsDistinctInnerNames(t *testing.T) {
	candidate := acceptedCandidate()
	outer := strings.Repeat("c", 40)
	inner := strings.Repeat("d", 40)
	candidate.TagRefSHA = outer
	candidate.TagObjects = []TagObject{
		{SHA: outer, Name: candidate.Release.Tag, TargetSHA: inner, TargetType: "tag", Verification: Verification{Reason: "unsigned"}},
		{SHA: inner, Name: "upstream-inner-tag", TargetSHA: candidate.Commit, TargetType: "commit", Verification: Verification{Reason: "unsigned"}},
	}
	if !continuousTagChain(candidate) {
		t.Fatal("valid recursively annotated tag chain was rejected")
	}
}

func TestVerificationEvidenceRejectsInconsistentAndMixedStates(t *testing.T) {
	candidate := acceptedCandidate()
	malformed := candidate.CommitVerify
	malformed.Reason = "malformed_signature"
	candidate.TagObjects[0].Verification = malformed
	if eligibleAutomaticEvidence(candidate) {
		t.Fatal("one valid commit masked malformed tag verification evidence")
	}
	assertContains(t, validateCandidate(SourceConfig{Repository: candidate.Repository}, candidate, Selector{Mode: SelectorStableRelease}), "stable release lacks eligible verification evidence")

	manual := acceptedCandidate()
	manual.Release = nil
	manual.TagRefName = ""
	manual.TagRefType = ""
	manual.TagRefSHA = ""
	manual.TagObjects = nil
	manual.CommitVerify.Reason = "unavailable"
	if !invalidVerification(manual) {
		t.Fatal("manual candidate accepted unavailable verification evidence")
	}
	assertContains(t, validateCandidate(SourceConfig{Repository: manual.Repository}, manual, Selector{Mode: SelectorCommit, Ref: manual.Commit}), "manual candidate carries invalid verification evidence")
}

func TestConfigPathSafetyDiffAndCanonicalization(t *testing.T) {
	valid := `{"schema_version":1,"sources":[{"id":"s","provider":"github","repository":"o/r","selector":{"mode":"stable-release"},"resources":[{"pack_id":"p","kind":"skill","resource_id":"r","upstream_path":"skills/x/r"}]}]}`
	if _, err := LoadConfig(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(valid, "skills/x/r", "../escape", 1),
		strings.Replace(valid, `"resources"`, `"unknown":true,"resources"`, 1),
		strings.Replace(valid, `"stable-release"`, `"branch"`, 1),
	} {
		if _, err := LoadConfig(strings.NewReader(invalid)); err == nil {
			t.Fatalf("invalid configuration accepted: %s", invalid)
		}
	}
	binding := Binding{PackID: "p", ResourceID: "r", VendoredPath: "bundle/skills/r"}
	local := []FileEvidence{{Path: "a", SHA256: "old"}, {Path: "gone", SHA256: "gone"}}
	candidate := []FileEvidence{{Path: "a", SHA256: "new"}, {Path: "added", SHA256: "added"}}
	changes := diffFiles(binding, local, candidate)
	sort.Slice(changes, func(i, j int) bool { return changes[i].Kind < changes[j].Kind })
	if len(changes) != 3 {
		t.Fatalf("diff changes = %#v", changes)
	}
	plan := Plan{SchemaVersion: 1, Changes: changes, Blockers: []string{}, Discoveries: []string{}}
	sortPlan(&plan)
	id, err := seal(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanID = id
	first, _ := plan.CanonicalJSON()
	second, _ := plan.CanonicalJSON()
	if string(first) != string(second) || !plan.VerifySeal() {
		t.Fatal("plan canonicalization is not stable")
	}
}

func TestInventoryRejectsSymlinksAndUnsafePermissions(t *testing.T) {
	for _, setup := range []func(string) error{
		func(root string) error { return os.Symlink("target", filepath.Join(root, "link")) },
		func(root string) error {
			name := filepath.Join(root, "world-writable")
			if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
				return err
			}
			return os.Chmod(name, 0o666)
		},
	} {
		root := t.TempDir()
		if err := setup(root); err != nil {
			t.Fatal(err)
		}
		if _, err := inventory(root); err == nil {
			t.Fatal("unsafe inventory unexpectedly accepted")
		}
	}
}

func TestInventoryAcceptsAnInertRegularFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "idea-coach.md")
	writeFile(t, root, "inert agent instructions\n")

	files, err := inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileEvidence{{Path: ".", Size: 25, Mode: 0o644, SHA256: hashBytes([]byte("inert agent instructions\n"))}}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("file inventory = %#v, want %#v", files, want)
	}
}

func TestCheckInventoriesManifestV2FileResourceWithoutExecutingIt(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	writeFile(t, filepath.Join(repository, "bundle", "sources.json"), `{"schema_version":1,"sources":[{"id":"mattpocock-skills","provider":"github","repository":"mattpocock/skills","selector":{"mode":"stable-release"},"resources":[{"pack_id":"addy","kind":"agent","resource_id":"idea-coach","upstream_path":"content/agents/idea-coach.md"},{"pack_id":"addy","kind":"asset","resource_id":"shared-reference","upstream_path":"content/references/shared.md"},{"pack_id":"addy","kind":"command","resource_id":"refine-idea","upstream_path":"content/commands/refine-idea.md"},{"pack_id":"addy","kind":"notice","resource_id":"license","upstream_path":"content/notices/MIT.txt"},{"pack_id":"addy","kind":"skill","resource_id":"idea-refine","upstream_path":"content/skills/idea-refine"}]}]}`)
	writeFile(t, filepath.Join(repository, "bundle", "packs", "addy", "pack.json"), `{"schema_version":2,"id":"addy","version":"1.0.0","provides":["workflow:idea-refine"],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"agent","id":"idea-coach","source":"content/agents/idea-coach.md","description":"Coaches idea refinement","mode":"subagent","tools":["browser"],"permissions":["browser","network"],"requires":["skill:idea-refine"],"bindings":[{"surface":"codex","projection":"agent","name":"idea-coach","invocation":"@idea-coach","mode":"native","sharing":"exclusive"},{"surface":"opencode","projection":"agent","name":"idea-coach","invocation":"@idea-coach","mode":"native","sharing":"exclusive"}]},{"kind":"asset","id":"shared-reference","source":"content/references/shared.md","requires":[]},{"kind":"command","id":"refine-idea","source":"content/commands/refine-idea.md","arguments":{"mode":"freeform","placeholder":"$ARGUMENTS"},"requires":["agent:idea-coach","asset:shared-reference","skill:idea-refine"],"bindings":[{"surface":"codex","projection":"skill","name":"refine-idea","invocation":"$refine-idea","mode":"degraded","degradation":"codex-command-as-workflow-skill","sharing":"exclusive"},{"surface":"opencode","projection":"command","name":"refine-idea","invocation":"/refine-idea","mode":"native","sharing":"exclusive"}]},{"kind":"notice","id":"license","source":"content/notices/MIT.txt","license":"MIT","attribution":"Copyright Addy contributors","requires":[]},{"kind":"skill","id":"idea-refine","source":"content/skills/idea-refine","requires":["asset:shared-reference"],"bindings":[{"surface":"codex","projection":"skill","name":"idea-refine","invocation":"$idea-refine","mode":"native","sharing":"exclusive"},{"surface":"opencode","projection":"skill","name":"idea-refine","invocation":"$idea-refine","mode":"native","sharing":"exclusive"}]}],"contract":{"exclusions":[{"id":"upstream-hooks","source_paths":["hooks/pre-commit"],"reason":"hooks are inert"}],"optional_modes":[{"id":"browser-research","authorities":["browser","network"],"fallback":"continue from supplied evidence"}]}}`)
	contents := map[string]string{
		"content/agents/idea-coach.md": "agent", "content/references/shared.md": "asset", "content/commands/refine-idea.md": "command", "content/notices/MIT.txt": "MIT", "content/skills/idea-refine/SKILL.md": "skill", "content/skills/idea-refine/idea-refine.sh": "#!/bin/sh\n",
	}
	for path, content := range contents {
		writeFile(t, filepath.Join(repository, "bundle", filepath.FromSlash(path)), content)
	}
	os.RemoveAll(filepath.Join(repository, "bundle", "packs", "matty"))
	os.RemoveAll(filepath.Join(repository, "bundle", "skills"))
	os.RemoveAll(snapshot)
	snapshot = t.TempDir()
	for path, content := range contents {
		writeFile(t, filepath.Join(snapshot, filepath.FromSlash(path)), content)
	}

	plan := checkWith(t, repository, &fixtureSource{root: snapshot, candidate: acceptedCandidate()})
	if plan.Counts.Resources != 5 || plan.Counts.Files != 6 || len(plan.ProposedLock.Resources) != 5 {
		t.Fatalf("file resource was not inventoried: counts=%#v lock=%#v blockers=%v", plan.Counts, plan.ProposedLock.Resources, plan.Blockers)
	}
	if got := plan.ProposedLock.Resources[0].Files[0].Path; got != "." {
		t.Fatalf("file evidence path = %q, want root marker", got)
	}
}

func TestCheckRejectsManifestV2ProducerRuntimeDisagreement(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	writeFile(t, filepath.Join(repository, "bundle", "packs", "matty", "pack.json"), `{"schema_version":2,"id":"matty","version":"2.0.0","resources":[]}`)
	_, err := (Engine{allowBootstrap: true, Source: &fixtureSource{root: snapshot, candidate: acceptedCandidate()}}).Check(context.Background(), CheckRequest{RepositoryRoot: repository, SourceID: "mattpocock-skills", AcquisitionDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "disagrees with capability-pack contract") {
		t.Fatalf("error = %v", err)
	}
}

type fixtureSource struct {
	root      string
	candidate Candidate
}

type gatedSnapshotSource struct {
	fixtureSource
	acquired chan struct{}
	proceed  chan struct{}
}

func (source *gatedSnapshotSource) WithSnapshot(_ context.Context, _ Candidate, temporaryRoot string, visit func(string) error) error {
	snapshot := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeError(source.root, snapshot); err != nil {
		return err
	}
	close(source.acquired)
	<-source.proceed
	err := visit(snapshot)
	cleanupErr := os.RemoveAll(snapshot)
	if err != nil {
		return err
	}
	return cleanupErr
}

func (source *fixtureSource) Releases(context.Context, SourceConfig) ([]Release, error) {
	if source.candidate.Release == nil {
		return nil, nil
	}
	return []Release{*source.candidate.Release}, nil
}

func (source *fixtureSource) ResolveRelease(_ context.Context, _ SourceConfig, release Release) (Candidate, error) {
	candidate := source.candidate
	candidate.Release = &release
	return candidate, nil
}

func (source *fixtureSource) ResolveCommit(_ context.Context, _ SourceConfig, sha string) (Candidate, error) {
	candidate := source.candidate
	candidate.Release = nil
	candidate.TagObjects = nil
	candidate.TagRefSHA = ""
	candidate.Commit = sha
	return candidate, nil
}

func (source *fixtureSource) WithSnapshot(_ context.Context, _ Candidate, temporaryRoot string, visit func(string) error) error {
	snapshot := filepath.Join(temporaryRoot, "snapshot")
	if err := copyTreeError(source.root, snapshot); err != nil {
		return err
	}
	err := visit(snapshot)
	cleanupErr := os.RemoveAll(snapshot)
	if err != nil {
		return err
	}
	return cleanupErr
}

type selectorSource struct {
	fixtureSource
	releases []Release
}

type multiReleaseSource struct {
	root       string
	candidates map[string]Candidate
}

func (source *multiReleaseSource) Releases(context.Context, SourceConfig) ([]Release, error) {
	var releases []Release
	for _, candidate := range source.candidates {
		releases = append(releases, *candidate.Release)
	}
	return releases, nil
}

func (source *multiReleaseSource) ResolveRelease(_ context.Context, _ SourceConfig, release Release) (Candidate, error) {
	candidate, ok := source.candidates[release.Tag]
	if !ok {
		return Candidate{}, errors.New("release candidate not found")
	}
	candidate.Release = &release
	return candidate, nil
}

func (source *multiReleaseSource) ResolveCommit(_ context.Context, _ SourceConfig, sha string) (Candidate, error) {
	for _, candidate := range source.candidates {
		if candidate.Commit == sha {
			candidate.Release = nil
			candidate.TagObjects = nil
			candidate.TagRefName = ""
			candidate.TagRefType = ""
			candidate.TagRefSHA = ""
			return candidate, nil
		}
	}
	return Candidate{}, errors.New("commit candidate not found")
}

func (source *multiReleaseSource) WithSnapshot(_ context.Context, _ Candidate, temporaryRoot string, visit func(string) error) error {
	return (&fixtureSource{root: source.root}).WithSnapshot(context.Background(), Candidate{}, temporaryRoot, visit)
}

func (source *selectorSource) Releases(context.Context, SourceConfig) ([]Release, error) {
	return append([]Release(nil), source.releases...), nil
}

func (source *selectorSource) ResolveRelease(_ context.Context, _ SourceConfig, release Release) (Candidate, error) {
	candidate := source.candidate
	candidate.Release = &release
	return candidate, nil
}

func acceptedCandidate() Candidate {
	verifiedAt := time.Date(2026, 7, 8, 13, 20, 40, 0, time.UTC)
	signatureHash := strings.Repeat("a", 64)
	payloadHash := strings.Repeat("b", 64)
	return Candidate{
		Repository:       "mattpocock/skills",
		RepositoryID:     1148788086,
		RepositoryNodeID: "R_kgDORHkddg",
		RepositoryHTML:   "https://github.com/mattpocock/skills",
		RepositoryClone:  "https://github.com/mattpocock/skills.git",
		RepositoryAPI:    "https://api.github.com/repos/mattpocock/skills",
		Visibility:       "public",
		Owner:            "mattpocock",
		OwnerID:          28293365,
		OwnerNodeID:      "MDQ6VXNlcjI4MjkzMzY1",
		Public:           true,
		Release:          &Release{ID: 350942193, NodeID: "RE_kwDORHkdds4U6vPx", Tag: "v1.1.0", Name: "v1.1.0", Target: "main", CreatedAt: time.Date(2026, 7, 8, 13, 20, 55, 0, time.UTC), PublishedAt: time.Date(2026, 7, 8, 13, 20, 57, 0, time.UTC), Author: Actor{Login: "github-actions[bot]", ID: 41898282, NodeID: "MDM6Qm90NDE4OTgyODI="}},
		TagRefName:       "refs/tags/v1.1.0",
		TagRefType:       "tag",
		TagRefSHA:        "eabea89380927aadb93abf6e290a19334d249292",
		TagObjects:       []TagObject{{SHA: "eabea89380927aadb93abf6e290a19334d249292", Name: "v1.1.0", TargetSHA: "d574778f94cf620fcc8ce741584093bc650a61d3", TargetType: "commit", Verification: Verification{Reason: "unsigned"}}},
		Commit:           "d574778f94cf620fcc8ce741584093bc650a61d3",
		CommitNodeID:     "C_kwDORHkddtoAKGQ1",
		Tree:             "fa3f8882cef6fa6d9960283a49db0a58636af3ca",
		Parents:          []string{"cc1e24891df515a43a034cd91d3f64e17d1c9ffb", "47845ac1e15d048c2bbb20413a44de8681209601"},
		CommitVerify:     Verification{Verified: true, Reason: "valid", VerifiedAt: &verifiedAt, SignatureSHA256: &signatureHash, PayloadSHA256: &payloadHash},
	}
}

func checkWith(t *testing.T, repository string, source Source) Plan {
	t.Helper()
	acquisition := t.TempDir()
	plan, err := (Engine{allowBootstrap: true, Source: source}).Check(context.Background(), CheckRequest{RepositoryRoot: repository, SourceID: "mattpocock-skills", AcquisitionDir: acquisition})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(acquisition)
	if err != nil || len(entries) != 0 {
		t.Fatalf("acquisition area not cleaned: %v, %#v", err, entries)
	}
	return plan
}

func realSnapshot(t *testing.T, repository string, upstreamChanges bool) string {
	t.Helper()
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join(repository, "bundle", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	var resources []Binding
	for _, source := range config.Sources {
		if source.ID == "mattpocock-skills" {
			resources = source.Resources
			break
		}
	}
	if resources == nil {
		t.Fatal("mattpocock-skills source is missing")
	}
	for _, binding := range resources {
		copyTree(t, filepath.Join(repository, "bundle", filepath.FromSlash(binding.UpstreamPath)), filepath.Join(root, filepath.FromSlash(binding.UpstreamPath)))
	}
	if upstreamChanges {
		copyTree(t, filepath.Join(repository, "internal", "packsync", "testdata", "real-upstream"), root)
	}
	for _, discovery := range acceptedDiscoveries {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(discovery)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func tinyRepository(t *testing.T) (string, string) {
	t.Helper()
	repository := t.TempDir()
	writeFile(t, filepath.Join(repository, "bundle", "sources.json"), `{"schema_version":1,"sources":[{"id":"mattpocock-skills","provider":"github","repository":"mattpocock/skills","selector":{"mode":"stable-release"},"resources":[{"pack_id":"matty","kind":"skill","resource_id":"one","upstream_path":"skills/engineering/one"}]}]}`)
	writeFile(t, filepath.Join(repository, "bundle", "packs", "matty", "pack.json"), `{"schema_version":1,"id":"matty","version":"1.0.0","resources":[{"kind":"skill","id":"one","source":"skills/engineering/one"}]}`)
	writeFile(t, filepath.Join(repository, "bundle", "skills", "engineering", "one", "SKILL.md"), "same\n")
	snapshot := t.TempDir()
	writeFile(t, filepath.Join(snapshot, "skills", "engineering", "one", "SKILL.md"), "same\n")
	return repository, snapshot
}

func assertBlocker(t *testing.T, plan Plan, contains string) {
	t.Helper()
	for _, blocker := range plan.Blockers {
		if strings.Contains(blocker, contains) {
			return
		}
	}
	t.Fatalf("blockers %#v do not contain %q", plan.Blockers, contains)
}

func assertChange(t *testing.T, plan Plan, kind string) {
	t.Helper()
	for _, change := range plan.Changes {
		if change.Kind == kind {
			return
		}
	}
	t.Fatalf("changes %#v do not contain %q", plan.Changes, kind)
}

func assertContains(t *testing.T, values []string, contains string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(value, contains) {
			return
		}
	}
	t.Fatalf("values %#v do not contain %q", values, contains)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func bootstrapRepository(t *testing.T, source string) string {
	t.Helper()
	repository := t.TempDir()
	copyTree(t, filepath.Join(source, "bundle"), filepath.Join(repository, "bundle"))
	writeFile(t, filepath.Join(repository, "skills-lock.json"), "{}\n")
	removeProductionLock(t, repository)
	return repository
}

func removeProductionLock(t *testing.T, repository string) {
	t.Helper()
	err := os.Remove(filepath.Join(repository, "bundle", "sources/mattpocock-skills.lock.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, name string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, name, string(data)+"\n")
}

func mutateCompatibilityEvidence(t *testing.T, repository string, mutate func(map[string]any)) {
	t.Helper()
	name := filepath.Join(repository, "bundle", "compatibility", "matty", "1.0.0-to-2.0.0.json")
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	mutate(evidence)
	writeJSON(t, name, evidence)
}

func writeFile(t *testing.T, name, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	if err := copyTreeError(source, destination); err != nil {
		t.Fatal(err)
	}
}

func copyTreeError(source, destination string) error {
	return filepath.WalkDir(source, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		input, err := os.Open(name)
		if err != nil {
			return err
		}
		defer input.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
