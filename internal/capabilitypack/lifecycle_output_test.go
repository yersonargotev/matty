package capabilitypack

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLifecycleContractForIsCanonicalAndSurfaceScoped(t *testing.T) {
	pack := Pack{ID: "addy", Version: "1.0.0", Requires: Requirements{Capabilities: []string{"z", "a", "a"}},
		Resources: []Resource{
			{Kind: "agent", ID: "reviewer", Permissions: []string{"network", "filesystem", "network"}, Bindings: []Binding{
				{Surface: SurfaceOpenCode, Projection: "agent", Name: "addy-reviewer", Invocation: "@addy-reviewer", Mode: "native", Sharing: "exclusive"},
				{Surface: SurfaceCodex, Projection: "agent", Name: "reviewer", Invocation: "delegate", Mode: "degraded", Degradation: "no nested delegation", Sharing: "exclusive"},
			}},
			{Kind: "skill", ID: "run", Permissions: []string{"process"}, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "run", Invocation: "$run", Mode: "native", Sharing: "shared"}}},
		}, Contract: Contract{
			Exclusions:    []Exclusion{{ID: "hooks", SourcePaths: []string{"z", "a"}, Reason: "excluded"}},
			OptionalModes: []OptionalMode{{ID: "deploy", Authorities: []string{"write", "network"}, Fallback: "prompt"}},
		}}
	aliases := []SurfaceAlias{{Kind: "skill", ID: "run", Name: "z"}, {Kind: "agent", ID: "reviewer", Name: "a"}}
	got := LifecycleContractFor(pack, SurfaceCodex, aliases)
	if got.Counts != (ResourceCounts{Agents: 1, Skills: 1}) {
		t.Fatalf("counts = %#v", got.Counts)
	}
	if !reflect.DeepEqual(got.DependencyClosure, []string{"a", "z"}) {
		t.Fatalf("closure = %#v", got.DependencyClosure)
	}
	if len(got.Bindings) != 2 || got.Bindings[0].ID != "reviewer" || got.Bindings[0].Degradation != "no nested delegation" {
		t.Fatalf("bindings = %#v", got.Bindings)
	}
	if !reflect.DeepEqual(got.PromptAuthorities, []string{"filesystem", "network", "process", "write"}) {
		t.Fatalf("authorities = %#v", got.PromptAuthorities)
	}
	if got.Aliases[0].Kind != "agent" || !reflect.DeepEqual(got.Exclusions[0].SourcePaths, []string{"a", "z"}) {
		t.Fatalf("contract not canonical: %#v", got)
	}
	if pack.Contract.Exclusions[0].SourcePaths[0] != "z" {
		t.Fatal("derivation mutated pack")
	}
}

func TestLifecycleOutputsEncodeCollectionsAsArrays(t *testing.T) {
	contract := LifecycleContractFor(Pack{}, SurfaceCodex, nil)
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"dependency_closure", "bindings", "exclusions", "optional_modes", "prompt_authorities", "aliases"} {
		if _, ok := decoded[key].([]any); !ok {
			t.Fatalf("%s is not a JSON array: %s", key, encoded)
		}
	}
}

func TestLifecycleCompatibilityIsIndependentFromReadinessAndIntent(t *testing.T) {
	resource := func(binding *Binding, exclusion *SurfaceExclusion) Resource {
		r := Resource{Kind: "instruction", ID: "guide"}
		if binding != nil {
			r.Bindings = []Binding{*binding}
		}
		if exclusion != nil {
			r.SurfaceExclusions = []SurfaceExclusion{*exclusion}
		}
		return r
	}
	tests := []struct {
		name string
		pack Pack
		want Compatibility
	}{
		{"complete", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{resource(&Binding{Surface: SurfaceClaude, Mode: "native"}, nil)}}, CompatibilityComplete},
		{"degraded binding", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{resource(&Binding{Surface: SurfaceClaude, Mode: "degraded", Degradation: "fallback"}, nil)}}, CompatibilityDegraded},
		{"optional exclusion", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{resource(nil, &SurfaceExclusion{Surface: SurfaceClaude, Mode: "optional"})}}, CompatibilityDegraded},
		{"mandatory exclusion", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{resource(nil, &SurfaceExclusion{Surface: SurfaceClaude, Mode: "mandatory"})}}, CompatibilityBlocked},
		{"missing outcome", Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}, CompatibilityBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := LifecycleContractFor(tt.pack, SurfaceClaude, nil)
			if contract.Compatibility != tt.want {
				t.Fatalf("compatibility = %q, want %q", contract.Compatibility, tt.want)
			}
		})
	}
}

