package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallDryRunDoesNotWriteCodexPrompt(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	fixture := newCLITestFixture(t, opts)

	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "write-codex-prompt: write Codex Packy prompt markers") {
		t.Fatalf("dry-run did not report Codex prompt action:\n%s", out)
	}
	if exists(fixture.codex.PromptFile()) || exists(filepath.Dir(fixture.codex.PromptFile())) {
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
	fixture := newCLITestFixture(t, opts)

	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "write-opencode-prompt: write OpenCode Packy prompt reference") {
		t.Fatalf("dry-run did not report OpenCode prompt action:\n%s", out)
	}
	if exists(fixture.opencode.ConfigFile()) || exists(fixture.opencode.PromptFile()) || exists(filepath.Dir(fixture.opencode.ConfigFile())) {
		t.Fatalf("dry-run wrote OpenCode prompt/config")
	}
}
