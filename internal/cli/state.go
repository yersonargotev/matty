package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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

func DesiredState(paths Paths, checkedAt time.Time) State {
	return State{
		SchemaVersion:      stateSchemaVersion,
		MattyVersion:       version,
		ManagedSkills:      desiredManagedSkills(paths),
		ConfiguredSurfaces: append([]string(nil), defaultConfiguredSurfaces...),
		Paths: StatePaths{
			StateFile:      paths.StateFile,
			AgentSkillsDir: paths.AgentSkillsDir,
		},
		LastInstallCheck: checkedAt.UTC().Format(time.RFC3339),
	}
}

func desiredManagedSkills(paths Paths) []ManagedSkill {
	skills := []struct {
		name string
		src  string
	}{
		{name: "ask-matt", src: "skills/engineering/ask-matt"},
		{name: "code-review", src: "skills/engineering/code-review"},
		{name: "codebase-design", src: "skills/engineering/codebase-design"},
		{name: "diagnosing-bugs", src: "skills/engineering/diagnosing-bugs"},
		{name: "domain-modeling", src: "skills/engineering/domain-modeling"},
		{name: "grill-with-docs", src: "skills/engineering/grill-with-docs"},
		{name: "implement", src: "skills/engineering/implement"},
		{name: "improve-codebase-architecture", src: "skills/engineering/improve-codebase-architecture"},
		{name: "prototype", src: "skills/engineering/prototype"},
		{name: "research", src: "skills/engineering/research"},
		{name: "resolving-merge-conflicts", src: "skills/engineering/resolving-merge-conflicts"},
		{name: "setup-matt-pocock-skills", src: "skills/engineering/setup-matt-pocock-skills"},
		{name: "tdd", src: "skills/engineering/tdd"},
		{name: "to-issues", src: "skills/engineering/to-issues"},
		{name: "to-prd", src: "skills/engineering/to-prd"},
		{name: "triage", src: "skills/engineering/triage"},
		{name: "grill-me", src: "skills/productivity/grill-me"},
		{name: "grilling", src: "skills/productivity/grilling"},
		{name: "handoff", src: "skills/productivity/handoff"},
		{name: "teach", src: "skills/productivity/teach"},
		{name: "writing-great-skills", src: "skills/productivity/writing-great-skills"},
		{name: "loop-me", src: "skills/in-progress/loop-me"},
		{name: "wayfinder", src: "skills/in-progress/wayfinder"},
	}

	managed := make([]ManagedSkill, 0, len(skills))
	for _, skill := range skills {
		managed = append(managed, ManagedSkill{
			Name:       skill.name,
			SourcePath: skill.src,
			LinkPath:   paths.SkillLinkPath(skill.name),
		})
	}
	sort.Slice(managed, func(i, j int) bool { return managed[i].Name < managed[j].Name })
	return managed
}
