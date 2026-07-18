package corelifecycle

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/skillbundle"
)

func TestClassicLayoutDerivesStateFromPackyHome(t *testing.T) {
	packyHome := filepath.Join(t.TempDir(), ".packy")
	layout := NewLayout(packyHome)

	if layout.PackyHome() != packyHome {
		t.Fatalf("PackyHome = %q, want %q", layout.PackyHome(), packyHome)
	}
	if layout.StateFile() != filepath.Join(packyHome, "config.json") {
		t.Fatalf("StateFile = %q", layout.StateFile())
	}
}

func TestFacadeConfigDerivesInternalPathsFromOwnerValues(t *testing.T) {
	home := t.TempDir()
	packyHome := filepath.Join(home, ".packy")
	source := skillbundle.Source{Root: filepath.Join(t.TempDir(), "bundle", "skills")}
	installed := bootstrap.InstalledSourceAt(filepath.Join(home, ".local", "share", "packy"))
	facade := NewFacade(FacadeConfig{
		PackyHome:       packyHome,
		Skills:          skillbundle.NewGlobalLayout(home),
		SkillSource:     source,
		Codex:           codex.NewCanonicalLayout(home),
		OpenCode:        opencode.NewCanonicalLayout(filepath.Join(home, ".config")),
		Engram:          engrambin.NewTopology(filepath.Join(home, "homebrew")),
		InstalledSource: installed,
		RunningVersion:  "v1.2.3",
	}, &installTestCommands{}, time.Now)

	if facade.config.State.StateFile() != filepath.Join(packyHome, "config.json") {
		t.Fatalf("StateFile = %q", facade.config.State.StateFile())
	}
	if facade.config.Skills.Root() != filepath.Join(home, ".agents", "skills") {
		t.Fatalf("AgentSkillsDir = %q", facade.config.Skills.Root())
	}
	if facade.config.InstalledSource.Root() != installed.Root() {
		t.Fatalf("InstalledSource = %q", facade.config.InstalledSource.Root())
	}
}
