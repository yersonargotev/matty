package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type recordingRunner struct {
	calls  []Command
	result Result
}

func (r *recordingRunner) Run(_ context.Context, c Command) Result {
	r.calls = append(r.calls, c)
	return r.result
}

func TestVersionCompatibilityMatrix(t *testing.T) {
	for _, tc := range []struct {
		v    string
		want Compatibility
	}{{"2.1.202", CompatibilityBelowFloor}, {"2.1.203", CompatibilitySupported}, {"2.1.204", CompatibilitySupported}, {"2.1.203-beta.1", CompatibilityPrerelease}, {"garbage", CompatibilityUnreadable}} {
		if got := ClassifyVersion(VersionObservation{Executable: "claude", Version: tc.v}); got != tc.want {
			t.Errorf("%q = %s, want %s", tc.v, got, tc.want)
		}
	}
	if got := ClassifyVersion(VersionObservation{Missing: true}); got != CompatibilityMissing {
		t.Fatal(got)
	}
	if got := ClassifyVersion(VersionObservation{Executable: "claude", TimedOut: true}); got != CompatibilityTimedOut {
		t.Fatal(got)
	}
}

func TestUserMCPObservationIsStaticAndRedactedIdentity(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	os.WriteFile(path, []byte(`{"mcpServers":{"memory":{"command":"engram","args":["mcp"],"env":{"TOKEN":"secret"}}}}`), 0600)
	r := &recordingRunner{}
	o := ObserveUserMCP(path, "memory")
	if !o.Present || len(r.calls) != 0 {
		t.Fatalf("observation=%+v calls=%d", o, len(r.calls))
	}
	if strings.Contains(o.DefinitionFingerprint, "secret") {
		t.Fatal("fingerprint rendered secret")
	}
}

func TestInstructionUpsertPreservesForeignAndOtherContributorBytes(t *testing.T) {
	doc := "foreign before\n" + instructionStart + "\n<!-- contributor:classic -->\nold\n<!-- /contributor:classic -->\n" + instructionEnd + "\nforeign after\n"
	got, err := UpsertInstructionContribution(doc, InstructionContribution{ContributorID: "pack:p:r", Content: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "foreign before\n") || !strings.HasSuffix(got, "foreign after\n") || !strings.Contains(got, "classic -->\nold") {
		t.Fatalf("foreign bytes changed:\n%s", got)
	}
	repeated, err := UpsertInstructionContribution(got, InstructionContribution{ContributorID: "pack:p:r", Content: "new"})
	if err != nil || repeated != got {
		t.Fatalf("not idempotent: %v", err)
	}
}

func TestApplyRejectsUnsealedAndStaleActions(t *testing.T) {
	home := t.TempDir()
	layout := NewCanonicalLayout(home)
	a := NewSurfaceAdapter("", layout, filepath.Join(home, "state"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
	if err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{{ID: "x", Kind: "foreign", Target: layout.SettingsFile}}); err == nil {
		t.Fatal("unsealed action accepted")
	}
	os.MkdirAll(layout.ConfigDir, 0700)
	os.WriteFile(layout.SettingsFile, []byte("old"), 0600)
	err := a.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{{ID: "hook:x", Kind: ActionCommandHook, Target: layout.SettingsFile, Content: "new", Command: Fingerprint([]byte("prior"))}})
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale action error = %v", err)
	}
	b, _ := os.ReadFile(layout.SettingsFile)
	if string(b) != "old" {
		t.Fatal("stale action mutated document")
	}
}

func TestCommandDescriptionDoesNotRenderEnvironment(t *testing.T) {
	c := Command{Env: []string{"TOKEN=secret"}, Description: "configure redacted MCP"}
	if strings.Contains(c.String(), "secret") || c.String() != "configure redacted MCP" {
		t.Fatal(c.String())
	}
}
