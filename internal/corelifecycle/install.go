package corelifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/ownedcontainer"
	"github.com/yersonargotev/packy/internal/prompt"
	"github.com/yersonargotev/packy/internal/skillbundle"
)

type Operation string

const (
	Install   Operation = "install"
	Update    Operation = "update"
	Uninstall Operation = "uninstall"
)

type ActionKind string

const (
	ActionWriteFile            ActionKind = "write-file"
	ActionWriteCodexPrompt     ActionKind = "write-codex-prompt"
	ActionWriteOpenCodePrompt  ActionKind = "write-opencode-prompt"
	ActionSymlink              ActionKind = "symlink"
	ActionRun                  ActionKind = "run"
	ActionSkip                 ActionKind = "skip"
	ActionRemove               ActionKind = "remove"
	ActionRemoveCodexPrompt    ActionKind = "remove-codex-prompt"
	ActionRemoveOpenCodePrompt ActionKind = "remove-opencode-prompt"
	ActionCleanup              ActionKind = "cleanup"
)

// Outcome classifies the complete lifecycle plan/result independently from
// command-line exit policy.
type Outcome string

const (
	OutcomeConverged                      Outcome = "converged"
	OutcomeApplied                        Outcome = "applied"
	OutcomeAppliedWithPendingPrerequisite Outcome = "applied-with-pending-prerequisite"
	OutcomeBlocked                        Outcome = "blocked"
	OutcomePartiallyApplied               Outcome = "partially-applied"
	OutcomeRolledBack                     Outcome = "rolled-back"
	OutcomeRecoveryRequired               Outcome = "recovery-required"
	OutcomeUninstallIncomplete            Outcome = "uninstall-incomplete"
)

type StateTransitionView struct {
	FromSchemaVersion int
	ToSchemaVersion   int
	FromStatus        InstallStatus
	ToStatus          InstallStatus
}

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
	AgentSkillsDir         string
	SkillSourceRoot        string
	SkillSourceMissingHint string
}

type facadeConfig struct {
	State           Layout
	Skills          skillbundle.GlobalLayout
	SkillSource     skillbundle.Source
	Codex           codex.CanonicalLayout
	OpenCode        opencode.CanonicalLayout
	Engram          engrambin.Topology
	InstalledSource bootstrap.InstalledSource
	RunningVersion  string
	Claude          *claudecode.SurfaceAdapter
	ClaudeDesired   claudecode.ClassicDesired
}

// FacadeConfig is the narrow composition contract for classic lifecycle.
// Every derived artifact path comes from its owning module.
type FacadeConfig struct {
	PackyHome       string
	Skills          skillbundle.GlobalLayout
	SkillSource     skillbundle.Source
	Codex           codex.CanonicalLayout
	OpenCode        opencode.CanonicalLayout
	Engram          engrambin.Topology
	InstalledSource bootstrap.InstalledSource
	RunningVersion  string
	Claude          *claudecode.SurfaceAdapter
	ClaudeDesired   claudecode.ClassicDesired
}

type Commands interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, name string, args ...string) error
}

type Facade struct {
	config   facadeConfig
	commands Commands
	now      func() time.Time
}

func NewFacade(owners FacadeConfig, commands Commands, now func() time.Time) *Facade {
	if now == nil {
		now = time.Now
	}
	state := NewLayout(owners.PackyHome)
	claudeDesired := cloneClassicDesired(owners.ClaudeDesired)
	if owners.Claude != nil && len(claudeDesired.Skills) == 0 && claudeDesired.Instruction == nil && claudeDesired.MCP == nil {
		claudeDesired = defaultClaudeDesired()
	}
	config := facadeConfig{
		State:           state,
		Skills:          owners.Skills,
		SkillSource:     owners.SkillSource,
		Codex:           owners.Codex,
		OpenCode:        owners.OpenCode,
		RunningVersion:  owners.RunningVersion,
		InstalledSource: owners.InstalledSource,
		Engram:          owners.Engram,
		Claude:          owners.Claude,
		ClaudeDesired:   claudeDesired,
	}
	return &Facade{config: config, commands: commands, now: now}
}

type plannedAction struct {
	ActionView
	skipReason SkillLinkCondition
}

