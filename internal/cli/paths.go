package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/matty/internal/skillbundle"
)

type SkillSourceOrigin string

type SkillSource struct {
	Root        string
	MissingHint string
	IsDefault   bool
	Origin      SkillSourceOrigin
}

const (
	SkillSourceOriginOverride  SkillSourceOrigin = "override"
	SkillSourceOriginRepo      SkillSourceOrigin = "repo"
	SkillSourceOriginInstalled SkillSourceOrigin = "installed"
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

func resolveSkillSourceRoot(env Env, installedSourceRoot string) (SkillSource, error) {
	configured := env.Getenv("MATTY_SKILLS_SOURCE")
	if configured != "" {
		path, err := filepath.Abs(configured)
		return SkillSource{Root: path, Origin: SkillSourceOriginOverride}, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return SkillSource{}, fmt.Errorf("resolve skill source root: %w", err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := skillbundle.SourceRoot(dir)
		if skillbundle.SourceRootExists(candidate) {
			path, err := filepath.Abs(candidate)
			return SkillSource{Root: path, Origin: SkillSourceOriginRepo}, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	path, err := filepath.Abs(skillbundle.SourceRoot(installedSourceRoot))
	return SkillSource{Root: path, MissingHint: "run matty init to initialize it", IsDefault: true, Origin: SkillSourceOriginInstalled}, err
}

func DefaultInstalledSourceRoot(home string) string {
	return filepath.Join(home, ".local", "share", "matty")
}
