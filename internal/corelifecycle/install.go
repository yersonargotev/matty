package corelifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/ownedcontainer"
	"github.com/yersonargotev/matty/internal/prompt"
	"github.com/yersonargotev/matty/internal/skillbundle"
)

type Operation string

const (
	Install Operation = "install"
	Update  Operation = "update"
)

type ActionKind string

const (
	ActionWriteFile           ActionKind = "write-file"
	ActionWriteCodexPrompt    ActionKind = "write-codex-prompt"
	ActionWriteOpenCodePrompt ActionKind = "write-opencode-prompt"
	ActionSymlink             ActionKind = "symlink"
	ActionRun                 ActionKind = "run"
	ActionSkip                ActionKind = "skip"
)

// ActionView is a detached, read-only view of one ordered lifecycle action.
// Mutating a returned view cannot change the opaque plan consumed by Apply.
type ActionView struct {
	Kind        ActionKind
	Path        string
	Target      string
	Command     string
	Args        []string
	Description string
}

type Config struct {
	HomeDir                string
	ConfigHome             string
	MattyDir               string
	StateFile              string
	AgentSkillsDir         string
	SkillSourceRoot        string
	SkillSourceMissingHint string
	CodexPromptFile        string
	OpenCodeConfigFile     string
	OpenCodePromptFile     string
	HomebrewPrefix         string
	InstalledSourceRoot    string
	SkillSourceIsDefault   bool
	RunningVersion         string
}

type Commands interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, name string, args ...string) error
}

type Facade struct {
	config   Config
	commands Commands
	now      func() time.Time
}

func NewFacade(config Config, commands Commands, now func() time.Time) *Facade {
	if now == nil {
		now = time.Now
	}
	return &Facade{config: config, commands: commands, now: now}
}

type plannedAction struct {
	ActionView
	skipReason skillLinkStatus
}

// Plan deliberately exposes behavior only through detached views. Its state,
// action ordering, and owning facade remain unavailable to callers.
type Plan struct {
	owner     *Facade
	operation Operation
	actions   []plannedAction
	desired   State
}

var ErrForeignPlan = errors.New("core lifecycle plan was not previewed by this facade")

type Result struct {
	warnings          []string
	managedSkillCount int
	stateFile         string
}

func (result Result) Warnings() []string     { return append([]string(nil), result.warnings...) }
func (result Result) ManagedSkillCount() int { return result.managedSkillCount }
func (result Result) StateFile() string      { return result.stateFile }

func (plan Plan) Actions() []ActionView {
	actions := make([]ActionView, len(plan.actions))
	for i, action := range plan.actions {
		actions[i] = action.ActionView
		actions[i].Args = append([]string(nil), action.Args...)
	}
	return actions
}

func (plan Plan) ManagedSkillCount() int { return len(plan.desired.ManagedSkills) }

func (facade *Facade) Preview(operation Operation) (Plan, error) {
	if operation != Install && operation != Update {
		return Plan{}, fmt.Errorf("preview unsupported core lifecycle operation %q", operation)
	}
	if operation == Update {
		if err := facade.validateUpdateInstalledSource(); err != nil {
			return Plan{}, err
		}
	}
	if _, _, err := LoadState(facade.config.StateFile); err != nil {
		return Plan{}, err
	}
	discovered, err := skillbundle.Discover(facade.config.SkillSourceRoot, facade.config.AgentSkillsDir, facade.config.SkillSourceMissingHint)
	if err != nil {
		return Plan{}, err
	}
	actions := []plannedAction{{ActionView: ActionView{Kind: ActionWriteFile, Path: facade.config.StateFile, Description: "persist Matty state metadata"}}}
	managed := make([]ManagedSkill, 0, len(discovered))
	for _, skill := range discovered {
		managedSkill := ManagedSkill{Name: skill.Name, SourcePath: skill.SourcePath, LinkPath: skill.LinkPath}
		action, err := previewSkillLink(managedSkill)
		if err != nil {
			return Plan{}, err
		}
		if action.Kind != "" {
			actions = append(actions, action)
		}
		if action.Kind != ActionSkip {
			managed = append(managed, managedSkill)
		}
	}
	if operation == Install && !facade.homebrewEngramInstalled() {
		actions = append(actions, plannedAction{ActionView: ActionView{Kind: ActionRun, Command: "brew", Args: []string{"install", engrambin.Formula}, Description: "install Engram via Homebrew"}})
	}
	if operation == Update {
		actions = append([]plannedAction{
			{ActionView: ActionView{Kind: ActionRun, Command: "brew", Args: []string{"update"}, Description: "refresh Homebrew formula metadata"}},
			{ActionView: ActionView{Kind: ActionRun, Command: "brew", Args: []string{"upgrade", "engram"}, Description: "update Engram via Homebrew"}},
		}, actions...)
	}
	engram := engrambin.ExpectedHomebrewPath(facade.config.HomebrewPrefix)
	actions = append(actions,
		plannedAction{ActionView: ActionView{Kind: ActionRun, Command: engram, Args: []string{"setup", "codex"}, Description: "delegate Codex Engram setup through Homebrew binary"}},
		plannedAction{ActionView: ActionView{Kind: ActionRun, Command: engram, Args: []string{"setup", "opencode"}, Description: "delegate OpenCode Engram setup through Homebrew binary"}},
		plannedAction{ActionView: ActionView{Kind: ActionWriteCodexPrompt, Path: facade.config.CodexPromptFile, Description: "write Codex Matty prompt markers"}},
		plannedAction{ActionView: ActionView{Kind: ActionWriteOpenCodePrompt, Path: facade.config.OpenCodeConfigFile, Target: facade.config.OpenCodePromptFile, Description: "write OpenCode Matty prompt reference"}},
	)
	return Plan{
		owner:     facade,
		operation: operation,
		actions:   actions,
		desired: DesiredState(StateConfig{
			StateFile:      facade.config.StateFile,
			AgentSkillsDir: facade.config.AgentSkillsDir,
		}, facade.now(), managed),
	}, nil
}

