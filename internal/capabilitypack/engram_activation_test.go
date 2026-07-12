package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeExecutableResolver struct {
	resolutions []ExecutableResolution
	calls       int
}

func (f *fakeExecutableResolver) Resolve(context.Context, string) (ExecutableResolution, error) {
	f.calls++
	if len(f.resolutions) == 0 {
		return ExecutableResolution{}, errors.New("no resolution configured")
	}
	index := f.calls - 1
	if index >= len(f.resolutions) {
		index = len(f.resolutions) - 1
	}
	return f.resolutions[index], nil
}

type fakeExternalExecutor struct {
	actions []ProjectionAction
	events  *[]string
	failID  string
	failErr error
}

func (f *fakeExternalExecutor) Execute(_ context.Context, action ProjectionAction) error {
	f.actions = append(f.actions, action)
	if f.events != nil {
		*f.events = append(*f.events, "external:"+action.ID)
	}
	if action.ID == f.failID {
		return f.failErr
	}
	return nil
}

func engramPackFixture() Pack {
	return Pack{
		ID: "engram", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode},
		Requires: Requirements{Capabilities: []string{}, Tools: []string{"engram"}},
		Resources: []Resource{
			{Kind: "instruction", ID: "engram-memory", Source: "/bundle/instructions/engram-memory.md"},
			{Kind: "mcp_server", ID: "engram", Command: "engram", Args: []string{"mcp", "--tools=agent"}},
			{Kind: "lifecycle", ID: "engram-memory"},
		},
	}
}

func engramObservation(observed string) ActivationObservation {
	instructionObserved, mcpObserved := observed, observed
	if observed == "ready" {
		instructionObserved, mcpObserved = "instruction-new", "mcp-new"
	}
	return ActivationObservation{
		Revision:            "host-1",
		Readiness:           ReadinessStatus{Authorized: false, Usable: false},
		PendingHumanActions: []string{"review host trust", "reload host"},
		Projections: []ObservedProjection{
			{ID: "instruction:engram-memory", Exists: observed != "missing", ObservedFingerprint: instructionObserved, DesiredFingerprint: "instruction-new", Action: ProjectionAction{ID: "instruction:engram-memory", Kind: ActionInstructionFile, Description: "write Engram instruction"}},
			{ID: "mcp_server:engram", Exists: observed != "missing", ObservedFingerprint: mcpObserved, DesiredFingerprint: "mcp-new", Action: ProjectionAction{ID: "mcp_server:engram", Kind: ActionCodexMCPConfig, Description: "configure Engram MCP"}},
		},
	}
}

func engramFacadeForTest(resolver ExecutableResolver, executor ExternalExecutor, observations ...ActivationObservation) (Facade, *fakeActivationAdapter, *fakeActivationStore) {
	pack := engramPackFixture()
	adapter := &fakeActivationAdapter{observations: observations}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, nil,
		WithActivation(store, map[Surface]ActivationAdapter{SurfaceCodex: adapter, SurfaceOpenCode: adapter}),
		WithExternalEffects(resolver, executor),
	)
	return facade, adapter, store
}

func availableEngramResolution(path string) ExecutableResolution {
	return ExecutableResolution{Available: true, Path: path, ResolvedPath: path, Origin: "homebrew", Precondition: path + "|v1"}
}

func missingEngramResolution() ExecutableResolution {
	return ExecutableResolution{Available: false, Path: "/opt/homebrew/bin/engram", Origin: "homebrew", AcquisitionSupported: true, AcquisitionCommand: "brew", AcquisitionArgs: []string{"install", "gentleman-programming/tap/engram"}, Precondition: "missing|/opt/homebrew/bin/engram"}
}

func TestEngramPreviewSealsGlobalExecutableAndSeparatesPhases(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"))

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Resolutions(); len(got) != 1 || got[0].Path != "/opt/homebrew/bin/engram" || !got[0].Available {
		t.Fatalf("resolutions = %#v", got)
	}
	phases := plan.Phases()
	if len(phases) != 3 || phases[0].Kind != ConsentReversibleLocal || phases[1].Kind != ConsentExecutableExternal || phases[2].Kind != ConsentHostFollowUp {
		t.Fatalf("phases = %#v", phases)
	}
	if !phases[0].ApprovalRequired || !phases[1].ApprovalRequired || phases[2].ApprovalRequired {
		t.Fatalf("approval policy = %#v", phases)
	}
	if got := phases[1].Actions; len(got) != 1 || got[0].Command != "/opt/homebrew/bin/engram" || !reflect.DeepEqual(got[0].Args, []string{"setup", "codex"}) {
		t.Fatalf("external actions = %#v", got)
	}
	if resolver.calls != 1 || adapter.inspectCalls != 1 || len(store.saves) != 0 || len(executor.actions) != 0 {
		t.Fatalf("preview side effects: resolver=%d inspect=%d saves=%d external=%d", resolver.calls, adapter.inspectCalls, len(store.saves), len(executor.actions))
	}
}

func TestEngramMissingExecutableUsesSupportedAcquisitionAction(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{missingEngramResolution()}}
	facade, _, _ := engramFacadeForTest(resolver, &fakeExternalExecutor{}, engramObservation("missing"))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	actions := plan.Phases()[1].Actions
	if len(actions) != 2 || actions[0].ID != "external:engram:acquire" || actions[1].ID != "external:engram:setup:opencode" {
		t.Fatalf("external actions = %#v", actions)
	}
	if actions[0].Command != "brew" || !strings.Contains(strings.Join(actions[0].Args, " "), "engram") {
		t.Fatalf("acquisition = %#v", actions[0])
	}
}

