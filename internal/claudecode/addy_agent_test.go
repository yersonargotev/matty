package claudecode

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestRenderAddyClaudeAgentExactBytesAndEffectiveDependency(t *testing.T) {
	authority := &capabilitypack.AgentAuthority{PermissionMode: "default", Authorities: []capabilitypack.AuthorityRecord{
		{Portable: "browser", Declarations: []string{"optional-mode:browser-network:browser", "tool:browser"}, Outcome: "fallback", ClaudeTools: []string{}, Fallback: "static evidence-only analysis"},
		{Portable: "commit", Declarations: []string{"optional-mode:privileged-shipping:commit"}, Outcome: "guarded", ClaudeTools: []string{"Bash"}, Fallback: "none"},
		{Portable: "deploy", Declarations: []string{"optional-mode:privileged-shipping:deploy"}, Outcome: "guarded", ClaudeTools: []string{"Bash"}, Fallback: "none"},
		{Portable: "filesystem", Declarations: []string{"permission:filesystem"}, Outcome: "native", ClaudeTools: []string{"Glob", "Grep", "Read"}, Fallback: "none"},
		{Portable: "network", Declarations: []string{"optional-mode:browser-network:network"}, Outcome: "fallback", ClaudeTools: []string{}, Fallback: "static evidence-only analysis"},
		{Portable: "package-manager", Declarations: []string{"optional-mode:package-tools:package-manager"}, Outcome: "native", ClaudeTools: []string{"Bash"}, Fallback: "report commands without running them"},
		{Portable: "process", Declarations: []string{"optional-mode:package-tools:process", "permission:process"}, Outcome: "native", ClaudeTools: []string{"Bash"}, Fallback: "report commands without running them"},
		{Portable: "subagent", Declarations: []string{"optional-mode:specialist-fanout:subagent"}, Outcome: "fallback", ClaudeTools: []string{}, Fallback: "sequential single-agent analysis"},
	}}
	agent := capabilitypack.Resource{
		Kind: "agent", ID: "code-reviewer", Description: "Portable catalog description", Requires: []string{"skill:using-agent-skills"},
		Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: "addy-code-reviewer", AgentAuthority: authority}},
	}
	pack := capabilitypack.Pack{ID: "addy", Version: "1.1.0", Resources: []capabilitypack.Resource{
		agent,
		{Kind: "skill", ID: "using-agent-skills", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "addy-using-agent-skills"}}},
	}}
	source := []byte("---\nname: code-reviewer\ndescription: \"Review: exactly.\"\n---\n\n# Review\n\nKeep these body bytes.  \n")
	got, err := renderAddyClaudeAgent(pack, agent, agent.Bindings[0], source)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("---\n" +
		"name: addy-code-reviewer\n" +
		"description: \"Review: exactly.\"\n" +
		"permissionMode: default\n" +
		"tools: Bash, Glob, Grep, Read\n" +
		"skills:\n" +
		"  - addy-using-agent-skills\n" +
		"---\n\n" +
		"## Packy authority contract\n\n" +
		"- permission_mode: default\n" +
		"- browser: declarations=[optional-mode:browser-network:browser, tool:browser]; outcome=fallback; claude_tools=[none]; fallback=static evidence-only analysis\n" +
		"- commit: declarations=[optional-mode:privileged-shipping:commit]; outcome=guarded; claude_tools=[Bash]; fallback=none\n" +
		"- deploy: declarations=[optional-mode:privileged-shipping:deploy]; outcome=guarded; claude_tools=[Bash]; fallback=none\n" +
		"- filesystem: declarations=[permission:filesystem]; outcome=native; claude_tools=[Glob, Grep, Read]; fallback=none\n" +
		"- network: declarations=[optional-mode:browser-network:network]; outcome=fallback; claude_tools=[none]; fallback=static evidence-only analysis\n" +
		"- package-manager: declarations=[optional-mode:package-tools:package-manager]; outcome=native; claude_tools=[Bash]; fallback=report commands without running them\n" +
		"- process: declarations=[optional-mode:package-tools:process, permission:process]; outcome=native; claude_tools=[Bash]; fallback=report commands without running them\n" +
		"- subagent: declarations=[optional-mode:specialist-fanout:subagent]; outcome=fallback; claude_tools=[none]; fallback=sequential single-agent analysis\n\n" +
		"# Review\n\nKeep these body bytes.  \n")
	if !bytes.Equal(got, want) {
		t.Fatalf("rendered bytes differ\nwant:\n%s\ngot:\n%s", want, got)
	}
	for _, forbidden := range []string{"\npermissions:", "\nmodel:", "\neffort:", "\nmemory:", "\nbackground:", "\nhooks:", "\nmcp:", "\nisolation:", "\ninitial-prompt:"} {
		if strings.Contains(string(got), forbidden) {
			t.Fatalf("render contains forbidden field %q", forbidden)
		}
	}
}

