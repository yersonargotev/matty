package setuphealth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/codex"
	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/skillbundle"
	"github.com/yersonargotev/matty/internal/workstation"
)

func TestDiagnoseReturnsCompleteOrderedReadOnlyReportAfterPartialFailures(t *testing.T) {
	config := sandboxConfig(t)
	canonical := writeExecutable(t, filepath.Join(config.HomebrewPrefix, "bin", "engram"))
	lookup := &lookupStub{err: errors.New("lookup failed")}
	facts := engrambin.Facts{
		Version: func(string) (string, error) { t.Fatal("version observed without executable"); return "", nil },
		ServeProcesses: func() ([]engrambin.Process, error) {
			return nil, errors.New("process inspection failed")
		},
	}
	before := snapshot(t, config.HomeDir)

	report := diagnose(config, lookup, facts)

	want := []Check{
		{Severity: Warn, Name: "matty-state", Detail: "missing at " + config.StateFile + "; run matty install"},
		{Severity: Warn, Name: "skill-symlinks", Detail: "state is missing, so Matty-owned skill links are unknown; run matty install"},
		{Severity: Fail, Name: "engram-binary", Detail: "engram is not available on PATH; Homebrew Engram exists at " + canonical + "; add it to PATH or run matty install"},
		{Severity: Pass, Name: "engram-local-bin", Detail: "no ~/.local/bin/engram compatibility symlink is present; Homebrew remains the Engram owner"},
		{Severity: Warn, Name: "engram-runtime", Detail: "could not inspect active engram serve processes: process inspection failed"},
		{Severity: Warn, Name: "engram-setup", Detail: "state is missing, so delegated setup cannot be confirmed; run matty install"},
		{Severity: Warn, Name: "codex-config", Detail: "missing Matty Codex prompt markers at " + config.CodexPromptFile + "; run matty install"},
		{Severity: Warn, Name: "opencode-config", Detail: "missing OpenCode config; run matty install"},
	}
	if !reflect.DeepEqual(report.Checks, want) {
		t.Fatalf("checks changed:\ngot:  %#v\nwant: %#v", report.Checks, want)
	}
	wantContext := Context{HomeDir: config.HomeDir, ConfigHome: config.ConfigHome, StateFile: config.StateFile, StateStatus: "missing", AgentSkillsDir: config.AgentSkillsDir}
	if report.SchemaVersion != 1 || report.Kind != "doctor" || report.Context != wantContext {
		t.Fatalf("report metadata = %#v", report)
	}
	if report.Summary != (Summary{Status: "failures", Passes: 1, Warnings: 6, Failures: 1}) {
		t.Fatalf("summary = %#v", report.Summary)
	}
	if got := snapshot(t, config.HomeDir); got != before {
		t.Fatalf("diagnosis mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, got)
	}
	if !reflect.DeepEqual(lookup.calls, []string{"engram", "engram"}) {
		t.Fatalf("lookup calls = %#v", lookup.calls)
	}
}

func TestDiagnoseHealthySetupReport(t *testing.T) {
	config := sandboxConfig(t)
	canonical := writeExecutable(t, filepath.Join(config.HomebrewPrefix, "bin", "engram"))
	source := filepath.Join(config.HomeDir, "source", "skill")
	link := filepath.Join(config.AgentSkillsDir, "skill")
	writeFile(t, filepath.Join(source, "SKILL.md"), "skill")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	saveState(t, config, desiredState(config, []corelifecycle.ManagedSkill{{Name: "skill", SourcePath: source, LinkPath: link}}))
	writeFile(t, config.CodexPromptFile, "<!-- matty:skills-router -->\n<!-- /matty:skills-router -->")
	writeFile(t, config.OpenCodePromptFile, "prompt")
	writeFile(t, config.OpenCodeConfigFile, fmt.Sprintf(`{"instructions":[%q]}`, config.OpenCodePromptFile))

	report := diagnose(config, &lookupStub{path: canonical}, facts("1.19.0", nil, nil))

	if report.Summary != (Summary{Status: "healthy", Passes: 8}) {
		t.Fatalf("summary = %#v", report.Summary)
	}
	wantNames := []string{"matty-state", "skill-symlinks", "engram-binary", "engram-local-bin", "engram-runtime", "engram-setup", "codex-config", "opencode-config"}
	gotNames := make([]string, 0, len(report.Checks))
	for _, check := range report.Checks {
		gotNames = append(gotNames, check.Name)
		if check.Severity != Pass {
			t.Fatalf("non-PASS healthy check: %#v", check)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("check order = %#v, want %#v", gotNames, wantNames)
	}
}

func TestDiagnoseStateAndSkillSemanticMatrix(t *testing.T) {
	t.Run("corrupt state", func(t *testing.T) {
		config := sandboxConfig(t)
		writeFile(t, config.StateFile, "{not json")
		report := diagnoseWithoutEngram(t, config)
		assertCheck(t, report, Fail, "matty-state", "invalid JSON")
		assertCheck(t, report, Warn, "skill-symlinks", "state is missing")
		assertSummaryMatchesChecks(t, report)
	})

	t.Run("recovery required reuses recorded ownership", func(t *testing.T) {
		config := sandboxConfig(t)
		source := filepath.Join(config.HomeDir, "source", "skill")
		link := filepath.Join(config.AgentSkillsDir, "skill")
		writeFile(t, filepath.Join(source, "SKILL.md"), "skill")
		if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(source, link); err != nil {
			t.Fatal(err)
		}
		state := desiredState(config, []corelifecycle.ManagedSkill{{Name: "skill", SourcePath: source, LinkPath: link}})
		state.InstallStatus = corelifecycle.InstallRecoveryRequired
		saveState(t, config, state)
		report := diagnoseWithoutEngram(t, config)
		assertCheck(t, report, Fail, "matty-state", "installation was interrupted")
		assertCheck(t, report, Pass, "skill-symlinks", "1 managed links")
		if report.Context.StateStatus != "present" {
			t.Fatalf("state status = %q", report.Context.StateStatus)
		}
	})

	for _, tt := range []struct {
		name       string
		makeLink   func(t *testing.T, source, link string)
		severity   Severity
		detailPart string
	}{
		{name: "managed", makeLink: func(t *testing.T, source, link string) {
			if err := os.Symlink(source, link); err != nil {
				t.Fatal(err)
			}
		}, severity: Pass, detailPart: "1 managed links"},
		{name: "missing", makeLink: func(*testing.T, string, string) {}, severity: Fail, detailPart: "missing: skill"},
		{name: "changed symlink", makeLink: func(t *testing.T, _, link string) {
			if err := os.Symlink(filepath.Join(filepath.Dir(link), "other"), link); err != nil {
				t.Fatal(err)
			}
		}, severity: Fail, detailPart: "changed: skill"},
		{name: "unmanaged path", makeLink: func(t *testing.T, _, link string) { writeFile(t, link, "keep") }, severity: Fail, detailPart: "skill is not a symlink"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := sandboxConfig(t)
			source := filepath.Join(config.HomeDir, "source", "skill")
			link := filepath.Join(config.AgentSkillsDir, "skill")
			writeFile(t, filepath.Join(source, "SKILL.md"), "skill")
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			tt.makeLink(t, source, link)
			saveState(t, config, desiredState(config, []corelifecycle.ManagedSkill{{Name: "skill", SourcePath: source, LinkPath: link}}))
			assertCheck(t, diagnoseWithoutEngram(t, config), tt.severity, "skill-symlinks", tt.detailPart)
		})
	}

	t.Run("zero recorded skills and mostly unmanaged expected links", func(t *testing.T) {
		config := sandboxConfig(t)
		createSkillSource(t, config.SkillSourceRoot)
		link := filepath.Join(config.AgentSkillsDir, "loop-me")
		if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(config.HomeDir, "stale", "loop-me"), link); err != nil {
			t.Fatal(err)
		}
		saveState(t, config, desiredState(config, nil))
		assertCheck(t, diagnoseWithoutEngram(t, config), Warn, "skill-symlinks", "expected skill symlinks are unmanaged")
	})

	t.Run("zero recorded skills without unmanaged links", func(t *testing.T) {
		config := sandboxConfig(t)
		createSkillSource(t, config.SkillSourceRoot)
		saveState(t, config, desiredState(config, nil))
		assertExactCheck(t, diagnoseWithoutEngram(t, config), Check{Severity: Warn, Name: "skill-symlinks", Detail: "state has no managed skills; run matty install"})
	})
}

