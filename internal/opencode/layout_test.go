package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalLayoutOwnsOpenCodePaths(t *testing.T) {
	configHome := t.TempDir()
	layout := NewCanonicalLayout(configHome)

	if layout.ConfigurationHome() != configHome {
		t.Fatalf("ConfigurationHome = %q, want %q", layout.ConfigurationHome(), configHome)
	}
	if layout.ConfigFile() != filepath.Join(configHome, "opencode", "opencode.json") {
		t.Fatalf("ConfigFile = %q", layout.ConfigFile())
	}
	if layout.PromptFile() != filepath.Join(configHome, "opencode", "packy.md") {
		t.Fatalf("PromptFile = %q", layout.PromptFile())
	}
}

func TestObserveSetupUsesCanonicalOpenCodeLayout(t *testing.T) {
	layout := NewCanonicalLayout(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(layout.ConfigFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.PromptFile(), []byte("prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.ConfigFile(), []byte(fmt.Sprintf(`{"instructions":[%q]}`, layout.PromptFile())), 0o600); err != nil {
		t.Fatal(err)
	}

	observation := ObserveSetup(layout)

	inspection := observation.Inspection()
	if observation.ConfigFile() != layout.ConfigFile() || observation.PromptFile() != layout.PromptFile() || observation.Err() != nil || !inspection.ConfigExists || !inspection.PromptExists || !inspection.HasPackyInstruction {
		t.Fatalf("observation = %#v inspection = %#v", observation, inspection)
	}
}
