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

func TestInspectDeactivationRemovesManagedBlocksAndPreservesUnmanagedCodexConfig(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "guide.md")
	prompt := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(source, []byte("guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, []byte("unmanaged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewActivationAdapterWithConfig(root, filepath.Join(root, "skills"), prompt, filepath.Join(root, "config.toml"))
	active := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md"}}}
	observed, err := adapter.InspectActivation(context.Background(), active)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{observed.Projections[0].Action}); err != nil {
		t.Fatal(err)
	}
	removal, err := adapter.InspectDeactivation(context.Background(), active, capabilitypack.Pack{ID: "desired"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(removal.RemovalCandidates) != 1 || removal.RemovalCandidates[0].Action.Mode != capabilitypack.ProjectionRemoveContent {
		t.Fatalf("removals = %+v", removal.RemovalCandidates)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{removal.RemovalCandidates[0].Action}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(prompt)
	if err != nil || strings.TrimSpace(string(data)) != "unmanaged" {
		t.Fatalf("prompt = %q err=%v", data, err)
	}
}

func TestInspectDeactivationComposesMultipleRemovalsFromOneCodexFile(t *testing.T) {
	root := t.TempDir()
	prompt := filepath.Join(root, "AGENTS.md")
	for _, name := range []string{"one.md", "two.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	content := "unmanaged\n"
	for _, id := range []string{"one", "two"} {
		start, end := instructionMarkers(id)
		content = mergeBlock(content, start+"\n"+id+"\n"+end, start, end)
	}
	if err := os.WriteFile(prompt, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewActivationAdapterWithConfig(root, filepath.Join(root, "skills"), prompt, filepath.Join(root, "config.toml"))
	active := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "one", Source: "one.md"}, {Kind: "instruction", ID: "two", Source: "two.md"}}}
	removal, err := adapter.InspectDeactivation(context.Background(), active, capabilitypack.Pack{ID: "desired"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(removal.RemovalCandidates) != 2 {
		t.Fatalf("removals=%+v", removal.RemovalCandidates)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{removal.RemovalCandidates[0].Action, removal.RemovalCandidates[1].Action}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(prompt)
	if strings.TrimSpace(string(got)) != "unmanaged" {
		t.Fatalf("prompt=%q", got)
	}
}
