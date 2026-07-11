package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/matty/internal/capabilitypack"
)

func newPackCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "pack", Short: "Discover and manage capability packs"}
	cmd.AddCommand(newPackListCommand(opts), newPackShowCommand(opts), newPackStatusCommand(opts))
	return cmd
}

func newPackStatusCommand(opts Options) *cobra.Command {
	var surface string
	cmd := &cobra.Command{
		Use: "status [pack]", Short: "Inspect capability pack status", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, err := discoverPackCatalog(opts)
			if err != nil {
				return err
			}
			paths, err := ResolvePaths(opts.Env)
			if err != nil {
				return err
			}
			packID := ""
			if len(args) == 1 {
				packID = args[0]
			}
			facade := capabilitypack.NewFacade(catalog, map[capabilitypack.Surface]capabilitypack.SurfaceInspector{
				capabilitypack.SurfaceCodex:    capabilitypack.NewCodexInspector(paths.CodexPromptFile),
				capabilitypack.SurfaceOpenCode: capabilitypack.NewOpenCodeInspector(paths.OpenCodeConfigFile, paths.OpenCodePromptFile),
			})
			report, err := facade.Status(cmd.Context(), capabilitypack.StatusRequest{PackID: packID, Surface: capabilitypack.Surface(surface)})
			if err != nil {
				return err
			}
			if packID == "" {
				return renderPackStatusOverview(cmd, report)
			}
			return renderPackStatusDetail(cmd, report.Entries[0])
		},
	}
	cmd.Flags().StringVar(&surface, "surface", "", "CLI surface (codex or opencode)")
	return cmd
}

func renderPackStatusOverview(cmd *cobra.Command, report capabilitypack.StatusReport) error {
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "PACK\tSURFACE\tINTENT\tATTEMPT\tCONFIGURED\tAUTHORIZED\tUSABLE\tACTION")
	for _, entry := range report.Entries {
		configured, authorized, usable := "—", "—", "—"
		if entry.Intent.Active {
			configured = yesNo(entry.Readiness.Configured)
			authorized = yesNo(entry.Readiness.Authorized)
			usable = yesNo(entry.Readiness.Usable)
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", entry.Pack.ID, entry.Surface, renderIntent(entry.Intent), renderAttempt(entry.LatestAttempt), configured, authorized, usable, renderPendingAction(entry.PendingHumanActions))
	}
	return writer.Flush()
}

func renderPackStatusDetail(cmd *cobra.Command, entry capabilitypack.StatusEntry) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s on %s\nIntent: %s\nLatest attempt: %s\nReadiness: configured=%s, authorized=%s, usable=%s\nProjections: %d verified; %d drifted; %d ambiguous\nPending human actions: %s\n", entry.Pack.ID, entry.Pack.Version, entry.Surface, renderIntent(entry.Intent), renderAttempt(entry.LatestAttempt), yesNo(entry.Readiness.Configured), yesNo(entry.Readiness.Authorized), yesNo(entry.Readiness.Usable), entry.Projections.Verified, entry.Projections.Drifted, entry.Projections.Ambiguous, renderPendingAction(entry.PendingHumanActions))
	return err
}

func renderIntent(intent capabilitypack.IntentStatus) string {
	if !intent.Active {
		return "inactive"
	}
	return fmt.Sprintf("active at revision %d", intent.Revision)
}

func renderAttempt(attempt *capabilitypack.AttemptStatus) string {
	if attempt == nil {
		return "none"
	}
	if attempt.PlanID == "" {
		return attempt.Outcome
	}
	return fmt.Sprintf("%s (%s)", attempt.Outcome, attempt.PlanID)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func renderPendingAction(actions []string) string {
	if len(actions) == 0 {
		return "none"
	}
	return strings.Join(actions, "; ")
}

func newPackListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List available capability packs", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			catalog, err := discoverPackCatalog(opts)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "PACK\tVERSION\tDESCRIPTION\tAVAILABLE ON")
			for _, pack := range catalog.List() {
				surfaces := make([]string, len(pack.Surfaces))
				for i, surface := range pack.Surfaces {
					surfaces[i] = string(surface)
				}
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", pack.ID, pack.Version, pack.Description, strings.Join(surfaces, ", "))
			}
			return writer.Flush()
		},
	}
}

func newPackShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use: "show <pack>", Short: "Show a capability pack", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			catalog, err := discoverPackCatalog(opts)
			if err != nil {
				return err
			}
			pack, err := catalog.Show(args[0])
			if err != nil {
				return err
			}
			counts := pack.ResourceCounts()
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\nDescription: %s\nSupported CLI surfaces: %s\nProvides capabilities: %s\nRequires capabilities: %s\nRequires global tools: %s\nConflicts with capabilities: %s\nResources: %d skill, %d instruction, %d mcp_server, %d lifecycle\n",
				pack.ID, pack.Version, pack.Description, joinSurfaces(pack.Surfaces), joinOrNone(pack.Provides), joinOrNone(pack.Requires.Capabilities), joinOrNone(pack.Requires.Tools), joinOrNone(pack.Conflicts), counts.Skills, counts.Instructions, counts.MCPServers, counts.Lifecycles)
			return nil
		},
	}
}

func discoverPackCatalog(opts Options) (capabilitypack.Catalog, error) {
	paths, err := ResolvePaths(opts.Env)
	if err != nil {
		return capabilitypack.Catalog{}, err
	}
	return capabilitypack.Discover(paths.BundleSourceRoot)
}

func joinSurfaces(values []capabilitypack.Surface) string {
	items := make([]string, len(values))
	for i, value := range values {
		items[i] = string(value)
	}
	return strings.Join(items, ", ")
}
func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