func TestDecodeAddyAgentSourceRejectsNegativeTwins(t *testing.T) {
	valid := "---\nname: reviewer\ndescription: Review exactly\n---\n\nBody\n"
	tests := map[string][]byte{
		"invalid UTF-8":       append([]byte(valid), 0xff),
		"no frontmatter":      []byte("name: reviewer\n"),
		"unterminated":        []byte("---\nname: reviewer\n"),
		"unknown key":         []byte(strings.Replace(valid, "description:", "model: x\ndescription:", 1)),
		"duplicate name":      []byte(strings.Replace(valid, "name: reviewer", "name: reviewer\nname: reviewer", 1)),
		"missing description": []byte(strings.Replace(valid, "description: Review exactly\n", "", 1)),
		"malformed line":      []byte(strings.Replace(valid, "description: Review exactly", "description Review exactly", 1)),
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeAddyAgentSource(source); err == nil {
				t.Fatal("accepted invalid Addy agent source")
			}
		})
	}
}

func TestRenderAddyClaudeAgentRejectsPortableAndDependencyDrift(t *testing.T) {
	authority := &capabilitypack.AgentAuthority{PermissionMode: "default"}
	base := capabilitypack.Resource{Kind: "agent", ID: "reviewer", Description: "Review", Requires: []string{"skill:using-agent-skills"}}
	binding := capabilitypack.Binding{Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: "reviewer", AgentAuthority: authority}
	skill := capabilitypack.Resource{Kind: "skill", ID: "using-agent-skills", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "using-agent-skills"}}}
	source := []byte("---\nname: reviewer\ndescription: Review\n---\n\nBody")
	tests := []struct {
		name   string
		agent  capabilitypack.Resource
		pack   capabilitypack.Pack
		source []byte
	}{
		{"name drift", base, capabilitypack.Pack{Resources: []capabilitypack.Resource{skill}}, bytes.Replace(source, []byte("name: reviewer"), []byte("name: other"), 1)},
		{"missing require", func() capabilitypack.Resource { r := base; r.Requires = nil; return r }(), capabilitypack.Pack{Resources: []capabilitypack.Resource{skill}}, source},
		{"missing binding", base, capabilitypack.Pack{}, source},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := renderAddyClaudeAgent(tt.pack, tt.agent, binding, tt.source); err == nil {
				t.Fatal("accepted drift")
			}
		})
	}
}

