package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

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
