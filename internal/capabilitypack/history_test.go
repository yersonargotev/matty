package capabilitypack

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCheckedInMattyHistoryIsExactSelfContainedAndDeterministic(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	pack, err := loadHistoricalArtifact(filepath.Join(bundleRoot, "history", "matty", "1.0.0"), bundleRoot, "matty", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range pack.Resources {
		if resource.Source != "" && !strings.HasPrefix(resource.Source, "history/matty/1.0.0/") {
			t.Fatalf("historical resource %s:%s escaped its artifact root: %q", resource.Kind, resource.ID, resource.Source)
		}
	}
	root := filepath.Join(bundleRoot, "history", "matty", "1.0.0")
	expected, err := inspectHistoricalArtifact(root, mustDecodeHistoricalManifest(t, root))
	if err != nil {
		t.Fatal(err)
	}
	checkedIn := readHistoricalArtifact(t, root)
	if !reflect.DeepEqual(expected, checkedIn) {
		t.Fatal("checked-in artifact evidence is not the deterministic construction from retained bytes")
	}
}

func TestCurrentBuiltInManifestsAreArchivedByteExactBeforeV3CatalogCutover(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	workflowPackID := strings.Join([]string{"ma", "tty"}, "")
	for _, item := range []struct{ id, version string }{{"engram", "1.0.0"}, {workflowPackID, "2.0.0"}} {
		t.Run(item.id, func(t *testing.T) {
			current := mustRead(t, filepath.Join(bundleRoot, "packs", item.id, "pack.json"))
			root := filepath.Join(bundleRoot, "history", item.id, item.version)
			if archived := mustRead(t, filepath.Join(root, "pack.json")); !reflect.DeepEqual(archived, current) {
				t.Fatal("archived manifest bytes differ from catalog-current bytes")
			}
			pack, err := loadHistoricalArtifact(root, bundleRoot, item.id, item.version)
			if err != nil {
				t.Fatal(err)
			}
			if pack.Version != item.version {
				t.Fatalf("historical version = %q", pack.Version)
			}
		})
	}
}

func TestV3UpdateRoutesPreserveExistingSurfaceIntent(t *testing.T) {
	type route struct {
		SchemaVersion      int    `json:"schema_version"`
		PackID             string `json:"pack_id"`
		FromVersion        string `json:"from_version"`
		ToVersion          string `json:"to_version"`
		HistoricalArtifact struct {
			Path            string `json:"path"`
			ArtifactSHA256  string `json:"artifact_sha256"`
			AggregateSHA256 string `json:"aggregate_sha256"`
		} `json:"historical_artifact"`
		PreservedSurfaces []Surface `json:"preserved_surfaces"`
		AddsClaudeIntent  bool      `json:"adds_claude_intent"`
	}
	bundleRoot := filepath.Join("..", "..", "bundle")
	workflowPackID := strings.Join([]string{"ma", "tty"}, "")
	for _, item := range []struct{ id, from, to string }{{"engram", "1.0.0", "2.0.0"}, {workflowPackID, "2.0.0", "3.0.0"}} {
		data := mustRead(t, filepath.Join(bundleRoot, "compatibility", item.id, item.from+"-to-"+item.to+".json"))
		var got route
		if err := strictDecode(data, &got); err != nil {
			t.Fatal(err)
		}
		if got.SchemaVersion != 1 || got.PackID != item.id || got.FromVersion != item.from || got.ToVersion != item.to || got.AddsClaudeIntent || !reflect.DeepEqual(got.PreservedSurfaces, []Surface{SurfaceCodex, SurfaceOpenCode}) {
			t.Fatalf("unsupported update route: %#v", got)
		}
		artifact := readHistoricalArtifact(t, filepath.Join(bundleRoot, "history", item.id, item.from))
		if got.HistoricalArtifact.AggregateSHA256 != artifact.AggregateSHA256 {
			t.Fatal("route does not identify exact historical aggregate")
		}
	}
}

func TestHistoricalArtifactRoundTripsManifestV2FileAndDirectoryResources(t *testing.T) {
	fixture, _ := writeManifestV2Fixture(t)
	bundle := filepath.Join(t.TempDir(), "bundle")
	root := filepath.Join(bundle, "history", "addy", "1.0.0")
	copyHistoricalTree(t, filepath.Join(fixture, "content"), filepath.Join(root, "content"))
	mustWrite(t, filepath.Join(root, "pack.json"), mustRead(t, filepath.Join(fixture, "packs", "addy", "pack.json")), 0o600)
	mustWrite(t, filepath.Join(root, "artifact.json"), []byte("{}\n"), 0o600)

	pack := mustDecodeHistoricalManifest(t, root)
	artifact, err := inspectHistoricalArtifact(root, pack)
	if err != nil {
		t.Fatal(err)
	}
	writeHistoricalArtifact(t, root, artifact)

	if _, err := loadHistoricalArtifact(root, bundle, "addy", "1.0.0"); err == nil || !strings.Contains(err.Error(), "no trusted immutable aggregate") {
		t.Fatalf("synthetic history was accepted without an explicit trust root: %v", err)
	}
	trustKey := "addy@1.0.0"
	trustedHistoricalAggregates[trustKey] = artifact.AggregateSHA256
	t.Cleanup(func() { delete(trustedHistoricalAggregates, trustKey) })

	loaded, err := loadHistoricalArtifact(root, bundle, "addy", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Resources) != len(pack.Resources) {
		t.Fatalf("loaded %d resources, want %d", len(loaded.Resources), len(pack.Resources))
	}
	for i := range loaded.Resources {
		want := pack.Resources[i]
		want.Source = filepath.ToSlash(filepath.Join("history", "addy", "1.0.0", filepath.FromSlash(want.Source)))
		if !reflect.DeepEqual(loaded.Resources[i], want) {
			t.Fatalf("resource %d changed across historical round-trip:\n got: %#v\nwant: %#v", i, loaded.Resources[i], want)
		}
	}
	if got := len(artifact.Resources); got != 5 {
		t.Fatalf("artifact evidence contains %d resources, want 5", got)
	}
}

func TestCheckedInMattyTwoPreservesWorkflowConventionsInOneOwnedInstruction(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	catalog, err := Discover(bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := catalog.Show("matty")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != "2.0.0" {
		t.Fatalf("matty version = %s", pack.Version)
	}
	var source string
	for _, resource := range pack.Resources {
		if resource.Kind == "instruction" && resource.ID == "matty-workflow-conventions" {
			source = resource.Source
		}
	}
	if source != "instructions/matty-workflow-conventions.md" {
		t.Fatalf("workflow conventions source = %q", source)
	}
	content := string(mustRead(t, filepath.Join(bundleRoot, filepath.FromSlash(source))))
	for _, convention := range []string{"Specs and tickets", ".scratch/<feature-slug>/", "tracker-defined wayfinding operations"} {
		if !strings.Contains(content, convention) {
			t.Fatalf("workflow conventions instruction omits %q", convention)
		}
	}
}

func TestHistoricalArtifactFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, string)
	}{
		{name: "missing bytes", mutate: func(t *testing.T, root, _ string) {
			mustRemove(t, firstHistoricalResourceFile(t, root))
		}},
		{name: "one changed byte", mutate: func(t *testing.T, root, _ string) {
			path := firstHistoricalResourceFile(t, root)
			data := mustRead(t, path)
			data[0] ^= 0xff
			mustWrite(t, path, data, 0o644)
		}},
		{name: "manifest mismatch", mutate: func(t *testing.T, root, _ string) {
			data := strings.Replace(string(mustRead(t, filepath.Join(root, "pack.json"))), `"version": "1.0.0"`, `"version": "9.0.0"`, 1)
			mustWrite(t, filepath.Join(root, "pack.json"), []byte(data), 0o644)
			refreshHistoricalManifestEvidence(t, root)
		}},
		{name: "absolute source", mutate: func(t *testing.T, root, _ string) {
			rewriteFirstHistoricalSource(t, root, "/tmp/outside")
		}},
		{name: "traversal source", mutate: func(t *testing.T, root, _ string) {
			rewriteFirstHistoricalSource(t, root, "../outside")
		}},
		{name: "symlink source", mutate: func(t *testing.T, root, bundle string) {
			artifact := readHistoricalArtifact(t, root)
			source := filepath.Join(root, filepath.FromSlash(artifact.Resources[len(artifact.Resources)-1].Source))
			mustRemove(t, source)
			fallback := filepath.Join(bundle, "instructions", "matty-guidance.md")
			mustWrite(t, fallback, []byte("catalog-current fallback\n"), 0o644)
			if err := os.Symlink(fallback, source); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unsafe permissions", mutate: func(t *testing.T, root, _ string) {
			if err := os.Chmod(firstHistoricalResourceFile(t, root), 0o666); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "manipulated evidence", mutate: func(t *testing.T, root, _ string) {
			artifact := readHistoricalArtifact(t, root)
			artifact.Resources[0].Files[0].SHA256 = strings.Repeat("0", 64)
			artifact.Resources[0].SHA256 = historicalFilesHash(artifact.Resources[0].Files)
			artifact.AggregateSHA256 = historicalAggregateHash(artifact)
			writeHistoricalArtifact(t, root, artifact)
		}},
		{name: "coordinated bytes and evidence mutation", mutate: func(t *testing.T, root, _ string) {
			path := firstHistoricalResourceFile(t, root)
			mustWrite(t, path, append(mustRead(t, path), '\n'), 0o644)
			artifact, err := inspectHistoricalArtifact(root, mustDecodeHistoricalManifest(t, root))
			if err != nil {
				t.Fatal(err)
			}
			writeHistoricalArtifact(t, root, artifact)
		}},
		{name: "wrong artifact identity", mutate: func(t *testing.T, root, _ string) {
			artifact := readHistoricalArtifact(t, root)
			artifact.PackID = "other"
			artifact.AggregateSHA256 = historicalAggregateHash(artifact)
			writeHistoricalArtifact(t, root, artifact)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog, root, bundle := clonedHistoricalCatalog(t)
			test.mutate(t, root, bundle)
			if _, err := catalog.resolveIntentPack("matty", "1.0.0"); err == nil {
				t.Fatal("mutated historical artifact was accepted")
			}
		})
	}
}