// Plan deliberately exposes behavior only through detached views. Its state,
// action ordering, and owning facade remain unavailable to callers.
type Plan struct {
	owner           *Facade
	operation       Operation
	actions         []plannedAction
	desired         State
	cleanup         ownedcontainer.Plan
	preconditions   ownedcontainer.Plan
	hasWork         bool
	outcome         Outcome
	blockers        []string
	pending         []string
	preserved       []string
	recovery        []string
	transition      StateTransitionView
	legacyMigration bool
	claudePlan      claudecode.ClassicPlan
	hasClaudePlan   bool
}

var ErrForeignPlan = errors.New("core lifecycle plan was not previewed by this facade")
var ErrBlockedPlan = errors.New("core lifecycle plan is blocked")

type Result struct {
	warnings          []string
	managedSkillCount int
	stateFile         string
	hasWork           bool
	outcome           Outcome
	committed         bool
	stateTransition   StateTransitionView
	completedEffects  []string
	failedEffect      string
	notStartedEffects []string
}

func (result Result) Warnings() []string     { return append([]string(nil), result.warnings...) }
func (result Result) ManagedSkillCount() int { return result.managedSkillCount }
func (result Result) StateFile() string      { return result.stateFile }
func (result Result) HasWork() bool          { return result.hasWork }
func (result Result) Outcome() Outcome       { return result.outcome }
func (result Result) Committed() bool        { return result.committed }
func (result Result) StateTransition() StateTransitionView {
	return result.stateTransition
}
func (result Result) CompletedEffects() []string {
	return append([]string(nil), result.completedEffects...)
}
func (result Result) FailedEffect() string { return result.failedEffect }
func (result Result) NotStartedEffects() []string {
	return append([]string(nil), result.notStartedEffects...)
}

func (plan Plan) Actions() []ActionView {
	actions := make([]ActionView, len(plan.actions))
	for i, action := range plan.actions {
		actions[i] = action.ActionView
		actions[i].Args = append([]string(nil), action.Args...)
	}
	return actions
}

func (plan Plan) ManagedSkillCount() int               { return len(plan.desired.ManagedSkills) }
func (plan Plan) Outcome() Outcome                     { return plan.outcome }
func (plan Plan) Blockers() []string                   { return append([]string(nil), plan.blockers...) }
func (plan Plan) PendingPrerequisites() []string       { return append([]string(nil), plan.pending...) }
func (plan Plan) Preserved() []string                  { return append([]string(nil), plan.preserved...) }
func (plan Plan) RecoveryEvidence() []string           { return append([]string(nil), plan.recovery...) }
func (plan Plan) StateTransition() StateTransitionView { return plan.transition }
func (plan Plan) DesiredSurfaces() []string {
	return append([]string(nil), plan.desired.DesiredSurfaces...)
}

