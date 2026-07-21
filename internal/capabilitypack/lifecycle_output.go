package capabilitypack

import (
	"errors"
	"sort"

	"github.com/yersonargotev/packy/internal/reportredaction"
)

const LifecycleJSONSchemaVersion = 2

// LifecycleContract is the canonical, host-neutral description rendered by
// every lifecycle entry point. Renderers must not reconstruct these facts
// from a manifest.
type LifecycleContract struct {
	Compatibility         Compatibility        `json:"compatibility,omitempty"`
	CompatibilityObserved bool                 `json:"-"`
	Counts                ResourceCounts       `json:"logical_resource_counts"`
	DependencyClosure     []string             `json:"dependency_closure"`
	Bindings              []LifecycleBinding   `json:"bindings"`
	Exclusions            []LifecycleExclusion `json:"exclusions"`
	OptionalModes         []OptionalMode       `json:"optional_modes"`
	PromptAuthorities     []string             `json:"prompt_authorities"`
	Aliases               []SurfaceAlias       `json:"aliases"`
	AuthorityDisclosure   string               `json:"authority_disclosure"`
}

// LifecycleExclusion is the rendered union of portable source exclusions and
// v3 surface outcomes. Surface exclusions retain the resource and stable code
// that explain compatibility without being mistaken for runtime projections.
type LifecycleExclusion struct {
	ID           string   `json:"id"`
	ResourceKind string   `json:"resource_kind,omitempty"`
	Surface      Surface  `json:"surface,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	Code         string   `json:"code,omitempty"`
	SourcePaths  []string `json:"source_paths"`
	Reason       string   `json:"reason"`
}

type Compatibility string

const (
	CompatibilityComplete Compatibility = "complete"
	CompatibilityDegraded Compatibility = "degraded"
	CompatibilityBlocked  Compatibility = "blocked"
)

type LifecycleBinding struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Projection  string `json:"projection"`
	Name        string `json:"name"`
	Invocation  string `json:"invocation"`
	Mode        string `json:"mode"`
	Degradation string `json:"degradation,omitempty"`
	Sharing     string `json:"sharing"`
}

// LifecycleContractFor derives the complete portable contract for one
// surface. Every slice is allocated so JSON preserves [] rather than null.
func LifecycleContractFor(pack Pack, surface Surface, aliases []SurfaceAlias) LifecycleContract {
	contract := LifecycleContract{
		Compatibility: compatibilityFor(pack, surface), CompatibilityObserved: pack.manifestVersion >= manifestSchemaV3,
		Counts: pack.ResourceCounts(), DependencyClosure: []string{}, Bindings: []LifecycleBinding{},
		Exclusions: []LifecycleExclusion{}, OptionalModes: []OptionalMode{}, PromptAuthorities: []string{}, Aliases: []SurfaceAlias{},
		AuthorityDisclosure: "Activation grants only the sealed local projection actions; later workflow effects require host approval.",
	}
	if !contract.CompatibilityObserved {
		contract.Compatibility = ""
	}
	contract.DependencyClosure = sortedUnique(pack.Requires.Capabilities)
	authorities := []string{}
	for _, resource := range pack.Resources {
		resolved := resourceWithSurfaceAlias(resource, aliases, surface)
		for _, binding := range resolved.Bindings {
			if binding.Surface != surface {
				continue
			}
			contract.Bindings = append(contract.Bindings, LifecycleBinding{
				Kind: resource.Kind, ID: resource.ID, Projection: binding.Projection, Name: binding.Name,
				Invocation: binding.Invocation, Mode: binding.Mode, Degradation: binding.Degradation, Sharing: binding.Sharing,
			})
			authorities = append(authorities, resource.Permissions...)
		}
	}
	sort.Slice(contract.Bindings, func(i, j int) bool {
		a, b := contract.Bindings[i], contract.Bindings[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Projection != b.Projection {
			return a.Projection < b.Projection
		}
		return a.Name < b.Name
	})
	contract.PromptAuthorities = sortedUnique(authorities)
	for _, exclusion := range pack.Contract.Exclusions {
		contract.Exclusions = append(contract.Exclusions, LifecycleExclusion{ID: exclusion.ID, SourcePaths: sortedUnique(exclusion.SourcePaths), Reason: exclusion.Reason})
	}
	for _, resource := range pack.Resources {
		for _, exclusion := range resource.SurfaceExclusions {
			if exclusion.Surface == surface {
				contract.Exclusions = append(contract.Exclusions, LifecycleExclusion{ID: resource.Kind + ":" + resource.ID, ResourceKind: resource.Kind, Surface: surface, Mode: exclusion.Mode, Code: exclusion.Code, SourcePaths: []string{}, Reason: exclusion.Reason})
			}
		}
	}
	for i := range contract.Exclusions {
		contract.Exclusions[i].SourcePaths = sortedUnique(contract.Exclusions[i].SourcePaths)
	}
	sort.Slice(contract.Exclusions, func(i, j int) bool {
		if contract.Exclusions[i].ID != contract.Exclusions[j].ID {
			return contract.Exclusions[i].ID < contract.Exclusions[j].ID
		}
		return contract.Exclusions[i].Code < contract.Exclusions[j].Code
	})
	contract.OptionalModes = append(contract.OptionalModes, pack.Contract.OptionalModes...)
	for i := range contract.OptionalModes {
		contract.OptionalModes[i].Authorities = sortedUnique(contract.OptionalModes[i].Authorities)
		authorities = append(authorities, contract.OptionalModes[i].Authorities...)
	}
	sort.Slice(contract.OptionalModes, func(i, j int) bool { return contract.OptionalModes[i].ID < contract.OptionalModes[j].ID })
	contract.PromptAuthorities = sortedUnique(authorities)
	contract.Aliases = append(contract.Aliases, aliases...)
	sort.Slice(contract.Aliases, func(i, j int) bool {
		if contract.Aliases[i].Kind != contract.Aliases[j].Kind {
			return contract.Aliases[i].Kind < contract.Aliases[j].Kind
		}
		if contract.Aliases[i].ID != contract.Aliases[j].ID {
			return contract.Aliases[i].ID < contract.Aliases[j].ID
		}
		return contract.Aliases[i].Name < contract.Aliases[j].Name
	})
	return contract
}

func compatibilityFor(pack Pack, surface Surface) Compatibility {
	if pack.manifestVersion < manifestSchemaV3 {
		return CompatibilityComplete
	}
	result := CompatibilityComplete
	resources := make(map[string]Resource, len(pack.Resources))
	included := make(map[string]bool, len(pack.Resources))
	for _, resource := range pack.Resources {
		resources[resource.Kind+":"+resource.ID] = resource
		if resource.Kind == "asset" || resource.Kind == "notice" {
			continue
		}
		outcome := false
		for _, binding := range resource.Bindings {
			if binding.Surface != surface {
				continue
			}
			outcome = true
			included[resource.Kind+":"+resource.ID] = true
			if binding.Mode != "native" || binding.Degradation != "" {
				result = CompatibilityDegraded
			}
		}
		for _, exclusion := range resource.SurfaceExclusions {
			if exclusion.Surface != surface {
				continue
			}
			outcome = true
			if exclusion.Mode == "mandatory" {
				return CompatibilityBlocked
			}
			result = CompatibilityDegraded
		}
		if !outcome {
			return CompatibilityBlocked
		}
	}
	// Assets have no standalone surface outcome. They participate only when a
	// compatible runtime consumer reaches them through its declared closure.
	visiting := map[string]bool{}
	var closureCompatible func(string) bool
	closureCompatible = func(identity string) bool {
		if visiting[identity] {
			return true
		}
		resource, ok := resources[identity]
		if !ok || resource.Kind == "notice" {
			return false
		}
		if resource.Kind != "asset" && !included[identity] {
			return false
		}
		visiting[identity] = true
		defer delete(visiting, identity)
		for _, dependency := range resource.Requires {
			if !closureCompatible(dependency) {
				return false
			}
		}
		return true
	}
	for identity := range included {
		if !closureCompatible(identity) {
			return CompatibilityBlocked
		}
	}
	return result
}

func (p ReconciliationPlan) LifecycleContract() LifecycleContract {
	contract := LifecycleContractFor(p.pack, p.surface, p.aliases)
	if contract.CompatibilityObserved && len(p.blockers) > 0 {
		contract.Compatibility = CompatibilityBlocked
	}
	return contract
}

func sortedUnique(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

type JSONLifecyclePhase struct {
	Kind             ConsentKind        `json:"kind"`
	Digest           string             `json:"digest"`
	ApprovalRequired bool               `json:"approval_required"`
	Actions          []ProjectionAction `json:"actions"`
}

type JSONLifecyclePlan struct {
	SchemaVersion       int                        `json:"schema_version"`
	Report              string                     `json:"report"`
	PlanID              string                     `json:"plan_id"`
	Operation           Operation                  `json:"operation"`
	Disposition         PlanDisposition            `json:"disposition"`
	Digest              string                     `json:"digest"`
	Pack                string                     `json:"pack"`
	PackVersion         string                     `json:"pack_version"`
	Surface             Surface                    `json:"surface"`
	IntentRevision      int                        `json:"intent_revision"`
	Contract            LifecycleContract          `json:"contract"`
	Aliases             []SurfaceAlias             `json:"aliases"`
	Contributors        map[string][]string        `json:"contributors"`
	Blockers            []PlanBlocker              `json:"blockers"`
	Phases              []JSONLifecyclePhase       `json:"phases"`
	PendingHumanActions []string                   `json:"pending_human_actions"`
	ExpectedReadiness   ReadinessStatus            `json:"expected_readiness"`
	ReadinessObserved   ReadinessObservationStatus `json:"readiness_observed"`
	Evidence            []string                   `json:"evidence"`
	PendingEvidence     []string                   `json:"pending_evidence"`
	Recovery            bool                       `json:"recovery"`
	MandatoryActions    []ProjectionAction         `json:"mandatory_actions"`
	ContractDiff        JSONContractDiff           `json:"contract_diff"`
	Migrations          []string                   `json:"migrations"`
	RetainedProjections []RetainedProjection       `json:"retained_projections"`
	RemovedContributors map[string]string          `json:"removed_contributors"`
	DryRun              bool                       `json:"dry_run"`
}

type JSONContractDiff struct {
	Added    []string `json:"added"`
	Changed  []string `json:"changed"`
	Removed  []string `json:"removed"`
	Retained []string `json:"retained"`
}

func (p ReconciliationPlan) JSONReport(dryRun bool) JSONLifecyclePlan {
	phases := make([]JSONLifecyclePhase, 0, len(p.phases))
	mandatory := []ProjectionAction{}
	for _, phase := range p.Phases() {
		actions := append([]ProjectionAction{}, phase.Actions...)
		for i := range actions {
			actions[i] = actionForReport(actions[i])
		}
		phases = append(phases, JSONLifecyclePhase{Kind: phase.Kind, Digest: phase.Digest, ApprovalRequired: phase.ApprovalRequired, Actions: actions})
		mandatory = append(mandatory, actions...)
	}
	contributors := p.Contributors()
	if contributors == nil {
		contributors = map[string][]string{}
	}
	for id := range contributors {
		contributors[id] = sortedUnique(contributors[id])
	}
	blockers := append([]PlanBlocker{}, p.Blockers()...)
	sort.Slice(blockers, func(i, j int) bool {
		if blockers[i].Kind != blockers[j].Kind {
			return blockers[i].Kind < blockers[j].Kind
		}
		if blockers[i].Subject != blockers[j].Subject {
			return blockers[i].Subject < blockers[j].Subject
		}
		return blockers[i].Detail < blockers[j].Detail
	})
	contract := p.LifecycleContract()
	diff := lifecycleContractDiff(p.beforeCompositionFacts, p.compositionFacts)
	removed := p.RemovedContributors()
	if removed == nil {
		removed = map[string]string{}
	}
	retained := p.RetainedProjections()
	if retained == nil {
		retained = []RetainedProjection{}
	}
	return JSONLifecyclePlan{SchemaVersion: LifecycleJSONSchemaVersion, Report: "pack-lifecycle-preview", PlanID: p.id,
		Operation: p.operation, Disposition: p.Disposition(), Digest: p.digest, Pack: p.pack.ID, PackVersion: p.pack.Version,
		Surface: p.surface, IntentRevision: p.intentRevision, Contract: contract, Aliases: contract.Aliases,
		Contributors: contributors, Blockers: blockers, Phases: phases, PendingHumanActions: sortedCopy(p.pendingHumanActions),
		ExpectedReadiness: p.readiness, ReadinessObserved: p.readinessObserved, Evidence: sortedCopy(p.observedEvidence), PendingEvidence: sortedCopy(p.pendingEvidence),
		Recovery: p.recovery, MandatoryActions: mandatory, ContractDiff: diff, Migrations: lifecycleMigrations(p),
		RetainedProjections: retained, RemovedContributors: removed, DryRun: dryRun}
}

func actionForReport(action ProjectionAction) ProjectionAction {
	// Host effects can carry complete merged documents so an adapter can apply
	// the sealed plan. Structured reports disclose the ordered redacted effect,
	// never raw owned or mixed-store content.
	action.Content = ""
	action.Args = reportredaction.EnvironmentArguments(action.Args)
	return action
}

func lifecycleContractDiff(before, after []Pack) JSONContractDiff {
	prior, next := map[string]string{}, map[string]string{}
	collect := func(target map[string]string, packs []Pack) {
		for _, pack := range packs {
			for _, resource := range pack.Resources {
				target[resource.Kind+":"+resource.ID] = digestJSON(resource)
			}
		}
	}
	collect(prior, before)
	collect(next, after)
	diff := JSONContractDiff{Added: []string{}, Changed: []string{}, Removed: []string{}, Retained: []string{}}
	for id, digest := range next {
		if old, ok := prior[id]; !ok {
			diff.Added = append(diff.Added, id)
		} else if old != digest {
			diff.Changed = append(diff.Changed, id)
		} else {
			diff.Retained = append(diff.Retained, id)
		}
	}
	for id := range prior {
		if _, ok := next[id]; !ok {
			diff.Removed = append(diff.Removed, id)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Changed)
	sort.Strings(diff.Removed)
	sort.Strings(diff.Retained)
	return diff
}

func lifecycleMigrations(p ReconciliationPlan) []string {
	result := []string{}
	if digestJSON(p.previousAliases) != digestJSON(p.aliases) {
		result = append(result, "surface-local aliases change")
	}
	for _, blocker := range p.blockers {
		if blocker.Kind == BlockerAlias {
			result = append(result, blocker.Detail)
		}
	}
	return sortedUnique(result)
}

type JSONLifecycleFailure struct {
	SchemaVersion     int                `json:"schema_version"`
	Report            string             `json:"report"`
	Stage             string             `json:"stage"`
	Error             string             `json:"error"`
	Plan              *JSONLifecyclePlan `json:"plan,omitempty"`
	ActionsExecuted   *int               `json:"actions_executed,omitempty"`
	ApprovalRequested *bool              `json:"approval_requested,omitempty"`
}

func JSONFailureFor(stage string, err error, plan *ReconciliationPlan, approvalRequested *bool, actionsExecuted *int) JSONLifecycleFailure {
	err = ReportSafeError(err, plan)
	result := JSONLifecycleFailure{SchemaVersion: LifecycleJSONSchemaVersion, Report: "pack-lifecycle-failure", Stage: stage, Error: err.Error()}
	result.ApprovalRequested, result.ActionsExecuted = approvalRequested, actionsExecuted
	if plan != nil {
		report := plan.JSONReport(false)
		if errors.Is(err, ErrStalePlan) && report.Contract.CompatibilityObserved {
			report.Contract.Compatibility = CompatibilityBlocked
		}
		result.Plan = &report
	}
	return result
}

// ReportSafeError removes sealed action payloads and environment values from
// lifecycle diagnostics without changing their errors.Is/As identity.
func ReportSafeError(err error, plan *ReconciliationPlan) error {
	if plan == nil {
		return err
	}
	argumentSets := make([][]string, 0)
	sealedPayloads := make([]string, 0)
	for _, phase := range plan.phases {
		for _, action := range phase.Actions {
			argumentSets = append(argumentSets, action.Args)
			sealedPayloads = append(sealedPayloads, action.Content)
		}
	}
	return reportredaction.Error(err, argumentSets, sealedPayloads)
}

type JSONApplyResult struct {
	SchemaVersion       int               `json:"schema_version"`
	Report              string            `json:"report"`
	Plan                JSONLifecyclePlan `json:"plan"`
	Verified            bool              `json:"verified"`
	Projections         int               `json:"projections"`
	Readiness           JSONReadiness     `json:"readiness"`
	PendingHumanActions []string          `json:"pending_human_actions"`
}

func JSONApplyResultFor(plan ReconciliationPlan, applied ApplyResult) JSONApplyResult {
	return JSONApplyResult{SchemaVersion: LifecycleJSONSchemaVersion, Report: "pack-lifecycle-apply", Plan: plan.JSONReport(false),
		Verified: applied.Verified, Projections: applied.Projections,
		Readiness: JSONReadiness{
			Configured: optionalBool(applied.ReadinessObserved.Configured, applied.Readiness.Configured),
			Authorized: optionalBool(applied.ReadinessObserved.Authorization, applied.Readiness.Authorized),
			Usable:     optionalBool(applied.ReadinessObserved.Usability, applied.Readiness.Usable),
		}, PendingHumanActions: sortedCopy(applied.PendingHumanActions)}
}