// saveInstallState is a private persistence-failure seam. Filesystem behavior
// otherwise remains concrete and is exercised in sandbox directories.
var saveInstallState = SaveState

// saveUpdateState is the focused persistence-failure seam for update recovery.
var saveUpdateState = SaveState

func (facade *Facade) Apply(ctx context.Context, plan Plan) (Result, error) {
	if plan.owner != facade || (plan.operation != Install && plan.operation != Update) {
		return Result{}, ErrForeignPlan
	}
	saveState := saveInstallState
	if plan.operation == Update {
		saveState = saveUpdateState
	}
	previous, previousFound, err := LoadState(facade.config.StateFile)
	if err != nil {
		return Result{}, err
	}
	anchor, err := facade.provisionStateAnchor()
	if err != nil {
		return Result{}, err
	}
	recovery := facade.recoveryState(plan.desired, previous, previousFound, anchor)
	if err := saveState(facade.config.StateFile, recovery); err != nil {
		if cleanupErr := cleanupInstallContainers(anchor); cleanupErr != nil {
			return Result{}, fmt.Errorf("%w; clean up unrecorded Matty containers: %v", err, cleanupErr)
		}
		return Result{}, err
	}
	created, provisionErr := ownedcontainer.Provision(facade.effectContainerRecords())
	recovery.CreatedContainers = ownedcontainer.Merge(recovery.CreatedContainers, created)
	if err := saveState(facade.config.StateFile, recovery); err != nil {
		if cleanupErr := cleanupInstallContainers(created); cleanupErr != nil {
			return Result{}, fmt.Errorf("%w; clean up unrecorded Matty containers: %v", err, cleanupErr)
		}
		return Result{}, err
	}
	if provisionErr != nil {
		return Result{}, provisionErr
	}
	if err := os.MkdirAll(facade.config.MattyDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create Matty config directory %s: %w", facade.config.MattyDir, err)
	}
	if err := os.MkdirAll(facade.config.AgentSkillsDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create agent skills directory %s: %w", facade.config.AgentSkillsDir, err)
	}
	var warnings []string
	for _, action := range plan.actions {
		switch action.Kind {
		case ActionSymlink:
			if err := os.Symlink(action.Target, action.Path); err != nil {
				return Result{}, fmt.Errorf("create skill symlink %s -> %s: %w", action.Path, action.Target, err)
			}
			recovery.ManagedSkills = append(recovery.ManagedSkills, managedSkillForInstallAction(plan.desired.ManagedSkills, action))
			if err := saveState(facade.config.StateFile, recovery); err != nil {
				if removeErr := os.Remove(action.Path); removeErr != nil {
					return Result{}, fmt.Errorf("%w; roll back unrecorded skill symlink %s: %v", err, action.Path, removeErr)
				}
				return Result{}, err
			}
		case ActionWriteCodexPrompt:
			result, err := prompt.WriteCodex(action.Path)
			if err != nil {
				return Result{}, err
			}
			warnings = append(warnings, result.Warnings...)
		case ActionWriteOpenCodePrompt:
			result, err := opencode.Write(action.Path, action.Target)
			if err != nil {
				return Result{}, err
			}
			warnings = append(warnings, result.Warnings...)
		case ActionRun:
			command := action.Command
			if isInstallEngramSetup(action) {
				canonical := engrambin.DiscoverHomebrew(facade.config.HomebrewPrefix)
				if canonical == nil {
					return Result{}, missingInstallEngramError(action, engrambin.HomebrewCandidatePaths(engrambin.HomebrewPrefixes(facade.config.HomebrewPrefix)))
				}
				command = canonical.Path
			}
			if err := facade.commands.Run(ctx, command, action.Args...); err != nil {
				action.Command = command
				return Result{}, lifecycleActionRunError(action, err)
			}
		}
	}
	confirmed := plan.desired
	if previous.RecoveryRequired() {
		confirmed.ManagedSkills = append([]ManagedSkill(nil), recovery.ManagedSkills...)
	}
	confirmed.CreatedContainers = append([]ownedcontainer.Record(nil), recovery.CreatedContainers...)
	confirmed.InstallStatus = InstallConfirmed
	if err := saveState(facade.config.StateFile, confirmed); err != nil {
		return Result{}, err
	}
	if warning, ok := unmanagedInstallSymlinkWarning(plan); ok {
		warnings = append(warnings, warning)
	}
	return Result{warnings: warnings, managedSkillCount: len(plan.desired.ManagedSkills), stateFile: facade.config.StateFile}, nil
}