func TestLifecycleCompatibilityBlocksExcludedDependencyAndRendersSurfaceExclusion(t *testing.T) {
	pack := Pack{manifestVersion: manifestSchemaV3, Resources: []Resource{
		{Kind: "instruction", ID: "guide", Requires: []string{"lifecycle:memory"}, Bindings: []Binding{{Surface: SurfaceClaude, Mode: "native"}}},
		{Kind: "lifecycle", ID: "memory", SurfaceExclusions: []SurfaceExclusion{{Surface: SurfaceClaude, Mode: "optional", Code: "generic-lifecycle-unsupported", Reason: "requires an explicit typed hook"}}},
	}}
	contract := LifecycleContractFor(pack, SurfaceClaude, nil)
	if contract.Compatibility != CompatibilityBlocked {
		t.Fatalf("compatibility = %s", contract.Compatibility)
	}
	if len(contract.Exclusions) != 1 || contract.Exclusions[0].ID != "lifecycle:memory" || contract.Exclusions[0].Code != "generic-lifecycle-unsupported" || contract.Exclusions[0].Mode != "optional" {
		t.Fatalf("exclusions = %#v", contract.Exclusions)
	}

	pack.Resources[0].Requires = []string{}
	if got := LifecycleContractFor(pack, SurfaceClaude, nil).Compatibility; got != CompatibilityDegraded {
		t.Fatalf("independent optional exclusion = %s", got)
	}
}

func TestReconciliationPlanJSONReportIsDeterministicAndComplete(t *testing.T) {
	plan := ReconciliationPlan{id: "p", digest: "d", pack: Pack{ID: "addy", Version: "1.0.0", manifestVersion: manifestSchemaV3}, operation: OperationActivate,
		surface: SurfaceCodex, intentRevision: 3, aliases: []SurfaceAlias{{Kind: "skill", ID: "z", Name: "z"}}, recovery: true,
		readiness: ReadinessStatus{Configured: false}, readinessObserved: ReadinessObservationStatus{Configured: true}, pendingEvidence: []string{"z", "a"},
		contributors: map[string][]string{"projection": {"z", "a"}}, blockers: []PlanBlocker{{Kind: BlockerAlias, Subject: "z", Detail: "collision"}},
		pendingHumanActions: []string{"z", "a"}, phases: []PlanPhase{{Kind: ConsentReversibleLocal, Digest: "phase", ApprovalRequired: true,
			Actions: []ProjectionAction{{ID: "z"}, {ID: "a"}}}}}
	first, err := json.Marshal(plan.JSONReport(true))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(plan.JSONReport(true))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("unstable JSON:\n%s\n%s", first, second)
	}
	got := plan.JSONReport(true)
	if got.SchemaVersion != LifecycleJSONSchemaVersion || got.Disposition != PlanMixed || !got.Recovery || got.IntentRevision != 3 {
		t.Fatalf("facts = %#v", got)
	}
	if got.Contract.Compatibility != CompatibilityBlocked || got.ExpectedReadiness.Configured || !got.ReadinessObserved.Configured || !reflect.DeepEqual(got.PendingEvidence, []string{"a", "z"}) {
		t.Fatalf("planned lifecycle facts = %#v", got)
	}
	failure := JSONFailureFor("apply", ErrStalePlan, &plan, nil, nil)
	if failure.SchemaVersion != LifecycleJSONSchemaVersion || failure.Plan.Contract.Compatibility != CompatibilityBlocked {
		t.Fatalf("stale failure lifecycle facts = %#v", failure)
	}
	failureJSON, err := json.Marshal(failure)
	if err != nil || !json.Valid(failureJSON) || !strings.Contains(string(failureJSON), `"compatibility":"blocked"`) {
		t.Fatalf("stale failure wire contract = %s, err=%v", failureJSON, err)
	}
	if !reflect.DeepEqual(got.Contributors["projection"], []string{"a", "z"}) || got.MandatoryActions[0].ID != "a" {
		t.Fatalf("canonical facts = %#v", got)
	}
}

func TestLifecycleReportRedactsSealedExternalHostContent(t *testing.T) {
	plan := ReconciliationPlan{pack: Pack{ID: "p", Version: "1"}, phases: []PlanPhase{{Kind: ConsentExecutableExternal, Actions: []ProjectionAction{{ID: "hook:x", Consent: ConsentExecutableExternal, Content: "foreign-secret", Description: "event=SessionStart command=engram"}}}}}
	encoded, err := json.Marshal(plan.JSONReport(true))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "foreign-secret") || !strings.Contains(string(encoded), "event=SessionStart command=engram") {
		t.Fatalf("report = %s", encoded)
	}
}
