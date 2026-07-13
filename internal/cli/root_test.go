package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/ownedcontainer"
	"github.com/yersonargotev/matty/internal/skillbundle"
	mattyversion "github.com/yersonargotev/matty/internal/version"
)

func TestDoctorJSONHealthyWarningsAndFailures(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		opts, _, _ := sandboxOptions(t)
		opts.DoctorReportBuilder = func(Paths, Runner) DoctorReport {
			return DoctorReport{SchemaVersion: 1, Report: "doctor", Checks: []doctorCheck{{status: doctorPass, name: "fixture", detail: "healthy"}}, Summary: DoctorSummary{Status: "healthy", Passes: 1}}
		}
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, out)
		}
		var doc struct {
			SchemaVersion int    `json:"schema_version"`
			Report        string `json:"report"`
			Checks        []struct{ Name, Severity, Detail string }
			Summary       DoctorSummary `json:"summary"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, out)
		}
		if doc.SchemaVersion != 1 || doc.Report != "doctor" || doc.Summary.Status != "healthy" || len(doc.Checks) == 0 {
			t.Fatalf("unexpected report: %#v", doc)
		}
		if strings.Contains(out, "HOME=") || strings.Contains(out, "PASS ") {
			t.Fatalf("human output mixed into JSON: %s", out)
		}
	})
	t.Run("warnings", func(t *testing.T) {
		opts, _, _ := sandboxOptions(t)
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, out)
		}
		var doc struct {
			Summary DoctorSummary `json:"summary"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Summary.Status != "warnings" || doc.Summary.Warnings == 0 {
			t.Fatalf("warning report: %#v err=%v", doc, err)
		}
	})
	t.Run("failures emit full report before error", func(t *testing.T) {
		opts, _, _ := sandboxOptions(t)
		opts.Runner = &fakeRunner{}
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if !errors.Is(err, ErrDoctorUnhealthy) {
			t.Fatalf("error=%v", err)
		}
		var doc struct {
			Checks  []struct{ Name, Severity string }
			Summary DoctorSummary `json:"summary"`
		}
		if json.Unmarshal([]byte(out), &doc) != nil || doc.Summary.Failures == 0 || len(doc.Checks) < 2 {
			t.Fatalf("incomplete report: %s", out)
		}
	})
}

type fakeRunner struct {
	calls []fakeCall
	path  map[string]string
	fail  map[string]error
	after map[string]func()
}

type fakeCall struct {
	name string
	args []string
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if f.path != nil {
		if path, ok := f.path[name]; ok {
			return path, nil
		}
	}
	return "", os.ErrNotExist
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, fakeCall{name: name, args: append([]string(nil), args...)})
	key := strings.Join(append([]string{name}, args...), " ")
	if f.fail != nil {
		if err, ok := f.fail[key]; ok {
			return err
		}
	}
	if f.after != nil {
		if after, ok := f.after[key]; ok {
			after()
		}
	}
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
	sourceRoot := createSkillSource(t)
	homebrewPrefix := filepath.Join(t.TempDir(), "homebrew")
	homebrewBin := filepath.Join(homebrewPrefix, "bin")
	engram := writeEngramExecutable(t, homebrewBin, "engram version 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": engram}}
	return Options{
		Env: MapEnv{
			"HOME":                home,
			"XDG_CONFIG_HOME":     filepath.Join(home, "xdg-config"),
			"XDG_CACHE_HOME":      filepath.Join(home, "xdg-cache"),
			"CODEX_HOME":          filepath.Join(home, ".codex"),
			"PATH":                homebrewBin,
			"HOMEBREW_PREFIX":     homebrewPrefix,
			"MATTY_SKILLS_SOURCE": sourceRoot,
		},
		Runner: runner,
		EngramFacts: engrambin.Facts{
			Version:        func(string) (string, error) { return "1.19.0", nil },
			ServeProcesses: func() ([]engrambin.Process, error) { return nil, nil },
		},
	}, runner, home
}

func expandHomebrewEngramCalls(t *testing.T, opts Options, calls []string) []string {
	t.Helper()
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	engram := engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv)
	expanded := make([]string, 0, len(calls))
	for _, call := range calls {
		expanded = append(expanded, strings.ReplaceAll(call, "<homebrew-engram>", engram))
	}
	return expanded
}

func engramSetupCallStrings(t *testing.T, opts Options) []string {
	t.Helper()
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	engram := engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv)
	return []string{engram + " setup codex", engram + " setup opencode"}
}

func engramUpdateCallStrings(t *testing.T, opts Options) []string {
	t.Helper()
	return append([]string{"brew update", "brew upgrade engram"}, engramSetupCallStrings(t, opts)...)
}

func createSkillSource(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	createSkillSourceAt(t, root)
	return root
}

func createSkillSourceAt(t *testing.T, root string) {
	t.Helper()
	for _, rel := range testSkillSourceRels() {
		dir := filepath.Join(root, rel)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir skill source: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+filepath.Base(dir)+"\n---\n"), 0o600); err != nil {
			t.Fatalf("write skill source: %v", err)
		}
	}
}

func testSkillSourceRels() []string {
	return []string{
		"engineering/ask-matt",
		"engineering/codebase-design",
		"productivity/grilling",
		"productivity/handoff",
		"in-progress/loop-me",
		"engineering/wayfinder",
	}
}

func testSkillNames() []string {
	names := make([]string, 0, len(testSkillSourceRels()))
	for _, rel := range testSkillSourceRels() {
		names = append(names, filepath.Base(rel))
	}
	return names
}

func createUnmanagedSkillSymlinks(t *testing.T, paths Paths, targetRoot string) {
	t.Helper()
	if err := os.MkdirAll(paths.AgentSkillsDir, 0o700); err != nil {
		t.Fatalf("mkdir agent skills: %v", err)
	}
	if err := os.MkdirAll(targetRoot, 0o700); err != nil {
		t.Fatalf("mkdir unmanaged target root: %v", err)
	}
	for _, name := range testSkillNames() {
		target := filepath.Join(targetRoot, name)
		if err := os.Symlink(target, filepath.Join(paths.AgentSkillsDir, name)); err != nil {
			t.Fatalf("write unmanaged symlink %s: %v", name, err)
		}
	}
}

func installedSkillSourceRoot(home string) string {
	return skillbundle.SourceRoot(DefaultInstalledSourceRoot(home))
}

func createRepoCheckoutSkillSource(t *testing.T) (string, string) {
	t.Helper()
	repoRoot := t.TempDir()
	skillSource := skillbundle.SourceRoot(repoRoot)
	createSkillSourceAt(t, skillSource)
	return repoRoot, skillSource
}

func withVersion(t *testing.T, value string) {
	t.Helper()
	previous := mattyversion.Value
	mattyversion.Value = value
	t.Cleanup(func() {
		mattyversion.Value = previous
	})
}

func TestHelpRendersForRootAndV0Subcommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root", args: []string{"--help"}, want: []string{"Install and configure", "init", "install", "doctor", "update", "uninstall"}},
		{name: "install", args: []string{"install", "--help"}, want: []string{"Install Matty-managed", "--dry-run"}},
		{name: "doctor", args: []string{"doctor", "--help"}, want: []string{"Check Matty setup"}},
		{name: "update", args: []string{"update", "--help"}, want: []string{"Refresh Matty-managed", "--dry-run"}},
		{name: "uninstall", args: []string{"uninstall", "--help"}, want: []string{"Remove only Matty-managed", "--dry-run"}},
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

func TestVersionOutput(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{name: "default dev", version: "dev"},
		{name: "injected release", version: "v0.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withVersion(t, tt.version)
			opts, _, _ := sandboxOptions(t)

			out, err := executeCommand(t, NewRootCommand(opts), "--version")
			if err != nil {
				t.Fatalf("version command failed: %v\n%s", err, out)
			}
			if !strings.Contains(out, tt.version) {
				t.Fatalf("version output missing %q:\n%s", tt.version, out)
			}
		})
	}
}

