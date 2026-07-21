package claudecode

import (
	"context"
	"encoding/json"
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

func TestObserveSetupAggregatesStaticOwnershipAndPolicyWithBoundedVersion(t *testing.T) {
	home := t.TempDir()
	layout := NewCanonicalLayout(home)
	source := filepath.Join(home, "source", "skill")
	target := filepath.Join(layout.SkillsDir, "skill")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}
	hook := CommandHookEntry{Type: "command", Event: "SessionStart", Command: "engram", Args: []string{"session"}, TimeoutSeconds: 5, Blocking: true, Failure: "block"}
	settings, err := MergeCommandHook(nil, hook, false)
	if err != nil {
		t.Fatal(err)
	}
	var settingsRoot map[string]any
	if err := json.Unmarshal(settings, &settingsRoot); err != nil {
		t.Fatal(err)
	}
	settingsRoot["disableAllHooks"] = true
	settings, err = json.Marshal(settingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.SettingsFile, settings, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.UserMCPFile, []byte(`{"mcpServers":{"memory":{"command":"engram","args":["mcp"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{result: Result{Stdout: "2.1.203"}}
	hookOwnershipFingerprint := HookOwnershipFingerprint(hook.Event, hook.Fingerprint())
	ownership := NewOwnershipSnapshot(
		OwnershipRecord{Kind: string(ActionSkillLink), Target: target, Skill: SkillIdentity{ExpectedSource: source}},
		OwnershipRecord{Kind: string(ActionCommandHook), Target: layout.SettingsFile, Fingerprint: hookOwnershipFingerprint},
		OwnershipRecord{Kind: string(ActionUserMCP), Target: "memory"},
	)
	observation := ObserveSetup(context.Background(), layout, "/bin/claude", runner, ownership)

	if len(runner.calls) != 1 || runner.calls[0].Executable != "/bin/claude" || len(runner.calls[0].Args) != 1 || runner.calls[0].Args[0] != "--version" {
		t.Fatalf("version calls = %#v", runner.calls)
	}
	if len(observation.Skills) != 1 || observation.Skills[0].Kind != PathSymlink || len(observation.MCP) != 1 || !observation.MCP[0].Present {
		t.Fatalf("ownership observations = %#v", observation)
	}
	if !observation.Hooks.Parseable || !observation.Hooks.Disabled || len(observation.Hooks.MatchingEntries) != 1 || observation.Hooks.MatchingEntries[0] != hookOwnershipFingerprint || !observation.Authorization.PolicyObserved || !observation.Authorization.Disabled {
		t.Fatalf("policy observations = %#v", observation)
	}
	hooks := settingsRoot["hooks"].(map[string]any)
	hooks["Stop"] = hooks[hook.Event]
	delete(hooks, hook.Event)
	settings, err = json.Marshal(settingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.SettingsFile, settings, 0o600); err != nil {
		t.Fatal(err)
	}
	wrongEvent := ObserveSetup(context.Background(), layout, "/bin/claude", runner, ownership)
	if len(wrongEvent.Hooks.MatchingEntries) != 0 {
		t.Fatalf("wrong-event hook matched ownership: %#v", wrongEvent.Hooks)
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

func TestInstructionLastContributorCleanupRestoresForeignDocumentByteExact(t *testing.T) {
	document := "operator-owned guidance\n"
	merged, err := UpsertInstructionContribution(document, InstructionContribution{ContributorID: "pack:p:one", Content: "one"})
	if err != nil {
		t.Fatal(err)
	}
	merged, err = UpsertInstructionContribution(merged, InstructionContribution{ContributorID: "pack:p:two", Content: "two"})
	if err != nil {
		t.Fatal(err)
	}
	for _, contributor := range []string{"pack:p:one", "pack:p:two"} {
		merged, err = RemoveInstructionContribution(merged, contributor)
		if err != nil {
			t.Fatal(err)
		}
	}
	if merged != document {
		t.Fatalf("foreign document changed:\n got %q\nwant %q", merged, document)
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
