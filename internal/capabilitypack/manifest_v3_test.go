package capabilitypack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestV3DecodesExplicitSurfaceOutcomesAndTypedClaudeBindings(t *testing.T) {
	bundle, path, manifest := writeManifestV3Fixture(t)
	pack, err := LoadPortableManifest(path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Surfaces) != 3 || pack.Surfaces[0] != SurfaceClaude {
		t.Fatalf("v3 surfaces were not retained: %#v", pack.Surfaces)
	}
	if pack.Resources[0].Bindings[0].AgentAuthority == nil || pack.Resources[2].Bindings[0].Hook == nil {
		t.Fatalf("typed Claude bindings were not retained: %#v", pack.Resources)
	}
	if got := pack.Resources[1].SurfaceExclusions[0].Code; got != "unsupported-instruction" {
		t.Fatalf("exclusion code = %q", got)
	}
	_ = manifest
}

func TestDiscoverAcceptsManifestV3ClaudeSurface(t *testing.T) {
	bundle, path, _ := writeManifestV3Fixture(t)
	target := filepath.Join(bundle, "packs", "example", "pack.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	catalog, err := discoverCatalog(bundle, []catalogEntry{{ID: "example", Description: "Example", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}}})
	if err != nil {
		t.Fatal(err)
	}
	pack, err := catalog.Show("example")
	if err != nil {
		t.Fatal(err)
	}
	if got := pack.Surfaces; len(got) != 3 || got[0] != SurfaceClaude {
		t.Fatalf("discovered surfaces = %#v", got)
	}
}

func TestManifestV3FailsClosedOnInvalidSurfaceContracts(t *testing.T) {
	tests := []struct {
		name, want string
		edit       func(map[string]any)
	}{
		{"missing surfaces", "surfaces is a required", func(m map[string]any) { delete(m, "surfaces") }},
		{"null surfaces", "surfaces is a required", func(m map[string]any) { m["surfaces"] = nil }},
		{"unsorted surfaces", "surfaces must be a sorted set", func(m map[string]any) { m["surfaces"] = []any{"codex", "claude", "opencode"} }},
		{"unknown surface", "unsupported CLI surface", func(m map[string]any) { m["surfaces"] = []any{"claude", "future"} }},
		{"missing outcome", "missing binding-or-exclusion", func(m map[string]any) {
			r := resource(m, "instruction", "guide")
			x := r["surface_exclusions"].([]any)
			r["surface_exclusions"] = x[:2]
		}},
		{"duplicate outcome", "duplicate or contradictory", func(m map[string]any) {
			r := resource(m, "instruction", "guide")
			r["bindings"] = []any{claudeBinding("instruction", "guide", "guide")}
		}},
		{"dangling outcome", "undeclared surface", func(m map[string]any) { m["surfaces"] = []any{"claude", "codex"} }},
		{"unsorted exclusions", "surface_exclusions must be sorted", func(m map[string]any) {
			r := resource(m, "instruction", "guide")
			x := r["surface_exclusions"].([]any)
			x[0], x[1] = x[1], x[0]
		}},
		{"unknown hook event", "hook type, event", func(m map[string]any) {
			hook := resource(m, "lifecycle", "memory")["bindings"].([]any)[0].(map[string]any)["hook"].(map[string]any)
			hook["event"] = "Invented"
		}},
		{"missing hook matcher", "hook matcher is required", func(m map[string]any) {
			hook := resource(m, "lifecycle", "memory")["bindings"].([]any)[0].(map[string]any)["hook"].(map[string]any)
			delete(hook, "matcher")
		}},
		{"missing hook blocking", "hook blocking is required", func(m map[string]any) {
			hook := resource(m, "lifecycle", "memory")["bindings"].([]any)[0].(map[string]any)["hook"].(map[string]any)
			delete(hook, "blocking")
		}},
		{"missing translation", "agent_authority tools", func(m map[string]any) {
			a := resource(m, "agent", "helper")["bindings"].([]any)[0].(map[string]any)["agent_authority"].(map[string]any)
			a["tools"] = []any{map[string]any{"portable": "browser"}}
		}},
		{"field from another resource kind", "unknown field", func(m map[string]any) {
			resource(m, "instruction", "guide")["command"] = "not-legal-on-an-instruction"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle, path, m := writeManifestV3Fixture(t)
			tt.edit(m)
			data, _ := json.Marshal(m)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadPortableManifest(path, bundle)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func writeManifestV3Fixture(t *testing.T) (string, string, map[string]any) {
	t.Helper()
	bundle := t.TempDir()
	for p := range map[string]bool{"agents/helper.md": true, "instructions/guide.md": true} {
		target := filepath.Join(bundle, p)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(p), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	exclusion := func(surface string) any {
		return map[string]any{"surface": surface, "mode": "optional", "code": "unsupported-" + surface, "reason": "not projected on this surface"}
	}
	manifest := map[string]any{"schema_version": 3, "id": "example", "version": "3.0.0", "surfaces": []any{"claude", "codex", "opencode"}, "provides": []any{}, "requires": map[string]any{"capabilities": []any{}, "tools": []any{}}, "conflicts": []any{}, "contract": map[string]any{"exclusions": []any{}, "optional_modes": []any{}}, "resources": []any{
		map[string]any{"kind": "agent", "id": "helper", "source": "agents/helper.md", "description": "Helps", "mode": "subagent", "tools": []any{"browser"}, "permissions": []any{"browser"}, "requires": []any{}, "bindings": []any{map[string]any{"surface": "claude", "projection": "agent", "name": "helper", "invocation": "@helper", "mode": "native", "sharing": "exclusive", "agent_authority": map[string]any{"tools": []any{map[string]any{"portable": "browser", "claude": "WebSearch"}}, "permissions": []any{map[string]any{"portable": "browser", "claude": "WebSearch"}}}}}, "surface_exclusions": []any{exclusion("codex"), exclusion("opencode")}},
		map[string]any{"kind": "instruction", "id": "guide", "source": "instructions/guide.md", "requires": []any{}, "bindings": []any{}, "surface_exclusions": []any{map[string]any{"surface": "claude", "mode": "optional", "code": "unsupported-instruction", "reason": "test"}, exclusion("codex"), exclusion("opencode")}},
		map[string]any{"kind": "lifecycle", "id": "memory", "requires": []any{}, "bindings": []any{map[string]any{"surface": "claude", "projection": "command_hook", "name": "memory", "invocation": "SessionStart", "mode": "native", "sharing": "exclusive", "hook": map[string]any{"type": "command", "event": "SessionStart", "matcher": "", "command": "engram", "args": []any{"session"}, "timeout_seconds": 5, "blocking": true, "failure": "block", "authorities": []any{"process"}}}}, "surface_exclusions": []any{exclusion("codex"), exclusion("opencode")}},
	}}
	data, _ := json.Marshal(manifest)
	path := filepath.Join(bundle, "pack.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return bundle, path, manifest
}

func claudeBinding(projection, name, invocation string) any {
	return map[string]any{"surface": "claude", "projection": projection, "name": name, "invocation": invocation, "mode": "native", "sharing": "exclusive"}
}