func TestCommandsResolvePathsFromInjectedEnvironment(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, "custom-xdg")
	opts := Options{
		Env:    MapEnv{"HOME": home, "XDG_CONFIG_HOME": xdg},
		Runner: &fakeRunner{path: map[string]string{"engram": "/fake/bin/engram"}},
		EngramFacts: engrambin.Facts{
			Version:        func(string) (string, error) { return "1.19.0", nil },
			ServeProcesses: func() ([]engrambin.Process, error) { return nil, nil },
		},
	}

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

func TestCommandsUseFakeRunnerForExternalCommands(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantCalls []string
	}{
		{name: "install dry-run", args: []string{"install", "--dry-run"}},
		{name: "install", args: []string{"install"}, wantCalls: []string{"<homebrew-engram> setup codex", "<homebrew-engram> setup opencode"}},
		{name: "doctor", args: []string{"doctor"}},
		{name: "update", args: []string{"update"}, wantCalls: []string{"brew update", "brew upgrade engram", "<homebrew-engram> setup codex", "<homebrew-engram> setup opencode"}},
		{name: "uninstall", args: []string{"uninstall"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, runner, _ := sandboxOptions(t)
			out, err := executeCommand(t, NewRootCommand(opts), tt.args...)
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			wantCalls := expandHomebrewEngramCalls(t, opts, tt.wantCalls)
			if got := callStrings(runner.calls); strings.Join(got, "\n") != strings.Join(wantCalls, "\n") {
				t.Fatalf("runner calls = %#v, want %#v", got, wantCalls)
			}
		})
	}
}

func TestReadOnlyOrScaffoldCommandsDoNotCreateFilesInSandboxHome(t *testing.T) {
	tests := [][]string{
		{"install", "--dry-run"},
		{"doctor"},
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

func TestDoctorUsesInjectedEngramFactsWithoutMutationOrRunnerSideEffects(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	canonical := engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv)
	versionCalls := []string{}
	processCalls := 0
	opts.EngramFacts = engrambin.Facts{
		Version: func(path string) (string, error) {
			versionCalls = append(versionCalls, path)
			return "1.19.0", nil
		},
		ServeProcesses: func() ([]engrambin.Process, error) {
			processCalls++
			return []engrambin.Process{{PID: 42, ExecutablePath: canonical, Command: canonical + " serve"}}, nil
		},
	}
	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	assertDoctorManagedPathsAbsent(t, paths)
	if len(runner.calls) != 0 {
		t.Fatalf("doctor ran side-effect commands: %#v", runner.calls)
	}
	if len(versionCalls) != 1 || versionCalls[0] != canonical || processCalls != 1 {
		t.Fatalf("Engram facts calls: versions=%#v processes=%d", versionCalls, processCalls)
	}
	if !strings.Contains(out, "PASS engram-runtime: pid 42 running "+canonical) {
		t.Fatalf("doctor did not render injected runtime fact:\n%s", out)
	}
	if !strings.Contains(out, "WARN engram-setup: state is missing") {
		t.Fatalf("doctor setup intent was not reported independently:\n%s", out)
	}
}

func TestDoctorReportsInjectedEngramInspectionFailuresStably(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	opts.EngramFacts = engrambin.Facts{
		Version:        func(string) (string, error) { return "", errors.New("version unavailable") },
		ServeProcesses: func() ([]engrambin.Process, error) { return nil, errors.New("process inspection unavailable") },
	}
	out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	assertDoctorManagedPathsAbsent(t, paths)
	if len(runner.calls) != 0 {
		t.Fatalf("doctor ran side effects: calls=%#v", runner.calls)
	}
	for _, want := range []string{"version unavailable", "could not inspect active engram serve processes", "process inspection unavailable"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor JSON missing %q:\n%s", want, out)
		}
	}
}

func assertDoctorManagedPathsAbsent(t *testing.T, paths Paths) {
	t.Helper()
	for _, path := range []string{paths.StateFile, paths.AgentSkillsDir, paths.CodexPromptFile, paths.OpenCodeConfigFile, paths.OpenCodePromptFile} {
		if exists(path) {
			t.Fatalf("doctor unexpectedly created managed path %s", path)
		}
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

func TestResolvePathsDefaultsToMattyOwnedSkillBundle(t *testing.T) {
	home := t.TempDir()
	paths, err := ResolvePaths(MapEnv{"HOME": home})
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	wantSuffix := filepath.Join("bundle", "skills")
	if !strings.HasSuffix(paths.SkillSourceRoot, wantSuffix) {
		t.Fatalf("SkillSourceRoot = %q, want suffix %q", paths.SkillSourceRoot, wantSuffix)
	}
	if strings.Contains(paths.SkillSourceRoot, filepath.Join("skills", "skills")) {
		t.Fatalf("SkillSourceRoot should not default to external skills clone: %q", paths.SkillSourceRoot)
	}
}

func TestResolvePathsFallsBackToInstalledSourceOutsideRepo(t *testing.T) {
	home := t.TempDir()
	chdirTempOutsideRepo(t)

	paths, err := ResolvePaths(MapEnv{"HOME": home})
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	want := installedSkillSourceRoot(home)
	if paths.SkillSourceRoot != want {
		t.Fatalf("SkillSourceRoot = %q, want %q", paths.SkillSourceRoot, want)
	}
}

func TestInstallDryRunReportsRepoSourceAndInstalledSourceWarning(t *testing.T) {
	home := t.TempDir()
	repoRoot, _ := createRepoCheckoutSkillSource(t)
	createSkillSourceAt(t, installedSkillSourceRoot(home))

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}, Runner: &fakeRunner{path: map[string]string{"engram": "/fake/bin/engram"}}}
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Skill source: repo checkout (" + paths.SkillSourceRoot + ")",
		"warning: installed source also exists at " + installedSkillSourceRoot(home),
		"repo checkout source may create a development-mode install",
		"For package-installed setup, run matty install outside the repo or set MATTY_SKILLS_SOURCE explicitly.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("install --dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestInstallAndUpdateReportInstalledSourceOutsideRepo(t *testing.T) {
	home := t.TempDir()
	createSkillSourceAt(t, installedSkillSourceRoot(home))
	chdirTempOutsideRepo(t)

	homebrewPrefix := filepath.Join(t.TempDir(), "homebrew")
	engram := writeEngramExecutable(t, filepath.Join(homebrewPrefix, "bin"), "engram version 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": engram}}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config"), "PATH": filepath.Dir(engram), "HOMEBREW_PREFIX": homebrewPrefix}, Runner: runner}
	installedSkillSource := installedSkillSourceRoot(home)

	for _, args := range [][]string{{"install", "--dry-run"}, {"install"}, {"update", "--dry-run"}, {"update"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			want := "Skill source: installed source (" + installedSkillSource + ")"
			if !strings.Contains(out, want) {
				t.Fatalf("output missing %q:\n%s", want, out)
			}
			if strings.Contains(out, "development-mode install") || strings.Contains(out, "installed source also exists") {
				t.Fatalf("installed-source flow should not warn about repo source:\n%s", out)
			}
		})
	}
}

func TestInstallDryRunReportsExplicitOverrideSource(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	sourceRoot := opts.Env.Getenv("MATTY_SKILLS_SOURCE")
	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	want := "Skill source: explicit override (MATTY_SKILLS_SOURCE=" + sourceRoot + ")"
	if !strings.Contains(out, want) {
		t.Fatalf("install --dry-run output missing %q:\n%s", want, out)
	}
}

