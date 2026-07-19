package packsync

import (
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