func TestHistoricalOperationsUseOnlyHistoryWhileSelectionStaysCatalogCurrent(t *testing.T) {
	catalog, root, bundle := clonedHistoricalCatalog(t)
	intent := ActivationIntent{PackID: "matty", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}
	adapter := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	report, err := facade.Status(context.Background(), StatusRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatalf("historical status failed: %v", err)
	}
	if len(report.Entries) != 1 || report.Entries[0].Intent.Version != "1.0.0" || !report.Entries[0].UpdateAvailable {
		t.Fatalf("status omitted pinned version or update availability: %+v", report)
	}
	jsonReport := report.JSONReport(true)
	if jsonReport.Entries[0].Intent.Version != "1.0.0" || !jsonReport.Entries[0].UpdateAvailable {
		t.Fatalf("structured status omitted pinned version or update availability: %+v", jsonReport)
	}
	update, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatalf("historical update comparison failed: %v", err)
	}
	if update.Pack().Version != "2.0.0" || len(update.beforeCompositionFacts) != 1 || update.beforeCompositionFacts[0].Version != "1.0.0" {
		t.Fatalf("update did not compare historical 1.0.0 to catalog-current 2.0.0: %+v", update)
	}
	if selected, err := catalog.Show("matty"); err != nil || selected.Version != "2.0.0" {
		t.Fatalf("catalog-current selection did not remain on 2.0.0: pack=%+v err=%v", selected, err)
	}
	if err := os.Remove(filepath.Join(bundle, "instructions", "matty-guidance.md")); err != nil {
		t.Fatal(err)
	}

	historicalInstruction := filepath.Join(root, "instructions", "matty-guidance.md")
	desired := historicalHash(mustRead(t, historicalInstruction))
	drift := SurfaceInspection{Revision: "drift", Projections: []ObservedProjection{{ID: "instruction:matty-guidance", Exists: true, ObservedFingerprint: "drifted", DesiredFingerprint: desired, Action: ProjectionAction{ID: "instruction:matty-guidance", Kind: ActionInstructionFile, Source: historicalInstruction}}}}
	verified := drift
	verified.Revision = "verified"
	verified.Projections = append([]ObservedProjection(nil), drift.Projections...)
	verified.Projections[0].ObservedFingerprint = desired
	adapter = &fakeSurfaceAdapter{observations: []SurfaceInspection{drift, drift, verified}}
	store.state.Ownership = []ProjectionOwnership{{ID: "instruction:matty-guidance", Contributors: []string{"matty"}, Fingerprint: desired}}
	facade = NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	repair, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil || len(repair.Phases()) != 1 || !strings.Contains(repair.Phases()[0].Actions[0].Description, "intent-selected content") {
		t.Fatalf("historical repair plan failed: plan=%+v err=%v", repair, err)
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: repair, Approvals: []ApprovalReceipt{facade.Approve(repair, ConsentReversibleLocal)}, Interactive: true})
	if err != nil || !result.Verified {
		t.Fatalf("historical repair apply failed: result=%+v err=%v", result, err)
	}
	assertHistoricalTransitionSources(t, adapter.calls)

	deletion := SurfaceInspection{Revision: "present", Projections: []ObservedProjection{{ID: "instruction:matty-guidance", Exists: true, ObservedFingerprint: desired, Action: ProjectionAction{ID: "instruction:matty-guidance", Kind: ActionInstructionFile, Mode: ProjectionRemoveContent}}}}
	adapter = &fakeSurfaceAdapter{observations: []SurfaceInspection{deletion, deletion, {Revision: "removed"}}}
	facade = NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	deactivate, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil || deactivate.OldVersion() != "1.0.0" {
		t.Fatalf("historical deactivate comparison failed: plan=%+v err=%v", deactivate, err)
	}
	result, err = facade.Apply(context.Background(), ApplyRequest{Plan: deactivate, Approvals: []ApprovalReceipt{facade.Approve(deactivate, ConsentDestructiveCleanup)}, Interactive: true})
	if err != nil || !result.Verified || store.state.Intent.Active {
		t.Fatalf("historical deactivate apply failed: result=%+v state=%+v err=%v", result, store.state, err)
	}
	assertHistoricalTransitionSources(t, adapter.calls)

	store.state = ActivationState{}
	if _, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex}); err == nil {
		t.Fatal("fresh activation selected history after catalog-current bytes were removed")
	}
	if _, err := catalog.Show("matty"); err == nil {
		t.Fatal("catalog-current selection ignored missing current bytes")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
}

