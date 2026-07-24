package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestPackStatusDetailRendersOrderedOptionalAuthorityFacts(t *testing.T) {
	var output strings.Builder
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	entry := capabilitypack.StatusEntry{
		Pack:    capabilitypack.Pack{ID: "addy", Version: "1.1.0"},
		Surface: capabilitypack.SurfaceClaude,
		OptionalAuthorities: []capabilitypack.OptionalAuthorityObservation{
			{ModeID: "shipping", Authority: "deploy", State: capabilitypack.OptionalAuthorityUnavailable, Fallback: "none"},
			{ModeID: "browser-network", Authority: "network", State: capabilitypack.OptionalAuthorityAvailable, Fallback: "static evidence-only analysis"},
			{ModeID: "browser-network", Authority: "browser", State: capabilitypack.OptionalAuthorityUnknown, Fallback: "static evidence-only analysis"},
		},
	}
	if err := renderPackStatusDetail(cmd, entry); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	first := strings.Index(got, "Optional authority: mode=browser-network authority=browser state=unknown fallback=static evidence-only analysis")
	second := strings.Index(got, "Optional authority: mode=browser-network authority=network state=available fallback=static evidence-only analysis")
	third := strings.Index(got, "Optional authority: mode=shipping authority=deploy state=unavailable fallback=none")
	if first < 0 || second <= first || third <= second {
		t.Fatalf("optional authority facts are missing or unordered:\n%s", got)
	}
	if !strings.Contains(got, "Readiness: configured=unknown, authorized=unknown, usable=unknown") {
		t.Fatalf("optional authority facts changed readiness rendering:\n%s", got)
	}
}

func TestPackShowRenderersExposeTheSameWithdrawnHistoryRouteAndIntentFacts(t *testing.T) {
	alias := capabilitypack.SurfaceAlias{Kind: "agent", ID: "reviewer", Name: "addy-reviewer"}
	counts := capabilitypack.ResourceCounts{
		Skills: 24, Agents: 4, Commands: 8, Assets: 7, Notices: 1,
	}
	contract := capabilitypack.LifecycleContract{
		Compatibility: capabilitypack.CompatibilityComplete, CompatibilityObserved: true,
		Counts: counts, DependencyClosure: []string{"skill:using-agent-skills"},
		Bindings: []capabilitypack.LifecycleBinding{{
			Kind: "agent", ID: "reviewer", Projection: "agent", Name: alias.Name,
			Invocation: alias.Name, Mode: "native", Sharing: "exclusive",
		}},
		Exclusions: []capabilitypack.LifecycleExclusion{}, OptionalModes: []capabilitypack.OptionalMode{{
			ID: "browser-network", Authorities: []string{"browser", "network"}, Fallback: "static evidence",
		}},
		PromptAuthorities: []string{"browser", "network"}, Aliases: []capabilitypack.SurfaceAlias{alias},
		AuthorityDisclosure: "Host approval remains required.",
	}
	report := capabilitypack.ShowReport{
		Detail: capabilitypack.CatalogDetail{
			Withdrawn: true,
			Pack: capabilitypack.Pack{
				ID: "addy", Version: "1.1.0", Description: "Agent workflows",
				Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceClaude},
				Provides: []string{"workflow:addy"},
			},
			HistoricalVersions: []string{"1.0.0", "1.1.0"},
			UpdateRoutes: []capabilitypack.UpdateRoute{{
				FromVersion: "1.0.0", ToVersion: "1.1.0",
				ExistingSurfaces: []capabilitypack.Surface{capabilitypack.SurfaceCodex, capabilitypack.SurfaceOpenCode},
			}},
		},
		SourceIdentity: capabilitypack.PackSourceIdentity{
			PackID: "addy", Version: "1.1.0", SchemaVersion: 3,
			Limitation: "Upstream provenance is unavailable.",
		},
		ResourceCounts: counts,
		Surfaces: []capabilitypack.ShowSurfaceReport{{
			Surface: capabilitypack.SurfaceClaude, Contract: contract,
			Intent: capabilitypack.ShowIntent{
				Present: true, Active: true, Version: "1.1.0", Revision: 7,
				Aliases: []capabilitypack.SurfaceAlias{alias},
			},
		}},
	}

	var structured bytes.Buffer
	if err := renderPackShowJSON(&structured, report); err != nil {
		t.Fatal(err)
	}
	var document packShowJSON
	if err := json.Unmarshal(structured.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateStructuredOutput(t, root, "pack-show.schema.json", structured.Bytes()); err != nil {
		t.Fatalf("withdrawn structured show failed schema validation: %v\n%s", err, structured.String())
	}
	if document.CatalogState != "withdrawn" ||
		len(document.HistoricalVersions) != 2 ||
		len(document.UpdateRoutes) != 1 ||
		len(document.SurfaceContracts) != 1 ||
		document.SurfaceContracts[0].Intent.State != "known" ||
		document.SurfaceContracts[0].Intent.Active == nil ||
		!*document.SurfaceContracts[0].Intent.Active ||
		len(document.SurfaceContracts[0].Intent.Aliases) != 1 ||
		document.ResourceCounts != counts {
		t.Fatalf("structured show facts = %#v", document)
	}

	var human bytes.Buffer
	if err := renderPackShowHuman(&human, report); err != nil {
		t.Fatal(err)
	}
	for _, fact := range []string{
		"Catalog state: withdrawn",
		"Source identity: pack=addy version=1.1.0 schema=3",
		"Source limitation: Upstream provenance is unavailable.",
		"Historical versions: 1.0.0, 1.1.0",
		"Update route: 1.0.0 -> 1.1.0 on codex, opencode",
		"Resources: 24 skill, 0 instruction, 0 mcp_server, 0 lifecycle, 4 agent, 8 command, 7 asset, 1 notice",
		"Dependency closure: skill:using-agent-skills",
		"Optional mode: browser-network — browser, network; fallback=static evidence",
		"Contract aliases: agent:reviewer=addy-reviewer",
		"Surface intent: known active=yes version=1.1.0 revision=7",
		"Intent aliases: agent:reviewer=addy-reviewer",
	} {
		if !strings.Contains(human.String(), fact) {
			t.Fatalf("human show missing %q:\n%s", fact, human.String())
		}
	}

	var repeated bytes.Buffer
	if err := renderPackShowJSON(&repeated, report); err != nil {
		t.Fatal(err)
	}
	if repeated.String() != structured.String() {
		t.Fatalf("structured show is nondeterministic:\nfirst=%s\nsecond=%s", structured.String(), repeated.String())
	}
}
