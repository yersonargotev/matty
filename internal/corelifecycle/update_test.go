package corelifecycle

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/bootstrap"
)

func TestUpdatePreviewIsReadOnlyAndItsActionViewCannotMutateThePlan(t *testing.T) {
	config := installTestConfig(t)
	commands := &installTestCommands{}
	facade := newTestFacade(config, commands, func() time.Time { return time.Unix(456, 0) })
	before := installTestSnapshot(t, installTestHome(config))

	plan, err := facade.Preview(Update)
	if err != nil {
		t.Fatalf("Preview(Update) failed: %v", err)
	}
	wantPrefix := []string{"brew update", "brew upgrade engram"}
	for i, want := range wantPrefix {
		action := plan.Actions()[i]
		if got := strings.Join(append([]string{action.Command}, action.Args...), " "); got != want {
			t.Fatalf("action %d = %q, want %q", i, got, want)
		}
	}
	first := plan.Actions()
	want := plan.Actions()
	first[0].Args[0] = "caller mutation"
	if got := plan.Actions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("caller changed opaque update plan:\ngot  %#v\nwant %#v", got, want)
	}
	if after := installTestSnapshot(t, installTestHome(config)); after != before {
		t.Fatalf("Preview(Update) mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(commands.lookups) != 0 || len(commands.runs) != 0 {
		t.Fatalf("Preview(Update) used command seam: lookups %#v runs %#v", commands.lookups, commands.runs)
	}
}

func TestUpdatePreviewDoesNotExecuteGit(t *testing.T) {
	config := installTestConfig(t)
	config.SkillSource.IsDefault = true
	config.RunningVersion = "v1.2.3"
	prepareUpdateSourceRepository(t, &config, config.RunningVersion, false)
	runUpdateTestGit(t, config.InstalledSource.Root(), "pack-refs", "--all")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "git-called")
	gitShim := filepath.Join(t.TempDir(), "git")
	script := "#!/bin/sh\nprintf called > '" + marker + "'\nexec '" + realGit + "' \"$@\"\n"
	if err := os.WriteFile(gitShim, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(gitShim))

	if _, err := newTestFacade(config, &installTestCommands{}, time.Now).Preview(Update); err != nil {
		t.Fatalf("Preview(Update) failed: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("Preview(Update) executed git; marker error = %v", err)
	}
}

func TestUpdatePreviewAcceptsAlignedLinkedWorktree(t *testing.T) {
	config := installTestConfig(t)
	config.SkillSource.IsDefault = true
	config.RunningVersion = "v1.2.3"
	prepareUpdateSourceRepository(t, &config, config.RunningVersion, false)

	worktree := filepath.Join(t.TempDir(), "installed-source-worktree")
	runUpdateTestGit(t, config.InstalledSource.Root(), "worktree", "add", "--detach", worktree, config.RunningVersion)
	config.InstalledSource = bootstrap.InstalledSourceAt(worktree)
	config.SkillSource.Root = filepath.Join(worktree, "bundle", "skills")

	if _, err := newTestFacade(config, &installTestCommands{}, time.Now).Preview(Update); err != nil {
		t.Fatalf("Preview(Update) rejected aligned linked worktree: %v", err)
	}
}

func TestUpdatePreviewAcceptsAlignedAnnotatedTagWithPackedObject(t *testing.T) {
	config := installTestConfig(t)
	config.SkillSource.IsDefault = true
	config.RunningVersion = "v1.2.3"
	prepareUpdateSourceRepository(t, &config, "", false)
	runUpdateTestGit(t, config.InstalledSource.Root(), "config", "user.name", "Packy Test")
	runUpdateTestGit(t, config.InstalledSource.Root(), "config", "user.email", "packy@example.test")
	runUpdateTestGit(t, config.InstalledSource.Root(), "tag", "-a", config.RunningVersion, "-m", "release")
	runUpdateTestGit(t, config.InstalledSource.Root(), "gc")
	tagObject := runUpdateTestGitOutput(t, config.InstalledSource.Root(), "rev-parse", "refs/tags/"+config.RunningVersion)
	runUpdateTestGit(t, config.InstalledSource.Root(), "update-ref", "-d", "refs/tags/"+config.RunningVersion)
	runUpdateTestGit(t, config.InstalledSource.Root(), "update-ref", "refs/tags/"+config.RunningVersion, tagObject)

	if _, err := newTestFacade(config, &installTestCommands{}, time.Now).Preview(Update); err != nil {
		t.Fatalf("Preview(Update) rejected aligned annotated tag with packed object: %v", err)
	}
}

