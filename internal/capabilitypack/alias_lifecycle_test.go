package capabilitypack

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestActivateAliasIsCanonicalSealedIntentAndPersistsOnApply(t *testing.T) {
	pack := Pack{ID: "addy", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "command", ID: "review", Source: "review.md", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "review", Invocation: "$review", Mode: "degraded", Degradation: "codex-command-as-workflow-skill", Sharing: "exclusive"}}}}}
	pending := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "workflow:review", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "workflow:review"}}}}
	verified := pending
	verified.Projections = append([]ObservedProjection(nil), pending.Projections...)
	verified.Projections[0].ObservedFingerprint = "new"
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, verified}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	want := []SurfaceAlias{{Kind: "command", ID: "review", Name: "addy-review"}}
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "addy", Surface: SurfaceCodex, Aliases: want})
	if err != nil || !reflect.DeepEqual(plan.Aliases(), want) {
		t.Fatalf("plan aliases=%+v err=%v", plan.Aliases(), err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err != nil || !reflect.DeepEqual(store.state.Intent.Aliases, want) {
		t.Fatalf("persisted aliases=%+v err=%v", store.state.Intent.Aliases, err)
	}
}

func TestAliasRequestsValidatePortableIdentityAndReconcileScope(t *testing.T) {
	pack := Pack{ID: "addy", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "command", ID: "review", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "review", Invocation: "$review", Mode: "degraded", Degradation: "codex-command-as-workflow-skill", Sharing: "exclusive"}}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1", Active: true}}
	facade, _, _ := reconcileFixture([]Pack{pack}, state, SurfaceInspection{Revision: "host"})
	_, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "addy", Surface: SurfaceCodex, Aliases: []SurfaceAlias{{Kind: "command", ID: "missing", Name: "alias"}}})
	if err == nil || !strings.Contains(err.Error(), "does not identify") {
		t.Fatalf("invalid identity err=%v", err)
	}
	_, err = facade.PreviewReconcile(context.Background(), ReconcileRequest{Surface: SurfaceCodex, Aliases: []SurfaceAlias{{Kind: "command", ID: "review", Name: "alias"}}})
	if err == nil || !strings.Contains(err.Error(), "surface-wide") {
		t.Fatalf("surface-wide alias err=%v", err)
	}
}

func TestOmittedAliasInputPreservesDurableIntentOnUpdateAndTargetedReconcile(t *testing.T) {
	pack := Pack{
		ID: "addy", Version: "2", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{{
			Kind: "command", ID: "review",
			Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "review", Invocation: "$review", Mode: "degraded", Degradation: "codex-command-as-workflow-skill", Sharing: "exclusive"}},
		}},
	}
	want := []SurfaceAlias{{Kind: "command", ID: "review", Name: "addy-review"}}
	state := ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 3, Aliases: want}}
	inspection := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "workflow:review", ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "workflow:review"}}}}
	facade, _, _ := updateFixture([]Pack{pack}, state, inspection)
	update, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil || !reflect.DeepEqual(update.Aliases(), want) {
		t.Fatalf("update aliases=%+v err=%v", update.Aliases(), err)
	}
	facade, _, _ = reconcileFixture([]Pack{pack}, state, inspection)
	reconcile, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil || !reflect.DeepEqual(reconcile.Aliases(), want) {
		t.Fatalf("reconcile aliases=%+v err=%v", reconcile.Aliases(), err)
	}
}
