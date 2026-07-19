package addyacceptance

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/opencode"
)

func TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent(t *testing.T) {
	for _, surface := range []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode} {
		t.Run(string(surface), func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "acquisition")
			home := filepath.Join(root, "home")
			xdg := filepath.Join(root, "xdg")
			state := filepath.Join(root, "state")
			for _, path := range []string{source, home, xdg, state} {
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", xdg)
			sentinel := filepath.Join(root, "hostile-executed")
			t.Setenv("ADDY_ACCEPTANCE_SENTINEL", sentinel)
			if err := os.WriteFile(filepath.Join(source, "occupied"), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := WriteSnapshot(source); err == nil {
				t.Fatal("snapshot writer accepted a non-empty acquisition root")
			}
			// WriteSnapshot owns empty-root admission, so use a fresh child.
			source = filepath.Join(root, "candidate")
			if err := WriteSnapshot(source); err != nil {
				t.Fatal(err)
			}

			pack := capabilityPack(Canonical())
			adapter := surfaceAdapter(surface, source, home, xdg)
			before := treeDigest(t, root)
			first, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
			if err != nil {
				t.Fatal(err)
			}
			second, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
			if err != nil {
				t.Fatal(err)
			}
			if first.Revision != second.Revision || !reflect.DeepEqual(first.Projections, second.Projections) {
				t.Fatal("surface inspection did not produce a deterministic oracle")
			}
			if got := treeDigest(t, root); got != before {
				t.Fatalf("preview mutated sandbox: %s != %s", got, before)
			}
			assertCompleteProjection(t, surface, first.Projections)
			if first.Readiness.UsabilityObserved || first.Readiness.Usable {
				t.Fatalf("filesystem preview guessed usability: %+v", first.Readiness)
			}

			actions := make([]capabilitypack.ProjectionAction, len(first.Projections))
			for i := range first.Projections {
				actions[i] = first.Projections[i].Action
			}
			if err := adapter.ApplyProjections(context.Background(), actions); err != nil {
				t.Fatal(err)
			}
			owners := make([]capabilitypack.ProjectionOwnership, len(first.Projections))
			for i, projection := range first.Projections {
				owners[i] = capabilitypack.ProjectionOwnership{ID: projection.ID, Fingerprint: projection.DesiredFingerprint, Contributors: []string{"addy"}}
			}
			verified, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack, CurrentOwnership: owners})
			if err != nil {
				t.Fatal(err)
			}
			for _, projection := range verified.Projections {
				if !projection.Exists || projection.ObservedFingerprint != projection.DesiredFingerprint {
					t.Fatalf("projection did not verify: %+v", projection)
				}
			}
			if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
				t.Fatalf("inert content executed: %v", err)
			}
		})
	}
}

func TestLifecycleOracleExposesExactCountsAuthoritiesAndSurfaceBindings(t *testing.T) {
	pack := capabilityPack(Canonical())
	for _, surface := range []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode} {
		contract := capabilitypack.LifecycleContractFor(pack, surface, nil)
		if contract.Counts != (capabilitypack.ResourceCounts{Skills: 24, Agents: 4, Commands: 8, Assets: 7, Notices: 1}) {
			t.Fatalf("%s counts = %+v", surface, contract.Counts)
		}
		if len(contract.Bindings) != 36 {
			t.Fatalf("%s bindings = %d, want 36", surface, len(contract.Bindings))
		}
		for _, authority := range []string{"browser", "commit", "deploy", "filesystem", "network", "package-manager", "process", "subagent"} {
			if !contains(contract.PromptAuthorities, authority) {
				t.Fatalf("%s omitted authority %q: %v", surface, authority, contract.PromptAuthorities)
			}
		}
		if len(contract.Exclusions) != 2 || len(contract.OptionalModes) != 4 {
			t.Fatalf("%s lost exclusions or optional modes: %+v", surface, contract)
		}
	}
}

func TestOneFactNegativeTwinBlocksCompleteInventory(t *testing.T) {
	fixture := Canonical()
	for i, resource := range fixture.Manifest.Resources {
		if resource.Kind == "skill" {
			fixture.Manifest.Resources = append(fixture.Manifest.Resources[:i:i], fixture.Manifest.Resources[i+1:]...)
			break
		}
	}
	counts := capabilitypack.LifecycleContractFor(capabilityPack(fixture), capabilitypack.SurfaceCodex, nil).Counts
	if counts.Skills != 23 {
		t.Fatalf("negative twin did not change exactly the skill inventory fact: %+v", counts)
	}
	if counts.Agents != 4 || counts.Commands != 8 || counts.Assets != 7 || counts.Notices != 1 {
		t.Fatalf("negative twin changed unrelated inventory facts: %+v", counts)
	}
}

