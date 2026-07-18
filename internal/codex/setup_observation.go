package codex

import (
	"os"
	"strings"

	"github.com/yersonargotev/packy/internal/prompt"
)

// SetupObservation is a detached read-only view of Packy's canonical Codex
// prompt surface.
type SetupObservation struct {
	promptFile      string
	exists          bool
	hasPackyMarkers bool
	warnings        []string
	err             error
}

func ObserveSetup(layout CanonicalLayout) SetupObservation {
	observation := SetupObservation{promptFile: layout.PromptFile()}
	data, err := os.ReadFile(layout.PromptFile())
	if err != nil {
		if !os.IsNotExist(err) {
			observation.err = err
		}
		return observation
	}
	content := string(data)
	observation.exists = true
	observation.hasPackyMarkers = strings.Contains(content, "<!-- packy:skills-router -->") && strings.Contains(content, "<!-- /packy:skills-router -->")
	observation.warnings = prompt.DetectExternalManagedBlocks(content)
	return observation
}

func (o SetupObservation) PromptFile() string    { return o.promptFile }
func (o SetupObservation) Exists() bool          { return o.exists }
func (o SetupObservation) HasPackyMarkers() bool { return o.hasPackyMarkers }
func (o SetupObservation) Err() error            { return o.err }
func (o SetupObservation) Warnings() []string    { return append([]string(nil), o.warnings...) }
