package capabilitypack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverLoadsManifestV2Contract(t *testing.T) {
	bundle, entries := writeManifestV2Fixture(t)
	catalog, err := discoverCatalog(bundle, entries)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := catalog.Show("addy")
	if err != nil {
		t.Fatal(err)
	}
	if pack.Contract.OptionalModes[0].ID != "browser-research" || pack.Resources[4].Kind != "skill" {
		t.Fatalf("manifest v2 contract was not retained: %#v", pack)
	}
	command := pack.Resources[2]
	if command.Arguments != (CommandArguments{Mode: "freeform", Placeholder: "$ARGUMENTS"}) || command.Bindings[0].Projection != "skill" {
		t.Fatalf("command contract = %#v", command)
	}
}

func TestDiscoverRejectsInvalidManifestV2Contracts(t *testing.T) {
	tests := []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{"missing contract", func(m map[string]any) { delete(m, "contract") }, "contract is required"},
		{"unsorted resources", func(m map[string]any) {
			r := m["resources"].([]any)
			r[0], r[1] = r[1], r[0]
		}, "sorted by kind and id"},
		{"dangling dependency", func(m map[string]any) {
			resource(m, "command", "refine-idea")["requires"] = []any{"skill:missing"}
		}, "does not exist"},
		{"dependency cycle", func(m map[string]any) {
			resource(m, "asset", "shared-reference")["requires"] = []any{"asset:shared-reference"}
		}, "dependency cycle"},
		{"notice dependency", func(m map[string]any) {
			resource(m, "skill", "idea-refine")["requires"] = []any{"notice:license"}
		}, "may not target notice"},
		{"missing surface binding", func(m map[string]any) {
			bindings := resource(m, "skill", "idea-refine")["bindings"].([]any)
			resource(m, "skill", "idea-refine")["bindings"] = bindings[:1]
		}, "exactly one binding"},
		{"degraded without reason", func(m map[string]any) {
			binding := resource(m, "command", "refine-idea")["bindings"].([]any)[0].(map[string]any)
			delete(binding, "degradation")
		}, "degradation is required"},
		{"native with reason", func(m map[string]any) {
			binding := resource(m, "command", "refine-idea")["bindings"].([]any)[1].(map[string]any)
			binding["degradation"] = "not-degraded"
		}, "degradation is forbidden"},
		{"native with null reason", func(m map[string]any) {
			binding := resource(m, "skill", "idea-refine")["bindings"].([]any)[0].(map[string]any)
			binding["degradation"] = nil
		}, "degradation is forbidden"},
		{"none arguments with null placeholder", func(m map[string]any) {
			arguments := resource(m, "command", "refine-idea")["arguments"].(map[string]any)
			arguments["mode"] = "none"
			arguments["placeholder"] = nil
		}, "none arguments forbid placeholder"},
		{"degraded skill", func(m map[string]any) {
			binding := resource(m, "skill", "idea-refine")["bindings"].([]any)[0].(map[string]any)
			binding["mode"] = "degraded"
			binding["degradation"] = "unsupported-skill-fallback"
		}, "skill bindings must be native"},
		{"degraded agent", func(m map[string]any) {
			binding := resource(m, "agent", "idea-coach")["bindings"].([]any)[0].(map[string]any)
			binding["mode"] = "degraded"
			binding["degradation"] = "unsupported-agent-fallback"
		}, "agent bindings must be native"},
		{"unsorted authorities", func(m map[string]any) {
			mode := m["contract"].(map[string]any)["optional_modes"].([]any)[0].(map[string]any)
			mode["authorities"] = []any{"network", "browser"}
		}, "authorities must be sorted"},
		{"exclusion overlaps source", func(m map[string]any) {
			exclusion := m["contract"].(map[string]any)["exclusions"].([]any)[0].(map[string]any)
			exclusion["source_paths"] = []any{"content/skills/idea-refine"}
		}, "overlaps selected resource"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle, entries := writeManifestV2Fixture(t)
			path := filepath.Join(bundle, "packs", "addy", "pack.json")
			var manifest map[string]any
			data, err := os.ReadFile(path)
			if err != nil || json.Unmarshal(data, &manifest) != nil {
				t.Fatal(err)
			}
			tt.edit(manifest)
			data, _ = json.Marshal(manifest)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = discoverCatalog(bundle, entries)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func writeManifestV2Fixture(t *testing.T) (string, []catalogEntry) {
	t.Helper()
	bundle := filepath.Join(t.TempDir(), "bundle")
	for path, data := range map[string]string{
		"content/agents/idea-coach.md":              "agent",
		"content/commands/refine-idea.md":           "command",
		"content/references/shared.md":              "asset",
		"content/notices/MIT.txt":                   "MIT",
		"content/skills/idea-refine/SKILL.md":       "skill",
		"content/skills/idea-refine/idea-refine.sh": "#!/bin/sh\n",
	} {
		target := filepath.Join(bundle, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := map[string]any{
		"schema_version": 2,
		"id":             "addy", "version": "1.0.0",
		"provides":  []any{"workflow:idea-refine"},
		"requires":  map[string]any{"capabilities": []any{}, "tools": []any{}},
		"conflicts": []any{},
		"contract": map[string]any{
			"exclusions":     []any{map[string]any{"id": "upstream-hooks", "source_paths": []any{"hooks/pre-commit"}, "reason": "hooks are inert"}},
			"optional_modes": []any{map[string]any{"id": "browser-research", "authorities": []any{"browser", "network"}, "fallback": "continue from supplied evidence"}},
		},
		"resources": []any{
			map[string]any{"kind": "agent", "id": "idea-coach", "source": "content/agents/idea-coach.md", "description": "Coaches idea refinement", "mode": "subagent", "tools": []any{"browser"}, "permissions": []any{"browser", "network"}, "requires": []any{"skill:idea-refine"}, "bindings": nativeBindings("agent", "idea-coach", "@idea-coach")},
			map[string]any{"kind": "asset", "id": "shared-reference", "source": "content/references/shared.md", "requires": []any{}},
			map[string]any{"kind": "command", "id": "refine-idea", "source": "content/commands/refine-idea.md", "arguments": map[string]any{"mode": "freeform", "placeholder": "$ARGUMENTS"}, "requires": []any{"agent:idea-coach", "asset:shared-reference", "skill:idea-refine"}, "bindings": []any{
				map[string]any{"surface": "codex", "projection": "skill", "name": "refine-idea", "invocation": "$refine-idea", "mode": "degraded", "degradation": "codex-command-as-workflow-skill", "sharing": "exclusive"},
				map[string]any{"surface": "opencode", "projection": "command", "name": "refine-idea", "invocation": "/refine-idea", "mode": "native", "sharing": "exclusive"},
			}},
			map[string]any{"kind": "notice", "id": "license", "source": "content/notices/MIT.txt", "license": "MIT", "attribution": "Copyright Addy contributors", "requires": []any{}},
			map[string]any{"kind": "skill", "id": "idea-refine", "source": "content/skills/idea-refine", "requires": []any{"asset:shared-reference"}, "bindings": nativeBindings("skill", "idea-refine", "$idea-refine")},
		},
	}
	data, _ := json.Marshal(manifest)
	path := filepath.Join(bundle, "packs", "addy", "pack.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return bundle, []catalogEntry{{ID: "addy", Description: "Addy", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}}}
}

func nativeBindings(projection, name, invocation string) []any {
	return []any{
		map[string]any{"surface": "codex", "projection": projection, "name": name, "invocation": invocation, "mode": "native", "sharing": "exclusive"},
		map[string]any{"surface": "opencode", "projection": projection, "name": name, "invocation": invocation, "mode": "native", "sharing": "exclusive"},
	}
}

func resource(manifest map[string]any, kind, id string) map[string]any {
	for _, value := range manifest["resources"].([]any) {
		candidate := value.(map[string]any)
		if candidate["kind"] == kind && candidate["id"] == id {
			return candidate
		}
	}
	panic("resource not found")
}
