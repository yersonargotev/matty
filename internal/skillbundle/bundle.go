package skillbundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/yersonargotev/matty/internal/bundletransaction"
)

var defaultGroups = []string{"engineering", "productivity"}
var selectedInProgress = []string{"loop-me"}

type SourceOrigin string

const (
	SourceOriginOverride   SourceOrigin = "override"
	SourceOriginRepository SourceOrigin = "repo"
	SourceOriginInstalled  SourceOrigin = "installed"
)

// InstalledSource is the narrow bootstrap-owned descriptor consumed when the
// package installation is the selected fallback.
type InstalledSource interface {
	BundleRoot() string
}

// SourceOptions supplies the process-specific candidates used to select a
// skill source. Callers own environment and cwd lookup; this package owns
// precedence and the Matty bundle layout.
type SourceOptions struct {
	ExplicitRoot    string
	RepositoryStart string
	InstalledSource InstalledSource
}

type Source struct {
	Root        string
	MissingHint string
	IsDefault   bool
	Origin      SourceOrigin
}

// ResolveSource selects an explicit development source, then a repository
// ancestor, then the package Installed Source. Selection is deliberately
// separate from Discover validation so path-only commands can still inspect a
// missing installation and mutating commands fail when they request resources.
func ResolveSource(opts SourceOptions) (Source, error) {
	repositoryStart, err := filepath.Abs(opts.RepositoryStart)
	if err != nil {
		return Source{}, fmt.Errorf("resolve repository start: %w", err)
	}
	if opts.ExplicitRoot != "" {
		root := opts.ExplicitRoot
		if !filepath.IsAbs(root) {
			root = filepath.Join(repositoryStart, root)
		}
		return Source{Root: filepath.Clean(root), Origin: SourceOriginOverride}, nil
	}

	for dir := repositoryStart; ; dir = filepath.Dir(dir) {
		candidate := SourceRoot(dir)
		if SourceRootExists(candidate) {
			return Source{Root: candidate, Origin: SourceOriginRepository}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return Source{
		Root:        InstalledSourceRoot(opts.InstalledSource),
		MissingHint: "run matty init to initialize it",
		IsDefault:   true,
		Origin:      SourceOriginInstalled,
	}, nil
}

// InstalledSourceRoot derives the skill source beneath bootstrap's Installed
// Source bundle without reacquiring checkout layout knowledge.
func InstalledSourceRoot(source InstalledSource) string {
	return filepath.Join(source.BundleRoot(), "skills")
}

func SourceRoot(mattyRoot string) string {
	return filepath.Join(mattyRoot, "bundle", "skills")
}

// BundleRoot returns the Matty-owned bundle containing a selected skill source.
// Keeping this physical relationship here prevents capability modules from
// learning the source tree layout.
func BundleRoot(skillSourceRoot string) string {
	return filepath.Dir(filepath.Clean(skillSourceRoot))
}

func SourceRootExists(sourceRoot string) bool {
	exists := false
	repositoryRoot := filepath.Dir(BundleRoot(sourceRoot))
	err := bundletransaction.WithExclusive(context.Background(), repositoryRoot, func() error {
		info, statErr := os.Stat(sourceRoot)
		exists = statErr == nil && info.IsDir()
		return nil
	})
	return err == nil && exists
}

// Skill is the installer's ownership metadata for one bundled skill.
type Skill struct {
	Name       string
	SourcePath string
	LinkPath   string
}

// MissingSourceError reports a selected bundle source that does not exist.
type MissingSourceError struct {
	Path string
	Hint string
}

func (err MissingSourceError) Error() string {
	if err.Hint == "" {
		return fmt.Sprintf("skill source is missing at %s", err.Path)
	}
	return fmt.Sprintf("skill source is missing at %s; %s", err.Path, err.Hint)
}

// MalformedSourceError reports a selected source that exists but does not
// satisfy the Matty-owned bundle structure.
type MalformedSourceError struct {
	Path string
	Err  error
}

func (err MalformedSourceError) Error() string {
	return fmt.Sprintf("skill source is malformed at %s: %v", err.Path, err.Err)
}

func (err MalformedSourceError) Unwrap() error {
	return err.Err
}

// Discover returns Matty's v0 skill bundle from a Matty-owned source root.
// The root is expected to contain engineering/, productivity/, and the selected
// in-progress/ skills. Callers provide linkDir so this package owns the bundle
// shape without knowing HOME or CLI state details. missingSourceHint adds
// source-selection context to a MissingSourceError without moving validation out
// of this package.
func Discover(sourceRoot, linkDir, missingSourceHint string) ([]Skill, error) {
	var skills []Skill
	err := bundletransaction.WithExclusive(context.Background(), transactionRoot(sourceRoot), func() error {
		var err error
		skills, err = discover(sourceRoot, linkDir, missingSourceHint)
		return err
	})
	return skills, err
}

func discover(sourceRoot, linkDir, missingSourceHint string) ([]Skill, error) {
	if err := requireSourceRoot(sourceRoot, missingSourceHint); err != nil {
		return nil, err
	}

	var skills []Skill

	for _, group := range defaultGroups {
		groupDir := filepath.Join(sourceRoot, group)
		entries, err := os.ReadDir(groupDir)
		if err != nil {
			return nil, malformedSource(sourceRoot, fmt.Errorf("discover %s skills in %s: %w", group, groupDir, err))
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skill, err := fromSource(linkDir, filepath.Join(groupDir, entry.Name()))
			if err != nil {
				return nil, malformedSource(sourceRoot, err)
			}
			skills = append(skills, skill)
		}
	}

	for _, name := range selectedInProgress {
		skill, err := fromSource(linkDir, filepath.Join(sourceRoot, "in-progress", name))
		if err != nil {
			return nil, malformedSource(sourceRoot, err)
		}
		skills = append(skills, skill)
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

func transactionRoot(sourceRoot string) string {
	bundleRoot := BundleRoot(sourceRoot)
	root := bundleRoot
	if filepath.Base(bundleRoot) == "bundle" {
		root = filepath.Dir(bundleRoot)
	}
	for {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			return root
		}
		root = parent
	}
}

// ValidateSource verifies that a selected root contains the complete Matty v0
// skill structure without writing or publishing any resources.
func ValidateSource(sourceRoot, missingSourceHint string) error {
	_, err := Discover(sourceRoot, "", missingSourceHint)
	return err
}

func requireSourceRoot(sourceRoot, missingSourceHint string) error {
	info, err := os.Stat(sourceRoot)
	if err == nil {
		if !info.IsDir() {
			return malformedSource(sourceRoot, fmt.Errorf("skill source path is not a directory: %s", sourceRoot))
		}
		return nil
	}
	if os.IsNotExist(err) {
		return MissingSourceError{Path: sourceRoot, Hint: missingSourceHint}
	}
	return fmt.Errorf("inspect skill source %s: %w", sourceRoot, err)
}

func malformedSource(sourceRoot string, err error) error {
	return MalformedSourceError{Path: sourceRoot, Err: err}
}

func fromSource(linkDir, sourcePath string) (Skill, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return Skill{}, fmt.Errorf("source skill %s: %w", sourcePath, err)
	}
	if !info.IsDir() {
		return Skill{}, fmt.Errorf("source skill %s is not a directory", sourcePath)
	}
	if _, err := os.Stat(filepath.Join(sourcePath, "SKILL.md")); err != nil {
		return Skill{}, fmt.Errorf("source skill %s missing SKILL.md: %w", sourcePath, err)
	}
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return Skill{}, fmt.Errorf("resolve source skill %s: %w", sourcePath, err)
	}
	name := filepath.Base(sourcePath)
	return Skill{Name: name, SourcePath: absSource, LinkPath: (GlobalLayout{root: linkDir}).Skill(name)}, nil
}
