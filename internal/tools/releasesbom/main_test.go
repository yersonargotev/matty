package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/release"
)

func TestGenerateIsDeterministicSortedAndDomainValid(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	os.Mkdir(first, 0o700)
	os.Mkdir(second, 0o700)
	writeBinary(t, first, "zeta", []byte("z"))
	writeBinary(t, first, "alpha", []byte("a"))
	writeBinary(t, second, "alpha", []byte("a"))
	writeBinary(t, second, "zeta", []byte("z"))
	out1 := filepath.Join(root, "out1", release.SBOMName)
	out2 := filepath.Join(root, "out2", release.SBOMName)
	os.Mkdir(filepath.Dir(out1), 0o700)
	os.Mkdir(filepath.Dir(out2), 0o700)
	var stdout1, stdout2 bytes.Buffer
	args := func(dist, out string) []string {
		return []string{"--version", "v0.1.2", "--created", "2026-01-02T03:04:05Z", "--dist", dist, "--out", out}
	}
	if err := run(args(first, out1), &stdout1); err != nil {
		t.Fatal(err)
	}
	if err := run(args(second, out2), &stdout2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout1.Bytes(), stdout2.Bytes()) || !bytes.Equal(stdout1.Bytes(), mustRead(t, out1)) {
		t.Fatal("output depends on filesystem creation order")
	}
	if strings.Index(stdout1.String(), `"fileName":"alpha"`) > strings.Index(stdout1.String(), `"fileName":"zeta"`) {
		t.Fatal("files are not sorted")
	}
	subjects, err := observeBinaries(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := release.VerifySPDXSBOM(stdout1.Bytes(), "v0.1.2", subjects); err != nil {
		t.Fatalf("generated document failed domain validation: %v", err)
	}
}

func TestCreatedTimestampIsNormalizedToCanonicalUTC(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	parent := filepath.Join(root, "out")
	os.Mkdir(dist, 0o700)
	os.Mkdir(parent, 0o700)
	writeBinary(t, dist, "packy", []byte("binary"))
	out := filepath.Join(parent, release.SBOMName)
	var stdout bytes.Buffer
	if err := run([]string{"--version", "v0.1.2", "--created", "2026-01-01T22:04:05-05:00", "--dist", dist, "--out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"created":"2026-01-02T03:04:05Z"`) {
		t.Fatalf("timestamp was not normalized: %s", stdout.String())
	}
}

func TestRejectsInvalidVersionAndUnsafeFilesystem(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	outParent := filepath.Join(root, "out")
	os.Mkdir(dist, 0o700)
	os.Mkdir(outParent, 0o700)
	writeBinary(t, dist, "packy", []byte("binary"))
	base := []string{"--created", "2026-01-02T03:04:05Z", "--dist", dist, "--out", filepath.Join(outParent, release.SBOMName)}
	if err := run(append([]string{"--version", "v1.2.3"}, base...), ioDiscard{}); err == nil {
		t.Fatal("invalid version accepted")
	}
	if err := os.Symlink(filepath.Join(dist, "packy"), filepath.Join(dist, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := run(append([]string{"--version", "v0.1.2"}, base...), ioDiscard{}); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink error = %v", err)
	}
	os.Remove(filepath.Join(dist, "linked"))
	writeBinary(t, dist, ".hidden", []byte("hidden"))
	if err := run(append([]string{"--version", "v0.1.2"}, base...), ioDiscard{}); err == nil || !strings.Contains(err.Error(), "hidden") {
		t.Fatalf("hidden error = %v", err)
	}
	os.Remove(filepath.Join(dist, ".hidden"))
	existing := filepath.Join(outParent, release.SBOMName)
	os.WriteFile(existing, []byte("preserve"), 0o600)
	if err := run(append([]string{"--version", "v0.1.2"}, base...), ioDiscard{}); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("existing output error = %v", err)
	}
	if string(mustRead(t, existing)) != "preserve" {
		t.Fatal("existing output changed")
	}
}

func TestRejectsEmptyDistAndOutputOverlap(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	os.Mkdir(dist, 0o700)
	outside := filepath.Join(root, "outside")
	os.Mkdir(outside, 0o700)
	args := func(out string) []string {
		return []string{"--version", "v0.1.2", "--created", "2026-01-02T03:04:05Z", "--dist", dist, "--out", out}
	}
	if err := run(args(filepath.Join(outside, release.SBOMName)), ioDiscard{}); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("empty error = %v", err)
	}
	writeBinary(t, dist, "packy", []byte("binary"))
	if err := run(args(filepath.Join(dist, release.SBOMName)), ioDiscard{}); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlap error = %v", err)
	}
}

func writeBinary(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
