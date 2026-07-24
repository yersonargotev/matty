package localprojection

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestStagingFailureRemovesOnlyTransactionCreatedDirectories(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "new", "nested")
	executor := Executor{Host: "test", SymlinkKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionSkillLink: true}}
	err := executor.Apply([]capabilitypack.ProjectionAction{{ID: "skill:missing", Kind: capabilitypack.ActionSkillLink, Source: filepath.Join(root, "missing"), Target: filepath.Join(targetDir, "skill")}})
	if err == nil {
		t.Fatal("broken staged link was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
		t.Fatalf("failed transaction left created directories: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("transaction removed pre-existing parent: %v", err)
	}
}

func TestExecutorDeletesOnlyExplicitTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "managed")
	keep := filepath.Join(root, "keep")
	if err := os.WriteFile(target, []byte("managed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("unmanaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := Executor{Host: "test", FileKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionInstructionFile: true}}
	if err := executor.Apply([]capabilitypack.ProjectionAction{{ID: "instruction:managed", Kind: capabilitypack.ActionInstructionFile, Target: target, Mode: capabilitypack.ProjectionDeleteTarget}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target remains: %v", err)
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "unmanaged" {
		t.Fatalf("unmanaged file changed: %q %v", data, err)
	}
}

func TestReplaceTreePublishesOneExactTreeAndPreservesTargetOnFailedVerification(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "skills", "build")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []TreeFile{
		{Path: "SKILL.md", Content: []byte("canonical\n"), Mode: 0o644},
		{Path: "references/checklist.md", Content: []byte("reference\n"), Mode: 0o644},
		{Path: "scripts/inert.sh", Content: []byte("#!/bin/sh\nexit 97\n"), Mode: 0o755},
	}
	fingerprint, err := FingerprintTreeFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReplaceTree(target, files, "wrong"); err == nil {
		t.Fatal("mismatched sealed fingerprint was accepted")
	}
	if data, err := os.ReadFile(filepath.Join(target, "old.md")); err != nil || string(data) != "old" {
		t.Fatalf("failed stage changed prior target: %q %v", data, err)
	}
	if err := ReplaceTree(target, files, fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "old.md")); !os.IsNotExist(err) {
		t.Fatalf("obsolete target content remains: %v", err)
	}
	got, err := FingerprintExactTree(target)
	if err != nil || got != fingerprint {
		t.Fatalf("published fingerprint = %q, want %q: %v", got, fingerprint, err)
	}
	info, err := os.Stat(filepath.Join(target, "scripts", "inert.sh"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("executable mode = %v: %v", info.Mode().Perm(), err)
	}
}

func TestExactTreeFingerprintRejectsUnsafeOrUnexpectedEntries(t *testing.T) {
	if _, err := FingerprintTreeFiles([]TreeFile{{Path: "../escape", Content: []byte("x"), Mode: 0o644}}); err == nil {
		t.Fatal("traversal path was accepted")
	}
	if _, err := FingerprintTreeFiles([]TreeFile{{Path: "duplicate", Content: []byte("a"), Mode: 0o644}, {Path: "duplicate", Content: []byte("b"), Mode: 0o644}}); err == nil {
		t.Fatal("duplicate path was accepted")
	}
	if _, err := FingerprintTreeFiles([]TreeFile{{Path: "bad-mode", Content: []byte("x"), Mode: 0o600}}); err == nil {
		t.Fatal("noncanonical mode was accepted")
	}
	root := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(root, "unexpected")); err != nil {
		t.Fatal(err)
	}
	if _, err := FingerprintExactTree(root); err == nil {
		t.Fatal("unexpected symlink was accepted")
	}
}
