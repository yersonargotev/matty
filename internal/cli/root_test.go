package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/bootstrap"
	"github.com/yersonargotev/packy/internal/corelifecycle"
	"github.com/yersonargotev/packy/internal/engrambin"
	"github.com/yersonargotev/packy/internal/setuphealth"
	"github.com/yersonargotev/packy/internal/skillbundle"
	packyversion "github.com/yersonargotev/packy/internal/version"
)

func TestDoctorJSONHealthyWarningsAndFailures(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		opts, _, _ := sandboxOptions(t)
		opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
			return setuphealth.Report{SchemaVersion: 1, Kind: "doctor", Checks: []setuphealth.Check{{Severity: setuphealth.Pass, Name: "fixture", Detail: "healthy"}}, Summary: setuphealth.Summary{Status: "healthy", Passes: 1}}, nil
		}
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, out)
		}
		var doc struct {
			SchemaVersion int    `json:"schema_version"`
			Report        string `json:"report"`
			Checks        []struct{ Name, Severity, Detail string }
			Summary       setupHealthJSONSummary `json:"summary"`
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
		opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
			return setuphealth.Report{SchemaVersion: 1, Kind: "doctor", Checks: []setuphealth.Check{{Severity: setuphealth.Warn, Name: "fixture", Detail: "warning"}}, Summary: setuphealth.Summary{Status: "warnings", Warnings: 1}}, nil
		}
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, out)
		}
		var doc struct {
			Summary setupHealthJSONSummary `json:"summary"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Summary.Status != "warnings" || doc.Summary.Warnings == 0 {
			t.Fatalf("warning report: %#v err=%v", doc, err)
		}
	})
	t.Run("failures emit full report before error", func(t *testing.T) {
		opts, _, _ := sandboxOptions(t)
		opts.SetupHealthDiagnose = func() (setuphealth.Report, error) {
			return setuphealth.Report{SchemaVersion: 1, Kind: "doctor", Checks: []setuphealth.Check{
				{Severity: setuphealth.Fail, Name: "failed", Detail: "failure"},
				{Severity: setuphealth.Warn, Name: "later", Detail: "complete report"},
			}, Summary: setuphealth.Summary{Status: "failures", Warnings: 1, Failures: 1}}, nil
		}
		out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
		if !errors.Is(err, ErrDoctorUnhealthy) {
			t.Fatalf("error=%v", err)
		}
		var doc struct {
			Checks  []struct{ Name, Severity string }
			Summary setupHealthJSONSummary `json:"summary"`
		}
		if json.Unmarshal([]byte(out), &doc) != nil || doc.Summary.Failures == 0 || len(doc.Checks) < 2 {
			t.Fatalf("incomplete report: %s", out)
		}
	})
}

func TestClassicLifecycleJSONV2PreviewAndResults(t *testing.T) {
	opts, _, _ := sandboxOptions(t)

	for _, operation := range []string{"install", "update", "uninstall"} {
		t.Run(operation+" preview", func(t *testing.T) {
			out, err := executeCommand(t, NewRootCommand(opts), operation, "--dry-run", "--json")
			if err != nil {
				t.Fatalf("%s preview: %v\n%s", operation, err, out)
			}
			var report classicLifecyclePlanJSON
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, out)
			}
			if report.SchemaVersion != 2 || report.Report != "classic-lifecycle-preview" || string(report.Operation) != operation || !report.DryRun || report.DesiredSurfaces == nil || report.PendingPrerequisites == nil || report.Preserved == nil || report.Blockers == nil || report.Recovery == nil || report.Actions == nil {
				t.Fatalf("preview contract = %#v", report)
			}
			if strings.Contains(out, "Skill source:") || strings.Contains(out, "planned actions") {
				t.Fatalf("human output mixed into JSON: %s", out)
			}
		})
	}

	for _, operation := range []string{"install", "update", "uninstall"} {
		t.Run(operation+" result", func(t *testing.T) {
			out, err := executeCommand(t, NewRootCommand(opts), operation, "--json")
			if err != nil {
				t.Fatalf("%s result: %v\n%s", operation, err, out)
			}
			var report classicLifecycleResultJSON
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, out)
			}
			if report.SchemaVersion != 2 || report.Report != "classic-lifecycle-result" || string(report.Operation) != operation || !report.Committed || report.DesiredSurfaces == nil || report.PendingPrerequisites == nil || report.Preserved == nil || report.Blockers == nil || report.Recovery == nil || report.CompletedEffects == nil || report.NotStartedEffects == nil || report.Warnings == nil {
				t.Fatalf("result contract = %#v", report)
			}
		})
	}
}

func TestClassicLifecycleHumanResultHasStableEvidenceOrder(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	wants := []string{
		"Outcome:", "Desired surfaces:", "State transition:", "Pending prerequisites:",
		"Preserved:", "Lifecycle blockers:", "Recovery:", "Completed effects:",
		"Failed effect:", "Not started effects:", "packy install: synced",
	}
	prior := -1
	for _, want := range wants {
		index := strings.Index(out, want)
		if index < 0 || index <= prior {
			t.Fatalf("%q missing or out of order:\n%s", want, out)
		}
		prior = index
	}
	if !strings.Contains(out, "install Claude Code 2.1.203 or newer") {
		t.Fatalf("pending prerequisite lacks remediation: %s", out)
	}
}

func TestClassicLifecycleOutcomeExitMappingIsStable(t *testing.T) {
	for _, outcome := range []corelifecycle.Outcome{corelifecycle.OutcomeConverged, corelifecycle.OutcomeApplied, corelifecycle.OutcomeAppliedWithPendingPrerequisite} {
		if err := classicLifecycleOutcomeError(outcome); err != nil {
			t.Fatalf("%s returned %v", outcome, err)
		}
	}
	for _, outcome := range []corelifecycle.Outcome{corelifecycle.OutcomeBlocked, corelifecycle.OutcomePartiallyApplied, corelifecycle.OutcomeRolledBack, corelifecycle.OutcomeRecoveryRequired, corelifecycle.OutcomeUninstallIncomplete} {
		if err := classicLifecycleOutcomeError(outcome); !errors.Is(err, ErrClassicLifecycleIncomplete) {
			t.Fatalf("%s returned %v", outcome, err)
		}
	}
}

func TestDoctorCompositionAcceptsExplicitClaudeEvidence(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	observer := &countingClaudeObserver{}
	opts.ClaudeLookPath = func(string) (string, error) { return "/sandbox/bin/claude", nil }
	opts.ClaudeRunner = observer
	opts.ClaudeAuthorization = observer
	opts.ClaudeRuntimeEvidence = observer
	out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if observer.runnerCalls == 0 || observer.authorizationCalls == 0 || observer.runtimeCalls == 0 {
		t.Fatalf("evidence calls runner=%d authorization=%d runtime=%d", observer.runnerCalls, observer.authorizationCalls, observer.runtimeCalls)
	}
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

type countingEnv struct {
	values map[string]string
	calls  map[string]int
}

type changingSkillSourceEnv struct {
	MapEnv
	first string
	calls int
}

func (e *changingSkillSourceEnv) Getenv(key string) string {
	if key != "PACKY_SKILLS_SOURCE" {
		return e.MapEnv.Getenv(key)
	}
	e.calls++
	if e.calls == 1 {
		return e.first
	}
	return filepath.Join(e.MapEnv["HOME"], "missing-second-source")
}

func (e *countingEnv) Getenv(key string) string {
	e.calls[key]++
	return e.values[key]
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
			"PACKY_SKILLS_SOURCE": sourceRoot,
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
	engram := newCLITestFixture(t, opts).engram.ExpectedPath()
	expanded := make([]string, 0, len(calls))
	for _, call := range calls {
		expanded = append(expanded, strings.ReplaceAll(call, "<homebrew-engram>", engram))
	}
	return expanded
}

func engramSetupCallStrings(t *testing.T, opts Options) []string {
	t.Helper()
	engram := newCLITestFixture(t, opts).engram.ExpectedPath()
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

func createUnmanagedSkillSymlinks(t *testing.T, skills skillbundle.GlobalLayout, targetRoot string) {
	t.Helper()
	if err := os.MkdirAll(skills.Root(), 0o700); err != nil {
		t.Fatalf("mkdir agent skills: %v", err)
	}
	if err := os.MkdirAll(targetRoot, 0o700); err != nil {
		t.Fatalf("mkdir unmanaged target root: %v", err)
	}
	for _, name := range testSkillNames() {
		target := filepath.Join(targetRoot, name)
		if err := os.Symlink(target, skills.Skill(name)); err != nil {
			t.Fatalf("write unmanaged symlink %s: %v", name, err)
		}
	}
}

func installedSkillSourceRoot(home string) string {
	return skillbundle.SourceRoot(bootstrap.DefaultInstalledSourceRoot(home))
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
	previous := packyversion.Value
	packyversion.Value = value
	t.Cleanup(func() {
		packyversion.Value = previous
	})
}

func TestHelpRendersForRootAndV0Subcommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root", args: []string{"--help"}, want: []string{"Install and configure", "version", "init", "install", "doctor", "update", "uninstall"}},
		{name: "version", args: []string{"version", "--help"}, want: []string{"Print the Packy version"}},
		{name: "install", args: []string{"install", "--help"}, want: []string{"Install Packy-managed", "--dry-run"}},
		{name: "doctor", args: []string{"doctor", "--help"}, want: []string{"Check Packy setup"}},
		{name: "update", args: []string{"update", "--help"}, want: []string{"Refresh Packy-managed", "--dry-run"}},
		{name: "uninstall", args: []string{"uninstall", "--help"}, want: []string{"Remove only Packy-managed", "--dry-run"}},
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

			for _, args := range [][]string{{"--version"}, {"version"}} {
				out, err := executeCommand(t, NewRootCommand(opts), args...)
				if err != nil {
					t.Fatalf("version command %v failed: %v\n%s", args, err, out)
				}
				want := "packy version " + tt.version + "\n"
				if out != want {
					t.Fatalf("version output for %v = %q, want %q", args, out, want)
				}
			}
		})
	}
}

func TestHelpAndVersionDoNotResolveWorkstation(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"init", "--help"}, {"version", "--help"}, {"--version"}, {"version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env := &countingEnv{values: map[string]string{}, calls: map[string]int{}}
			getwdCalls := 0
			opts := Options{
				Env: env,
				Getwd: func() (string, error) {
					getwdCalls++
					return "", errors.New("must not capture cwd")
				},
			}
			if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
				t.Fatalf("command failed: %v\n%s", err, out)
			}
			if getwdCalls != 0 || len(env.calls) != 0 {
				t.Fatalf("workstation captured for %v: getwd=%d env=%v", args, getwdCalls, env.calls)
			}
		})
	}
}

func TestLifecycleCommandCapturesOneWorkstationSnapshot(t *testing.T) {
	home := t.TempDir()
	source := createSkillSource(t)
	getwdCalls := 0
	opts := Options{
		Env: MapEnv{
			"HOME":                home,
			"XDG_CONFIG_HOME":     filepath.Join(home, "xdg"),
			"PACKY_SKILLS_SOURCE": source,
		},
		Getwd: func() (string, error) {
			getwdCalls++
			return t.TempDir(), nil
		},
		Runner: &fakeRunner{},
	}

	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run: %v\n%s", err, out)
	}
	if getwdCalls != 1 {
		t.Fatalf("Getwd calls = %d, want one captured workstation", getwdCalls)
	}
}

func TestLifecycleCommandResolvesSkillSourceOnce(t *testing.T) {
	home := t.TempDir()
	source := createSkillSource(t)
	env := &changingSkillSourceEnv{
		MapEnv: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg")},
		first:  source,
	}
	opts := Options{Env: env, Getwd: func() (string, error) { return t.TempDir(), nil }, Runner: &fakeRunner{}}

	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run: %v\n%s", err, out)
	}
	if env.calls != 1 {
		t.Fatalf("PACKY_SKILLS_SOURCE reads = %d, want one", env.calls)
	}
	if !strings.Contains(out, "PACKY_SKILLS_SOURCE="+source) {
		t.Fatalf("source report did not use resolved source:\n%s", out)
	}
}

func TestCommandsResolveOwnerLayoutsFromInjectedEnvironment(t *testing.T) {
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
		"PACKY_STATE=" + filepath.Join(home, ".packy", "config.json"),
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
			for _, path := range []string{filepath.Join(home, ".packy"), filepath.Join(home, ".agents")} {
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
	fixture := newCLITestFixture(t, opts)
	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Skill source: repo checkout (" + fixture.skillSource.Root + ")",
		"warning: installed source also exists at " + installedSkillSourceRoot(home),
		"repo checkout source may create a development-mode install",
		"For package-installed setup, run packy install outside the repo or set PACKY_SKILLS_SOURCE explicitly.",
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
	sourceRoot := opts.Env.Getenv("PACKY_SKILLS_SOURCE")
	out, err := executeCommand(t, NewRootCommand(opts), "install", "--dry-run")
	if err != nil {
		t.Fatalf("install --dry-run failed: %v\n%s", err, out)
	}
	want := "Skill source: explicit override (PACKY_SKILLS_SOURCE=" + sourceRoot + ")"
	if !strings.Contains(out, want) {
		t.Fatalf("install --dry-run output missing %q:\n%s", want, out)
	}
}

func TestPackageInstalledCommandsUseInitializedSourceOutsideRepo(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
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
	fixture := newCLITestFixture(t, opts)
	if got, want := fixture.skillSource.Root, skillbundle.InstalledSourceRoot(fixture.installedSource); got != want {
		t.Fatalf("SkillSourceRoot = %q, want installed source %q", got, want)
	}
	if !exists(fixture.classicState.StateFile()) || !exists(fixture.skills.Skill("wayfinder")) {
		t.Fatalf("install did not create Packy-managed artifacts from installed source")
	}

	runner.calls = nil
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
	if exists(fixture.classicState.StateFile()) || exists(fixture.skills.Skill("wayfinder")) {
		t.Fatalf("uninstall left Packy-managed artifacts in sandbox")
	}
	if !exists(fixture.skillSource.Root) {
		t.Fatalf("uninstall should not remove Installed Source at %s", fixture.skillSource.Root)
	}
}

func TestPackageInstalledInstallSuggestsInitWhenSourceMissing(t *testing.T) {
	home := t.TempDir()
	chdirTempOutsideRepo(t)

	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}, Runner: &fakeRunner{path: map[string]string{"engram": "/fake/bin/engram"}}}
	missing := filepath.Join(home, ".local", "share", "packy", "bundle", "skills")
	for _, args := range [][]string{{"install", "--dry-run"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err == nil {
				t.Fatalf("expected missing Installed Source error, got output:\n%s", out)
			}
			for _, want := range []string{"run packy init", missing} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error missing %q: %v", want, err)
				}
			}
			if exists(filepath.Join(home, ".packy")) || exists(filepath.Join(home, ".agents")) {
				t.Fatalf("missing source command mutated sandbox")
			}
		})
	}
}

func TestPackageInstalledInstallRejectsMalformedSourceBeforeMutation(t *testing.T) {
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
	for _, args := range [][]string{{"install"}} {
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
	if !strings.Contains(out, "packy update dry-run: planned actions") || !strings.Contains(out, "run: update Engram via Homebrew") {
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
		"packy uninstall dry-run: planned actions",
		"remove: remove managed skill ask-matt",
		"remove-codex-prompt: remove Codex Packy prompt markers",
		"remove-opencode-prompt: remove OpenCode Packy prompt reference",
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

	fixture := newCLITestFixture(t, opts)
	engram := fixture.engram.ExpectedPath()
	wants := []string{
		"packy install dry-run: planned actions",
		"write-file: persist Packy state metadata",
		filepath.Join(home, ".packy", "config.json"),
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
	for _, path := range []string{filepath.Join(home, ".packy"), filepath.Join(home, ".agents")} {
		if exists(path) {
			t.Fatalf("dry-run unexpectedly created %s", path)
		}
	}
}

func TestInstallRejectsCorruptState(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	fixture := newCLITestFixture(t, opts)
	if err := os.MkdirAll(fixture.classicState.PackyHome(), 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(fixture.classicState.StateFile(), []byte("{not json"), 0o600); err != nil {
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

func TestInstallWarnsWhenMostExpectedSkillsAreUnmanagedSymlinks(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	fixture := newCLITestFixture(t, opts)
	createUnmanagedSkillSymlinks(t, fixture.skills, filepath.Join(home, "stale-repo-skills"))

	out, err := executeCommand(t, NewRootCommand(opts), "install")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	recoveryAdvice := "Safe recovery: verify these are stale Packy-created links, remove them, then run packy install; Packy will not overwrite arbitrary files or links."
	for _, want := range []string{
		"warning: skipped 6 unmanaged skill symlinks; setup may be incomplete",
		"Example: " + fixture.skills.Skill("ask-matt") + " -> " + filepath.Join(home, "stale-repo-skills", "ask-matt"),
		recoveryAdvice,
		"packy install: synced 0 managed skills",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("install output missing %q:\n%s", want, out)
		}
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
	fixture := newCLITestFixture(t, opts)
	for _, path := range []string{fixture.classicState.StateFile(), fixture.codex.PromptFile(), fixture.opencode.ConfigFile(), fixture.opencode.PromptFile(), fixture.skills.Root()} {
		if strings.HasPrefix(path, realHome+string(os.PathSeparator)) {
			t.Fatalf("sandbox path %q points inside real HOME %q", path, realHome)
		}
	}

	if err := os.MkdirAll(filepath.Dir(fixture.codex.PromptFile()), 0o700); err != nil {
		t.Fatalf("mkdir codex config: %v", err)
	}
	codexOriginal := "# Existing Codex\n\n<!-- gentle-ai:persona -->\nkeep gentle codex\n<!-- /gentle-ai:persona -->\n"
	if err := os.WriteFile(fixture.codex.PromptFile(), []byte(codexOriginal), 0o600); err != nil {
		t.Fatalf("write codex fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(fixture.opencode.ConfigFile()), 0o700); err != nil {
		t.Fatalf("mkdir opencode config: %v", err)
	}
	openCodeOriginal := `{"plugin":["gentle-ai"],"instructions":["CONTRIBUTING.md"]}` + "\n"
	if err := os.WriteFile(fixture.opencode.ConfigFile(), []byte(openCodeOriginal), 0o600); err != nil {
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
	if !exists(fixture.classicState.StateFile()) || !exists(fixture.skills.Skill("ask-matt")) {
		t.Fatalf("install did not create expected Packy-managed artifacts in sandbox")
	}

	runner.calls = nil
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
	if exists(fixture.classicState.StateFile()) || exists(fixture.skills.Skill("ask-matt")) || exists(fixture.opencode.PromptFile()) {
		t.Fatalf("uninstall left Packy-managed artifacts in sandbox")
	}
	if got := readFileString(t, fixture.codex.PromptFile()); got != codexOriginal {
		t.Fatalf("uninstall did not restore Codex user/Gentle AI content:\ngot:\n%s\nwant:\n%s", got, codexOriginal)
	}
	openCodeAfter := readFileString(t, fixture.opencode.ConfigFile())
	for _, want := range []string{"gentle-ai", "CONTRIBUTING.md"} {
		if !strings.Contains(openCodeAfter, want) {
			t.Fatalf("uninstall lost OpenCode user/Gentle AI content %q:\n%s", want, openCodeAfter)
		}
	}
	if strings.Contains(openCodeAfter, fixture.opencode.PromptFile()) {
		t.Fatalf("uninstall left Packy OpenCode prompt reference:\n%s", openCodeAfter)
	}
}

func TestPackyLifecycleIgnoresAndPreservesLegacyMattyOwnership(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	env := opts.Env.(MapEnv)
	packySource := env["PACKY_SKILLS_SOURCE"]
	legacySource := createSkillSource(t)
	env["MATTY_SKILLS_SOURCE"] = legacySource

	legacyState := filepath.Join(home, ".matty", "config.json")
	legacyPackState := filepath.Join(home, ".matty", "packs.json")
	legacyPackLock := filepath.Join(home, ".matty", "packs.lock")
	legacyCheckout := filepath.Join(home, ".local", "share", "matty", "sentinel")
	legacyPrompt := filepath.Join(home, "xdg-config", "opencode", "matty.md")
	legacyCodexBlock := "<!-- matty:skills-router -->\nlegacy Matty instructions\n<!-- /matty:skills-router -->\n" +
		"<!-- matty:pack:matty-guidance:start -->\nlegacy Matty pack projection\n<!-- matty:pack:matty-guidance:end -->\n"
	legacyOpenCodeConfig := "{\n  \"instructions\": [\"" + legacyPrompt + "\"]\n}\n"
	for path, content := range map[string]string{
		legacyState:     "{not valid Packy state",
		legacyPackState: "legacy pack state must survive\n",
		legacyPackLock:  "legacy pack lock must survive\n",
		legacyCheckout:  "legacy checkout must survive\n",
		legacyPrompt:    "legacy prompt must survive\n",
		filepath.Join(home, ".codex", "AGENTS.md"):                     legacyCodexBlock,
		filepath.Join(home, "xdg-config", "opencode", "opencode.json"): legacyOpenCodeConfig,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	legacyLinkTarget := filepath.Join(home, ".local", "share", "matty", "bundle", "skills", "legacy-matty")
	legacyLink := filepath.Join(home, ".agents", "skills", "legacy-matty")
	if err := os.MkdirAll(legacyLinkTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyLink), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(legacyLinkTarget, legacyLink); err != nil {
		t.Fatal(err)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err != nil {
		t.Fatalf("Packy install adopted legacy Matty state: %v\n%s", err, out)
	}
	packyState := filepath.Join(home, ".packy", "config.json")
	data, err := os.ReadFile(packyState)
	if err != nil || !strings.Contains(string(data), `"packy_version"`) || strings.Contains(string(data), `"matty_version"`) {
		t.Fatalf("Packy state = %q, %v", data, err)
	}
	state, found, err := corelifecycle.LoadState(packyState)
	if err != nil || !found {
		t.Fatalf("load Packy state: found=%v err=%v", found, err)
	}
	for _, skill := range state.ManagedSkills {
		if !strings.HasPrefix(skill.SourcePath, packySource+string(filepath.Separator)) || strings.HasPrefix(skill.SourcePath, legacySource+string(filepath.Separator)) {
			t.Fatalf("Packy adopted legacy MATTY_SKILLS_SOURCE: %+v", skill)
		}
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "uninstall"); err != nil {
		t.Fatalf("Packy uninstall touched legacy Matty ownership: %v\n%s", err, out)
	}

	for path, want := range map[string]string{
		legacyState:     "{not valid Packy state",
		legacyPackState: "legacy pack state must survive\n",
		legacyPackLock:  "legacy pack lock must survive\n",
		legacyCheckout:  "legacy checkout must survive\n",
		legacyPrompt:    "legacy prompt must survive\n",
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("legacy artifact %s = %q, %v; want preserved", path, got, err)
		}
	}
	codexContent := readFileString(t, filepath.Join(home, ".codex", "AGENTS.md"))
	if !strings.Contains(codexContent, legacyCodexBlock) || strings.Contains(codexContent, "packy:skills-router") {
		t.Fatalf("legacy Codex ownership was changed or Packy ownership survived uninstall:\n%s", codexContent)
	}
	openCodeContent := readFileString(t, filepath.Join(home, "xdg-config", "opencode", "opencode.json"))
	if !strings.Contains(openCodeContent, legacyPrompt) || strings.Contains(openCodeContent, "packy.md") {
		t.Fatalf("legacy OpenCode ownership was changed or Packy ownership survived uninstall:\n%s", openCodeContent)
	}
	if exists(packyState) {
		t.Fatalf("Packy state survived uninstall at %s", packyState)
	}
	if target, err := os.Readlink(legacyLink); err != nil || target != legacyLinkTarget {
		t.Fatalf("legacy Matty link changed: target=%q err=%v", target, err)
	}
}

func TestInstallDryRunReportsUnmanagedSkipsWithoutMutating(t *testing.T) {
	opts, _, _ := sandboxOptions(t)
	fixture := newCLITestFixture(t, opts)
	if err := os.MkdirAll(fixture.skills.Root(), 0o700); err != nil {
		t.Fatalf("mkdir agent skills: %v", err)
	}
	unmanaged := fixture.skills.Skill("ask-matt")
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
	if exists(fixture.classicState.StateFile()) {
		t.Fatalf("dry-run wrote state")
	}
}

func TestInterruptedInstallIsExplicitAndPersistsRecoveryState(t *testing.T) {
	opts, runner, _ := sandboxOptions(t)
	fixture := newCLITestFixture(t, opts)
	runner.fail = map[string]error{fixture.engram.ExpectedPath() + " setup codex": errors.New("interrupted")}

	if out, err := executeCommand(t, NewRootCommand(opts), "install"); err == nil {
		t.Fatalf("install unexpectedly succeeded:\n%s", out)
	}
	state, found, err := corelifecycle.LoadState(fixture.classicState.StateFile())
	if err != nil || !found {
		t.Fatalf("LoadState = found %v err %v", found, err)
	}
	if !state.RecoveryRequired() {
		t.Fatalf("install status = %q, want recovery-required", state.InstallStatus)
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
	repo := createPackySourceRepo(t)
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
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

func TestInitCapturesOneWorkstationSnapshot(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	env := &countingEnv{
		values: map[string]string{
			"HOME":            home,
			"XDG_CONFIG_HOME": filepath.Join(home, "xdg-config"),
			"PATH":            "/sandbox/bin",
			"HOMEBREW_PREFIX": "/sandbox/homebrew",
		},
		calls: map[string]int{},
	}
	getwdCalls := 0
	opts := Options{
		Env: env,
		Getwd: func() (string, error) {
			getwdCalls++
			return t.TempDir(), nil
		},
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if getwdCalls != 1 {
		t.Fatalf("getwd calls = %d; want 1", getwdCalls)
	}
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "PATH", "HOMEBREW_PREFIX"} {
		if env.calls[key] != 1 {
			t.Fatalf("%s calls = %d; want 1 (all calls: %v)", key, env.calls[key], env.calls)
		}
	}
}

func TestInitPreservesMissingHomeError(t *testing.T) {
	out, err := executeCommand(t, NewRootCommand(Options{
		Env:   MapEnv{},
		Getwd: func() (string, error) { return "", errors.New("cwd unavailable") },
	}), "init")
	if err == nil || err.Error() != "HOME is required" {
		t.Fatalf("error = %v; want HOME is required\n%s", err, out)
	}
}

func TestInitWithAbsoluteSourceDoesNotRequireCurrentDirectory(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	sourceRoot := filepath.Join(t.TempDir(), "installed")
	opts := Options{
		Env:   MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")},
		Getwd: func() (string, error) { return "", errors.New("cwd unavailable") },
	}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--source-root", sourceRoot, "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(sourceRoot, "bundle", "skills")) {
		t.Fatalf("init did not use absolute source root")
	}
}

func TestInitRejectsMalformedExistingInstalledSourceWithoutMutation(t *testing.T) {
	home := t.TempDir()
	installedRoot := bootstrap.DefaultInstalledSourceRoot(home)
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
	for _, want := range []string{"not a valid Packy checkout", "Move it aside", "--source-root"} {
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
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "malformed")
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
	if exists(bootstrap.DefaultInstalledSourceRoot(home)) {
		t.Fatalf("init published malformed source at %s", bootstrap.DefaultInstalledSourceRoot(home))
	}
}

func TestInitDoesNotReplaceValidInstalledSourceWithMalformedRef(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.1.0")
	if err := os.RemoveAll(filepath.Join(repo, "bundle", "skills", "productivity")); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repo, "add", "-A")
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "malformed")
	runGitCommand(t, repo, "tag", "v0.2.0")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}
	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.1.0"); err != nil {
		t.Fatalf("initialize valid source: %v\n%s", err, out)
	}
	installedRoot := bootstrap.DefaultInstalledSourceRoot(home)
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
	repo := createPackySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.1.0")
	if err := os.WriteFile(filepath.Join(repo, "UPDATED"), []byte("updated"), 0o600); err != nil {
		t.Fatalf("write update fixture: %v", err)
	}
	runGitCommand(t, repo, "add", "UPDATED")
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "updated")
	runGitCommand(t, repo, "tag", "v0.2.0")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	if out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.1.0"); err != nil {
		t.Fatalf("initial init failed: %v\n%s", err, out)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo, "--repository-ref", "v0.2.0")
	if err != nil {
		t.Fatalf("update init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	for _, want := range []string{"updating Installed Source at " + sourceRoot + " to v0.2.0", "updated Installed Source"} {
		if !strings.Contains(out, want) {
			t.Fatalf("update output missing %q:\n%s", want, out)
		}
	}
}

func TestInitReportsProgressAndGitContextWhenCloneFails(t *testing.T) {
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", filepath.Join(t.TempDir(), "missing-repository"))
	if err == nil {
		t.Fatalf("expected clone failure, got output:\n%s", out)
	}
	if !strings.Contains(out, "cloning Installed Source into "+sourceRoot) {
		t.Fatalf("clone failure output did not include progress:\n%s", out)
	}
	for _, want := range []string{"clone Packy Source of Truth", "git clone", "failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("clone failure missing %q: %v", want, err)
		}
	}
}

func TestInitSupportsHomeFlag(t *testing.T) {
	envHome := t.TempDir()
	flagHome := t.TempDir()
	repo := createPackySourceRepo(t)
	opts := Options{Env: MapEnv{"HOME": envHome, "XDG_CONFIG_HOME": filepath.Join(flagHome, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--home", flagHome, "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(flagHome, ".local", "share", "packy", "bundle", "skills")) {
		t.Fatalf("init did not use --home for default Installed Source")
	}
	if exists(filepath.Join(envHome, ".local", "share", "packy")) {
		t.Fatalf("init --home unexpectedly wrote Env HOME")
	}
}

func TestInitSupportsExplicitSourceRoot(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	sourceRoot := filepath.Join(t.TempDir(), "custom-source")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--source-root", sourceRoot, "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(sourceRoot, "bundle", "skills")) {
		t.Fatalf("init did not clone into explicit source root")
	}
	if exists(filepath.Join(home, ".local", "share", "packy")) {
		t.Fatalf("init with --source-root unexpectedly wrote default Installed Source")
	}
}

func TestInitNormalizesRelativeSourceRootFromCapturedDirectory(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	repo := createPackySourceRepo(t)
	opts := Options{
		Env:   MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")},
		Getwd: func() (string, error) { return cwd, nil },
	}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--source-root", filepath.Join("relative", "source"), "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !exists(filepath.Join(cwd, "relative", "source", "bundle", "skills")) {
		t.Fatalf("init did not resolve relative --source-root from captured cwd")
	}
}

func TestInitRejectsInvalidNonEmptyDestination(t *testing.T) {
	home := t.TempDir()
	repo := createPackySourceRepo(t)
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "README.md"), []byte("not packy"), 0o600); err != nil {
		t.Fatalf("write invalid destination: %v", err)
	}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err == nil {
		t.Fatalf("expected invalid destination error, got output:\n%s", out)
	}
	for _, want := range []string{"not a valid Packy checkout", "Move it aside", "--source-root"} {
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
	repo := createPackySourceRepo(t)
	runGitCommand(t, repo, "tag", "v0.2.3")
	runGitCommand(t, repo, "checkout", "-b", "next")
	if err := os.WriteFile(filepath.Join(repo, "UNRELEASED"), []byte("main only"), 0o600); err != nil {
		t.Fatalf("write unreleased file: %v", err)
	}
	runGitCommand(t, repo, "add", "UNRELEASED")
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "unreleased")
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg-config")}}

	out, err := executeCommand(t, NewRootCommand(opts), "init", "--repository-url", repo)
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	sourceRoot := filepath.Join(home, ".local", "share", "packy")
	if exists(filepath.Join(sourceRoot, "UNRELEASED")) {
		t.Fatalf("release init cloned repository HEAD instead of release tag")
	}
	got := strings.TrimSpace(runGitCommand(t, sourceRoot, "rev-parse", "HEAD"))
	want := strings.TrimSpace(runGitCommand(t, repo, "rev-parse", "v0.2.3^{commit}"))
	if got != want {
		t.Fatalf("cloned HEAD = %s, want release tag commit %s", got, want)
	}
}

func createPackySourceRepo(t *testing.T) string {
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
	runGitCommand(t, repo, "-c", "user.name=Packy Test", "-c", "user.email=packy@example.test", "commit", "-m", "initial")
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

func writeEngramExecutable(t *testing.T, dir, versionOutput string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir executable dir: %v", err)
	}
	path := filepath.Join(dir, "engram")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo '" + versionOutput + "'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}
