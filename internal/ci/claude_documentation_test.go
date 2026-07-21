package ci_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/claudecode"
)

func TestClaudeDocumentationContractStaysCurrent(t *testing.T) {
	root := repositoryRoot(t)
	documents := map[string][]string{
		"README.md":                  {"Claude Code", "docs/claude-code.md", claudecode.MinimumSupportedVersion},
		"docs/claude-code.md":        {"Prerequisite", "Global projections", "migration", "Preservation", "recovery", "readiness", "cleanup", "No authentication or model calls", claudecode.MinimumSupportedVersion},
		"docs/product/packy-v0.md":   {"Claude Code", claudecode.MinimumSupportedVersion},
		"docs/roadmap.md":            {"Claude Code", claudecode.MinimumSupportedVersion},
		"docs/capability-packs.md":   {"manifest v3", "Claude Code", "matty 3.0.0", "engram 2.0.0"},
		"docs/structured-output.md":  {"schema_version: 2", "claude-readiness"},
		"docs/release.md":            {"./scripts/validate-packy.sh", "Fail-closed publication gate", claudecode.MinimumSupportedVersion},
		"docs/release-notes/next.md": {"{{TAG}}", claudecode.MinimumSupportedVersion, "state schema v2", "matty 3.0.0", "engram 2.0.0", "degraded"},
	}

	staleTwoSurface := regexp.MustCompile(`(?is)(?:supports? only.{0,80}(?:codex.{0,30}opencode|opencode.{0,30}codex)|both supported surfaces|two[- ]surface support|cli surfaces\s*\|\s*codex and opencode only)`)
	versionLiteral := regexp.MustCompile(`\d+\.\d+\.\d+`)
	for path, required := range documents {
		text := readFile(t, filepath.Join(root, path))
		for _, want := range required {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing documentation contract text %q", path, want)
			}
		}
		if staleTwoSurface.MatchString(text) {
			t.Errorf("%s retains a stale two-surface support claim: %q", path, staleTwoSurface.FindString(text))
		}
		for _, line := range strings.Split(text, "\n") {
			lower := strings.ToLower(line)
			if !strings.Contains(lower, "floor") && !strings.Contains(lower, "prerequisite") && !strings.Contains(lower, "or newer") && !strings.Contains(line, "+") {
				continue
			}
			for _, version := range versionLiteral.FindAllString(line, -1) {
				if version != claudecode.MinimumSupportedVersion {
					t.Errorf("%s publishes Claude floor %s, want code authority %s", path, version, claudecode.MinimumSupportedVersion)
				}
			}
		}
	}
}
