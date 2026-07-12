package engrambin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolverUsesHomebrewIdentityWithoutExecutingEngram(t *testing.T) {
	prefix := t.TempDir()
	path := filepath.Join(prefix, "bin", "engram")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not executed"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(prefix, func(string) (string, error) {
		return path, nil
	})
	resolution, err := resolver.Resolve(context.Background(), "engram")
	if err != nil {
		t.Fatal(err)
	}
	if !resolution.Available || resolution.Path != path || resolution.Origin != "homebrew" || resolution.Precondition == "" {
		t.Fatalf("resolution = %+v", resolution)
	}
}

func TestResolverReportsSupportedHomebrewAcquisitionWhenMissing(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "homebrew")
	resolver := NewResolver(prefix, func(string) (string, error) { return "", os.ErrNotExist })
	resolution, err := resolver.Resolve(context.Background(), "engram")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Available || !resolution.AcquisitionSupported || resolution.AcquisitionCommand != "brew" || !strings.Contains(strings.Join(resolution.AcquisitionArgs, " "), Formula) {
		t.Fatalf("missing resolution = %+v", resolution)
	}
	if resolution.Path != filepath.Join(prefix, "bin", "engram") {
		t.Fatalf("missing path = %q", resolution.Path)
	}
}
