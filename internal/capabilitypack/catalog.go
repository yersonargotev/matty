// Package capabilitypack owns capability-pack discovery and policy.
package capabilitypack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

const schemaVersion = 1

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
)

type Surface string

const (
	SurfaceCodex    Surface = "codex"
	SurfaceOpenCode Surface = "opencode"
)

type Requirements struct {
	Capabilities []string `json:"capabilities"`
	Tools        []string `json:"tools"`
}

type Resource struct {
	Kind    string
	ID      string
	Source  string
	Command string
	Args    []string
}

type Pack struct {
	ID          string
	Version     string
	Description string
	Surfaces    []Surface
	Provides    []string
	Requires    Requirements
	Conflicts   []string
	Resources   []Resource
}

type ResourceCounts struct {
	Skills       int
	Instructions int
	MCPServers   int
	Lifecycles   int
}

func (p Pack) ResourceCounts() ResourceCounts {
	var counts ResourceCounts
	for _, resource := range p.Resources {
		switch resource.Kind {
		case "skill":
			counts.Skills++
		case "instruction":
			counts.Instructions++
		case "mcp_server":
			counts.MCPServers++
		case "lifecycle":
			counts.Lifecycles++
		}
	}
	return counts
}

type Catalog struct {
	packs                 []Pack
	bundleRoot            string
	entries               []catalogEntry
	allowSyntheticHistory bool
	deferSourceValidation bool
	transactionHeld       bool
}

type catalogEntry struct {
	ID          string
	Description string
	Surfaces    []Surface
}

var initialCatalog = []catalogEntry{
	{ID: "engram", Description: "Persistent memory for agent work", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
	{ID: "matty", Description: "Matty workflow", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
}

// Discover loads the strict initial catalog from a Packy-owned bundle root.
func Discover(bundleRoot string) (Catalog, error) {
	return discoverCatalog(bundleRoot, initialCatalog)
}

// DiscoverForDurableIntents loads catalog metadata while deferring current
// source validation until a catalog-current pack is selected. This lets an
// existing pinned intent be reproduced solely from its historical artifact.
func DiscoverForDurableIntents(bundleRoot string) (Catalog, error) {
	return discoverCatalogWithSourceValidation(bundleRoot, initialCatalog, false)
}

func discoverCatalog(bundleRoot string, entries []catalogEntry) (Catalog, error) {
	return discoverCatalogWithSourceValidation(bundleRoot, entries, true)
}

func discoverCatalogWithSourceValidation(bundleRoot string, entries []catalogEntry, validateSources bool) (Catalog, error) {
	var catalog Catalog
	err := bundletransaction.WithExclusive(context.Background(), filepath.Dir(filepath.Clean(bundleRoot)), func() error {
		var err error
		catalog, err = discoverCatalogUnlocked(bundleRoot, entries, validateSources)
		return err
	})
	return catalog, err
}

func discoverCatalogUnlocked(bundleRoot string, entries []catalogEntry, validateSources bool) (Catalog, error) {
	packs := make([]Pack, 0, len(entries))
	for _, entry := range entries {
		manifestPath := filepath.Join(bundleRoot, "packs", entry.ID, "pack.json")
		pack, err := decodeManifestWithSourceValidation(manifestPath, bundleRoot, validateSources)
		if err != nil {
			return Catalog{}, err
		}
		if pack.ID != entry.ID {
			return Catalog{}, fmt.Errorf("catalog entry %q: manifest id is %q", entry.ID, pack.ID)
		}
		pack.Description = entry.Description
		pack.Surfaces = append([]Surface(nil), entry.Surfaces...)
		if err := validateSurfaces(pack.Surfaces); err != nil {
			return Catalog{}, fmt.Errorf("pack %q: %w", pack.ID, err)
		}
		packs = append(packs, pack)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].ID < packs[j].ID })
	return Catalog{packs: packs, bundleRoot: bundleRoot, entries: append([]catalogEntry(nil), entries...), deferSourceValidation: !validateSources}, nil
}

