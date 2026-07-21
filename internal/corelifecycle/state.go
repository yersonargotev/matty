package corelifecycle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/ownedcontainer"
	packyversion "github.com/yersonargotev/packy/internal/version"
)

const SchemaVersion = 2

const LegacySchemaVersion = 1

type InstallStatus string

const (
	InstallConfirmed           InstallStatus = "confirmed"
	InstallRecoveryRequired    InstallStatus = "recovery-required"
	InstallUninstallIncomplete InstallStatus = "uninstall-incomplete"
)

var defaultDesiredSurfaces = []string{"codex", "opencode", "claude"}

var (
	writeStateTemp = func(file *os.File, data []byte) error {
		_, err := file.Write(data)
		return err
	}
	publishStateTemp = os.Rename
)

// ManagedSkill records the small amount of metadata Packy needs to know which
// global skill symlinks it owns. It intentionally stores paths, not skill
// prompt bodies.
type ManagedSkill struct {
	Name       string `json:"name"`
	SourcePath string `json:"source_path"`
	LinkPath   string `json:"link_path"`
}

// State is Packy's small global state file. It tracks ownership metadata only;
// prompt contents and skill bodies stay on disk outside this JSON file.
type State struct {
	SchemaVersion int            `json:"schema_version"`
	PackyVersion  string         `json:"packy_version"`
	ManagedSkills []ManagedSkill `json:"managed_skills"`
	// ConfiguredSurfaces is decoded only from schema v1. It is retained in the
	// in-memory model so older observers remain source compatible, but schema v2
	// never writes it.
	ConfiguredSurfaces []string                `json:"configured_surfaces,omitempty"`
	DesiredSurfaces    []string                `json:"desired_surfaces,omitempty"`
	ClaudeOwnership    []ClaudeOwnership       `json:"claude_ownership"`
	Paths              StatePaths              `json:"paths"`
	LastInstallCheck   string                  `json:"last_install_check,omitempty"`
	CreatedContainers  []ownedcontainer.Record `json:"created_containers"`
	InstallStatus      InstallStatus           `json:"install_status,omitempty"`
	LatestAttempt      *LatestAttempt          `json:"latest_attempt,omitempty"`
}

type ClaudeOwnership struct {
	ID                     string   `json:"id"`
	Kind                   string   `json:"kind"`
	Target                 string   `json:"target"`
	Fingerprint            string   `json:"fingerprint"`
	Contributors           []string `json:"contributors"`
	SourcePath             string   `json:"source_path,omitempty"`
	LinkTarget             string   `json:"link_target,omitempty"`
	Command                string   `json:"command,omitempty"`
	Args                   []string `json:"args,omitempty"`
	EnvironmentKeys        []string `json:"environment_keys,omitempty"`
	EnvironmentFingerprint string   `json:"environment_fingerprint,omitempty"`
	DeletionAuthorized     bool     `json:"deletion_authorized"`
}

type AttemptOutcome string

const (
	AttemptVerified            AttemptOutcome = "verified"
	AttemptBlocked             AttemptOutcome = "blocked"
	AttemptPartiallyApplied    AttemptOutcome = "partially-applied"
	AttemptRecoveryRequired    AttemptOutcome = "recovery-required"
	AttemptUninstallIncomplete AttemptOutcome = "uninstall-incomplete"
)

