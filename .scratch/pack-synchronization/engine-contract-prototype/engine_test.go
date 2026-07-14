package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sandbox(t *testing.T) (string, fixtureGateway, SourceConfig, CheckRequest) {
	t.Helper()
	root, gateway, source, err := buildDemo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME"} {
		dir := filepath.Join(root, strings.ToLower(key))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(key, dir)
	}
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	t.Setenv("https_proxy", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	req := CheckRequest{Source: source, Selector: Selector{Mode: "stable-release"}, RepositoryRoot: filepath.Join(root, "repo"), TempRoot: filepath.Join(root, "tmp"), Historical: "immutable-artifact"}
	return root, gateway, source, req
}

func TestCheckPlansDeterministicallyAndReportsUnselected(t *testing.T) {
	_, gateway, _, req := sandbox(t)
	engine := Engine{Gateway: gateway}
	one, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	two, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if one.ID != two.ID || !reflect.DeepEqual(one.Changes, two.Changes) {
		t.Fatalf("plan is not deterministic: %s != %s", one.ID, two.ID)
	}
	entries, err := os.ReadDir(req.TempRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary acquisitions leaked: %v %v", entries, err)
	}
	if one.Status != statusReview || len(one.AffectedPacks) != 1 || !one.AffectedPacks[0].SemanticEvidence {
		t.Fatalf("unexpected plan: %#v", one)
	}
	if len(one.Notices) != 1 || !strings.Contains(one.Notices[0], "unselected") {
		t.Fatalf("missing addition notice: %#v", one.Notices)
	}
}

func TestExplicitResolutionRejectsFloatingAndAcceptsExactCommit(t *testing.T) {
	_, gateway, _, req := sandbox(t)
	engine := Engine{Gateway: gateway}
	req.Selector = Selector{Mode: "branch", Ref: "main"}
	if _, err := engine.Check(context.Background(), req); err == nil {
		t.Fatal("floating branch accepted")
	}
	req.Selector = Selector{Mode: "commit", Ref: "short"}
	if _, err := engine.Check(context.Background(), req); err == nil {
		t.Fatal("short SHA accepted")
	}
	req.Selector = Selector{Mode: "commit", Ref: strings.Repeat("1", 40)}
	plan, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Candidate.Commit != strings.Repeat("1", 40) {
		t.Fatal("exact commit not resolved")
	}
	req.Selector = Selector{Mode: "prerelease", Ref: "v1.2.0-beta.1"}
	plan, err = engine.Check(context.Background(), req)
	if err != nil || !plan.Candidate.Prerelease {
		t.Fatalf("explicit prerelease not resolved: %#v %v", plan.Candidate, err)
	}
}

func TestProvenanceAndLocalDriftFailClosed(t *testing.T) {
	root, gateway, _, req := sandbox(t)
	bad := gateway
	bad.releases = append([]Candidate(nil), gateway.releases...)
	bad.releases[0].RepositoryID++
	plan, err := (Engine{Gateway: bad}).Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != statusBlocked {
		t.Fatalf("identity replacement did not block: %#v", plan.Blockers)
	}
	if err := os.WriteFile(filepath.Join(root, "repo/bundle/skills/alpha/SKILL.md"), []byte("local edit"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err = (Engine{Gateway: gateway}).Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != statusBlocked || !contains(plan.Blockers, "drifted") {
		t.Fatalf("local drift did not block: %#v", plan.Blockers)
	}
}

func TestMissingMoveRemovalAndModificationAreVisible(t *testing.T) {
	root, gateway, source, req := sandbox(t)
	if err := os.RemoveAll(filepath.Join(root, "upstream/1111111111111111111111111111111111111111/skills/alpha")); err != nil {
		t.Fatal(err)
	}
	plan, err := (Engine{Gateway: gateway}).Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(plan.Blockers, "selected resource missing") {
		t.Fatalf("missing resource not visible: %#v", plan.Blockers)
	}
	if err := os.MkdirAll(filepath.Join(root, "upstream/1111111111111111111111111111111111111111/skills/moved"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "upstream/1111111111111111111111111111111111111111/skills/moved/SKILL.md"), []byte("old bytes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	source.Bindings[0].UpstreamPath = "skills/moved"
	req.Source = source
	plan, err = (Engine{Gateway: gateway}).Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(plan, "upstream-path-moved") {
		t.Fatalf("move not classified: %#v", plan.Changes)
	}
	req.Source.Bindings = nil
	plan, err = (Engine{Gateway: gateway}).Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(plan, "resource-removed") || plan.AffectedPacks[0].MechanicalFloor != "major" {
		t.Fatalf("removal not major: %#v", plan)
	}
}

func TestApplyIsAtomicIdempotentAndRecovers(t *testing.T) {
	root, gateway, _, req := sandbox(t)
	engine := Engine{Gateway: gateway}
	before, _ := treeHash(filepath.Join(root, "repo/bundle"))
	plan, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &plan); err != nil {
		t.Fatal(err)
	}
	failing := engine
	failing.Failpoint = "after-backup"
	if _, err := failing.Apply(context.Background(), ApplyRequest{CheckRequest: req, Plan: plan, Classifications: demoClassification()}); err == nil {
		t.Fatal("failure not injected")
	}
	after, _ := treeHash(filepath.Join(root, "repo/bundle"))
	if before != after {
		t.Fatal("partial writes survived failed apply")
	}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: req, Plan: plan, Classifications: demoClassification()}); err != nil {
		t.Fatal(err)
	}
	second, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != statusNoop {
		t.Fatalf("second check is not idempotent: %s %#v", second.Status, second.Changes)
	}
	transaction := filepath.Join(root, "crash")
	backup := filepath.Join(transaction, "bundle.backup")
	if err := os.MkdirAll(transaction, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "repo/bundle"), backup); err != nil {
		t.Fatal(err)
	}
	marker := `{"bundle":"` + filepath.Join(root, "repo/bundle") + `","backup":"` + backup + `","staged":"` + filepath.Join(transaction, "staged") + `"}`
	if err := os.WriteFile(filepath.Join(root, "repo/.sync-transaction.json"), []byte(marker), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Recover(filepath.Join(root, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Recovered {
		t.Fatal("interrupted transaction not recovered")
	}
}

func TestClassificationFloorAndHistoricalAlternatives(t *testing.T) {
	_, gateway, _, req := sandbox(t)
	engine := Engine{Gateway: gateway}
	plan, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: req, Plan: plan}); err == nil {
		t.Fatal("missing semantic evidence accepted")
	}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: req, Plan: plan, Classifications: demoClassification()}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyHistorical(req.RepositoryRoot, "matty", "1.0.0", "immutable-artifact"); err != nil {
		t.Fatalf("immutable historical artifact unavailable: %v", err)
	}
	_, gateway, _, snapshotReq := sandbox(t)
	snapshotReq.Historical = "contract-snapshot"
	engine = Engine{Gateway: gateway}
	plan, err = engine.Check(context.Background(), snapshotReq)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Apply(context.Background(), ApplyRequest{CheckRequest: snapshotReq, Plan: plan, Classifications: demoClassification()}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyHistorical(snapshotReq.RepositoryRoot, "matty", "1.0.0", "contract-snapshot"); err == nil {
		t.Fatal("contract-only snapshot falsely claimed old bytes are operable")
	}
}

func contains(in []string, want string) bool {
	for _, s := range in {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
func hasChange(plan Plan, kind string) bool {
	for _, c := range plan.Changes {
		if c.Kind == kind {
			return true
		}
	}
	return false
}
