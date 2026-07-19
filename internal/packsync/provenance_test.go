package packsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalSourceLockSortsResourcesAndUsesRepositoryJSONBytes(t *testing.T) {
	lock := Lock{
		SchemaVersion: 2,
		SourceID:      "addy",
		Resources: []ResourceEvidence{
			{Binding: Binding{PackID: "p", Kind: "skill", ResourceID: "z"}},
			{Binding: Binding{PackID: "p", Kind: "asset", ResourceID: "a"}},
		},
	}
	data, digest, err := CanonicalSourceLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") || strings.HasSuffix(string(data), "\n\n") || !strings.Contains(string(data), "\n  \"schema_version\": 2,") {
		t.Fatalf("canonical bytes do not use two-space JSON plus one LF: %q", data)
	}
	asset := strings.Index(string(data), `"kind": "asset"`)
	skill := strings.Index(string(data), `"kind": "skill"`)
	if asset < 0 || skill < 0 || asset >= skill {
		t.Fatalf("resources are not sorted by (pack_id, kind, resource_id): %s", data)
	}
	if digest != "ac22aed04329e0b99ee45c1135e15f0b9f10e1832a3baf873e11922e3cea39d0" {
		t.Fatalf("source lock digest = %s", digest)
	}
	if lock.Resources[0].Kind != "skill" {
		t.Fatal("canonicalization mutated caller-owned lock")
	}
}

func TestValidatePreconditionsRejectsUnrelatedSourceGenerationWithoutMutation(t *testing.T) {
	repository := t.TempDir()
	bundle := filepath.Join(repository, "bundle")
	config := Config{SchemaVersion: 1, Sources: []SourceConfig{
		{ID: "addy", Provider: "github", Repository: "o/addy", Selector: Selector{Mode: SelectorStableRelease}, Resources: []Binding{}},
		{ID: "mattpocock-skills", Provider: "github", Repository: "o/target", Selector: Selector{Mode: SelectorStableRelease}, Resources: []Binding{}},
	}}
	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	configBytes = append(configBytes, '\n')
	if err := os.MkdirAll(filepath.Join(bundle, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "sources.json"), configBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"addy", "mattpocock-skills"} {
		if err := writeCanonicalLock(filepath.Join(bundle, "sources", id+".lock.json"), Lock{SchemaVersion: 1, SourceID: id, Resources: []ResourceEvidence{}}); err != nil {
			t.Fatal(err)
		}
	}
	parsed, err := LoadConfig(strings.NewReader(string(configBytes)))
	if err != nil {
		t.Fatal(err)
	}
	set, err := loadSourceLockSet(bundle, parsed)
	if err != nil {
		t.Fatal(err)
	}
	_, manifests, err := loadManifests(repository)
	if err != nil {
		t.Fatal(err)
	}
	bundleDigest, err := treeHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	expected := Preconditions{ConfigSHA256: hashBytes(configBytes), ManifestsSHA256: manifests, BundleSHA256: bundleDigest, SourceLockSHA256: set.Digests["mattpocock-skills"], LockSetSHA256: set.LockSetSHA256}
	if err := writeCanonicalLock(filepath.Join(bundle, "sources", "addy.lock.json"), Lock{SchemaVersion: 2, SourceID: "addy", Resources: []ResourceEvidence{}}); err != nil {
		t.Fatal(err)
	}
	before, err := treeHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	err = validatePreconditions(repository, "mattpocock-skills", expected, false)
	if err == nil || !strings.Contains(err.Error(), "complete provenance lock set changed") {
		t.Fatalf("unrelated-source stale error = %v", err)
	}
	after, err := treeHash(bundle)
	if err != nil || after != before {
		t.Fatalf("freshness validation mutated bundle: %s -> %s, %v", before, after, err)
	}
}

func TestLoadSourceLockSetRequiresCanonicalBijection(t *testing.T) {
	bundle := t.TempDir()
	config := Config{Sources: []SourceConfig{{ID: "addy"}, {ID: "mattpocock-skills"}}}
	locks := []Lock{{SchemaVersion: 2, SourceID: "addy"}, {SchemaVersion: 1, SourceID: "mattpocock-skills"}}
	for _, lock := range locks {
		data, _, err := CanonicalSourceLock(lock)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(bundle, "sources", lock.SourceID+".lock.json")
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	set, err := loadSourceLockSet(bundle, config)
	if err != nil {
		t.Fatal(err)
	}
	if set.Locks["addy"].SourceID != "addy" || len(set.Locks) != 2 || len(set.LockSetSHA256) != 64 {
		t.Fatalf("set = %#v", set)
	}

	tests := map[string]func(string){
		"missing": func(root string) { os.Remove(filepath.Join(root, "sources", "addy.lock.json")) },
		"orphan": func(root string) {
			os.Rename(filepath.Join(root, "sources", "addy.lock.json"), filepath.Join(root, "sources", "orphan.lock.json"))
		},
		"mixed": func(root string) { os.WriteFile(filepath.Join(root, "sources.lock.json"), []byte("{}\n"), 0o644) },
		"noncanonical": func(root string) {
			name := filepath.Join(root, "sources", "addy.lock.json")
			data, _ := os.ReadFile(name)
			os.WriteFile(name, append(data, '\n'), 0o644)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := copyTreeExact(bundle, root); err != nil {
				t.Fatal(err)
			}
			mutate(root)
			if _, err := loadSourceLockSet(root, config); err == nil {
				t.Fatal("invalid topology accepted")
			}
		})
	}
}

func TestLoadConfigRejectsPathUnsafeSourceIDsAndSharedBindings(t *testing.T) {
	binding := `{"pack_id":"p","kind":"skill","resource_id":"r","upstream_path":"skills/r"}`
	for _, document := range []string{
		`{"schema_version":1,"sources":[{"id":"../escape","provider":"github","repository":"o/r","selector":{"mode":"stable-release"},"resources":[]}]}`,
		`{"schema_version":1,"sources":[{"id":"a","provider":"github","repository":"o/a","selector":{"mode":"stable-release"},"resources":[` + binding + `]},{"id":"b","provider":"github","repository":"o/b","selector":{"mode":"stable-release"},"resources":[` + binding + `]}]}`,
	} {
		if _, err := LoadConfig(strings.NewReader(document)); err == nil {
			t.Fatalf("invalid config accepted: %s", document)
		}
	}
}

func TestLockSetSHA256UsesSortedSourceIDNULDigestLFRecords(t *testing.T) {
	got, err := LockSetSHA256([]SourceLockDigest{
		{SourceID: "zeta", SHA256: strings.Repeat("b", 64)},
		{SourceID: "addy", SHA256: strings.Repeat("a", 64)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "2acbb5e3844a0615a4539b3751d3d775d2ecfc89b5e248c5f05d273f14af3814" {
		t.Fatalf("lock-set digest = %s", got)
	}
	for _, invalid := range [][]SourceLockDigest{
		{{SourceID: "../escape", SHA256: strings.Repeat("a", 64)}},
		{{SourceID: "addy", SHA256: "bad"}},
		{{SourceID: "addy", SHA256: strings.Repeat("a", 64)}, {SourceID: "addy", SHA256: strings.Repeat("b", 64)}},
	} {
		if _, err := LockSetSHA256(invalid); err == nil {
			t.Fatalf("invalid lock set accepted: %#v", invalid)
		}
	}
}
