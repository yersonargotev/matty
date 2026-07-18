package corelifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/codex"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/skillbundle"
)

func TestInstallPreviewIsReadOnlyAndItsActionViewCannotMutateThePlan(t *testing.T) {
	config := installTestConfig(t)
	commands := &installTestCommands{}
	facade := newTestFacade(config, commands, func() time.Time { return time.Unix(123, 0) })
	before := installTestSnapshot(t, installTestHome(config))

	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatalf("Preview(Install) failed: %v", err)
	}
	first := plan.Actions()
	if len(first) == 0 {
		t.Fatal("Preview(Install) returned no actions")
	}
	want := append([]ActionView(nil), first...)
	first[0].Description = "caller mutation"
	if len(first[0].Args) > 0 {
		first[0].Args[0] = "caller mutation"
	}

	if got := plan.Actions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("caller changed opaque install plan:\ngot  %#v\nwant %#v", got, want)
	}
	if after := installTestSnapshot(t, installTestHome(config)); after != before {
		t.Fatalf("Preview(Install) mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(commands.lookups) != 1 || commands.lookups[0] != "engram" {
		t.Fatalf("command lookups = %#v, want [engram]", commands.lookups)
	}
	if len(commands.runs) != 0 {
		t.Fatalf("Preview(Install) executed commands: %#v", commands.runs)
	}
}

func TestInstallApplyConsumesThePreviewedPlanAndPublishesConfirmedOwnership(t *testing.T) {
	config := installTestConfig(t)
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	commands := &installTestCommands{}
	facade := newTestFacade(config, commands, func() time.Time { return time.Unix(123, 0) })
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	actions := plan.Actions()
	wantTail := []ActionKind{ActionRun, ActionRun, ActionWriteCodexPrompt, ActionWriteOpenCodePrompt}
	if len(actions) < len(wantTail) {
		t.Fatalf("install actions = %#v", actions)
	}
	for i, want := range wantTail {
		if got := actions[len(actions)-len(wantTail)+i].Kind; got != want {
			t.Fatalf("install action tail[%d] = %q, want %q; actions %#v", i, got, want, actions)
		}
	}

	result, err := facade.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if result.ManagedSkillCount() != 6 || result.StateFile() != config.State.StateFile() || len(result.Warnings()) != 0 {
		t.Fatalf("result = skills %d state %q warnings %#v", result.ManagedSkillCount(), result.StateFile(), result.Warnings())
	}
	state, found, err := LoadState(config.State.StateFile())
	if err != nil || !found {
		t.Fatalf("LoadState = %#v, %v, %v", state, found, err)
	}
	if state.RecoveryRequired() || state.InstallStatus != InstallConfirmed || len(state.ManagedSkills) != 6 {
		t.Fatalf("confirmed state = %#v", state)
	}
	if state.LastInstallCheck != "1970-01-01T00:02:03Z" {
		t.Fatalf("LastInstallCheck = %q, want injected clock", state.LastInstallCheck)
	}
	for _, path := range []string{config.State.PackyHome(), config.State.StateFile(), config.Skills.Root(), config.Codex.PromptFile(), config.OpenCode.ConfigFile(), config.OpenCode.PromptFile()} {
		if !installTestHasContainer(state, path) {
			t.Fatalf("confirmed state missing container provenance for %s: %#v", path, state.CreatedContainers)
		}
	}
	if got := commands.runs; !reflect.DeepEqual(got, []string{
		config.Engram.ExpectedPath() + " setup codex",
		config.Engram.ExpectedPath() + " setup opencode",
	}) {
		t.Fatalf("commands = %#v", got)
	}
	for _, skill := range state.ManagedSkills {
		if target, err := os.Readlink(skill.LinkPath); err != nil || target != skill.SourcePath {
			t.Fatalf("managed link %s = %q, %v", skill.LinkPath, target, err)
		}
	}
	if _, err := os.Stat(config.Codex.PromptFile()); err != nil {
		t.Fatalf("Codex prompt was not projected: %v", err)
	}
	if _, err := os.Stat(config.OpenCode.PromptFile()); err != nil {
		t.Fatalf("OpenCode prompt was not projected: %v", err)
	}

	other := newTestFacade(config, commands, time.Now)
	if _, err := other.Apply(context.Background(), plan); !errors.Is(err, ErrForeignPlan) {
		t.Fatalf("other facade Apply error = %v, want ErrForeignPlan", err)
	}
}

