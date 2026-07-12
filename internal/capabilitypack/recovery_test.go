package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRepeatUpdateRecoversTowardPersistedIntentWithoutNewRevision(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	pending := ActivationObservation{Revision: "host-1", Projections: []ObservedProjection{{ID: "instruction:guide", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide", Description: "write v2"}}}}
	verified := pending
	verified.Revision = "host-2"
	verified.Projections = append([]ObservedProjection(nil), pending.Projections...)
	verified.Projections[0].ObservedFingerprint = "new"
	history := ApplyingJournal{PlanID: "plan-old", PlanDigest: "old-digest", Operation: OperationUpdate, Surface: SurfaceCodex, PackID: "app", Outcome: "recovery-required", Actions: []string{"instruction:guide"}, FailedAction: "reversible-local", FailureDetail: "interrupted"}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "2.0.0", Active: true, Revision: 5}, Journal: &history, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
	facade, _, store := updateFixture([]Pack{pack}, state, pending, pending, verified)

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Recovery() || plan.ID() == history.PlanID || !reflect.DeepEqual(*plan.HistoricalAttempt(), history) {
		t.Fatalf("plan/history = %+v %+v", plan, plan.HistoricalAttempt())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if store.state.Intent.Revision != 5 || store.state.Intent.Version != "2.0.0" || len(store.state.History) != 1 || !reflect.DeepEqual(store.state.History[0], history) {
		t.Fatalf("state = %+v", store.state)
	}
}

func TestRepeatDeactivateRecoversPersistedInactiveIntentWithoutNewRevision(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	history := ApplyingJournal{PlanID: "plan-old", PlanDigest: "old-digest", Operation: OperationDeactivate, Surface: SurfaceCodex, PackID: "app", Outcome: "recovery-required", Actions: []string{"instruction:guide"}, FailedAction: "destructive-cleanup", FailureDetail: "interrupted"}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 8}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 8}}, Journal: &history, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "verified"}}}
	facade, _, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host-1", "verified", true), deletionObservation("host-1", "verified", true), deletionObservation("host-2", "", false))

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Recovery() || !plan.Applicable() || len(plan.Phases()) != 1 || plan.Phases()[0].Kind != ConsentDestructiveCleanup {
		t.Fatalf("recovery plan = %+v blockers=%+v", plan.Phases(), plan.Blockers())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if store.state.Intent.Revision != 8 || store.state.Intent.Active || len(store.state.History) != 1 || !reflect.DeepEqual(store.state.History[0], history) {
		t.Fatalf("state = %+v", store.state)
	}
}

func TestConvergedRecoveryStillRequiresFreshApprovalToCloseAttempt(t *testing.T) {
	ready := pendingObservation("missing")
	for i := range ready.Projections {
		ready.Projections[i].ObservedFingerprint = ready.Projections[i].DesiredFingerprint
	}
	facade, adapter, store := activationFixture(ready, ready)
	history := ApplyingJournal{PlanID: "plan-old", PlanDigest: "old-digest", Operation: OperationActivate, Surface: SurfaceCodex, PackID: "matty", Outcome: AttemptApplying, Actions: []string{"skill:ask-matt"}, Completed: []string{"skill:ask-matt"}}
	store.state = ActivationState{Intent: ActivationIntent{PackID: "matty", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3}, Journal: &history, Ownership: []ProjectionOwnership{{ID: "instruction:matty-guidance", Contributors: []string{"matty"}, Fingerprint: "instruction-new"}, {ID: "skill:ask-matt", Contributors: []string{"matty"}, Fingerprint: "skill-new"}}}

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "matty", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.NoOp() || !plan.Recovery() || len(plan.Phases()) != 1 || !plan.Phases()[0].ApprovalRequired {
		t.Fatalf("plan = noop:%v recovery:%v phases:%+v", plan.NoOp(), plan.Recovery(), plan.Phases())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("missing approval = %v", err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if len(adapter.actions) != 0 || store.state.Journal != nil || store.state.Intent.Revision != 3 || len(store.state.History) != 1 || store.state.History[0].Outcome != AttemptRecoveryRequired || store.state.History[0].FailedAction != "interrupted" {
		t.Fatalf("state/actions = %+v %+v", store.state, adapter.actions)
	}
}

func TestRecoveryBecomesStaleWithZeroEffectsWhenOwnershipChanges(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	pending := ActivationObservation{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}}}}
	history := ApplyingJournal{PlanID: "old", PlanDigest: "old-digest", Operation: OperationUpdate, Surface: SurfaceCodex, PackID: "app", Outcome: AttemptRecoveryRequired, Actions: []string{"instruction:guide"}, FailedAction: "instruction:guide"}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "2.0.0", Active: true, Revision: 4}, Journal: &history, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, pending)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	store.state.Ownership[0].Fingerprint = "concurrent-change"
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 || !reflect.DeepEqual(*store.state.Journal, history) {
		t.Fatalf("stale recovery effects: err=%v state=%+v", err, store.state)
	}
}

func TestDestructiveCompletionIsTruthfulWhenVerificationFails(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "verified"}}}
	stillPresent := deletionObservation("host-2", "verified", true)
	facade, _, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host-1", "verified", true), deletionObservation("host-1", "verified", true), stillPresent)
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
	if !errors.Is(err, ErrVerificationFailed) || store.state.Journal == nil || !reflect.DeepEqual(store.state.Journal.Completed, []string{"instruction:guide"}) || len(store.state.Journal.NotStarted()) != 0 || len(store.state.Ownership) != 1 {
		t.Fatalf("facts/state = err:%v %+v", err, store.state)
	}
}