func TestDiagnoseEngramSemanticMatrix(t *testing.T) {
	t.Run("absent from PATH without canonical installation", func(t *testing.T) {
		config := sandboxConfig(t)
		report := diagnoseWithoutEngram(t, config)
		assertExactCheck(t, report, Check{Severity: Fail, Name: "engram-binary", Detail: "engram is not available on PATH; run matty install"})
	})

	t.Run("canonical binary no runtime", func(t *testing.T) {
		config := sandboxConfig(t)
		canonical := writeExecutable(t, filepath.Join(config.HomebrewPrefix, "bin", "engram"))
		report := diagnose(config, &lookupStub{path: canonical}, facts("1.19.0", nil, nil))
		assertCheck(t, report, Pass, "engram-binary", canonical+" version 1.19.0")
		assertCheck(t, report, Pass, "engram-local-bin", "no ~/.local/bin/engram")
		assertCheck(t, report, Pass, "engram-runtime", "no active")
	})

	t.Run("noncanonical shadowing version mismatch and inspection failure", func(t *testing.T) {
		config := sandboxConfig(t)
		local := writeExecutable(t, filepath.Join(config.HomeDir, "bin", "engram"))
		canonical := writeExecutable(t, filepath.Join(config.HomebrewPrefix, "bin", "engram"))
		config.PathEnv = strings.Join([]string{filepath.Dir(local), filepath.Dir(canonical)}, string(os.PathListSeparator))
		f := engrambin.Facts{
			Version: func(path string) (string, error) {
				if path == local {
					return "1.18.0", nil
				}
				return "", errors.New("version failed")
			},
			ServeProcesses: func() ([]engrambin.Process, error) { return nil, nil },
		}
		report := diagnose(config, &lookupStub{path: local}, f)
		assertCheck(t, report, Warn, "engram-binary", "non-Homebrew")
		assertCheck(t, report, Warn, "engram-version", "version failed")
		assertCheck(t, report, Warn, "engram-path-shadowing", local+" appears before Homebrew Engram at "+canonical)
		assertNoCheck(t, report, "engram-version-mismatch")
		assertCheckNames(t, report, []string{"matty-state", "skill-symlinks", "engram-binary", "engram-version", "engram-path-shadowing", "engram-local-bin", "engram-runtime", "engram-setup", "codex-config", "opencode-config"})

		f.Version = func(path string) (string, error) {
			if path == local {
				return "1.18.0", nil
			}
			return "1.19.0", nil
		}
		report = diagnose(config, &lookupStub{path: local}, f)
		assertCheck(t, report, Warn, "engram-version-mismatch", "1.18.0")
	})

	t.Run("local compatibility link and binary", func(t *testing.T) {
		config := sandboxConfig(t)
		canonical := writeExecutable(t, filepath.Join(config.HomebrewPrefix, "bin", "engram"))
		if err := os.MkdirAll(filepath.Dir(config.LocalBinEngram), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(canonical, config.LocalBinEngram); err != nil {
			t.Fatal(err)
		}
		report := diagnose(config, &lookupStub{path: config.LocalBinEngram}, facts("1.19.0", nil, nil))
		assertCheck(t, report, Pass, "engram-local-bin", "points to Homebrew Engram")
		if err := os.Remove(config.LocalBinEngram); err != nil {
			t.Fatal(err)
		}
		writeExecutable(t, config.LocalBinEngram)
		report = diagnose(config, &lookupStub{path: canonical}, facts("1.19.0", nil, nil))
		assertExactCheck(t, report, Check{Severity: Warn, Name: "engram-local-bin", Detail: config.LocalBinEngram + " exists but is not a symlink; Matty will not install a second Engram binary there"})
	})

	t.Run("stale local compatibility symlink", func(t *testing.T) {
		config := sandboxConfig(t)
		canonical := writeExecutable(t, filepath.Join(config.HomebrewPrefix, "bin", "engram"))
		other := filepath.Join(config.HomeDir, "other", "engram")
		if err := os.MkdirAll(filepath.Dir(config.LocalBinEngram), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other, config.LocalBinEngram); err != nil {
			t.Fatal(err)
		}
		report := diagnose(config, &lookupStub{path: canonical}, facts("1.19.0", nil, nil))
		assertExactCheck(t, report, Check{Severity: Warn, Name: "engram-local-bin", Detail: config.LocalBinEngram + " -> " + other + " does not point to Homebrew Engram at " + canonical + "; replace it with a symlink if PATH compatibility is needed"})
	})

	t.Run("matching and mismatched runtime", func(t *testing.T) {
		config := sandboxConfig(t)
		canonical := writeExecutable(t, filepath.Join(config.HomebrewPrefix, "bin", "engram"))
		matching := []engrambin.Process{{PID: 10, ExecutablePath: canonical, Command: canonical + " serve"}}
		report := diagnose(config, &lookupStub{path: canonical}, facts("1.19.0", matching, nil))
		assertCheck(t, report, Pass, "engram-runtime", "matches PATH and canonical")
		other := writeExecutable(t, filepath.Join(config.HomeDir, "other", "engram"))
		mismatched := []engrambin.Process{{PID: 11, ExecutablePath: other, Command: other + " serve"}}
		report = diagnose(config, &lookupStub{path: canonical}, facts("1.19.0", mismatched, nil))
		assertExactCheck(t, report, Check{Severity: Warn, Name: "engram-runtime", Detail: "pid 11 running " + other + "; different from PATH Engram " + canonical + "; does not match canonical Homebrew Engram " + canonical + "; safe remediation: pkill -f 'engram serve' && " + canonical + " serve"})
	})
}

func TestDiagnoseDelegatedSetupCodexAndOpenCodeMatrix(t *testing.T) {
	config := sandboxConfig(t)
	state := desiredState(config, nil)
	state.ConfiguredSurfaces = []string{"codex"}
	saveState(t, config, state)
	report := diagnoseWithoutEngram(t, config)
	assertCheck(t, report, Fail, "engram-setup", "does not record both")

	state.ConfiguredSurfaces = []string{"codex", "opencode"}
	saveState(t, config, state)
	writeFile(t, config.CodexPromptFile, "<!-- matty:skills-router -->\n<!-- /matty:skills-router -->\n<!-- gentle-ai:persona -->x<!-- /gentle-ai:persona -->")
	writeFile(t, config.OpenCodePromptFile, "prompt")
	writeFile(t, config.OpenCodeConfigFile, fmt.Sprintf(`{"instructions":[%q],"plugin":["gentle-ai"]}`, config.OpenCodePromptFile))
	report = diagnoseWithoutEngram(t, config)
	assertCheck(t, report, Pass, "engram-setup", "records Codex and OpenCode")
	assertCheck(t, report, Pass, "codex-config", "markers are present")
	assertCheck(t, report, Warn, "codex-conflict", "gentle-ai")
	assertCheck(t, report, Pass, "opencode-config", "reference and prompt file are present")
	assertCheck(t, report, Warn, "opencode-conflict", "gentle-ai")

	writeFile(t, config.CodexPromptFile, "<!-- matty:skills-router -->")
	report = diagnoseWithoutEngram(t, config)
	assertExactCheck(t, report, Check{Severity: Warn, Name: "codex-config", Detail: "Matty prompt markers are missing; run matty install"})

	for _, tt := range []struct {
		name, configBody, prompt, detail string
		severity                         Severity
	}{
		{name: "missing config", detail: "missing OpenCode config", severity: Warn},
		{name: "missing reference", configBody: `{}`, detail: "instruction reference is missing", severity: Warn},
		{name: "missing prompt", configBody: fmt.Sprintf(`{"instructions":[%q]}`, config.OpenCodePromptFile), detail: "prompt file is missing", severity: Warn},
		{name: "malformed jsonc", configBody: `{"instructions":`, detail: "invalid JSONC", severity: Fail},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := sandboxConfig(t)
			if tt.configBody != "" {
				writeFile(t, c.OpenCodeConfigFile, strings.ReplaceAll(tt.configBody, config.OpenCodePromptFile, c.OpenCodePromptFile))
			}
			if tt.prompt != "" {
				writeFile(t, c.OpenCodePromptFile, tt.prompt)
			}
			assertCheck(t, diagnoseWithoutEngram(t, c), tt.severity, "opencode-config", tt.detail)
		})
	}

	t.Run("codex unreadable path is failure", func(t *testing.T) {
		c := sandboxConfig(t)
		if err := os.MkdirAll(c.CodexPromptFile, 0o700); err != nil {
			t.Fatal(err)
		}
		assertCheck(t, diagnoseWithoutEngram(t, c), Fail, "codex-config", "cannot read")
	})

	t.Run("opencode unreadable path is failure", func(t *testing.T) {
		c := sandboxConfig(t)
		if err := os.MkdirAll(c.OpenCodeConfigFile, 0o700); err != nil {
			t.Fatal(err)
		}
		assertCheck(t, diagnoseWithoutEngram(t, c), Fail, "opencode-config", "is a directory; inspect the config or run matty install")
	})
}