func (facade *Facade) Preview(operation Operation) (Plan, error) {
	if operation == Uninstall {
		return facade.previewUninstall()
	}
	if operation != Install && operation != Update {
		return Plan{}, fmt.Errorf("preview unsupported core lifecycle operation %q", operation)
	}
	if operation == Update {
		if err := facade.validateUpdateInstalledSource(); err != nil {
			return Plan{}, err
		}
	}
	prior, priorFound, err := LoadState(facade.config.State.StateFile())
	if err != nil {
		return Plan{}, err
	}
	discovered, err := skillbundle.Discover(facade.config.SkillSource.Root, facade.config.Skills.Root(), facade.config.SkillSource.MissingHint)
	if err != nil {
		return Plan{}, err
	}
	actions := []plannedAction{{ActionView: ActionView{Kind: ActionWriteFile, Path: facade.config.State.StateFile(), Description: "persist Packy state metadata"}}}
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
	engram := facade.config.Engram.ExpectedPath()
	actions = append(actions,
		plannedAction{ActionView: ActionView{Kind: ActionRun, Command: engram, Args: []string{"setup", "codex"}, Description: "delegate Codex Engram setup through Homebrew binary"}},
		plannedAction{ActionView: ActionView{Kind: ActionRun, Command: engram, Args: []string{"setup", "opencode"}, Description: "delegate OpenCode Engram setup through Homebrew binary"}},
		plannedAction{ActionView: ActionView{Kind: ActionWriteCodexPrompt, Path: facade.config.Codex.PromptFile(), Description: "write Codex Packy prompt markers"}},
		plannedAction{ActionView: ActionView{Kind: ActionWriteOpenCodePrompt, Path: facade.config.OpenCode.ConfigFile(), Target: facade.config.OpenCode.PromptFile(), Description: "write OpenCode Packy prompt reference"}},
	)
	claudeDesired := cloneClassicDesired(facade.config.ClaudeDesired)
	if len(claudeDesired.Skills) == 0 {
		claudeDesired.Skills = make([]claudecode.ClassicSkill, 0, len(discovered))
		for _, skill := range discovered {
			claudeDesired.Skills = append(claudeDesired.Skills, claudecode.ClassicSkill{
				ID: "classic:skill:" + skill.Name, Name: skill.Name, SourcePath: skill.SourcePath,
			})
		}
	}
	var claudePlan claudecode.ClassicPlan
	hasClaudePlan := facade.config.Claude != nil
	if hasClaudePlan {
		claudePlan, err = facade.config.Claude.InspectClassic(context.Background(), claudecode.ClassicRequest{Goal: claudecode.ClassicPresent, Desired: claudeDesired})
		if err != nil {
			return Plan{}, err
		}
		for _, action := range claudePlan.Actions() {
			actions = append(actions, plannedAction{ActionView: ActionView{Kind: ActionKind("claude-" + string(action.Kind)), Path: action.Target, Description: action.Description}})
		}
	}
	desired := DesiredState(StateConfig{
		StateFile:      facade.config.State.StateFile(),
		AgentSkillsDir: facade.config.Skills.Root(),
	}, facade.now(), managed)
	fromSchema := 0
	fromStatus := InstallStatus("")
	if priorFound {
		fromSchema, fromStatus = prior.SchemaVersion, prior.InstallStatus
	}
	outcome := OutcomeApplied
	if len(actions) == 1 {
		outcome = OutcomeConverged
	}
	var blockers, preserved []string
	var pending []string
	if hasClaudePlan {
		blockers = append(blockers, claudePlan.Blockers()...)
		preserved = append(preserved, claudePlan.Preserved()...)
		pending = append(pending, claudePlan.PendingPrerequisites()...)
		if len(claudePlan.Blockers()) > 0 {
			outcome = OutcomeBlocked
		}
		if len(pending) > 0 && outcome != OutcomeBlocked {
			outcome = OutcomeAppliedWithPendingPrerequisite
		}
	}
	return Plan{
		owner:           facade,
		operation:       operation,
		actions:         actions,
		desired:         desired,
		outcome:         outcome,
		blockers:        blockers,
		preserved:       preserved,
		pending:         pending,
		transition:      StateTransitionView{FromSchemaVersion: fromSchema, ToSchemaVersion: SchemaVersion, FromStatus: fromStatus, ToStatus: InstallConfirmed},
		legacyMigration: priorFound && prior.Legacy(),
		claudePlan:      claudePlan,
		hasClaudePlan:   hasClaudePlan,
	}, nil
}

func cloneClassicDesired(in claudecode.ClassicDesired) claudecode.ClassicDesired {
	out := in
	out.Skills = append([]claudecode.ClassicSkill(nil), in.Skills...)
	if in.Instruction != nil {
		instruction := *in.Instruction
		out.Instruction = &instruction
	}
	if in.MCP != nil {
		mcp := *in.MCP
		mcp.Args = append([]string(nil), in.MCP.Args...)
		mcp.Environment = make(map[string]string, len(in.MCP.Environment))
		for key, value := range in.MCP.Environment {
			mcp.Environment[key] = value
		}
		out.MCP = &mcp
	}
	return out
}

// saveInstallState is a private persistence-failure seam. Filesystem behavior
// otherwise remains concrete and is exercised in sandbox directories.
var saveInstallState = SaveState

// saveUpdateState is the focused persistence-failure seam for update recovery.
var saveUpdateState = SaveState

func (facade *Facade) Apply(ctx context.Context, plan Plan) (Result, error) {
	if plan.owner != facade {
		return Result{}, ErrForeignPlan
	}
	var result Result
	var err error
	switch plan.operation {
	case Uninstall:
		result, err = facade.applyUninstall(ctx, plan)
	case Install, Update:
		result, err = facade.applyInstall(ctx, plan)
	default:
		return Result{}, ErrForeignPlan
	}
	return plan.withPublicationFacts(result), err
}