func TestOptionalAuthorityReadinessIsCompleteCanonicalAndSeparate(t *testing.T) {
	pack := capabilitypack.Pack{Contract: capabilitypack.Contract{OptionalModes: []capabilitypack.OptionalMode{
		{ID: "browser-network", Authorities: []string{"browser", "network"}, Fallback: "static evidence-only analysis"},
		{ID: "privileged-shipping", Authorities: []string{"commit", "deploy"}, Fallback: "none"},
	}}}
	auth := AuthorizationObservation{
		PolicyObserved: true, ToolPermissionObserved: true,
		OptionalAuthorities: []OptionalAuthorityAvailability{
			{ModeID: "privileged-shipping", Authority: "commit", State: capabilitypack.OptionalAuthorityAvailable},
			{ModeID: "browser-network", Authority: "browser", State: capabilitypack.OptionalAuthorityUnavailable},
		},
	}
	got, err := optionalAuthorityReadiness(pack, auth)
	if err != nil {
		t.Fatal(err)
	}
	want := []capabilitypack.OptionalAuthorityObservation{
		{ModeID: "browser-network", Authority: "browser", State: capabilitypack.OptionalAuthorityUnavailable, Fallback: "static evidence-only analysis"},
		{ModeID: "browser-network", Authority: "network", State: capabilitypack.OptionalAuthorityUnknown, Fallback: "static evidence-only analysis"},
		{ModeID: "privileged-shipping", Authority: "commit", State: capabilitypack.OptionalAuthorityAvailable, Fallback: "none"},
		{ModeID: "privileged-shipping", Authority: "deploy", State: capabilitypack.OptionalAuthorityUnknown, Fallback: "none"},
	}
	if len(got) != len(want) {
		t.Fatalf("optional authorities = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("optional authorities[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}

	auth.OptionalAuthorities = append(auth.OptionalAuthorities, auth.OptionalAuthorities[0])
	if _, err := optionalAuthorityReadiness(pack, auth); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate optional authority error = %v", err)
	}
	auth.OptionalAuthorities = []OptionalAuthorityAvailability{{ModeID: "unknown", Authority: "browser", State: capabilitypack.OptionalAuthorityAvailable}}
	if _, err := optionalAuthorityReadiness(pack, auth); err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("undeclared optional authority error = %v", err)
	}
}

func TestRuntimeEvidenceInvalidatesOnHostPrecedencePolicyAndOptionalAuthorityChanges(t *testing.T) {
	pack := capabilitypack.Pack{
		ID: "addy", Version: "1.1.0",
		Contract: capabilitypack.Contract{OptionalModes: []capabilitypack.OptionalMode{{
			ID: "browser-network", Authorities: []string{"browser"}, Fallback: "static evidence-only analysis",
		}}},
	}
	projection := capabilitypack.ObservedProjection{
		ID: "agent:code-reviewer", Goal: capabilitypack.ProjectionPresent,
		ObservedFingerprint: "definition", DesiredFingerprint: "definition",
		Action: capabilitypack.ProjectionAction{
			ID: "agent:code-reviewer", Kind: ActionAgentFile, Target: "/sandbox/.claude/agents/code-reviewer.md",
		},
	}
	auth := AuthorizationObservation{
		PolicyObserved: true, ToolPermissionObserved: true,
		OptionalAuthorities: []OptionalAuthorityAvailability{{
			ModeID: "browser-network", Authority: "browser", State: capabilitypack.OptionalAuthorityUnavailable,
		}},
	}
	evidence := NewRuntimeEvidence(pack, projection, "2.1.203", auth, "loading")
	adapter := (&SurfaceAdapter{}).WithRuntimeEvidence(staticRuntimeEvidence([]RuntimeEvidence{evidence}))
	if usable, observed, _ := adapter.runtimeReadiness(context.Background(), pack, []capabilitypack.ObservedProjection{projection}, VersionObservation{Version: "2.1.203"}, auth); !usable || !observed {
		t.Fatal("matching runtime evidence was not accepted")
	}
	tests := []struct {
		name       string
		projection capabilitypack.ObservedProjection
		auth       AuthorizationObservation
	}{
		{name: "host target", projection: func() capabilitypack.ObservedProjection {
			changed := projection
			changed.Action.Target = "/other/.claude/agents/code-reviewer.md"
			return changed
		}(), auth: auth},
		{name: "precedence", projection: projection, auth: func() AuthorizationObservation {
			changed := auth
			changed.Shadowed = true
			return changed
		}()},
		{name: "policy", projection: projection, auth: func() AuthorizationObservation {
			changed := auth
			changed.ToolPermissionObserved = false
			return changed
		}()},
		{name: "optional authority", projection: projection, auth: func() AuthorizationObservation {
			changed := auth
			changed.OptionalAuthorities = []OptionalAuthorityAvailability{{
				ModeID: "browser-network", Authority: "browser", State: capabilitypack.OptionalAuthorityAvailable,
			}}
			return changed
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if usable, observed, _ := adapter.runtimeReadiness(context.Background(), pack, []capabilitypack.ObservedProjection{test.projection}, VersionObservation{Version: "2.1.203"}, test.auth); usable || observed {
				t.Fatal("stale runtime evidence remained valid")
			}
		})
	}
}

