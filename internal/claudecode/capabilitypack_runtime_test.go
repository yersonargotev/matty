package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type ownershipStore struct {
	state capabilitypack.ActivationState
}

func (s ownershipStore) Load(context.Context, capabilitypack.Surface) (capabilitypack.ActivationState, error) {
	return s.state, nil
}
func (s ownershipStore) Save(context.Context, capabilitypack.Surface, int, capabilitypack.ActivationState) error {
	return nil
}

type staticRuntimeEvidence []RuntimeEvidence

func (s staticRuntimeEvidence) ObserveRuntimeEvidence(context.Context) []RuntimeEvidence {
	return append([]RuntimeEvidence(nil), s...)
}

func TestRuntimeEvidenceIsBoundToEveryCurrentProjectionFact(t *testing.T) {
	pack := capabilitypack.Pack{ID: "pack", Version: "3.0.0"}
	auth := AuthorizationObservation{PolicyObserved: true, ToolPermissionObserved: true}
	projections := []capabilitypack.ObservedProjection{
		{ID: "skill:one", Goal: capabilitypack.ProjectionPresent, ObservedFingerprint: "one", DesiredFingerprint: "one", Action: capabilitypack.ProjectionAction{Kind: ActionSkillLink}},
		{ID: "mcp_server:memory", Goal: capabilitypack.ProjectionPresent, ObservedFingerprint: "mcp", DesiredFingerprint: "mcp", Action: capabilitypack.ProjectionAction{Kind: ActionUserMCP}},
	}
	evidence := []RuntimeEvidence{NewRuntimeEvidence(pack, projections[0], "2.1.203", auth, "loading"), NewRuntimeEvidence(pack, projections[1], "2.1.203", auth, "connection")}
	a := (&SurfaceAdapter{}).WithRuntimeEvidence(staticRuntimeEvidence(evidence))
	usable, observed, facts := a.runtimeReadiness(context.Background(), pack, projections, VersionObservation{Version: "2.1.203"}, auth)
	if !usable || !observed || len(facts) != 2 {
		t.Fatalf("usable=%v observed=%v facts=%v", usable, observed, facts)
	}
	projections[0].DesiredFingerprint = "changed-definition"
	if usable, observed, _ := a.runtimeReadiness(context.Background(), pack, projections, VersionObservation{Version: "2.1.203"}, auth); usable || observed {
		t.Fatal("definition change retained stale evidence")
	}
	projections[0].DesiredFingerprint = "one"
	pack.Version = "3.0.1"
	if usable, observed, _ := a.runtimeReadiness(context.Background(), pack, projections, VersionObservation{Version: "2.1.203"}, auth); usable || observed {
		t.Fatal("pack version change retained stale evidence")
	}
}

func TestClaudeCommandAndAgentRenderingCarryNativeContracts(t *testing.T) {
	command := capabilitypack.Resource{Description: "Refine", Arguments: capabilitypack.CommandArguments{Mode: "freeform", Placeholder: "$ARGUMENTS"}}
	if got := claudeCommandSkill(command, "refine", []byte("Use $ARGUMENTS.")); !strings.Contains(got, "name: refine") || !strings.Contains(got, "$ARGUMENTS") {
		t.Fatal(got)
	}
	agent := capabilitypack.Resource{ID: "reviewer", Description: "Review changes"}
	authority := capabilitypack.AgentAuthority{Tools: []capabilitypack.AuthorityTranslation{{Portable: "shell", Claude: "Bash"}}, Permissions: []capabilitypack.AuthorityTranslation{{Portable: "write", Claude: "Edit"}}}
	got := claudeAgentDocument(agent, "aliased-reviewer", authority, []byte("Review."))
	if !strings.Contains(got, "name: aliased-reviewer") || !strings.Contains(got, "tools: Bash") || !strings.Contains(got, "shell=Bash") || !strings.Contains(got, "write=Edit") || strings.Contains(got, "\npermissions:") {
		t.Fatal(got)
	}
}

