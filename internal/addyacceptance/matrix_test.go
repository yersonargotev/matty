package addyacceptance

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type matrixCase struct {
	row      int
	surface  string
	mutate   func(*Fixture)
	validate func(Fixture) error
}

func TestEveryOwnedMatrixRowHasAStableOneFactNegativeCase(t *testing.T) {
	for _, tc := range completeMatrixCases() {
		t.Run(fmt.Sprintf("row-%02d-%s", tc.row, tc.surface), func(t *testing.T) {
			root := t.TempDir()
			home, xdg := filepath.Join(root, "home"), filepath.Join(root, "xdg")
			if err := os.MkdirAll(home, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(xdg, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", xdg)
			before := treeDigest(t, root)
			positive := Canonical()
			if err := tc.validate(positive); err != nil {
				t.Fatalf("positive oracle rejected: %v", err)
			}
			negative := cloneFixture(t, positive)
			tc.mutate(&negative)
			first := tc.validate(negative)
			second := tc.validate(cloneFixture(t, negative))
			want := fmt.Sprintf("ADDY-ROW-%02d-BLOCKED", tc.row)
			if first == nil || first.Error() != want || second == nil || second.Error() != want {
				t.Fatalf("unstable diagnostic: first=%v second=%v want=%s", first, second, want)
			}
			if after := treeDigest(t, root); after != before {
				t.Fatalf("failed row crossed zero-mutation boundary: %s != %s", after, before)
			}
		})
	}
}

func completeMatrixCases() []matrixCase {
	blocked := func(row int, ok func(Fixture) bool) func(Fixture) error {
		return func(f Fixture) error {
			if !ok(f) {
				return fmt.Errorf("ADDY-ROW-%02d-BLOCKED", row)
			}
			return nil
		}
	}
	count := func(f Fixture, kind string) int {
		n := 0
		for _, resource := range f.Manifest.Resources {
			if resource.Kind == kind {
				n++
			}
		}
		return n
	}
	bindings := func(f Fixture, surface string) []Binding {
		var out []Binding
		for _, resource := range f.Manifest.Resources {
			for _, binding := range resource.Bindings {
				if binding.Surface == surface {
					out = append(out, binding)
				}
			}
		}
		return out
	}
	return []matrixCase{
		{1, "portable", func(f *Fixture) { f.Provenance.Commit = strings.Repeat("a", 40) }, blocked(1, func(f Fixture) bool {
			return f.Provenance.Commit == Commit && len(f.Provenance.CommitParents) == 2 && f.Provenance.CommitVerification.Verified && len(f.Provenance.ArchiveSHA256) == 64 && len(f.Provenance.SelectedSHA256) == 44
		})},
		{2, "portable", func(f *Fixture) { f.Provenance.ArchiveSHA256 = strings.Repeat("0", 64) }, blocked(2, func(f Fixture) bool { return f.Provenance.ArchiveSHA256 == fmt.Sprintf("%x", fixtureArchiveDigest()) })},
		{4, "portable", func(f *Fixture) { f.Manifest.Resources = append(f.Manifest.Resources, f.Manifest.Resources[0]) }, blocked(4, uniqueOwnership)},
		{6, "portable", func(f *Fixture) { f.Manifest.Resources = removeFirstKind(f.Manifest.Resources, "skill") }, blocked(6, func(f Fixture) bool {
			return count(f, "skill") == 24 && count(f, "agent") == 4 && count(f, "command") == 8 && count(f, "asset") == 7 && count(f, "notice") == 1
		})},
		{7, "portable", func(f *Fixture) {
			f.Manifest.Resources[0].Requires = append(f.Manifest.Resources[0].Requires, "skill:missing")
		}, blocked(7, dependencyClosed)},
		{8, "portable", func(f *Fixture) {
			f.Manifest.Resources = append(f.Manifest.Resources, Resource{Kind: "asset", ID: "hook", Source: "hooks/session-start.sh", Requires: []string{}})
		}, blocked(8, exclusionsDisjoint)},
		{9, "portable", func(f *Fixture) { f.Manifest.SchemaVersion = 99 }, blocked(9, func(f Fixture) bool {
			data, _ := json.Marshal(f.Manifest)
			var round Manifest
			return f.Manifest.SchemaVersion == 2 && json.Unmarshal(data, &round) == nil && reflect.DeepEqual(round, f.Manifest)
		})},
		{10, "both", func(f *Fixture) { f.SupportedRoutes[0].Actions = nil }, blocked(10, func(f Fixture) bool {
			return len(f.SupportedRoutes) == 2 && len(f.SupportedRoutes[0].Actions) > 0 && f.SupportedRoutes[1].Kind == "exact-no-op" && len(f.SupportedRoutes[1].Actions) == 0
		})},
		{11, "both", func(f *Fixture) { f.Manifest.Contract.OptionalModes = f.Manifest.Contract.OptionalModes[1:] }, blocked(11, completeAuthorities)},
		{12, "codex", func(f *Fixture) { removeSurfaceBinding(f, "codex") }, blocked(12, func(f Fixture) bool {
			values := bindings(f, "codex")
			return len(values) == 36 && countProjection(values, "skill", "degraded") == 8
		})},
		{13, "opencode", func(f *Fixture) { removeSurfaceBinding(f, "opencode") }, blocked(13, func(f Fixture) bool {
			values := bindings(f, "opencode")
			return len(values) == 36 && countProjection(values, "command", "native") == 8
		})},
		{14, "both", func(f *Fixture) { f.Manifest.Resources[1].Bindings[0].Name = f.Manifest.Resources[0].Bindings[0].Name }, blocked(14, uniqueSurfaceNames)},
		{15, "both", func(f *Fixture) {
			f.Manifest.Resources[0], f.Manifest.Resources[1] = f.Manifest.Resources[1], f.Manifest.Resources[0]
		}, blocked(15, canonicalResourceOrder)},
		{16, "both", func(f *Fixture) { f.Lifecycle.SurfaceStateIsolation = false }, blocked(16, func(f Fixture) bool {
			return reflect.DeepEqual(f.Manifest.Surfaces, []string{"codex", "opencode"}) && f.Lifecycle.SurfaceStateIsolation
		})},
		{17, "both", func(f *Fixture) { f.Lifecycle.ActivationConsent = "untyped" }, blocked(17, func(f Fixture) bool {
			return f.Lifecycle.ActivationConsent == "reversible-local" && f.Lifecycle.FreshPreflight
		})},
		{18, "both", func(f *Fixture) { f.Lifecycle.AtomicApply = false }, blocked(18, func(f Fixture) bool { return f.Lifecycle.AtomicApply && f.Lifecycle.RecoveryRequiresPlan })},
		{19, "both", func(f *Fixture) { f.Lifecycle.ReadinessValues = []string{"no", "yes"} }, blocked(19, func(f Fixture) bool {
			return reflect.DeepEqual(f.Lifecycle.ReadinessValues, []string{"no", "unknown", "yes"}) && reflect.DeepEqual(f.Lifecycle.RequireUsableRejects, []string{"no", "unknown"})
		})},
		{20, "both", func(f *Fixture) { f.Manifest.Contract.OptionalModes[0].ID = f.Manifest.Contract.Exclusions[0].ID }, blocked(20, distinctContractFacts)},
		{21, "both", func(f *Fixture) { f.SupportedRoutes = f.SupportedRoutes[1:] }, blocked(21, func(f Fixture) bool {
			return len(f.SupportedRoutes) == 2 && f.SupportedRoutes[0].From == "absent" && f.SupportedRoutes[1].Kind == "exact-no-op"
		})},
		{22, "both", func(f *Fixture) { f.Lifecycle.PreserveShared = false }, blocked(22, func(f Fixture) bool {
			return f.Lifecycle.RemovalConsent == "destructive-cleanup" && f.Lifecycle.PreserveShared
		})},
	}
}

func cloneFixture(t *testing.T, fixture Fixture) Fixture {
	t.Helper()
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var result Fixture
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func fixtureArchiveDigest() [32]byte { return sha256Bytes(ExactArchive()) }

func sha256Bytes(data []byte) [32]byte {
	// Kept local to make the row oracle independent from fixture construction.
	return sha256.Sum256(data)
}

func uniqueOwnership(f Fixture) bool {
	seen := map[string]bool{}
	for _, resource := range f.Manifest.Resources {
		key := resource.Kind + ":" + resource.ID
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func dependencyClosed(f Fixture) bool {
	if !uniqueOwnership(f) {
		return false
	}
	for _, resource := range f.Manifest.Resources {
		for _, dependency := range resource.Requires {
			if !hasResource(f.Manifest.Resources, dependency) {
				return false
			}
		}
	}
	return true
}

func exclusionsDisjoint(f Fixture) bool {
	for _, resource := range f.Manifest.Resources {
		for _, exclusion := range f.Manifest.Contract.Exclusions {
			for _, path := range exclusion.SourcePaths {
				prefix := strings.TrimSuffix(path, "/**")
				if resource.Source == path || (strings.HasSuffix(path, "/**") && strings.HasPrefix(resource.Source, prefix+"/")) {
					return false
				}
			}
		}
	}
	return true
}

func completeAuthorities(f Fixture) bool {
	seen := map[string]bool{}
	for _, mode := range f.Manifest.Contract.OptionalModes {
		for _, authority := range mode.Authorities {
			seen[authority] = true
		}
	}
	for _, resource := range f.Manifest.Resources {
		for _, authority := range resource.Permissions {
			seen[authority] = true
		}
	}
	for _, authority := range []string{"browser", "commit", "deploy", "filesystem", "network", "package-manager", "process", "subagent"} {
		if !seen[authority] {
			return false
		}
	}
	return true
}

func removeSurfaceBinding(f *Fixture, surface string) {
	for i := range f.Manifest.Resources {
		for j, binding := range f.Manifest.Resources[i].Bindings {
			if binding.Surface == surface {
				f.Manifest.Resources[i].Bindings = append(f.Manifest.Resources[i].Bindings[:j:j], f.Manifest.Resources[i].Bindings[j+1:]...)
				return
			}
		}
	}
}

func countProjection(values []Binding, projection, mode string) int {
	n := 0
	for _, binding := range values {
		if binding.Projection == projection && binding.Mode == mode {
			n++
		}
	}
	return n
}

func uniqueSurfaceNames(f Fixture) bool {
	seen := map[string]bool{}
	for _, resource := range f.Manifest.Resources {
		for _, binding := range resource.Bindings {
			key := binding.Surface + ":" + binding.Projection + ":" + binding.Name
			if seen[key] {
				return false
			}
			seen[key] = true
		}
	}
	return true
}

func canonicalResourceOrder(f Fixture) bool {
	keys := make([]string, len(f.Manifest.Resources))
	for i, resource := range f.Manifest.Resources {
		keys[i] = resource.Kind + ":" + resource.ID
	}
	return sort.StringsAreSorted(keys)
}

func distinctContractFacts(f Fixture) bool {
	seen := map[string]bool{}
	for _, exclusion := range f.Manifest.Contract.Exclusions {
		seen[exclusion.ID] = true
	}
	for _, mode := range f.Manifest.Contract.OptionalModes {
		if seen[mode.ID] {
			return false
		}
		seen[mode.ID] = true
	}
	return true
}

func removeFirstKind(resources []Resource, kind string) []Resource {
	for i, resource := range resources {
		if resource.Kind == kind {
			return append(resources[:i:i], resources[i+1:]...)
		}
	}
	return resources
}
