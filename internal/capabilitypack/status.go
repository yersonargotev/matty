package capabilitypack

import (
	"context"
	"fmt"
	"sort"
)

type StatusRequest struct {
	PackID  string
	Surface Surface
}

type IntentStatus struct {
	Active   bool
	Revision int
}

type AttemptStatus struct {
	Outcome string
	PlanID  string
}

type ReadinessStatus struct {
	Configured bool
	Authorized bool
	Usable     bool
}

type ProjectionHealth string

const (
	ProjectionVerified  ProjectionHealth = "verified"
	ProjectionMissing   ProjectionHealth = "missing"
	ProjectionDrifted   ProjectionHealth = "drifted"
	ProjectionAmbiguous ProjectionHealth = "ambiguous"
	ProjectionUnmanaged ProjectionHealth = "unmanaged"
)

type ProjectionStatus struct {
	ID, Target, ObservedFingerprint, DesiredFingerprint string
	Health                                              ProjectionHealth
	Contributors                                        []string
}

type ProjectionSummary struct {
	Verified, Missing, Drifted, Ambiguous, Unmanaged int
}

// ReadinessObservation is fresh host-owned evidence. Observed distinguishes a
// negative observation from an adapter that cannot inspect that dimension.
type ReadinessObservation struct {
	AuthorizationObserved bool
	Authorized            bool
	UsabilityObserved     bool
	Usable                bool
	PendingHumanActions   []string
	Evidence              []string
}

type StatusEntry struct {
	Pack                Pack
	Surface             Surface
	Intent              IntentStatus
	IntentPresent       bool
	LatestAttempt       *AttemptStatus
	Readiness           ReadinessStatus
	ReadinessObserved   ReadinessObservationStatus
	Projections         ProjectionSummary
	ProjectionDetails   []ProjectionStatus
	Blockers            []string
	PendingHumanActions []string
	Evidence            []string
}

type ReadinessObservationStatus struct {
	Configured    bool
	Authorization bool
	Usability     bool
}

type StatusReport struct{ Entries []StatusEntry }

// Facade is the single capability-pack use-case boundary consumed by the CLI.
type Facade struct {
	catalog    Catalog
	activation *activationDependencies
}

func NewFacade(catalog Catalog, options ...FacadeOption) Facade {
	facade := Facade{catalog: catalog}
	for _, option := range options {
		option(&facade)
	}
	return facade
}

func (f Facade) Status(ctx context.Context, request StatusRequest) (StatusReport, error) {
	packs := f.catalog.List()
	if request.PackID != "" {
		if request.Surface == "" {
			return StatusReport{}, fmt.Errorf("--surface is required when a pack is specified")
		}
		pack, err := f.catalog.Show(request.PackID)
		if err != nil {
			return StatusReport{}, err
		}
		packs = []Pack{pack}
	} else if request.Surface != "" {
		return StatusReport{}, fmt.Errorf("a pack is required when --surface is specified")
	}
	var report StatusReport
	for _, pack := range packs {
		for _, surface := range pack.Surfaces {
			if request.Surface != "" && request.Surface != surface {
				continue
			}
			entry, err := f.statusEntry(ctx, pack, surface)
			if err != nil {
				return StatusReport{}, fmt.Errorf("inspect pack %q on %s: %w", pack.ID, surface, err)
			}
			report.Entries = append(report.Entries, entry)
		}
	}
	if request.Surface != "" && len(report.Entries) == 0 {
		return StatusReport{}, fmt.Errorf("pack %q does not support CLI surface %q", request.PackID, request.Surface)
	}
	return report, nil
}