func TestInspectionSealsExternalConsentAndExactCommandAssetCleanup(t *testing.T) {
	home, bundle := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(bundle, "commands"), 0700)
	os.WriteFile(filepath.Join(bundle, "commands", "run.md"), []byte("Run $ARGUMENTS"), 0600)
	os.WriteFile(filepath.Join(bundle, "commands", "guide.txt"), []byte("guide"), 0600)
	bind := func(projection, name string) capabilitypack.Binding {
		return capabilitypack.Binding{Surface: capabilitypack.SurfaceClaude, Projection: projection, Name: name}
	}
	hookBinding := bind("command_hook", "start")
	hookBinding.Hook = &capabilitypack.CommandHook{Type: "command", Event: "SessionStart", Command: "echo", Args: []string{}, TimeoutSeconds: 1, Blocking: true, Failure: "block", Authorities: []string{}}
	pack := capabilitypack.Pack{ID: "p", Version: "1.0.0", Resources: []capabilitypack.Resource{
		{Kind: "command", ID: "run", Source: "commands/run.md", Requires: []string{"asset:guide"}, Bindings: []capabilitypack.Binding{bind("skill", "run")}},
		{Kind: "asset", ID: "guide", Source: "commands/guide.txt"},
		{Kind: "lifecycle", ID: "start", Bindings: []capabilitypack.Binding{hookBinding}},
		{Kind: "mcp_server", ID: "memory", Command: "engram", Args: []string{"mcp"}, Bindings: []capabilitypack.Binding{bind("mcp_server", "memory")}},
	}}
	layout := NewCanonicalLayout(home)
	os.MkdirAll(filepath.Join(layout.SkillsDir, "run"), 0700)
	os.WriteFile(filepath.Join(layout.SkillsDir, "run", "SKILL.md"), []byte(claudeCommandSkill(pack.Resources[0], "run", []byte("Run $ARGUMENTS"))), 0600)
	os.WriteFile(filepath.Join(layout.SkillsDir, "run", "guide.txt"), []byte("guide"), 0600)
	state := capabilitypack.ActivationState{Intent: capabilitypack.ActivationIntent{PackID: "p", Version: "1.0.0", Surface: capabilitypack.SurfaceClaude, Active: true}, Ownership: []capabilitypack.ProjectionOwnership{{ID: "command:run", Contributors: []string{"p"}}, {ID: "asset:command:run:guide", Contributors: []string{"p"}}}}
	provider := NewCapabilityPackOwnershipProvider(ownershipStore{state}, map[string]capabilitypack.Pack{"p": pack}, layout, bundle)
	a := NewSurfaceAdapter(bundle, layout, filepath.Join(home, "state"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, provider)
	inspection, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lifecycle:start", "mcp_server:memory"} {
		found := false
		for _, p := range inspection.Projections {
			if p.ID == id {
				found = true
				if p.Action.Consent != capabilitypack.ConsentExecutableExternal {
					t.Fatalf("%s consent=%q", id, p.Action.Consent)
				}
			}
		}
		if !found {
			t.Fatal(id + " missing")
		}
	}
	removal, err := a.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: pack})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"command:run": false, "asset:command:run:guide": false}
	for _, p := range removal.Projections {
		if _, ok := want[p.ID]; ok {
			want[p.ID] = p.Goal == capabilitypack.ProjectionAbsent && p.Action.Mode == capabilitypack.ProjectionDeleteTarget && p.Action.Consent == ""
		}
	}
	for id, ok := range want {
		if !ok {
			t.Fatalf("exact cleanup missing for %s: %+v", id, removal.Projections)
		}
	}
	actions := []capabilitypack.ProjectionAction{}
	for _, p := range removal.Projections {
		if _, ok := want[p.ID]; ok && p.Goal == capabilitypack.ProjectionAbsent {
			actions = append(actions, p.Action)
		}
	}
	if err := a.ApplyProjections(context.Background(), actions); err != nil {
		t.Fatalf("provider-backed cleanup: %v", err)
	}
	for _, path := range []string{filepath.Join(layout.SkillsDir, "run", "SKILL.md"), filepath.Join(layout.SkillsDir, "run", "guide.txt")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("not removed: %s", path)
		}
	}
}

