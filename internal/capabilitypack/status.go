package capabilitypack

import (
	"context"
	"fmt"

	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/prompt"
)

// SurfaceInspector is the host-observation boundary used by capability-pack
// policy. Implementations inspect one surface but do not decide lifecycle or
// readiness state.
type SurfaceInspector interface {
	Inspect(context.Context, Pack) (SurfaceObservation, error)
}

// SurfaceObservation records that a host adapter freshly inspected the
// requested pack/surface pair. Projection interpretation remains facade policy.
type SurfaceObservation struct {
	Inspected bool
}

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

type ProjectionSummary struct {
	Verified  int
	Drifted   int
	Ambiguous int
}

type StatusEntry struct {
	Pack                Pack
	Surface             Surface
	Intent              IntentStatus
	LatestAttempt       *AttemptStatus
	Readiness           ReadinessStatus
	Projections         ProjectionSummary
	PendingHumanActions []string
	Observation         SurfaceObservation
}

type StatusReport struct {
	Entries []StatusEntry
}

// Facade is the single capability-pack use-case boundary consumed by the CLI.
type Facade struct {
	catalog    Catalog
	inspectors map[Surface]SurfaceInspector
	activation *activationDependencies
}

func NewFacade(catalog Catalog, inspectors map[Surface]SurfaceInspector, options ...FacadeOption) Facade {
	facade := Facade{catalog: catalog, inspectors: inspectors}
	for _, option := range options {
		option(&facade)
	}
	return facade
}

// Status freshly inspects every requested host pair. Until activation intent
// and ownership exist, all catalog packs truthfully start inactive, with no
// attempt, readiness success, owned projections, or pending human action.
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
			if request.Surface != "" && surface != request.Surface {
				continue
			}
			entry, err := f.inspectStatusEntry(ctx, pack, surface)
			if err != nil {
				return StatusReport{}, err
			}
			report.Entries = append(report.Entries, entry)
		}
	}
	if request.Surface != "" && len(report.Entries) == 0 {
		return StatusReport{}, fmt.Errorf("pack %q does not support CLI surface %q", request.PackID, request.Surface)
	}
	return report, nil
}

func (f Facade) inspectStatusEntry(ctx context.Context, pack Pack, surface Surface) (StatusEntry, error) {
	if f.activation != nil && f.activation.store != nil {
		adapter, ok := f.activation.adapters[surface]
		if !ok {
			return StatusEntry{}, fmt.Errorf("no activation adapter configured for CLI surface %q", surface)
		}
		observation, err := adapter.InspectActivation(ctx, pack)
		if err != nil {
			return StatusEntry{}, fmt.Errorf("inspect pack %q on %s: %w", pack.ID, surface, err)
		}
		state, err := f.activation.store.Load(ctx, surface)
		if err != nil {
			return StatusEntry{}, err
		}
		entry := StatusEntry{Pack: pack, Surface: surface, Observation: SurfaceObservation{Inspected: true}}
		if !state.Intent.Active || state.Intent.PackID != pack.ID {
			return entry, nil
		}
		entry.Intent = IntentStatus{Active: true, Revision: state.Intent.Revision}
		entry.Readiness = observation.Readiness
		entry.Readiness.Configured = ownershipMatches(state.Ownership, observation.Projections, pack.ID)
		if !entry.Readiness.Configured {
			entry.Readiness.Authorized = false
			entry.Readiness.Usable = false
		} else if !entry.Readiness.Authorized {
			entry.Readiness.Usable = false
		}
		entry.PendingHumanActions = append([]string(nil), observation.PendingHumanActions...)
		for _, projection := range observation.Projections {
			if ownedAtFingerprint(state.Ownership, projection.ID, projection.ObservedFingerprint, pack.ID) && projection.ObservedFingerprint == projection.DesiredFingerprint {
				entry.Projections.Verified++
			} else if projection.Exists {
				entry.Projections.Ambiguous++
			} else {
				entry.Projections.Drifted++
			}
		}
		if state.Journal != nil {
			entry.LatestAttempt = &AttemptStatus{Outcome: "recovery-required", PlanID: state.Journal.PlanID}
		}
		return entry, nil
	}
	inspector, ok := f.inspectors[surface]
	if !ok {
		return StatusEntry{}, fmt.Errorf("no inspector configured for CLI surface %q", surface)
	}
	observation, err := inspector.Inspect(ctx, pack)
	if err != nil {
		return StatusEntry{}, fmt.Errorf("inspect pack %q on %s: %w", pack.ID, surface, err)
	}
	if !observation.Inspected {
		return StatusEntry{}, fmt.Errorf("inspect pack %q on %s: adapter returned no fresh observation", pack.ID, surface)
	}
	return StatusEntry{Pack: pack, Surface: surface, Observation: observation}, nil
}

type codexInspector struct{ promptPath string }

func NewCodexInspector(promptPath string) SurfaceInspector {
	return codexInspector{promptPath: promptPath}
}

func (i codexInspector) Inspect(_ context.Context, _ Pack) (SurfaceObservation, error) {
	_, err := prompt.InspectCodex(i.promptPath)
	return SurfaceObservation{Inspected: err == nil}, err
}

type openCodeInspector struct {
	configPath string
	promptPath string
}

func NewOpenCodeInspector(configPath, promptPath string) SurfaceInspector {
	return openCodeInspector{configPath: configPath, promptPath: promptPath}
}

func (i openCodeInspector) Inspect(_ context.Context, _ Pack) (SurfaceObservation, error) {
	_, err := opencode.Inspect(i.configPath, i.promptPath)
	return SurfaceObservation{Inspected: err == nil}, err
}