type LatestAttempt struct {
	Operation         Operation      `json:"operation"`
	Outcome           AttemptOutcome `json:"outcome"`
	CompletedEffects  []string       `json:"completed_effects"`
	FailedEffect      string         `json:"failed_effect,omitempty"`
	NotStartedEffects []string       `json:"not_started_effects"`
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

// LoadState reads Packy state. Missing state is a safe default; corrupt state is
// returned as a clear error because applying changes from unknown ownership data
// would be unsafe.
func LoadState(path string) (State, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, false, nil
		}
		return State{}, false, fmt.Errorf("read Packy state %s: %w", path, err)
	}
	var probe struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return State{}, false, fmt.Errorf("read Packy state %s: invalid JSON: %w", path, err)
	}
	decode := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		return d.Decode(v)
	}
	switch probe.SchemaVersion {
	case LegacySchemaVersion:
		var w stateV1
		if err := decode(&w); err != nil {
			return State{}, false, fmt.Errorf("read Packy state %s: invalid schema v1: %w", path, err)
		}
		if len(w.ConfiguredSurfaces) != 2 || w.ConfiguredSurfaces[0] != "codex" || w.ConfiguredSurfaces[1] != "opencode" {
			return State{}, false, fmt.Errorf("read Packy state %s: invalid schema v1 configured_surfaces", path)
		}
		return State{SchemaVersion: 1, PackyVersion: w.PackyVersion, ManagedSkills: w.ManagedSkills, ConfiguredSurfaces: w.ConfiguredSurfaces, Paths: w.Paths, LastInstallCheck: w.LastInstallCheck, CreatedContainers: w.CreatedContainers, InstallStatus: w.InstallStatus}, true, nil
	case SchemaVersion:
		var w stateV2
		if err := decode(&w); err != nil {
			return State{}, false, fmt.Errorf("read Packy state %s: invalid schema v2: %w", path, err)
		}
		state := w.state()
		if err := validateDesiredSurfaces(state.DesiredSurfaces); err != nil {
			return State{}, false, fmt.Errorf("read Packy state %s: %w", path, err)
		}
		if state.ManagedSkills == nil || state.ClaudeOwnership == nil {
			return State{}, false, fmt.Errorf("read Packy state %s: schema v2 required arrays must be non-null", path)
		}
		return state, true, nil
	default:
		return State{}, false, fmt.Errorf("read Packy state %s: unsupported schema_version %d", path, probe.SchemaVersion)
	}
}

type stateV1 struct {
	SchemaVersion      int                     `json:"schema_version"`
	PackyVersion       string                  `json:"packy_version"`
	ManagedSkills      []ManagedSkill          `json:"managed_skills"`
	ConfiguredSurfaces []string                `json:"configured_surfaces"`
	Paths              StatePaths              `json:"paths"`
	LastInstallCheck   string                  `json:"last_install_check,omitempty"`
	CreatedContainers  []ownedcontainer.Record `json:"created_containers,omitempty"`
	InstallStatus      InstallStatus           `json:"install_status,omitempty"`
}
type stateV2 struct {
	SchemaVersion     int                     `json:"schema_version"`
	PackyVersion      string                  `json:"packy_version"`
	ManagedSkills     []ManagedSkill          `json:"managed_skills"`
	DesiredSurfaces   []string                `json:"desired_surfaces"`
	ClaudeOwnership   []ClaudeOwnership       `json:"claude_ownership"`
	Paths             StatePaths              `json:"paths"`
	LastInstallCheck  string                  `json:"last_install_check,omitempty"`
	CreatedContainers []ownedcontainer.Record `json:"created_containers"`
	InstallStatus     InstallStatus           `json:"install_status,omitempty"`
	LatestAttempt     *LatestAttempt          `json:"latest_attempt,omitempty"`
}

func (w stateV2) state() State {
	return State{SchemaVersion: w.SchemaVersion, PackyVersion: w.PackyVersion, ManagedSkills: w.ManagedSkills, DesiredSurfaces: w.DesiredSurfaces, ClaudeOwnership: w.ClaudeOwnership, Paths: w.Paths, LastInstallCheck: w.LastInstallCheck, CreatedContainers: w.CreatedContainers, InstallStatus: w.InstallStatus, LatestAttempt: w.LatestAttempt}
}

