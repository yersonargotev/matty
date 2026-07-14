package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type fixtureGateway struct {
	root       string
	releases   []Candidate
	candidates map[string]Candidate
}

func (g fixtureGateway) Releases(_ context.Context, _ SourceConfig) ([]Candidate, error) {
	return append([]Candidate(nil), g.releases...), nil
}
func (g fixtureGateway) ResolveExplicit(_ context.Context, _ SourceConfig, selector Selector) (Candidate, error) {
	return g.candidates[selector.Mode], nil
}
func (g fixtureGateway) Acquire(_ context.Context, candidate Candidate, destination string) error {
	return copyTree(filepath.Join(g.root, candidate.Commit), destination)
}

func main() {
	scenario := flag.String("scenario", "success", "success, invalid-provenance, idempotent, rollback, or historical")
	format := flag.String("format", "human", "human or json")
	flag.Parse()
	root, gateway, source, err := buildDemo()
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)
	engine := Engine{Gateway: gateway}
	req := CheckRequest{Source: source, Selector: Selector{Mode: "stable-release"}, RepositoryRoot: filepath.Join(root, "repo"), TempRoot: filepath.Join(root, "tmp"), Historical: "immutable-artifact"}
	if *scenario == "invalid-provenance" {
		bad := gateway
		bad.releases = append([]Candidate(nil), gateway.releases...)
		bad.releases[0].RepositoryID++
		engine.Gateway = bad
	}
	plan, err := engine.Check(context.Background(), req)
	if err != nil {
		panic(err)
	}
	if *scenario == "rollback" {
		engine.Failpoint = "after-backup"
		_, err = engine.Apply(context.Background(), ApplyRequest{CheckRequest: req, Plan: plan, Classifications: demoClassification()})
		fmt.Println("apply error:", err)
	}
	if *scenario == "success" || *scenario == "historical" {
		result, applyErr := engine.Apply(context.Background(), ApplyRequest{CheckRequest: req, Plan: plan, Classifications: demoClassification()})
		if applyErr != nil {
			panic(applyErr)
		}
		fmt.Printf("apply: %s (%s)\n", result.Status, result.PlanID)
		if *scenario == "historical" {
			fmt.Println("old version operable:", VerifyHistorical(req.RepositoryRoot, "matty", "1.0.0", "immutable-artifact") == nil)
		}
	}
	if *scenario == "idempotent" {
		_, err = engine.Apply(context.Background(), ApplyRequest{CheckRequest: req, Plan: plan, Classifications: demoClassification()})
		if err != nil {
			panic(err)
		}
		plan, err = engine.Check(context.Background(), req)
		if err != nil {
			panic(err)
		}
	}
	printPlan(plan, *format)
}

func printPlan(plan Plan, format string) {
	if format == "json" {
		data, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Println(string(data))
		return
	}
	fmt.Printf("plan %s  status=%s\nsource %s -> %s (%s)\n", plan.ID, plan.Status, plan.SourceID, plan.Candidate.Release, plan.Candidate.Commit)
	for _, c := range plan.Changes {
		fmt.Printf("  %-20s %-18s %s\n", c.Kind, c.PackID+"/"+c.ResourceID, c.Path)
	}
	for _, p := range plan.AffectedPacks {
		fmt.Printf("  pack %-10s floor=%s semantic-evidence=%v current=%s\n", p.PackID, p.MechanicalFloor, p.SemanticEvidence, p.CurrentVersion)
	}
	for _, n := range plan.Notices {
		fmt.Println("  notice:", n)
	}
	for _, b := range plan.Blockers {
		fmt.Println("  BLOCKER:", b)
	}
}

func demoClassification() map[string]Classification {
	return map[string]Classification{"matty": {Level: "patch", ProposedVersion: "1.0.1", Rationale: "existing workflow intent is preserved", ClassifierType: "human", ClassifierID: "prototype-reviewer"}}
}

func cloneCandidates(in map[string]Candidate) map[string]Candidate {
	out := map[string]Candidate{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func buildDemo() (string, fixtureGateway, SourceConfig, error) {
	root, err := os.MkdirTemp("", "sync-engine-prototype-")
	if err != nil {
		return "", fixtureGateway{}, SourceConfig{}, err
	}
	for _, path := range []string{"repo/bundle/packs/matty", "repo/bundle/skills/alpha", "upstream/1111111111111111111111111111111111111111/skills/alpha", "upstream/1111111111111111111111111111111111111111/skills/unselected", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0755); err != nil {
			return "", fixtureGateway{}, SourceConfig{}, err
		}
	}
	manifest := `{"id":"matty","version":"1.0.0","resources":[{"kind":"skill","id":"alpha","source":"skills/alpha"}]}`
	_ = os.WriteFile(filepath.Join(root, "repo/bundle/packs/matty/pack.json"), []byte(manifest), 0644)
	_ = os.WriteFile(filepath.Join(root, "repo/bundle/skills/alpha/SKILL.md"), []byte("old bytes\n"), 0644)
	_ = os.WriteFile(filepath.Join(root, "upstream/1111111111111111111111111111111111111111/skills/alpha/SKILL.md"), []byte("new bytes\n"), 0644)
	_ = os.WriteFile(filepath.Join(root, "upstream/1111111111111111111111111111111111111111/skills/unselected/SKILL.md"), []byte("new resource\n"), 0644)
	binding := Binding{PackID: "matty", Kind: "skill", ResourceID: "alpha", UpstreamPath: "skills/alpha", VendoredPath: "bundle/skills/alpha"}
	oldFiles, _, _ := inventory(filepath.Join(root, "repo/bundle/skills/alpha"))
	old := LockedResource{Binding: binding, Files: oldFiles, SHA256: resourceHash(oldFiles)}
	lock := Lock{SchemaVersion: 1, SourceID: "example", Repository: "example/public", RepositoryID: 42, Release: "v1.0.0", TagObject: "tag-old", Commit: "0000000000000000000000000000000000000000", Resources: []LockedResource{old}}
	lock.Snapshot = snapshotHash(lock.Resources)
	data, _ := json.MarshalIndent(lock, "", "  ")
	_ = os.WriteFile(filepath.Join(root, "repo/bundle/sources.lock.json"), append(data, '\n'), 0644)
	candidate := Candidate{Repository: "example/public", RepositoryID: 42, Release: "v1.1.0", ReleaseID: 11, PublishedAt: "2026-07-02T00:00:00Z", Commit: "1111111111111111111111111111111111111111", TagObject: "tag-new", Public: true, Verification: Verification{Verified: true, Reason: "valid"}}
	older := candidate
	older.Release, older.ReleaseID, older.PublishedAt = "v1.0.1", 10, "2026-07-01T00:00:00Z"
	prerelease := candidate
	prerelease.Release, prerelease.Prerelease = "v1.2.0-beta.1", true
	gateway := fixtureGateway{root: filepath.Join(root, "upstream"), releases: []Candidate{candidate, prerelease, older}, candidates: map[string]Candidate{"prerelease": prerelease, "commit": candidate}}
	return root, gateway, SourceConfig{ID: "example", Repository: "example/public", RepositoryID: 42, Bindings: []Binding{binding}}, nil
}
