package cli

import "github.com/yersonargotev/matty/internal/skillbundle"

func DiscoverManagedSkills(paths Paths) ([]ManagedSkill, error) {
	skills, err := skillbundle.Discover(
		paths.SkillSourceRoot,
		paths.AgentSkillsDir,
		skillbundle.WithMissingSourceHint(paths.SkillSourceMissingHint),
	)
	if err != nil {
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
