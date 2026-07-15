package capabilitypack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCheckedInMattyHistoryIsExactSelfContainedAndDeterministic(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	currentManifest, err := os.ReadFile(filepath.Join(bundleRoot, "packs", "matty", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	historicalManifest, err := os.ReadFile(filepath.Join(bundleRoot, "history", "matty", "1.0.0", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(historicalManifest, currentManifest) {
		t.Fatal("historical pack manifest is not the exact matty@1.0.0 manifest")
	}
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
	catalog.packs[0].Version = "2.0.0"
	for i := range catalog.packs[0].Resources {
		if catalog.packs[0].Resources[i].Source != "" {
			catalog.packs[0].Resources[i].Source = "catalog-current/removed/" + catalog.packs[0].Resources[i].ID
		}
	}
	if err := os.RemoveAll(filepath.Join(bundle, "catalog-current")); err != nil {
		t.Fatal(err)
	}
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
	fresh, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil || fresh.Pack().Version != "2.0.0" {
		t.Fatalf("fresh activation selected history: version=%s err=%v", fresh.Pack().Version, err)
	}
	if selected, err := catalog.Show("matty"); err != nil || selected.Version != "2.0.0" {
		t.Fatalf("catalog selection exposed history: pack=%+v err=%v", selected, err)
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

func clonedHistoricalCatalog(t *testing.T) (Catalog, string, string) {
	t.Helper()
	repositoryBundle := filepath.Join("..", "..", "bundle")
	current, err := decodeManifest(filepath.Join(repositoryBundle, "packs", "matty", "pack.json"), repositoryBundle)
	if err != nil {
		t.Fatal(err)
	}
	current.Surfaces = []Surface{SurfaceCodex, SurfaceOpenCode}
	current.Version = "2.0.0"
	bundle := filepath.Join(t.TempDir(), "bundle")
	root := filepath.Join(bundle, "history", "matty", "1.0.0")
	copyHistoricalTree(t, filepath.Join(repositoryBundle, "history", "matty", "1.0.0"), root)
	return Catalog{packs: []Pack{current}, bundleRoot: bundle}, root, bundle
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
