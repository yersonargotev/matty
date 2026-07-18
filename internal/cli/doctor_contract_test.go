package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/yersonargotev/packy/internal/engrambin"
)

func TestDoctorCommandPreservesHumanAndJSONWarningContractsReadOnly(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	fixture := newCLITestFixture(t, opts)
	runner := &doctorContractRunner{path: fixture.engram.ExpectedPath()}
	versionCalls := []string{}
	processCalls := 0
	opts.Runner = runner
	opts.EngramFacts = engrambin.Facts{
		Version: func(path string) (string, error) {
			versionCalls = append(versionCalls, path)
			return "1.19.0", nil
		},
		ServeProcesses: func() ([]engrambin.Process, error) {
			processCalls++
			return nil, nil
		},
	}

	before := snapshotTree(t, home)
	human, err := executeCommand(t, NewRootCommand(opts), "doctor")
	if err != nil {
		t.Fatalf("doctor returned a fatal warning: %v\n%s", err, human)
	}
	wantHuman := fmt.Sprintf("HOME=%s\nCONFIG_HOME=%s\nPACKY_STATE=%s\nPACKY_STATE_STATUS=missing\nAGENT_SKILLS=%s\n", fixture.workstation.Home(), fixture.workstation.ConfigurationHome(), fixture.classicState.StateFile(), fixture.skills.Root()) +
		fmt.Sprintf("WARN packy-state: missing at %s; run packy install\n", fixture.classicState.StateFile()) +
		"WARN skill-symlinks: state is missing, so Packy-owned skill links are unknown; run packy install\n" +
		fmt.Sprintf("PASS engram-binary: PATH resolves to canonical Homebrew Engram: %s version 1.19.0\n", runner.path) +
		"PASS engram-local-bin: no ~/.local/bin/engram compatibility symlink is present; Homebrew remains the Engram owner\n" +
		"PASS engram-runtime: no active engram serve process found\n" +
		"WARN engram-setup: state is missing, so delegated setup cannot be confirmed; run packy install\n" +
		fmt.Sprintf("WARN codex-config: missing Packy Codex prompt markers at %s; run packy install\n", fixture.codex.PromptFile()) +
		"WARN opencode-config: missing OpenCode config; run packy install\n"
	if human != wantHuman {
		t.Fatalf("human doctor contract changed:\ngot:\n%s\nwant:\n%s", human, wantHuman)
	}

	jsonOutput, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json returned a fatal warning: %v\n%s", err, jsonOutput)
	}
	wantJSON := fmt.Sprintf("{\"schema_version\":1,\"report\":\"doctor\",\"checks\":["+
		"{\"name\":\"packy-state\",\"severity\":\"WARN\",\"detail\":%s},"+
		"{\"name\":\"skill-symlinks\",\"severity\":\"WARN\",\"detail\":\"state is missing, so Packy-owned skill links are unknown; run packy install\"},"+
		"{\"name\":\"engram-binary\",\"severity\":\"PASS\",\"detail\":%s},"+
		"{\"name\":\"engram-local-bin\",\"severity\":\"PASS\",\"detail\":\"no ~/.local/bin/engram compatibility symlink is present; Homebrew remains the Engram owner\"},"+
		"{\"name\":\"engram-runtime\",\"severity\":\"PASS\",\"detail\":\"no active engram serve process found\"},"+
		"{\"name\":\"engram-setup\",\"severity\":\"WARN\",\"detail\":\"state is missing, so delegated setup cannot be confirmed; run packy install\"},"+
		"{\"name\":\"codex-config\",\"severity\":\"WARN\",\"detail\":%s},"+
		"{\"name\":\"opencode-config\",\"severity\":\"WARN\",\"detail\":\"missing OpenCode config; run packy install\"}],"+
		"\"summary\":{\"status\":\"warnings\",\"passes\":3,\"warnings\":5,\"failures\":0}}\n",
		jsonQuote(t, "missing at "+fixture.classicState.StateFile()+"; run packy install"),
		jsonQuote(t, "PATH resolves to canonical Homebrew Engram: "+runner.path+" version 1.19.0"),
		jsonQuote(t, "missing Packy Codex prompt markers at "+fixture.codex.PromptFile()+"; run packy install"),
	)
	if jsonOutput != wantJSON {
		t.Fatalf("JSON doctor contract changed:\ngot:\n%s\nwant:\n%s", jsonOutput, wantJSON)
	}

	if after := snapshotTree(t, home); after != before {
		t.Fatalf("doctor mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("doctor ran mutating commands: %#v", runner.runs)
	}
	if !reflect.DeepEqual(runner.lookups, []string{"engram", "engram", "engram", "engram"}) {
		t.Fatalf("lookup calls = %#v", runner.lookups)
	}
	if !reflect.DeepEqual(versionCalls, []string{runner.path, runner.path}) || processCalls != 2 {
		t.Fatalf("active fact calls: versions=%#v processes=%d", versionCalls, processCalls)
	}
}

