package skillbundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var defaultGroups = []string{"engineering", "productivity"}
var selectedInProgress = []string{"loop-me"}

func SourceRoot(mattyRoot string) string {
	return filepath.Join(mattyRoot, "bundle", "skills")
}

func SourceRootExists(sourceRoot string) bool {
	info, err := os.Stat(sourceRoot)
	return err == nil && info.IsDir()
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

// Discover returns Matty's v0 skill bundle from a Matty-owned source root.
// The root is expected to contain engineering/, productivity/, and the selected
// in-progress/ skills. Callers provide linkDir so this package owns the bundle
// shape without knowing HOME or CLI state details. missingSourceHint adds
// source-selection context to a MissingSourceError without moving validation out
// of this package.
func Discover(sourceRoot, linkDir, missingSourceHint string) ([]Skill, error) {
	if err := requireSourceRoot(sourceRoot, missingSourceHint); err != nil {
		return nil, err
	}

	var skills []Skill

	for _, group := range defaultGroups {
		groupDir := filepath.Join(sourceRoot, group)
		entries, err := os.ReadDir(groupDir)
		if err != nil {
			return nil, fmt.Errorf("discover %s skills in %s: %w", group, groupDir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skill, err := fromSource(linkDir, filepath.Join(groupDir, entry.Name()))
			if err != nil {
				return nil, err
			}
			skills = append(skills, skill)
		}
	}

	for _, name := range selectedInProgress {
		skill, err := fromSource(linkDir, filepath.Join(sourceRoot, "in-progress", name))
		if err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

func requireSourceRoot(sourceRoot, missingSourceHint string) error {
	info, err := os.Stat(sourceRoot)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("skill source path is not a directory: %s", sourceRoot)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return MissingSourceError{Path: sourceRoot, Hint: missingSourceHint}
	}
	return fmt.Errorf("inspect skill source %s: %w", sourceRoot, err)
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
	return Skill{Name: name, SourcePath: absSource, LinkPath: filepath.Join(linkDir, name)}, nil
}