func (c Catalog) refreshed() (Catalog, error) {
	if c.bundleRoot == "" {
		return c, nil
	}
	var refreshed Catalog
	err := c.withBundleLock(context.Background(), func(locked Catalog) error {
		var err error
		refreshed, err = discoverCatalogUnlocked(c.bundleRoot, c.entries, !c.deferSourceValidation)
		refreshed.allowSyntheticHistory = c.allowSyntheticHistory
		refreshed.transactionHeld = locked.transactionHeld
		return err
	})
	return refreshed, err
}

func (c Catalog) withBundleLock(ctx context.Context, observe func(Catalog) error) error {
	if c.bundleRoot == "" || c.transactionHeld {
		return observe(c)
	}
	return bundletransaction.WithExclusive(ctx, filepath.Dir(filepath.Clean(c.bundleRoot)), func() error {
		c.transactionHeld = true
		return observe(c)
	})
}

func (c Catalog) List() []Pack {
	packs := make([]Pack, len(c.packs))
	for i, pack := range c.packs {
		packs[i] = clonePack(pack)
	}
	return packs
}

// ListCurrent returns only after every advertised catalog-current pack has
// passed the same source validation as direct current selection.
func (c Catalog) ListCurrent() ([]Pack, error) {
	var packs []Pack
	err := c.withBundleLock(context.Background(), func(locked Catalog) error {
		fresh, err := locked.refreshed()
		if err != nil {
			return err
		}
		packs = make([]Pack, 0, len(fresh.packs))
		for _, metadata := range fresh.packs {
			pack, err := fresh.showUnlocked(metadata.ID)
			if err != nil {
				return err
			}
			packs = append(packs, pack)
		}
		return nil
	})
	return packs, err
}

func (c Catalog) Show(id string) (Pack, error) {
	if !c.deferSourceValidation {
		return c.showUnlocked(id)
	}
	var pack Pack
	err := c.withBundleLock(context.Background(), func(locked Catalog) error {
		fresh, err := locked.refreshed()
		if err != nil {
			return err
		}
		pack, err = fresh.showUnlocked(id)
		return err
	})
	return pack, err
}

func (c Catalog) showUnlocked(id string) (Pack, error) {
	for _, pack := range c.packs {
		if pack.ID == id {
			if c.deferSourceValidation {
				if err := validatePackSources(pack, c.bundleRoot); err != nil {
					return Pack{}, fmt.Errorf("invalid catalog-current pack %q: %w", id, err)
				}
			}
			return clonePack(pack), nil
		}
	}
	return Pack{}, fmt.Errorf("unknown capability pack %q; run `packy pack list` to see available packs", id)
}

func (c Catalog) catalogMetadata(id string) (Pack, error) {
	for _, pack := range c.packs {
		if pack.ID == id {
			return clonePack(pack), nil
		}
	}
	return Pack{}, fmt.Errorf("unknown capability pack %q; run `packy pack list` to see available packs", id)
}

func clonePack(pack Pack) Pack {
	pack.Surfaces = append([]Surface(nil), pack.Surfaces...)
	pack.Provides = append([]string(nil), pack.Provides...)
	pack.Requires.Capabilities = append([]string(nil), pack.Requires.Capabilities...)
	pack.Requires.Tools = append([]string(nil), pack.Requires.Tools...)
	pack.Conflicts = append([]string(nil), pack.Conflicts...)
	pack.Resources = append([]Resource(nil), pack.Resources...)
	for i := range pack.Resources {
		pack.Resources[i].Args = append([]string(nil), pack.Resources[i].Args...)
	}
	return pack
}

type manifest struct {
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	Version       string            `json:"version"`
	Provides      []string          `json:"provides"`
	Requires      Requirements      `json:"requires"`
	Conflicts     []string          `json:"conflicts"`
	Resources     []json.RawMessage `json:"resources"`
}

