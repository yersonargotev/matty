package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/matty/internal/capabilitypack"
)

func TestEngramCodexSetupContractIsObservedWithoutCompetingLocalWrites(t *testing.T) {
	root := t.TempDir()
	prompt := filepath.Join(root, ".codex", "AGENTS.md")
	config := filepath.Join(root, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(prompt), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, []byte("# keep Codex guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	engramPath := "/opt/homebrew/bin/engram"
	instructionsFile := filepath.Join(filepath.Dir(config), "engram-instructions.md")
	compactFile := filepath.Join(filepath.Dir(config), "engram-compact-prompt.md")
	instructionsGolden, err := os.ReadFile(filepath.Join("testdata", "engram-1.19.0", "engram-instructions.md"))
	if err != nil {
		t.Fatal(err)
	}
	compactGolden, err := os.ReadFile(filepath.Join("testdata", "engram-1.19.0", "engram-compact-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	configContent := `model_instructions_file = "` + instructionsFile + `"
experimental_compact_prompt_file = "` + compactFile + `"
[mcp_servers.engram]
command = "` + engramPath + `"
args = ["mcp", "--tools=agent"]

[marketplaces.engram]
last_updated = "volatile"
source_type = "git"
source = "https://github.com/Gentleman-Programming/engram.git"
ref = "main"

[plugins."engram@engram"]
enabled = true
`
	if err := os.WriteFile(config, []byte(configContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instructionsFile, instructionsGolden, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compactFile, compactGolden, 0o600); err != nil {
		t.Fatal(err)
	}
	pack := capabilitypack.Pack{ID: "engram", Version: "1.0.0", Resources: []capabilitypack.Resource{
		{Kind: "instruction", ID: "engram-memory", Source: "instructions/engram-memory.md"},
		{Kind: "mcp_server", ID: "engram", Command: "engram", Args: []string{"mcp", "--tools=agent"}},
	}}
	adapter := NewSurfaceAdapterWithConfig(root, filepath.Join(root, ".agents", "skills"), prompt, config)
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ResolvedExecutables: []capabilitypack.ExecutableResolution{{Tool: "engram", Available: true, Path: engramPath}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(observed.Projections) != 5 {
		t.Fatalf("projections = %#v", observed.Projections)
	}
	for _, projection := range observed.Projections {
		if !projection.ExternallyManaged || projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("projection did not verify: %+v", projection)
		}
	}
	for _, test := range []struct {
		name, id, config, instructions, compact string
	}{
		{"mcp args", "mcp", strings.Replace(configContent, `["mcp", "--tools=agent"]`, `["mcp"]`, 1), string(instructionsGolden), string(compactGolden)},
		{"instructions", "instructions", configContent, "incomplete", string(compactGolden)},
		{"compact prompt", "compact-prompt", configContent, string(instructionsGolden), "incomplete"},
		{"marketplace", "marketplace", strings.Replace(configContent, `ref = "main"`, `ref = "other"`, 1), string(instructionsGolden), string(compactGolden)},
		{"plugin", "plugin", strings.Replace(configContent, `enabled = true`, `enabled = false`, 1), string(instructionsGolden), string(compactGolden)},
	} {
		t.Run(test.name, func(t *testing.T) {
			for path, content := range map[string]string{config: test.config, instructionsFile: test.instructions, compactFile: test.compact} {
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			changed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ResolvedExecutables: []capabilitypack.ExecutableResolution{{Tool: "engram", Available: true, Path: engramPath}}})
			if err != nil {
				t.Fatal(err)
			}
			for _, projection := range changed.Projections {
				if strings.HasSuffix(projection.ID, ":"+test.id) && projection.ObservedFingerprint == projection.DesiredFingerprint {
					t.Fatalf("contract change was not detected: %+v", projection)
				}
			}
		})
	}
	unchangedPrompt, err := os.ReadFile(prompt)
	if err != nil || string(unchangedPrompt) != "# keep Codex guidance\n" {
		t.Fatalf("Matty competed for Engram instructions: %q err=%v", unchangedPrompt, err)
	}
}

func TestEngramCodexContractAcrossExternalProcessBoundary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	instructionsGolden, _ := filepath.Abs(filepath.Join("testdata", "engram-1.19.0", "engram-instructions.md"))
	compactGolden, _ := filepath.Abs(filepath.Join("testdata", "engram-1.19.0", "engram-compact-prompt.md"))
	engram := filepath.Join(root, "engram")
	script := `#!/bin/sh
set -eu
test "$1 $2" = "setup codex"
mkdir -p "$HOME/.codex"
cp "$ENGRAM_INSTRUCTIONS_GOLDEN" "$HOME/.codex/engram-instructions.md"
cp "$ENGRAM_COMPACT_GOLDEN" "$HOME/.codex/engram-compact-prompt.md"
cat > "$HOME/.codex/config.toml" <<EOF
model_instructions_file = "$HOME/.codex/engram-instructions.md"
experimental_compact_prompt_file = "$HOME/.codex/engram-compact-prompt.md"
[mcp_servers.engram]
command = "$0"
args = ["mcp", "--tools=agent"]
[marketplaces.engram]
last_updated = "ignored"
source_type = "git"
source = "https://github.com/Gentleman-Programming/engram.git"
ref = "main"
[plugins."engram@engram"]
enabled = true
EOF
`
	if err := os.WriteFile(engram, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(engram, "setup", "codex")
	command.Env = []string{"HOME=" + home, "ENGRAM_INSTRUCTIONS_GOLDEN=" + instructionsGolden, "ENGRAM_COMPACT_GOLDEN=" + compactGolden, "PATH=/usr/bin:/bin"}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture Engram setup: %v: %s", err, output)
	}
	adapter := NewSurfaceAdapterWithConfig(root, filepath.Join(root, "skills"), filepath.Join(codexDir, "AGENTS.md"), filepath.Join(codexDir, "config.toml"))
	pack := capabilitypack.Pack{ID: "engram", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "engram-memory"}, {Kind: "mcp_server", ID: "engram", Command: "engram", Args: []string{"mcp", "--tools=agent"}}}}
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, ResolvedExecutables: []capabilitypack.ExecutableResolution{{Tool: "engram", Available: true, Path: engram}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range observed.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("external boundary contract mismatch: %+v", projection)
		}
	}
}

func TestPriorTransitionInspectionRemovesManagedBlocksAndPreservesUnmanagedCodexConfig(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "guide.md")
	prompt := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(source, []byte("guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, []byte("unmanaged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapterWithConfig(root, filepath.Join(root, "skills"), prompt, filepath.Join(root, "config.toml"))
	active := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md"}}}
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: active})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{observed.Projections[0].Action}); err != nil {
		t.Fatal(err)
	}
	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: active, Desired: capabilitypack.Pack{ID: "desired"}, ResolvedExecutables: nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(removal.Projections) != 1 || removal.Projections[0].Action.Mode != capabilitypack.ProjectionRemoveContent {
		t.Fatalf("removals = %+v", removal.Projections)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{removal.Projections[0].Action}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(prompt)
	if err != nil || strings.TrimSpace(string(data)) != "unmanaged" {
		t.Fatalf("prompt = %q err=%v", data, err)
	}
}

func TestPriorTransitionInspectionComposesMultipleRemovalsFromOneCodexFile(t *testing.T) {
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
	adapter := NewSurfaceAdapterWithConfig(root, filepath.Join(root, "skills"), prompt, filepath.Join(root, "config.toml"))
	active := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "one", Source: "one.md"}, {Kind: "instruction", ID: "two", Source: "two.md"}}}
	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: active, Desired: capabilitypack.Pack{ID: "desired"}, ResolvedExecutables: nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(removal.Projections) != 2 {
		t.Fatalf("removals=%+v", removal.Projections)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{removal.Projections[0].Action, removal.Projections[1].Action}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(prompt)
	if strings.TrimSpace(string(got)) != "unmanaged" {
		t.Fatalf("prompt=%q", got)
	}
}

func TestSurfaceAdapterComposesMultipleInstructionWritesToOneCodexFile(t *testing.T) {
	root := t.TempDir()
	prompt := filepath.Join(root, "AGENTS.md")
	for _, name := range []string{"one.md", "two.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	adapter := NewSurfaceAdapterWithConfig(root, filepath.Join(root, "skills"), prompt, filepath.Join(root, "config.toml"))
	pack := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "one", Source: "one.md"}, {Kind: "instruction", ID: "two", Source: "two.md"}}}
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	actions := []capabilitypack.ProjectionAction{observed.Projections[0].Action, observed.Projections[1].Action}
	if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	verified, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range verified.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("instruction did not converge: %+v", projection)
		}
	}
}

func TestOwnershipResidualInspectionDiscoversObsoleteOwnedCodexProjectionAndPreservesUnmanagedContent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "guide.md")
	prompt := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(source, []byte("managed guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prompt, []byte("unmanaged guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapterWithConfig(root, filepath.Join(root, "skills"), prompt, filepath.Join(root, "config.toml"))
	pack := capabilitypack.Pack{ID: "app", Resources: []capabilitypack.Resource{{Kind: "instruction", ID: "guide", Source: "guide.md"}}}
	observed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{observed.Projections[0].Action}); err != nil {
		t.Fatal(err)
	}
	verified, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	owner := capabilitypack.ProjectionOwnership{ID: verified.Projections[0].ID, Fingerprint: verified.Projections[0].ObservedFingerprint, Contributors: []string{"app"}}
	reconcile, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: capabilitypack.Pack{ID: "desired"}, ResidualOwnership: []capabilitypack.ProjectionOwnership{owner}, ResolvedExecutables: nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(reconcile.Projections) != 1 || reconcile.Projections[0].ObservedFingerprint != owner.Fingerprint || reconcile.Projections[0].Action.Mode != capabilitypack.ProjectionRemoveContent {
		t.Fatalf("ownership residual projections = %+v", reconcile.Projections)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{reconcile.Projections[0].Action}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(prompt)
	if err != nil || strings.TrimSpace(string(got)) != "unmanaged guidance" {
		t.Fatalf("prompt = %q err=%v", got, err)
	}
}