func (facade *Facade) applyInstall(ctx context.Context, plan Plan) (applyResult Result, err error) {
	recoveryPublished := false
	defer func() {
		if err != nil && recoveryPublished && applyResult.outcome == "" {
			applyResult = Result{outcome: OutcomeRecoveryRequired}
		}
	}()
	if plan.legacyMigration && len(plan.blockers) > 0 {
		return Result{outcome: OutcomeBlocked, notStartedEffects: actionEffectIDs(plan.actions)}, fmt.Errorf("%w: legacy state remains authoritative", ErrBlockedPlan)
	}
	saveState := saveInstallState
	if plan.operation == Update {
		saveState = saveUpdateState
	}
	previous, previousFound, err := LoadState(facade.config.State.StateFile())
	if err != nil {
		return Result{}, err
	}
	anchor, err := facade.provisionStateAnchor()
	if err != nil {
		return Result{}, err
	}
	recovery := facade.recoveryState(plan.desired, previous, previousFound, anchor)
	if previousFound && !previous.Legacy() {
		recovery.ClaudeOwnership = append([]ClaudeOwnership(nil), previous.ClaudeOwnership...)
	}
	legacyMigration := previousFound && previous.Legacy()
	if !legacyMigration {
		if err := saveState(facade.config.State.StateFile(), recovery); err != nil {
			if cleanupErr := cleanupInstallContainers(anchor); cleanupErr != nil {
				return Result{}, fmt.Errorf("%w; clean up unrecorded Packy containers: %v", err, cleanupErr)
			}
			return Result{}, err
		}
		recoveryPublished = true
	}
	created, provisionErr := ownedcontainer.Provision(facade.effectContainerRecords())
	recovery.CreatedContainers = ownedcontainer.Merge(recovery.CreatedContainers, created)
	if !legacyMigration {
		if err := saveState(facade.config.State.StateFile(), recovery); err != nil {
			if cleanupErr := cleanupInstallContainers(created); cleanupErr != nil {
				return Result{}, fmt.Errorf("%w; clean up unrecorded Packy containers: %v", err, cleanupErr)
			}
			return Result{}, err
		}
	}
	if provisionErr != nil {
		return Result{}, provisionErr
	}
	if err := os.MkdirAll(facade.config.State.PackyHome(), 0o700); err != nil {
		return Result{}, fmt.Errorf("create Packy config directory %s: %w", facade.config.State.PackyHome(), err)
	}
	if err := os.MkdirAll(facade.config.Skills.Root(), 0o700); err != nil {
		return Result{}, fmt.Errorf("create agent skills directory %s: %w", facade.config.Skills.Root(), err)
	}
	var warnings []string
	for _, action := range plan.actions {
		if strings.HasPrefix(string(action.Kind), "claude-") {
			continue
		}
		switch action.Kind {
		case ActionSymlink:
			if err := os.Symlink(action.Target, action.Path); err != nil {
				return Result{}, fmt.Errorf("create skill symlink %s -> %s: %w", action.Path, action.Target, err)
			}
			recovery.ManagedSkills = append(recovery.ManagedSkills, managedSkillForInstallAction(plan.desired.ManagedSkills, action))
			if !legacyMigration {
				if err := saveState(facade.config.State.StateFile(), recovery); err != nil {
					if removeErr := os.Remove(action.Path); removeErr != nil {
						return Result{}, fmt.Errorf("%w; roll back unrecorded skill symlink %s: %v", err, action.Path, removeErr)
					}
					return Result{}, err
				}
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
				canonical := facade.config.Engram.Observe(nil).Homebrew()
				if canonical == nil {
					return Result{}, missingInstallEngramError(action, facade.config.Engram.Candidates())
				}
				command = canonical.Path
			}
			if err := facade.commands.Run(ctx, command, action.Args...); err != nil {
				action.Command = command
				return Result{}, lifecycleActionRunError(action, err)
			}
		}
	}
	var claudeResult claudecode.ClassicApplyResult
	if plan.hasClaudePlan {
		claudeResult, err = facade.config.Claude.ApplyClassic(ctx, plan.claudePlan)
		if err != nil {
			if claudeResult.RolledBack {
				if legacyMigration {
					return Result{outcome: OutcomeRolledBack, completedEffects: actionEffectIDs(plan.actions), failedEffect: claudeResult.Failed, notStartedEffects: claudeResult.NotStarted}, err
				}
				rolledBack := plan.desired
				rolledBack.ManagedSkills = append([]ManagedSkill(nil), recovery.ManagedSkills...)
				rolledBack.CreatedContainers = append([]ownedcontainer.Record(nil), recovery.CreatedContainers...)
				rolledBack.ClaudeOwnership = append([]ClaudeOwnership(nil), previous.ClaudeOwnership...)
				rolledBack.InstallStatus = InstallConfirmed
				rolledBack.LatestAttempt = &LatestAttempt{Operation: plan.operation, Outcome: AttemptRolledBack, CompletedEffects: actionEffectIDs(plan.actions), FailedEffect: claudeResult.Failed, NotStartedEffects: claudeResult.NotStarted}
				if saveErr := saveState(facade.config.State.StateFile(), rolledBack); saveErr != nil {
					return Result{}, fmt.Errorf("%w; publish exact-rollback attempt: %v", err, saveErr)
				}
				return Result{outcome: OutcomeRolledBack, completedEffects: actionEffectIDs(plan.actions), failedEffect: claudeResult.Failed, notStartedEffects: claudeResult.NotStarted}, err
			}
			if !claudeResult.Attempted {
				if legacyMigration {
					return Result{outcome: OutcomePartiallyApplied, completedEffects: actionEffectIDs(plan.actions), notStartedEffects: claudeResult.NotStarted}, err
				}
				blocked := plan.desired
				blocked.ManagedSkills = append([]ManagedSkill(nil), recovery.ManagedSkills...)
				blocked.CreatedContainers = append([]ownedcontainer.Record(nil), recovery.CreatedContainers...)
				blocked.ClaudeOwnership = append([]ClaudeOwnership(nil), previous.ClaudeOwnership...)
				blocked.InstallStatus = InstallConfirmed
				blocked.LatestAttempt = &LatestAttempt{Operation: plan.operation, Outcome: AttemptPartiallyApplied, CompletedEffects: actionEffectIDs(plan.actions), NotStartedEffects: claudeResult.NotStarted}
				if saveErr := saveState(facade.config.State.StateFile(), blocked); saveErr != nil {
					return Result{}, fmt.Errorf("%w; publish blocked Claude attempt: %v", err, saveErr)
				}
				return Result{outcome: OutcomePartiallyApplied, completedEffects: actionEffectIDs(plan.actions), notStartedEffects: claudeResult.NotStarted}, err
			}
			recovery.ClaudeOwnership = mergeClaudeOwnership(recovery.ClaudeOwnership, classicStateOwnership(claudeResult.VerifiedOwnership))
			recovery.InstallStatus = InstallRecoveryRequired
			recovery.LatestAttempt = &LatestAttempt{Operation: plan.operation, Outcome: AttemptRecoveryRequired, CompletedEffects: append(actionEffectIDs(plan.actions), claudeResult.Completed...), FailedEffect: claudeResult.Failed, NotStartedEffects: claudeResult.NotStarted}
			if saveErr := saveState(facade.config.State.StateFile(), recovery); saveErr != nil {
				return Result{}, fmt.Errorf("%w; publish Claude recovery state: %v", err, saveErr)
			}
			return Result{outcome: OutcomeRecoveryRequired, completedEffects: claudeResult.Completed, failedEffect: claudeResult.Failed, notStartedEffects: claudeResult.NotStarted}, err
		}
		ownership := plan.claudePlan.DesiredOwnership()
		if len(plan.pending) > 0 {
			ownership = withoutPendingMCPOwnership(ownership)
		}
		plan.desired.ClaudeOwnership = mergeClaudeOwnership(previous.ClaudeOwnership, classicStateOwnership(ownership))
	}
	confirmed := plan.desired
	if previous.RecoveryRequired() {
		confirmed.ManagedSkills = append([]ManagedSkill(nil), recovery.ManagedSkills...)
	}
	confirmed.CreatedContainers = append([]ownedcontainer.Record(nil), recovery.CreatedContainers...)
	confirmed.InstallStatus = InstallConfirmed
	attemptOutcome := AttemptVerified
	if len(plan.blockers) > 0 {
		attemptOutcome = AttemptBlocked
		if len(actionEffectIDs(plan.actions))+len(claudeResult.Completed) > 0 {
			attemptOutcome = AttemptPartiallyApplied
		}
	}
	confirmed.LatestAttempt = &LatestAttempt{Operation: plan.operation, Outcome: attemptOutcome, CompletedEffects: append(actionEffectIDs(plan.actions), claudeResult.Completed...), NotStartedEffects: []string{}}
	if err := saveState(facade.config.State.StateFile(), confirmed); err != nil {
		return Result{}, err
	}
	if warning, ok := unmanagedInstallSymlinkWarning(plan); ok {
		warnings = append(warnings, warning)
	}
	completed := append(actionEffectIDs(plan.actions), claudeResult.Completed...)
	return Result{warnings: warnings, managedSkillCount: len(plan.desired.ManagedSkills), stateFile: facade.config.State.StateFile(), hasWork: true, outcome: plan.outcome, completedEffects: completed}, nil
}

