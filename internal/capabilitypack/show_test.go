package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type showReportStore struct {
	states map[Surface]ActivationState
	loads  []Surface
	err    error
	saves  int
}

func (s *showReportStore) Load(_ context.Context, surface Surface) (ActivationState, error) {
	s.loads = append(s.loads, surface)
	if s.err != nil {
		return ActivationState{}, s.err
	}
	return s.states[surface], nil
}

func (s *showReportStore) Save(context.Context, Surface, int, ActivationState) error {
	s.saves++
	return errors.New("show must not save")
}

func TestFacadeShowReturnsDetachedCanonicalCatalogContractsAndIntents(t *testing.T) {
	pack := Pack{
		manifestVersion: 3,
		ID:              "addy",
		Version:         "1.1.0",
		Description:     "Addy workflows",
		Surfaces:        []Surface{SurfaceOpenCode, SurfaceClaude, SurfaceCodex},
		Resources: []Resource{{
			Kind: "skill", ID: "review", Source: "skills/review",
			Bindings: []Binding{
				{Surface: SurfaceOpenCode, Projection: "skill", Name: "review"},
				{Surface: SurfaceClaude, Projection: "skill", Name: "review"},
				{Surface: SurfaceCodex, Projection: "skill", Name: "review"},
			},
		}},
	}
	catalog := Catalog{
		packs: []Pack{pack},
		entries: []catalogEntry{{
			ID:                 "addy",
			Withdrawn:          true,
			HistoricalVersions: []string{"1.0.0", "1.1.0"},
			UpdateRoutes: []UpdateRoute{{
				FromVersion: "1.0.0", ToVersion: "1.1.0",
				ExistingSurfaces: []Surface{SurfaceCodex, SurfaceOpenCode},
			}},
		}},
	}
	store := &showReportStore{states: map[Surface]ActivationState{
		SurfaceClaude: {
			Intents: []ActivationIntent{{
				PackID: "addy", Surface: SurfaceClaude, Version: "1.1.0", Active: true, Revision: 4,
				Aliases: []SurfaceAlias{
					{Kind: "skill", ID: "review", Name: "z-review"},
					{Kind: "agent", ID: "reviewer", Name: "a-reviewer"},
				},
			}},
		},
		SurfaceOpenCode: {
			Intent: ActivationIntent{PackID: "addy", Surface: SurfaceOpenCode, Version: "1.0.0", Active: true, Revision: 2},
		},
	}}
	facade := NewFacade(catalog, WithActivation(store, nil))

	first, err := facade.Show(context.Background(), "addy")
	if err != nil {
		t.Fatal(err)
	}
	second, err := facade.Show(context.Background(), "addy")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("show report is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.Detail.Current || !first.Detail.Withdrawn ||
		!reflect.DeepEqual(first.Detail.HistoricalVersions, []string{"1.0.0", "1.1.0"}) ||
		len(first.Detail.UpdateRoutes) != 1 {
		t.Fatalf("catalog detail = %#v", first.Detail)
	}
	if first.SourceIdentity != (PackSourceIdentity{
		PackID: "addy", Version: "1.1.0", SchemaVersion: 3,
		Limitation: packSourceIdentityLimitation,
	}) {
		t.Fatalf("source identity = %#v", first.SourceIdentity)
	}
	if first.ResourceCounts.Skills != 1 {
		t.Fatalf("resource counts = %#v", first.ResourceCounts)
	}
	wantSurfaces := []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode}
	for i, surface := range wantSurfaces {
		if first.Surfaces[i].Surface != surface {
			t.Fatalf("surface[%d] = %q, want %q", i, first.Surfaces[i].Surface, surface)
		}
	}
	claude := first.Surfaces[0]
	if !claude.Intent.Present || !claude.Intent.Active || claude.Intent.Version != "1.1.0" || claude.Intent.Revision != 4 {
		t.Fatalf("Claude intent = %#v", claude.Intent)
	}
	wantAliases := []SurfaceAlias{
		{Kind: "agent", ID: "reviewer", Name: "a-reviewer"},
		{Kind: "skill", ID: "review", Name: "z-review"},
	}
	if !reflect.DeepEqual(claude.Intent.Aliases, wantAliases) || !reflect.DeepEqual(claude.Contract.Aliases, wantAliases) {
		t.Fatalf("Claude aliases intent=%#v contract=%#v", claude.Intent.Aliases, claude.Contract.Aliases)
	}
	if first.Surfaces[1].Intent.Present {
		t.Fatalf("Codex intent = %#v", first.Surfaces[1].Intent)
	}
	if got := first.Surfaces[2].Intent; !got.Present || !got.Active || got.Version != "1.0.0" || got.Revision != 2 {
		t.Fatalf("OpenCode legacy intent = %#v", got)
	}
	if store.saves != 0 {
		t.Fatalf("show saved activation state %d times", store.saves)
	}
	if !reflect.DeepEqual(store.loads, []Surface{
		SurfaceClaude, SurfaceCodex, SurfaceOpenCode,
		SurfaceClaude, SurfaceCodex, SurfaceOpenCode,
	}) {
		t.Fatalf("store loads = %#v", store.loads)
	}

	first.Detail.Pack.Resources[0].Bindings[0].Name = "mutated"
	first.Detail.HistoricalVersions[0] = "mutated"
	first.Detail.UpdateRoutes[0].ExistingSurfaces[0] = SurfaceClaude
	first.Surfaces[0].Contract.Bindings[0].Name = "mutated"
	first.Surfaces[0].Contract.Aliases[0].Name = "mutated"
	first.Surfaces[0].Intent.Aliases[0].Name = "mutated"
	third, err := facade.Show(context.Background(), "addy")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second, third) {
		t.Fatalf("caller mutation leaked into show report:\nwant=%#v\ngot=%#v", second, third)
	}
}

func TestFacadeShowRequiresDurableIntentStoreWithoutHostInspection(t *testing.T) {
	pack := Pack{ID: "addy", Version: "1.1.0", Surfaces: []Surface{SurfaceClaude}}
	catalog := Catalog{packs: []Pack{pack}, entries: []catalogEntry{{ID: "addy"}}}

	if _, err := NewFacade(catalog).Show(context.Background(), "addy"); err == nil || !strings.Contains(err.Error(), "intent observation") {
		t.Fatalf("missing store error = %v", err)
	}
	store := &showReportStore{states: map[Surface]ActivationState{}, err: errors.New("state unavailable")}
	if _, err := NewFacade(catalog, WithActivation(store, nil)).Show(context.Background(), "addy"); err == nil || !strings.Contains(err.Error(), "load claude surface intent") {
		t.Fatalf("store load error = %v", err)
	}
	if store.saves != 0 {
		t.Fatalf("show attempted %d saves", store.saves)
	}
}
