package opencode

import "path/filepath"

// CanonicalLayout contains the OpenCode-owned global configuration paths.
type CanonicalLayout struct {
	configurationHome string
	configFile        string
	promptFile        string
}

func NewCanonicalLayout(configurationHome string) CanonicalLayout {
	root := filepath.Join(configurationHome, "opencode")
	return CanonicalLayout{
		configurationHome: configurationHome,
		configFile:        filepath.Join(root, "opencode.json"),
		promptFile:        filepath.Join(root, "packy.md"),
	}
}

func (l CanonicalLayout) ConfigurationHome() string { return l.configurationHome }
func (l CanonicalLayout) ConfigFile() string        { return l.configFile }
func (l CanonicalLayout) PromptFile() string        { return l.promptFile }
