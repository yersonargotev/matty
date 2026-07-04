package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallUpdateAndUninstallManageCodexPromptIdempotently(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.CodexPromptFile), 0o700); err != nil {
		t.Fatalf("mkdir codex config: %v", err)
	}
	original := "# Personal Codex notes\n\n<!-- gentle-ai:persona -->\nKeep Gentle AI.\n<!-- /gentle-ai:persona -->\n"
	if err := os.WriteFile(paths.CodexPromptFile, []byte(original), 0o600); err != nil {
		t.Fatalf("write original prompt: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "warning: Codex prompt contains gentle-ai managed blocks") {
		t.Fatalf("install did not warn about preserved gentle-ai block:\n%s", out)
	}
	installed := readFileString(t, paths.CodexPromptFile)
	if strings.Count(installed, "<!-- matty:skills-router -->") != 1 {
		t.Fatalf("install should write one Matty block:\n%s", installed)
	}
	if !strings.Contains(installed, original) {
		t.Fatalf("install did not preserve existing content:\n%s", installed)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "update")
	if err != nil {
		t.Fatalf("update failed: %v\n%s", err, out)
	}
	updated := readFileString(t, paths.CodexPromptFile)
	if updated != installed {
		t.Fatalf("update should be idempotent for Codex prompt:\nbefore:\n%s\nafter:\n%s", installed, updated)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "uninstall")
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}
	uninstalled := readFileString(t, paths.CodexPromptFile)
	if uninstalled != original {
		t.Fatalf("uninstall should remove only Matty block:\ngot:\n%s\nwant:\n%s", uninstalled, original)
	}
}

func TestInstallDryRunDoesNotWriteCodexPrompt(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "write-codex-prompt: write Codex Matty prompt markers") {
		t.Fatalf("dry-run did not report Codex prompt action:\n%s", out)
	}
	if exists(paths.CodexPromptFile) || exists(filepath.Dir(paths.CodexPromptFile)) {
		t.Fatalf("dry-run wrote Codex prompt/config directory")
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
