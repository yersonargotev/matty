package cli

import (
	"fmt"
	"path/filepath"
)

// Paths contains every global path Matty v0 will manage or inspect. Keeping
// this derived from injected Env makes command tests independent from real HOME.
type Paths struct {
	HomeDir        string
	ConfigHome     string
	MattyDir       string
	StateFile      string
	AgentSkillsDir string
}

func ResolvePaths(env Env) (Paths, error) {
	home := env.Getenv("HOME")
	if home == "" {
		return Paths{}, fmt.Errorf("HOME is required")
	}

	configHome := env.Getenv("XDG_CONFIG_HOME")
	if configHome == "" || !filepath.IsAbs(configHome) {
		configHome = filepath.Join(home, ".config")
	}

	mattyDir := filepath.Join(home, ".matty")
	return Paths{
		HomeDir:        home,
		ConfigHome:     configHome,
		MattyDir:       mattyDir,
		StateFile:      filepath.Join(mattyDir, "config.json"),
		AgentSkillsDir: filepath.Join(home, ".agents", "skills"),
	}, nil
}

func (p Paths) SkillLinkPath(name string) string {
	return filepath.Join(p.AgentSkillsDir, name)
}
