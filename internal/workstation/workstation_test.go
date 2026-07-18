package workstation

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveNormalizesWorkstationFacts(t *testing.T) {
	home := t.TempDir()
	absoluteConfig := filepath.Join(t.TempDir(), "config")
	cwd := t.TempDir()

	tests := []struct {
		name       string
		configHome string
		wantConfig string
	}{
		{name: "absolute XDG configuration", configHome: absoluteConfig, wantConfig: absoluteConfig},
		{name: "missing XDG configuration", wantConfig: filepath.Join(home, ".config")},
		{name: "relative XDG configuration", configHome: "relative-config", wantConfig: filepath.Join(home, ".config")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Resolve(Inputs{
				Home:                 home,
				ConfigurationHome:    tt.configHome,
				ExecutableSearchPath: "/sandbox/bin",
				HomebrewPrefix:       "/sandbox/homebrew",
				CurrentDirectory:     cwd,
			}, Options{})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if snapshot.Home() != home || snapshot.ConfigurationHome() != tt.wantConfig {
				t.Fatalf("home facts = %q, %q; want %q, %q", snapshot.Home(), snapshot.ConfigurationHome(), home, tt.wantConfig)
			}
			currentDirectory, currentDirectoryErr := snapshot.CurrentDirectory()
			if snapshot.ExecutableSearchPath() != "/sandbox/bin" || snapshot.HomebrewPrefix() != "/sandbox/homebrew" || currentDirectory != cwd || currentDirectoryErr != nil {
				t.Fatalf("ambient facts were not preserved: %#v", snapshot)
			}
			if snapshot.PackyHome() != filepath.Join(home, ".packy") {
				t.Fatalf("PackyHome = %q", snapshot.PackyHome())
			}
		})
	}
}

func TestResolveRequiresHome(t *testing.T) {
	_, err := Resolve(Inputs{CurrentDirectory: t.TempDir()}, Options{})
	if err == nil || err.Error() != "HOME is required" {
		t.Fatalf("error = %v; want HOME is required", err)
	}
}

func TestResolveExplicitHomeIgnoresAmbientConfiguration(t *testing.T) {
	ambientHome := t.TempDir()
	overrideHome := t.TempDir()
	snapshot, err := Resolve(Inputs{
		Home:              ambientHome,
		ConfigurationHome: filepath.Join(ambientHome, "xdg"),
		CurrentDirectory:  t.TempDir(),
	}, Options{Home: overrideHome})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if snapshot.Home() != overrideHome || snapshot.ConfigurationHome() != filepath.Join(overrideHome, ".config") {
		t.Fatalf("override facts = %q, %q", snapshot.Home(), snapshot.ConfigurationHome())
	}
}

func TestResolverIsLazyAndReusesImmutableSnapshot(t *testing.T) {
	home := t.TempDir()
	captures := 0
	resolver := NewResolver(func() (Inputs, error) {
		captures++
		return Inputs{Home: home, CurrentDirectory: t.TempDir()}, nil
	})
	if captures != 0 {
		t.Fatalf("captured inputs eagerly")
	}

	first, err := resolver.Resolve(Options{})
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	second, err := resolver.Resolve(Options{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if captures != 1 {
		t.Fatalf("captures = %d; want 1", captures)
	}
	if first != second {
		t.Fatalf("snapshot changed between resolutions: %#v != %#v", first, second)
	}
}

func TestResolverCachesCaptureFailure(t *testing.T) {
	wantErr := errors.New("capture failed")
	captures := 0
	resolver := NewResolver(func() (Inputs, error) {
		captures++
		return Inputs{}, wantErr
	})
	for range 2 {
		_, err := resolver.Resolve(Options{})
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v", err)
		}
	}
	if captures != 1 {
		t.Fatalf("captures = %d; want 1", captures)
	}
}