func TestUpdatePreviewRejectsCyclicHeadReference(t *testing.T) {
	config := installTestConfig(t)
	config.SkillSource.IsDefault = true
	config.RunningVersion = "v1.2.3"
	prepareUpdateSourceRepository(t, &config, config.RunningVersion, false)
	if err := os.WriteFile(filepath.Join(config.InstalledSource.Root(), ".git", "HEAD"), []byte("ref: HEAD\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := newTestFacade(config, &installTestCommands{}, time.Now).Preview(Update)
	if err == nil || !strings.Contains(err.Error(), "inspect Installed Source HEAD") {
		t.Fatalf("Preview(Update) error = %v, want actionable cyclic HEAD error", err)
	}
}

func TestUpdateApplyConvergesAndIsIdempotent(t *testing.T) {
	config := installTestConfig(t)
	engram := config.Engram.ExpectedPath()
	writeInstallTestExecutable(t, engram)
	commands := &installTestCommands{}
	facade := newTestFacade(config, commands, func() time.Time { return time.Unix(456, 0) })

	plan, err := facade.Preview(Update)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.ManagedSkillCount() != 6 || result.StateFile() != config.State.StateFile() {
		t.Fatalf("result = skills %d state %q", result.ManagedSkillCount(), result.StateFile())
	}
	wantCalls := []string{"brew update", "brew upgrade engram", engram + " setup codex", engram + " setup opencode"}
	if !reflect.DeepEqual(commands.runs, wantCalls) {
		t.Fatalf("commands = %#v, want %#v", commands.runs, wantCalls)
	}
	state, found, err := LoadState(config.State.StateFile())
	if err != nil || !found || state.RecoveryRequired() || state.LastInstallCheck != "1970-01-01T00:07:36Z" {
		t.Fatalf("state = %#v found %v err %v", state, found, err)
	}

	before := installTestSnapshot(t, installTestHome(config))
	retry, err := facade.Preview(Update)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	if after := installTestSnapshot(t, installTestHome(config)); after != before {
		t.Fatalf("idempotent update changed sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestUpdateFailuresLeaveRecoveryAndReturnActionableErrors(t *testing.T) {
	for _, tc := range []struct {
		name, failed, want string
	}{
		{"homebrew refresh", "brew update", "failed to update Engram via Homebrew"},
		{"homebrew upgrade", "brew upgrade engram", "failed to update Engram via Homebrew"},
		{"Engram setup", "<engram> setup codex", "failed to configure Engram for codex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := installTestConfig(t)
			engram := config.Engram.ExpectedPath()
			writeInstallTestExecutable(t, engram)
			failed := strings.Replace(tc.failed, "<engram>", engram, 1)
			commands := &installTestCommands{fail: map[string]error{failed: errors.New("interrupted")}}
			facade := newTestFacade(config, commands, time.Now)
			plan, err := facade.Preview(Update)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := facade.Apply(context.Background(), plan); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Apply error = %v, want %q", err, tc.want)
			}
			state, found, err := LoadState(config.State.StateFile())
			if err != nil || !found || !state.RecoveryRequired() {
				t.Fatalf("recovery state = %#v found %v err %v", state, found, err)
			}
		})
	}
}

func TestUpdateRecoveryRetryPreservesConfirmedOwnership(t *testing.T) {
	config := installTestConfig(t)
	engram := config.Engram.ExpectedPath()
	writeInstallTestExecutable(t, engram)
	failed := engram + " setup codex"
	commands := &installTestCommands{fail: map[string]error{failed: errors.New("interrupted")}}
	facade := newTestFacade(config, commands, time.Now)
	plan, _ := facade.Preview(Update)
	if _, err := facade.Apply(context.Background(), plan); err == nil {
		t.Fatal("Apply unexpectedly succeeded")
	}
	delete(commands.fail, failed)
	retry, err := facade.Preview(Update)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	state, _, err := LoadState(config.State.StateFile())
	if err != nil || state.RecoveryRequired() || len(state.ManagedSkills) != 6 {
		t.Fatalf("recovered state = %#v err %v", state, err)
	}
}

func TestUpdatePersistenceFailuresPreserveRecoveryGuarantees(t *testing.T) {
	t.Run("final confirmation remains recoverable", func(t *testing.T) {
		config := installTestConfig(t)
		writeInstallTestExecutable(t, config.Engram.ExpectedPath())
		original := saveUpdateState
		t.Cleanup(func() { saveUpdateState = original })
		saveUpdateState = func(path string, state State) error {
			if state.InstallStatus == InstallConfirmed {
				return errors.New("final commit interrupted")
			}
			return SaveState(path, state)
		}
		facade := newTestFacade(config, &installTestCommands{}, time.Now)
		plan, _ := facade.Preview(Update)
		if _, err := facade.Apply(context.Background(), plan); err == nil {
			t.Fatal("Apply unexpectedly succeeded")
		}
		state, found, err := LoadState(config.State.StateFile())
		if err != nil || !found || !state.RecoveryRequired() {
			t.Fatalf("state = %#v found %v err %v", state, found, err)
		}
	})

	t.Run("preparation failure leaves no local writes", func(t *testing.T) {
		config := installTestConfig(t)
		before := installTestSnapshot(t, installTestHome(config))
		original := saveUpdateState
		t.Cleanup(func() { saveUpdateState = original })
		saveUpdateState = func(string, State) error { return errors.New("preparation interrupted") }
		facade := newTestFacade(config, &installTestCommands{}, time.Now)
		plan, _ := facade.Preview(Update)
		if _, err := facade.Apply(context.Background(), plan); err == nil {
			t.Fatal("Apply unexpectedly succeeded")
		}
		if after := installTestSnapshot(t, installTestHome(config)); after != before {
			t.Fatalf("preparation failure left local writes:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})

	t.Run("ownership failure rolls back unrecorded symlink", func(t *testing.T) {
		config := installTestConfig(t)
		original := saveUpdateState
		t.Cleanup(func() { saveUpdateState = original })
		saveUpdateState = func(path string, state State) error {
			if state.RecoveryRequired() && len(state.ManagedSkills) == 1 {
				return errors.New("ownership persistence interrupted")
			}
			return SaveState(path, state)
		}
		facade := newTestFacade(config, &installTestCommands{}, time.Now)
		plan, _ := facade.Preview(Update)
		if _, err := facade.Apply(context.Background(), plan); err == nil {
			t.Fatal("Apply unexpectedly succeeded")
		}
		if _, err := os.Lstat(filepath.Join(config.Skills.Root(), "ask-matt")); !os.IsNotExist(err) {
			t.Fatal("unrecorded symlink was not rolled back")
		}
		state, found, err := LoadState(config.State.StateFile())
		if err != nil || !found || !state.RecoveryRequired() || len(state.ManagedSkills) != 0 {
			t.Fatalf("state = %#v found %v err %v", state, found, err)
		}
	})
}

func TestUpdateRequiresCanonicalHomebrewEngramForSetup(t *testing.T) {
	config := installTestConfig(t)
	facade := newTestFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Update)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "canonical Homebrew Engram was not found") {
		t.Fatalf("Apply error = %v", err)
	}
	if _, err := os.Stat(config.State.StateFile()); err != nil {
		t.Fatalf("missing recovery state: %v", err)
	}
}

func TestUpdatePreviewEnforcesDefaultInstalledSourceAlignment(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prepare   func(*testing.T, *facadeConfig)
		wantError string
	}{
		{
			name: "aligned",
			prepare: func(t *testing.T, config *facadeConfig) {
				prepareUpdateSourceRepository(t, config, "v1.2.3", false)
			},
		},
		{
			name: "missing",
			prepare: func(t *testing.T, config *facadeConfig) {
				config.InstalledSource = bootstrap.InstalledSourceAt(filepath.Join(t.TempDir(), "missing"))
				config.SkillSource.Root = filepath.Join(config.InstalledSource.Root(), "bundle", "skills")
			},
			wantError: "missing or invalid",
		},
		{
			name: "malformed",
			prepare: func(t *testing.T, config *facadeConfig) {
				root := t.TempDir()
				config.InstalledSource = bootstrap.InstalledSourceAt(root)
				config.SkillSource.Root = filepath.Join(root, "bundle", "skills")
				if err := os.MkdirAll(config.SkillSource.Root, 0o700); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "missing or invalid",
		},
		{
			name: "stale",
			prepare: func(t *testing.T, config *facadeConfig) {
				prepareUpdateSourceRepository(t, config, "v1.2.3", true)
			},
			wantError: "stale for Packy v1.2.3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := installTestConfig(t)
			config.SkillSource.IsDefault = true
			config.RunningVersion = "v1.2.3"
			tc.prepare(t, &config)
			before := installTestSnapshot(t, installTestHome(config))
			commands := &installTestCommands{}
			_, err := newTestFacade(config, commands, time.Now).Preview(Update)
			if tc.wantError == "" && err != nil {
				t.Fatalf("Preview(Update) failed: %v", err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("Preview(Update) error = %v, want %q", err, tc.wantError)
			}
			if after := installTestSnapshot(t, installTestHome(config)); after != before {
				t.Fatalf("Preview(Update) mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
			}
			if len(commands.lookups) != 0 || len(commands.runs) != 0 {
				t.Fatalf("Preview(Update) used command seam: %#v %#v", commands.lookups, commands.runs)
			}
		})
	}
}

func TestUpdatePreviewSkipsReleaseAlignmentForRepositoryAndExplicitSources(t *testing.T) {
	for _, source := range []string{"repository checkout", "explicit override"} {
		t.Run(source, func(t *testing.T) {
			config := installTestConfig(t)
			config.SkillSource.IsDefault = false
			config.RunningVersion = "v1.2.3"
			config.InstalledSource = bootstrap.InstalledSourceAt(filepath.Join(t.TempDir(), "missing-default-source"))
			if _, err := newTestFacade(config, &installTestCommands{}, time.Now).Preview(Update); err != nil {
				t.Fatalf("Preview(Update) rejected %s: %v", source, err)
			}
		})
	}
}

func prepareUpdateSourceRepository(t *testing.T, config *facadeConfig, tag string, stale bool) {
	t.Helper()
	root := filepath.Dir(filepath.Dir(config.SkillSource.Root))
	config.InstalledSource = bootstrap.InstalledSourceAt(root)
	runUpdateTestGit(t, root, "init", "-q")
	runUpdateTestGit(t, root, "add", ".")
	runUpdateTestGit(t, root, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-qm", "source")
	if tag != "" {
		runUpdateTestGit(t, root, "tag", tag)
	}
	if stale {
		if err := os.WriteFile(filepath.Join(root, "STALE"), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
		runUpdateTestGit(t, root, "add", ".")
		runUpdateTestGit(t, root, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-qm", "stale")
	}
}

func runUpdateTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	runUpdateTestGitOutput(t, dir, args...)
}

func runUpdateTestGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	gitHome := t.TempDir()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = []string{"HOME=" + gitHome, "XDG_CONFIG_HOME=" + filepath.Join(gitHome, "xdg"), "PATH=" + os.Getenv("PATH")}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
