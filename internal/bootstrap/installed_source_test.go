package bootstrap

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/workstation"
)

func TestResolveInstalledSource(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	absoluteRoot := filepath.Join(t.TempDir(), "installed")
	snapshot, err := workstation.Resolve(workstation.Inputs{Home: home, CurrentDirectory: cwd}, workstation.Options{})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		explicitRoot string
		wantRoot     string
	}{
		{name: "default", wantRoot: filepath.Join(home, ".local", "share", "packy")},
		{name: "absolute override", explicitRoot: absoluteRoot, wantRoot: absoluteRoot},
		{name: "relative override", explicitRoot: filepath.Join("relative", "installed"), wantRoot: filepath.Join(cwd, "relative", "installed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installed, err := ResolveInstalledSource(snapshot, tt.explicitRoot)
			if err != nil {
				t.Fatalf("ResolveInstalledSource: %v", err)
			}
			if installed.Root() != tt.wantRoot {
				t.Fatalf("Root = %q; want %q", installed.Root(), tt.wantRoot)
			}
			if installed.BundleRoot() != filepath.Join(tt.wantRoot, "bundle") {
				t.Fatalf("BundleRoot = %q", installed.BundleRoot())
			}
		})
	}
}

func TestResolveInstalledSourceNeedsCurrentDirectoryOnlyForRelativeRoot(t *testing.T) {
	home := t.TempDir()
	wantErr := errors.New("cwd unavailable")
	snapshot, err := workstation.Resolve(workstation.Inputs{
		Home:                home,
		CurrentDirectoryErr: wantErr,
	}, workstation.Options{})
	if err != nil {
		t.Fatal(err)
	}

	absoluteRoot := filepath.Join(t.TempDir(), "installed")
	installed, err := ResolveInstalledSource(snapshot, absoluteRoot)
	if err != nil || installed.Root() != absoluteRoot {
		t.Fatalf("absolute root = %q, %v", installed.Root(), err)
	}
	if _, err := ResolveInstalledSource(snapshot, filepath.Join("relative", "installed")); !errors.Is(err, wantErr) {
		t.Fatalf("relative root error = %v; want %v", err, wantErr)
	}
}

func TestInstalledSourceAtOwnsCheckoutAndBundleLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "installed")
	installed := InstalledSourceAt(root)
	if installed.Root() != root {
		t.Fatalf("Root = %q, want %q", installed.Root(), root)
	}
	if installed.BundleRoot() != filepath.Join(root, "bundle") {
		t.Fatalf("BundleRoot = %q", installed.BundleRoot())
	}
}

func TestValidateInstalledSourceRefUsesDescriptor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "descriptor-source")
	err := ValidateInstalledSourceRef(BootstrapOptions{
		InstalledSource: InstalledSourceAt(root),
		RepositoryRef:   "v1.2.3",
	})
	if err == nil || !strings.Contains(err.Error(), filepath.Join(root, "bundle", "skills")) {
		t.Fatalf("ValidateInstalledSourceRef error = %v, want descriptor path", err)
	}
}

func TestBootstrapOptionsHasOneInstalledSourceLocation(t *testing.T) {
	typeOfOptions := reflect.TypeOf(BootstrapOptions{})
	if _, found := typeOfOptions.FieldByName("SourceRoot"); found {
		t.Fatal("BootstrapOptions retains legacy SourceRoot beside InstalledSource")
	}
}
