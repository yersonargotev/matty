package capabilitypack

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// PreviewReconcile repairs the complete desired state implied by existing
// activation intents. An empty PackID selects every active pack on the surface.
func (f Facade) PreviewReconcile(ctx context.Context, request ReconcileRequest) (ReconciliationPlan, error) {
	if request.PackID != "" {
		activation := ActivationRequest{PackID: request.PackID, Surface: request.Surface}
		_, _, state, err := f.activationInputsForOperation(ctx, activation, OperationReconcile)
		if err != nil {
			return ReconciliationPlan{}, err
		}
		intent, ok := intentForPack(state, request.PackID, request.Surface)
		if !ok || !intent.Active {
			return ReconciliationPlan{}, fmt.Errorf("capability pack %q is not active on %s; reconcile does not activate packs", request.PackID, request.Surface)
		}
		if _, err := f.catalog.resolveIntentPack(request.PackID, intent.Version); err != nil {
			return ReconciliationPlan{}, err
		}
		plan, err := f.preview(ctx, activation, OperationReconcile, "")
		if err != nil {
			return ReconciliationPlan{}, err
		}
		plan.reconcileScope = ReconcileTargeted
		plan.limitReconcileTo(request.PackID)
		plan.seal()
		return plan, nil
	}

	if f.activation == nil || f.activation.store == nil {
		return ReconciliationPlan{}, fmt.Errorf("activation is not configured")
	}
	if request.Surface != SurfaceCodex && request.Surface != SurfaceOpenCode {
		return ReconciliationPlan{}, fmt.Errorf("reconcile does not support CLI surface %q", request.Surface)
	}
	state, err := f.activation.store.Load(ctx, request.Surface)
	if err != nil {
		return ReconciliationPlan{}, err
	}
	ids := make([]string, 0)
	for _, intent := range activeIntents(state) {
		if intent.Active && intent.Surface == request.Surface {
			if _, showErr := f.catalog.resolveIntentPack(intent.PackID, intent.Version); showErr != nil {
				return ReconciliationPlan{}, showErr
			}
			ids = append(ids, intent.PackID)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return ReconciliationPlan{}, fmt.Errorf("no active capability packs on %s; reconcile does not activate packs", request.Surface)
	}
	plan, err := f.preview(ctx, ActivationRequest{PackID: ids[0], Surface: request.Surface}, OperationReconcile, "")
	if err != nil {
		return ReconciliationPlan{}, err
	}
	plan.reconcileScope = ReconcileSurfaceWide
	plan.seal()
	return plan, nil
}

func (p *ReconciliationPlan) limitReconcileTo(packID string) {
	inScope := func(id string) bool {
		for _, contributor := range p.actionContributors(id) {
			if contributor == packID {
				return true
			}
		}
		return false
	}
	for i := range p.phases {
		actions := p.phases[i].Actions[:0]
		for _, action := range p.phases[i].Actions {
			if action.Kind == ActionHostFollowUp || inScope(action.ID) {
				actions = append(actions, action)
			}
		}
		p.phases[i].Actions = actions
	}
	phases := p.phases[:0]
	for _, phase := range p.phases {
		if len(phase.Actions) > 0 {
			phases = append(phases, phase)
		}
	}
	p.phases = phases
	desired := p.desired[:0]
	for _, expectation := range p.desired {
		if inScope(expectation.ID) {
			desired = append(desired, expectation)
		}
	}
	p.desired = desired
	p.noOp = len(p.phases) == 0 && len(p.blockers) == 0 && len(p.pendingHumanActions) == 0
}

func (p *ReconciliationPlan) actionContributors(id string) []string {
	matched := map[string]bool{}
	for key, contributors := range p.contributors {
		resourceID := key
		if colon := strings.IndexByte(key, ':'); colon >= 0 {
			resourceID = key[colon+1:]
		}
		if key == id || strings.HasSuffix(key, ":"+id) || strings.HasSuffix(id, ":"+key) || strings.HasSuffix(id, ":"+resourceID) {
			for _, contributor := range contributors {
				matched[contributor] = true
			}
		}
	}
	if len(matched) == 0 {
		for _, owner := range p.ownershipFacts {
			if owner.ID == id {
				for _, contributor := range owner.Contributors {
					matched[contributor] = true
				}
			}
		}
	}
	if strings.HasPrefix(id, "external:") {
		for _, pack := range p.compositionFacts {
			for _, tool := range pack.Requires.Tools {
				if strings.Contains(id, ":"+tool+":") || strings.HasSuffix(id, ":"+tool) {
					matched[pack.ID] = true
				}
			}
		}
	}
	result := make([]string, 0, len(matched))
	for contributor := range matched {
		result = append(result, contributor)
	}
	sort.Strings(result)
	return result
}