func surfaceAdapter(surface capabilitypack.Surface, source, home, xdg string) capabilitypack.SurfaceAdapter {
	if surface == capabilitypack.SurfaceCodex {
		return codex.NewSurfaceAdapterWithConfig(source, filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".codex", "AGENTS.md"), filepath.Join(home, ".codex", "config.toml"))
	}
	return opencode.NewSurfaceAdapter(source, filepath.Join(home, ".agents", "skills"), filepath.Join(xdg, "opencode", "opencode.json"), filepath.Join(xdg, "opencode", "packy.md"))
}

func capabilityPack(f Fixture) capabilitypack.Pack {
	resources := make([]capabilitypack.Resource, len(f.Manifest.Resources))
	for i, resource := range f.Manifest.Resources {
		bindings := make([]capabilitypack.Binding, len(resource.Bindings))
		for j, binding := range resource.Bindings {
			bindings[j] = capabilitypack.Binding{Surface: capabilitypack.Surface(binding.Surface), Projection: binding.Projection, Name: binding.Name, Invocation: binding.Invocation, Mode: binding.Mode, Degradation: binding.Degradation, Sharing: binding.Sharing}
		}
		arguments := capabilitypack.CommandArguments{}
		if resource.Arguments != nil {
			arguments = capabilitypack.CommandArguments{Mode: resource.Arguments.Mode, Placeholder: resource.Arguments.Placeholder}
		}
		resources[i] = capabilitypack.Resource{Kind: resource.Kind, ID: resource.ID, Source: resource.Source, Description: resource.Description, Mode: resource.Mode, Tools: append([]string(nil), resource.Tools...), Permissions: append([]string(nil), resource.Permissions...), Requires: append([]string(nil), resource.Requires...), Bindings: bindings, Arguments: arguments, License: resource.License, Attribution: resource.Attribution}
	}
	exclusions := make([]capabilitypack.Exclusion, len(f.Manifest.Contract.Exclusions))
	for i, exclusion := range f.Manifest.Contract.Exclusions {
		exclusions[i] = capabilitypack.Exclusion{ID: exclusion.ID, SourcePaths: append([]string(nil), exclusion.SourcePaths...), Reason: exclusion.Reason}
	}
	modes := make([]capabilitypack.OptionalMode, len(f.Manifest.Contract.OptionalModes))
	for i, mode := range f.Manifest.Contract.OptionalModes {
		modes[i] = capabilitypack.OptionalMode{ID: mode.ID, Authorities: append([]string(nil), mode.Authorities...), Fallback: mode.Fallback}
	}
	return capabilitypack.Pack{ID: f.Manifest.ID, Version: f.Manifest.Version, Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode}, Provides: append([]string(nil), f.Manifest.Provides...), Requires: capabilitypack.Requirements{Capabilities: []string{}, Tools: []string{}}, Conflicts: []string{}, Resources: resources, Contract: capabilitypack.Contract{Exclusions: exclusions, OptionalModes: modes}}
}

func assertCompleteProjection(t *testing.T, surface capabilitypack.Surface, projections []capabilitypack.ObservedProjection) {
	t.Helper()
	kinds := map[string]int{}
	for _, projection := range projections {
		kinds[strings.SplitN(projection.ID, ":", 2)[0]]++
	}
	wantWorkflowKind := "workflow"
	if surface == capabilitypack.SurfaceOpenCode {
		wantWorkflowKind = "command"
	}
	if kinds["skill"] != 24 || kinds["agent"] != 4 || kinds[wantWorkflowKind] != 8 {
		t.Fatalf("%s incomplete projection kinds: %v", surface, kinds)
	}
	if surface == capabilitypack.SurfaceOpenCode && kinds["command"] != 8 {
		t.Fatalf("OpenCode native command projection missing: %v", kinds)
	}
	if surface == capabilitypack.SurfaceCodex && kinds["command"] != 0 {
		t.Fatalf("Codex falsely claimed native commands: %v", kinds)
	}
}

func treeDigest(t *testing.T, root string) string {
	t.Helper()
	var rows []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, _ := filepath.Rel(root, path)
		rows = append(rows, fmt.Sprintf("%s %x", filepath.ToSlash(rel), sum))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(rows)
	sum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return fmt.Sprintf("%x", sum)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