func TestClassicLifecycleDoesNotReadOrMutateCapabilityPackStateOrArtifacts(t *testing.T) {
	for _, operation := range []Operation{Install, Update, Uninstall} {
		t.Run(string(operation), func(t *testing.T) {
			config := installTestConfig(t)
			writeInstallTestExecutable(t, config.Engram.ExpectedPath())
			commands := &installTestCommands{}
			facade := newTestFacade(config, commands, func() time.Time { return time.Unix(123, 0) })

			if operation != Install {
				plan, err := facade.Preview(Install)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := facade.Apply(context.Background(), plan); err != nil {
					t.Fatal(err)
				}
			}

			packState := filepath.Join(config.State.PackyHome(), "packs.json")
			packArtifact := filepath.Join(installTestHome(config), ".codex", "pack-owned-artifact")
			if err := os.MkdirAll(config.State.PackyHome(), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(packArtifact), 0o700); err != nil {
				t.Fatal(err)
			}
			packStateContents := []byte(`{"deliberately":"not a classic state schema"}`)
			packArtifactContents := []byte("capability-pack-owned")
			if err := os.WriteFile(packState, packStateContents, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(packArtifact, packArtifactContents, 0o600); err != nil {
				t.Fatal(err)
			}

			plan, err := facade.Preview(operation)
			if err != nil {
				t.Fatalf("Preview(%s) read capability-pack state: %v", operation, err)
			}
			if _, err := facade.Apply(context.Background(), plan); err != nil {
				t.Fatalf("Apply(%s) failed: %v", operation, err)
			}
			if got, err := os.ReadFile(packState); err != nil || !reflect.DeepEqual(got, packStateContents) {
				t.Fatalf("%s changed pack state: contents %q err %v", operation, got, err)
			}
			if got, err := os.ReadFile(packArtifact); err != nil || !reflect.DeepEqual(got, packArtifactContents) {
				t.Fatalf("%s changed pack artifact: contents %q err %v", operation, got, err)
			}
		})
	}
}

func TestInstallApplyAcquiresMissingEngramBeforeDelegatedSetup(t *testing.T) {
	config := installTestConfig(t)
	engram := config.Engram.ExpectedPath()
	commands := &installTestCommands{after: map[string]func(){
		"brew install gentleman-programming/tap/engram": func() { writeInstallTestExecutable(t, engram) },
	}}
	facade := newTestFacade(config, commands, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"brew install gentleman-programming/tap/engram",
		engram + " setup codex",
		engram + " setup opencode",
	}
	if !reflect.DeepEqual(commands.runs, want) {
		t.Fatalf("commands = %#v, want %#v", commands.runs, want)
	}
}

func TestInstallPreviewDoesNotAdoptANonHomebrewEngramFromPATH(t *testing.T) {
	config := installTestConfig(t)
	other := filepath.Join(t.TempDir(), "engram")
	writeInstallTestExecutable(t, other)
	commands := &installTestCommands{paths: map[string]string{"engram": other}}
	facade := newTestFacade(config, commands, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if !installTestHasCommand(plan, "brew", "install gentleman-programming/tap/engram") {
		t.Fatal("non-Homebrew PATH executable incorrectly satisfied Engram acquisition")
	}
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	plan, err = facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if installTestHasCommand(plan, "brew", "install gentleman-programming/tap/engram") {
		t.Fatal("canonical Homebrew Engram still planned acquisition")
	}
}

func TestInstallPreviewRejectsCorruptStateBeforeMutation(t *testing.T) {
	config := installTestConfig(t)
	if err := os.MkdirAll(config.State.PackyHome(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.State.StateFile(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	commands := &installTestCommands{}
	before := installTestSnapshot(t, installTestHome(config))
	_, err := newTestFacade(config, commands, time.Now).Preview(Install)
	if err == nil || !containsInstallTestText(err.Error(), "invalid JSON") {
		t.Fatalf("Preview error = %v, want invalid JSON", err)
	}
	if after := installTestSnapshot(t, installTestHome(config)); after != before {
		t.Fatalf("rejected preview mutated sandbox:\n%s", after)
	}
	if len(commands.lookups) != 0 || len(commands.runs) != 0 {
		t.Fatalf("rejected preview used commands: lookups %#v runs %#v", commands.lookups, commands.runs)
	}
}

func TestInstallPreviewRejectsMalformedSkillSourceBeforeMutation(t *testing.T) {
	config := installTestConfig(t)
	if err := os.RemoveAll(filepath.Join(config.SkillSource.Root, "in-progress", "loop-me")); err != nil {
		t.Fatal(err)
	}
	before := installTestSnapshot(t, installTestHome(config))
	commands := &installTestCommands{}
	_, err := newTestFacade(config, commands, time.Now).Preview(Install)
	if err == nil || !containsInstallTestText(err.Error(), "loop-me") {
		t.Fatalf("Preview error = %v, want missing loop-me", err)
	}
	if after := installTestSnapshot(t, installTestHome(config)); after != before {
		t.Fatalf("rejected source mutated sandbox:\n%s", after)
	}
	if len(commands.lookups) != 0 || len(commands.runs) != 0 {
		t.Fatalf("rejected source used commands: %#v %#v", commands.lookups, commands.runs)
	}
}

func TestInstallApplyPreservesMixedUnmanagedSkillPaths(t *testing.T) {
	config := installTestConfig(t)
	if err := os.MkdirAll(config.Skills.Root(), 0o700); err != nil {
		t.Fatal(err)
	}
	unmanagedFile := filepath.Join(config.Skills.Root(), "ask-matt")
	if err := os.WriteFile(unmanagedFile, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	unmanagedTarget := filepath.Join(installTestHome(config), "elsewhere")
	if err := os.Mkdir(unmanagedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	unmanagedLink := filepath.Join(config.Skills.Root(), "handoff")
	if err := os.Symlink(unmanagedTarget, unmanagedLink); err != nil {
		t.Fatal(err)
	}
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(unmanagedFile); err != nil || string(data) != "keep me" {
		t.Fatalf("unmanaged file changed: %q, %v", data, err)
	}
	if target, err := os.Readlink(unmanagedLink); err != nil || target != unmanagedTarget {
		t.Fatalf("unmanaged symlink changed: %q, %v", target, err)
	}
	state, _, err := LoadState(config.State.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range state.ManagedSkills {
		if skill.Name == "ask-matt" || skill.Name == "handoff" {
			t.Fatalf("unmanaged collision recorded as owned: %#v", state.ManagedSkills)
		}
	}
}

func TestInstallApplyPreservesUnmanagedPathsAndReturnsStructuredWarning(t *testing.T) {
	config := installTestConfig(t)
	if err := os.MkdirAll(config.Skills.Root(), 0o700); err != nil {
		t.Fatal(err)
	}
	targetRoot := filepath.Join(installTestHome(config), "unmanaged")
	if err := os.MkdirAll(targetRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ask-matt", "codebase-design", "grilling", "handoff", "loop-me", "wayfinder"} {
		if err := os.Symlink(filepath.Join(targetRoot, name), filepath.Join(config.Skills.Root(), name)); err != nil {
			t.Fatal(err)
		}
	}
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.ManagedSkillCount() != 0 || len(result.Warnings()) != 1 || !containsInstallTestText(result.Warnings()[0], "skipped 6 unmanaged skill symlinks") {
		t.Fatalf("result = count %d warnings %#v", result.ManagedSkillCount(), result.Warnings())
	}
	for _, action := range plan.Actions() {
		if action.Kind == ActionSkip {
			if target, err := os.Readlink(action.Path); err != nil || target != action.Target {
				t.Fatalf("unmanaged path changed: %s -> %q, %v", action.Path, target, err)
			}
		}
	}
}

func TestInstallExternalFailurePublishesRecoveryAndSafeRetryConfirmsIt(t *testing.T) {
	config := installTestConfig(t)
	engram := config.Engram.ExpectedPath()
	writeInstallTestExecutable(t, engram)
	failedCall := engram + " setup codex"
	commands := &installTestCommands{fail: map[string]error{failedCall: errors.New("interrupted")}}
	facade := newTestFacade(config, commands, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err == nil || !containsInstallTestText(err.Error(), "failed to configure Engram for codex") {
		t.Fatalf("Apply error = %v", err)
	}
	state, found, err := LoadState(config.State.StateFile())
	if err != nil || !found || !state.RecoveryRequired() || len(state.ManagedSkills) != 6 {
		t.Fatalf("recovery state = %#v found %v err %v", state, found, err)
	}
	delete(commands.fail, failedCall)
	retry, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), retry); err != nil {
		t.Fatalf("safe retry failed: %v", err)
	}
	state, _, err = LoadState(config.State.StateFile())
	if err != nil || state.RecoveryRequired() || state.InstallStatus != InstallConfirmed || len(state.ManagedSkills) != 6 {
		t.Fatalf("retried state = %#v err %v", state, err)
	}
}

func TestInstallSafeRetryDoesNotAdoptANewConflict(t *testing.T) {
	config := installTestConfig(t)
	engram := config.Engram.ExpectedPath()
	writeInstallTestExecutable(t, engram)
	failedCall := engram + " setup codex"
	commands := &installTestCommands{fail: map[string]error{failedCall: errors.New("interrupted")}}
	facade := newTestFacade(config, commands, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err == nil {
		t.Fatal("Apply unexpectedly succeeded")
	}
	conflict := filepath.Join(config.Skills.Root(), "ask-matt")
	if err := os.Remove(conflict); err != nil {
		t.Fatal(err)
	}
	unmanaged := filepath.Join(installTestHome(config), "unmanaged")
	if err := os.Mkdir(unmanaged, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unmanaged, conflict); err != nil {
		t.Fatal(err)
	}
	delete(commands.fail, failedCall)
	retry, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	state, _, err := LoadState(config.State.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range state.ManagedSkills {
		if skill.LinkPath == conflict {
			t.Fatalf("retry adopted conflict: %#v", skill)
		}
	}
	if target, err := os.Readlink(conflict); err != nil || target != unmanaged {
		t.Fatalf("conflict changed: %q, %v", target, err)
	}
}

func TestInstallEngramFailuresAreActionableAndRecoverable(t *testing.T) {
	t.Run("brew acquisition failure", func(t *testing.T) {
		config := installTestConfig(t)
		call := "brew install gentleman-programming/tap/engram"
		commands := &installTestCommands{fail: map[string]error{call: errors.New("brew missing")}}
		facade := newTestFacade(config, commands, time.Now)
		plan, err := facade.Preview(Install)
		if err != nil {
			t.Fatal(err)
		}
		_, err = facade.Apply(context.Background(), plan)
		for _, want := range []string{"failed to install Engram via Homebrew", "ensure Homebrew is installed", "brew missing"} {
			if err == nil || !containsInstallTestText(err.Error(), want) {
				t.Fatalf("Apply error = %v, want %q", err, want)
			}
		}
	})

	t.Run("brew succeeds without canonical setup binary", func(t *testing.T) {
		config := installTestConfig(t)
		facade := newTestFacade(config, &installTestCommands{}, time.Now)
		plan, err := facade.Preview(Install)
		if err != nil {
			t.Fatal(err)
		}
		_, err = facade.Apply(context.Background(), plan)
		for _, want := range []string{"canonical Homebrew Engram was not found", config.Engram.ExpectedPath(), "HOMEBREW_PREFIX"} {
			if err == nil || !containsInstallTestText(err.Error(), want) {
				t.Fatalf("Apply error = %v, want %q", err, want)
			}
		}
	})
}

func TestInstallPersistenceFailuresPreserveTruthfulRecovery(t *testing.T) {
	t.Run("state preparation leaves no local writes", func(t *testing.T) {
		config := installTestConfig(t)
		plan, err := newTestFacade(config, &installTestCommands{}, time.Now).Preview(Install)
		if err != nil {
			t.Fatal(err)
		}
		facade := plan.owner
		before := installTestSnapshot(t, installTestHome(config))
		original := saveInstallState
		saveInstallState = func(string, State) error { return errors.New("preparation interrupted") }
		t.Cleanup(func() { saveInstallState = original })
		if _, err := facade.Apply(context.Background(), plan); err == nil {
			t.Fatal("Apply unexpectedly succeeded")
		}
		if got := installTestSnapshot(t, installTestHome(config)); got != before {
			t.Fatalf("preparation failure left writes:\nbefore:\n%s\nafter:\n%s", before, got)
		}
	})

	t.Run("symlink ownership publication rolls back the unrecorded link", func(t *testing.T) {
		config := installTestConfig(t)
		writeInstallTestExecutable(t, config.Engram.ExpectedPath())
		facade := newTestFacade(config, &installTestCommands{}, time.Now)
		plan, err := facade.Preview(Install)
		if err != nil {
			t.Fatal(err)
		}
		original := saveInstallState
		saveInstallState = func(path string, state State) error {
			if state.RecoveryRequired() && len(state.ManagedSkills) == 1 {
				return errors.New("ownership persistence interrupted")
			}
			return SaveState(path, state)
		}
		t.Cleanup(func() { saveInstallState = original })
		if _, err := facade.Apply(context.Background(), plan); err == nil {
			t.Fatal("Apply unexpectedly succeeded")
		}
		if _, err := os.Lstat(filepath.Join(config.Skills.Root(), "ask-matt")); !os.IsNotExist(err) {
			t.Fatalf("unrecorded symlink remains: %v", err)
		}
		state, found, err := LoadState(config.State.StateFile())
		if err != nil || !found || !state.RecoveryRequired() || len(state.ManagedSkills) != 0 {
			t.Fatalf("state = %#v found %v err %v", state, found, err)
		}
	})

	t.Run("final publication leaves recovery state", func(t *testing.T) {
		config := installTestConfig(t)
		writeInstallTestExecutable(t, config.Engram.ExpectedPath())
		facade := newTestFacade(config, &installTestCommands{}, time.Now)
		plan, err := facade.Preview(Install)
		if err != nil {
			t.Fatal(err)
		}
		original := saveInstallState
		saveInstallState = func(path string, state State) error {
			if state.InstallStatus == InstallConfirmed {
				return errors.New("final publication interrupted")
			}
			return SaveState(path, state)
		}
		t.Cleanup(func() { saveInstallState = original })
		if _, err := facade.Apply(context.Background(), plan); err == nil {
			t.Fatal("Apply unexpectedly succeeded")
		}
		state, found, err := LoadState(config.State.StateFile())
		if err != nil || !found || !state.RecoveryRequired() || len(state.ManagedSkills) != 6 {
			t.Fatalf("state = %#v found %v err %v", state, found, err)
		}
	})
}

func TestInstallPartialContainerCreationKeepsRecoveryEvidence(t *testing.T) {
	config := installTestConfig(t)
	conflict := filepath.Dir(config.Codex.PromptFile())
	if err := os.WriteFile(conflict, []byte("contributor"), 0o600); err != nil {
		t.Fatal(err)
	}
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err == nil {
		t.Fatal("Apply unexpectedly succeeded")
	}
	state, found, err := LoadState(config.State.StateFile())
	if err != nil || !found || !state.RecoveryRequired() {
		t.Fatalf("state = %#v found %v err %v", state, found, err)
	}
	data, err := os.ReadFile(conflict)
	if err != nil || string(data) != "contributor" {
		t.Fatalf("container conflict changed: %q, %v", data, err)
	}
}

type installTestCommands struct {
	lookups []string
	runs    []string
	paths   map[string]string
	fail    map[string]error
	after   map[string]func()
}

func (commands *installTestCommands) LookPath(name string) (string, error) {
	commands.lookups = append(commands.lookups, name)
	if path := commands.paths[name]; path != "" {
		return path, nil
	}
	return "", os.ErrNotExist
}

func (commands *installTestCommands) Run(_ context.Context, name string, args ...string) error {
	call := name + " " + joinInstallTestArgs(args)
	commands.runs = append(commands.runs, call)
	if after := commands.after[call]; after != nil {
		after()
	}
	if err := commands.fail[call]; err != nil {
		return err
	}
	return nil
}

func installTestConfig(t *testing.T) facadeConfig {
	t.Helper()
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "bundle", "skills")
	for _, rel := range []string{"engineering/ask-matt", "engineering/codebase-design", "productivity/grilling", "productivity/handoff", "in-progress/loop-me", "engineering/wayfinder"} {
		dir := filepath.Join(source, rel)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+filepath.Base(dir)+"\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configHome := filepath.Join(home, "xdg")
	homebrewPrefix := filepath.Join(home, "homebrew")
	return NewFacade(FacadeConfig{
		PackyHome:       filepath.Join(home, ".packy"),
		Skills:          skillbundle.NewGlobalLayout(home),
		SkillSource:     skillbundle.Source{Root: source},
		Codex:           codex.NewCanonicalLayout(home),
		OpenCode:        opencode.NewCanonicalLayout(configHome),
		Engram:          engrambin.NewTopology(homebrewPrefix),
		InstalledSource: bootstrap.InstalledSourceAt(filepath.Join(home, ".local", "share", "packy")),
	}, &installTestCommands{}, time.Now).config
}

func newTestFacade(config facadeConfig, commands Commands, now func() time.Time) *Facade {
	if now == nil {
		now = time.Now
	}
	return &Facade{config: config, commands: commands, now: now}
}

func installTestHome(config facadeConfig) string { return filepath.Dir(config.State.PackyHome()) }

func installTestSnapshot(t *testing.T, root string) string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, rel+" "+info.Mode().String())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return joinInstallTestArgs(paths)
}

func joinInstallTestArgs(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += " "
		}
		result += value
	}
	return result
}

func containsInstallTestText(value, want string) bool { return strings.Contains(value, want) }

func installTestHasCommand(plan Plan, command, args string) bool {
	for _, action := range plan.Actions() {
		if action.Command == command && joinInstallTestArgs(action.Args) == args {
			return true
		}
	}
	return false
}

func installTestHasContainer(state State, path string) bool {
	for _, record := range state.CreatedContainers {
		if record.Path == path {
			return true
		}
	}
	return false
}

func writeInstallTestExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}
