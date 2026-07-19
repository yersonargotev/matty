package addyacceptance

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalInventoryAndDeterminism(t *testing.T) {
	mutableIDs := skillIDs()
	mutableIDs[0] = "corrupted"
	a, b := Canonical(), Canonical()
	counts := map[string]int{}
	for _, r := range a.Manifest.Resources {
		counts[r.Kind]++
	}
	for kind, want := range map[string]int{"skill": 24, "agent": 4, "command": 8, "asset": 7, "notice": 1} {
		if counts[kind] != want {
			t.Fatalf("%s count = %d, want %d", kind, counts[kind], want)
		}
	}
	if len(a.AcceptanceRows) != 20 || a.AcceptanceRows[0].Row != 1 || a.AcceptanceRows[len(a.AcceptanceRows)-1].Row != 22 {
		t.Fatalf("unexpected row metadata: %#v", a.AcceptanceRows)
	}
	if a.Manifest.Resources[len(a.Manifest.Resources)-1].ID == "corrupted" {
		t.Fatal("mutable construction inventory changed the canonical oracle")
	}
	first, _ := CanonicalJSON()
	second, _ := CanonicalJSON()
	if !bytes.Equal(first, second) {
		t.Fatal("canonical JSON changed between reruns")
	}
	a.Manifest.Resources[0].ID = "mutated"
	if b.Manifest.Resources[0].ID == "mutated" {
		t.Fatal("Canonical returned shared resource storage")
	}
}

func TestSnapshotPreservesInertExecutableHelper(t *testing.T) {
	root := t.TempDir()
	if err := WriteSnapshot(root); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(root, "skills", "idea-refine", "scripts", "idea-refine.sh")
	info, err := os.Stat(helper)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("helper mode = %o", info.Mode().Perm())
	}
	data, _ := os.ReadFile(helper)
	if !bytes.Contains(data, []byte("INERT TEST FIXTURE")) {
		t.Fatal("helper lacks inert marker")
	}
	if err := WriteSnapshot(root); err == nil {
		t.Fatal("non-empty root was accepted")
	}
}

func TestNegativeTwinChangesOneFact(t *testing.T) {
	canonical, _ := CanonicalJSON()
	twin, err := NegativeTwin("moved-tag")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(canonical, twin) {
		t.Fatal("negative twin equals canonical fixture")
	}
	if _, err := NegativeTwin("unknown"); err == nil {
		t.Fatal("unknown negative fact accepted")
	}
}