func (plan Plan) withPublicationFacts(result Result) Result {
	result.stateTransition = plan.transition
	switch result.outcome {
	case "":
		result.stateTransition.ToSchemaVersion = result.stateTransition.FromSchemaVersion
		result.stateTransition.ToStatus = result.stateTransition.FromStatus
	case OutcomeBlocked:
		if plan.legacyMigration {
			result.stateTransition.ToSchemaVersion = result.stateTransition.FromSchemaVersion
			result.stateTransition.ToStatus = result.stateTransition.FromStatus
			return result
		}
		result.committed = true
		result.stateTransition.ToSchemaVersion = SchemaVersion
		result.stateTransition.ToStatus = InstallConfirmed
	case OutcomeConverged, OutcomeApplied, OutcomeAppliedWithPendingPrerequisite:
		result.committed = true
	case OutcomeRolledBack, OutcomePartiallyApplied:
		if plan.legacyMigration {
			result.stateTransition.ToSchemaVersion = result.stateTransition.FromSchemaVersion
			result.stateTransition.ToStatus = result.stateTransition.FromStatus
			return result
		}
		result.committed = true
		result.stateTransition.ToSchemaVersion = SchemaVersion
		result.stateTransition.ToStatus = InstallConfirmed
	case OutcomeRecoveryRequired:
		result.committed = true
		result.stateTransition.ToSchemaVersion = SchemaVersion
		result.stateTransition.ToStatus = InstallRecoveryRequired
	case OutcomeUninstallIncomplete:
		result.committed = true
		result.stateTransition.ToSchemaVersion = SchemaVersion
		result.stateTransition.ToStatus = InstallUninstallIncomplete
	}
	return result
}