func TestAddyCodeReviewerAgentInspectApplyVerifyAndRemove(t *testing.T) {
	bundle, pack := addyCompositeFixture(t)
	pack.Resources[0].ID = "using-agent-skills"
	pack.Resources[0].Bindings[0].Name = "using-agent-skills"
	authority := codeReviewerAuthority()
	agent := capabilitypack.Resource{
		Kind: "agent", ID: "code-reviewer", Description: "Portable catalog description",
		Source: "agents/code-reviewer.md", Requires: []string{"skill:using-agent-skills"},
		Bindings: []capabilitypack.Binding{{
			Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: "addy-code-reviewer",
			AgentAuthority: authority,
		}},
	}
	pack.Resources = append(pack.Resources, agent)
	pack.Contract.OptionalModes = []capabilitypack.OptionalMode{{
		ID: "browser-network", Authorities: []string{"browser", "network"}, Fallback: "static evidence-only analysis",
	}}
	source := []byte("---\nname: code-reviewer\ndescription: Review changes exactly\n---\n\n# Code reviewer\n\nPreserve this body.\n")
	writeAddyFile(t, bundle, agent.Source, source, 0o644)

	home := t.TempDir()
	layout := NewCanonicalLayout(home)
	var ownership OwnershipSnapshot
	provider := OwnershipSnapshotFunc(func(context.Context) (OwnershipSnapshot, error) { return ownership, nil })
	auth := AuthorizationObservation{
		PolicyObserved: true, ToolPermissionObserved: true,
		OptionalAuthorities: []OptionalAuthorityAvailability{
			{ModeID: "browser-network", Authority: "browser", State: capabilitypack.OptionalAuthorityUnavailable},
			{ModeID: "browser-network", Authority: "network", State: capabilitypack.OptionalAuthorityUnknown},
		},
	}
	adapter := NewSurfaceAdapterWithAuthorization(bundle, layout, filepath.Join(home, "state"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, provider, AuthorizationObserverFunc(func(context.Context) AuthorizationObservation { return auth }))

	missing, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing.Projections) != 2 || missing.Readiness.Authorized || missing.Readiness.Usable {
		t.Fatalf("missing inspection = %#v", missing)
	}
	actions := []capabilitypack.ProjectionAction{missing.Projections[0].Action, missing.Projections[1].Action}
	var agentTarget, skillTarget string
	for _, action := range actions {
		switch action.Kind {
		case ActionAgentFile:
			agentTarget = action.Target
		case ActionSkillTree:
			skillTarget = action.Target
		}
	}
	if err := os.MkdirAll(filepath.Dir(agentTarget), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentTarget, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if applyErr := adapter.ApplyProjections(context.Background(), actions); applyErr == nil {
		t.Fatal("foreign agent collision was accepted")
	}
	if _, err := os.Stat(skillTarget); !os.IsNotExist(err) {
		t.Fatalf("collision partially installed dependency: %v", err)
	}
	if got, err := os.ReadFile(agentTarget); err != nil || string(got) != "foreign" {
		t.Fatalf("foreign agent was changed: %q err=%v", got, err)
	}
	if err := os.Remove(agentTarget); err != nil {
		t.Fatal(err)
	}
	if applyErr := adapter.ApplyProjections(context.Background(), actions); applyErr != nil {
		t.Fatal(applyErr)
	}
	records := make([]OwnershipRecord, 0, len(missing.Projections))
	for _, projection := range missing.Projections {
		switch projection.Action.Kind {
		case ActionSkillTree:
			records = append(records, compositeOwnershipRecord(t, projection, "addy"))
		case ActionAgentFile:
			records = append(records, OwnershipRecord{
				StateOwner: "capabilitypack", ContributorID: "addy", Contributors: []string{"addy"},
				ID: projection.ID, Kind: string(ActionAgentFile), Target: projection.Action.Target,
				Fingerprint: projection.DesiredFingerprint, DeletionAuthorized: true,
			})
		}
	}
	ownership = NewOwnershipSnapshot(records...)
	installed, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	evidence := make([]RuntimeEvidence, 0, len(installed.Projections))
	for _, projection := range installed.Projections {
		if projection.ObservedFingerprint != projection.DesiredFingerprint {
			t.Fatalf("projection did not verify: %#v", projection)
		}
		evidence = append(evidence, NewRuntimeEvidence(pack, projection, "2.1.203", auth, "loading"))
	}
	ready, err := adapter.WithRuntimeEvidence(staticRuntimeEvidence(evidence)).InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if !ready.Readiness.Authorized || !ready.Readiness.Usable || len(ready.Readiness.OptionalAuthorities) != 2 {
		t.Fatalf("ready inspection = %#v", ready.Readiness)
	}
	agentPath := filepath.Join(layout.AgentsDir, "addy-code-reviewer.md")
	agentBytes, err := os.ReadFile(agentPath)
	if err != nil || !bytes.Contains(agentBytes, []byte("tools: Bash, Glob, Grep, Read")) || bytes.Contains(agentBytes, []byte("\npermissions:")) {
		t.Fatalf("installed agent = %q err=%v", agentBytes, err)
	}

	removal, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Prior: pack})
	if err != nil {
		t.Fatal(err)
	}
	removalActions := make([]capabilitypack.ProjectionAction, 0, len(removal.Projections))
	for _, projection := range removal.Projections {
		if projection.Goal != capabilitypack.ProjectionAbsent {
			t.Fatalf("removal projection = %#v", projection)
		}
		removalActions = append(removalActions, projection.Action)
	}
	if applyErr := adapter.ApplyProjections(context.Background(), removalActions); applyErr != nil {
		t.Fatal(applyErr)
	}
	for _, target := range []string{agentPath, filepath.Join(layout.SkillsDir, "using-agent-skills")} {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("owned target was not removed: %s err=%v", target, err)
		}
	}
}

