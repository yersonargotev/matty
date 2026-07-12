package capabilitypack

import (
	"fmt"
	"sort"
	"strings"
)

type ActivationRole string
type BlockerKind string

const (
	ActivationRequested             ActivationRole = "requested"
	ActivationRequired              ActivationRole = "required"
	BlockerDependency               BlockerKind    = "dependency"
	BlockerCapabilityConflict       BlockerKind    = "capability-conflict"
	BlockerIncompatibleContribution BlockerKind    = "incompatible-contribution"
	BlockerOwnership                BlockerKind    = "ownership"
	BlockerGlobalRequirement        BlockerKind    = "global-requirement"
	BlockerActiveDependent          BlockerKind    = "active-dependent"
)

type PlannedActivation struct {
	Pack Pack
	Role ActivationRole
}

type ActiveDependent struct {
	PackID     string
	Dependency string
}

type PlanBlocker struct {
	Kind            BlockerKind
	Subject, Detail string
}

type composition struct {
	requested    Pack
	packs        []Pack
	activations  []PlannedActivation
	contributors map[string][]string
	blockers     []PlanBlocker
	intentFacts  []ActivationIntent
}

func (c composition) combinedPack() Pack {
	p := clonePack(c.requested)
	p.Resources = nil
	p.Requires.Capabilities = nil
	p.Requires.Tools = nil
	resources := map[string]Resource{}
	tools := map[string]bool{}
	for _, pack := range c.packs {
		for _, tool := range pack.Requires.Tools {
			tools[tool] = true
		}
		for _, r := range pack.Resources {
			key := r.Kind + ":" + r.ID
			if _, ok := resources[key]; !ok {
				resources[key] = r
			}
		}
	}
	keys := make([]string, 0, len(resources))
	for key := range resources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p.Resources = append(p.Resources, resources[key])
	}
	for tool := range tools {
		p.Requires.Tools = append(p.Requires.Tools, tool)
	}
	sort.Strings(p.Requires.Tools)
	return p
}

func (c composition) contributorSet(projectionID string) []string {
	if values := c.contributors[projectionID]; len(values) > 0 {
		return append([]string(nil), values...)
	}
	matched := map[string]bool{}
	for key, values := range c.contributors {
		resourceID := key
		if colon := strings.IndexByte(key, ':'); colon >= 0 {
			resourceID = key[colon+1:]
		}
		if key == projectionID || strings.HasSuffix(key, ":"+projectionID) || strings.HasSuffix(projectionID, ":"+key) || strings.HasSuffix(projectionID, ":"+resourceID) {
			for _, value := range values {
				matched[value] = true
			}
		}
	}
	if len(matched) > 0 {
		values := make([]string, 0, len(matched))
		for value := range matched {
			values = append(values, value)
		}
		sort.Strings(values)
		return values
	}
	ids := make([]string, len(c.packs))
	for i, p := range c.packs {
		ids[i] = p.ID
	}
	sort.Strings(ids)
	return ids
}

func (f Facade) compose(requested Pack, state ActivationState, surface Surface) composition {
	result := composition{requested: requested, contributors: map[string][]string{}}
	selected := map[string]Pack{}
	active := activeIntents(state)
	activeIDs := map[string]bool{}
	for _, intent := range active {
		if !intent.Active || intent.Surface != surface {
			continue
		}
		if pack, err := f.catalog.Show(intent.PackID); err == nil {
			selected[pack.ID] = pack
			activeIDs[pack.ID] = intent.Active && intent.Version == pack.Version
		}
	}
	var visit func(Pack, ActivationRole)
	visiting := map[string]bool{}
	expanded := map[string]bool{}
	roles := map[string]ActivationRole{requested.ID: ActivationRequested}
	visit = func(pack Pack, role ActivationRole) {
		if existing, ok := selected[pack.ID]; ok {
			pack = existing
		}
		if expanded[pack.ID] {
			return
		}
		if visiting[pack.ID] {
			result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, pack.ID, "dependency cycle prevents a deterministic closure"})
			return
		}
		selected[pack.ID] = pack
		if _, ok := roles[pack.ID]; !ok && !activeIDs[pack.ID] {
			roles[pack.ID] = role
		}
		visiting[pack.ID] = true
		for _, capability := range pack.Requires.Capabilities {
			providers := f.providers(capability, surface)
			if len(providers) != 1 {
				detail := "required capability has no provider"
				if len(providers) > 1 {
					detail = "required capability has multiple providers"
				}
				result.blockers = append(result.blockers, PlanBlocker{BlockerDependency, capability, detail})
				continue
			}
			visit(providers[0], ActivationRequired)
		}
		delete(visiting, pack.ID)
		expanded[pack.ID] = true
	}
	visit(requested, ActivationRequested)
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		result.packs = append(result.packs, selected[id])
		if role, ok := roles[id]; ok {
			result.activations = append(result.activations, PlannedActivation{selected[id], role})
		}
	}
	for _, intent := range active {
		if _, ok := selected[intent.PackID]; ok {
			result.intentFacts = append(result.intentFacts, intent)
		}
	}
	sort.Slice(result.intentFacts, func(i, j int) bool { return result.intentFacts[i].PackID < result.intentFacts[j].PackID })
	provided := map[string]string{}
	for _, pack := range result.packs {
		for _, capability := range pack.Provides {
			provided[capability] = pack.ID
		}
	}
	for _, pack := range result.packs {
		for _, conflict := range pack.Conflicts {
			if other, ok := provided[conflict]; ok && other != pack.ID {
				result.blockers = append(result.blockers, PlanBlocker{BlockerCapabilityConflict, conflict, fmt.Sprintf("pack %s conflicts with capability provided by %s", pack.ID, other)})
			}
		}
	}
	resources := map[string]Resource{}
	for _, pack := range result.packs {
		for _, resource := range pack.Resources {
			key := resource.Kind + ":" + resource.ID
			if previous, ok := resources[key]; ok && digestJSON(previous) != digestJSON(resource) {
				result.blockers = append(result.blockers, PlanBlocker{BlockerIncompatibleContribution, key, "contributors declare different portable resources"})
				continue
			}
			resources[key] = resource
			result.contributors[key] = append(result.contributors[key], pack.ID)
		}
	}
	for key := range result.contributors {
		sort.Strings(result.contributors[key])
	}
	sortBlockers(result.blockers)
	return result
}

