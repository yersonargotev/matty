package corelifecycle

import "github.com/yersonargotev/packy/internal/skillbundle"

// SetupObservation is the detached, read-only classic lifecycle view used by
// setup diagnosis. Classic lifecycle remains the owner of state parsing and
// managed-skill ownership interpretation.
type SetupObservation struct {
	stateFile     string
	skillsRoot    string
	state         StateObservation
	managedLinks  []SkillLinkObservation
	expectedLinks []SkillLinkObservation
	expectedErr   error
}

func ObserveSetup(layout Layout, skills skillbundle.GlobalLayout, source skillbundle.Source) SetupObservation {
	state := ObserveState(layout.StateFile())
	observation := SetupObservation{stateFile: layout.StateFile(), skillsRoot: skills.Root(), state: state}
	if !state.Found() {
		return observation
	}
	managed := state.Ownership().ManagedSkills
	if len(managed) != 0 {
		observation.managedLinks = ObserveManagedSkillLinks(managed)
		return observation
	}
	observation.expectedLinks, observation.expectedErr = ObserveExpectedManagedSkillLinks(Config{
		AgentSkillsDir:         skills.Root(),
		SkillSourceRoot:        source.Root,
		SkillSourceMissingHint: source.MissingHint,
	})
	return observation
}

func (o SetupObservation) StateFile() string            { return o.stateFile }
func (o SetupObservation) SkillsRoot() string           { return o.skillsRoot }
func (o SetupObservation) State() StateObservation      { return o.state }
func (o SetupObservation) ExpectedSkillLinksErr() error { return o.expectedErr }
func (o SetupObservation) ManagedSkillLinks() []SkillLinkObservation {
	return append([]SkillLinkObservation(nil), o.managedLinks...)
}
func (o SetupObservation) ExpectedSkillLinks() []SkillLinkObservation {
	return append([]SkillLinkObservation(nil), o.expectedLinks...)
}
