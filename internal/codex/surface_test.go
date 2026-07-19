package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
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
		t.Fatalf("Packy competed for Engram instructions: %q err=%v", unchangedPrompt, err)
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

func TestPortableCodexWorkflowProjectsNativeBindingsAndRequiredDegradation(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "content", "skills", "idea", "SKILL.md"), "native skill\n")
	write(filepath.Join(root, "content", "agents", "coach.md"), "Keep the agent policy exact.\n")
	write(filepath.Join(root, "content", "commands", "refine.md"), "Use the coach and read shared.md. Input: $ARGUMENTS\n")
	write(filepath.Join(root, "content", "references", "shared.md"), "dependency bytes\x00exact\n")
	write(filepath.Join(root, "content", "references", "unrelated.md"), "must stay inert\n")
	write(filepath.Join(root, "content", "notices", "MIT.txt"), "display only\n")

	codexBinding := func(projection, name, invocation, mode, degradation string) []capabilitypack.Binding {
		return []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: projection, Name: name, Invocation: invocation, Mode: mode, Degradation: degradation, Sharing: "exclusive"}}
	}
	pack := capabilitypack.Pack{ID: "synthetic", Resources: []capabilitypack.Resource{
		{Kind: "notice", ID: "license", Source: "content/notices/MIT.txt"},
		{Kind: "command", ID: "refine", Source: "content/commands/refine.md", Description: "Refine an idea", Arguments: capabilitypack.CommandArguments{Mode: "freeform", Placeholder: "$ARGUMENTS"}, Requires: []string{"agent:coach", "asset:shared", "skill:idea"}, Bindings: codexBinding("skill", "addy-refine", "$addy-refine", "degraded", "codex-command-as-workflow-skill")},
		{Kind: "asset", ID: "shared", Source: "content/references/shared.md"},
		{Kind: "asset", ID: "unrelated", Source: "content/references/unrelated.md"},
		{Kind: "agent", ID: "coach", Source: "content/agents/coach.md", Description: "Coach ideas", Mode: "subagent", Tools: []string{"browser"}, Permissions: []string{"browser", "network"}, Bindings: codexBinding("agent", "addy-coach", "@addy-coach", "native", "")},
		{Kind: "skill", ID: "idea", Source: "content/skills/idea", Bindings: codexBinding("skill", "addy-idea", "$addy-idea", "native", "")},
	}, Contract: capabilitypack.Contract{Exclusions: []capabilitypack.Exclusion{{ID: "hooks", SourcePaths: []string{"hooks/pre-commit"}, Reason: "inert"}}}}
	adapter := NewSurfaceAdapterWithConfig(root, filepath.Join(root, "home", ".agents", "skills"), filepath.Join(root, "home", ".codex", "AGENTS.md"), filepath.Join(root, "home", ".codex", "config.toml"))

	first, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision {
		t.Fatalf("revision is not deterministic: %q != %q", first.Revision, second.Revision)
	}
	wantIDs := []string{"agent:addy-coach", "asset:workflow:addy-refine:shared:shared.md", "skill:addy-idea", "workflow:addy-refine"}
	if len(first.Projections) != len(wantIDs) {
		t.Fatalf("projections = %+v", first.Projections)
	}
	for i, projection := range first.Projections {
		if projection.ID != wantIDs[i] {
			t.Fatalf("projection[%d] = %q, want %q", i, projection.ID, wantIDs[i])
		}
	}
	if first.Projections[1].Action.Kind != capabilitypack.ActionCodexAssetFile {
		t.Fatalf("asset action kind = %q", first.Projections[1].Action.Kind)
	}
	if first.Readiness.UsabilityObserved || first.Readiness.Usable {
		t.Fatalf("host usability was guessed: %+v", first.Readiness)
	}
	if err := adapter.ApplyProjections(context.Background(), projectionActions(first.Projections)); err != nil {
		t.Fatal(err)
	}
	verified, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range verified.Projections {
		if !projection.Exists || projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("projection did not verify: %+v", projection)
		}
	}
	agent, _ := os.ReadFile(filepath.Join(root, "home", ".codex", "agents", "addy-coach.toml"))
	if len(verified.OccupiedNames) != 3 || verified.OccupiedNames[0].Namespace != "agent" || verified.OccupiedNames[0].OwnerType != "packy" || verified.OccupiedNames[1].Name != "addy-idea" || verified.OccupiedNames[2].Name != "addy-refine" {
		t.Fatalf("occupied names = %+v", verified.OccupiedNames)
	}
	for _, preserved := range []string{"Keep the agent policy exact.", `mode=subagent`, `tools=[\"browser\"]`, `permissions=[\"browser\", \"network\"]`, "Preserve these constraints when executing."} {
		if !strings.Contains(string(agent), preserved) {
			t.Fatalf("agent lost %q: %s", preserved, agent)
		}
	}
	workflow, _ := os.ReadFile(filepath.Join(root, "home", ".agents", "skills", "addy-refine", "SKILL.md"))
	for _, preserved := range []string{"$ARGUMENTS", "$addy-refine", "does not provide or claim `/addy-refine`", "Use the coach and read shared.md."} {
		if !strings.Contains(string(workflow), preserved) {
			t.Fatalf("workflow lost %q: %s", preserved, workflow)
		}
	}
	asset, _ := os.ReadFile(filepath.Join(root, "home", ".agents", "skills", "addy-refine", "shared.md"))
	if string(asset) != "dependency bytes\x00exact\n" {
		t.Fatalf("asset bytes = %q", asset)
	}
	if _, err := os.Stat(filepath.Join(root, "content", "notices", "MIT.txt")); err != nil {
		t.Fatal(err)
	}
	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: pack, Desired: capabilitypack.Pack{ID: "empty"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.ApplyProjections(context.Background(), projectionActions(removal.Projections)); err != nil {
		t.Fatal(err)
	}
	for _, projection := range removal.Projections {
		if projection.Action.Mode != capabilitypack.ProjectionDeleteTarget {
			t.Fatalf("unsafe removal: %+v", projection.Action)
		}
		if _, err := os.Lstat(projection.Action.Target); !os.IsNotExist(err) {
			t.Fatalf("target was not removed: %s err=%v", projection.Action.Target, err)
		}
	}
}

