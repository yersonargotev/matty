package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths contains every global path Matty v0 will manage or inspect. Keeping
// this derived from injected Env makes command tests independent from real HOME.
type Paths struct {
	HomeDir         string
	ConfigHome      string
	MattyDir        string
	StateFile       string
	AgentSkillsDir  string
	SkillSourceRoot string
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
	skillSourceRoot, err := resolveSkillSourceRoot(env)
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		HomeDir:         home,
		ConfigHome:      configHome,
		MattyDir:        mattyDir,
		StateFile:       filepath.Join(mattyDir, "config.json"),
		AgentSkillsDir:  filepath.Join(home, ".agents", "skills"),
		SkillSourceRoot: skillSourceRoot,
	}, nil
}

func (p Paths) SkillLinkPath(name string) string {
	return filepath.Join(p.AgentSkillsDir, name)
}

func resolveSkillSourceRoot(env Env) (string, error) {
	configured := env.Getenv("MATTY_SKILLS_SOURCE")
	if configured != "" {
		return filepath.Abs(configured)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve skill source root: %w", err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "skills", "skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return filepath.Abs(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return filepath.Abs(filepath.Join(cwd, "skills", "skills"))
}
