package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type packShowSourceIdentityJSON struct {
	PackID        string `json:"pack_id"`
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	Limitation    string `json:"limitation"`
}

type packShowRouteJSON struct {
	FromVersion      string                   `json:"from_version"`
	ToVersion        string                   `json:"to_version"`
	ExistingSurfaces []capabilitypack.Surface `json:"existing_surfaces"`
}

type packShowIntentJSON struct {
	State    string                        `json:"state"`
	Active   *bool                         `json:"active"`
	Version  string                        `json:"version,omitempty"`
	Revision *int                          `json:"revision"`
	Aliases  []capabilitypack.SurfaceAlias `json:"aliases"`
}

type packShowLifecycleAvailabilityJSON struct {
	FreshActivationAvailable bool `json:"fresh_activation_available"`
	CatalogUpdateAvailable   bool `json:"catalog_update_available"`
	LifecycleVerbsAvailable  bool `json:"lifecycle_verbs_available"`
	AutomaticDowngrade       bool `json:"automatic_downgrade"`
}

type packShowSurfaceJSON struct {
	Surface  capabilitypack.Surface           `json:"surface"`
	Contract capabilitypack.LifecycleContract `json:"contract"`
	Intent   packShowIntentJSON               `json:"intent"`
}

type packShowRequirementsJSON struct {
	Capabilities []string `json:"capabilities"`
	Tools        []string `json:"tools"`
}

type packShowJSON struct {
	SchemaVersion         int                               `json:"schema_version"`
	Report                string                            `json:"report"`
	CatalogState          string                            `json:"catalog_state"`
	ID                    string                            `json:"id"`
	Version               string                            `json:"version"`
	Description           string                            `json:"description"`
	SourceIdentity        packShowSourceIdentityJSON        `json:"source_identity"`
	HistoricalVersions    []string                          `json:"historical_versions"`
	UpdateRoutes          []packShowRouteJSON               `json:"update_routes"`
	Surfaces              []capabilitypack.Surface          `json:"surfaces"`
	Provides              []string                          `json:"provides"`
	Requires              packShowRequirementsJSON          `json:"requires"`
	Conflicts             []string                          `json:"conflicts"`
	ResourceCounts        capabilitypack.ResourceCounts     `json:"resource_counts"`
	LifecycleAvailability packShowLifecycleAvailabilityJSON `json:"lifecycle_availability"`
	SurfaceContracts      []packShowSurfaceJSON             `json:"surface_contracts"`
}

func packShowDocument(report capabilitypack.ShowReport) packShowJSON {
	pack := report.Detail.Pack
	state := "current"
	if report.Detail.Withdrawn {
		state = "withdrawn"
	}
	routes := make([]packShowRouteJSON, 0, len(report.Detail.UpdateRoutes))
	for _, route := range report.Detail.UpdateRoutes {
		routes = append(routes, packShowRouteJSON{
			FromVersion: route.FromVersion, ToVersion: route.ToVersion,
			ExistingSurfaces: append([]capabilitypack.Surface{}, route.ExistingSurfaces...),
		})
	}
	surfaces := make([]capabilitypack.Surface, 0, len(report.Surfaces))
	contracts := make([]packShowSurfaceJSON, 0, len(report.Surfaces))
	for _, surface := range report.Surfaces {
		surfaces = append(surfaces, surface.Surface)
		contracts = append(contracts, packShowSurfaceJSON{
			Surface: surface.Surface, Contract: surface.Contract, Intent: packShowIntentDocument(surface.Intent),
		})
	}
	return packShowJSON{
		SchemaVersion: capabilitypack.LifecycleJSONSchemaVersion, Report: "pack-show", CatalogState: state,
		ID: pack.ID, Version: pack.Version, Description: pack.Description,
		SourceIdentity: packShowSourceIdentityJSON{
			PackID: report.SourceIdentity.PackID, Version: report.SourceIdentity.Version,
			SchemaVersion: report.SourceIdentity.SchemaVersion, Limitation: report.SourceIdentity.Limitation,
		},
		HistoricalVersions: append([]string{}, report.Detail.HistoricalVersions...), UpdateRoutes: routes,
		Surfaces: surfaces, Provides: sortedStrings(pack.Provides),
		Requires: packShowRequirementsJSON{
			Capabilities: sortedStrings(pack.Requires.Capabilities), Tools: sortedStrings(pack.Requires.Tools),
		},
		Conflicts: sortedStrings(pack.Conflicts), ResourceCounts: report.ResourceCounts,
		LifecycleAvailability: packShowLifecycleAvailabilityJSON{
			FreshActivationAvailable: report.LifecycleAvailability.FreshActivationAvailable,
			CatalogUpdateAvailable:   report.LifecycleAvailability.CatalogUpdateAvailable,
			LifecycleVerbsAvailable:  report.LifecycleAvailability.LifecycleVerbsAvailable,
			AutomaticDowngrade:       report.LifecycleAvailability.AutomaticDowngrade,
		},
		SurfaceContracts: contracts,
	}
}

func packShowIntentDocument(intent capabilitypack.ShowIntent) packShowIntentJSON {
	result := packShowIntentJSON{State: "absent", Aliases: append([]capabilitypack.SurfaceAlias{}, intent.Aliases...)}
	if intent.Present {
		active, revision := intent.Active, intent.Revision
		result.State, result.Active, result.Version, result.Revision = "known", &active, intent.Version, &revision
	}
	return result
}

func renderPackShowJSON(w io.Writer, report capabilitypack.ShowReport) error {
	return json.NewEncoder(w).Encode(packShowDocument(report))
}

