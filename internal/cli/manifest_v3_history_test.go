package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func copyArchivedEngramFixture(t *testing.T, bundle, repoRoot string) {
	t.Helper()
	source := filepath.Join(repoRoot, "bundle", "history", "engram", "1.0.0")
	target := filepath.Join(bundle, "history", "engram", "1.0.0")
	if err := os.CopyFS(target, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
}
