package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestInstallDryRunDoesNotWriteOpenCodePrompt(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "write-opencode-prompt: write OpenCode Matty prompt reference") {
		t.Fatalf("dry-run did not report OpenCode prompt action:\n%s", out)
	}
	if exists(paths.OpenCodeConfigFile) || exists(paths.OpenCodePromptFile) || exists(filepath.Dir(paths.OpenCodeConfigFile)) {
		t.Fatalf("dry-run wrote OpenCode prompt/config")
	}
}
