package capabilitypack

import (
	"encoding/json"
	"reflect"
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

func TestReconciliationPlanJSONReportIsDeterministicAndComplete(t *testing.T) {
	plan := ReconciliationPlan{id: "p", digest: "d", pack: Pack{ID: "addy", Version: "1.0.0"}, operation: OperationActivate,
		surface: SurfaceCodex, intentRevision: 3, aliases: []SurfaceAlias{{Kind: "skill", ID: "z", Name: "z"}}, recovery: true,
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
	if got.Disposition != PlanMixed || !got.Recovery || got.IntentRevision != 3 {
		t.Fatalf("facts = %#v", got)
	}
	if !reflect.DeepEqual(got.Contributors["projection"], []string{"a", "z"}) || got.MandatoryActions[0].ID != "a" {
		t.Fatalf("canonical facts = %#v", got)
	}
}
