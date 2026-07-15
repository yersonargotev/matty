package skillbundle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/bundletransaction"
)

type installedSourceStub struct{ bundleRoot string }

func (source installedSourceStub) BundleRoot() string { return source.bundleRoot }

func TestDiscoverWaitsForCompleteBundleTransaction(t *testing.T) {
	repository := t.TempDir()
	source := SourceRoot(repository)
	writeValidSkillSource(t, source)
	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Discover(source, t.TempDir(), "")
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Discover completed outside the shared lock: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Discover did not resume after the bundle transaction")
	}
}

func TestResolveSourceWaitsBetweenBundleRenames(t *testing.T) {
	repository := t.TempDir()
	repositorySource := SourceRoot(repository)
	writeValidSkillSource(t, repositorySource)
	stageRoot := filepath.Join(repository, "bundle-stage")
	writeValidSkillSource(t, filepath.Join(stageRoot, "skills"))
	installedRoot := t.TempDir()
	writeValidSkillSource(t, SourceRoot(installedRoot))
	installed := installedSourceStub{bundleRoot: filepath.Join(installedRoot, "bundle")}

	guard, err := bundletransaction.Acquire(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(repository, "bundle-backup")
	if err := os.Rename(filepath.Join(repository, "bundle"), backup); err != nil {
		t.Fatal(err)
	}
	resolved := make(chan Source, 1)
	resolveErr := make(chan error, 1)
	go func() {
		source, err := ResolveSource(SourceOptions{RepositoryStart: repository, InstalledSource: installed})
		if err != nil {
			resolveErr <- err
			return
		}
		resolved <- source
	}()
	select {
	case source := <-resolved:
		t.Fatalf("ResolveSource observed the between-renames gap and selected %+v", source)
	case err := <-resolveErr:
		t.Fatal(err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := os.Rename(stageRoot, filepath.Join(repository, "bundle")); err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case source := <-resolved:
		if source.Origin != SourceOriginRepository || source.Root != repositorySource {
			t.Fatalf("ResolveSource=%+v, want the complete new repository source", source)
		}
	case err := <-resolveErr:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("ResolveSource did not resume after the bundle transaction")
	}
}

func TestResolveSourcePrecedenceAndFallbacks(t *testing.T) {
	repositoryRoot := t.TempDir()
	repositorySource := SourceRoot(repositoryRoot)
	writeValidSkillSource(t, repositorySource)
	installedRoot := t.TempDir()
	installedSource := SourceRoot(installedRoot)
	writeValidSkillSource(t, installedSource)
	installed := installedSourceStub{bundleRoot: filepath.Join(installedRoot, "bundle")}
	override := filepath.Join(t.TempDir(), "explicit-skills")
	writeValidSkillSource(t, override)

	tests := []struct {
		name      string
		opts      SourceOptions
		wantRoot  string
		origin    SourceOrigin
		isDefault bool
	}{
		{
			name:     "explicit override wins over repository and installed sources",
			opts:     SourceOptions{ExplicitRoot: override, RepositoryStart: filepath.Join(repositoryRoot, "nested"), InstalledSource: installed},
			wantRoot: override, origin: SourceOriginOverride,
		},
		{
			name:     "repository source wins over initialized installed source",
			opts:     SourceOptions{RepositoryStart: filepath.Join(repositoryRoot, "nested"), InstalledSource: installed},
			wantRoot: repositorySource, origin: SourceOriginRepository,
		},
		{
			name:     "initialized installed source is the package fallback",
			opts:     SourceOptions{RepositoryStart: t.TempDir(), InstalledSource: installed},
			wantRoot: installedSource, origin: SourceOriginInstalled, isDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.MkdirAll(tt.opts.RepositoryStart, 0o700); err != nil {
				t.Fatal(err)
			}
			got, err := ResolveSource(tt.opts)
			if err != nil {
				t.Fatalf("ResolveSource: %v", err)
			}
			if got.Root != tt.wantRoot || got.Origin != tt.origin || got.IsDefault != tt.isDefault {
				t.Fatalf("ResolveSource = %#v, want root=%q origin=%q default=%t", got, tt.wantRoot, tt.origin, tt.isDefault)
			}
		})
	}
}

func TestResolveSourceKeepsInvalidExplicitOverrideSelected(t *testing.T) {
	repositoryRoot := t.TempDir()
	writeValidSkillSource(t, SourceRoot(repositoryRoot))
	installedRoot := t.TempDir()
	writeValidSkillSource(t, SourceRoot(installedRoot))
	missingOverride := filepath.Join(t.TempDir(), "missing")

	got, err := ResolveSource(SourceOptions{
		ExplicitRoot:    missingOverride,
		RepositoryStart: repositoryRoot,
		InstalledSource: installedSourceStub{bundleRoot: filepath.Join(installedRoot, "bundle")},
	})
	if err != nil {
		t.Fatalf("ResolveSource: %v", err)
	}
	if got.Root != missingOverride || got.Origin != SourceOriginOverride || got.MissingHint != "" || got.IsDefault {
		t.Fatalf("ResolveSource = %#v, want invalid override preserved without Installed Source guidance", got)
	}
	_, err = Discover(got.Root, t.TempDir(), got.MissingHint)
	if err == nil || strings.Contains(err.Error(), "matty init") {
		t.Fatalf("explicit override error = %v, want stable missing error without Installed Source guidance", err)
	}
}

func TestResolveSourceMakesRelativeOverrideAbsoluteFromRepositoryStart(t *testing.T) {
	start := t.TempDir()
	got, err := ResolveSource(SourceOptions{ExplicitRoot: filepath.Join("fixtures", "skills"), RepositoryStart: start, InstalledSource: installedSourceStub{bundleRoot: filepath.Join(t.TempDir(), "bundle")}})
	if err != nil {
		t.Fatalf("ResolveSource: %v", err)
	}
	want := filepath.Join(start, "fixtures", "skills")
	if got.Root != want || !filepath.IsAbs(got.Root) || got.Origin != SourceOriginOverride {
		t.Fatalf("ResolveSource = %#v, want absolute override %q", got, want)
	}
}

func TestResolveSourceMissingInstalledFallbackCarriesInitializationGuidance(t *testing.T) {
	installedRoot := filepath.Join(t.TempDir(), "installed")
	got, err := ResolveSource(SourceOptions{RepositoryStart: t.TempDir(), InstalledSource: installedSourceStub{bundleRoot: filepath.Join(installedRoot, "bundle")}})
	if err != nil {
		t.Fatalf("ResolveSource: %v", err)
	}
	_, err = Discover(got.Root, t.TempDir(), got.MissingHint)
	if err == nil {
		t.Fatal("expected missing Installed Source error")
	}
	for _, want := range []string{SourceRoot(installedRoot), "run matty init"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestBundleRootOwnsPhysicalSourceLayout(t *testing.T) {
	want := filepath.Join(t.TempDir(), "bundle")
	if got := BundleRoot(filepath.Join(want, "skills")); got != want {
		t.Fatalf("BundleRoot = %q, want %q", got, want)
	}
}

func TestDiscoverReportsMissingSourceWithHint(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "bundle", "skills")
	_, err := Discover(missing, t.TempDir(), "run matty init to initialize it")
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

func TestDiscoverValidatesRepresentativeSkillResources(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "bundle", "skills")
	writeValidSkillSource(t, sourceRoot)
	linkDir := t.TempDir()

	skills, err := Discover(sourceRoot, linkDir, "")
	if err != nil {
		t.Fatalf("Discover valid source: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("Discover returned %d skills, want 3", len(skills))
	}
	for _, skill := range skills {
		if !filepath.IsAbs(skill.SourcePath) || skill.LinkPath != filepath.Join(linkDir, skill.Name) {
			t.Fatalf("invalid discovered skill: %#v", skill)
		}
	}
}

func TestDiscoverRejectsMalformedSkillResources(t *testing.T) {
	tests := []struct {
		name        string
		breakSource func(*testing.T, string)
		want        string
	}{
		{
			name:        "missing required group",
			breakSource: func(t *testing.T, root string) { t.Helper(); os.RemoveAll(filepath.Join(root, "productivity")) },
			want:        "discover productivity skills",
		},
		{
			name: "skill missing manifest",
			breakSource: func(t *testing.T, root string) {
				t.Helper()
				os.Remove(filepath.Join(root, "engineering", "ask-matt", "SKILL.md"))
			},
			want: "missing SKILL.md",
		},
		{
			name: "selected skill path is not a directory",
			breakSource: func(t *testing.T, root string) {
				t.Helper()
				path := filepath.Join(root, "in-progress", "loop-me")
				os.RemoveAll(path)
				if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "is not a directory",
		},
		{
			name: "source root is not a directory",
			breakSource: func(t *testing.T, root string) {
				t.Helper()
				if err := os.RemoveAll(root); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(root, []byte("invalid"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "source path is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := filepath.Join(t.TempDir(), "bundle", "skills")
			writeValidSkillSource(t, sourceRoot)
			tt.breakSource(t, sourceRoot)
			_, err := Discover(sourceRoot, t.TempDir(), "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Discover error = %v, want containing %q", err, tt.want)
			}
			var malformed MalformedSourceError
			if !errors.As(err, &malformed) || malformed.Path != sourceRoot {
				t.Fatalf("Discover error = %T %v, want MalformedSourceError for %s", err, err, sourceRoot)
			}
		})
	}
}

func writeValidSkillSource(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		"engineering/ask-matt/SKILL.md",
		"productivity/grilling/SKILL.md",
		"in-progress/loop-me/SKILL.md",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
