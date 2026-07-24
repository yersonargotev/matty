package localprojection

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestReplaceTreesRollsBackEarlierPublicationWhenLaterPublicationFails(t *testing.T) {
	root := t.TempDir()
	targets := []string{filepath.Join(root, "skills", "one"), filepath.Join(root, "skills", "two")}
	for i, target := range targets {
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("old-"+string(rune('1'+i))), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	changes := make([]TreeChange, len(targets))
	for i, target := range targets {
		files := []TreeFile{{Path: "SKILL.md", Content: []byte("new-" + string(rune('1'+i))), Mode: 0o644}}
		fingerprint, err := FingerprintTreeFiles(files)
		if err != nil {
			t.Fatal(err)
		}
		changes[i] = TreeChange{ID: "skill:" + string(rune('1'+i)), Target: target, Files: files, ExpectedFingerprint: fingerprint}
	}

	originalRename := renameTreePath
	t.Cleanup(func() { renameTreePath = originalRename })
	publications := 0
	renameTreePath = func(oldPath, newPath string) error {
		if strings.HasPrefix(filepath.Base(oldPath), ".packy-tree-stage-") && !strings.HasSuffix(oldPath, ".backup") {
			publications++
			if publications == 2 {
				return errors.New("injected second publication failure")
			}
		}
		return originalRename(oldPath, newPath)
	}
	err := ReplaceTrees(changes)
	if err == nil || !strings.Contains(err.Error(), "skill:2") {
		t.Fatalf("error = %v, want second tree identity", err)
	}
	for i, target := range targets {
		data, readErr := os.ReadFile(filepath.Join(target, "SKILL.md"))
		want := "old-" + string(rune('1'+i))
		if readErr != nil || string(data) != want {
			t.Fatalf("target %d after rollback = %q, want %q: %v", i, data, want, readErr)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(targets[0]))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".packy-tree-stage-") {
			t.Fatalf("transaction artifact remains after rollback: %s", entry.Name())
		}
	}
}

func TestReplaceTreesRollsBackEarlierPublicationWhenCurrentRestoreFails(t *testing.T) {
	root := t.TempDir()
	targets := []string{filepath.Join(root, "skills", "one"), filepath.Join(root, "skills", "two")}
	changes := make([]TreeChange, len(targets))
	for i, target := range targets {
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		files := []TreeFile{{Path: "SKILL.md", Content: []byte("new"), Mode: 0o644}}
		fingerprint, err := FingerprintTreeFiles(files)
		if err != nil {
			t.Fatal(err)
		}
		changes[i] = TreeChange{ID: "skill:" + string(rune('1'+i)), Target: target, Files: files, ExpectedFingerprint: fingerprint}
	}

	originalRename := renameTreePath
	t.Cleanup(func() { renameTreePath = originalRename })
	publications := 0
	renameTreePath = func(oldPath, newPath string) error {
		if strings.HasPrefix(filepath.Base(oldPath), ".packy-tree-stage-") && !strings.HasSuffix(oldPath, ".backup") {
			publications++
			if publications == 2 {
				return errors.New("injected second publication failure")
			}
		}
		if strings.HasSuffix(oldPath, ".backup") && newPath == targets[1] {
			return errors.New("injected current restore failure")
		}
		return originalRename(oldPath, newPath)
	}
	err := ReplaceTrees(changes)
	if err == nil || !strings.Contains(err.Error(), "restore current target failed") {
		t.Fatalf("error = %v, want current restore failure", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(targets[0], "SKILL.md")); readErr != nil || string(data) != "old" {
		t.Fatalf("earlier target after rollback = %q, want old: %v", data, readErr)
	}
	if _, statErr := os.Stat(targets[1]); !os.IsNotExist(statErr) {
		t.Fatalf("unrestored current target unexpectedly present: %v", statErr)
	}
	entries, readErr := os.ReadDir(filepath.Dir(targets[1]))
	if readErr != nil {
		t.Fatal(readErr)
	}
	recoveryBackup := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".backup") {
			recoveryBackup = true
		}
	}
	if !recoveryBackup {
		t.Fatal("failed current restore discarded its recovery backup")
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
