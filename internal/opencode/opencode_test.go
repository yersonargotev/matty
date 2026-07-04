package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesPromptAndConfigInstruction(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")

	result, err := Write(configPath, promptPath)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", result.Warnings)
	}
	prompt := readString(t, promptPath)
	for _, want := range []string{"~/.agents/skills", "ask-matt", "Engram memory tools", "delegation rules"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	config := readJSON(t, configPath)
	instructions := stringSlice(t, config["instructions"])
	if len(instructions) != 1 || instructions[0] != promptPath {
		t.Fatalf("instructions = %#v, want only %q", instructions, promptPath)
	}
}

func TestWriteMergesOpenCodeConfigAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")
	existing := `{
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
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	result, err := Write(configPath, promptPath)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "gentle-ai") {
		t.Fatalf("warnings = %#v, want gentle-ai warning", result.Warnings)
	}
	first := readString(t, configPath)
	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("second Write failed: %v", err)
	}
	second := readString(t, configPath)
	if second != first {
		t.Fatalf("Write should be idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	config := readJSON(t, configPath)
	for _, key := range []string{"$schema", "model", "mcp", "provider", "plugin", "agent", "profile"} {
		if _, ok := config[key]; !ok {
			t.Fatalf("merged config lost %q: %#v", key, config)
		}
	}
	instructions := stringSlice(t, config["instructions"])
	if got := strings.Join(instructions, "\n"); got != strings.Join([]string{"CONTRIBUTING.md", promptPath}, "\n") {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWriteMergesOpenCodeJSONCConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")
	existing := `{
  // OpenCode accepts JSONC global config.
  "$schema": "https://opencode.ai/config.json",
  "model": "anthropic/claude-sonnet-4-5",
  "mcp": {
    "jira": {
      "type": "remote",
      "url": "https://jira.example.com/mcp", // keep URL strings intact
      "enabled": true,
    },
  },
  "instructions": [
    "CONTRIBUTING.md",
  ],
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed for JSONC config: %v", err)
	}
	updated := readString(t, configPath)
	for _, want := range []string{"// OpenCode accepts JSONC global config.", "// keep URL strings intact", "\"enabled\": true,"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("JSONC merge did not preserve %q:\n%s", want, updated)
		}
	}
	config := decodeJSONC(t, updated)
	if config["model"] != "anthropic/claude-sonnet-4-5" {
		t.Fatalf("model was not preserved: %#v", config)
	}
	mcp, ok := config["mcp"].(map[string]any)
	if !ok || mcp["jira"] == nil {
		t.Fatalf("mcp config was not preserved: %#v", config["mcp"])
	}
	instructions := stringSlice(t, config["instructions"])
	if got := strings.Join(instructions, "\n"); got != strings.Join([]string{"CONTRIBUTING.md", promptPath}, "\n") {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWritePreservesLeadingJSONCComments(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")
	existing := `// global OpenCode config
{
  "model": "anthropic/claude-sonnet-4-5"
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	if !strings.HasPrefix(updated, "// global OpenCode config") {
		t.Fatalf("leading comment was not preserved:\n%s", updated)
	}
	instructions := stringSlice(t, decodeJSONC(t, updated)["instructions"])
	if len(instructions) != 1 || instructions[0] != promptPath {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWriteAndRemovePreserveInstructionComments(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")
	existing := `{
  "instructions": [
    // keep user rationale
    "CONTRIBUTING.md", // keep inline note
  ]
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	for _, want := range []string{"// keep user rationale", "// keep inline note", "\"CONTRIBUTING.md\",", promptPath} {
		if !strings.Contains(updated, want) {
			t.Fatalf("OpenCode merge did not preserve %q:\n%s", want, updated)
		}
	}
	if err := Remove(configPath, promptPath); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	removed := readString(t, configPath)
	for _, want := range []string{"// keep user rationale", "// keep inline note", "\"CONTRIBUTING.md\","} {
		if !strings.Contains(removed, want) {
			t.Fatalf("OpenCode remove did not preserve %q:\n%s", want, removed)
		}
	}
	if strings.Contains(removed, promptPath) {
		t.Fatalf("Remove left Matty instruction reference:\n%s", removed)
	}
}

func TestWritePreservesTrailingPropertyCommentWhenAddingInstructions(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")
	existing := `{
  "model": "anthropic/claude-sonnet-4-5" // keep model note
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	for _, want := range []string{"\"model\": \"anthropic/claude-sonnet-4-5\", // keep model note", promptPath} {
		if !strings.Contains(updated, want) {
			t.Fatalf("OpenCode merge did not preserve/comment-comma %q:\n%s", want, updated)
		}
	}
	instructions := stringSlice(t, decodeJSONC(t, updated)["instructions"])
	if len(instructions) != 1 || instructions[0] != promptPath {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWritePreservesTrailingInstructionCommentWhenAppending(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")
	existing := `{
  "instructions": [
    "CONTRIBUTING.md" // keep instruction note
  ]
}
`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	for _, want := range []string{"\"CONTRIBUTING.md\", // keep instruction note", promptPath} {
		if !strings.Contains(updated, want) {
			t.Fatalf("OpenCode merge did not preserve/comment-comma %q:\n%s", want, updated)
		}
	}
	instructions := stringSlice(t, decodeJSONC(t, updated)["instructions"])
	if got := strings.Join(instructions, "\n"); got != strings.Join([]string{"CONTRIBUTING.md", promptPath}, "\n") {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestWriteInsertsIntoCommentOnlyJSONCConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")
	existing := "{\n  // keep this comment\n}\n"
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write jsonc config: %v", err)
	}

	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	updated := readString(t, configPath)
	if !strings.Contains(updated, "// keep this comment") {
		t.Fatalf("comment was not preserved:\n%s", updated)
	}
	instructions := stringSlice(t, decodeJSONC(t, updated)["instructions"])
	if len(instructions) != 1 || instructions[0] != promptPath {
		t.Fatalf("instructions = %#v", instructions)
	}
}

func TestRemoveDeletesOnlyMattyOpenCodeEntries(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	promptPath := filepath.Join(dir, "matty.md")
	if _, err := Write(configPath, promptPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	config := readJSON(t, configPath)
	config["instructions"] = []any{"CONTRIBUTING.md", promptPath, "docs/rules.md", promptPath}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("encode test config: %v", err)
	}
	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	if err := Remove(configPath, promptPath); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("prompt still exists or stat failed: %v", err)
	}
	instructions := stringSlice(t, readJSON(t, configPath)["instructions"])
	if got := strings.Join(instructions, "\n"); got != "CONTRIBUTING.md\ndocs/rules.md" {
		t.Fatalf("instructions after remove = %#v", instructions)
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	return decodeJSONC(t, readString(t, path))
}

func decodeJSONC(t *testing.T, content string) map[string]any {
	t.Helper()
	data, err := jsoncToJSON(content)
	if err != nil {
		t.Fatalf("convert JSONC: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode JSONC: %v", err)
	}
	return out
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want []any", value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("item = %#v, want string", item)
		}
		out = append(out, s)
	}
	return out
}
