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
)

func TestUpdatePreviewIsReadOnlyAndItsActionViewCannotMutateThePlan(t *testing.T) {
	config := installTestConfig(t)
	commands := &installTestCommands{}
	facade := NewFacade(config, commands, func() time.Time { return time.Unix(456, 0) })
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

func TestUpdateApplyConvergesAndIsIdempotent(t *testing.T) {
	config := installTestConfig(t)
	engram := filepath.Join(config.HomebrewPrefix, "bin", "engram")
	writeInstallTestExecutable(t, engram)
	commands := &installTestCommands{}
	facade := NewFacade(config, commands, func() time.Time { return time.Unix(456, 0) })

	plan, err := facade.Preview(Update)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.ManagedSkillCount() != 6 || result.StateFile() != config.StateFile {
		t.Fatalf("result = skills %d state %q", result.ManagedSkillCount(), result.StateFile())
	}
	wantCalls := []string{"brew update", "brew upgrade engram", engram + " setup codex", engram + " setup opencode"}
	if !reflect.DeepEqual(commands.runs, wantCalls) {
		t.Fatalf("commands = %#v, want %#v", commands.runs, wantCalls)
	}
	state, found, err := LoadState(config.StateFile)
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
			engram := filepath.Join(config.HomebrewPrefix, "bin", "engram")
			writeInstallTestExecutable(t, engram)
			failed := strings.Replace(tc.failed, "<engram>", engram, 1)
			commands := &installTestCommands{fail: map[string]error{failed: errors.New("interrupted")}}
			facade := NewFacade(config, commands, time.Now)
			plan, err := facade.Preview(Update)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := facade.Apply(context.Background(), plan); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Apply error = %v, want %q", err, tc.want)
			}
			state, found, err := LoadState(config.StateFile)
			if err != nil || !found || !state.RecoveryRequired() {
				t.Fatalf("recovery state = %#v found %v err %v", state, found, err)
			}
		})
	}
}

func TestUpdateRecoveryRetryPreservesConfirmedOwnership(t *testing.T) {
	config := installTestConfig(t)
	engram := filepath.Join(config.HomebrewPrefix, "bin", "engram")
	writeInstallTestExecutable(t, engram)
	failed := engram + " setup codex"
	commands := &installTestCommands{fail: map[string]error{failed: errors.New("interrupted")}}
	facade := NewFacade(config, commands, time.Now)
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
	state, _, err := LoadState(config.StateFile)
	if err != nil || state.RecoveryRequired() || len(state.ManagedSkills) != 6 {
		t.Fatalf("recovered state = %#v err %v", state, err)
	}
}

func TestUpdatePersistenceFailuresPreserveRecoveryGuarantees(t *testing.T) {
	t.Run("final confirmation remains recoverable", func(t *testing.T) {
		config := installTestConfig(t)
		writeInstallTestExecutable(t, filepath.Join(config.HomebrewPrefix, "bin", "engram"))
		original := saveUpdateState
		t.Cleanup(func() { saveUpdateState = original })
		saveUpdateState = func(path string, state State) error {
			if state.InstallStatus == InstallConfirmed {
				return errors.New("final commit interrupted")
			}
			return SaveState(path, state)
		}
		facade := NewFacade(config, &installTestCommands{}, time.Now)
		plan, _ := facade.Preview(Update)
		if _, err := facade.Apply(context.Background(), plan); err == nil {
			t.Fatal("Apply unexpectedly succeeded")
		}
		state, found, err := LoadState(config.StateFile)
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
		facade := NewFacade(config, &installTestCommands{}, time.Now)
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
		facade := NewFacade(config, &installTestCommands{}, time.Now)
		plan, _ := facade.Preview(Update)
		if _, err := facade.Apply(context.Background(), plan); err == nil {
			t.Fatal("Apply unexpectedly succeeded")
		}
		if _, err := os.Lstat(filepath.Join(config.AgentSkillsDir, "ask-matt")); !os.IsNotExist(err) {
			t.Fatal("unrecorded symlink was not rolled back")
		}
		state, found, err := LoadState(config.StateFile)
		if err != nil || !found || !state.RecoveryRequired() || len(state.ManagedSkills) != 0 {
			t.Fatalf("state = %#v found %v err %v", state, found, err)
		}
	})
}