func TestPackageInstalledCommandsUseInitializedSourceOutsideRepo(t *testing.T) {
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	chdirTempOutsideRepo(t)

	homebrewPrefix := filepath.Join(t.TempDir(), "homebrew")
	engram := writeEngramExecutable(t, filepath.Join(homebrewPrefix, "bin"), "engram version 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": engram}}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config"), "PATH": filepath.Dir(engram), "HOMEBREW_PREFIX": homebrewPrefix}, Runner: runner}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed outside repo: %v\n%s", err, out)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed outside repo after init: %v\n%s", err, out)
	}
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if got, want := paths.SkillSourceRoot, filepath.Join(home, ".local", "share", "matty", "bundle", "skills"); got != want {
		t.Fatalf("SkillSourceRoot = %q, want installed source %q", got, want)
	}
	if !exists(paths.StateFile) || !exists(filepath.Join(paths.AgentSkillsDir, "wayfinder")) {
		t.Fatalf("install did not create Matty-managed artifacts from installed source")
	}

	beforeDoctor := snapshotTree(t, home)
	runner.calls = nil
	out, err = executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor failed outside repo after init: %v\n%s", err, out)
	}
	if afterDoctor := snapshotTree(t, home); afterDoctor != beforeDoctor {
		t.Fatalf("doctor mutated sandbox outside repo:\nbefore:\n%s\nafter:\n%s", beforeDoctor, afterDoctor)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("doctor ran external commands: %#v", runner.calls)
	}
	if !strings.Contains(out, "PASS skill-symlinks:") {
		t.Fatalf("doctor did not report installed-source skill links healthy:\n%s", out)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "update")
	if err != nil {
		t.Fatalf("update failed outside repo after init: %v\n%s", err, out)
	}
	if got, want := callStrings(runner.calls), engramUpdateCallStrings(t, opts); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("update runner calls = %#v, want %#v", got, want)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "uninstall")
	if err != nil {
		t.Fatalf("uninstall failed outside repo after init: %v\n%s", err, out)
	}
	if exists(paths.StateFile) || exists(filepath.Join(paths.AgentSkillsDir, "wayfinder")) {
		t.Fatalf("uninstall left Matty-managed artifacts in sandbox")
	}
	if !exists(paths.SkillSourceRoot) {
		t.Fatalf("uninstall should not remove Installed Source at %s", paths.SkillSourceRoot)
	}
}

func TestPackageInstalledInstallAndUpdateSuggestInitWhenSourceMissing(t *testing.T) {
	home := t.TempDir()
	chdirTempOutsideRepo(t)

	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}, Runner: &fakeRunner{path: map[string]string{"engram": "/fake/bin/engram"}}}
	missing := filepath.Join(home, ".local", "share", "matty", "bundle", "skills")
	for _, args := range [][]string{{"install", "--dry-run"}, {"update", "--dry-run"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err == nil {
				t.Fatalf("expected missing Installed Source error, got output:\n%s", out)
			}
			for _, want := range []string{"run matty init", missing} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error missing %q: %v", want, err)
				}
			}
			if exists(filepath.Join(home, ".matty")) || exists(filepath.Join(home, ".agents")) {
				t.Fatalf("missing source command mutated sandbox")
			}
		})
	}
}