func decodeManifest(path, bundleRoot string) (Pack, error) {
	return decodeManifestWithSourceValidation(path, bundleRoot, true)
}

func decodeManifestWithSourceValidation(path, bundleRoot string, validateSources bool) (Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, fmt.Errorf("read pack manifest %s: %w", path, err)
	}
	var raw manifest
	if err := strictDecode(data, &raw); err != nil {
		return Pack{}, fmt.Errorf("decode pack manifest %s: %w", path, err)
	}
	pack := Pack{ID: raw.ID, Version: raw.Version, Provides: raw.Provides, Requires: raw.Requires, Conflicts: raw.Conflicts}
	for i, encoded := range raw.Resources {
		resource, err := decodeResource(encoded)
		if err != nil {
			return Pack{}, fmt.Errorf("pack %q resource %d: %w", raw.ID, i, err)
		}
		pack.Resources = append(pack.Resources, resource)
	}
	if err := validatePackMetadata(pack, raw.SchemaVersion); err != nil {
		return Pack{}, fmt.Errorf("invalid pack manifest %s: %w", path, err)
	}
	if validateSources {
		if err := validatePackSources(pack, bundleRoot); err != nil {
			return Pack{}, fmt.Errorf("invalid pack manifest %s: %w", path, err)
		}
	}
	return pack, nil
}

func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func decodeResource(data []byte) (Resource, error) {
	var discriminator struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return Resource{}, err
	}
	switch discriminator.Kind {
	case "skill", "instruction":
		var raw struct{ Kind, ID, Source string }
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Source: raw.Source}, nil
	case "mcp_server":
		var raw struct {
			Kind, ID, Command string
			Args              []string
		}
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID, Command: raw.Command, Args: raw.Args}, nil
	case "lifecycle":
		var raw struct{ Kind, ID string }
		if err := strictDecode(data, &raw); err != nil {
			return Resource{}, err
		}
		return Resource{Kind: raw.Kind, ID: raw.ID}, nil
	default:
		return Resource{}, fmt.Errorf("unsupported resource kind %q", discriminator.Kind)
	}
}

func validatePack(pack Pack, version int, bundleRoot string) error {
	if err := validatePackMetadata(pack, version); err != nil {
		return err
	}
	return validatePackSources(pack, bundleRoot)
}

func validatePackMetadata(pack Pack, version int) error {
	if version != schemaVersion {
		return fmt.Errorf("schema_version must be %d", schemaVersion)
	}
	if !idPattern.MatchString(pack.ID) {
		return fmt.Errorf("id %q must be lowercase kebab-case", pack.ID)
	}
	if !validSemver(pack.Version) {
		return fmt.Errorf("version %q must be SemVer", pack.Version)
	}
	if pack.Provides == nil || pack.Requires.Capabilities == nil || pack.Requires.Tools == nil || pack.Conflicts == nil || pack.Resources == nil {
		return fmt.Errorf("provides, requires.capabilities, requires.tools, conflicts, and resources are required arrays")
	}
	seenCapabilities := map[string]string{}
	for _, group := range []struct {
		name   string
		values []string
	}{{"provides", pack.Provides}, {"requires.capabilities", pack.Requires.Capabilities}, {"conflicts", pack.Conflicts}} {
		for _, capability := range group.values {
			if err := validateCapability(capability); err != nil {
				return fmt.Errorf("%s: %w", group.name, err)
			}
			if previous, ok := seenCapabilities[capability]; ok {
				return fmt.Errorf("capability %q appears in both %s and %s", capability, previous, group.name)
			}
			seenCapabilities[capability] = group.name
		}
	}
	seenTools := map[string]bool{}
	for _, tool := range pack.Requires.Tools {
		if !idPattern.MatchString(tool) {
			return fmt.Errorf("required tool %q must be lowercase kebab-case", tool)
		}
		if seenTools[tool] {
			return fmt.Errorf("duplicate required tool %q", tool)
		}
		seenTools[tool] = true
	}
	seenResources := map[string]bool{}
	for _, resource := range pack.Resources {
		if !idPattern.MatchString(resource.ID) {
			return fmt.Errorf("resource id %q must be lowercase kebab-case", resource.ID)
		}
		identity := resource.Kind + ":" + resource.ID
		if seenResources[identity] {
			return fmt.Errorf("duplicate resource %q", identity)
		}
		seenResources[identity] = true
		if _, duplicate := seenCapabilities[identity]; duplicate {
			return fmt.Errorf("resource capability %q must not be declared at top level", identity)
		}
		switch resource.Kind {
		case "skill", "instruction":
			if err := validateSourcePath(resource.Source); err != nil {
				return fmt.Errorf("resource %q source: %w", identity, err)
			}
		case "mcp_server":
			if strings.TrimSpace(resource.Command) == "" {
				return fmt.Errorf("resource %q command is required", identity)
			}
			if resource.Args == nil {
				return fmt.Errorf("resource %q args is required", identity)
			}
		case "lifecycle":
		default:
			return fmt.Errorf("unsupported resource kind %q", resource.Kind)
		}
	}
	return nil
}