func renderPackShowHuman(w io.Writer, report capabilitypack.ShowReport) error {
	document := packShowDocument(report)
	if _, err := fmt.Fprintf(w,
		"%s %s\nCatalog state: %s\nDescription: %s\nSource identity: pack=%s version=%s schema=%d\nSource limitation: %s\nHistorical versions: %s\nSupported CLI surfaces: %s\nProvides capabilities: %s\nRequires capabilities: %s\nRequires global tools: %s\nConflicts with capabilities: %s\nResources: %d skill, %d instruction, %d mcp_server, %d lifecycle, %d agent, %d command, %d asset, %d notice\nLifecycle availability: fresh_activation=%s catalog_update=%s lifecycle_verbs=%s automatic_downgrade=%s\n",
		document.ID, document.Version, document.CatalogState, document.Description,
		document.SourceIdentity.PackID, document.SourceIdentity.Version, document.SourceIdentity.SchemaVersion,
		document.SourceIdentity.Limitation, joinOrNone(document.HistoricalVersions), joinSurfaces(document.Surfaces),
		joinOrNone(document.Provides), joinOrNone(document.Requires.Capabilities), joinOrNone(document.Requires.Tools),
		joinOrNone(document.Conflicts), document.ResourceCounts.Skills, document.ResourceCounts.Instructions,
		document.ResourceCounts.MCPServers, document.ResourceCounts.Lifecycles, document.ResourceCounts.Agents,
		document.ResourceCounts.Commands, document.ResourceCounts.Assets, document.ResourceCounts.Notices,
		yesNo(document.LifecycleAvailability.FreshActivationAvailable), yesNo(document.LifecycleAvailability.CatalogUpdateAvailable),
		yesNo(document.LifecycleAvailability.LifecycleVerbsAvailable), yesNo(document.LifecycleAvailability.AutomaticDowngrade),
	); err != nil {
		return err
	}
	if len(document.UpdateRoutes) == 0 {
		if _, err := fmt.Fprintln(w, "Update routes: none"); err != nil {
			return err
		}
	} else {
		for _, route := range document.UpdateRoutes {
			if _, err := fmt.Fprintf(w, "Update route: %s -> %s on %s\n", route.FromVersion, route.ToVersion, joinSurfaces(route.ExistingSurfaces)); err != nil {
				return err
			}
		}
	}
	for _, surface := range report.Surfaces {
		if _, err := fmt.Fprintf(w, "Surface contract: %s\n", surface.Surface); err != nil {
			return err
		}
		if err := renderPackShowContract(w, surface.Contract); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Surface intent: %s\n", renderPackShowIntent(surface.Intent)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Intent aliases: %s\n", renderShowAliases(surface.Intent.Aliases)); err != nil {
			return err
		}
	}
	return nil
}

func renderPackShowContract(w io.Writer, contract capabilitypack.LifecycleContract) error {
	if contract.CompatibilityObserved {
		if _, err := fmt.Fprintf(w, "Compatibility: %s\n", contract.Compatibility); err != nil {
			return err
		}
	}
	counts := contract.Counts
	if _, err := fmt.Fprintf(w,
		"Logical resources: %d skill, %d instruction, %d mcp_server, %d lifecycle, %d agent, %d command, %d asset, %d notice\nDependency closure: %s\n",
		counts.Skills, counts.Instructions, counts.MCPServers, counts.Lifecycles, counts.Agents, counts.Commands,
		counts.Assets, counts.Notices, joinFacts(contract.DependencyClosure),
	); err != nil {
		return err
	}
	for _, binding := range contract.Bindings {
		mode := binding.Mode
		if binding.Degradation != "" {
			mode += " (" + binding.Degradation + ")"
		}
		if _, err := fmt.Fprintf(w, "Binding: %s:%s -> %s [%s]; projection=%s name=%s sharing=%s\n",
			binding.Kind, binding.ID, binding.Invocation, mode, binding.Projection, binding.Name, binding.Sharing); err != nil {
			return err
		}
	}
	for _, exclusion := range contract.Exclusions {
		if _, err := fmt.Fprintf(w,
			"Exclusion: %s — %s; resource_kind=%s surface=%s mode=%s code=%s source_paths=%s\n",
			exclusion.ID, exclusion.Reason, factOrNone(exclusion.ResourceKind), factOrNone(string(exclusion.Surface)),
			factOrNone(exclusion.Mode), factOrNone(exclusion.Code), joinFacts(exclusion.SourcePaths),
		); err != nil {
			return err
		}
	}
	for _, mode := range contract.OptionalModes {
		if _, err := fmt.Fprintf(w, "Optional mode: %s — %s; fallback=%s\n", mode.ID, strings.Join(mode.Authorities, ", "), mode.Fallback); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Invocation-time prompt authority: %s\nContract aliases: %s\n%s\n",
		joinFacts(contract.PromptAuthorities), renderShowAliases(contract.Aliases), contract.AuthorityDisclosure); err != nil {
		return err
	}
	return nil
}

func renderPackShowIntent(intent capabilitypack.ShowIntent) string {
	if !intent.Present {
		return "absent"
	}
	return fmt.Sprintf("known active=%s version=%s revision=%d", yesNo(intent.Active), intent.Version, intent.Revision)
}

func renderShowAliases(aliases []capabilitypack.SurfaceAlias) string {
	if len(aliases) == 0 {
		return "none"
	}
	result := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		result = append(result, alias.Kind+":"+alias.ID+"="+alias.Name)
	}
	return strings.Join(result, ", ")
}
