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
