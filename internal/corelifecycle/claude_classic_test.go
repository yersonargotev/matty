package corelifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/claudecode"
)

type classicVersionRunner struct{ result claudecode.Result }

func (r classicVersionRunner) Run(context.Context, claudecode.Command) claudecode.Result {
	return r.result
}

type classicMCPRunner struct {
	registry string
	failAdd  bool
	calls    []claudecode.Command
}

type cancelAwareClassicRunner struct {
	registry string
	seenErr  error
}

func (runner *cancelAwareClassicRunner) Run(ctx context.Context, command claudecode.Command) claudecode.Result {
	if len(command.Args) == 1 && command.Args[0] == "--version" {
		return claudecode.Result{Stdout: "2.1.203"}
	}
	runner.seenErr = ctx.Err()
	return claudecode.Result{Err: ctx.Err()}
}

func (runner *classicMCPRunner) Run(_ context.Context, command claudecode.Command) claudecode.Result {
	runner.calls = append(runner.calls, command)
	if len(command.Args) == 1 && command.Args[0] == "--version" {
		return claudecode.Result{Stdout: "2.1.203"}
	}
	if runner.failAdd && len(command.Args) >= 2 && command.Args[0] == "mcp" && command.Args[1] == "add" {
		return claudecode.Result{Err: errors.New("MCP add failed")}
	}
	if len(command.Args) >= 2 && command.Args[0] == "mcp" {
		body := `{"mcpServers":{"engram":{"command":"engram","args":["mcp","--tools=agent"],"env":{}}}}`
		if command.Args[1] == "remove" {
			body = `{"mcpServers":{}}`
		}
		if err := os.WriteFile(runner.registry, []byte(body), 0o600); err != nil {
			return claudecode.Result{Err: err}
		}
	}
	return claudecode.Result{}
}

func configureClassicClaude(config *facadeConfig, runner claudecode.Runner) {
	home := installTestHome(*config)
	layout := claudecode.NewCanonicalLayout(home)
	provider := claudecode.OwnershipSnapshotFunc(func(context.Context) (claudecode.OwnershipSnapshot, error) {
		return ObserveClaudeOwnershipSnapshot(config.State.StateFile())
	})
	config.Claude = claudecode.NewSurfaceAdapter("", layout, config.State.PackyHome(), "claude", runner, provider)
	config.ClaudeDesired = claudecode.ClassicDesired{
		Instruction: &claudecode.ClassicInstruction{ID: "classic:instruction", Content: "Use Packy skills when relevant."},
		MCP:         &claudecode.ClassicMCP{ID: "classic:mcp:engram", Name: "engram", Command: "engram", Args: []string{"mcp", "--tools=agent"}},
	}
}

func TestClassicClaudePendingConvergesLocalWorkWithoutPendingMCPOwnership(t *testing.T) {
	config := installTestConfig(t)
	home := installTestHome(config)
	source := filepath.Join(t.TempDir(), "claude-skill")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("classic"), 0600); err != nil {
		t.Fatal(err)
	}
	config.Claude = claudecode.NewSurfaceAdapter("", claudecode.NewCanonicalLayout(home), config.State.PackyHome(), "", classicVersionRunner{}, claudecode.StaticOwnershipSnapshot(claudecode.OwnershipSnapshot{}))
	config.ClaudeDesired = claudecode.ClassicDesired{
		Skills:      []claudecode.ClassicSkill{{ID: "classic:skill:test", Name: "classic-test", SourcePath: source}},
		Instruction: &claudecode.ClassicInstruction{ID: "classic:instruction", Content: "Use Packy skills when relevant."},
		MCP:         &claudecode.ClassicMCP{ID: "classic:mcp:engram", Name: "engram", Command: "engram", Args: []string{"mcp"}},
	}
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Outcome() != OutcomeAppliedWithPendingPrerequisite || len(plan.PendingPrerequisites()) == 0 {
		t.Fatalf("outcome=%s pending=%v", plan.Outcome(), plan.PendingPrerequisites())
	}
	result, err := facade.Apply(context.Background(), plan)
	if err != nil || result.Outcome() != OutcomeAppliedWithPendingPrerequisite {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	state, _, err := LoadState(config.State.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ClaudeOwnership) != 2 {
		t.Fatalf("pending MCP ownership was recorded: %+v", state.ClaudeOwnership)
	}
	for _, ownership := range state.ClaudeOwnership {
		if ownership.Kind == "mcp" {
			t.Fatal("pending MCP ownership recorded")
		}
	}
}