func validatePackSources(pack Pack, bundleRoot string) error {
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" && resource.Kind != "instruction" {
			continue
		}
		if err := validateSource(bundleRoot, resource); err != nil {
			return fmt.Errorf("resource %q source: %w", resource.Kind+":"+resource.ID, err)
		}
	}
	return nil
}

func validateSourcePath(source string) error {
	if source == "" || filepath.IsAbs(source) {
		return fmt.Errorf("%q must be a relative path", source)
	}
	clean := filepath.Clean(source)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%q escapes the bundle root", source)
	}
	return nil
}

func validateCapability(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || !idPattern.MatchString(parts[0]) || !idPattern.MatchString(parts[1]) {
		return fmt.Errorf("capability %q must have two lowercase kebab-case segments", value)
	}
	return nil
}

func validSemver(version string) bool {
	if !semverPattern.MatchString(version) {
		return false
	}
	withoutBuild := strings.SplitN(version, "+", 2)[0]
	parts := strings.SplitN(withoutBuild, "-", 2)
	if len(parts) == 1 {
		return true
	}
	for _, identifier := range strings.Split(parts[1], ".") {
		if len(identifier) > 1 && identifier[0] == '0' {
			numeric := true
			for _, char := range identifier {
				if char < '0' || char > '9' {
					numeric = false
					break
				}
			}
			if numeric {
				return false
			}
		}
	}
	return true
}

func validateSource(root string, resource Resource) error {
	source := resource.Source
	if err := validateSourcePath(source); err != nil {
		return err
	}
	clean := filepath.Clean(source)
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve bundle root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil {
		return fmt.Errorf("resolve %q: %w", source, err)
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%q resolves outside the bundle root", source)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("inspect %q: %w", source, err)
	}
	if resource.Kind == "skill" {
		if !info.IsDir() {
			return fmt.Errorf("%q must be a skill directory", source)
		}
		if _, err := os.Stat(filepath.Join(resolved, "SKILL.md")); err != nil {
			return fmt.Errorf("%q missing SKILL.md: %w", source, err)
		}
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("%q must be an instruction file", source)
	}
	return nil
}

func validateSurfaces(surfaces []Surface) error {
	if len(surfaces) == 0 {
		return fmt.Errorf("at least one supported CLI surface is required")
	}
	seen := map[Surface]bool{}
	for _, surface := range surfaces {
		if surface != SurfaceCodex && surface != SurfaceOpenCode {
			return fmt.Errorf("unsupported CLI surface %q", surface)
		}
		if seen[surface] {
			return fmt.Errorf("duplicate CLI surface %q", surface)
		}
		seen[surface] = true
	}
	return nil
}