func (facade *Facade) homebrewEngramInstalled() bool {
	if engrambin.DiscoverHomebrew(facade.config.HomebrewPrefix) != nil {
		return true
	}
	resolved, err := facade.commands.LookPath("engram")
	return err == nil && engrambin.IsExpectedHomebrewPath(resolved, engrambin.ExpectedHomebrewPath(facade.config.HomebrewPrefix))
}

type skillLinkStatus string

const (
	skillLinkMissing          skillLinkStatus = "missing"
	skillLinkUnmanagedPath    skillLinkStatus = "unmanaged-path"
	skillLinkUnmanagedSymlink skillLinkStatus = "unmanaged-symlink"
)

func previewSkillLink(skill ManagedSkill) (plannedAction, error) {
	info, err := os.Lstat(skill.LinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return plannedAction{ActionView: ActionView{Kind: ActionSymlink, Path: skill.LinkPath, Target: skill.SourcePath, Description: "link managed skill " + skill.Name}, skipReason: skillLinkMissing}, nil
		}
		return plannedAction{}, fmt.Errorf("inspect skill link %s: %w", skill.LinkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return plannedAction{ActionView: ActionView{Kind: ActionSkip, Path: skill.LinkPath, Target: skill.SourcePath, Description: "preserve unmanaged path for skill " + skill.Name}, skipReason: skillLinkUnmanagedPath}, nil
	}
	target, err := os.Readlink(skill.LinkPath)
	if err != nil {
		return plannedAction{}, fmt.Errorf("read skill link %s: %w", skill.LinkPath, err)
	}
	if sameLinkTarget(skill.LinkPath, target, skill.SourcePath) {
		return plannedAction{}, nil
	}
	return plannedAction{ActionView: ActionView{Kind: ActionSkip, Path: skill.LinkPath, Target: target, Description: "preserve unmanaged symlink for skill " + skill.Name}, skipReason: skillLinkUnmanagedSymlink}, nil
}

func sameLinkTarget(linkPath, gotTarget, wantTarget string) bool {
	if gotTarget == wantTarget {
		return true
	}
	if !filepath.IsAbs(gotTarget) {
		gotTarget = filepath.Join(filepath.Dir(linkPath), gotTarget)
	}
	gotAbs, gotErr := filepath.Abs(gotTarget)
	wantAbs, wantErr := filepath.Abs(wantTarget)
	return gotErr == nil && wantErr == nil && gotAbs == wantAbs
}

func (facade *Facade) provisionStateAnchor() ([]ownedcontainer.Record, error) {
	var created []ownedcontainer.Record
	if _, err := os.Lstat(facade.config.MattyDir); os.IsNotExist(err) {
		if err := os.Mkdir(facade.config.MattyDir, 0o700); err != nil {
			return nil, fmt.Errorf("create Matty config directory %s: %w", facade.config.MattyDir, err)
		}
		created = append(created, ownedcontainer.Record{Path: facade.config.MattyDir, Kind: ownedcontainer.Directory})
	} else if err != nil {
		return nil, fmt.Errorf("inspect Matty config directory %s: %w", facade.config.MattyDir, err)
	}
	if _, err := os.Lstat(facade.config.StateFile); os.IsNotExist(err) {
		created = append(created, ownedcontainer.Record{Path: facade.config.StateFile, Kind: ownedcontainer.File})
	} else if err != nil {
		return nil, fmt.Errorf("inspect Matty state %s: %w", facade.config.StateFile, err)
	}
	return created, nil
}