func (f Facade) statusEntry(ctx context.Context, pack Pack, surface Surface) (StatusEntry, error) {
	if f.activation == nil || f.activation.store == nil {
		return StatusEntry{}, fmt.Errorf("surface inspection is not configured")
	}
	adapter := f.activation.adapters[surface]
	if adapter == nil {
		return StatusEntry{}, fmt.Errorf("no activation adapter configured for CLI surface %q", surface)
	}
	state, err := f.activation.store.Load(ctx, surface)
	if err != nil {
		return StatusEntry{}, err
	}
	entry := StatusEntry{Pack: pack, Surface: surface}
	evidencePack := pack
	if intent, ok := intentForPack(state, pack.ID, surface); ok {
		entry.Intent = IntentStatus{Active: intent.Active, Revision: intent.Revision}
		entry.IntentPresent = true
		if intent.Active {
			evidencePack, err = f.catalog.showVersion(intent.PackID, intent.Version)
			if err != nil {
				return StatusEntry{}, err
			}
		}
	}
	entry.LatestAttempt = latestAttemptStatus(state, pack.ID, surface)
	surfaceComposition, err := f.compose(evidencePack, state, surface, true)
	if err != nil {
		return StatusEntry{}, err
	}
	relevantPack, err := f.statusEvidencePack(evidencePack, surface)
	if err != nil {
		return StatusEntry{}, err
	}
	resolutions, err := f.resolveExecutables(ctx, relevantPack)
	if err != nil {
		entry.Blockers = append(entry.Blockers, err.Error())
		resolutions = nil
	}
	for _, resolution := range resolutions {
		if !resolution.Available {
			entry.Blockers = append(entry.Blockers, fmt.Sprintf("required executable %s is missing", resolution.Tool))
			if entry.Intent.Active {
				entry.PendingHumanActions = append(entry.PendingHumanActions, fmt.Sprintf("install %s and rerun status; Matty will not install it during Status", resolution.Tool))
			}
		}
	}
	observation, inspectErr := inspectSurface(ctx, adapter, SurfaceTransition{Desired: relevantPack, ResolvedExecutables: resolutions})
	if inspectErr != nil {
		return StatusEntry{}, inspectErr
	}
	entry.ProjectionDetails, entry.Projections = deriveProjectionStatus(pack.ID, observation.Projections, state.Ownership, surfaceComposition)
	entry.Readiness.Configured = entry.Projections.Verified == len(observation.Projections) && len(observation.Projections) > 0
	entry.ReadinessObserved.Configured = true
	for _, detail := range entry.ProjectionDetails {
		entry.Evidence = append(entry.Evidence, fmt.Sprintf("%s: %s observed=%s desired=%s target=%s", detail.ID, detail.Health, detail.ObservedFingerprint, detail.DesiredFingerprint, detail.Target))
		if detail.Health != ProjectionVerified {
			entry.Blockers = append(entry.Blockers, fmt.Sprintf("%s is %s", detail.ID, detail.Health))
		}
	}
	fresh := observation.Readiness
	if entry.Readiness.Configured {
		entry.PendingHumanActions = append(entry.PendingHumanActions, fresh.PendingHumanActions...)
	}
	entry.Evidence = append(entry.Evidence, fresh.Evidence...)
	entry.ReadinessObserved.Authorization = fresh.AuthorizationObserved
	entry.ReadinessObserved.Usability = fresh.UsabilityObserved
	entry.Readiness.Authorized = entry.Readiness.Configured && fresh.AuthorizationObserved && fresh.Authorized
	entry.Readiness.Usable = entry.Readiness.Authorized && fresh.UsabilityObserved && fresh.Usable
	if entry.Readiness.Configured && len(fresh.PendingHumanActions) == 0 {
		entry.PendingHumanActions = append(entry.PendingHumanActions, observation.PendingHumanActions...)
	}
	if entry.Readiness.Configured && !entry.Readiness.Authorized {
		entry.Blockers = append(entry.Blockers, "authorization/trust is not freshly demonstrated")
	}
	if entry.Readiness.Authorized && !entry.Readiness.Usable {
		entry.Blockers = append(entry.Blockers, "runtime usability is not freshly demonstrated")
	}
	sort.Strings(entry.Blockers)
	sort.Strings(entry.PendingHumanActions)
	sort.Strings(entry.Evidence)
	return entry, nil
}

// statusEvidencePack excludes unrelated active packs while retaining the
// requested pack's dependency closure.
func (f Facade) statusEvidencePack(pack Pack, surface Surface) (Pack, error) {
	composition, err := f.compose(pack, ActivationState{}, surface, false)
	if err != nil {
		return Pack{}, err
	}
	return composition.combinedPack(), nil
}

func deriveProjectionStatus(packID string, observed []ObservedProjection, ownership []ProjectionOwnership, c composition) ([]ProjectionStatus, ProjectionSummary) {
	result := make([]ProjectionStatus, 0, len(observed))
	var summary ProjectionSummary
	for _, p := range observed {
		status := ProjectionStatus{ID: p.ID, Target: p.Action.Target, ObservedFingerprint: p.ObservedFingerprint, DesiredFingerprint: p.DesiredFingerprint, Contributors: c.contributorSet(p.ID)}
		owner, owned := ownershipByID(ownership, p.ID)
		switch {
		case !p.Exists:
			status.Health = ProjectionMissing
			summary.Missing++
		case p.ExternallyManaged && p.ObservedFingerprint == p.DesiredFingerprint:
			status.Health = ProjectionVerified
			summary.Verified++
		case p.ExternallyManaged:
			status.Health = ProjectionDrifted
			summary.Drifted++
		case p.ObservedFingerprint != p.DesiredFingerprint && owned:
			status.Health = ProjectionDrifted
			summary.Drifted++
		case p.ObservedFingerprint != p.DesiredFingerprint:
			status.Health = ProjectionUnmanaged
			summary.Unmanaged++
		case !owned:
			status.Health = ProjectionUnmanaged
			summary.Unmanaged++
		case owner.Fingerprint != p.DesiredFingerprint || digestJSON(owner.Contributors) != digestJSON(status.Contributors):
			status.Health = ProjectionAmbiguous
			summary.Ambiguous++
		default:
			status.Health = ProjectionVerified
			summary.Verified++
		}
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, summary
}

func latestAttemptStatus(state ActivationState, packID string, surface Surface) *AttemptStatus {
	var candidate *ApplyingJournal
	for i := range state.History {
		if state.History[i].PackID == packID && state.History[i].Surface == surface {
			candidate = &state.History[i]
		}
	}
	for i := range state.LastAttempts {
		if state.LastAttempts[i].PackID == packID && state.LastAttempts[i].Surface == surface {
			candidate = &state.LastAttempts[i]
		}
	}
	if state.Journal != nil && state.Journal.PackID == packID && state.Journal.Surface == surface {
		candidate = state.Journal
	}
	if candidate == nil {
		return nil
	}
	return &AttemptStatus{Outcome: string(candidate.Outcome), PlanID: candidate.PlanID}
}