func TestPackageInstalledInstallAndUpdateRejectMalformedSourceBeforeMutation(t *testing.T) {
	home := t.TempDir()
	chdirTempOutsideRepo(t)
	malformed := installedSkillSourceRoot(home)
	for _, rel := range []string{"engineering/ask-matt/SKILL.md", "in-progress/loop-me/SKILL.md"} {
		path := filepath.Join(malformed, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	runner := &fakeRunner{path: map[string]string{"engram": "/fake/bin/engram"}}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}, Runner: runner}
	before := snapshotTree(t, home)
	for _, args := range [][]string{{"install"}, {"update"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			runner.calls = nil
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err == nil {
				t.Fatalf("expected malformed Installed Source error, got output:\n%s", out)
			}
			for _, want := range []string{"malformed", malformed, "productivity"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error missing %q: %v", want, err)
				}
			}
			if len(runner.calls) != 0 {
				t.Fatalf("malformed source ran external commands: %#v", runner.calls)
			}
			if after := snapshotTree(t, home); after != before {
				t.Fatalf("malformed source mutated sandbox\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

func TestPackageInstalledUpdateRejectsStaleDefaultInstalledSource(t *testing.T) {
	withVersion(t, "v0.2.0")
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.1.0")
	if err := os.WriteFile(filepath.Join(repo, "bundle", "skills", "engineering", "ask-matt", "CHANGELOG.md"), []byte("v0.2.0 only"), 0o600); err != nil {
		t.Fatalf("write newer source fixture: %v", err)
	}
	runGitCommand(t, repo, "add", ".")
	runGitCommand(t, repo, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-m", "v0.2.0")
	runGitCommand(t, repo, "tag", "v0.2.0")
	chdirTempOutsideRepo(t)

	runner := &fakeRunner{path: map[string]string{"engram": "/fake/bin/engram"}}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}, Runner: runner}
	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.1.0")
	if err != nil {
		t.Fatalf("init old source failed: %v\n%s", err, out)
	}
	before := snapshotTree(t, home)

	for _, args := range [][]string{{"update", "--dry-run"}, {"update"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			runner.calls = nil
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err == nil {
				t.Fatalf("expected stale Installed Source error, got output:\n%s", out)
			}
			for _, want := range []string{"stale", "v0.2.0", "run matty init"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error missing %q: %v", want, err)
				}
			}
			if len(runner.calls) != 0 {
				t.Fatalf("stale update ran external commands: %#v", runner.calls)
			}
			if after := snapshotTree(t, home); after != before {
				t.Fatalf("stale update mutated sandbox home\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

func TestPackageInstalledUpdateAcceptsSourceAfterInitAlignsReleaseTag(t *testing.T) {
	withVersion(t, "v0.2.0")
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.1.0")
	chdirTempOutsideRepo(t)

	runner := &fakeRunner{path: map[string]string{"engram": "/fake/bin/engram"}}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}, Runner: runner}
	repositoryURL := (&url.URL{Scheme: "file", Path: repo}).String()
	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repositoryURL, "--repository-ref", "v0.1.0"); err != nil {
		t.Fatalf("init old source failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "matty")
	if got := strings.TrimSpace(runGitCommand(t, sourceRoot, "rev-parse", "--is-shallow-repository")); got != "true" {
		t.Fatalf("initial Installed Source shallow = %q, want true", got)
	}

	if err := os.WriteFile(filepath.Join(repo, "bundle", "skills", "engineering", "ask-matt", "CHANGELOG.md"), []byte("v0.2.0 only"), 0o600); err != nil {
		t.Fatalf("write newer source fixture: %v", err)
	}
	runGitCommand(t, repo, "add", ".")
	runGitCommand(t, repo, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-m", "v0.2.0")
	runGitCommand(t, repo, "tag", "v0.2.0")

	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repositoryURL); err != nil {
		t.Fatalf("align source with current release failed: %v\n%s", err, out)
	}
	got := strings.TrimSpace(runGitCommand(t, sourceRoot, "rev-parse", "--verify", "v0.2.0^{commit}"))
	want := strings.TrimSpace(runGitCommand(t, repo, "rev-parse", "--verify", "v0.2.0^{commit}"))
	if got != want {
		t.Fatalf("Installed Source v0.2.0 = %s, want %s", got, want)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "update", "--dry-run"); err != nil {
		t.Fatalf("update --dry-run after init failed: %v\n%s", err, out)
	}
}

func TestUpdateSkipsReleaseRefValidationForConfiguredSkillSource(t *testing.T) {
	withVersion(t, "v0.2.0")
	opts, runner, home := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	before := snapshotTree(t, home)
	runner.calls = nil

	out, err = executeCommand(t, NewRootCommand(opts), "update", "--dry-run")
	if err != nil {
		t.Fatalf("update --dry-run with MATTY_SKILLS_SOURCE failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "matty update dry-run: planned actions") {
		t.Fatalf("update --dry-run did not report plan:\n%s", out)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("update --dry-run executed external commands: %#v", runner.calls)
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("update --dry-run mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func chdirTempOutsideRepo(t *testing.T) string {
	t.Helper()
	cwd := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	return cwd
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestUpdateDryRunReportsPlanAndDoesNotMutateSandbox(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	before := snapshotTree(t, home)
	runner.calls = nil

	out, err = executeCommand(t, NewRootCommand(opts), "update", "--dry-run")
	if err != nil {
		t.Fatalf("update --dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "matty update dry-run: planned actions") || !strings.Contains(out, "run: update Engram via Homebrew") {
		t.Fatalf("update --dry-run did not report expected plan:\n%s", out)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("update --dry-run executed external commands: %#v", runner.calls)
	}
	after := snapshotTree(t, home)
	if after != before {
		t.Fatalf("update --dry-run mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestUninstallDryRunReportsPlanAndDoesNotMutateSandbox(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	before := snapshotTree(t, home)

	out, err = executeCommand(t, NewRootCommand(opts), "uninstall", "--dry-run")
	if err != nil {
		t.Fatalf("uninstall --dry-run failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"matty uninstall dry-run: planned actions",
		"remove: remove managed skill ask-matt",
		"remove-codex-prompt: remove Codex Matty prompt markers",
		"remove-opencode-prompt: remove OpenCode Matty prompt reference",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("uninstall --dry-run output missing %q:\n%s", want, out)
		}
	}
	after := snapshotTree(t, home)
	if after != before {
		t.Fatalf("uninstall --dry-run mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestInstallDryRunReportsPlanAndDoesNotMutateSandbox(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	runner.path = nil
	missingPrefix := filepath.Join(t.TempDir(), "missing-homebrew")
	opts.Env.(MapEnv)["HOMEBREW_PREFIX"] = missingPrefix
	opts.Env.(MapEnv)["PATH"] = ""
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

	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	engram := engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv)
	wants := []string{
		"matty install dry-run: planned actions",
		"write-file: persist Matty state metadata",
		filepath.Join(home, ".matty", "config.json"),
		"symlink: link managed skill ask-matt",
		"run: install Engram via Homebrew (brew install gentleman-programming/tap/engram)",
		"run: delegate Codex Engram setup through Homebrew binary (" + engram + " setup codex)",
		"run: delegate OpenCode Engram setup through Homebrew binary (" + engram + " setup opencode)",
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

func TestInstallWritesSmallStateAndRunsEngramSetup(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if got, want := callStrings(runner.calls), engramSetupCallStrings(t, opts); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("runner calls = %#v, want %#v", got, want)
	}

	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	state, found, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if !found {
		t.Fatal("expected state file to be written")
	}
	if state.SchemaVersion != corelifecycle.SchemaVersion || state.MattyVersion != mattyversion.Value {
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
	for _, name := range []string{"ask-matt", "codebase-design", "grilling", "handoff", "loop-me", "wayfinder"} {
		link := filepath.Join(home, ".agents", "skills", name)
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("expected managed skill link %s: %v", link, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is not a symlink", link)
		}
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
		t.Fatalf("doctor with only PASS and WARN checks failed: %v\n%s", err, out)
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

func TestDoctorWarnsWhenNullManagedSkillsHaveExpectedUnmanagedSymlinks(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	createUnmanagedSkillSymlinks(t, paths, filepath.Join(home, "stale-repo-skills"))
	if err := os.MkdirAll(paths.MattyDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	state := corelifecycle.DesiredState(classicStateConfig(paths), fixedTestTime(), nil)
	if err := corelifecycle.SaveState(paths.StateFile, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}
	before := snapshotTree(t, home)
	runner.calls = nil

	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	after := snapshotTree(t, home)
	if before != after {
		t.Fatalf("doctor mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("doctor ran external commands: %#v", runner.calls)
	}
	recoveryAdvice := unmanagedSymlinkRecoveryAdvice()
	for _, want := range []string{
		"WARN skill-symlinks: state has no managed skills",
		"6 expected skill symlinks are unmanaged by current Matty state",
		filepath.Join(paths.AgentSkillsDir, "ask-matt") + " -> " + filepath.Join(home, "stale-repo-skills", "ask-matt"),
		recoveryAdvice,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsFullSetupHealthAndIsReadOnly(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.CodexPromptFile), 0o700); err != nil {
		t.Fatalf("mkdir codex config: %v", err)
	}
	if err := os.WriteFile(paths.CodexPromptFile, []byte("<!-- gentle-ai:persona -->\nkeep\n<!-- /gentle-ai:persona -->\n"), 0o600); err != nil {
		t.Fatalf("write codex conflict fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.OpenCodeConfigFile), 0o700); err != nil {
		t.Fatalf("mkdir opencode config: %v", err)
	}
	if err := os.WriteFile(paths.OpenCodeConfigFile, []byte(`{"plugin":["gentle-ai"]}`), 0o600); err != nil {
		t.Fatalf("write opencode conflict fixture: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	before := snapshotTree(t, paths.HomeDir)
	runner.calls = nil

	out, err = executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	after := snapshotTree(t, paths.HomeDir)
	if before != after {
		t.Fatalf("doctor mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("doctor ran external commands: %#v", runner.calls)
	}
	for _, want := range []string{
		"PASS matty-state:",
		"PASS skill-symlinks:",
		"PASS engram-binary:",
		"PASS engram-setup:",
		"PASS codex-config:",
		"PASS opencode-config:",
		"WARN codex-conflict:",
		"WARN opencode-conflict:",
		"state records Codex and OpenCode setup expectations",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsCorruptStateAsFailedCheck(t *testing.T) {
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

	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if !errors.Is(err, ErrDoctorUnhealthy) {
		t.Fatalf("doctor error = %v, want ErrDoctorUnhealthy\n%s", err, out)
	}
	if !strings.Contains(out, "FAIL matty-state:") || !strings.Contains(out, "invalid JSON") {
		t.Fatalf("doctor did not report corrupt state as failed check:\n%s", out)
	}
}

func TestDoctorReportsMissingRequiredExecutableAndFailsHealthGate(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	opts.Runner = &fakeRunner{}

	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if !errors.Is(err, ErrDoctorUnhealthy) {
		t.Fatalf("doctor error = %v, want ErrDoctorUnhealthy\n%s", err, out)
	}
	if !strings.Contains(out, "FAIL engram-binary: engram is not available on PATH") {
		t.Fatalf("doctor did not preserve the missing executable report:\n%s", out)
	}
	if !strings.Contains(out, "WARN opencode-config:") {
		t.Fatalf("doctor stopped rendering after the failed check:\n%s", out)
	}
}

func TestDoctorReportsIncompleteEngramSetupExpectationsAsFailedCheck(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if err := os.MkdirAll(paths.MattyDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	state := corelifecycle.DesiredState(classicStateConfig(paths), fixedTestTime(), nil)
	state.ConfiguredSurfaces = []string{"codex"}
	if err := corelifecycle.SaveState(paths.StateFile, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if !errors.Is(err, ErrDoctorUnhealthy) {
		t.Fatalf("doctor error = %v, want ErrDoctorUnhealthy\n%s", err, out)
	}
	if !strings.Contains(out, "FAIL engram-setup:") || !strings.Contains(out, "Codex and OpenCode setup expectations") {
		t.Fatalf("doctor did not fail incomplete Engram setup expectations:\n%s", out)
	}
}

func TestDoctorReportsOpenCodeInspectErrorsUnderConfigCheck(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.OpenCodeConfigFile), 0o700); err != nil {
		t.Fatalf("mkdir opencode config: %v", err)
	}
	if err := os.WriteFile(paths.OpenCodeConfigFile, []byte(`{"instructions": "wrong"}`), 0o600); err != nil {
		t.Fatalf("write invalid opencode config: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if !errors.Is(err, ErrDoctorUnhealthy) {
		t.Fatalf("doctor error = %v, want ErrDoctorUnhealthy\n%s", err, out)
	}
	if !strings.Contains(out, "FAIL opencode-config:") || strings.Contains(out, "FAIL opencode:") {
		t.Fatalf("doctor did not report OpenCode inspect error under opencode-config:\n%s", out)
	}
}

func hasManagedSkill(state corelifecycle.State, name string) bool {
	for _, skill := range state.ManagedSkills {
		if skill.Name == name {
			return true
		}
	}
	return false
}

func TestInstallFailsWhenSelectedInProgressSkillMissing(t *testing.T) {
	home := t.TempDir()
	sourceRoot := createSkillSource(t)
	if err := os.RemoveAll(filepath.Join(sourceRoot, "in-progress", "loop-me")); err != nil {
		t.Fatalf("remove source skill: %v", err)
	}
	opts := Options{Env: MapEnv{"HOME": home, "MATTY_SKILLS_SOURCE": sourceRoot}, Runner: &fakeRunner{path: map[string]string{"engram": "/fake/bin/engram"}}}

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err == nil {
		t.Fatalf("expected missing source skill error, got output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "loop-me") {
		t.Fatalf("error = %v, want loop-me", err)
	}
	if exists(filepath.Join(home, ".agents")) || exists(filepath.Join(home, ".matty")) {
		t.Fatalf("install mutated sandbox despite missing source skill")
	}
}

func TestInstallPreservesUnmanagedPaths(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if err := os.MkdirAll(paths.AgentSkillsDir, 0o700); err != nil {
		t.Fatalf("mkdir agent skills: %v", err)
	}
	unmanagedFile := filepath.Join(paths.AgentSkillsDir, "ask-matt")
	if err := os.WriteFile(unmanagedFile, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}
	otherTarget := filepath.Join(home, "elsewhere")
	if err := os.MkdirAll(otherTarget, 0o700); err != nil {
		t.Fatalf("mkdir unmanaged target: %v", err)
	}
	unmanagedSymlink := filepath.Join(paths.AgentSkillsDir, "handoff")
	if err := os.Symlink(otherTarget, unmanagedSymlink); err != nil {
		t.Fatalf("write unmanaged symlink: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	data, err := os.ReadFile(unmanagedFile)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("unmanaged file was not preserved: data=%q err=%v", data, err)
	}
	gotTarget, err := os.Readlink(unmanagedSymlink)
	if err != nil || gotTarget != otherTarget {
		t.Fatalf("unmanaged symlink target = %q, %v; want %q", gotTarget, err, otherTarget)
	}

	state, found, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil || !found {
		t.Fatalf("LoadState = found %v err %v", found, err)
	}
	if hasManagedSkill(state, "ask-matt") || hasManagedSkill(state, "handoff") {
		t.Fatalf("unmanaged collisions should not be recorded as managed: %#v", state.ManagedSkills)
	}
	if !hasManagedSkill(state, "wayfinder") {
		t.Fatalf("non-conflicting skills should still be managed: %#v", state.ManagedSkills)
	}
}

func TestInstallWarnsWhenMostExpectedSkillsAreUnmanagedSymlinks(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	createUnmanagedSkillSymlinks(t, paths, filepath.Join(home, "stale-repo-skills"))

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	recoveryAdvice := unmanagedSymlinkRecoveryAdvice()
	for _, want := range []string{
		"warning: skipped 6 unmanaged skill symlinks; setup may be incomplete",
		"Example: " + filepath.Join(paths.AgentSkillsDir, "ask-matt") + " -> " + filepath.Join(home, "stale-repo-skills", "ask-matt"),
		recoveryAdvice,
		"matty install: synced 0 managed skills",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("install output missing %q:\n%s", want, out)
		}
	}
	state, found, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil || !found {
		t.Fatalf("LoadState = found %v err %v", found, err)
	}
	if len(state.ManagedSkills) != 0 {
		t.Fatalf("expected no managed skills when all skill links were unmanaged, got %#v", state.ManagedSkills)
	}
	for _, name := range testSkillNames() {
		target, err := os.Readlink(filepath.Join(paths.AgentSkillsDir, name))
		if err != nil {
			t.Fatalf("read unmanaged symlink %s: %v", name, err)
		}
		if want := filepath.Join(home, "stale-repo-skills", name); target != want {
			t.Fatalf("unmanaged symlink %s target = %q, want %q", name, target, want)
		}
	}
}

func TestInstallAndUpdateAreIdempotent(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	before := readSkillLinks(t, paths)

	plan, err := BuildInstallPlan(paths, fixedTestTime(), true)
	if err != nil {
		t.Fatalf("BuildInstallPlan failed: %v", err)
	}
	for _, action := range plan.Actions {
		if action.Kind == ActionSymlink {
			t.Fatalf("idempotent plan should not recreate symlink: %#v", action)
		}
	}

	out, err = executeCommand(t, NewRootCommand(opts), "update")
	if err != nil {
		t.Fatalf("update failed: %v\n%s", err, out)
	}
	after := readSkillLinks(t, paths)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("skill links changed after update:\nbefore=%v\nafter=%v", before, after)
	}
}

func TestUninstallWithoutStateRemovesOnlyMarkerOwnedPrompts(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if err := os.Remove(paths.StateFile); err != nil {
		t.Fatalf("remove state fixture: %v", err)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "uninstall")
	if err != nil {
		t.Fatalf("uninstall without state failed: %v\n%s", err, out)
	}
	if strings.Contains(readFileString(t, paths.CodexPromptFile), "<!-- matty:skills-router -->") {
		t.Fatalf("uninstall without state left Codex Matty marker")
	}
	config := readFileString(t, paths.OpenCodeConfigFile)
	if strings.Contains(config, paths.OpenCodePromptFile) {
		t.Fatalf("uninstall without state left OpenCode Matty instruction:\n%s", config)
	}
	if exists(paths.OpenCodePromptFile) {
		t.Fatalf("uninstall without state left OpenCode Matty prompt file")
	}
	if !exists(filepath.Join(paths.AgentSkillsDir, "wayfinder")) {
		t.Fatalf("uninstall without state should not infer/remove managed skill symlinks")
	}

	out, err = executeCommand(t, NewRootCommand(opts), "uninstall")
	if err != nil {
		t.Fatalf("second uninstall without state should be safe: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no Matty-managed artifacts found") {
		t.Fatalf("second uninstall should report no-op:\n%s", out)
	}
}

func TestUninstallRemovesOnlyManagedSymlinks(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}

	unmanagedFile := filepath.Join(paths.AgentSkillsDir, "personal-note")
	if err := os.WriteFile(unmanagedFile, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}
	unmanagedTarget := filepath.Join(home, "unmanaged-target")
	if err := os.MkdirAll(unmanagedTarget, 0o700); err != nil {
		t.Fatalf("mkdir unmanaged target: %v", err)
	}
	unmanagedLink := filepath.Join(paths.AgentSkillsDir, "external-skill")
	if err := os.Symlink(unmanagedTarget, unmanagedLink); err != nil {
		t.Fatalf("write unmanaged symlink: %v", err)
	}
	changedManaged := filepath.Join(paths.AgentSkillsDir, "ask-matt")
	if err := os.Remove(changedManaged); err != nil {
		t.Fatalf("remove managed link for replacement: %v", err)
	}
	if err := os.Symlink(unmanagedTarget, changedManaged); err != nil {
		t.Fatalf("replace managed link with unmanaged target: %v", err)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "uninstall")
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}
	if exists(paths.StateFile) {
		t.Fatalf("state file still exists after uninstall")
	}
	if exists(filepath.Join(paths.AgentSkillsDir, "wayfinder")) {
		t.Fatalf("managed wayfinder link still exists after uninstall")
	}
	if !exists(unmanagedFile) || !exists(unmanagedLink) || !exists(changedManaged) {
		t.Fatalf("uninstall removed unmanaged paths")
	}
}

func TestPristineInstallUninstallRemovesOnlyMattyCreatedContainers(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "uninstall"); err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}

	for _, path := range []string{paths.MattyDir, filepath.Dir(paths.AgentSkillsDir), filepath.Dir(paths.CodexPromptFile), filepath.Dir(paths.OpenCodeConfigFile)} {
		if exists(path) {
			t.Fatalf("uninstall left Matty-created container %s under pristine HOME %s", path, home)
		}
	}
}

func TestUninstallPreservesPreexistingContainersAndUnmanagedBytes(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.MattyDir, filepath.Dir(paths.AgentSkillsDir), filepath.Dir(paths.CodexPromptFile), filepath.Dir(paths.OpenCodeConfigFile)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	unmanaged := filepath.Join(filepath.Dir(paths.CodexPromptFile), "user.txt")
	want := []byte{0, 1, 2, '\n'}
	if err := os.WriteFile(unmanaged, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "uninstall"); err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}
	for _, path := range []string{paths.MattyDir, filepath.Dir(paths.AgentSkillsDir), filepath.Dir(paths.CodexPromptFile), filepath.Dir(paths.OpenCodeConfigFile)} {
		if !exists(path) {
			t.Fatalf("uninstall removed preexisting container %s", path)
		}
	}
	got, err := os.ReadFile(unmanaged)
	if err != nil || string(got) != string(want) {
		t.Fatalf("unmanaged bytes changed: %v %v", got, err)
	}
}

func TestApplyUninstallRejectsContainerChangeAfterPreviewWithoutRemovingArtifacts(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	state, _, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildUninstallPlan(paths, state)
	concurrent := filepath.Join(filepath.Dir(paths.CodexPromptFile), "concurrent.txt")
	if err := os.WriteFile(concurrent, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyUninstallPlan(context.Background(), paths, plan); !errors.Is(err, ownedcontainer.ErrStalePlan) {
		t.Fatalf("ApplyUninstallPlan() error = %v, want ErrStalePlan", err)
	}
	if !exists(paths.StateFile) || !exists(filepath.Join(paths.AgentSkillsDir, "ask-matt")) {
		t.Fatal("stale uninstall removed Matty artifacts before rejecting the plan")
	}
	if got := readFileString(t, concurrent); got != "keep" {
		t.Fatalf("stale uninstall changed concurrent content: %q", got)
	}
}

func TestUninstallDoesNotTrustContainerPathsOutsideCurrentMattyAllowlist(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "unmanaged-empty")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	state := corelifecycle.DesiredState(classicStateConfig(paths), time.Now(), nil)
	state.CreatedContainers = []ownedcontainer.Record{{Path: outside, Kind: ownedcontainer.Directory}}
	plan := BuildUninstallPlan(paths, state)
	if err := ApplyUninstallPlan(context.Background(), paths, plan); err != nil {
		t.Fatal(err)
	}
	if !exists(outside) {
		t.Fatal("uninstall trusted forged provenance outside its exact path allowlist")
	}
}

func TestEndToEndSandboxLifecyclePreservesGentleAIAndRealHome(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve real home: %v", err)
	}
	if home == realHome {
		t.Fatalf("sandbox HOME unexpectedly equals real HOME %q", realHome)
	}
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	for _, path := range []string{paths.StateFile, paths.CodexPromptFile, paths.OpenCodeConfigFile, paths.OpenCodePromptFile, paths.AgentSkillsDir} {
		if strings.HasPrefix(path, realHome+string(os.PathSeparator)) {
			t.Fatalf("sandbox path %q points inside real HOME %q", path, realHome)
		}
	}

	if err := os.MkdirAll(filepath.Dir(paths.CodexPromptFile), 0o700); err != nil {
		t.Fatalf("mkdir codex config: %v", err)
	}
	codexOriginal := "# Existing Codex\n\n<!-- gentle-ai:persona -->\nkeep gentle codex\n<!-- /gentle-ai:persona -->\n"
	if err := os.WriteFile(paths.CodexPromptFile, []byte(codexOriginal), 0o600); err != nil {
		t.Fatalf("write codex fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.OpenCodeConfigFile), 0o700); err != nil {
		t.Fatalf("mkdir opencode config: %v", err)
	}
	openCodeOriginal := `{"plugin":["gentle-ai"],"instructions":["CONTRIBUTING.md"]}` + "\n"
	if err := os.WriteFile(paths.OpenCodeConfigFile, []byte(openCodeOriginal), 0o600); err != nil {
		t.Fatalf("write opencode fixture: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"warning: Codex prompt contains gentle-ai managed blocks",
		"warning: OpenCode config contains gentle-ai references",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("install output missing %q:\n%s", want, out)
		}
	}
	if !exists(paths.StateFile) || !exists(filepath.Join(paths.AgentSkillsDir, "ask-matt")) {
		t.Fatalf("install did not create expected Matty-managed artifacts in sandbox")
	}

	beforeDoctor := snapshotTree(t, home)
	runner.calls = nil
	out, err = executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	if afterDoctor := snapshotTree(t, home); afterDoctor != beforeDoctor {
		t.Fatalf("doctor mutated sandbox:\nbefore:\n%s\nafter:\n%s", beforeDoctor, afterDoctor)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("doctor ran external commands: %#v", runner.calls)
	}
	for _, want := range []string{"PASS matty-state:", "WARN codex-conflict:", "WARN opencode-conflict:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}

	out, err = executeCommand(t, NewRootCommand(opts), "update")
	if err != nil {
		t.Fatalf("update failed: %v\n%s", err, out)
	}
	if got, want := callStrings(runner.calls), engramUpdateCallStrings(t, opts); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("update runner calls = %#v, want %#v", got, want)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "uninstall")
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, out)
	}
	if exists(paths.StateFile) || exists(filepath.Join(paths.AgentSkillsDir, "ask-matt")) || exists(paths.OpenCodePromptFile) {
		t.Fatalf("uninstall left Matty-managed artifacts in sandbox")
	}
	if got := readFileString(t, paths.CodexPromptFile); got != codexOriginal {
		t.Fatalf("uninstall did not restore Codex user/Gentle AI content:\ngot:\n%s\nwant:\n%s", got, codexOriginal)
	}
	openCodeAfter := readFileString(t, paths.OpenCodeConfigFile)
	for _, want := range []string{"gentle-ai", "CONTRIBUTING.md"} {
		if !strings.Contains(openCodeAfter, want) {
			t.Fatalf("uninstall lost OpenCode user/Gentle AI content %q:\n%s", want, openCodeAfter)
		}
	}
	if strings.Contains(openCodeAfter, paths.OpenCodePromptFile) {
		t.Fatalf("uninstall left Matty OpenCode prompt reference:\n%s", openCodeAfter)
	}
}

func TestInstallDryRunReportsUnmanagedSkipsWithoutMutating(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	if err := os.MkdirAll(paths.AgentSkillsDir, 0o700); err != nil {
		t.Fatalf("mkdir agent skills: %v", err)
	}
	unmanaged := filepath.Join(paths.AgentSkillsDir, "ask-matt")
	if err := os.WriteFile(unmanaged, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "skip: preserve unmanaged path for skill ask-matt") {
		t.Fatalf("dry-run did not report unmanaged skip:\n%s", out)
	}
	data, err := os.ReadFile(unmanaged)
	if err != nil || string(data) != "keep" {
		t.Fatalf("dry-run mutated unmanaged file: data=%q err=%v", data, err)
	}
	if exists(paths.StateFile) {
		t.Fatalf("dry-run wrote state")
	}
}

func readSkillLinks(t *testing.T, paths Paths) []string {
	t.Helper()
	entries, err := os.ReadDir(paths.AgentSkillsDir)
	if err != nil {
		t.Fatalf("read agent skills dir: %v", err)
	}
	var links []string
	for _, entry := range entries {
		path := filepath.Join(paths.AgentSkillsDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("lstat %s: %v", path, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(path)
		if err != nil {
			t.Fatalf("readlink %s: %v", path, err)
		}
		links = append(links, entry.Name()+"->"+target)
	}
	return links
}

func TestInstallInstallsEngramViaHomebrewWhenMissing(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	runner.path = nil
	missingPrefix := filepath.Join(t.TempDir(), "missing-homebrew")
	opts.Env.(MapEnv)["HOMEBREW_PREFIX"] = missingPrefix
	opts.Env.(MapEnv)["PATH"] = ""
	engram := filepath.Join(missingPrefix, "bin", "engram")
	runner.after = map[string]func(){"brew install gentleman-programming/tap/engram": func() {
		writeEngramExecutable(t, filepath.Dir(engram), "engram version 1.19.0")
	}}
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	want := []string{"brew install gentleman-programming/tap/engram", engram + " setup codex", engram + " setup opencode"}
	if got := callStrings(runner.calls); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("runner calls = %#v, want %#v", got, want)
	}
}

func TestUpdateRunsEngramHomebrewUpdateAndSetup(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "update")
	if err != nil {
		t.Fatalf("update failed: %v\n%s", err, out)
	}
	want := engramUpdateCallStrings(t, opts)
	if got := callStrings(runner.calls); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("runner calls = %#v, want %#v", got, want)
	}
}

func TestInstallFailsClearlyWhenHomebrewSetupBinaryMissingAfterBrewInstall(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	runner.path = nil
	missingPrefix := filepath.Join(t.TempDir(), "missing-homebrew")
	opts.Env.(MapEnv)["HOMEBREW_PREFIX"] = missingPrefix
	opts.Env.(MapEnv)["PATH"] = ""

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err == nil {
		t.Fatalf("expected missing canonical Engram error, got output:\n%s", out)
	}
	for _, want := range []string{"canonical Homebrew Engram was not found", filepath.Join(missingPrefix, "bin", "engram"), "HOMEBREW_PREFIX"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestExternalCommandFailureIsActionable(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	runner.path = nil
	opts.Env.(MapEnv)["HOMEBREW_PREFIX"] = filepath.Join(t.TempDir(), "missing-homebrew")
	opts.Env.(MapEnv)["PATH"] = ""
	runner.fail = map[string]error{"brew install gentleman-programming/tap/engram": errors.New("brew missing")}
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err == nil {
		t.Fatalf("expected install failure, got output:\n%s", out)
	}
	for _, want := range []string{"failed to install Engram via Homebrew", "ensure Homebrew is installed", "brew missing"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestInterruptedInstallIsExplicitAndDoctorReportsSafeRecovery(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	runner.fail = map[string]error{engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv) + " setup codex": errors.New("interrupted")}

	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err == nil {
		t.Fatalf("install unexpectedly succeeded:\n%s", out)
	}
	state, found, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil || !found {
		t.Fatalf("LoadState = found %v err %v", found, err)
	}
	if !state.RecoveryRequired() {
		t.Fatalf("install status = %q, want recovery-required", state.InstallStatus)
	}

	before := snapshotTree(t, paths.HomeDir)
	runner.calls = nil
	out, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if !errors.Is(err, ErrDoctorUnhealthy) {
		t.Fatalf("doctor error = %v, want ErrDoctorUnhealthy\n%s", err, out)
	}
	for _, want := range []string{"FAIL matty-state:", "interrupted", "matty install or matty update"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
	if after := snapshotTree(t, paths.HomeDir); after != before {
		t.Fatalf("doctor mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("doctor ran commands: %#v", runner.calls)
	}
}

func callStrings(calls []fakeCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, strings.Join(append([]string{call.name}, call.args...), " "))
	}
	return out
}

func fixedTestTime() time.Time { return time.Unix(0, 0).UTC() }

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries = append(entries, fmt.Sprintf("%s symlink %s mode=%s mod=%d", rel, target, info.Mode(), info.ModTime().UnixNano()))
		case entry.IsDir():
			entries = append(entries, fmt.Sprintf("%s dir mode=%s mod=%d", rel, info.Mode(), info.ModTime().UnixNano()))
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entries = append(entries, fmt.Sprintf("%s file mode=%s mod=%d size=%d %s", rel, info.Mode(), info.ModTime().UnixNano(), info.Size(), string(data)))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return strings.Join(entries, "\n")
}

func TestInitClonesDefaultInstalledSourceAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "matty")
	if !exists(filepath.Join(sourceRoot, "bundle", "skills")) {
		t.Fatalf("init did not clone bundle/skills into %s", sourceRoot)
	}
	for _, want := range []string{"cloning Installed Source into " + sourceRoot, "initialized Installed Source"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, sourceRoot) {
		t.Fatalf("init output did not report initialized source:\n%s", out)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("second init failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already initialized") {
		t.Fatalf("second init did not report idempotent state:\n%s", out)
	}
}

func TestInitRejectsMalformedExistingInstalledSourceWithoutMutation(t *testing.T) {
	home := t.TempDir()
	installedRoot := DefaultInstalledSourceRoot(home)
	manifest := filepath.Join(installedRoot, "bundle", "skills", "engineering", "ask-matt", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, home)
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init")
	if err == nil {
		t.Fatalf("expected malformed Installed Source error, got output:\n%s", out)
	}
	for _, want := range []string{"not a valid Matty checkout", "Move it aside", "--source-root"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if after := snapshotTree(t, home); after != before {
		t.Fatalf("init mutated malformed Installed Source\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestInitDoesNotPublishMalformedClonedSource(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	manifest := filepath.Join(repo, "bundle", "skills", "engineering", "ask-matt", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "add", ".")
	runGitCommand(t, repo, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-m", "malformed")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err == nil {
		t.Fatalf("expected malformed cloned source error, got output:\n%s", out)
	}
	for _, want := range []string{"invalid skill bundle", "productivity"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if exists(DefaultInstalledSourceRoot(home)) {
		t.Fatalf("init published malformed source at %s", DefaultInstalledSourceRoot(home))
	}
}

func TestInitDoesNotReplaceValidInstalledSourceWithMalformedRef(t *testing.T) {
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.1.0")
	if err := os.RemoveAll(filepath.Join(repo, "bundle", "skills", "productivity")); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repo, "add", "-A")
	runGitCommand(t, repo, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-m", "malformed")
	runGitCommand(t, repo, "tag", "v0.2.0")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}
	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.1.0"); err != nil {
		t.Fatalf("initialize valid source: %v\n%s", err, out)
	}
	installedRoot := DefaultInstalledSourceRoot(home)
	beforeHead := strings.TrimSpace(runGitCommand(t, installedRoot, "rev-parse", "HEAD"))
	beforeBundle := snapshotTree(t, filepath.Join(installedRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.2.0")
	if err == nil {
		t.Fatalf("expected malformed ref error, got output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "invalid skill bundle") || !strings.Contains(err.Error(), "productivity") {
		t.Fatalf("malformed ref error is not actionable: %v", err)
	}
	if afterHead := strings.TrimSpace(runGitCommand(t, installedRoot, "rev-parse", "HEAD")); afterHead != beforeHead {
		t.Fatalf("Installed Source HEAD changed from %s to %s", beforeHead, afterHead)
	}
	if afterBundle := snapshotTree(t, filepath.Join(installedRoot, "bundle")); afterBundle != beforeBundle {
		t.Fatalf("malformed ref replaced valid bundle\nbefore:\n%s\nafter:\n%s", beforeBundle, afterBundle)
	}
}

func TestInitReportsUpdateProgress(t *testing.T) {
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.1.0")
	if err := os.WriteFile(filepath.Join(repo, "UPDATED"), []byte("updated"), 0o600); err != nil {
		t.Fatalf("write update fixture: %v", err)
	}
	runGitCommand(t, repo, "add", "UPDATED")
	runGitCommand(t, repo, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-m", "updated")
	runGitCommand(t, repo, "tag", "v0.2.0")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.1.0"); err != nil {
		t.Fatalf("initial init failed: %v\n%s", err, out)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.2.0")
	if err != nil {
		t.Fatalf("update init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "matty")
	for _, want := range []string{"updating Installed Source at " + sourceRoot + " to v0.2.0", "updated Installed Source"} {
		if !strings.Contains(out, want) {
			t.Fatalf("update output missing %q:\n%s", want, out)
		}
	}
}

func TestInitReportsProgressAndGitContextWhenCloneFails(t *testing.T) {
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".local", "share", "matty")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", filepath.Join(t.TempDir(), "missing-repository"))
	if err == nil {
		t.Fatalf("expected clone failure, got output:\n%s", out)
	}
	if !strings.Contains(out, "cloning Installed Source into "+sourceRoot) {
		t.Fatalf("clone failure output did not include progress:\n%s", out)
	}
	for _, want := range []string{"clone Matty Source of Truth", "git clone", "failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("clone failure missing %q: %v", want, err)
		}
	}
}

func TestInitSupportsHomeFlag(t *testing.T) {
	envHome := t.TempDir()
	flagHome := t.TempDir()
	repo := createMattySourceRepo(t)
	opts := Options{Env: MapEnv{"HOME": envHome, "XDG_CONFIG_HOME": filepath.Join(flagHome, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--home", flagHome, "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(flagHome, ".local", "share", "matty", "bundle", "skills")) {
		t.Fatalf("init did not use --home for default Installed Source")
	}
	if exists(filepath.Join(envHome, ".local", "share", "matty")) {
		t.Fatalf("init --home unexpectedly wrote Env HOME")
	}
}

func TestInitSupportsExplicitSourceRoot(t *testing.T) {
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	sourceRoot := filepath.Join(t.TempDir(), "custom-source")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--source-root", sourceRoot, "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(sourceRoot, "bundle", "skills")) {
		t.Fatalf("init did not clone into explicit source root")
	}
	if exists(filepath.Join(home, ".local", "share", "matty")) {
		t.Fatalf("init with --source-root unexpectedly wrote default Installed Source")
	}
}

func TestInitRejectsInvalidNonEmptyDestination(t *testing.T) {
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	sourceRoot := filepath.Join(home, ".local", "share", "matty")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "README.md"), []byte("not matty"), 0o600); err != nil {
		t.Fatalf("write invalid destination: %v", err)
	}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err == nil {
		t.Fatalf("expected invalid destination error, got output:\n%s", out)
	}
	for _, want := range []string{"not a valid Matty checkout", "Move it aside", "--source-root"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if !exists(filepath.Join(sourceRoot, "README.md")) {
		t.Fatalf("init removed user data from invalid destination")
	}
}

func TestInitDefaultsReleaseVersionAsRepositoryRef(t *testing.T) {
	withVersion(t, "v0.2.3")
	home := t.TempDir()
	repo := createMattySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.2.3")
	runGitCommand(t, repo, "checkout", "-b", "next")
	if err := os.WriteFile(filepath.Join(repo, "UNRELEASED"), []byte("main only"), 0o600); err != nil {
		t.Fatalf("write unreleased file: %v", err)
	}
	runGitCommand(t, repo, "add", "UNRELEASED")
	runGitCommand(t, repo, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-m", "unreleased")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "matty")
	if exists(filepath.Join(sourceRoot, "UNRELEASED")) {
		t.Fatalf("release init cloned repository HEAD instead of release tag")
	}
	got := strings.TrimSpace(runGitCommand(t, sourceRoot, "rev-parse", "HEAD"))
	want := strings.TrimSpace(runGitCommand(t, repo, "rev-parse", "v0.2.3^{commit}"))
	if got != want {
		t.Fatalf("cloned HEAD = %s, want release tag commit %s", got, want)
	}
}

func createMattySourceRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, rel := range []string{
		"bundle/skills/engineering/ask-matt/SKILL.md",
		"bundle/skills/engineering/codebase-design/SKILL.md",
		"bundle/skills/productivity/grilling/SKILL.md",
		"bundle/skills/productivity/handoff/SKILL.md",
		"bundle/skills/in-progress/loop-me/SKILL.md",
		"bundle/skills/engineering/wayfinder/SKILL.md",
	} {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir repo fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
			t.Fatalf("write repo fixture: %v", err)
		}
	}
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "add", ".")
	runGitCommand(t, repo, "-c", "user.name=Matty Test", "-c", "user.email=matty@example.test", "commit", "-m", "initial")
	return repo
}

func runGitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	home := t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "xdg-config"), "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func TestInterruptedInstallConvergesWithoutAdoptingConflict(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	setup := engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv) + " setup codex"
	runner.fail = map[string]error{setup: errors.New("interrupted")}
	if _, err := executeCommand(t, NewRootCommand(opts), "install"); err == nil {
		t.Fatal("expected interrupted install")
	}
	conflict := filepath.Join(paths.AgentSkillsDir, "ask-matt")
	if err := os.Remove(conflict); err != nil {
		t.Fatal(err)
	}
	unmanagedTarget := filepath.Join(home, "unmanaged-target")
	if err := os.Mkdir(unmanagedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unmanagedTarget, conflict); err != nil {
		t.Fatal(err)
	}
	delete(runner.fail, setup)
	if out, err := executeCommand(t, NewRootCommand(opts), "update"); err != nil {
		t.Fatalf("update recovery failed: %v\n%s", err, out)
	}
	state, _, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if state.RecoveryRequired() || state.InstallStatus != corelifecycle.InstallConfirmed {
		t.Fatalf("recovered status = %q", state.InstallStatus)
	}
	for _, skill := range state.ManagedSkills {
		if skill.LinkPath == conflict {
			t.Fatal("recovery adopted conflicting symlink")
		}
	}
	if target, err := os.Readlink(conflict); err != nil || target != unmanagedTarget {
		t.Fatalf("conflict changed: %q %v", target, err)
	}
}

func TestUninstallFromInterruptedInstallUsesOnlyVerifiedOwnership(t *testing.T) {
	opts, runner, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	setup := engrambin.ExpectedHomebrewPath(paths.HomebrewPrefixEnv) + " setup codex"
	runner.fail = map[string]error{setup: errors.New("interrupted")}
	if _, err := executeCommand(t, NewRootCommand(opts), "install"); err == nil {
		t.Fatal("expected interrupted install")
	}
	changed := filepath.Join(paths.AgentSkillsDir, "ask-matt")
	if err := os.Remove(changed); err != nil {
		t.Fatal(err)
	}
	unmanagedTarget := filepath.Join(home, "unmanaged-target")
	if err := os.Mkdir(unmanagedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unmanagedTarget, changed); err != nil {
		t.Fatal(err)
	}
	contributor := filepath.Join(paths.AgentSkillsDir, "contributor.txt")
	if err := os.WriteFile(contributor, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "uninstall"); err != nil {
		t.Fatalf("partial uninstall failed: %v\n%s", err, out)
	}
	if !exists(changed) || !exists(contributor) {
		t.Fatal("partial uninstall removed unmanaged content")
	}
	if exists(filepath.Join(paths.AgentSkillsDir, "wayfinder")) {
		t.Fatal("partial uninstall left verified owned symlink")
	}
}

func TestFinalStateCommitFailureLeavesRecoverableState(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	originalPersist := persistClassicState
	t.Cleanup(func() { persistClassicState = originalPersist })
	persistClassicState = func(path string, state corelifecycle.State) error {
		if state.InstallStatus == corelifecycle.InstallConfirmed {
			return errors.New("final commit interrupted")
		}
		return corelifecycle.SaveState(path, state)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err == nil {
		t.Fatalf("install unexpectedly succeeded:\n%s", out)
	}
	state, found, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil || !found || !state.RecoveryRequired() {
		t.Fatalf("state = %#v found %v err %v", state, found, err)
	}
}

func TestStatePreparationFailureLeavesNoLocalWrites(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	originalPersist := persistClassicState
	t.Cleanup(func() { persistClassicState = originalPersist })
	persistClassicState = func(string, corelifecycle.State) error { return errors.New("preparation interrupted") }
	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err == nil {
		t.Fatalf("install unexpectedly succeeded:\n%s", out)
	}
	if got := snapshotTree(t, home); strings.Count(strings.TrimSpace(got), "\n") != 0 {
		t.Fatalf("preparation failure left local writes:\n%s", got)
	}
}

func TestPartialContainerProvisionFailureKeepsRecoveryEvidence(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	conflict := filepath.Join(home, ".codex")
	if err := os.WriteFile(conflict, []byte("contributor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err == nil {
		t.Fatalf("install unexpectedly succeeded:\n%s", out)
	}
	state, found, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil || !found || !state.RecoveryRequired() {
		t.Fatalf("state = %#v found %v err %v", state, found, err)
	}
	if got := readFileString(t, conflict); got != "contributor" {
		t.Fatalf("container conflict changed: %q", got)
	}
}

func TestSymlinkOwnershipPersistenceFailureRollsBackUnrecordedLink(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	originalPersist := persistClassicState
	t.Cleanup(func() { persistClassicState = originalPersist })
	persistClassicState = func(path string, state corelifecycle.State) error {
		if state.RecoveryRequired() && len(state.ManagedSkills) == 1 {
			return errors.New("ownership persistence interrupted")
		}
		return corelifecycle.SaveState(path, state)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err == nil {
		t.Fatalf("install unexpectedly succeeded:\n%s", out)
	}
	if exists(filepath.Join(paths.AgentSkillsDir, "ask-matt")) {
		t.Fatal("unrecorded symlink was not rolled back")
	}
	state, found, err := corelifecycle.LoadState(paths.StateFile)
	if err != nil || !found || !state.RecoveryRequired() || len(state.ManagedSkills) != 0 {
		t.Fatalf("state = %#v found %v err %v", state, found, err)
	}
}
