package addyacceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestPortableCohortClosesExactInventoryOwnershipIntegrityAndEvolution(t *testing.T) {
	fixture := Canonical()
	if fixture.Provenance.Repository != "addyosmani/agent-skills" || fixture.Provenance.Release != Release || fixture.Provenance.Commit != Commit || fixture.Provenance.Tree != Tree {
		t.Fatalf("pinned provenance drifted: %+v", fixture.Provenance)
	}
	if fixture.Provenance.TagSHA == "" || fixture.Provenance.TagVerification.Reason != "unsigned" || !fixture.Provenance.CommitVerification.Verified || len(fixture.Provenance.CommitParents) != 2 || len(fixture.Provenance.ArchiveSHA256) != 64 || len(fixture.Provenance.SnapshotSHA256) != 64 || len(fixture.Provenance.SelectedSHA256) != 44 {
		t.Fatalf("incomplete provenance evidence: %+v", fixture.Provenance)
	}

	identities := map[string]bool{}
	paths := map[string]bool{}
	for _, resource := range fixture.Manifest.Resources {
		identity := resource.Kind + ":" + resource.ID
		if identities[identity] {
			t.Fatalf("duplicate portable ownership identity %s", identity)
		}
		identities[identity] = true
		if paths[resource.Source] {
			t.Fatalf("two resources claim source %s", resource.Source)
		}
		paths[resource.Source] = true
		for _, dependency := range resource.Requires {
			if !identities[dependency] && !hasResource(fixture.Manifest.Resources, dependency) {
				t.Fatalf("%s has dangling dependency %s", identity, dependency)
			}
		}
	}
	if len(identities) != 44 {
		t.Fatalf("complete contribution has %d resources, want 44", len(identities))
	}

	fileByPath := map[string]File{}
	for _, file := range fixture.Files {
		if fileByPath[file.Path].Path != "" {
			t.Fatalf("duplicate selected file %s", file.Path)
		}
		fileByPath[file.Path] = file
	}
	for _, resource := range fixture.Manifest.Resources {
		path := resource.Source
		if resource.Kind == "skill" {
			path += "/SKILL.md"
		}
		if _, ok := fileByPath[path]; !ok {
			t.Fatalf("resource %s:%s has no selected source", resource.Kind, resource.ID)
		}
	}
	helper := fileByPath["skills/idea-refine/scripts/idea-refine.sh"]
	if helper.Mode != 0o755 || !strings.Contains(helper.Content, "ADDY_ACCEPTANCE_SENTINEL") {
		t.Fatalf("inert helper contract = %+v", helper)
	}
	for _, exclusion := range fixture.Manifest.Contract.Exclusions {
		for _, excluded := range exclusion.SourcePaths {
			for selected := range fileByPath {
				prefix := strings.TrimSuffix(excluded, "/**")
				if selected == excluded || (strings.HasSuffix(excluded, "/**") && strings.HasPrefix(selected, prefix+"/")) {
					t.Fatalf("excluded source %s became selected resource %s", excluded, selected)
				}
			}
		}
	}
	if len(fixture.SupportedRoutes) != 2 || fixture.SupportedRoutes[0].From != "absent" || fixture.SupportedRoutes[1].Kind != "exact-no-op" {
		t.Fatalf("supported routes = %+v", fixture.SupportedRoutes)
	}
	if len(fixture.SupportedRoutes[0].Actions) == 0 || len(fixture.SupportedRoutes[1].Actions) != 0 {
		t.Fatalf("introduction/no-op actions are not exact: %+v", fixture.SupportedRoutes)
	}
}

func TestEvidenceEnvelopeUsesStableStructuredOracleDisposableRootsAndOneFactTwins(t *testing.T) {
	root := t.TempDir()
	home, xdg := filepath.Join(root, "home"), filepath.Join(root, "xdg")
	for _, path := range []string{home, xdg} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	sentinel := filepath.Join(root, "executed")
	t.Setenv("ADDY_ACCEPTANCE_SENTINEL", sentinel)

	first, err := CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalJSON()
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("canonical structured result changed on deterministic rerun")
	}
	var object map[string]any
	if err := json.Unmarshal(first, &object); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first, []byte(`"negative_fact"`)) || bytes.Contains(first, []byte(`"NegativeFact"`)) {
		t.Fatalf("oracle JSON is not canonical snake_case: %s", first)
	}
	for _, fact := range []string{"missing-skill", "moved-tag", "executable-helper-selected"} {
		twin, err := NegativeTwin(fact)
		if err != nil || bytes.Equal(first, twin) {
			t.Fatalf("negative twin %s: err=%v", fact, err)
		}
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("fixture content executed while building or comparing oracles: %v", err)
	}

	rows := append([]AcceptanceRow(nil), Canonical().AcceptanceRows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Row < rows[j].Row })
	want := []int{1, 2, 4, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22}
	for i, row := range rows {
		if row.Row != want[i] || row.Fact == "" || row.NegativeFact == "" || len(row.PositiveEvidence) == 0 || len(row.NegativeEvidence) == 0 {
			t.Fatalf("acceptance row envelope[%d] = %+v", i, row)
		}
	}
}

func hasResource(resources []Resource, identity string) bool {
	for _, resource := range resources {
		if resource.Kind+":"+resource.ID == identity {
			return true
		}
	}
	return false
}
