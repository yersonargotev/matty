package capabilitypack

import (
	"path/filepath"
	"testing"
)

func TestStateLayoutDerivesPackStateFromPackyHome(t *testing.T) {
	packyHome := filepath.Join(t.TempDir(), "home", ".packy")

	layout := NewStateLayout(packyHome)

	if got, want := layout.File(), filepath.Join(packyHome, "packs.json"); got != want {
		t.Fatalf("pack state file = %q, want %q", got, want)
	}
}
