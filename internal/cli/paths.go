package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths contains every global path Matty v0 will manage or inspect. Keeping
// this derived from injected Env makes command tests independent from real HOME.
type Paths struct {
	HomeDir                string
	ConfigHome             string
	MattyDir               string
	StateFile              string
	AgentSkillsDir         string
	InstalledSourceRoot    string
	SkillSourceRoot        string
	SkillSourceMissingHint string
	SkillSourceIsDefault   bool
	CodexPromptFile        string
	OpenCodeConfigFile     string
	OpenCodePromptFile     string
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
	installedSourceRoot := DefaultInstalledSourceRoot(home)
	skillSourceRoot, skillSourceMissingHint, skillSourceIsDefault, err := resolveSkillSourceRoot(env, installedSourceRoot)
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		HomeDir:                home,
		ConfigHome:             configHome,
		MattyDir:               mattyDir,
		StateFile:              filepath.Join(mattyDir, "config.json"),
		AgentSkillsDir:         filepath.Join(home, ".agents", "skills"),
		InstalledSourceRoot:    installedSourceRoot,
		SkillSourceRoot:        skillSourceRoot,
		SkillSourceMissingHint: skillSourceMissingHint,
		SkillSourceIsDefault:   skillSourceIsDefault,
		CodexPromptFile:        filepath.Join(home, ".codex", "AGENTS.md"),
		OpenCodeConfigFile:     filepath.Join(configHome, "opencode", "opencode.json"),
		OpenCodePromptFile:     filepath.Join(configHome, "opencode", "matty.md"),
	}, nil
}

func (p Paths) SkillLinkPath(name string) string {
	return filepath.Join(p.AgentSkillsDir, name)
}

func resolveSkillSourceRoot(env Env, installedSourceRoot string) (string, string, bool, error) {
	configured := env.Getenv("MATTY_SKILLS_SOURCE")
	if configured != "" {
		path, err := filepath.Abs(configured)
		return path, "", false, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", "", false, fmt.Errorf("resolve skill source root: %w", err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "bundle", "skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			path, err := filepath.Abs(candidate)
			return path, "", false, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	path, err := filepath.Abs(filepath.Join(installedSourceRoot, "bundle", "skills"))
	return path, "run matty init to initialize it", true, err
}

func DefaultInstalledSourceRoot(home string) string {
	return filepath.Join(home, ".local", "share", "matty")
}
