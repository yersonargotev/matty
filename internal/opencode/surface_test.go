package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestSurfaceAdapterAppliesHostSpecificProjectionsAndPreservesJSONC(t *testing.T) {
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
	if err := os.WriteFile(instruction, []byte("OpenCode Packy guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "xdg", "opencode", "opencode.json")
	prompt := filepath.Join(root, "xdg", "opencode", "packy.md")
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
	adapter := NewSurfaceAdapter(bundle, filepath.Join(root, ".agents", "skills"), config, prompt)
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
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
	verified, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
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
	if err != nil || string(promptData) != "OpenCode Packy guidance\n" {
		t.Fatalf("prompt=%q err=%v", promptData, err)
	}
}

func TestSurfaceAdapterComposesMultipleInstructionReferences(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.md", "two.md"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(root, "opencode.json")
	adapter := NewSurfaceAdapter(bundle, filepath.Join(root, "skills"), config, filepath.Join(root, "packy.md"))
	pack := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "one", Source: "one.md"}, {Kind: "instruction", ID: "two", Source: "two.md"}}}
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	var actions []capabilitypack.ProjectionAction
	for _, projection := range observed.Projections {
		actions = append(actions, projection.Action)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	verified, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range verified.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("instruction projection did not converge: %+v", projection)
		}
	}
}

func TestPriorTransitionInspectionPreservesUnmanagedOpenCodeConfiguration(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	source := filepath.Join(bundle, "guide.md")
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "opencode.json")
	prompt := filepath.Join(root, "guide.md")
	if err := os.WriteFile(config, []byte("// keep\n{\n  \"model\": \"test\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapter(bundle, filepath.Join(root, "skills"), config, prompt)
	active := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md"}}}
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: active})
	if err != nil {
		t.Fatal(err)
	}
	var actions []capabilitypack.ProjectionAction
	for _, projection := range observed.Projections {
		actions = append(actions, projection.Action)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: active, Desired: capabilitypack.Pack{ID: "desired"}, ResolvedExecutables: nil})
	if err != nil {
		t.Fatal(err)
	}
	actions = nil
	for _, projection := range removal.Projections {
		actions = append(actions, projection.Action)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "// keep") || !strings.Contains(string(data), `"model": "test"`) || strings.Contains(string(data), prompt) {
		t.Fatalf("config = %s", data)
	}
	if _, err := os.Stat(prompt); !os.IsNotExist(err) {
		t.Fatalf("instruction remains: %v", err)
	}
}

func TestSurfaceAdapterInspectDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "instructions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "instructions", "matty.md"), []byte("guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "xdg", "opencode", "opencode.json")
	prompt := filepath.Join(root, "xdg", "opencode", "packy.md")
	adapter := NewSurfaceAdapter(bundle, filepath.Join(root, ".agents", "skills"), config, prompt)
	pack := capabilitypack.Pack{ID: "matty", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "matty-guidance", Source: "instructions/matty.md"}}}
	if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(config)); !os.IsNotExist(err) {
		t.Fatalf("inspection wrote OpenCode config: %v", err)
	}
}

func TestEngramProjectionIsOpenCodeSpecificAndPreservesJSONC(t *testing.T) {
	root := t.TempDir()
	instructions := filepath.Join(root, "instructions")
	if err := os.MkdirAll(instructions, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instructions, "engram-memory.md"), []byte("remember safely\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "opencode.json")
	prompt := filepath.Join(root, "engram-memory.md")
	existing := `// keep OpenCode syntax
{
  "model": "anthropic/test",
  "mcp": {"jira": {"type": "remote", "url": "https://jira.example/mcp",},},
  "instructions": ["CONTRIBUTING.md",],
}
`
	if err := os.WriteFile(config, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "engram", Version: "1.0.0", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: "engram-memory", Source: "instructions/engram-memory.md"},
		{Kind: "mcp_server", ID: "engram", Command: "engram", Args: []string{"mcp", "--tools=agent"}},
	}}
	adapter := NewSurfaceAdapter(root, filepath.Join(root, ".agents", "skills"), config, prompt)
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Projections) != 3 {
		t.Fatalf("projections = %#v", observed.Projections)
	}
	var actions []capabilitypack.ProjectionAction
	for _, projection := range observed.Projections {
		actions = append(actions, projection.Action)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"// keep OpenCode syntax", `"model": "anthropic/test"`, `"jira"`, `"engram"`, prompt} {
		if !strings.Contains(string(updated), want) {
			t.Fatalf("OpenCode config lost/projected %q:\n%s", want, updated)
		}
	}
	if _, err := os.Stat(prompt); err != nil {
		t.Fatalf("Engram instruction file missing: %v", err)
	}
	verified, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
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

func TestSurfaceAdapterRejectsInvalidConfigBeforeAnyProjection(t *testing.T) {
	root := t.TempDir()
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "skills"), filepath.Join(root, "opencode.json"), filepath.Join(root, "packy.md"))
	actions := []capabilitypack.ProjectionAction{
		{ID: "instruction:matty-guidance", Kind: capabilitypack.ActionOpenCodeInstructionFile, Target: filepath.Join(root, "packy.md"), Content: "guidance\n"},
		{ID: "opencode-instruction-reference:matty-guidance", Kind: capabilitypack.ActionOpenCodeConfigReference, Target: filepath.Join(root, "opencode.json"), Content: `{invalid`},
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err == nil {
		t.Fatal("invalid OpenCode projection was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "packy.md")); !os.IsNotExist(err) {
		t.Fatalf("validation failure wrote prompt: %v", err)
	}
}

func TestOwnershipResidualInspectionDiscoversObsoleteOwnedOpenCodeProjectionsAndPreservesUnmanagedConfig(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "guide.md"), []byte("managed guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "opencode.json")
	prompt := filepath.Join(root, "guide.md")
	if err := os.WriteFile(config, []byte("// keep comment\n{\n  \"model\": \"test\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapter(bundle, filepath.Join(root, "skills"), config, prompt)
	pack := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md"}}}
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	var actions []capabilitypack.ProjectionAction
	for _, projection := range observed.Projections {
		actions = append(actions, projection.Action)
	}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	verified, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	owners := make([]capabilitypack.ProjectionOwnership, 0, len(verified.Projections))
	for _, projection := range verified.Projections {
		owners = append(owners, capabilitypack.ProjectionOwnership{ID: projection.ID, Fingerprint: projection.ObservedFingerprint, Contributors: []string{"app"}})
	}
	reconcile, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: capabilitypack.Pack{ID: "desired"}, ResidualOwnership: owners, ResolvedExecutables: nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(reconcile.Projections) != 2 {
		t.Fatalf("ownership residual projections = %+v", reconcile.Projections)
	}
	for _, projection := range reconcile.Projections {
		if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{projection.Action}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(config)
	if err != nil || !strings.Contains(string(got), "// keep comment") || !strings.Contains(string(got), `"model": "test"`) || strings.Contains(string(got), prompt) {
		t.Fatalf("config = %q err=%v", got, err)
	}
	if _, err := os.Stat(prompt); !os.IsNotExist(err) {
		t.Fatalf("obsolete instruction remains: %v", err)
	}
}
