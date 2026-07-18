package corelifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/opencode"
	"github.com/yersonargotev/packy/internal/ownedcontainer"
	"github.com/yersonargotev/packy/internal/prompt"
)

func TestUninstallPreviewIsReadOnlyOpaqueAndUsesNoCommands(t *testing.T) {
	config := installTestConfig(t)
	commands := &installTestCommands{}
	facade := newTestFacade(config, commands, time.Now)
	if _, err := prompt.WriteCodex(config.Codex.PromptFile()); err != nil {
		t.Fatal(err)
	}
	before := installTestSnapshot(t, installTestHome(config))

	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatalf("Preview(Uninstall) failed: %v", err)
	}
	views := plan.Actions()
	want := plan.Actions()
	if len(views) == 0 {
		t.Fatal("Preview(Uninstall) returned no marker-owned action")
	}
	views[0].Description = "caller mutation"
	if got := plan.Actions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("caller changed opaque uninstall plan:\ngot  %#v\nwant %#v", got, want)
	}
	if after := installTestSnapshot(t, installTestHome(config)); after != before {
		t.Fatalf("Preview(Uninstall) mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(commands.lookups) != 0 || len(commands.runs) != 0 {
		t.Fatalf("Preview(Uninstall) used commands: lookups %#v runs %#v", commands.lookups, commands.runs)
	}
}

func TestUninstallApplyRejectsPlanFromAnotherFacade(t *testing.T) {
	config := installTestConfig(t)
	commands := &installTestCommands{}
	owner := newTestFacade(config, commands, time.Now)
	plan, err := owner.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	other := newTestFacade(config, commands, time.Now)
	if _, err := other.Apply(context.Background(), plan); !errors.Is(err, ErrForeignPlan) {
		t.Fatalf("other facade Apply error = %v, want ErrForeignPlan", err)
	}
}

