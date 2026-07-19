package addyacceptance

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsafeArchiveTwinBlocksAndCleansBeforeExecution(t *testing.T) {
	tests := []struct {
		name string
		path string
		mode int64
		kind byte
	}{
		{name: "traversal", path: "root/../escape", mode: 0o644, kind: tar.TypeReg},
		{name: "absolute", path: "/absolute", mode: 0o644, kind: tar.TypeReg},
		{name: "setuid", path: "root/hostile", mode: 0o4755, kind: tar.TypeReg},
		{name: "world-writable", path: "root/hostile", mode: 0o777, kind: tar.TypeReg},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "acquisition")
			data := archiveTwin(t, tc.path, tc.mode, tc.kind)
			if _, err := acquireSafely(data, root); err == nil {
				t.Fatal("unsafe archive was accepted")
			}
			if entries, err := os.ReadDir(filepath.Dir(root)); err != nil || len(entries) != 0 {
				t.Fatalf("failed acquisition leaked staging or output: entries=%v err=%v", entries, err)
			}
		})
	}
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink} {
		root := filepath.Join(t.TempDir(), "acquisition")
		report, err := acquireSafely(archiveTwin(t, "root/link", 0o777, kind), root)
		if err != nil || len(report.Rejected) != 1 || len(report.Written) != 0 {
			t.Fatalf("link type %d was not explicitly rejected: report=%+v err=%v", kind, report, err)
		}
	}
}

func archiveTwin(t *testing.T, path string, mode int64, kind byte) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	content := []byte("#!/bin/sh\n: > \"$ADDY_ACCEPTANCE_SENTINEL\"\n")
	header := &tar.Header{Name: path, Mode: mode, Typeflag: kind, Size: int64(len(content))}
	if kind == tar.TypeSymlink || kind == tar.TypeLink {
		header.Size = 0
		header.Linkname = "target"
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if header.Size > 0 {
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestExactUpstreamArchiveInventoryAndSupportRemainInert(t *testing.T) {
	archive := ExactArchive()
	if got := fmt.Sprintf("%x", sha256.Sum256(archive)); got != Canonical().Provenance.ArchiveSHA256 {
		t.Fatalf("exact archive digest = %s", got)
	}
	files, err := InspectExactArchive()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	byPath := map[string]UpstreamFile{}
	for _, file := range files {
		byPath[file.Path] = file
		if filepath.Base(file.Path) == "SKILL.md" && strings.HasPrefix(filepath.ToSlash(file.Path), "skills/") && strings.Count(filepath.ToSlash(file.Path), "/") == 2 {
			counts["skills"]++
		}
		if filepath.Dir(file.Path) == "agents" && filepath.Ext(file.Path) == ".md" {
			counts["agents"]++
		}
		if filepath.Dir(file.Path) == "commands" && filepath.Ext(file.Path) == ".toml" {
			counts["workflows"]++
		}
		if filepath.Dir(file.Path) == "references" && filepath.Ext(file.Path) == ".md" {
			counts["references"]++
		}
	}
	for kind, want := range map[string]int{"skills": 24, "agents": 4, "workflows": 8, "references": 7} {
		if counts[kind] != want {
			t.Fatalf("exact archive %s = %d, want %d", kind, counts[kind], want)
		}
	}
	for _, path := range []string{"LICENSE", "skills/idea-refine/examples.md", "skills/idea-refine/frameworks.md", "skills/idea-refine/refinement-criteria.md", "skills/idea-refine/scripts/idea-refine.sh"} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("exact archive missing required support %s", path)
		}
	}
	for _, resource := range Canonical().Manifest.Resources {
		path := resource.Source
		if resource.Kind == "skill" {
			path += "/SKILL.md"
		}
		if _, ok := byPath[path]; !ok {
			t.Fatalf("canonical resource %s:%s is not backed by exact upstream path %s", resource.Kind, resource.ID, path)
		}
	}
	if byPath["skills/idea-refine/scripts/idea-refine.sh"].Mode&0o111 == 0 {
		t.Fatal("exact helper mode was not retained")
	}
	for _, path := range []string{"hooks/session-start.sh", "hooks/sdd-cache-pre.sh", "hooks/sdd-cache-post.sh", "hooks/simplify-ignore.sh", ".opencode/skills", ".github/workflows/test-plugin-install.yml"} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("exact inert source evidence missing %s", path)
		}
	}

	root := filepath.Join(t.TempDir(), "acquisition")
	sentinel := filepath.Join(t.TempDir(), "executed")
	t.Setenv("ADDY_ACCEPTANCE_SENTINEL", sentinel)
	report, err := AcquireExact(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rejected) != 1 || report.Rejected[0].Path != ".opencode/skills" {
		t.Fatalf("archive links were not explicitly rejected: %+v", report.Rejected)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("upstream content executed during exact acquisition: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "skills")); !os.IsNotExist(err) {
		t.Fatalf("upstream symlink was materialized: %v", err)
	}
}
