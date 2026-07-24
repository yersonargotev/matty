package capabilitypack

import (
	"strings"
	"testing"
)

func TestCatalogDetailsListCurrentAndShowKnownWithdrawn(t *testing.T) {
	bundle := writeCatalogFixture(t)
	workflowID := strings.Join([]string{"ma", "tty"}, "")
	entries := []catalogEntry{
		{
			ID:                 "engram",
			Description:        "memory",
			Surfaces:           []Surface{SurfaceCodex},
			HistoricalVersions: []string{"1.0.0", "2.0.0"},
			UpdateRoutes: []UpdateRoute{{
				FromVersion:      "1.0.0",
				ToVersion:        "2.0.0",
				ExistingSurfaces: []Surface{SurfaceCodex},
			}},
		},
		{
			ID:                 workflowID,
			Description:        "workflow",
			Surfaces:           []Surface{SurfaceCodex},
			Withdrawn:          true,
			HistoricalVersions: []string{"1.0.0"},
		},
	}
	catalog, err := discoverCatalog(bundle, entries)
	if err != nil {
		t.Fatal(err)
	}

	if got := catalog.List(); len(got) != 2 {
		t.Fatalf("internal catalog list has %d packs, want both known packs", len(got))
	}
	details, err := catalog.ListDetails()
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0].Pack.ID != "engram" || !details[0].Current || details[0].Withdrawn {
		t.Fatalf("advertised catalog details = %#v", details)
	}
	current, err := catalog.ListCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].ID != "engram" {
		t.Fatalf("current packs = %#v", current)
	}
	withdrawn, err := catalog.ShowDetail(workflowID)
	if err != nil {
		t.Fatal(err)
	}
	if withdrawn.Pack.ID != workflowID || withdrawn.Current || !withdrawn.Withdrawn {
		t.Fatalf("withdrawn detail = %#v", withdrawn)
	}
}

func TestCatalogDetailResultsDoNotExposeCatalogMetadata(t *testing.T) {
	bundle := writeCatalogFixture(t)
	entries := []catalogEntry{{
		ID:                 "engram",
		Description:        "memory",
		Surfaces:           []Surface{SurfaceCodex},
		HistoricalVersions: []string{"1.0.0", "2.0.0"},
		UpdateRoutes: []UpdateRoute{{
			FromVersion:      "1.0.0",
			ToVersion:        "2.0.0",
			ExistingSurfaces: []Surface{SurfaceCodex},
		}},
	}}
	catalog, err := discoverCatalog(bundle, entries)
	if err != nil {
		t.Fatal(err)
	}

	first, err := catalog.ShowDetail("engram")
	if err != nil {
		t.Fatal(err)
	}
	first.HistoricalVersions[0] = "9.9.9"
	first.UpdateRoutes[0].FromVersion = "9.9.9"
	first.UpdateRoutes[0].ExistingSurfaces[0] = SurfaceClaude
	first.Pack.Surfaces[0] = SurfaceClaude

	second, err := catalog.ShowDetail("engram")
	if err != nil {
		t.Fatal(err)
	}
	if second.HistoricalVersions[0] != "1.0.0" ||
		second.UpdateRoutes[0].FromVersion != "1.0.0" ||
		second.UpdateRoutes[0].ExistingSurfaces[0] != SurfaceCodex ||
		second.Pack.Surfaces[0] != SurfaceCodex {
		t.Fatalf("catalog metadata was mutated through a result: %#v", second)
	}
}

func TestCatalogMetadataRejectsNonCanonicalHistoryAndRoutes(t *testing.T) {
	tests := []struct {
		name    string
		entry   catalogEntry
		message string
	}{
		{
			name:    "malformed history",
			entry:   catalogEntry{ID: "engram", HistoricalVersions: []string{"not-a-version"}},
			message: "not valid SemVer",
		},
		{
			name:    "duplicate history",
			entry:   catalogEntry{ID: "engram", HistoricalVersions: []string{"1.0.0", "1.0.0"}},
			message: "duplicated",
		},
		{
			name:    "unordered history",
			entry:   catalogEntry{ID: "engram", HistoricalVersions: []string{"2.0.0", "1.0.0"}},
			message: "canonical order",
		},
		{
			name: "unknown route version",
			entry: catalogEntry{ID: "engram", HistoricalVersions: []string{"1.0.0"}, UpdateRoutes: []UpdateRoute{{
				FromVersion: "1.0.0", ToVersion: "2.0.0", ExistingSurfaces: []Surface{SurfaceCodex},
			}}},
			message: "unknown historical version",
		},
		{
			name: "duplicate route",
			entry: catalogEntry{
				ID:                 "engram",
				HistoricalVersions: []string{"1.0.0", "2.0.0"},
				UpdateRoutes: []UpdateRoute{
					{FromVersion: "1.0.0", ToVersion: "2.0.0", ExistingSurfaces: []Surface{SurfaceCodex}},
					{FromVersion: "1.0.0", ToVersion: "2.0.0", ExistingSurfaces: []Surface{SurfaceCodex}},
				},
			},
			message: "duplicated",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCatalogMetadata(test.entry)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("validateCatalogMetadata() error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestCatalogMetadataUsesSemanticVersionPrecedenceForCanonicalOrder(t *testing.T) {
	entry := catalogEntry{
		ID: "engram",
		HistoricalVersions: []string{
			"1.9.0",
			"1.10.0-alpha.2",
			"1.10.0-alpha.10",
			"1.10.0",
		},
		UpdateRoutes: []UpdateRoute{
			{FromVersion: "1.9.0", ToVersion: "1.10.0-alpha.2", ExistingSurfaces: []Surface{SurfaceCodex}},
			{FromVersion: "1.10.0-alpha.2", ToVersion: "1.10.0-alpha.10", ExistingSurfaces: []Surface{SurfaceCodex}},
			{FromVersion: "1.10.0-alpha.10", ToVersion: "1.10.0", ExistingSurfaces: []Surface{SurfaceCodex}},
		},
	}
	if err := validateCatalogMetadata(entry); err != nil {
		t.Fatalf("semantic canonical order rejected: %v", err)
	}

	entry.HistoricalVersions = []string{"1.10.0", "1.10.0-alpha.10"}
	entry.UpdateRoutes = nil
	if err := validateCatalogMetadata(entry); err == nil || !strings.Contains(err.Error(), "canonical order") {
		t.Fatalf("release-before-prerelease order error = %v", err)
	}
}

func TestCatalogMetadataRequiresCurrentVersionInImmutableHistory(t *testing.T) {
	bundle := writeCatalogFixture(t)
	_, err := discoverCatalog(bundle, []catalogEntry{{
		ID:                 "engram",
		Surfaces:           []Surface{SurfaceCodex},
		HistoricalVersions: []string{"9.0.0"},
	}})
	if err == nil || !strings.Contains(err.Error(), "current version") {
		t.Fatalf("discoverCatalog() error = %v, want missing current history", err)
	}
}