// composeWithout builds the complete desired state for a surface after one
// active pack is removed. The requested pack is never reintroduced through a
// dependency: callers must reject the sealed dependent facts instead.
func (f Facade) composeWithout(requested Pack, state ActivationState, surface Surface) (composition, []ActiveDependent) {
	targetState := cloneActivationState(state)
	active := activeIntents(state)
	remaining := make([]ActivationIntent, 0, len(active))
	provided := map[string]bool{}
	for _, capability := range requested.Provides {
		provided[capability] = true
	}
	var dependents []ActiveDependent
	for _, intent := range active {
		if intent.Surface != surface || !intent.Active {
			continue
		}
		if intent.PackID == requested.ID {
			continue
		}
		remaining = append(remaining, intent)
		if pack, err := f.catalog.Show(intent.PackID); err == nil {
			for _, dependency := range pack.Requires.Capabilities {
				if provided[dependency] {
					dependents = append(dependents, ActiveDependent{PackID: pack.ID, Dependency: dependency})
				}
			}
		}
	}
	sort.Slice(dependents, func(i, j int) bool {
		if dependents[i].PackID == dependents[j].PackID {
			return dependents[i].Dependency < dependents[j].Dependency
		}
		return dependents[i].PackID < dependents[j].PackID
	})
	targetState.Intents = remaining
	targetState.Intent = ActivationIntent{Surface: surface, Revision: state.Intent.Revision}
	if len(remaining) == 0 {
		return composition{requested: Pack{ID: requested.ID, Surfaces: []Surface{surface}}, contributors: map[string][]string{}}, dependents
	}
	root, err := f.catalog.Show(remaining[0].PackID)
	if err != nil {
		return composition{requested: requested, contributors: map[string][]string{}, blockers: []PlanBlocker{{Kind: BlockerDependency, Subject: remaining[0].PackID, Detail: err.Error()}}}, dependents
	}
	result := f.compose(root, targetState, surface)
	result.activations = nil
	return result, dependents
}

func (c composition) identityDigest() string {
	return digestJSON(struct {
		Packs        []Pack
		Activations  []PlannedActivation
		Contributors map[string][]string
		Blockers     []PlanBlocker
		Intents      []ActivationIntent
	}{c.packs, c.activations, c.contributors, c.blockers, c.intentFacts})
}

func (f Facade) providers(capability string, surface Surface) []Pack {
	var result []Pack
	for _, pack := range f.catalog.List() {
		if supportsSurface(pack, surface) {
			for _, provided := range pack.Provides {
				if provided == capability {
					result = append(result, pack)
				}
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func supportsSurface(pack Pack, surface Surface) bool {
	for _, item := range pack.Surfaces {
		if item == surface {
			return true
		}
	}
	return false
}
func sortBlockers(values []PlanBlocker) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		if values[i].Subject != values[j].Subject {
			return values[i].Subject < values[j].Subject
		}
		return values[i].Detail < values[j].Detail
	})
}
func activeIntents(state ActivationState) []ActivationIntent {
	if len(state.Intents) > 0 {
		return append([]ActivationIntent(nil), state.Intents...)
	}
	if state.Intent.Active {
		return []ActivationIntent{state.Intent}
	}
	return nil
}
