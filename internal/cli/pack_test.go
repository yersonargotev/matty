package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackListAndShowAreSideEffectFree(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	runner := &fakeRunner{}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "", "MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}, Runner: runner}
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "list")
	if err != nil {
		t.Fatalf("list failed: %v\n%s", err, out)
	}
	for _, want := range []string{"PACK", "engram", "matty", "Persistent memory", "codex, opencode"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list missing %q:\n%s", want, out)
		}
	}
	show, err := executeCommand(t, NewRootCommand(opts), "pack", "show", "engram")
	if err != nil {
		t.Fatalf("show failed: %v\n%s", err, show)
	}
	for _, want := range []string{"Provides capabilities: memory:persistent", "Requires global tools: engram", "Conflicts with capabilities: none", "0 skill, 1 instruction, 1 mcp_server, 1 lifecycle"} {
		if !strings.Contains(show, want) {
			t.Fatalf("show missing %q:\n%s", want, show)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("external calls = %v", runner.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("HOME changed\nbefore:\n%s\nafter:\n%s", beforeHome, got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("bundle changed during discovery")
	}
	if _, err := os.Stat(filepath.Join(home, ".matty", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("state file exists: %v", err)
	}
}

func TestPackShowRejectsUnknownPack(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}}
	opts.Env.(MapEnv)["PATH"] = ""
	_, err := executeCommand(t, NewRootCommand(opts), "pack", "show", "mobile")
	if err == nil || !strings.Contains(err.Error(), "unknown capability pack") {
		t.Fatalf("error = %v", err)
	}
}

func TestPackStatusRendersBaselineWithoutSideEffects(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	runner := &fakeRunner{}
	opts := Options{Env: MapEnv{
		"HOME": home, "XDG_CONFIG_HOME": xdg, "PATH": "",
		"MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills"),
	}, Runner: runner}
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	overview, err := executeCommand(t, NewRootCommand(opts), "pack", "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, overview)
	}
	for _, want := range []string{
		"PACK", "SURFACE", "INTENT", "ATTEMPT", "CONFIGURED", "AUTHORIZED", "USABLE", "ACTION",
		"engram  codex", "engram  opencode", "matty   codex", "matty   opencode", "inactive",
	} {
		if !strings.Contains(overview, want) {
			t.Fatalf("overview missing %q:\n%s", want, overview)
		}
	}

	detail, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "engram", "--surface", "codex")
	if err != nil {
		t.Fatalf("targeted status failed: %v\n%s", err, detail)
	}
	for _, want := range []string{
		"engram 1.0.0 on codex", "Intent: inactive", "Latest attempt: none",
		"Readiness: configured=no, authorized=no, usable=no",
		"Projections: 0 verified; 0 drifted; 0 ambiguous", "Pending human actions: none",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail missing %q:\n%s", want, detail)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("external calls = %v", runner.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("HOME changed\nbefore:\n%s\nafter:\n%s", beforeHome, got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("bundle changed during status")
	}
	if _, err := os.Stat(filepath.Join(home, ".matty", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("state file exists: %v", err)
	}
}

func TestPackStatusRequiresCompleteTarget(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	opts := Options{Env: MapEnv{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "",
		"MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills"),
	}}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"pack", "status", "engram"}, "--surface is required"},
		{[]string{"pack", "status", "--surface", "codex"}, "a pack is required"},
		{[]string{"pack", "status", "engram", "--surface", "vscode"}, "does not support CLI surface"},
		{[]string{"pack", "status", "missing", "--surface", "codex"}, "unknown capability pack"},
	} {
		_, err := executeCommand(t, NewRootCommand(opts), tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v error = %v, want %q", tc.args, err, tc.want)
		}
	}
}