func TestUpdateRequiresCanonicalHomebrewEngramForSetup(t *testing.T) {
	config := installTestConfig(t)
	facade := NewFacade(config, &installTestCommands{}, time.Now)
	plan, err := facade.Preview(Update)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "canonical Homebrew Engram was not found") {
		t.Fatalf("Apply error = %v", err)
	}
	if _, err := os.Stat(config.StateFile); err != nil {
		t.Fatalf("missing recovery state: %v", err)
	}
}

func TestUpdatePreviewEnforcesDefaultInstalledSourceAlignment(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prepare   func(*testing.T, *Config)
		wantError string
	}{
		{
			name: "aligned",
			prepare: func(t *testing.T, config *Config) {
				prepareUpdateSourceRepository(t, config, "v1.2.3", false)
			},
		},
		{
			name: "missing",
			prepare: func(t *testing.T, config *Config) {
				config.InstalledSourceRoot = filepath.Join(t.TempDir(), "missing")
				config.SkillSourceRoot = filepath.Join(config.InstalledSourceRoot, "bundle", "skills")
			},
			wantError: "missing or invalid",
		},
		{
			name: "malformed",
			prepare: func(t *testing.T, config *Config) {
				root := t.TempDir()
				config.InstalledSourceRoot = root
				config.SkillSourceRoot = filepath.Join(root, "bundle", "skills")
				if err := os.MkdirAll(config.SkillSourceRoot, 0o700); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "missing or invalid",
		},
		{
			name: "stale",
			prepare: func(t *testing.T, config *Config) {
				prepareUpdateSourceRepository(t, config, "v1.2.3", true)
			},
			wantError: "stale for Matty v1.2.3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sandboxUpdateTestGitPath(t)
			config := installTestConfig(t)
			config.SkillSourceIsDefault = true
			config.RunningVersion = "v1.2.3"
			config.HomeDir = installTestHome(config)
			tc.prepare(t, &config)
			before := installTestSnapshot(t, installTestHome(config))
			commands := &installTestCommands{}
			_, err := NewFacade(config, commands, time.Now).Preview(Update)
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
			config.SkillSourceIsDefault = false
			config.RunningVersion = "v1.2.3"
			config.InstalledSourceRoot = filepath.Join(t.TempDir(), "missing-default-source")
			if _, err := NewFacade(config, &installTestCommands{}, time.Now).Preview(Update); err != nil {
				t.Fatalf("Preview(Update) rejected %s: %v", source, err)
			}
		})
	}
}

func prepareUpdateSourceRepository(t *testing.T, config *Config, tag string, stale bool) {
	t.Helper()
	root := filepath.Dir(filepath.Dir(config.SkillSourceRoot))
	config.InstalledSourceRoot = root
	config.HomeDir = installTestHome(*config)
	runUpdateTestGit(t, root, "init", "-q")
	runUpdateTestGit(t, root, "add", ".")
	runUpdateTestGit(t, root, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-qm", "source")
	runUpdateTestGit(t, root, "tag", tag)
	if stale {
		if err := os.WriteFile(filepath.Join(root, "STALE"), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
		runUpdateTestGit(t, root, "add", ".")
		runUpdateTestGit(t, root, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-qm", "stale")
	}
}

func runUpdateTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitHome := t.TempDir()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = []string{"HOME=" + gitHome, "XDG_CONFIG_HOME=" + filepath.Join(gitHome, "xdg"), "PATH=" + os.Getenv("PATH")}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func sandboxUpdateTestGitPath(t *testing.T) {
	t.Helper()
	gitTarget, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gitBin := filepath.Join(t.TempDir(), "bin", "git")
	if err := os.MkdirAll(filepath.Dir(gitBin), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitTarget, gitBin); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(gitBin))
}