func defaultClaudeDesired() claudecode.ClassicDesired {
	return claudecode.ClassicDesired{
		Instruction: &claudecode.ClassicInstruction{ID: "classic:instruction", Content: prompt.CodexContent() + "\n" + prompt.RulesContent()},
		MCP:         &claudecode.ClassicMCP{ID: "classic:mcp:engram", Name: "engram", Command: "engram", Args: []string{"mcp", "--tools=agent"}},
	}
}

func mergeClaudeOwnership(prior, verified []ClaudeOwnership) []ClaudeOwnership {
	byID := make(map[string]ClaudeOwnership, len(prior)+len(verified))
	for _, ownership := range prior {
		byID[ownership.ID] = ownership
	}
	for _, ownership := range verified {
		byID[ownership.ID] = ownership
	}
	out := make([]ClaudeOwnership, 0, len(byID))
	for _, ownership := range byID {
		out = append(out, ownership)
	}
	return out
}

func withoutPendingMCPOwnership(records []claudecode.OwnershipRecord) []claudecode.OwnershipRecord {
	out := records[:0]
	for _, r := range records {
		if strings.TrimPrefix(r.Kind, "claude-") != "user-mcp" {
			out = append(out, r)
		}
	}
	return out
}

func classicStateOwnership(records []claudecode.OwnershipRecord) []ClaudeOwnership {
	out := make([]ClaudeOwnership, 0, len(records))
	for _, r := range records {
		kind := strings.TrimPrefix(r.Kind, "claude-")
		if kind == "skill-link" {
			kind = ClaudeOwnershipSkill
		}
		if kind == "instruction-contribution" {
			kind = ClaudeOwnershipInstruction
		}
		if kind == "agent-file" {
			kind = ClaudeOwnershipAgent
		}
		if kind == "command-hook" {
			kind = ClaudeOwnershipHook
		}
		if kind == "user-mcp" {
			kind = ClaudeOwnershipMCP
		}
		out = append(out, ClaudeOwnership{ID: r.ID, Kind: kind, Target: r.Target, Fingerprint: r.Fingerprint, Contributors: append([]string(nil), r.Contributors...), SourcePath: r.Skill.ExpectedSource, LinkTarget: r.Skill.ResolvedTarget, Command: r.Command, Args: append([]string(nil), r.Args...), EnvironmentKeys: append([]string(nil), r.EnvironmentKeys...), EnvironmentFingerprint: r.EnvironmentFingerprint, DeletionAuthorized: r.DeletionAuthorized})
	}
	return out
}

