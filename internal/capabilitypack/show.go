package capabilitypack

import (
	"context"
	"fmt"
	"sort"
)

const packSourceIdentityLimitation = "Packy records the trusted pack ID, version, and manifest schema, but no upstream source provenance."

// PackSourceIdentity is the stable source identity Packy can derive from its
// trusted domain facts. Upstream repository provenance is not part of Pack.
type PackSourceIdentity struct {
	PackID        string
	Version       string
	SchemaVersion int
	Limitation    string
}

// ShowIntent reports the durable surface-local intent without observing a
// host. Present distinguishes no record from an inactive record.
type ShowIntent struct {
	Present  bool
	Active   bool
	Version  string
	Revision int
	Aliases  []SurfaceAlias
}

// ShowSurfaceReport contains the deterministic portable contract and durable
// intent facts for one supported surface.
type ShowSurfaceReport struct {
	Surface  Surface
	Contract LifecycleContract
	Intent   ShowIntent
}

// ShowReport is the detached domain result used by pack show renderers.
type ShowReport struct {
	Detail         CatalogDetail
	SourceIdentity PackSourceIdentity
	ResourceCounts ResourceCounts
	Surfaces       []ShowSurfaceReport
}

// Show returns catalog metadata, portable per-surface contracts, and durable
// surface-local intent facts. It performs no host inspection or mutation.
func (f Facade) Show(ctx context.Context, id string) (ShowReport, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (ShowReport, error) {
		return locked.show(ctx, id)
	})
}

func (f Facade) show(ctx context.Context, id string) (ShowReport, error) {
	detail, err := f.catalog.ShowDetail(id)
	if err != nil {
		return ShowReport{}, err
	}
	if f.activation == nil || f.activation.store == nil {
		return ShowReport{}, fmt.Errorf("surface intent observation is not configured")
	}

	pack := detail.Pack
	report := ShowReport{
		Detail: detail,
		SourceIdentity: PackSourceIdentity{
			PackID:        pack.ID,
			Version:       pack.Version,
			SchemaVersion: pack.manifestVersion,
			Limitation:    packSourceIdentityLimitation,
		},
		ResourceCounts: pack.ResourceCounts(),
		Surfaces:       make([]ShowSurfaceReport, 0, len(pack.Surfaces)),
	}
	surfaces := append([]Surface(nil), pack.Surfaces...)
	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i] < surfaces[j] })
	for _, surface := range surfaces {
		state, err := f.activation.store.Load(ctx, surface)
		if err != nil {
			return ShowReport{}, fmt.Errorf("load %s surface intent: %w", surface, err)
		}
		intent, present := intentForPack(state, pack.ID, surface)
		aliases := []SurfaceAlias{}
		if present {
			aliases = canonicalShowAliases(intent.Aliases)
		}
		report.Surfaces = append(report.Surfaces, ShowSurfaceReport{
			Surface:  surface,
			Contract: LifecycleContractFor(pack, surface, aliases),
			Intent: ShowIntent{
				Present:  present,
				Active:   present && intent.Active,
				Version:  intent.Version,
				Revision: intent.Revision,
				Aliases:  cloneAliases(aliases),
			},
		})
	}
	return report, nil
}

func canonicalShowAliases(aliases []SurfaceAlias) []SurfaceAlias {
	result := cloneAliases(aliases)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result
}
