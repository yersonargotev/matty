package corelifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateV2WritesCanonicalSurfacesOwnershipAndAttemptWithoutEnvironmentValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	state := State{
		SchemaVersion:   SchemaVersion,
		DesiredSurfaces: []string{"codex", "opencode", "claude"},
		ClaudeOwnership: []ClaudeOwnership{{ID: "mcp:engram", Kind: "mcp", Target: "engram", Fingerprint: "sha256:x", Contributors: []string{"classic"}, Command: "engram", Args: []string{"mcp"}, EnvironmentKeys: []string{"TOKEN"}, EnvironmentFingerprint: "sha256:redacted", DeletionAuthorized: true}},
		LatestAttempt:   &LatestAttempt{Operation: Install, Outcome: AttemptVerified},
		InstallStatus:   InstallConfirmed,
	}
	if err := SaveState(path, state); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wire := string(b)
	for _, want := range []string{`"schema_version": 2`, `"desired_surfaces": [`, `"codex"`, `"opencode"`, `"claude"`, `"environment_keys"`, `"completed_effects": []`, `"not_started_effects": []`} {
		if !strings.Contains(wire, want) {
			t.Fatalf("state missing %s:\n%s", want, wire)
		}
	}
	if strings.Contains(wire, "configured_surfaces") || strings.Contains(wire, "secret-value") {
		t.Fatalf("legacy or secret data rendered:\n%s", wire)
	}
	loaded, found, err := LoadState(path)
	if err != nil || !found || loaded.SchemaVersion != 2 {
		t.Fatalf("LoadState=%+v,%v,%v", loaded, found, err)
	}
	if got := loaded.DesiredSurfaces; len(got) != 3 || got[0] != "codex" || got[2] != "claude" {
		t.Fatalf("surfaces=%v", got)
	}
}

func TestStateV1RemainsReadableLegacyProvenanceAndUnknownSchemaFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"schema_version":1,"packy_version":"old","managed_skills":[],"configured_surfaces":["codex","opencode"],"paths":{"state_file":"x","agent_skills_dir":"y"}}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	state, found, err := LoadState(path)
	if err != nil || !found || !state.Legacy() {
		t.Fatalf("legacy=%+v,%v,%v", state, found, err)
	}
	if err := SaveState(path, state); err == nil {
		t.Fatal("legacy state was rewritten without migration")
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":3}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadState(path); err == nil || !strings.Contains(err.Error(), "unsupported schema_version 3") {
		t.Fatalf("unknown schema error=%v", err)
	}
}

func TestStateDecodersRejectUnknownFieldsAndNonCanonicalSurfaceIntent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for name, wire := range map[string]string{
		"v1 unknown":                 `{"schema_version":1,"packy_version":"old","managed_skills":[],"configured_surfaces":["codex","opencode"],"paths":{"state_file":"x","agent_skills_dir":"y"},"surprise":true}`,
		"v1 nonhistorical":           `{"schema_version":1,"packy_version":"old","managed_skills":[],"configured_surfaces":["codex"],"paths":{"state_file":"x","agent_skills_dir":"y"}}`,
		"v2 unknown":                 `{"schema_version":2,"packy_version":"new","managed_skills":[],"desired_surfaces":["codex","opencode","claude"],"claude_ownership":[],"paths":{"state_file":"x","agent_skills_dir":"y"},"created_containers":[],"surprise":true}`,
		"v2 reordered":               `{"schema_version":2,"packy_version":"new","managed_skills":[],"desired_surfaces":["claude","codex","opencode"],"claude_ownership":[],"paths":{"state_file":"x","agent_skills_dir":"y"},"created_containers":[]}`,
		"v2 null containers":         `{"schema_version":2,"packy_version":"new","managed_skills":[],"desired_surfaces":["codex","opencode","claude"],"claude_ownership":[],"paths":{"state_file":"x","agent_skills_dir":"y"},"created_containers":null}`,
		"v2 null contributors":       `{"schema_version":2,"packy_version":"new","managed_skills":[],"desired_surfaces":["codex","opencode","claude"],"claude_ownership":[{"id":"x","kind":"skill","target":"/x","fingerprint":"sha256:x","contributors":null,"deletion_authorized":true}],"paths":{"state_file":"x","agent_skills_dir":"y"},"created_containers":[]}`,
		"v2 missing MCP arrays":      `{"schema_version":2,"packy_version":"new","managed_skills":[],"desired_surfaces":["codex","opencode","claude"],"claude_ownership":[{"id":"x","kind":"mcp","target":"x","fingerprint":"sha256:x","contributors":["classic"],"command":"engram","deletion_authorized":true}],"paths":{"state_file":"x","agent_skills_dir":"y"},"created_containers":[]}`,
		"v2 null attempt arrays":     `{"schema_version":2,"packy_version":"new","managed_skills":[],"desired_surfaces":["codex","opencode","claude"],"claude_ownership":[],"paths":{"state_file":"x","agent_skills_dir":"y"},"created_containers":[],"latest_attempt":{"operation":"install","outcome":"verified","completed_effects":null,"not_started_effects":null}}`,
		"v2 invalid attempt outcome": `{"schema_version":2,"packy_version":"new","managed_skills":[],"desired_surfaces":["codex","opencode","claude"],"claude_ownership":[],"paths":{"state_file":"x","agent_skills_dir":"y"},"created_containers":[],"latest_attempt":{"operation":"install","outcome":"fresh-retry","completed_effects":[],"not_started_effects":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(wire), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadState(path); err == nil {
				t.Fatal("invalid state accepted")
			}
		})
	}
}