func TestDoctorCommandPreservesCompleteReportAndFatalExitAfterObservationFailures(t *testing.T) {
	opts, _, home := sandboxOptions(t)
	fixture := newCLITestFixture(t, opts)
	lookupErr := errors.New("substituted PATH lookup failure")
	processErr := errors.New("substituted process inspection failure")
	runner := &doctorContractRunner{lookupErr: lookupErr}
	versionCalls := 0
	processCalls := 0
	opts.Runner = runner
	opts.EngramFacts = engrambin.Facts{
		Version: func(string) (string, error) {
			versionCalls++
			return "", errors.New("version must not be observed without an executable")
		},
		ServeProcesses: func() ([]engrambin.Process, error) {
			processCalls++
			return nil, processErr
		},
	}

	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "doctor", "--json")
	if !errors.Is(err, ErrDoctorUnhealthy) {
		t.Fatalf("doctor error = %v, want ErrDoctorUnhealthy\n%s", err, out)
	}
	want := fmt.Sprintf("{\"schema_version\":1,\"report\":\"doctor\",\"checks\":["+
		"{\"name\":\"packy-state\",\"severity\":\"WARN\",\"detail\":%s},"+
		"{\"name\":\"skill-symlinks\",\"severity\":\"WARN\",\"detail\":\"state is missing, so Packy-owned skill links are unknown; run packy install\"},"+
		"{\"name\":\"engram-binary\",\"severity\":\"FAIL\",\"detail\":%s},"+
		"{\"name\":\"engram-local-bin\",\"severity\":\"PASS\",\"detail\":\"no ~/.local/bin/engram compatibility symlink is present; Homebrew remains the Engram owner\"},"+
		"{\"name\":\"engram-runtime\",\"severity\":\"WARN\",\"detail\":%s},"+
		"{\"name\":\"engram-setup\",\"severity\":\"WARN\",\"detail\":\"state is missing, so delegated setup cannot be confirmed; run packy install\"},"+
		"{\"name\":\"codex-config\",\"severity\":\"WARN\",\"detail\":%s},"+
		"{\"name\":\"opencode-config\",\"severity\":\"WARN\",\"detail\":\"missing OpenCode config; run packy install\"}],"+
		"\"summary\":{\"status\":\"failures\",\"passes\":1,\"warnings\":6,\"failures\":1}}\n",
		jsonQuote(t, "missing at "+fixture.classicState.StateFile()+"; run packy install"),
		jsonQuote(t, "engram is not available on PATH; Homebrew Engram exists at "+fixture.engram.ExpectedPath()+"; add it to PATH or run packy install"),
		jsonQuote(t, "could not inspect active engram serve processes: "+processErr.Error()),
		jsonQuote(t, "missing Packy Codex prompt markers at "+fixture.codex.PromptFile()+"; run packy install"),
	)
	if out != want {
		t.Fatalf("failed-observation contract changed:\ngot:\n%s\nwant:\n%s", out, want)
	}

	if after := snapshotTree(t, home); after != before {
		t.Fatalf("doctor mutated sandbox:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !reflect.DeepEqual(runner.lookups, []string{"engram", "engram"}) || len(runner.runs) != 0 {
		t.Fatalf("runner activity: lookups=%#v runs=%#v", runner.lookups, runner.runs)
	}
	if versionCalls != 0 || processCalls != 1 {
		t.Fatalf("active fact calls: versions=%d processes=%d", versionCalls, processCalls)
	}
}

type doctorContractRunner struct {
	path      string
	lookupErr error
	lookups   []string
	runs      []fakeCall
}

func (runner *doctorContractRunner) LookPath(name string) (string, error) {
	runner.lookups = append(runner.lookups, name)
	if runner.lookupErr != nil {
		return "", runner.lookupErr
	}
	return runner.path, nil
}

func (runner *doctorContractRunner) Run(_ context.Context, name string, args ...string) error {
	runner.runs = append(runner.runs, fakeCall{name: name, args: append([]string(nil), args...)})
	return errors.New("doctor must not run commands")
}

func jsonQuote(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

var _ Runner = (*doctorContractRunner)(nil)
