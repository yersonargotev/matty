package cli

import (
	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/skillbundle"
)

func DiscoverManagedSkills(paths Paths) ([]corelifecycle.ManagedSkill, error) {
	skills, err := skillbundle.Discover(paths.SkillSourceRoot, paths.AgentSkillsDir, paths.SkillSourceMissingHint)
	if err != nil {
		return nil, err
	}
	managed := make([]corelifecycle.ManagedSkill, 0, len(skills))
	for _, skill := range skills {
		managed = append(managed, corelifecycle.ManagedSkill{
			Name:       skill.Name,
			SourcePath: skill.SourcePath,
			LinkPath:   skill.LinkPath,
		})
	}
	return managed, nil
}