func (facade *Facade) effectContainerRecords() []ownedcontainer.Record {
	records := facade.containerRecords()
	out := make([]ownedcontainer.Record, 0, len(records)-2)
	for _, record := range records {
		if record.Path != facade.config.MattyDir && record.Path != facade.config.StateFile {
			out = append(out, record)
		}
	}
	return out
}

func (facade *Facade) containerRecords() []ownedcontainer.Record {
	return []ownedcontainer.Record{
		{Path: facade.config.MattyDir, Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(facade.config.AgentSkillsDir), Kind: ownedcontainer.Directory},
		{Path: facade.config.AgentSkillsDir, Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(facade.config.CodexPromptFile), Kind: ownedcontainer.Directory},
		{Path: facade.config.ConfigHome, Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(facade.config.OpenCodeConfigFile), Kind: ownedcontainer.Directory},
		{Path: facade.config.StateFile, Kind: ownedcontainer.File},
		{Path: facade.config.CodexPromptFile, Kind: ownedcontainer.File},
		{Path: facade.config.OpenCodeConfigFile, Kind: ownedcontainer.File},
		{Path: facade.config.OpenCodePromptFile, Kind: ownedcontainer.File},
	}
}

func cleanupInstallContainers(created []ownedcontainer.Record) error {
	cleanup, err := ownedcontainer.Preview(created)
	if err != nil {
		return err
	}
	_, err = cleanup.Cleanup()
	return err
}

func (facade *Facade) recoveryState(desired, previous State, previousFound bool, created []ownedcontainer.Record) State {
	recovery := desired
	recovery.InstallStatus = InstallRecoveryRequired
	recovery.ManagedSkills = nil
	if previousFound {
		for _, skill := range previous.ManagedSkills {
			action, err := previewSkillLink(skill)
			if err == nil && action.Kind == "" {
				recovery.ManagedSkills = append(recovery.ManagedSkills, skill)
			}
		}
	}
	recovery.CreatedContainers = ownedcontainer.Merge(previous.CreatedContainers, created)
	return recovery
}

func managedSkillForInstallAction(skills []ManagedSkill, action plannedAction) ManagedSkill {
	for _, skill := range skills {
		if skill.LinkPath == action.Path && skill.SourcePath == action.Target {
			return skill
		}
	}
	panic("install plan symlink has no matching managed skill")
}

func isInstallEngramSetup(action plannedAction) bool {
	return filepath.Base(action.Command) == "engram" && len(action.Args) >= 2 && action.Args[0] == "setup"
}

func missingInstallEngramError(action plannedAction, candidates []string) error {
	return fmt.Errorf("run %s: canonical Homebrew Engram was not found at any expected Homebrew path (%s); run brew install %s or set HOMEBREW_PREFIX to the active Homebrew prefix, then retry matty install or matty update", strings.Join(append([]string{action.Command}, action.Args...), " "), strings.Join(candidates, ", "), engrambin.Formula)
}

func lifecycleActionRunError(action plannedAction, err error) error {
	command := strings.Join(append([]string{action.Command}, action.Args...), " ")
	switch {
	case action.Command == "brew" && len(action.Args) > 0 && action.Args[0] == "install":
		return fmt.Errorf("run %s: failed to install Engram via Homebrew; ensure Homebrew is installed and retry: %w", command, err)
	case action.Command == "brew" && len(action.Args) > 0 && (action.Args[0] == "update" || action.Args[0] == "upgrade"):
		return fmt.Errorf("run %s: failed to update Engram via Homebrew; ensure Homebrew is installed and retry: %w", command, err)
	case isInstallEngramSetup(action):
		return fmt.Errorf("run %s: failed to configure Engram for %s through the Homebrew-managed binary; run brew install %s or brew upgrade engram, then retry matty install or matty update: %w", command, action.Args[1], engrambin.Formula, err)
	default:
		return fmt.Errorf("run %s: %w", command, err)
	}
}

func unmanagedInstallSymlinkWarning(plan Plan) (string, bool) {
	count := 0
	skipped := 0
	var example plannedAction
	for _, action := range plan.actions {
		if action.Kind != ActionSkip {
			continue
		}
		skipped++
		if action.skipReason == skillLinkUnmanagedSymlink {
			if count == 0 {
				example = action
			}
			count++
		}
	}
	expected := len(plan.desired.ManagedSkills) + skipped
	if expected == 0 || count*2 <= expected {
		return "", false
	}
	return fmt.Sprintf("skipped %d unmanaged skill symlinks; setup may be incomplete. Example: %s -> %s. Safe recovery: verify these are stale Matty-created links, remove them, then run matty install; Matty will not overwrite arbitrary files or links.", count, example.Path, example.Target), true
}