func TestEngramLocalApprovalCannotAuthorizeExternalEffects(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"), engramObservation("ready"))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("error = %v", err)
	}
	if adapter.inspectCalls != 1 || len(store.saves) != 0 || len(executor.actions) != 0 {
		t.Fatalf("local approval authorized external work: inspect=%d saves=%d external=%d", adapter.inspectCalls, len(store.saves), len(executor.actions))
	}
}

func TestEngramExternalCommandMutationInvalidatesApproval(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	plan.phases[1].Actions[0].Command = "/tmp/unapproved-command"
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("error = %v", err)
	}
	if adapter.inspectCalls != 1 || len(store.saves) != 0 || len(executor.actions) != 0 {
		t.Fatalf("mutated external plan caused effects")
	}
}

func TestEngramApplyVerifiesLocalBeforeExternalAndReportsPendingReadiness(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	events := []string{}
	executor := &fakeExternalExecutor{events: &events}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"), engramObservation("ready"), engramObservation("ready"))
	adapter.events, store.events = &events, &events
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || !result.Readiness.Configured || result.Readiness.Authorized || result.Readiness.Usable || len(result.PendingHumanActions) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(events) < 4 || !reflect.DeepEqual(events[:4], []string{"persist", "effects", "persist", "external:external:engram:setup:codex"}) {
		t.Fatalf("effect order = %#v", events)
	}
	if len(store.saves) < 2 || store.saves[0].Journal == nil || len(store.saves[0].Ownership) != 0 {
		t.Fatalf("pre-effect state = %+v", store.saves)
	}
	last := store.saves[len(store.saves)-1]
	if last.Journal != nil || len(last.Ownership) != 2 || len(last.External) != 1 {
		t.Fatalf("verified state = %+v", last)
	}
}

func TestEngramStaleExecutableResolutionExecutesZeroActions(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{
		availableEngramResolution("/opt/homebrew/bin/engram-v1"),
		availableEngramResolution("/opt/homebrew/bin/engram-v2"),
	}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || !strings.Contains(err.Error(), "executable resolution changed") {
		t.Fatalf("error = %v", err)
	}
	if adapter.inspectCalls != 2 || len(store.saves) != 0 || len(executor.actions) != 0 {
		t.Fatalf("stale executable caused effects: inspect=%d saves=%d external=%d", adapter.inspectCalls, len(store.saves), len(executor.actions))
	}
}

func TestEngramStaleIntentExecutesZeroActions(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	store.state.Intent.Revision = 9
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || !strings.Contains(err.Error(), "intent revision changed") {
		t.Fatalf("error = %v", err)
	}
	if adapter.inspectCalls != 2 || len(store.saves) != 0 || len(executor.actions) != 0 {
		t.Fatalf("stale intent caused effects: inspect=%d saves=%d external=%d", adapter.inspectCalls, len(store.saves), len(executor.actions))
	}
}

func TestEngramLocalFailureRunsNoExternalEffect(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"))
	adapter.applyErr = errors.New("local projection failed")
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if err == nil || len(executor.actions) != 0 || len(store.saves) != 1 || store.saves[0].Journal == nil {
		t.Fatalf("local failure facts/effects: err=%v external=%d saves=%d", err, len(executor.actions), len(store.saves))
	}
}

func TestEngramExternalFailureStopsLaterActionsAndKeepsRecoveryFacts(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{missingEngramResolution()}}
	executor := &fakeExternalExecutor{failID: "external:engram:setup:opencode", failErr: errors.New("setup failed")}
	facade, _, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"), engramObservation("ready"))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if err == nil || !strings.Contains(err.Error(), "later actions stopped") {
		t.Fatalf("error = %v", err)
	}
	if len(executor.actions) != 2 || executor.actions[1].ID != "external:engram:setup:opencode" {
		t.Fatalf("external barrier actions = %#v", executor.actions)
	}
	state := store.state
	if state.Journal == nil || state.Journal.FailedAction != "external:engram:setup:opencode" || !reflect.DeepEqual(state.Journal.Completed, []string{"instruction:engram-memory", "mcp_server:engram", "external:engram:acquire"}) || len(state.Ownership) != 0 {
		t.Fatalf("recovery state = %+v", state)
	}
}

func TestEngramVerificationFailureDoesNotClaimOwnership(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	executor := &fakeExternalExecutor{}
	facade, _, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"), engramObservation("still-missing"))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true})
	if !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("error = %v", err)
	}
	if len(executor.actions) != 0 || len(store.state.Ownership) != 0 || store.state.Journal == nil {
		t.Fatalf("verification failure state/effects = %+v external=%d", store.state, len(executor.actions))
	}
}

func TestEngramConvergedActivationIsNoOpAfterExternalEffects(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	executor := &fakeExternalExecutor{}
	ready := engramObservation("ready")
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"), ready, ready)
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal), facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	saves, calls := len(store.saves), len(executor.actions)
	repeat, err := facade.Preview(context.Background(), ActivationRequest{PackID: "engram", Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if !repeat.NoOp() || len(repeat.Phases()) != 1 || repeat.Phases()[0].Kind != ConsentHostFollowUp {
		t.Fatalf("repeat plan = %#v", repeat)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: repeat, Interactive: false}); err != nil {
		t.Fatal(err)
	}
	if len(store.saves) != saves || len(executor.actions) != calls || adapter.inspectCalls < 4 {
		t.Fatalf("no-op caused effects: saves=%d/%d external=%d/%d inspect=%d", len(store.saves), saves, len(executor.actions), calls, adapter.inspectCalls)
	}
}
