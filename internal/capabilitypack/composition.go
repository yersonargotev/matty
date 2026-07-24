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
	BlockerAlias                    BlockerKind    = "alias"
	BlockerSharing                  BlockerKind    = "sharing"
	BlockerCompatibility            BlockerKind    = "compatibility"
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
	surface      Surface
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
		intent := intentByPackID(c.intentFacts, pack.ID)
		for _, tool := range pack.Requires.Tools {
			tools[tool] = true
		}
		for _, r := range pack.Resources {
			r = resourceWithSurfaceAlias(r, intent.Aliases, c.surface)
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

func (f Facade) compose(requested Pack, state ActivationState, surface Surface, useRequestedIntent bool) (composition, error) {
	result := composition{requested: requested, surface: surface, contributors: map[string][]string{}}
	selected := map[string]Pack{}
	active := activeIntents(state)
	activeIDs := map[string]bool{}
	for _, intent := range active {
		if !intent.Active || intent.Surface != surface {
			continue
		}
		if intent.PackID == requested.ID && !useRequestedIntent {
			continue
		}
		pack, err := f.catalog.resolveIntentPack(intent.PackID, intent.Version)
		if err != nil {
			return composition{}, err
		}
		selected[pack.ID] = pack
		activeIDs[pack.ID] = true
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
	projectedNames := map[string]string{}
	for _, pack := range result.packs {
		intent := intentByPackID(active, pack.ID)
		for _, alias := range intent.Aliases {
			if !packHasAliasTarget(pack, alias, surface) {
				result.blockers = append(result.blockers, PlanBlocker{BlockerAlias, alias.Kind + ":" + alias.ID, "saved surface alias no longer targets a bound portable resource"})
			}
		}
		for _, resource := range pack.Resources {
			resolved := resourceWithSurfaceAlias(resource, intent.Aliases, surface)
			key := resource.Kind + ":" + resource.ID
			if previous, ok := resources[key]; ok {
				if digestJSON(previous) != digestJSON(resource) {
					result.blockers = append(result.blockers, PlanBlocker{BlockerIncompatibleContribution, key, "contributors declare different portable resources"})
					continue
				}
				previousSharing, currentSharing := bindingSharing(previous, surface), bindingSharing(resource, surface)
				if (previousSharing != "" || currentSharing != "") && (previousSharing != "shared" || currentSharing != "shared") {
					result.blockers = append(result.blockers, PlanBlocker{BlockerSharing, key, "every contributor must explicitly declare shared for an overlapping surface binding"})
				}
			}
			resources[key] = resource
			result.contributors[key] = append(result.contributors[key], pack.ID)
			if projectionID, ok := effectiveProjectionID(resolved, surface); ok && projectionID != key {
				result.contributors[projectionID] = append(result.contributors[projectionID], pack.ID)
			}
			if namespace, name, ok := projectedNamespace(resolved, surface); ok {
				projection := namespace + ":" + name
				if prior, exists := projectedNames[projection]; exists && prior != key {
					result.blockers = append(result.blockers, PlanBlocker{BlockerAlias, projection, fmt.Sprintf("portable resources %s and %s collide in the %s namespace; declare an explicit alias", prior, key, surface)})
				} else {
					projectedNames[projection] = key
				}
			}
		}
		if compatibilityFor(pack, surface) == CompatibilityBlocked {
			result.blockers = append(result.blockers, PlanBlocker{BlockerCompatibility, pack.ID, "declared surface outcomes do not form a compatible runtime dependency closure"})
		}
	}
	for key := range result.contributors {
		sort.Strings(result.contributors[key])
	}
	sortBlockers(result.blockers)
	return result, nil
}

func projectedNamespace(resource Resource, surface Surface) (string, string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface != surface {
			continue
		}
		switch binding.Projection {
		case "skill":
			return "personal-skill", binding.Name, true
		case "agent":
			return "agent", binding.Name, true
		case "mcp_server":
			return "mcp", binding.Name, true
		case "command_hook":
			if binding.Hook == nil {
				return "hook", binding.Name, true
			}
			return "hook", binding.Hook.Event + ":" + binding.Hook.Matcher + ":" + binding.Name, true
		}
	}
	return "", "", false
}

func effectiveProjectionID(resource Resource, surface Surface) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == surface && binding.Name != "" {
			return resource.Kind + ":" + binding.Name, true
		}
	}
	return "", false
}

func resourceWithSurfaceAlias(resource Resource, aliases []SurfaceAlias, surface Surface) Resource {
	for _, alias := range aliases {
		if alias.Kind != resource.Kind || alias.ID != resource.ID {
			continue
		}
		resource.Bindings = append([]Binding(nil), resource.Bindings...)
		for i := range resource.Bindings {
			if resource.Bindings[i].Surface != surface {
				continue
			}
			resource.Bindings[i].Name = alias.Name
			switch resource.Bindings[i].Projection {
			case "skill":
				resource.Bindings[i].Invocation = "$" + alias.Name
			case "command":
				resource.Bindings[i].Invocation = "/" + alias.Name
			case "agent":
				resource.Bindings[i].Invocation = "@" + alias.Name
			}
		}
		return resource
	}
	return resource
}

func intentByPackID(intents []ActivationIntent, packID string) ActivationIntent {
	for _, intent := range intents {
		if intent.PackID == packID {
			return intent
		}
	}
	return ActivationIntent{}
}

func packHasAliasTarget(pack Pack, alias SurfaceAlias, surface Surface) bool {
	for _, resource := range pack.Resources {
		if resource.Kind == alias.Kind && resource.ID == alias.ID && bindingSharing(resource, surface) != "" {
			return true
		}
	}
	return false
}

func bindingSharing(resource Resource, surface Surface) string {
	for _, binding := range resource.Bindings {
		if binding.Surface == surface {
			return binding.Sharing
		}
	}
	return ""
}

// composeWithout builds the complete desired state for a surface after one
// active pack is removed. The requested pack is never reintroduced through a
// dependency: callers must reject the sealed dependent facts instead.
func (f Facade) composeWithout(requested Pack, state ActivationState, surface Surface) (composition, []ActiveDependent, error) {
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
		pack, err := f.catalog.resolveIntentPack(intent.PackID, intent.Version)
		if err != nil {
			return composition{}, nil, err
		}
		for _, dependency := range pack.Requires.Capabilities {
			if provided[dependency] {
				dependents = append(dependents, ActiveDependent{PackID: pack.ID, Dependency: dependency})
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
		return composition{requested: Pack{ID: requested.ID, Surfaces: []Surface{surface}}, contributors: map[string][]string{}}, dependents, nil
	}
	root, err := f.catalog.resolveIntentPack(remaining[0].PackID, remaining[0].Version)
	if err != nil {
		return composition{}, nil, err
	}
	result, err := f.compose(root, targetState, surface, true)
	if err != nil {
		return composition{}, nil, err
	}
	result.activations = nil
	return result, dependents, nil
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
