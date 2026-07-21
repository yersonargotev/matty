package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/claudecode"
)

type countingClaudeObserver struct {
	runnerCalls, authorizationCalls, runtimeCalls int
}

func (o *countingClaudeObserver) Run(context.Context, claudecode.Command) claudecode.Result {
	o.runnerCalls++
	return claudecode.Result{Stdout: claudecode.MinimumSupportedVersion}
}

func (o *countingClaudeObserver) ObserveAuthorization(context.Context) claudecode.AuthorizationObservation {
	o.authorizationCalls++
	return claudecode.AuthorizationObservation{PolicyObserved: true, ToolPermissionObserved: true}
}

func (o *countingClaudeObserver) ObserveRuntimeEvidence(context.Context) []claudecode.RuntimeEvidence {
	o.runtimeCalls++
	return []claudecode.RuntimeEvidence{}
}

func TestPackHelpDocumentsClaudeAndStructuredOutput(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{})
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Claude Code, Codex, and OpenCode", "packy pack show engram --json",
		"packy pack status engram --surface claude",
		"packy pack activate engram --surface claude --dry-run --json",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pack help missing %q:\n%s", want, out)
		}
	}
}

func TestPackShowJSONV2PublishesSurfaceContracts(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}}
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "show", "engram", "--json")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	var report packShowJSON
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if report.SchemaVersion != 2 || report.Report != "pack-show" || report.ID != "engram" || len(report.SurfaceContracts) != 3 || report.Surfaces == nil || report.Provides == nil || report.Requires.Capabilities == nil || report.Requires.Tools == nil || report.Conflicts == nil {
		t.Fatalf("show contract = %#v", report)
	}
	for _, surface := range report.SurfaceContracts {
		if surface.Contract.Compatibility == "" || surface.Contract.Bindings == nil || surface.Contract.Exclusions == nil {
			t.Fatalf("incomplete %s contract: %#v", surface.Surface, surface.Contract)
		}
	}
	if strings.Contains(out, "Surface contract:") || strings.Contains(out, "environment") {
		t.Fatalf("human or environment output leaked into JSON: %s", out)
	}
}

func TestClaudePackCompositionUsesDedicatedObservationSeams(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	observer := &countingClaudeObserver{}
	lookupCalls := 0
	opts := Options{
		Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")},
		ClaudeLookPath: func(name string) (string, error) {
			lookupCalls++
			if name != "claude" {
				t.Fatalf("lookup = %q", name)
			}
			return "/sandbox/bin/claude", nil
		},
		ClaudeRunner: observer, ClaudeAuthorization: observer, ClaudeRuntimeEvidence: observer,
	}
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "ma"+"tty", "--surface", "claude", "--json")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	if lookupCalls != 1 || observer.runnerCalls == 0 || observer.authorizationCalls == 0 || observer.runtimeCalls == 0 {
		t.Fatalf("seam calls lookup=%d runner=%d authorization=%d runtime=%d", lookupCalls, observer.runnerCalls, observer.authorizationCalls, observer.runtimeCalls)
	}
}