func TestCapabilityPackOwnershipProviderReturnsHookAndMCPIdentity(t *testing.T) {
	home := t.TempDir()
	layout := NewCanonicalLayout(home)
	os.MkdirAll(layout.ConfigDir, 0700)
	hook := capabilitypack.CommandHook{Type: "command", Event: "SessionStart", Command: "echo", Args: []string{}, TimeoutSeconds: 1, Blocking: true, Failure: "block", Authorities: []string{}}
	binding := capabilitypack.Binding{Surface: capabilitypack.SurfaceClaude, Projection: "command_hook", Name: "start", Hook: &hook}
	entry := fromBindingHook(binding)
	settings, _, err := MergeCommandHookWithProvenance(nil, entry, false, HookMergeProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(layout.SettingsFile, settings, 0600)
	os.WriteFile(layout.UserMCPFile, []byte(`{"mcpServers":{"memory":{"command":"engram","args":["mcp"]}}}`), 0600)
	pack := capabilitypack.Pack{ID: "p", Version: "1.0.0", Resources: []capabilitypack.Resource{{Kind: "lifecycle", ID: "start", Bindings: []capabilitypack.Binding{binding}}, {Kind: "mcp_server", ID: "memory", Command: "engram", Args: []string{"mcp"}, Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "mcp_server", Name: "memory"}}}}}
	state := capabilitypack.ActivationState{Intent: capabilitypack.ActivationIntent{PackID: "p", Version: "1.0.0", Surface: capabilitypack.SurfaceClaude, Active: true}, Ownership: []capabilitypack.ProjectionOwnership{{ID: "lifecycle:start", Fingerprint: entry.Fingerprint(), Contributors: []string{"p"}}, {ID: "mcp_server:memory", Contributors: []string{"p"}}}}
	snapshot, err := NewCapabilityPackOwnershipProvider(ownershipStore{state}, map[string]capabilitypack.Pack{"p": pack}, layout, t.TempDir()).ObserveOwnership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Records) != 2 {
		t.Fatalf("records=%+v", snapshot.Records)
	}
	for _, record := range snapshot.Records {
		switch record.ID {
		case "lifecycle:start":
			if record.Kind != string(ActionCommandHook) || record.Fingerprint != entry.Fingerprint() || record.HookProvenance != (HookMergeProvenance{}).Seal() {
				t.Fatalf("hook=%+v", record)
			}
		case "mcp_server:memory":
			if record.Kind != string(ActionUserMCP) || record.Command != "engram" || len(record.Args) != 1 {
				t.Fatalf("mcp=%+v", record)
			}
		}
	}
}

func TestMultipleTypedHooksShareOneSealedSettingsDocument(t *testing.T) {
	home := t.TempDir()
	layout := NewCanonicalLayout(home)
	hook := func(id, event string) capabilitypack.Resource {
		return capabilitypack.Resource{Kind: "lifecycle", ID: id, Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "command_hook", Name: id, Hook: &capabilitypack.CommandHook{Type: "command", Event: event, Command: "engram", Args: []string{id}, TimeoutSeconds: 5, Failure: "warn", Authorities: []string{}}}}}
	}
	pack := capabilitypack.Pack{ID: "p", Version: "1", Resources: []capabilitypack.Resource{hook("start", "SessionStart"), hook("stop", "SessionEnd")}}
	adapter := NewSurfaceAdapter("", layout, filepath.Join(home, "state"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, StaticOwnershipSnapshot(OwnershipSnapshot{}))
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 2 || inspection.Projections[0].Action.Content != inspection.Projections[1].Action.Content {
		t.Fatalf("hooks were not aggregated: %#v", inspection.Projections)
	}
	for _, projection := range inspection.Projections {
		if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{projection.Action}); err != nil {
			t.Fatal(err)
		}
	}
	settings := ObserveSettings(layout.SettingsFile, nil)
	if settings.Err != nil || len(EnrichHookObservation(settings, fromBindingHook(pack.Resources[0].Bindings[0])).MatchingEntries) != 1 || len(EnrichHookObservation(settings, fromBindingHook(pack.Resources[1].Bindings[0])).MatchingEntries) != 1 {
		t.Fatalf("settings = %#v", settings)
	}
}
