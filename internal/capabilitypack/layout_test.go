package capabilitypack

import (
	"path/filepath"
	"testing"
)

func TestStateLayoutDerivesPackStateFromMattyHome(t *testing.T) {
	mattyHome := filepath.Join(t.TempDir(), "home", ".matty")

	layout := NewStateLayout(mattyHome)

	if got, want := layout.File(), filepath.Join(mattyHome, "packs.json"); got != want {
		t.Fatalf("pack state file = %q, want %q", got, want)
	}
}