func actionEffectIDs(actions []plannedAction) []string {
	ids := make([]string, 0, len(actions))
	for i, action := range actions {
		if action.Kind == ActionSkip || strings.HasPrefix(string(action.Kind), "claude-") {
			continue
		}
		id := fmt.Sprintf("%s:%d", action.Kind, i)
		if action.Path != "" {
			id = string(action.Kind) + ":" + action.Path
		}
		ids = append(ids, id)
	}
	return ids
}

func (facade *Facade) homebrewEngramInstalled() bool {
	return facade.config.Engram.Observe(facade.commands.LookPath).Installed()
}

type SkillLinkCondition string

const (
	SkillLinkMissing          SkillLinkCondition = "missing"
	SkillLinkManaged          SkillLinkCondition = "managed"
	SkillLinkUnmanagedPath    SkillLinkCondition = "unmanaged-path"
	SkillLinkUnmanagedSymlink SkillLinkCondition = "unmanaged-symlink"
)

// SkillLinkObservation is a detached read-only view of one managed skill link.
type SkillLinkObservation struct {
	name      string
	linkPath  string
	target    string
	condition SkillLinkCondition
	err       error
}

func (observation SkillLinkObservation) Name() string                  { return observation.name }
func (observation SkillLinkObservation) LinkPath() string              { return observation.linkPath }
func (observation SkillLinkObservation) Target() string                { return observation.target }
func (observation SkillLinkObservation) Condition() SkillLinkCondition { return observation.condition }
func (observation SkillLinkObservation) Err() error                    { return observation.err }

// ObserveManagedSkillLinks inspects recorded ownership without mutating it or
// the filesystem. Inspection failures are reported on the corresponding fact.
func ObserveManagedSkillLinks(skills []ManagedSkill) []SkillLinkObservation {
	observations := make([]SkillLinkObservation, len(skills))
	for i, skill := range skills {
		observations[i] = observeManagedSkillLink(skill)
	}
	return observations
}

// ObserveExpectedManagedSkillLinks discovers the configured bundle through
// the lifecycle owner and returns detached, read-only link facts for doctor.
func ObserveExpectedManagedSkillLinks(config Config) ([]SkillLinkObservation, error) {
	discovered, err := skillbundle.Discover(config.SkillSourceRoot, config.AgentSkillsDir, config.SkillSourceMissingHint)
	if err != nil {
		return nil, err
	}
	skills := make([]ManagedSkill, 0, len(discovered))
	for _, skill := range discovered {
		skills = append(skills, ManagedSkill{Name: skill.Name, SourcePath: skill.SourcePath, LinkPath: skill.LinkPath})
	}
	return ObserveManagedSkillLinks(skills), nil
}

func observeManagedSkillLink(skill ManagedSkill) SkillLinkObservation {
	observation := SkillLinkObservation{name: skill.Name, linkPath: skill.LinkPath}
	info, err := os.Lstat(skill.LinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			observation.condition = SkillLinkMissing
			return observation
		}
		observation.err = fmt.Errorf("inspect skill link %s: %w", skill.LinkPath, err)
		return observation
	}
	if info.Mode()&os.ModeSymlink == 0 {
		observation.condition = SkillLinkUnmanagedPath
		return observation
	}
	target, err := os.Readlink(skill.LinkPath)
	if err != nil {
		observation.err = fmt.Errorf("read skill link %s: %w", skill.LinkPath, err)
		return observation
	}
	observation.target = target
	if sameLinkTarget(skill.LinkPath, target, skill.SourcePath) {
		observation.condition = SkillLinkManaged
		return observation
	}
	observation.condition = SkillLinkUnmanagedSymlink
	return observation
}

