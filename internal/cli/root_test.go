package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type fakeRunner struct {
	calls []fakeCall
}

type fakeCall struct {
	name string
	args []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, fakeCall{name: name, args: append([]string(nil), args...)})
	return nil
}

func executeCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func sandboxOptions(t *testing.T) (Options, *fakeRunner, string) {
	t.Helper()
	home := t.TempDir()
	runner := &fakeRunner{}
	return Options{
		Env: MapEnv{
			"HOME":            home,
			"XDG_CONFIG_HOME": filepath.Join(home, "xdg-config"),
		},
		Runner: runner,
	}, runner, home
}

func TestHelpRendersForRootAndV0Subcommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root", args: []string{"--help"}, want: []string{"Install and configure", "install", "doctor", "update", "uninstall"}},
		{name: "install", args: []string{"install", "--help"}, want: []string{"Install Matty-managed", "--dry-run"}},
		{name: "doctor", args: []string{"doctor", "--help"}, want: []string{"Check Matty setup"}},
		{name: "update", args: []string{"update", "--help"}, want: []string{"Refresh Matty-managed"}},
		{name: "uninstall", args: []string{"uninstall", "--help"}, want: []string{"Remove only Matty-managed"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, _ := sandboxOptions(t)
			out, err := executeCommand(t, NewRootCommand(opts), tt.args...)
			if err != nil {
				t.Fatalf("help command failed: %v\n%s", err, out)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("help output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestCommandsResolvePathsFromInjectedEnvironment(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "custom-xdg")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": xdg}, Runner: &fakeRunner{}}

	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}

	wants := []string{
		"HOME=" + home,
		"CONFIG_HOME=" + xdg,
		"MATTY_STATE=" + filepath.Join(home, ".matty", "config.json"),
		"AGENT_SKILLS=" + filepath.Join(home, ".agents", "skills"),
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestCommandsAcceptFakeRunnerWithoutExecutingExternalCommands(t *testing.T) {
	tests := [][]string{
		{"install", "--dry-run"},
		{"install"},
		{"doctor"},
		{"update"},
		{"uninstall"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			opts, runner, _ := sandboxOptions(t)
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("expected scaffold command not to execute external tools, got %#v", runner.calls)
			}
		})
	}
}

func TestReadOnlyOrScaffoldCommandsDoNotCreateFilesInSandboxHome(t *testing.T) {
	tests := [][]string{
		{"install", "--dry-run"},
		{"doctor"},
		{"update"},
		{"uninstall"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			opts, _, home := sandboxOptions(t)
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			for _, path := range []string{filepath.Join(home, ".matty"), filepath.Join(home, ".agents")} {
				if t.Failed() {
					return
				}
				if exists(path) {
					t.Fatalf("command %q unexpectedly created %s", strings.Join(args, " "), path)
				}
			}
		})
	}
}

func TestResolvePathsRejectsMissingHome(t *testing.T) {
	_, err := ResolvePaths(MapEnv{})
	if err == nil {
		t.Fatal("expected missing HOME error")
	}
}

func TestResolvePathsFallsBackForRelativeXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	paths, err := ResolvePaths(MapEnv{"HOME": home, "XDG_CONFIG_HOME": "relative"})
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	want := filepath.Join(home, ".config")
	if paths.ConfigHome != want {
		t.Fatalf("ConfigHome = %q, want %q", paths.ConfigHome, want)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestInstallDryRunReportsPlanAndDoesNotMutateSandbox(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	cmd := NewRootCommand(opts)

	out, err := executeCommand(t, cmd, "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}

	outAgain, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("second install --dry-run failed: %v\n%s", err, outAgain)
	}
	if outAgain != out {
		t.Fatalf("dry-run output changed between runs:\nfirst:\n%s\nsecond:\n%s", out, outAgain)
	}

	wants := []string{
		"matty install dry-run: planned actions",
		"write-file: persist Matty state metadata",
		filepath.Join(home, ".matty", "config.json"),
		"symlink: link managed skill ask-matt",
		"run: install or verify Engram (brew install engram)",
		"run: delegate Codex Engram setup (engram setup codex)",
		"run: delegate OpenCode Engram setup (engram setup opencode)",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("dry-run executed external commands: %#v", runner.calls)
	}
	for _, path := range []string{filepath.Join(home, ".matty"), filepath.Join(home, ".agents")} {
		if exists(path) {
			t.Fatalf("dry-run unexpectedly created %s", path)
		}
	}
}

func TestInstallWritesSmallStateWithoutRunningExternalCommands(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("install issue 02 should not execute external commands yet, got %#v", runner.calls)
	}

	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	state, found, err := LoadState(paths.StateFile)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if !found {
		t.Fatal("expected state file to be written")
	}
	if state.SchemaVersion != stateSchemaVersion || state.MattyVersion != version {
		t.Fatalf("unexpected state metadata: %#v", state)
	}
	if got, want := state.ConfiguredSurfaces, []string{"codex", "opencode"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ConfiguredSurfaces = %#v, want %#v", got, want)
	}
	if state.Paths.StateFile != filepath.Join(home, ".matty", "config.json") {
		t.Fatalf("state path = %q", state.Paths.StateFile)
	}
	if !hasManagedSkill(state, "ask-matt") || !hasManagedSkill(state, "wayfinder") {
		t.Fatalf("state missing expected managed skills: %#v", state.ManagedSkills)
	}
	data, err := os.ReadFile(paths.StateFile)
	if err != nil {
		t.Fatalf("read written state: %v", err)
	}
	if len(data) > 10000 {
		t.Fatalf("state file is too large for metadata-only state: %d bytes", len(data))
	}
	if strings.Contains(string(data), "You are") || strings.Contains(string(data), "## Instructions") {
		t.Fatalf("state appears to contain prompt content:\n%s", data)
	}
	if exists(filepath.Join(home, ".agents")) {
		t.Fatalf("issue 02 install should not create skill symlinks yet")
	}
}

func TestInstallRejectsCorruptState(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if err := os.MkdirAll(paths.MattyDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(paths.StateFile, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err == nil {
		t.Fatalf("expected corrupt state error, got output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("error = %v, want invalid JSON", err)
	}
}

func TestDoctorReportsStateStatusWithoutCreatingState(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MATTY_STATE_STATUS=missing") {
		t.Fatalf("doctor did not report missing state:\n%s", out)
	}
	if exists(filepath.Join(home, ".matty")) {
		t.Fatalf("doctor created state directory")
	}

	out, err = executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	out, err = executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor after install failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "MATTY_STATE_STATUS=present") {
		t.Fatalf("doctor did not report present state:\n%s", out)
	}
}

func hasManagedSkill(state State, name string) bool {
	for _, skill := range state.ManagedSkills {
		if skill.Name == name {
			return true
		}
	}
	return false
}
