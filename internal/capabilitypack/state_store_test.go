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

func TestFileActivationStoreMigratesLegacyDocumentsToCanonicalAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	legacy := `{"schema_version":2,"activations":[{"schema_version":1,"intent":{"pack_id":"matty","surface":"codex","version":"1.0.0","active":true,"revision":1}}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewFileActivationStore(path)
	state, err := store.Load(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 2 || state.Intent.Aliases == nil || len(state.Intent.Aliases) != 0 {
		t.Fatalf("migrated state = %+v", state)
	}
	if err := store.Save(context.Background(), SurfaceCodex, 1, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["schema_version"] != float64(3) || !strings.Contains(string(data), `"aliases": []`) {
		t.Fatalf("document = %s", data)
	}
}

func TestFileActivationStorePersistsAliasesInCanonicalOrder(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	state := ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Active: true, Revision: 1, Aliases: []SurfaceAlias{{Kind: "command", ID: "review", Name: "addy-review"}, {Kind: "agent", ID: "reviewer", Name: "addy-reviewer"}}}}
	if err := store.Save(context.Background(), SurfaceCodex, 0, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	want := []SurfaceAlias{{Kind: "agent", ID: "reviewer", Name: "addy-reviewer"}, {Kind: "command", ID: "review", Name: "addy-review"}}
	if !reflect.DeepEqual(loaded.Intent.Aliases, want) {
		t.Fatalf("aliases = %+v, want %+v", loaded.Intent.Aliases, want)
	}
}

func TestFileActivationStoreRejectsInvalidAliases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		aliases []SurfaceAlias
	}{
		{"duplicate identity", []SurfaceAlias{{Kind: "command", ID: "review", Name: "one"}, {Kind: "command", ID: "review", Name: "two"}}},
		{"unsupported kind", []SurfaceAlias{{Kind: "asset", ID: "reference", Name: "ref"}}},
		{"invalid identity", []SurfaceAlias{{Kind: "skill", ID: "Bad ID", Name: "good"}}},
		{"empty name", []SurfaceAlias{{Kind: "agent", ID: "reviewer", Name: ""}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
			state := ActivationState{Intent: ActivationIntent{PackID: "addy", Revision: 1, Aliases: tc.aliases}}
			if err := store.Save(context.Background(), SurfaceCodex, 0, state); err == nil || !strings.Contains(err.Error(), "alias") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestFileActivationStoreRejectsFutureContainedStateVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	data := `{"schema_version":3,"activations":[{"schema_version":999,"intent":{"surface":"codex","aliases":[]}}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewFileActivationStore(path).Load(context.Background(), SurfaceCodex)
	if err == nil || !strings.Contains(err.Error(), "unsupported activation schema_version 999") {
		t.Fatalf("error = %v", err)
	}
}

func TestFileActivationStoreExplainsCompareAndSwapStaleRevision(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	state := ActivationState{SchemaVersion: 1, Intent: ActivationIntent{Revision: 1}}
	if err := store.Save(context.Background(), SurfaceCodex, 0, state); err != nil {
		t.Fatal(err)
	}

	err := store.Save(context.Background(), SurfaceCodex, 0, ActivationState{SchemaVersion: 1, Intent: ActivationIntent{Revision: 2}})
	if !errors.Is(err, ErrStalePlan) || !strings.Contains(err.Error(), "changed from 0 to 1 before persistence") || !strings.Contains(err.Error(), "rerun activation") {
		t.Fatalf("error = %v", err)
	}
}

func TestFileActivationStorePreservesIndependentSurfaceState(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode} {
		state := ActivationState{Intent: ActivationIntent{PackID: "matty", Surface: surface, Active: true, Revision: 1}, Ownership: []ProjectionOwnership{{ID: "instruction:matty-guidance", Contributors: []string{"matty"}, Fingerprint: string(surface)}}}
		if err := store.Save(context.Background(), surface, 0, state); err != nil {
			t.Fatal(err)
		}
	}
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode} {
		state, err := store.Load(context.Background(), surface)
		if err != nil {
			t.Fatal(err)
		}
		if state.Intent.Surface != surface || len(state.Ownership) != 1 || state.Ownership[0].Fingerprint != string(surface) {
			t.Fatalf("%s state = %+v", surface, state)
		}
	}
}
