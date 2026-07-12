package opencodeactivation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/matty/internal/capabilitypack"
)

func TestActivationAdapterAppliesHostSpecificProjectionsAndPreservesJSONC(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	skill := filepath.Join(bundle, "skills", "ask-matt")
	instruction := filepath.Join(bundle, "instructions", "matty.md")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(instruction), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Ask Matt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instruction, []byte("OpenCode Matty guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "xdg", "opencode", "opencode.json")
	prompt := filepath.Join(root, "xdg", "opencode", "matty.md")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "// keep OpenCode syntax\n{\n  \"model\": \"anthropic/test\",\n  \"instructions\": [\"CONTRIBUTING.md\",],\n}\n"
	if err := os.WriteFile(config, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	pack := capabilitypack.Pack{ID: "matty", Version: "1.0.0", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "ask-matt", Source: "skills/ask-matt"},
		{Kind: "instruction", ID: "matty-guidance", Source: "instructions/matty.md"},
	}}
	adapter := NewActivationAdapter(bundle, filepath.Join(root, ".agents", "skills"), config, prompt)
	observed, err := adapter.InspectActivation(context.Background(), pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Projections) != 3 {
		t.Fatalf("projections = %+v", observed.Projections)
	}
	var actions []capabilitypack.ProjectionAction
	for _, projection := range observed.Projections {
		actions = append(actions, projection.Action)
	}
	if actions[0].Kind != capabilitypack.ActionOpenCodeSkillLink || actions[1].Kind != capabilitypack.ActionOpenCodeInstructionFile || actions[2].Kind != capabilitypack.ActionOpenCodeConfigReference {
		t.Fatalf("OpenCode action kinds = %+v", actions)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	verified, err := adapter.InspectActivation(context.Background(), pack)
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range verified.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("not converged: %+v", projection)
		}
	}
	updated, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"// keep OpenCode syntax", `"model": "anthropic/test"`, `"CONTRIBUTING.md"`, prompt} {
		if !strings.Contains(string(updated), want) {
			t.Fatalf("config lost %q:\n%s", want, updated)
		}
	}
	promptData, err := os.ReadFile(prompt)
	if err != nil || string(promptData) != "OpenCode Matty guidance\n" {
		t.Fatalf("prompt=%q err=%v", promptData, err)
	}
}

func TestActivationAdapterInspectDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "instructions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "instructions", "matty.md"), []byte("guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "xdg", "opencode", "opencode.json")
	prompt := filepath.Join(root, "xdg", "opencode", "matty.md")
	adapter := NewActivationAdapter(bundle, filepath.Join(root, ".agents", "skills"), config, prompt)
	pack := capabilitypack.Pack{ID: "matty", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "matty-guidance", Source: "instructions/matty.md"}}}
	if _, err := adapter.InspectActivation(context.Background(), pack); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(config)); !os.IsNotExist(err) {
		t.Fatalf("inspection wrote OpenCode config: %v", err)
	}
}

func TestActivationAdapterRejectsInvalidConfigBeforeAnyProjection(t *testing.T) {
	root := t.TempDir()
	adapter := NewActivationAdapter(root, filepath.Join(root, "skills"), filepath.Join(root, "opencode.json"), filepath.Join(root, "matty.md"))
	actions := []capabilitypack.ProjectionAction{
		{ID: "instruction:matty-guidance", Kind: capabilitypack.ActionOpenCodeInstructionFile, Target: filepath.Join(root, "matty.md"), Content: "guidance\n"},
		{ID: "opencode-instruction-reference:matty-guidance", Kind: capabilitypack.ActionOpenCodeConfigReference, Target: filepath.Join(root, "opencode.json"), Content: `{invalid`},
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err == nil {
		t.Fatal("invalid OpenCode projection was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "matty.md")); !os.IsNotExist(err) {
		t.Fatalf("validation failure wrote prompt: %v", err)
	}
}
