package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/skillbundle"
)

type SkillSourceOrigin = skillbundle.SourceOrigin

type SkillSource struct {
	Root        string
	MissingHint string
	IsDefault   bool
	Origin      SkillSourceOrigin
}

const (
	SkillSourceOriginOverride  = skillbundle.SourceOriginOverride
	SkillSourceOriginRepo      = skillbundle.SourceOriginRepository
	SkillSourceOriginInstalled = skillbundle.SourceOriginInstalled
)

// Paths contains every global path Matty v0 will manage or inspect. Keeping
// this derived from injected Env makes command tests independent from real HOME.
type Paths struct {
	HomeDir                string
	PathEnv                string
	HomebrewPrefixEnv      string
	ConfigHome             string
	MattyDir               string
	StateFile              string
	PackStateFile          string
	AgentSkillsDir         string
	InstalledSourceRoot    string
	SkillSourceRoot        string
	BundleSourceRoot       string
	SkillSourceMissingHint string
	SkillSourceIsDefault   bool
	SkillSourceOrigin      SkillSourceOrigin
	CodexConfigFile        string
	CodexPromptFile        string
	OpenCodeConfigFile     string
	OpenCodePromptFile     string
	LocalBinEngram         string
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
	skillSource, err := resolveSkillSourceRoot(env, installedSourceRoot)
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		HomeDir:                home,
		PathEnv:                env.Getenv("PATH"),
		HomebrewPrefixEnv:      env.Getenv("HOMEBREW_PREFIX"),
		ConfigHome:             configHome,
		MattyDir:               mattyDir,
		StateFile:              filepath.Join(mattyDir, "config.json"),
		PackStateFile:          filepath.Join(mattyDir, "packs.json"),
		AgentSkillsDir:         filepath.Join(home, ".agents", "skills"),
		InstalledSourceRoot:    installedSourceRoot,
		SkillSourceRoot:        skillSource.Root,
		BundleSourceRoot:       skillbundle.BundleRoot(skillSource.Root),
		SkillSourceMissingHint: skillSource.MissingHint,
		SkillSourceIsDefault:   skillSource.IsDefault,
		SkillSourceOrigin:      skillSource.Origin,
		CodexConfigFile:        filepath.Join(home, ".codex", "config.toml"),
		CodexPromptFile:        filepath.Join(home, ".codex", "AGENTS.md"),
		OpenCodeConfigFile:     filepath.Join(configHome, "opencode", "opencode.json"),
		OpenCodePromptFile:     filepath.Join(configHome, "opencode", "matty.md"),
		LocalBinEngram:         filepath.Join(home, ".local", "bin", "engram"),
	}, nil
}

func (p Paths) SkillLinkPath(name string) string {
	return filepath.Join(p.AgentSkillsDir, name)
}

func classicStateConfig(paths Paths) corelifecycle.StateConfig {
	return corelifecycle.StateConfig{StateFile: paths.StateFile, AgentSkillsDir: paths.AgentSkillsDir}
}

func classicLifecycleConfig(paths Paths, runningVersion string) corelifecycle.Config {
	return corelifecycle.Config{
		HomeDir:                paths.HomeDir,
		ConfigHome:             paths.ConfigHome,
		MattyDir:               paths.MattyDir,
		StateFile:              paths.StateFile,
		AgentSkillsDir:         paths.AgentSkillsDir,
		SkillSourceRoot:        paths.SkillSourceRoot,
		SkillSourceMissingHint: paths.SkillSourceMissingHint,
		CodexPromptFile:        paths.CodexPromptFile,
		OpenCodeConfigFile:     paths.OpenCodeConfigFile,
		OpenCodePromptFile:     paths.OpenCodePromptFile,
		HomebrewPrefix:         paths.HomebrewPrefixEnv,
		InstalledSourceRoot:    paths.InstalledSourceRoot,
		SkillSourceIsDefault:   paths.SkillSourceIsDefault,
		RunningVersion:         runningVersion,
	}
}

func resolveSkillSourceRoot(env Env, installedSourceRoot string) (SkillSource, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return SkillSource{}, fmt.Errorf("resolve skill source root: %w", err)
	}
	source, err := skillbundle.ResolveSource(skillbundle.SourceOptions{
		ExplicitRoot:    env.Getenv("MATTY_SKILLS_SOURCE"),
		RepositoryStart: cwd,
		InstalledRoot:   installedSourceRoot,
	})
	if err != nil {
		return SkillSource{}, err
	}
	return SkillSource(source), nil
}

func DefaultInstalledSourceRoot(home string) string {
	return filepath.Join(home, ".local", "share", "matty")
}
