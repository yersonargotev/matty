package capabilitypack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/bundletransaction"
)

func TestDiscoverWaitsForCompleteBundleTransaction(t *testing.T) {
	bundle := writeCatalogFixture(t)
	repository := filepath.Dir(bundle)
	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Discover(bundle)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Discover completed outside the shared lock: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Discover did not resume after the bundle transaction")
	}
}

type blockingBundleAdapter struct {
	bundleRoot string
	entered    chan string
	release    chan struct{}
}

func (a *blockingBundleAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	data, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.FromSlash(transition.Desired.Resources[0].Source)))
	if err != nil {
		return SurfaceInspection{}, err
	}
	a.entered <- string(data)
	<-a.release
	return SurfaceInspection{}, nil
}

func (*blockingBundleAdapter) ApplyProjections(context.Context, []ProjectionAction) *ProjectionActionError {
	return nil
}

func TestPreviewHoldsBundleTransactionThroughAdapterReads(t *testing.T) {
	bundle := writeCatalogFixture(t)
	catalog, err := Discover(bundle)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &blockingBundleAdapter{bundleRoot: bundle, entered: make(chan string, 1), release: make(chan struct{})}
	facade := NewFacade(catalog, WithActivation(&fakeActivationStore{}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	previewDone := make(chan error, 1)
	go func() {
		_, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
		previewDone <- err
	}()
	select {
	case got := <-adapter.entered:
		if got != "matty" {
			t.Fatalf("adapter read %q, want one complete old bundle generation", got)
		}
	case <-time.After(time.Second):
		t.Fatal("adapter did not inspect the bundle")
	}

	transaction := make(chan *bundletransaction.Guard, 1)
	transactionErr := make(chan error, 1)
	go func() {
		guard, err := bundletransaction.Acquire(context.Background(), filepath.Dir(bundle))
		if err != nil {
			transactionErr <- err
			return
		}
		transaction <- guard
	}()
	select {
	case guard := <-transaction:
		guard.Release()
		t.Fatal("bundle transaction started before the adapter completed its observation")
	case err := <-transactionErr:
		t.Fatal(err)
	case <-time.After(40 * time.Millisecond):
	}

	close(adapter.release)
	if err := <-previewDone; err != nil {
		t.Fatal(err)
	}
	select {
	case guard := <-transaction:
		if err := guard.Release(); err != nil {
			t.Fatal(err)
		}
	case err := <-transactionErr:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("bundle transaction did not resume after the complete observation")
	}
}

func TestDeferredCatalogRefreshesAfterBundleSwap(t *testing.T) {
	bundle := writeCatalogFixture(t)
	catalog, err := DiscoverForDurableIntents(bundle)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Dir(bundle)
	stage := filepath.Join(repository, "bundle-stage")
	for _, path := range []string{
		"instructions/engram.md",
		"instructions/matty.md",
		"packs/engram/pack.json",
		"packs/matty/pack.json",
	} {
		data, err := os.ReadFile(filepath.Join(bundle, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if path == "packs/matty/pack.json" {
			data = []byte(strings.Replace(string(data), `"version":"1.0.0"`, `"version":"2.0.0"`, 1))
		}
		target := filepath.Join(stage, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(repository, "bundle-backup")
	if err := os.Rename(bundle, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, bundle); err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}

	pack, err := catalog.Show("matty")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != "2.0.0" {
		t.Fatalf("Show version=%s, want complete new generation 2.0.0", pack.Version)
	}
	packs, err := catalog.ListCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if packs[1].ID != "matty" || packs[1].Version != "2.0.0" {
		t.Fatalf("ListCurrent packs=%+v, want complete new generation", packs)
	}
}

func TestDiscoverLoadsInitialStrictCatalog(t *testing.T) {
	bundleRoot := writeCatalogFixture(t)
	catalog, err := Discover(bundleRoot)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	packs := catalog.List()
	if len(packs) != 2 || packs[0].ID != "engram" || packs[1].ID != "matty" {
		t.Fatalf("packs = %#v", packs)
	}
	engram, err := catalog.Show("engram")
	if err != nil {
		t.Fatal(err)
	}
	if got := engram.ResourceCounts(); got != (ResourceCounts{Instructions: 1, MCPServers: 1, Lifecycles: 1}) {
		t.Fatalf("counts = %#v", got)
	}
	if strings.Join(engram.Requires.Tools, ",") != "engram" {
		t.Fatalf("tools = %v", engram.Requires.Tools)
	}
	if _, err := catalog.Show("web"); err == nil || !strings.Contains(err.Error(), "pack list") {
		t.Fatalf("unknown error = %v", err)
	}
}

func TestDiscoverRejectsInvalidManifests(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"unknown field", func(m map[string]any) { m["host_config"] = true }, "unknown field"},
		{"invalid id", func(m map[string]any) { m["id"] = "Engram" }, "lowercase kebab-case"},
		{"invalid version", func(m map[string]any) { m["version"] = "1" }, "SemVer"},
		{"invalid prerelease version", func(m map[string]any) { m["version"] = "1.0.0-01" }, "SemVer"},
		{"invalid composition", func(m map[string]any) { m["conflicts"] = []any{"memory:persistent"} }, "appears in both"},
		{"unknown resource", func(m map[string]any) { m["resources"] = []any{map[string]any{"kind": "config", "id": "bad"}} }, "unsupported resource kind"},
		{"duplicate resource", func(m map[string]any) { r := m["resources"].([]any); m["resources"] = append(r, r[0]) }, "duplicate resource"},
		{"traversing source", func(m map[string]any) {
			m["resources"] = []any{map[string]any{"kind": "instruction", "id": "bad-source", "source": "../outside"}}
		}, "escapes the bundle root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeCatalogFixture(t)
			path := filepath.Join(root, "packs", "engram", "pack.json")
			data, _ := os.ReadFile(path)
			var manifest map[string]any
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			tt.mutate(manifest)
			encoded, _ := json.Marshal(manifest)
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Discover(root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateSurfacesRejectsUnsupportedSurface(t *testing.T) {
	root := writeCatalogFixture(t)
	entries := append([]catalogEntry(nil), initialCatalog...)
	entries[0].Surfaces = []Surface{"codex", "mobile"}
	if _, err := discoverCatalog(root, entries); err == nil || !strings.Contains(err.Error(), "unsupported CLI surface") {
		t.Fatalf("error = %v", err)
	}
}

func writeCatalogFixture(t *testing.T) string {
	t.Helper()
	bundle := filepath.Join(t.TempDir(), "bundle")
	skillRoot := filepath.Join(bundle, "skills")
	instructionRoot := filepath.Join(bundle, "instructions")
	for _, dir := range []string{skillRoot, instructionRoot, filepath.Join(bundle, "packs", "engram"), filepath.Join(bundle, "packs", "matty")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(instructionRoot, "engram.md"), []byte("engram"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructionRoot, "matty.md"), []byte("matty"), 0o600); err != nil {
		t.Fatal(err)
	}
	engram := `{"schema_version":1,"id":"engram","version":"1.0.0","provides":["memory:persistent"],"requires":{"capabilities":[],"tools":["engram"]},"conflicts":[],"resources":[{"kind":"instruction","id":"engram-memory","source":"instructions/engram.md"},{"kind":"mcp_server","id":"engram","command":"engram","args":["mcp"]},{"kind":"lifecycle","id":"engram-memory"}]}`
	matty := `{"schema_version":1,"id":"matty","version":"1.0.0","provides":["workflow:matty"],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"instruction","id":"matty-guidance","source":"instructions/matty.md"}]}`
	for name, data := range map[string]string{"engram": engram, "matty": matty} {
		if err := os.WriteFile(filepath.Join(bundle, "packs", name, "pack.json"), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return bundle
}
