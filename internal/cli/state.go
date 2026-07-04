package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const stateSchemaVersion = 1

var defaultConfiguredSurfaces = []string{"codex", "opencode"}

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
	SchemaVersion      int            `json:"schema_version"`
	MattyVersion       string         `json:"matty_version"`
	ManagedSkills      []ManagedSkill `json:"managed_skills"`
	ConfiguredSurfaces []string       `json:"configured_surfaces"`
	Paths              StatePaths     `json:"paths"`
	LastInstallCheck   string         `json:"last_install_check,omitempty"`
}

type StatePaths struct {
	StateFile      string `json:"state_file"`
	AgentSkillsDir string `json:"agent_skills_dir"`
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
	if state.SchemaVersion != stateSchemaVersion {
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
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write Matty state %s: %w", path, err)
	}
	return nil
}

func DesiredState(paths Paths, checkedAt time.Time, managedSkills []ManagedSkill) State {
	return State{
		SchemaVersion:      stateSchemaVersion,
		MattyVersion:       version,
		ManagedSkills:      append([]ManagedSkill(nil), managedSkills...),
		ConfiguredSurfaces: append([]string(nil), defaultConfiguredSurfaces...),
		Paths: StatePaths{
			StateFile:      paths.StateFile,
			AgentSkillsDir: paths.AgentSkillsDir,
		},
		LastInstallCheck: checkedAt.UTC().Format(time.RFC3339),
	}
}