func previewSkillLink(skill ManagedSkill) (plannedAction, error) {
	observation := observeManagedSkillLink(skill)
	if observation.Err() != nil {
		return plannedAction{}, observation.Err()
	}
	switch observation.Condition() {
	case SkillLinkMissing:
		return plannedAction{ActionView: ActionView{Kind: ActionSymlink, Path: skill.LinkPath, Target: skill.SourcePath, Description: "link managed skill " + skill.Name}, skipReason: SkillLinkMissing}, nil
	case SkillLinkManaged:
		return plannedAction{}, nil
	case SkillLinkUnmanagedPath:
		return plannedAction{ActionView: ActionView{Kind: ActionSkip, Path: skill.LinkPath, Target: skill.SourcePath, Description: "preserve unmanaged path for skill " + skill.Name}, skipReason: SkillLinkUnmanagedPath}, nil
	case SkillLinkUnmanagedSymlink:
		return plannedAction{ActionView: ActionView{Kind: ActionSkip, Path: skill.LinkPath, Target: observation.Target(), Description: "preserve unmanaged symlink for skill " + skill.Name}, skipReason: SkillLinkUnmanagedSymlink}, nil
	default:
		return plannedAction{}, fmt.Errorf("inspect skill link %s: unknown condition %q", skill.LinkPath, observation.Condition())
	}
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
	if _, err := os.Lstat(facade.config.State.PackyHome()); os.IsNotExist(err) {
		if err := os.Mkdir(facade.config.State.PackyHome(), 0o700); err != nil {
			return nil, fmt.Errorf("create Packy config directory %s: %w", facade.config.State.PackyHome(), err)
		}
		created = append(created, ownedcontainer.Record{Path: facade.config.State.PackyHome(), Kind: ownedcontainer.Directory})
	} else if err != nil {
		return nil, fmt.Errorf("inspect Packy config directory %s: %w", facade.config.State.PackyHome(), err)
	}
	if _, err := os.Lstat(facade.config.State.StateFile()); os.IsNotExist(err) {
		created = append(created, ownedcontainer.Record{Path: facade.config.State.StateFile(), Kind: ownedcontainer.File})
	} else if err != nil {
		return nil, fmt.Errorf("inspect Packy state %s: %w", facade.config.State.StateFile(), err)
	}
	return created, nil
}

func (facade *Facade) effectContainerRecords() []ownedcontainer.Record {
	records := facade.containerRecords()
	out := make([]ownedcontainer.Record, 0, len(records)-2)
	for _, record := range records {
		if record.Path != facade.config.State.PackyHome() && record.Path != facade.config.State.StateFile() {
			out = append(out, record)
		}
	}
	return out
}

func (facade *Facade) containerRecords() []ownedcontainer.Record {
	return []ownedcontainer.Record{
		{Path: facade.config.State.PackyHome(), Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(facade.config.Skills.Root()), Kind: ownedcontainer.Directory},
		{Path: facade.config.Skills.Root(), Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(facade.config.Codex.PromptFile()), Kind: ownedcontainer.Directory},
		{Path: facade.config.OpenCode.ConfigurationHome(), Kind: ownedcontainer.Directory},
		{Path: filepath.Dir(facade.config.OpenCode.ConfigFile()), Kind: ownedcontainer.Directory},
		{Path: facade.config.State.StateFile(), Kind: ownedcontainer.File},
		{Path: facade.config.Codex.PromptFile(), Kind: ownedcontainer.File},
		{Path: facade.config.OpenCode.ConfigFile(), Kind: ownedcontainer.File},
		{Path: facade.config.OpenCode.PromptFile(), Kind: ownedcontainer.File},
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
	return fmt.Errorf("run %s: canonical Homebrew Engram was not found at any expected Homebrew path (%s); run brew install %s or set HOMEBREW_PREFIX to the active Homebrew prefix, then retry packy install or packy update", strings.Join(append([]string{action.Command}, action.Args...), " "), strings.Join(candidates, ", "), engrambin.Formula)
}

func lifecycleActionRunError(action plannedAction, err error) error {
	command := strings.Join(append([]string{action.Command}, action.Args...), " ")
	switch {
	case action.Command == "brew" && len(action.Args) > 0 && action.Args[0] == "install":
		return fmt.Errorf("run %s: failed to install Engram via Homebrew; ensure Homebrew is installed and retry: %w", command, err)
	case action.Command == "brew" && len(action.Args) > 0 && (action.Args[0] == "update" || action.Args[0] == "upgrade"):
		return fmt.Errorf("run %s: failed to update Engram via Homebrew; ensure Homebrew is installed and retry: %w", command, err)
	case isInstallEngramSetup(action):
		return fmt.Errorf("run %s: failed to configure Engram for %s through the Homebrew-managed binary; run brew install %s or brew upgrade engram, then retry packy install or packy update: %w", command, action.Args[1], engrambin.Formula, err)
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
		if action.skipReason == SkillLinkUnmanagedSymlink {
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
	return fmt.Sprintf("skipped %d unmanaged skill symlinks; setup may be incomplete. Example: %s -> %s. Safe recovery: verify these are stale Packy-created links, remove them, then run packy install; Packy will not overwrite arbitrary files or links.", count, example.Path, example.Target), true
}
