package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var bundledSkillGroups = []string{"engineering", "productivity"}
var selectedInProgressSkills = []string{"loop-me", "wayfinder"}

func DiscoverManagedSkills(paths Paths) ([]ManagedSkill, error) {
	var skills []ManagedSkill

	for _, group := range bundledSkillGroups {
		groupDir := filepath.Join(paths.SkillSourceRoot, group)
		entries, err := os.ReadDir(groupDir)
		if err != nil {
			return nil, fmt.Errorf("discover %s skills in %s: %w", group, groupDir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skill, err := managedSkillFromSource(paths, filepath.Join(groupDir, entry.Name()))
			if err != nil {
				return nil, err
			}
			skills = append(skills, skill)
		}
	}

	for _, name := range selectedInProgressSkills {
		skill, err := managedSkillFromSource(paths, filepath.Join(paths.SkillSourceRoot, "in-progress", name))
		if err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

func managedSkillFromSource(paths Paths, sourcePath string) (ManagedSkill, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return ManagedSkill{}, fmt.Errorf("source skill %s: %w", sourcePath, err)
	}
	if !info.IsDir() {
		return ManagedSkill{}, fmt.Errorf("source skill %s is not a directory", sourcePath)
	}
	if _, err := os.Stat(filepath.Join(sourcePath, "SKILL.md")); err != nil {
		return ManagedSkill{}, fmt.Errorf("source skill %s missing SKILL.md: %w", sourcePath, err)
	}
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return ManagedSkill{}, fmt.Errorf("resolve source skill %s: %w", sourcePath, err)
	}
	name := filepath.Base(sourcePath)
	return ManagedSkill{Name: name, SourcePath: absSource, LinkPath: paths.SkillLinkPath(name)}, nil
}
