package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalLayoutOwnsCodexPaths(t *testing.T) {
	home := t.TempDir()
	layout := NewCanonicalLayout(home)

	if layout.ConfigFile() != filepath.Join(home, ".codex", "config.toml") {
		t.Fatalf("ConfigFile = %q", layout.ConfigFile())
	}
	if layout.PromptFile() != filepath.Join(home, ".codex", "AGENTS.md") {
		t.Fatalf("PromptFile = %q", layout.PromptFile())
	}
}

func TestObserveSetupUsesCanonicalPromptAndReportsMarkersAndConflicts(t *testing.T) {
	layout := NewCanonicalLayout(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(layout.PromptFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "<!-- matty:skills-router -->\n<!-- /matty:skills-router -->\n<!-- gentle-ai:persona -->x<!-- /gentle-ai:persona -->"
	if err := os.WriteFile(layout.PromptFile(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	observation := ObserveSetup(layout)

	if observation.PromptFile() != layout.PromptFile() || !observation.Exists() || !observation.HasMattyMarkers() || observation.Err() != nil {
		t.Fatalf("observation = %#v", observation)
	}
	if len(observation.Warnings()) != 1 {
		t.Fatalf("warnings = %#v", observation.Warnings())
	}
}
