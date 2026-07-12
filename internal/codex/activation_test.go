package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/matty/internal/capabilitypack"
)

func TestEngramProjectionIsCodexSpecificAndPreservesUnmanagedFiles(t *testing.T) {
	root := t.TempDir()
	instructions := filepath.Join(root, "instructions")
	if err := os.MkdirAll(instructions, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructions, "engram-memory.md"), []byte("remember safely\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt := filepath.Join(root, ".codex", "AGENTS.md")
	config := filepath.Join(root, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(prompt), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, []byte("# keep Codex guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("[mcp_servers.jira]\ncommand = \"jira\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "engram", Version: "1.0.0", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: "engram-memory", Source: "instructions/engram-memory.md"},
		{Kind: "mcp_server", ID: "engram", Command: "engram", Args: []string{"mcp", "--tools=agent"}},
	}}
	adapter := NewActivationAdapterWithConfig(root, filepath.Join(root, ".agents", "skills"), prompt, config)
	observed, err := adapter.InspectActivation(context.Background(), pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Projections) != 2 || observed.Projections[0].Action.Kind != capabilitypack.ActionInstructionFile || observed.Projections[1].Action.Kind != capabilitypack.ActionCodexMCPConfig {
		t.Fatalf("projections = %#v", observed.Projections)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{observed.Projections[0].Action, observed.Projections[1].Action}); err != nil {
		t.Fatal(err)
	}
	updatedPrompt, err := os.ReadFile(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updatedPrompt), "keep Codex guidance") || !strings.Contains(string(updatedPrompt), "matty:pack:engram-memory:start") {
		t.Fatalf("Codex prompt was not preserved/projected: %s", updatedPrompt)
	}
	updatedConfig, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updatedConfig), "[mcp_servers.jira]") || !strings.Contains(string(updatedConfig), "[mcp_servers.engram]") {
		t.Fatalf("Codex config was not preserved/projected: %s", updatedConfig)
	}
	verified, err := adapter.InspectActivation(context.Background(), pack)
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range verified.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("projection did not verify: %+v", projection)
		}
	}
	if verified.Readiness.Authorized || verified.Readiness.Usable || len(verified.PendingHumanActions) != 2 {
		t.Fatalf("Engram readiness = %+v pending=%v", verified.Readiness, verified.PendingHumanActions)
	}
}