type lookupStub struct {
	path  string
	err   error
	calls []string
}

func (lookup *lookupStub) LookPath(name string) (string, error) {
	lookup.calls = append(lookup.calls, name)
	if lookup.err != nil {
		return "", lookup.err
	}
	return lookup.path, nil
}

type setupFixture struct {
	HomeDir, ConfigHome, StateFile, AgentSkillsDir          string
	SkillSourceRoot, SkillSourceMissingHint                 string
	CodexPromptFile, OpenCodeConfigFile, OpenCodePromptFile string
	PathEnv, LocalBinEngram, HomebrewPrefix                 string
	state                                                   corelifecycle.Layout
	skills                                                  skillbundle.GlobalLayout
	source                                                  skillbundle.Source
	codex                                                   codex.CanonicalLayout
	openCode                                                opencode.CanonicalLayout
	engram                                                  engrambin.SetupLayout
}

func sandboxConfig(t *testing.T) setupFixture {
	t.Helper()
	home := t.TempDir()
	configHome := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	pathEnv := filepath.Join(home, "bin")
	homebrewPrefix := filepath.Join(home, "homebrew")
	snapshot, err := workstation.Resolve(workstation.Inputs{Home: home, ConfigurationHome: configHome, ExecutableSearchPath: pathEnv, HomebrewPrefix: homebrewPrefix}, workstation.Options{})
	if err != nil {
		t.Fatal(err)
	}
	state := corelifecycle.NewLayout(snapshot.MattyHome())
	skills := skillbundle.NewGlobalLayout(snapshot.Home())
	source := skillbundle.Source{Root: filepath.Join(home, "bundle", "skills"), MissingHint: "run matty init to initialize it"}
	codexLayout := codex.NewCanonicalLayout(snapshot.Home())
	openCode := opencode.NewCanonicalLayout(snapshot.ConfigurationHome())
	engram := engrambin.NewSetupLayout(snapshot.Home(), snapshot.HomebrewPrefix())
	return setupFixture{
		HomeDir: snapshot.Home(), ConfigHome: snapshot.ConfigurationHome(),
		StateFile: state.StateFile(), AgentSkillsDir: skills.Root(),
		SkillSourceRoot: source.Root, SkillSourceMissingHint: source.MissingHint,
		CodexPromptFile:    codexLayout.PromptFile(),
		OpenCodeConfigFile: openCode.ConfigFile(), OpenCodePromptFile: openCode.PromptFile(),
		PathEnv: snapshot.ExecutableSearchPath(), LocalBinEngram: engram.LocalBin(), HomebrewPrefix: snapshot.HomebrewPrefix(),
		state: state, skills: skills, source: source, codex: codexLayout, openCode: openCode, engram: engram,
	}
}

