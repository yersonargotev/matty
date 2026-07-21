package cli

import (
	"encoding/json"
	"io"
	"sort"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type packShowSurfaceJSON struct {
	Surface  capabilitypack.Surface           `json:"surface"`
	Contract capabilitypack.LifecycleContract `json:"contract"`
}

type packShowRequirementsJSON struct {
	Capabilities []string `json:"capabilities"`
	Tools        []string `json:"tools"`
}

type packShowJSON struct {
	SchemaVersion    int                           `json:"schema_version"`
	Report           string                        `json:"report"`
	ID               string                        `json:"id"`
	Version          string                        `json:"version"`
	Description      string                        `json:"description"`
	Surfaces         []capabilitypack.Surface      `json:"surfaces"`
	Provides         []string                      `json:"provides"`
	Requires         packShowRequirementsJSON      `json:"requires"`
	Conflicts        []string                      `json:"conflicts"`
	ResourceCounts   capabilitypack.ResourceCounts `json:"resource_counts"`
	SurfaceContracts []packShowSurfaceJSON         `json:"surface_contracts"`
}

func renderPackShowJSON(w io.Writer, pack capabilitypack.Pack) error {
	surfaces := append([]capabilitypack.Surface{}, pack.Surfaces...)
	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i] < surfaces[j] })
	contracts := make([]packShowSurfaceJSON, 0, len(surfaces))
	for _, surface := range surfaces {
		contract := capabilitypack.LifecycleContractFor(pack, surface, nil)
		if contract.CompatibilityObserved {
			contracts = append(contracts, packShowSurfaceJSON{Surface: surface, Contract: contract})
		}
	}
	return json.NewEncoder(w).Encode(packShowJSON{
		SchemaVersion: capabilitypack.LifecycleJSONSchemaVersion, Report: "pack-show",
		ID: pack.ID, Version: pack.Version, Description: pack.Description, Surfaces: surfaces,
		Provides:  sortedStrings(pack.Provides),
		Requires:  packShowRequirementsJSON{Capabilities: sortedStrings(pack.Requires.Capabilities), Tools: sortedStrings(pack.Requires.Tools)},
		Conflicts: sortedStrings(pack.Conflicts), ResourceCounts: pack.ResourceCounts(), SurfaceContracts: contracts,
	})
}