func TestLegacyClaudeBlockerLeavesV1AuthoritativeAndRunsNoEffects(t *testing.T) {
	config := installTestConfig(t)
	home := installTestHome(config)
	if err := os.MkdirAll(config.State.PackyHome(), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"schema_version":1,"packy_version":"old","managed_skills":[],"configured_surfaces":["codex","opencode"],"paths":{"state_file":"x","agent_skills_dir":"y"}}`
	if err := os.WriteFile(config.State.StateFile(), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	layout := claudecode.NewCanonicalLayout(home)
	if err := os.MkdirAll(layout.SkillsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.SkillsDir, "blocked"), []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	config.Claude = claudecode.NewSurfaceAdapter("", layout, config.State.PackyHome(), "claude", classicVersionRunner{result: claudecode.Result{Stdout: "2.1.203"}}, claudecode.StaticOwnershipSnapshot(claudecode.OwnershipSnapshot{}))
	config.ClaudeDesired = claudecode.ClassicDesired{Skills: []claudecode.ClassicSkill{{ID: "classic:skill:blocked", Name: "blocked", SourcePath: source}}}
	commands := &installTestCommands{}
	facade := newTestFacade(config, commands, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Outcome() != OutcomeBlocked {
		t.Fatalf("outcome=%s blockers=%v", plan.Outcome(), plan.Blockers())
	}
	if _, err := facade.Apply(context.Background(), plan); !errors.Is(err, ErrBlockedPlan) {
		t.Fatalf("Apply error=%v", err)
	}
	if len(commands.runs) != 0 {
		t.Fatalf("effects ran: %v", commands.runs)
	}
	state, _, err := LoadState(config.State.StateFile())
	if err != nil || !state.Legacy() {
		t.Fatalf("legacy lost: %+v %v", state, err)
	}
}

func TestClassicPrototypeVerifiedV1MigrationPublishesV2OnlyAfterMCPVerification(t *testing.T) {
	config := installTestConfig(t)
	if err := os.MkdirAll(config.State.PackyHome(), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"schema_version":1,"packy_version":"old","managed_skills":[],"configured_surfaces":["codex","opencode"],"paths":{"state_file":"x","agent_skills_dir":"y"}}`
	if err := os.WriteFile(config.State.StateFile(), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	layout := claudecode.NewCanonicalLayout(installTestHome(config))
	runner := &classicMCPRunner{registry: layout.UserMCPFile}
	configureClassicClaude(&config, runner)
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Update)
	if err != nil || plan.StateTransition().FromSchemaVersion != LegacySchemaVersion {
		t.Fatalf("migration preview=%+v err=%v", plan.StateTransition(), err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	state, found, err := LoadState(config.State.StateFile())
	if err != nil || !found || state.SchemaVersion != SchemaVersion || state.InstallStatus != InstallConfirmed {
		t.Fatalf("migrated state=%+v found=%v err=%v", state, found, err)
	}
	if state.LatestAttempt == nil || state.LatestAttempt.Outcome != AttemptVerified || len(state.ClaudeOwnership) == 0 {
		t.Fatalf("migration evidence=%+v ownership=%+v", state.LatestAttempt, state.ClaudeOwnership)
	}
}

func TestClassicPrototypeRecoveryRetryBuildsFreshPlanAndConverges(t *testing.T) {
	config := installTestConfig(t)
	layout := claudecode.NewCanonicalLayout(installTestHome(config))
	failing := &classicMCPRunner{registry: layout.UserMCPFile, failAdd: true}
	configureClassicClaude(&config, failing)
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Apply(context.Background(), plan)
	if err == nil || result.Outcome() != OutcomeRecoveryRequired || result.FailedEffect() != "classic:mcp:engram" {
		t.Fatalf("first attempt=%+v err=%v", result, err)
	}
	recovery, _, err := LoadState(config.State.StateFile())
	if err != nil || !recovery.RecoveryRequired() || recovery.LatestAttempt == nil {
		t.Fatalf("recovery=%+v err=%v", recovery, err)
	}

	succeeding := &classicMCPRunner{registry: layout.UserMCPFile}
	configureClassicClaude(&config, succeeding)
	retry := newTestFacade(config, &installTestCommands{}, time.Now)
	fresh, err := retry.Preview(Install)
	if err != nil || len(fresh.Actions()) == 0 {
		t.Fatalf("fresh retry plan actions=%v err=%v", fresh.Actions(), err)
	}
	if _, err := retry.Apply(context.Background(), fresh); err != nil {
		t.Fatal(err)
	}
	confirmed, _, err := LoadState(config.State.StateFile())
	if err != nil || confirmed.RecoveryRequired() || confirmed.LatestAttempt == nil || confirmed.LatestAttempt.Outcome != AttemptVerified {
		t.Fatalf("confirmed=%+v err=%v", confirmed, err)
	}
}

func TestClassicExactLocalRollbackHasDistinctOutcomeAndAttempt(t *testing.T) {
	config := installTestConfig(t)
	home := installTestHome(config)
	config.Claude = claudecode.NewSurfaceAdapter("", claudecode.NewCanonicalLayout(home), config.State.PackyHome(), "claude", classicVersionRunner{result: claudecode.Result{Stdout: "2.1.203"}}, claudecode.StaticOwnershipSnapshot(claudecode.OwnershipSnapshot{}))
	config.ClaudeDesired = claudecode.ClassicDesired{Instruction: &claudecode.ClassicInstruction{ID: "classic:instruction", Content: "instructions"}}
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(skillsDir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillsDir, 0700) })
	result, err := facade.Apply(context.Background(), plan)
	if err == nil || result.Outcome() != OutcomeRolledBack || result.FailedEffect() != "classic:skill:ask-matt" {
		t.Fatalf("rollback result=%+v err=%v", result, err)
	}
	state, found, loadErr := LoadState(config.State.StateFile())
	if loadErr != nil || !found || state.RecoveryRequired() || state.LatestAttempt == nil || state.LatestAttempt.Outcome != AttemptRolledBack {
		t.Fatalf("rolled-back state=%+v found=%v err=%v", state, found, loadErr)
	}
}