func SaveState(path string, state State) error {
	if state.SchemaVersion != SchemaVersion {
		return fmt.Errorf("encode Packy state: unsupported schema_version %d", state.SchemaVersion)
	}
	state.ConfiguredSurfaces = nil
	state.ManagedSkills = append([]ManagedSkill(nil), state.ManagedSkills...)
	state.DesiredSurfaces = append([]string(nil), state.DesiredSurfaces...)
	state.CreatedContainers = append([]ownedcontainer.Record(nil), state.CreatedContainers...)
	state.ClaudeOwnership = append([]ClaudeOwnership(nil), state.ClaudeOwnership...)
	for i := range state.ClaudeOwnership {
		state.ClaudeOwnership[i].Contributors = append([]string(nil), state.ClaudeOwnership[i].Contributors...)
		state.ClaudeOwnership[i].Args = append([]string(nil), state.ClaudeOwnership[i].Args...)
		state.ClaudeOwnership[i].EnvironmentKeys = append([]string(nil), state.ClaudeOwnership[i].EnvironmentKeys...)
	}
	if state.LatestAttempt != nil {
		attempt := *state.LatestAttempt
		attempt.CompletedEffects = append([]string(nil), attempt.CompletedEffects...)
		attempt.NotStartedEffects = append([]string(nil), attempt.NotStartedEffects...)
		state.LatestAttempt = &attempt
	}
	if err := canonicalizeState(&state); err != nil {
		return fmt.Errorf("encode Packy state: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Packy state: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".packy-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create Packy state temporary file for %s: %w", path, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("set permissions on Packy state temporary file for %s: %w", path, err)
	}
	if err := writeStateTemp(temp, data); err != nil {
		temp.Close()
		return fmt.Errorf("write Packy state temporary file for %s: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync Packy state temporary file for %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close Packy state temporary file for %s: %w", path, err)
	}
	if err := publishStateTemp(tempPath, path); err != nil {
		return fmt.Errorf("publish Packy state %s: %w", path, err)
	}
	return nil
}

func canonicalizeState(state *State) error {
	if err := validateDesiredSurfaces(state.DesiredSurfaces); err != nil {
		return err
	}
	state.DesiredSurfaces = append([]string(nil), defaultDesiredSurfaces...)
	if state.ManagedSkills == nil {
		state.ManagedSkills = []ManagedSkill{}
	}
	if state.ClaudeOwnership == nil {
		state.ClaudeOwnership = []ClaudeOwnership{}
	}
	if state.CreatedContainers == nil {
		state.CreatedContainers = []ownedcontainer.Record{}
	}
	sort.Slice(state.ClaudeOwnership, func(i, j int) bool { return state.ClaudeOwnership[i].ID < state.ClaudeOwnership[j].ID })
	for i := range state.ClaudeOwnership {
		sort.Strings(state.ClaudeOwnership[i].Contributors)
		sort.Strings(state.ClaudeOwnership[i].EnvironmentKeys)
	}
	if state.LatestAttempt != nil {
		sort.Strings(state.LatestAttempt.CompletedEffects)
		sort.Strings(state.LatestAttempt.NotStartedEffects)
		if state.LatestAttempt.CompletedEffects == nil {
			state.LatestAttempt.CompletedEffects = []string{}
		}
		if state.LatestAttempt.NotStartedEffects == nil {
			state.LatestAttempt.NotStartedEffects = []string{}
		}
	}
	return nil
}
func validateDesiredSurfaces(in []string) error {
	if len(in) != 3 || in[0] != "codex" || in[1] != "opencode" || in[2] != "claude" {
		return fmt.Errorf("desired_surfaces must be exactly [codex, opencode, claude]")
	}
	return nil
}

func DesiredState(config StateConfig, checkedAt time.Time, managedSkills []ManagedSkill) State {
	return State{
		SchemaVersion:   SchemaVersion,
		PackyVersion:    packyversion.Value,
		ManagedSkills:   append([]ManagedSkill(nil), managedSkills...),
		DesiredSurfaces: append([]string(nil), defaultDesiredSurfaces...),
		ClaudeOwnership: []ClaudeOwnership{},
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

func (state State) Legacy() bool { return state.SchemaVersion == LegacySchemaVersion }

func (state State) UninstallIncomplete() bool {
	return state.InstallStatus == InstallUninstallIncomplete
}

type StateCondition string

const (
	StateMissing             StateCondition = "missing"
	StateValid               StateCondition = "valid"
	StateCorrupt             StateCondition = "corrupt"
	StateRecoveryRequired    StateCondition = "recovery-required"
	StateLegacy              StateCondition = "legacy"
	StateUninstallIncomplete StateCondition = "uninstall-incomplete"
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
	if state.Legacy() {
		condition = StateLegacy
	} else if state.RecoveryRequired() {
		condition = StateRecoveryRequired
	} else if state.UninstallIncomplete() {
		condition = StateUninstallIncomplete
	}
	return StateObservation{condition: condition, state: state}
}

func (observation StateObservation) Condition() StateCondition { return observation.condition }

func (observation StateObservation) Found() bool {
	return observation.condition == StateValid || observation.condition == StateLegacy || observation.condition == StateRecoveryRequired || observation.condition == StateUninstallIncomplete
}

func (observation StateObservation) Err() error { return observation.err }

func (observation StateObservation) Ownership() RecordedOwnership {
	return RecordedOwnership{
		ManagedSkills:     append([]ManagedSkill(nil), observation.state.ManagedSkills...),
		CreatedContainers: append([]ownedcontainer.Record(nil), observation.state.CreatedContainers...),
	}
}

func (observation StateObservation) ConfiguredSurfaces() []string {
	if observation.state.SchemaVersion == SchemaVersion && observation.state.ConfiguredSurfaces == nil {
		return append([]string(nil), observation.state.DesiredSurfaces...)
	}
	return append([]string(nil), observation.state.ConfiguredSurfaces...)
}

func (observation StateObservation) DesiredSurfaces() []string {
	return observation.ConfiguredSurfaces()
}
func (observation StateObservation) Legacy() bool { return observation.state.Legacy() }

// ObserveClaudeOwnershipSnapshot adapts authoritative classic state into the
// detached host ownership view used for fresh Claude preflight. Missing state
// is an empty snapshot; invalid state remains an error so cleanup fails closed.
func ObserveClaudeOwnershipSnapshot(path string) (claudecode.OwnershipSnapshot, error) {
	state, found, err := LoadState(path)
	if err != nil {
		return claudecode.OwnershipSnapshot{}, err
	}
	if !found || state.Legacy() {
		return claudecode.NewOwnershipSnapshot(), nil
	}
	records := make([]claudecode.OwnershipRecord, 0, len(state.ClaudeOwnership))
	for _, ownership := range state.ClaudeOwnership {
		kind := ownership.Kind
		switch kind {
		case "skill":
			kind = string(claudecode.ActionSkillLink)
		case "instruction":
			kind = string(claudecode.ActionInstructionContribution)
		case "mcp":
			kind = string(claudecode.ActionUserMCP)
		}
		records = append(records, claudecode.OwnershipRecord{
			StateOwner: "classic", ContributorID: "classic", ID: ownership.ID,
			Kind: kind, Target: ownership.Target, Fingerprint: ownership.Fingerprint,
			Contributors:       append([]string(nil), ownership.Contributors...),
			DeletionAuthorized: ownership.DeletionAuthorized,
			Skill: claudecode.SkillIdentity{Surface: "claude", ProjectionID: ownership.ID,
				Path: ownership.Target, SymlinkType: "directory", ResolvedTarget: ownership.LinkTarget,
				ExpectedSource: ownership.SourcePath, SourceTreeFingerprint: ownership.Fingerprint},
			Command: ownership.Command, Args: append([]string(nil), ownership.Args...),
			EnvironmentKeys:        append([]string(nil), ownership.EnvironmentKeys...),
			EnvironmentFingerprint: ownership.EnvironmentFingerprint,
		})
	}
	return claudecode.NewOwnershipSnapshot(records...), nil
}
