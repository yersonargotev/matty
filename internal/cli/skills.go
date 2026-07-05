package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/matty/internal/skillbundle"
)

func DiscoverManagedSkills(paths Paths) ([]ManagedSkill, error) {
	if err := requireSkillSource(paths); err != nil {
		return nil, err
	}
	skills, err := skillbundle.Discover(paths.SkillSourceRoot, paths.AgentSkillsDir)
	if err != nil {
		if os.IsNotExist(err) && usesDefaultInstalledSkillSource(paths) {
			return nil, missingInstalledSourceError(paths.SkillSourceRoot)
		}
		return nil, err
	}
	managed := make([]ManagedSkill, 0, len(skills))
	for _, skill := range skills {
		managed = append(managed, ManagedSkill{
			Name:       skill.Name,
			SourcePath: skill.SourcePath,
			LinkPath:   skill.LinkPath,
		})
	}
	return managed, nil
}

func requireSkillSource(paths Paths) error {
	info, err := os.Stat(paths.SkillSourceRoot)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("skill source path is not a directory: %s", paths.SkillSourceRoot)
		}
		return nil
	}
	if os.IsNotExist(err) && usesDefaultInstalledSkillSource(paths) {
		return missingInstalledSourceError(paths.SkillSourceRoot)
	}
	return fmt.Errorf("inspect skill source %s: %w", paths.SkillSourceRoot, err)
}

func usesDefaultInstalledSkillSource(paths Paths) bool {
	return filepath.Clean(paths.SkillSourceRoot) == filepath.Clean(filepath.Join(paths.InstalledSourceRoot, "bundle", "skills"))
}

func missingInstalledSourceError(missingPath string) error {
	return fmt.Errorf("Installed Source skill bundle is missing at %s; run matty init to initialize it", missingPath)
}
