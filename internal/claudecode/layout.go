package claudecode

import "path/filepath"

// CanonicalLayout is the complete user-global Claude Code filesystem surface.
// It is derived only from the home supplied by the composition root.
type CanonicalLayout struct {
	Home, ConfigDir, SkillsDir, AgentsDir, InstructionsFile, SettingsFile, UserMCPFile string
}

func NewCanonicalLayout(home string) CanonicalLayout {
	config := filepath.Join(filepath.Clean(home), ".claude")
	return CanonicalLayout{
		Home: filepath.Clean(home), ConfigDir: config,
		SkillsDir: filepath.Join(config, "skills"), AgentsDir: filepath.Join(config, "agents"),
		InstructionsFile: filepath.Join(config, "CLAUDE.md"), SettingsFile: filepath.Join(config, "settings.json"),
		UserMCPFile: filepath.Join(filepath.Clean(home), ".claude.json"),
	}
}
