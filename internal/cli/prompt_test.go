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
		t.Fatalf("install should write one Matty skills-router block:\n%s", installed)
	}
	if strings.Count(installed, "<!-- matty:rules -->") != 1 || !strings.Contains(installed, "Keep diffs surgical") {
		t.Fatalf("install should write one Matty rules block:\n%s", installed)
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

func TestInstallUpdateAndUninstallManageOpenCodePromptIdempotently(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.OpenCodeConfigFile), 0o700); err != nil {
		t.Fatalf("mkdir opencode config: %v", err)
	}
	original := `{
  "$schema": "https://opencode.ai/config.json",
  "model": "anthropic/claude-sonnet-4-5",
  "mcp": {"jira": {"type": "remote", "enabled": true}},
  "provider": {"openai": {"npm": "@ai-sdk/openai"}},
  "plugin": ["gentle-ai-plugin"],
  "agent": {"gentle-ai": {"prompt": "keep"}},
  "profile": {"gentle-ai": {"agent": "gentle-ai"}},
  "instructions": ["CONTRIBUTING.md"]
}
`
	if err := os.WriteFile(paths.OpenCodeConfigFile, []byte(original), 0o600); err != nil {
		t.Fatalf("write original opencode config: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "warning: OpenCode config contains gentle-ai references") {
		t.Fatalf("install did not warn about preserved gentle-ai OpenCode config:\n%s", out)
	}
	installedConfig := readFileString(t, paths.OpenCodeConfigFile)
	installedPrompt := readFileString(t, paths.OpenCodePromptFile)
	for _, want := range []string{"~/.agents/skills", "ask-matt", "Engram memory tools", "delegation rules", "<!-- matty:rules -->", "Keep diffs surgical"} {
		if !strings.Contains(installedPrompt, want) {
			t.Fatalf("OpenCode prompt missing %q:\n%s", want, installedPrompt)
		}
	}
	for _, want := range []string{"\"mcp\"", "\"provider\"", "\"model\"", "\"plugin\"", "\"agent\"", "\"profile\"", paths.OpenCodePromptFile, "CONTRIBUTING.md"} {
		if !strings.Contains(installedConfig, want) {
			t.Fatalf("OpenCode config missing %q:\n%s", want, installedConfig)
		}
	}

	out, err = executeCommand(t, NewRootCommand(opts), "update")
	if err != nil {
		t.Fatalf("update failed: %v\n%s", err, out)
	}
	updatedConfig := readFileString(t, paths.OpenCodeConfigFile)
	if updatedConfig != installedConfig {
		t.Fatalf("update should be idempotent for OpenCode config:\nbefore:\n%s\nafter:\n%s", installedConfig, updatedConfig)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "uninstall")
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}
	uninstalledConfig := readFileString(t, paths.OpenCodeConfigFile)
	if strings.Contains(uninstalledConfig, paths.OpenCodePromptFile) {
		t.Fatalf("uninstall should remove Matty instruction reference:\n%s", uninstalledConfig)
	}
	for _, want := range []string{"\"mcp\"", "\"provider\"", "\"model\"", "\"plugin\"", "\"agent\"", "\"profile\"", "CONTRIBUTING.md"} {
		if !strings.Contains(uninstalledConfig, want) {
			t.Fatalf("uninstall lost OpenCode user config %q:\n%s", want, uninstalledConfig)
		}
	}
	if exists(paths.OpenCodePromptFile) {
		t.Fatalf("uninstall should remove Matty OpenCode prompt file")
	}
}

func TestInstallCreatesOpenCodeConfigWhenMissing(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	config := readFileString(t, paths.OpenCodeConfigFile)
	if !strings.Contains(config, paths.OpenCodePromptFile) {
		t.Fatalf("OpenCode config missing Matty instruction reference:\n%s", config)
	}
	if !strings.Contains(readFileString(t, paths.OpenCodePromptFile), "ask-matt") {
		t.Fatalf("OpenCode prompt missing ask-matt")
	}
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