func TestUninstallMissingAndCorruptStateAreSafe(t *testing.T) {
	config := installTestConfig(t)
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Apply(context.Background(), plan)
	if err != nil || result.HasWork() {
		t.Fatalf("missing state result = %#v, %v", result, err)
	}
	if _, err := os.Stat(config.State.PackyHome()); !os.IsNotExist(err) {
		t.Fatalf("no-op uninstall created Packy directory: %v", err)
	}

	if err := os.MkdirAll(config.State.PackyHome(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.State.StateFile(), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := installTestSnapshot(t, installTestHome(config))
	if _, err := facade.Preview(Uninstall); err == nil {
		t.Fatal("Preview(Uninstall) accepted corrupt state")
	}
	if after := installTestSnapshot(t, installTestHome(config)); after != before {
		t.Fatalf("corrupt-state preview mutated sandbox:\n%s", after)
	}
}

func TestUninstallRemovesVerifiedArtifactsAndPreservesContributorBytes(t *testing.T) {
	config := installTestConfig(t)
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	install, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), install); err != nil {
		t.Fatal(err)
	}
	contributor := filepath.Join(config.Skills.Root(), "contributor.txt")
	if err := os.WriteFile(contributor, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Apply(context.Background(), plan)
	if err != nil || !result.HasWork() || result.StateFile() != config.State.StateFile() {
		t.Fatalf("Apply result = %#v, %v", result, err)
	}
	if _, err := os.Lstat(filepath.Join(config.Skills.Root(), "ask-matt")); !os.IsNotExist(err) {
		t.Fatalf("managed skill remains: %v", err)
	}
	if got, err := os.ReadFile(contributor); err != nil || string(got) != "keep" {
		t.Fatalf("contributor bytes = %q, %v", got, err)
	}
}

func TestUninstallWithoutStateRemovesOnlyMarkerOwnedPromptsAndThenHasNoWork(t *testing.T) {
	config := installTestConfig(t)
	if err := os.MkdirAll(filepath.Dir(config.Codex.PromptFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	original := "# contributor notes\n"
	if err := os.WriteFile(config.Codex.PromptFile(), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prompt.WriteCodex(config.Codex.PromptFile()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config.State.StateFile()); !os.IsNotExist(err) {
		t.Fatalf("fixture unexpectedly has state: %v", err)
	}
	unmanaged := filepath.Join(config.Skills.Root(), "ask-matt")
	if err := os.MkdirAll(filepath.Dir(unmanaged), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(config.SkillSource.Root, unmanaged); err != nil {
		t.Fatal(err)
	}

	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(config.Codex.PromptFile()); err != nil || string(got) != original {
		t.Fatalf("contributor prompt = %q, %v", got, err)
	}
	if _, err := os.Lstat(unmanaged); err != nil {
		t.Fatalf("uninstall inferred ownership without state: %v", err)
	}

	before := installTestSnapshot(t, installTestHome(config))
	repeated, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Apply(context.Background(), repeated)
	if err != nil || result.HasWork() {
		t.Fatalf("repeated result = %#v, %v", result, err)
	}
	if after := installTestSnapshot(t, installTestHome(config)); after != before {
		t.Fatalf("repeated uninstall mutated sandbox:\n%s", after)
	}
}

func TestUninstallRemovesOpenCodeProjectionAndPreservesContributorConfig(t *testing.T) {
	config := installTestConfig(t)
	if err := os.MkdirAll(filepath.Dir(config.OpenCode.ConfigFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	original := "{\n  // contributor setting\n  \"model\": \"keep-me\",\n  \"instructions\": [\"CONTRIBUTING.md\"]\n}\n"
	if err := os.WriteFile(config.OpenCode.ConfigFile(), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := opencode.Write(config.OpenCode.ConfigFile(), config.OpenCode.PromptFile()); err != nil {
		t.Fatal(err)
	}
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(config.OpenCode.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"// contributor setting", "\"model\": \"keep-me\"", "CONTRIBUTING.md"} {
		if !containsInstallTestText(string(got), want) {
			t.Fatalf("contributor config lost %q:\n%s", want, got)
		}
	}
	if containsInstallTestText(string(got), config.OpenCode.PromptFile()) {
		t.Fatalf("Packy instruction remains:\n%s", got)
	}
	if _, err := os.Stat(config.OpenCode.PromptFile()); !os.IsNotExist(err) {
		t.Fatalf("Packy prompt remains: %v", err)
	}
}

func TestUninstallPreservesUnmanagedAndRetargetedSkillLinks(t *testing.T) {
	config, facade := installedUninstallFixture(t)
	unmanagedTarget := filepath.Join(installTestHome(config), "unmanaged-target")
	if err := os.Mkdir(unmanagedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(config.Skills.Root(), "ask-matt")
	if err := os.Remove(changed); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unmanagedTarget, changed); err != nil {
		t.Fatal(err)
	}
	unmanagedFile := filepath.Join(config.Skills.Root(), "personal-note")
	if err := os.WriteFile(unmanagedFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(changed); err != nil {
		t.Fatalf("retargeted link was removed: %v", err)
	}
	if got, err := os.ReadFile(unmanagedFile); err != nil || string(got) != "keep" {
		t.Fatalf("unmanaged file = %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(config.Skills.Root(), "wayfinder")); !os.IsNotExist(err) {
		t.Fatalf("verified link remains: %v", err)
	}
}

func TestUninstallPreservesSkillRetargetedAfterPreview(t *testing.T) {
	config, facade := installedUninstallFixture(t)
	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(config.Skills.Root(), "ask-matt")
	if err := os.Remove(changed); err != nil {
		t.Fatal(err)
	}
	unmanagedTarget := filepath.Join(installTestHome(config), "unmanaged-target")
	if err := os.Mkdir(unmanagedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unmanagedTarget, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(changed); err != nil || target != unmanagedTarget {
		t.Fatalf("retargeted link = %q, %v", target, err)
	}
}

func TestUninstallRejectsContainerChangeAfterPreviewBeforeMutation(t *testing.T) {
	config, facade := installedUninstallFixture(t)
	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	concurrent := filepath.Join(filepath.Dir(config.Codex.PromptFile()), "concurrent.txt")
	if err := os.WriteFile(concurrent, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); !errors.Is(err, ownedcontainer.ErrStalePlan) {
		t.Fatalf("Apply error = %v, want ErrStalePlan", err)
	}
	if _, err := os.Stat(config.State.StateFile()); err != nil {
		t.Fatalf("stale plan removed state: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(config.Skills.Root(), "ask-matt")); err != nil {
		t.Fatalf("stale plan removed skill: %v", err)
	}
}

func TestUninstallRejectsArtifactChangeAfterPreviewBeforeMutation(t *testing.T) {
	config, facade := installedUninstallFixture(t)
	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(config.Codex.PromptFile(), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\nconcurrent contributor bytes\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); !errors.Is(err, ownedcontainer.ErrStalePlan) {
		t.Fatalf("Apply error = %v, want ErrStalePlan", err)
	}
	if _, err := os.Stat(config.State.StateFile()); err != nil {
		t.Fatalf("stale artifact plan removed state: %v", err)
	}
}

func TestUninstallCleansPristineContainersButPreservesPreexistingContainers(t *testing.T) {
	t.Run("pristine", func(t *testing.T) {
		config, facade := installedUninstallFixture(t)
		plan, err := facade.Preview(Uninstall)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := facade.Apply(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{config.State.PackyHome(), filepath.Dir(config.Skills.Root()), filepath.Dir(config.Codex.PromptFile()), filepath.Dir(config.OpenCode.ConfigFile())} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("pristine container remains %s: %v", path, err)
			}
		}
	})

	t.Run("preexisting", func(t *testing.T) {
		config := installTestConfig(t)
		for _, path := range []string{config.State.PackyHome(), filepath.Dir(config.Skills.Root()), filepath.Dir(config.Codex.PromptFile()), filepath.Dir(config.OpenCode.ConfigFile())} {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		contributor := filepath.Join(filepath.Dir(config.Codex.PromptFile()), "contributor.bin")
		want := []byte{0, 1, 2, '\n'}
		if err := os.WriteFile(contributor, want, 0o600); err != nil {
			t.Fatal(err)
		}
		facade := installUninstallFacade(t, config, &installTestCommands{})
		plan, err := facade.Preview(Uninstall)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := facade.Apply(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{config.State.PackyHome(), filepath.Dir(config.Skills.Root()), filepath.Dir(config.Codex.PromptFile()), filepath.Dir(config.OpenCode.ConfigFile())} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("preexisting container removed %s: %v", path, err)
			}
		}
		if got, err := os.ReadFile(contributor); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("contributor bytes = %v, %v", got, err)
		}
	})
}

func TestUninstallInterruptedInstallUsesOnlyRecordedAndVerifiedOwnership(t *testing.T) {
	config := installTestConfig(t)
	engram := config.Engram.ExpectedPath()
	writeInstallTestExecutable(t, engram)
	commands := &installTestCommands{fail: map[string]error{engram + " setup codex": errors.New("interrupted")}}
	facade := newTestFacade(config, commands, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err == nil {
		t.Fatal("install unexpectedly succeeded")
	}
	state, found, err := LoadState(config.State.StateFile())
	if err != nil || !found || !state.RecoveryRequired() {
		t.Fatalf("recovery state = %#v, %v, %v", state, found, err)
	}
	changed := filepath.Join(config.Skills.Root(), "ask-matt")
	if err := os.Remove(changed); err != nil {
		t.Fatal(err)
	}
	unmanagedTarget := filepath.Join(installTestHome(config), "unmanaged-target")
	if err := os.Mkdir(unmanagedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unmanagedTarget, changed); err != nil {
		t.Fatal(err)
	}
	uninstall, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), uninstall); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(changed); err != nil {
		t.Fatalf("recovery uninstall removed retargeted link: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(config.Skills.Root(), "wayfinder")); !os.IsNotExist(err) {
		t.Fatalf("recovery uninstall left verified link: %v", err)
	}
}

func TestUninstallRejectsForgedContainerProvenanceOutsideAllowlist(t *testing.T) {
	config := installTestConfig(t)
	if err := os.MkdirAll(config.State.PackyHome(), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(installTestHome(config), "unmanaged-empty")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	state := DesiredState(StateConfig{StateFile: config.State.StateFile(), AgentSkillsDir: config.Skills.Root()}, time.Now(), nil)
	state.CreatedContainers = []ownedcontainer.Record{{Path: outside, Kind: ownedcontainer.Directory}}
	if err := SaveState(config.State.StateFile(), state); err != nil {
		t.Fatal(err)
	}
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Uninstall)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("forged provenance removed outside path: %v", err)
	}
}

func installedUninstallFixture(t *testing.T) (facadeConfig, *Facade) {
	t.Helper()
	config := installTestConfig(t)
	return config, installUninstallFacade(t, config, &installTestCommands{})
}

func installUninstallFacade(t *testing.T, config facadeConfig, commands *installTestCommands) *Facade {
	t.Helper()
	writeInstallTestExecutable(t, config.Engram.ExpectedPath())
	facade := newTestFacade(config, commands, time.Now)
	plan, err := facade.Preview(Install)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	return facade
}