func assertHistoricalTransitionSources(t *testing.T, calls []surfaceInspectionCall) {
	t.Helper()
	for _, call := range calls {
		for _, pack := range []Pack{call.prior, call.desired} {
			for _, resource := range pack.Resources {
				if pack.Version == "1.0.0" && resource.Source != "" && !strings.HasPrefix(resource.Source, "history/matty/1.0.0/") {
					t.Fatalf("pinned pack used non-historical source %q", resource.Source)
				}
			}
		}
	}
}

func TestHistoryNeverFallsBackToCatalogCurrent(t *testing.T) {
	catalog, root, bundle := clonedHistoricalCatalog(t)
	artifact := readHistoricalArtifact(t, root)
	source := artifact.Resources[len(artifact.Resources)-1].Source
	mustRemove(t, filepath.Join(root, filepath.FromSlash(source)))
	fallback := filepath.Join(bundle, filepath.FromSlash(source))
	mustWrite(t, fallback, []byte("catalog-current bytes\n"), 0o644)
	if _, err := catalog.resolveIntentPack("matty", "1.0.0"); err == nil {
		t.Fatal("missing historical bytes fell back to catalog-current")
	}
}

func TestHistoricalPreflightRejectsChangedCurrentPackInMixedComposition(t *testing.T) {
	_, _, bundle := clonedHistoricalCatalog(t)
	engramManifest := `{"schema_version":1,"id":"engram","version":"1.0.0","provides":["memory:persistent"],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"instruction","id":"engram-memory","source":"instructions/engram-memory.md"}]}`
	mustWrite(t, filepath.Join(bundle, "packs", "engram", "pack.json"), []byte(engramManifest), 0o644)
	catalog, err := DiscoverForDurableIntents(bundle)
	if err != nil {
		t.Fatal(err)
	}
	mattyIntent := ActivationIntent{PackID: "matty", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3}
	engramIntent := ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}
	state := ActivationState{Intent: mattyIntent, Intents: []ActivationIntent{mattyIntent, engramIntent}}
	facade := NewFacade(catalog, WithActivation(&fakeActivationStore{state: state}, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}))
	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	mustRemove(t, filepath.Join(bundle, "instructions", "engram-memory.md"))
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err == nil || !strings.Contains(err.Error(), "changed after Preview") {
		t.Fatalf("mixed historical/current composition accepted stale current bytes: %v", err)
	}
}

