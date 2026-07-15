package packsync

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
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

func TestCheckReproducesAcceptedRealTracerWithoutWritingRepository(t *testing.T) {
	repository := repositoryRoot(t)
	snapshot := realSnapshot(t, repository, true)
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
	wantCounts := Counts{Resources: 23, Files: 45, Modified: 5, Discoveries: 15}
	if !reflect.DeepEqual(plan.Counts, wantCounts) {
		t.Fatalf("counts = %#v, want %#v", plan.Counts, wantCounts)
	}
	if plan.Status != "blocked" || plan.Authoritative || !plan.VerifySeal() {
		t.Fatalf("plan status=%s authoritative=%t sealed=%t", plan.Status, plan.Authoritative, plan.VerifySeal())
	}
	if !reflect.DeepEqual(plan.Discoveries, acceptedDiscoveries) {
		t.Fatalf("discoveries = %#v", plan.Discoveries)
	}
	wantChanges := []string{
		"bundle/skills/engineering/setup-matt-pocock-skills/issue-tracker-github.md",
		"bundle/skills/engineering/setup-matt-pocock-skills/issue-tracker-gitlab.md",
		"bundle/skills/engineering/setup-matt-pocock-skills/issue-tracker-local.md",
		"bundle/skills/engineering/to-tickets/SKILL.md",
		"bundle/skills/engineering/wayfinder/SKILL.md",
	}
	var gotChanges []string
	for _, change := range plan.Changes {
		if change.Kind == "file-modified" {
			gotChanges = append(gotChanges, change.Path)
		}
	}
	if !reflect.DeepEqual(gotChanges, wantChanges) {
		t.Fatalf("modified paths = %#v", gotChanges)
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

func TestByteIdenticalBootstrapIsSealedAndOneByteInvalidatesIt(t *testing.T) {
	repository := repositoryRoot(t)
	identicalRoot := realSnapshot(t, repository, false)
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
	writeJSON(t, filepath.Join(repository, "bundle", "sources.lock.json"), bootstrap.ProposedLock)
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
		writeJSON(t, filepath.Join(copyRoot, "bundle", "sources.lock.json"), lock)
		plan := checkWith(t, copyRoot, provider)
		assertBlocker(t, plan, "retained provenance is invalid")
		assertBlocker(t, plan, "duplicate selected resource")
	})
}

func TestNewReleaseWithIdenticalBytesProducesProvenanceUpdate(t *testing.T) {
	repository, snapshot := tinyRepository(t)
	provider := &fixtureSource{root: snapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, repository, provider)
	writeJSON(t, filepath.Join(repository, "bundle", "sources.lock.json"), bootstrap.ProposedLock)
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
	plan := checkWith(t, repository, &newer)
	if plan.Status != "review-required" {
		t.Fatalf("identical-byte new release status = %s, blockers=%#v", plan.Status, plan.Blockers)
	}
	assertChange(t, plan, "provenance-updated")
}

func TestAuthoritativeDiffReportsResourceAddRemoveAndMove(t *testing.T) {
	baseRepository, baseSnapshot := tinyRepository(t)
	provider := &fixtureSource{root: baseSnapshot, candidate: acceptedCandidate()}
	bootstrap := checkWith(t, baseRepository, provider)
	writeJSON(t, filepath.Join(baseRepository, "bundle", "sources.lock.json"), bootstrap.ProposedLock)

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
	engine := Engine{Source: provider}
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

type fixtureSource struct {
	root      string
	candidate Candidate
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
	plan, err := (Engine{Source: source}).Check(context.Background(), CheckRequest{RepositoryRoot: repository, SourceID: "mattpocock-skills", AcquisitionDir: acquisition})
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
	for _, binding := range config.Sources[0].Resources {
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

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func writeJSON(t *testing.T, name string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, name, string(data)+"\n")
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
