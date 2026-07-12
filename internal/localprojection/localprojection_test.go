package localprojection

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/matty/internal/capabilitypack"
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