func diagnose(config setupFixture, lookup *lookupStub, facts engrambin.Facts) Report {
	return Diagnose(
		config.HomeDir,
		config.ConfigHome,
		corelifecycle.ObserveSetup(config.state, config.skills, config.source),
		engrambin.ObserveSetup(config.engram, config.PathEnv, lookup.LookPath, facts),
		codex.ObserveSetup(config.codex),
		opencode.ObserveSetup(config.openCode),
	)
}

func diagnoseWithoutEngram(t *testing.T, config setupFixture) Report {
	t.Helper()
	return diagnose(config, &lookupStub{err: errors.New("not found")}, facts("", nil, nil))
}

func facts(version string, processes []engrambin.Process, processErr error) engrambin.Facts {
	return engrambin.Facts{
		Version:        func(string) (string, error) { return version, nil },
		ServeProcesses: func() ([]engrambin.Process, error) { return processes, processErr },
	}
}

func desiredState(config setupFixture, skills []corelifecycle.ManagedSkill) corelifecycle.State {
	return corelifecycle.DesiredState(corelifecycle.StateConfig{StateFile: config.StateFile, AgentSkillsDir: config.AgentSkillsDir}, time.Unix(1, 0), skills)
}

func saveState(t *testing.T, config setupFixture, state corelifecycle.State) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(config.StateFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := corelifecycle.SaveState(config.StateFile, state); err != nil {
		t.Fatal(err)
	}
}