func TestClassicExactLocalRollbackKeepsV1Authoritative(t *testing.T) {
	config := installTestConfig(t)
	home := installTestHome(config)
	if err := os.MkdirAll(config.State.PackyHome(), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"schema_version":1,"packy_version":"old","managed_skills":[],"configured_surfaces":["codex","opencode"],"paths":{"state_file":"x","agent_skills_dir":"y"}}`
	if err := os.WriteFile(config.State.StateFile(), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	config.Claude = claudecode.NewSurfaceAdapter("", claudecode.NewCanonicalLayout(home), config.State.PackyHome(), "claude", classicVersionRunner{result: claudecode.Result{Stdout: "2.1.203"}}, claudecode.StaticOwnershipSnapshot(claudecode.OwnershipSnapshot{}))
	config.ClaudeDesired = claudecode.ClassicDesired{Instruction: &claudecode.ClassicInstruction{ID: "classic:instruction", Content: "instructions"}}
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(skillsDir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillsDir, 0700) })
	result, err := facade.Apply(context.Background(), plan)
	if err == nil || result.Outcome() != OutcomeRolledBack {
		t.Fatalf("rollback result=%+v err=%v", result, err)
	}
	state, _, loadErr := LoadState(config.State.StateFile())
	if loadErr != nil || !state.Legacy() {
		t.Fatalf("legacy authority lost: %+v err=%v", state, loadErr)
	}
}

func TestClassicPrototypeResidualSafeUninstallRetainsThenClearsAuthority(t *testing.T) {
	config := installTestConfig(t)
	layout := claudecode.NewCanonicalLayout(installTestHome(config))
	runner := &classicMCPRunner{registry: layout.UserMCPFile}
	configureClassicClaude(&config, runner)
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	installPlan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), installPlan); err != nil {
		t.Fatal(err)
	}

	configureClassicClaude(&config, classicVersionRunner{})
	config.Claude = claudecode.NewSurfaceAdapter("", layout, config.State.PackyHome(), "", classicVersionRunner{}, claudecode.OwnershipSnapshotFunc(func(context.Context) (claudecode.OwnershipSnapshot, error) {
		return ObserveClaudeOwnershipSnapshot(config.State.StateFile())
	}))
	unavailable := newTestFacade(config, &installTestCommands{}, time.Now)
	uninstallPlan, err := unavailable.Preview(Uninstall)
	if err != nil || uninstallPlan.Outcome() != OutcomeUninstallIncomplete {
		t.Fatalf("unavailable preview outcome=%s pending=%v err=%v", uninstallPlan.Outcome(), uninstallPlan.PendingPrerequisites(), err)
	}
	result, err := unavailable.Apply(context.Background(), uninstallPlan)
	if err != nil || result.Outcome() != OutcomeUninstallIncomplete {
		t.Fatalf("unavailable uninstall=%+v err=%v", result, err)
	}
	residual, found, err := LoadState(config.State.StateFile())
	if err != nil || !found || !residual.UninstallIncomplete() {
		t.Fatalf("residual=%+v found=%v err=%v", residual, found, err)
	}
	if _, err := os.Stat(layout.UserMCPFile); err != nil {
		t.Fatalf("MCP residual was not preserved: %v", err)
	}

	cleanupRunner := &classicMCPRunner{registry: layout.UserMCPFile}
	configureClassicClaude(&config, cleanupRunner)
	cleanup := newTestFacade(config, &installTestCommands{}, time.Now)
	cleanupPlan, err := cleanup.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cleanup.Apply(context.Background(), cleanupPlan); err != nil {
		t.Fatal(err)
	}
	if _, found, err := LoadState(config.State.StateFile()); err != nil || found {
		t.Fatalf("residual authority not cleared: found=%v err=%v", found, err)
	}
}

func TestClassicPrototypeInertHealthPreviewPerformsNoEffects(t *testing.T) {
	config := installTestConfig(t)
	configureClassicClaude(&config, classicVersionRunner{})
	commands := &installTestCommands{}
	facade := newTestFacade(config, commands, time.Now)
	home := installTestHome(config)
	before := installTestSnapshot(t, home)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	after := installTestSnapshot(t, home)
	if before != after || len(commands.runs) != 0 {
		t.Fatalf("inert preview mutated sandbox or ran effects: before=%q after=%q runs=%v", before, after, commands.runs)
	}
	if len(plan.PendingPrerequisites()) == 0 {
		t.Fatalf("inert preview omitted compatibility evidence: %+v", plan)
	}
}

func TestSchemaV2BlockedClaudeProjectionRetainsResidualOwnership(t *testing.T) {
	config := installTestConfig(t)
	home := installTestHome(config)
	layout := claudecode.NewCanonicalLayout(home)
	source := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(layout.SkillsDir, "owned")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("foreign drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := DesiredState(StateConfig{StateFile: config.State.StateFile(), AgentSkillsDir: config.Skills.Root()}, time.Now(), nil)
	state.ClaudeOwnership = []ClaudeOwnership{{ID: "classic:skill:owned", Kind: "skill", Target: target, Fingerprint: "sha256:prior", Contributors: []string{"classic"}, SourcePath: source, LinkTarget: source, DeletionAuthorized: true}}
	if err := os.MkdirAll(config.State.PackyHome(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(config.State.StateFile(), state); err != nil {
		t.Fatal(err)
	}
	config.Claude = claudecode.NewSurfaceAdapter("", layout, config.State.PackyHome(), "", classicVersionRunner{}, claudecode.OwnershipSnapshotFunc(func(context.Context) (claudecode.OwnershipSnapshot, error) {
		return ObserveClaudeOwnershipSnapshot(config.State.StateFile())
	}))
	config.ClaudeDesired = claudecode.ClassicDesired{Skills: []claudecode.ClassicSkill{{ID: "classic:skill:owned", Name: "owned", SourcePath: source}}}
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil || plan.Outcome() != OutcomeBlocked {
		t.Fatalf("plan outcome=%s blockers=%v err=%v", plan.Outcome(), plan.Blockers(), err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	got, _, err := LoadState(config.State.StateFile())
	if err != nil || len(got.ClaudeOwnership) != 1 || got.ClaudeOwnership[0].ID != "classic:skill:owned" {
		t.Fatalf("blocked ownership was lost: %+v err=%v", got.ClaudeOwnership, err)
	}
}

func TestClaudeUninstallThreadsCallerCancellationToOfficialMCPRemoval(t *testing.T) {
	config := installTestConfig(t)
	layout := claudecode.NewCanonicalLayout(installTestHome(config))
	installRunner := &classicMCPRunner{registry: layout.UserMCPFile}
	configureClassicClaude(&config, installRunner)
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}

	cancelRunner := &cancelAwareClassicRunner{registry: layout.UserMCPFile}
	configureClassicClaude(&config, cancelRunner)
	uninstall := newTestFacade(config, &installTestCommands{}, time.Now)
	uninstallPlan, err := uninstall.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := uninstall.Apply(ctx, uninstallPlan); err == nil {
		t.Fatal("canceled MCP removal unexpectedly succeeded")
	}
	if !errors.Is(cancelRunner.seenErr, context.Canceled) {
		t.Fatalf("MCP runner did not receive caller cancellation: %v", cancelRunner.seenErr)
	}
}
