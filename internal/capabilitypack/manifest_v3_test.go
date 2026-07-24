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

func TestManifestV3AcceptsMultiAuthorityOptionalModeDeclarations(t *testing.T) {
	bundle, path, manifest := writeManifestV3Fixture(t)
	addMultiAuthorityOptionalMode(manifest)
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err := LoadPortableManifest(path, bundle)
	if err != nil {
		t.Fatal(err)
	}
	records := pack.Resources[0].Bindings[0].AgentAuthority.Authorities
	if len(records) != 2 ||
		records[0].Declarations[0] != "optional-mode:browser-network:browser" ||
		records[1].Declarations[0] != "optional-mode:browser-network:network" {
		t.Fatalf("optional-mode authority records = %#v", records)
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
		{"missing declaration", "declaration \"tool:browser\" is missing", func(m map[string]any) {
			a := resource(m, "agent", "helper")["bindings"].([]any)[0].(map[string]any)["agent_authority"].(map[string]any)
			record := a["authorities"].([]any)[0].(map[string]any)
			record["declarations"] = []any{"permission:browser"}
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

func TestManifestV3FailsClosedOnInvalidAgentAuthorityContracts(t *testing.T) {
	record := func(a map[string]any) map[string]any {
		return a["authorities"].([]any)[0].(map[string]any)
	}
	authority := func(m map[string]any) map[string]any {
		return resource(m, "agent", "helper")["bindings"].([]any)[0].(map[string]any)["agent_authority"].(map[string]any)
	}
	tests := []struct {
		name, want string
		edit       func(map[string]any)
	}{
		{"missing permission mode", "permission_mode must be default", func(m map[string]any) { delete(authority(m), "permission_mode") }},
		{"null permission mode", "permission_mode must be default", func(m map[string]any) { authority(m)["permission_mode"] = nil }},
		{"unknown permission mode", "permission_mode must be default", func(m map[string]any) { authority(m)["permission_mode"] = "bypassPermissions" }},
		{"missing authorities", "authorities is a required non-null array", func(m map[string]any) { delete(authority(m), "authorities") }},
		{"null authorities", "authorities is a required non-null array", func(m map[string]any) { authority(m)["authorities"] = nil }},
		{"duplicate authority", "sorted by portable without duplicates", func(m map[string]any) {
			a := authority(m)
			a["authorities"] = append(a["authorities"].([]any), record(a))
		}},
		{"unsorted authorities", "sorted by portable without duplicates", func(m map[string]any) {
			a := authority(m)
			agent := resource(m, "agent", "helper")
			agent["tools"], agent["permissions"] = []any{"browser", "network"}, []any{"browser", "network"}
			a["authorities"] = []any{
				map[string]any{"portable": "network", "declarations": []any{"permission:network", "tool:network"}, "outcome": "native", "claude_tools": []any{"WebFetch", "WebSearch"}, "fallback": "none"},
				record(a),
			}
		}},
		{"unknown portable", "portable authority", func(m map[string]any) { record(authority(m))["portable"] = "future" }},
		{"missing declarations", "required non-null arrays", func(m map[string]any) { delete(record(authority(m)), "declarations") }},
		{"null declarations", "required non-null arrays", func(m map[string]any) { record(authority(m))["declarations"] = nil }},
		{"empty undeclared record", "has no declarations", func(m map[string]any) {
			a := authority(m)
			a["authorities"] = []any{
				map[string]any{"portable": "browser", "declarations": []any{"permission:browser", "tool:browser"}, "outcome": "fallback", "claude_tools": []any{}, "fallback": "Continue without browser research"},
				map[string]any{"portable": "network", "declarations": []any{}, "outcome": "fallback", "claude_tools": []any{}, "fallback": "Continue without browser research"},
			}
		}},
		{"unsorted declarations", "declarations must be sorted", func(m map[string]any) {
			record(authority(m))["declarations"] = []any{"tool:browser", "permission:browser"}
		}},
		{"duplicate declarations", "declarations must be sorted", func(m map[string]any) {
			record(authority(m))["declarations"] = []any{"permission:browser", "permission:browser", "tool:browser"}
		}},
		{"dangling declaration", "dangling or unknown", func(m map[string]any) {
			record(authority(m))["declarations"] = []any{"permission:browser", "tool:browser", "tool:future"}
		}},
		{"missing optional-mode authority declaration", "optional-mode:browser-network:network\" is missing", func(m map[string]any) {
			addMultiAuthorityOptionalMode(m)
			a := authority(m)
			a["authorities"] = a["authorities"].([]any)[:1]
		}},
		{"dangling optional-mode authority declaration", "dangling or unknown", func(m map[string]any) {
			r := record(authority(m))
			r["declarations"] = []any{"optional-mode:missing:browser", "permission:browser", "tool:browser"}
		}},
		{"null claude tools", "required non-null arrays", func(m map[string]any) { record(authority(m))["claude_tools"] = nil }},
		{"unsorted claude tools", "claude_tools must be sorted", func(m map[string]any) {
			record(authority(m))["claude_tools"] = []any{"WebSearch", "Read"}
		}},
		{"duplicate claude tools", "claude_tools must be sorted", func(m map[string]any) {
			record(authority(m))["claude_tools"] = []any{"WebSearch", "WebSearch"}
		}},
		{"unknown claude tool", "Claude tool", func(m map[string]any) { record(authority(m))["claude_tools"] = []any{"Future"} }},
		{"browser native WebSearch", "does not allow native outcome", func(m map[string]any) {
			r := record(authority(m))
			r["outcome"], r["claude_tools"], r["fallback"] = "native", []any{"WebSearch"}, "none"
		}},
		{"process Read", "incompatible with portable authority", func(m map[string]any) {
			r := record(authority(m))
			r["portable"], r["declarations"] = "process", []any{"permission:process", "tool:process"}
			agent := resource(m, "agent", "helper")
			agent["tools"], agent["permissions"] = []any{"process"}, []any{"process"}
			r["outcome"], r["claude_tools"], r["fallback"] = "native", []any{"Read"}, "none"
		}},
		{"commit native", "does not allow native outcome", func(m map[string]any) {
			setSingleAuthority(m, "commit", "native", []any{"Bash"}, "none")
		}},
		{"deploy native", "does not allow native outcome", func(m map[string]any) {
			setSingleAuthority(m, "deploy", "native", []any{"Bash"}, "none")
		}},
		{"process guarded", "does not allow guarded outcome", func(m map[string]any) {
			setSingleAuthority(m, "process", "guarded", []any{"Bash"}, "none")
		}},
		{"network guarded", "does not allow guarded outcome", func(m map[string]any) {
			setSingleAuthority(m, "network", "guarded", []any{"WebSearch"}, "none")
		}},
		{"network native missing WebSearch", "requires exact claude_tools", func(m map[string]any) {
			setSingleAuthority(m, "network", "native", []any{"WebFetch"}, "none")
		}},
		{"network altered declared fallback", "fallback must exactly match declared fallback", func(m map[string]any) {
			addMultiAuthorityOptionalMode(m)
			records := authority(m)["authorities"].([]any)
			records[1].(map[string]any)["fallback"] = "Altered fallback"
		}},
		{"process altered declared fallback", "fallback must exactly match declared fallback", func(m map[string]any) {
			setSingleAuthority(m, "process", "native", []any{"Bash"}, "Altered fallback")
			addOptionalModeForPortable(m, "process-mode", "process", "Use the declared process fallback")
		}},
		{"conflicting declared fallbacks", "conflicting declared fallbacks", func(m map[string]any) {
			setSingleAuthority(m, "process", "native", []any{"Bash"}, "First fallback")
			m["contract"].(map[string]any)["optional_modes"] = []any{
				map[string]any{"id": "first-mode", "authorities": []any{"process"}, "fallback": "First fallback"},
				map[string]any{"id": "second-mode", "authorities": []any{"process"}, "fallback": "Second fallback"},
			}
		}},
		{"filesystem fallback", "does not allow fallback outcome", func(m map[string]any) {
			setSingleAuthority(m, "filesystem", "fallback", []any{}, "Continue without filesystem access")
		}},
		{"subagent native", "does not allow native outcome", func(m map[string]any) {
			setSingleAuthority(m, "subagent", "native", []any{}, "none")
		}},
		{"unknown outcome", "outcome", func(m map[string]any) { record(authority(m))["outcome"] = "magic" }},
		{"native without tools", "native outcome", func(m map[string]any) {
			r := record(authority(m))
			r["outcome"], r["claude_tools"] = "native", []any{}
		}},
		{"fallback with tools", "requires exact claude_tools", func(m map[string]any) {
			r := record(authority(m))
			r["portable"], r["declarations"], r["claude_tools"] = "network", []any{"permission:network", "tool:network"}, []any{"WebFetch", "WebSearch"}
			agent := resource(m, "agent", "helper")
			agent["tools"], agent["permissions"] = []any{"network"}, []any{"network"}
		}},
		{"fallback without fallback", "fallback outcome", func(m map[string]any) {
			r := record(authority(m))
			r["outcome"], r["claude_tools"], r["fallback"] = "fallback", []any{}, "none"
		}},
		{"guarded without tools", "guarded outcome", func(m map[string]any) {
			r := record(authority(m))
			r["outcome"], r["claude_tools"] = "guarded", []any{}
		}},
		{"guarded with fallback", "guarded outcome", func(m map[string]any) {
			r := record(authority(m))
			r["outcome"], r["fallback"] = "guarded", "Ask first"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle, path, manifest := writeManifestV3Fixture(t)
			tt.edit(manifest)
			data, _ := json.Marshal(manifest)
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

func setSingleAuthority(manifest map[string]any, portable, outcome string, claudeTools []any, fallback string) {
	agent := resource(manifest, "agent", "helper")
	agent["tools"], agent["permissions"] = []any{portable}, []any{portable}
	record := agent["bindings"].([]any)[0].(map[string]any)["agent_authority"].(map[string]any)["authorities"].([]any)[0].(map[string]any)
	record["portable"] = portable
	record["declarations"] = []any{"permission:" + portable, "tool:" + portable}
	record["outcome"], record["claude_tools"], record["fallback"] = outcome, claudeTools, fallback
}

func addOptionalModeForPortable(manifest map[string]any, id, portable, fallback string) {
	manifest["contract"].(map[string]any)["optional_modes"] = []any{map[string]any{
		"id": id, "authorities": []any{portable}, "fallback": fallback,
	}}
	record := resource(manifest, "agent", "helper")["bindings"].([]any)[0].(map[string]any)["agent_authority"].(map[string]any)["authorities"].([]any)[0].(map[string]any)
	record["declarations"] = []any{"optional-mode:" + id + ":" + portable, "permission:" + portable, "tool:" + portable}
}

func TestManifestV3AcceptsNativeAuthorityWithNonNoneFallback(t *testing.T) {
	bundle, path, manifest := writeManifestV3Fixture(t)
	agent := resource(manifest, "agent", "helper")
	agent["tools"], agent["permissions"] = []any{"network"}, []any{"network"}
	record := agent["bindings"].([]any)[0].(map[string]any)["agent_authority"].(map[string]any)["authorities"].([]any)[0].(map[string]any)
	record["portable"], record["declarations"] = "network", []any{"permission:network", "tool:network"}
	record["outcome"], record["claude_tools"], record["fallback"] = "native", []any{"WebFetch", "WebSearch"}, "Use configured network integration"
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPortableManifest(path, bundle); err != nil {
		t.Fatal(err)
	}
}

func addMultiAuthorityOptionalMode(manifest map[string]any) {
	contract := manifest["contract"].(map[string]any)
	contract["optional_modes"] = []any{map[string]any{
		"id":          "browser-network",
		"authorities": []any{"browser", "network"},
		"fallback":    "Continue without browser research",
	}}
	authority := resource(manifest, "agent", "helper")["bindings"].([]any)[0].(map[string]any)["agent_authority"].(map[string]any)
	browser := authority["authorities"].([]any)[0].(map[string]any)
	browser["declarations"] = []any{"optional-mode:browser-network:browser", "permission:browser", "tool:browser"}
	authority["authorities"] = []any{
		browser,
		map[string]any{
			"portable":     "network",
			"declarations": []any{"optional-mode:browser-network:network"},
			"outcome":      "native",
			"claude_tools": []any{"WebFetch", "WebSearch"},
			"fallback":     "Continue without browser research",
		},
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
		map[string]any{"kind": "agent", "id": "helper", "source": "agents/helper.md", "description": "Helps", "mode": "subagent", "tools": []any{"browser"}, "permissions": []any{"browser"}, "requires": []any{}, "bindings": []any{map[string]any{"surface": "claude", "projection": "agent", "name": "helper", "invocation": "@helper", "mode": "native", "sharing": "exclusive", "agent_authority": map[string]any{"permission_mode": "default", "authorities": []any{map[string]any{"portable": "browser", "declarations": []any{"permission:browser", "tool:browser"}, "outcome": "fallback", "claude_tools": []any{}, "fallback": "Continue without browser research"}}}}}, "surface_exclusions": []any{exclusion("codex"), exclusion("opencode")}},
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