func codeReviewerAuthority() *capabilitypack.AgentAuthority {
	return &capabilitypack.AgentAuthority{PermissionMode: "default", Authorities: []capabilitypack.AuthorityRecord{
		{Portable: "browser", Declarations: []string{"optional-mode:browser-network:browser", "tool:browser"}, Outcome: "fallback", ClaudeTools: []string{}, Fallback: "static evidence-only analysis"},
		{Portable: "commit", Declarations: []string{"optional-mode:privileged-shipping:commit"}, Outcome: "guarded", ClaudeTools: []string{"Bash"}, Fallback: "none"},
		{Portable: "deploy", Declarations: []string{"optional-mode:privileged-shipping:deploy"}, Outcome: "guarded", ClaudeTools: []string{"Bash"}, Fallback: "none"},
		{Portable: "filesystem", Declarations: []string{"permission:filesystem"}, Outcome: "native", ClaudeTools: []string{"Glob", "Grep", "Read"}, Fallback: "none"},
		{Portable: "network", Declarations: []string{"optional-mode:browser-network:network"}, Outcome: "fallback", ClaudeTools: []string{}, Fallback: "static evidence-only analysis"},
		{Portable: "package-manager", Declarations: []string{"optional-mode:package-tools:package-manager"}, Outcome: "native", ClaudeTools: []string{"Bash"}, Fallback: "report commands without running them"},
		{Portable: "process", Declarations: []string{"optional-mode:package-tools:process", "permission:process"}, Outcome: "native", ClaudeTools: []string{"Bash"}, Fallback: "report commands without running them"},
		{Portable: "subagent", Declarations: []string{"optional-mode:specialist-fanout:subagent"}, Outcome: "fallback", ClaudeTools: []string{}, Fallback: "sequential single-agent analysis"},
	}}
}
