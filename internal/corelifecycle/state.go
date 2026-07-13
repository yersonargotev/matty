package corelifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yersonargotev/matty/internal/ownedcontainer"
	mattyversion "github.com/yersonargotev/matty/internal/version"
)

const SchemaVersion = 1

type InstallStatus string

const (
	InstallConfirmed        InstallStatus = "confirmed"
	InstallRecoveryRequired InstallStatus = "recovery-required"
)

var defaultConfiguredSurfaces = []string{"codex", "opencode"}

var (
	writeStateTemp = func(file *os.File, data []byte) error {
		_, err := file.Write(data)
		return err
	}
	publishStateTemp = os.Rename
)

// ManagedSkill records the small amount of metadata Matty needs to know which
// global skill symlinks it owns. It intentionally stores paths, not skill
// prompt bodies.
type ManagedSkill struct {
	Name       string `json:"name"`
	SourcePath string `json:"source_path"`
	LinkPath   string `json:"link_path"`
}

// State is Matty's small global state file. It tracks ownership metadata only;
// prompt contents and skill bodies stay on disk outside this JSON file.
type State struct {
	SchemaVersion      int                     `json:"schema_version"`
	MattyVersion       string                  `json:"matty_version"`
	ManagedSkills      []ManagedSkill          `json:"managed_skills"`
	ConfiguredSurfaces []string                `json:"configured_surfaces"`
	Paths              StatePaths              `json:"paths"`
	LastInstallCheck   string                  `json:"last_install_check,omitempty"`
	CreatedContainers  []ownedcontainer.Record `json:"created_containers,omitempty"`
	InstallStatus      InstallStatus           `json:"install_status,omitempty"`
}

type StatePaths struct {
	StateFile      string `json:"state_file"`
	AgentSkillsDir string `json:"agent_skills_dir"`
}

// StateConfig contains the resolved paths needed to derive classic desired
// state without transferring workstation path resolution into this package.
type StateConfig struct {
	StateFile      string
	AgentSkillsDir string
}

// LoadState reads Matty state. Missing state is a safe default; corrupt state is
// returned as a clear error because applying changes from unknown ownership data
// would be unsafe.
func LoadState(path string) (State, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, false, nil
		}
		return State{}, false, fmt.Errorf("read Matty state %s: %w", path, err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, fmt.Errorf("read Matty state %s: invalid JSON: %w", path, err)
	}
	if state.SchemaVersion != SchemaVersion {
		return State{}, false, fmt.Errorf("read Matty state %s: unsupported schema_version %d", path, state.SchemaVersion)
	}
	return state, true, nil
}

func SaveState(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Matty state: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".matty-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create Matty state temporary file for %s: %w", path, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("set permissions on Matty state temporary file for %s: %w", path, err)
	}
	if err := writeStateTemp(temp, data); err != nil {
		temp.Close()
		return fmt.Errorf("write Matty state temporary file for %s: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync Matty state temporary file for %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close Matty state temporary file for %s: %w", path, err)
	}
	if err := publishStateTemp(tempPath, path); err != nil {
		return fmt.Errorf("publish Matty state %s: %w", path, err)
	}
	return nil
}

func DesiredState(config StateConfig, checkedAt time.Time, managedSkills []ManagedSkill) State {
	return State{
		SchemaVersion:      SchemaVersion,
		MattyVersion:       mattyversion.Value,
		ManagedSkills:      append([]ManagedSkill(nil), managedSkills...),
		ConfiguredSurfaces: append([]string(nil), defaultConfiguredSurfaces...),
		Paths: StatePaths{
			StateFile:      config.StateFile,
			AgentSkillsDir: config.AgentSkillsDir,
		},
		LastInstallCheck: checkedAt.UTC().Format(time.RFC3339),
		InstallStatus:    InstallConfirmed,
	}
}

func (state State) RecoveryRequired() bool {
	return state.InstallStatus == InstallRecoveryRequired
}

type StateCondition string

const (
	StateMissing          StateCondition = "missing"
	StateValid            StateCondition = "valid"
	StateCorrupt          StateCondition = "corrupt"
	StateRecoveryRequired StateCondition = "recovery-required"
)

// RecordedOwnership is the deletion authority recorded by classic state.
type RecordedOwnership struct {
	ManagedSkills     []ManagedSkill
	CreatedContainers []ownedcontainer.Record
}

// StateObservation exposes read-only classic state facts without exposing the
// persistence implementation.
type StateObservation struct {
	condition StateCondition
	state     State
	err       error
}

func ObserveState(path string) StateObservation {
	state, found, err := LoadState(path)
	if err != nil {
		return StateObservation{condition: StateCorrupt, err: err}
	}
	if !found {
		return StateObservation{condition: StateMissing}
	}
	condition := StateValid
	if state.RecoveryRequired() {
		condition = StateRecoveryRequired
	}
	return StateObservation{condition: condition, state: state}
}

func (observation StateObservation) Condition() StateCondition { return observation.condition }

func (observation StateObservation) Found() bool {
	return observation.condition == StateValid || observation.condition == StateRecoveryRequired
}

func (observation StateObservation) Err() error { return observation.err }

func (observation StateObservation) Ownership() RecordedOwnership {
	return RecordedOwnership{
		ManagedSkills:     append([]ManagedSkill(nil), observation.state.ManagedSkills...),
		CreatedContainers: append([]ownedcontainer.Record(nil), observation.state.CreatedContainers...),
	}
}

func (observation StateObservation) ConfiguredSurfaces() []string {
	return append([]string(nil), observation.state.ConfiguredSurfaces...)
}