func clonedHistoricalCatalog(t *testing.T) (Catalog, string, string) {
	t.Helper()
	repositoryBundle := filepath.Join("..", "..", "bundle")
	bundle := filepath.Join(t.TempDir(), "bundle")
	root := filepath.Join(bundle, "history", "matty", "1.0.0")
	copyHistoricalTree(t, filepath.Join(repositoryBundle, "history", "matty", "1.0.0"), root)
	for _, path := range []string{"packs", "skills", "instructions"} {
		copyHistoricalTree(t, filepath.Join(repositoryBundle, path), filepath.Join(bundle, path))
	}
	manifestPath := filepath.Join(bundle, "packs", "matty", "pack.json")
	var manifest map[string]any
	if err := json.Unmarshal(mustRead(t, manifestPath), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["version"] = "2.0.0"
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, manifestPath, encoded, 0o644)
	catalog, err := DiscoverForDurableIntents(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return catalog, root, bundle
}

func mustDecodeHistoricalManifest(t *testing.T, root string) Pack {
	t.Helper()
	pack, err := decodeManifest(filepath.Join(root, "pack.json"), root)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func readHistoricalArtifact(t *testing.T, root string) historicalArtifact {
	t.Helper()
	var artifact historicalArtifact
	if err := strictDecode(mustRead(t, filepath.Join(root, "artifact.json")), &artifact); err != nil {
		t.Fatal(err)
	}
	return artifact
}

func writeHistoricalArtifact(t *testing.T, root string, artifact historicalArtifact) {
	t.Helper()
	data, err := canonicalHistoricalArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "artifact.json"), data, 0o644)
}

func refreshHistoricalManifestEvidence(t *testing.T, root string) {
	t.Helper()
	artifact := readHistoricalArtifact(t, root)
	manifest, err := inspectHistoricalFile(root, filepath.Join(root, "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	artifact.Manifest = manifest
	artifact.AggregateSHA256 = historicalAggregateHash(artifact)
	writeHistoricalArtifact(t, root, artifact)
}

func rewriteFirstHistoricalSource(t *testing.T, root, source string) {
	t.Helper()
	path := filepath.Join(root, "pack.json")
	data := string(mustRead(t, path))
	artifact := readHistoricalArtifact(t, root)
	data = strings.Replace(data, `"source": "`+artifact.Resources[0].Source+`"`, `"source": "`+source+`"`, 1)
	mustWrite(t, path, []byte(data), 0o644)
	refreshHistoricalManifestEvidence(t, root)
}

func firstHistoricalResourceFile(t *testing.T, root string) string {
	t.Helper()
	artifact := readHistoricalArtifact(t, root)
	return filepath.Join(root, filepath.FromSlash(artifact.Resources[0].Files[0].Path))
}

func copyHistoricalTree(t *testing.T, source, target string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustWrite(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
