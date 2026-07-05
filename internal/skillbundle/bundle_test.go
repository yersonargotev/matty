package skillbundle

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverReportsMissingSourceWithHint(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "bundle", "skills")
	_, err := Discover(missing, t.TempDir(), WithMissingSourceHint("run matty init to initialize it"))
	if err == nil {
		t.Fatal("expected missing source error")
	}
	var missingErr MissingSourceError
	if !errors.As(err, &missingErr) {
		t.Fatalf("error = %T %v, want MissingSourceError", err, err)
	}
	if missingErr.Path != missing {
		t.Fatalf("MissingSourceError.Path = %q, want %q", missingErr.Path, missing)
	}
	for _, want := range []string{missing, "run matty init"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}
