package opencode

// SetupObservation binds the canonical OpenCode layout to its detached,
// read-only inspection result.
type SetupObservation struct {
	configFile string
	promptFile string
	inspection Inspection
	err        error
}

func ObserveSetup(layout CanonicalLayout) SetupObservation {
	inspection, err := Inspect(layout.ConfigFile(), layout.PromptFile())
	return SetupObservation{
		configFile: layout.ConfigFile(),
		promptFile: layout.PromptFile(),
		inspection: inspection,
		err:        err,
	}
}

func (o SetupObservation) ConfigFile() string     { return o.configFile }
func (o SetupObservation) PromptFile() string     { return o.promptFile }
func (o SetupObservation) Inspection() Inspection { return o.inspection }
func (o SetupObservation) Err() error             { return o.err }