func createSkillSource(t *testing.T, root string) {
	t.Helper()
	for _, group := range []string{"engineering", "productivity"} {
		if err := os.MkdirAll(filepath.Join(root, group), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, "in-progress", "loop-me", "SKILL.md"), "skill")
}

func writeExecutable(t *testing.T, path string) string {
	t.Helper()
	writeFile(t, path, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertCheck(t *testing.T, report Report, severity Severity, name, detail string) {
	t.Helper()
	for _, check := range report.Checks {
		if check.Severity == severity && check.Name == name && strings.Contains(check.Detail, detail) {
			return
		}
	}
	t.Fatalf("missing %s %s containing %q in %#v", severity, name, detail, report.Checks)
}

func assertExactCheck(t *testing.T, report Report, want Check) {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == want.Name {
			if check != want {
				t.Fatalf("%s check = %#v, want %#v", want.Name, check, want)
			}
			return
		}
	}
	t.Fatalf("missing exact check %#v in %#v", want, report.Checks)
}

func assertNoCheck(t *testing.T, report Report, name string) {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			t.Fatalf("unexpected %s: %#v", name, check)
		}
	}
}

func assertCheckNames(t *testing.T, report Report, want []string) {
	t.Helper()
	got := make([]string, 0, len(report.Checks))
	for _, check := range report.Checks {
		got = append(got, check.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("check order = %#v, want %#v", got, want)
	}
}

func assertSummaryMatchesChecks(t *testing.T, report Report) {
	t.Helper()
	want := summarize(report.Checks)
	if report.Summary != want {
		t.Fatalf("summary = %#v, want %#v", report.Summary, want)
	}
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s|%s|%o", rel, info.Mode().Type(), info.Mode().Perm())
		if entry.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			line += "|" + string(data)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			line += "|->" + target
		}
		entries = append(entries, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(entries, "\n")
}