func TestPortableCodexWorkflowRejectsOverlappingNativeSkillTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skill"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skill", "SKILL.md"), []byte("skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "command.md"), []byte("command"), 0o600); err != nil {
		t.Fatal(err)
	}
	binding := func(mode, degradation string) []capabilitypack.Binding {
		return []capabilitypack.Binding{{Surface: capabilitypack.SurfaceCodex, Projection: "skill", Name: "same", Invocation: "$same", Mode: mode, Degradation: degradation, Sharing: "exclusive"}}
	}
	pack := capabilitypack.Pack{ID: "collision", Resources: []capabilitypack.Resource{
		{Kind: "skill", ID: "skill", Source: "skill", Bindings: binding("native", "")},
		{Kind: "command", ID: "command", Source: "command.md", Arguments: capabilitypack.CommandArguments{Mode: "none"}, Bindings: binding("degraded", "codex-command-as-workflow-skill")},
	}}
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "home", "skills"), filepath.Join(root, "home", ".codex", "AGENTS.md"))
	if _, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack}); err == nil || !strings.Contains(err.Error(), "overlapping targets") {
		t.Fatalf("collision was not rejected: %v", err)
	}
}

func TestCodexApplyRollsBackWhenAProjectionCannotBeStaged(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "skills"), filepath.Join(root, ".codex", "AGENTS.md"))
	first := filepath.Join(root, ".codex", "agents", "first.toml")
	err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{
		{ID: "agent:first", Kind: capabilitypack.ActionCodexAgentFile, Target: first, Content: "first"},
		{ID: "agent:blocked", Kind: capabilitypack.ActionCodexAgentFile, Target: filepath.Join(blocker, "blocked.toml"), Content: "blocked"},
	})
	if err == nil {
		t.Fatal("partial failure unexpectedly succeeded")
	}
	if _, statErr := os.Stat(first); !os.IsNotExist(statErr) {
		t.Fatalf("first projection leaked after failure: %v", statErr)
	}
	got, _ := os.ReadFile(blocker)
	if string(got) != "keep" {
		t.Fatalf("blocker changed: %q", got)
	}
}

func TestCodexResidualInspectionReportsDriftBeforeOwnedRemoval(t *testing.T) {
	root := t.TempDir()
	prompt := filepath.Join(root, ".codex", "AGENTS.md")
	target := filepath.Join(root, ".codex", "agents", "coach.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "skills"), prompt)
	initial, exists, err := localprojection.FingerprintPath(target)
	if err != nil || !exists {
		t.Fatalf("fingerprint: %q %v %v", initial, exists, err)
	}
	if err := os.WriteFile(target, []byte("drifted"), 0o600); err != nil {
		t.Fatal(err)
	}
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: capabilitypack.Pack{ID: "empty"}, ResidualOwnership: []capabilitypack.ProjectionOwnership{{ID: "agent:coach", Fingerprint: initial, Contributors: []string{"pack"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 1 {
		t.Fatalf("projections = %+v", inspection.Projections)
	}
	projection := inspection.Projections[0]
	if projection.ObservedFingerprint == initial || projection.Action.Mode != capabilitypack.ProjectionDeleteTarget {
		t.Fatalf("drift was hidden: %+v", projection)
	}
}

func projectionActions(projections []capabilitypack.ObservedProjection) []capabilitypack.ProjectionAction {
	actions := make([]capabilitypack.ProjectionAction, len(projections))
	for i := range projections {
		actions[i] = projections[i].Action
	}
	return actions
}
