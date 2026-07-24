// Package addyacceptance owns the synthetic, inert Addy acceptance oracle.
//
// The fixture deliberately contains no runner. Callers may copy its bytes into
// disposable roots, but must never execute them.
package addyacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	PackVersion = "1.0.0"
	Release     = "0.6.4"
	Commit      = "98967c45a42b88d6b8fb3a88b7ff6273920763d6"
	Tree        = "3808d3bac44683c5af8979a169b31cb99af47de8"
)

func skillIDs() []string {
	return []string{
		"api-and-interface-design", "browser-testing-with-devtools", "ci-cd-and-automation",
		"code-review-and-quality", "code-simplification", "context-engineering",
		"debugging-and-error-recovery", "deprecation-and-migration", "documentation-and-adrs",
		"doubt-driven-development", "frontend-ui-engineering", "git-workflow-and-versioning",
		"idea-refine", "incremental-implementation", "interview-me", "observability-and-instrumentation",
		"performance-optimization", "planning-and-task-breakdown", "security-and-hardening",
		"shipping-and-launch", "source-driven-development", "spec-driven-development",
		"test-driven-development", "using-agent-skills",
	}
}

func agentIDs() []string {
	return []string{"code-reviewer", "security-auditor", "test-engineer", "web-performance-auditor"}
}
func workflowIDs() []string {
	return []string{"build", "code-simplify", "plan", "review", "ship", "spec", "test", "webperf"}
}
func referenceIDs() []string {
	return []string{
		"accessibility-checklist", "definition-of-done", "observability-checklist", "orchestration-patterns",
		"performance-checklist", "security-checklist", "testing-patterns",
	}
}

type Fixture struct {
	Manifest        Manifest        `json:"manifest"`
	Provenance      Provenance      `json:"provenance"`
	Lifecycle       LifecycleOracle `json:"lifecycle"`
	SupportedRoutes []VersionRoute  `json:"supported_version_routes"`
	AcceptanceRows  []AcceptanceRow `json:"acceptance_rows"`
	Files           []File          `json:"files"`
}

type LifecycleOracle struct {
	ActivationConsent     string   `json:"activation_consent"`
	RemovalConsent        string   `json:"removal_consent"`
	FreshPreflight        bool     `json:"fresh_preflight"`
	AtomicApply           bool     `json:"atomic_apply"`
	RecoveryRequiresPlan  bool     `json:"recovery_requires_new_plan"`
	ReadinessValues       []string `json:"readiness_values"`
	RequireUsableRejects  []string `json:"require_usable_rejects"`
	PreserveShared        bool     `json:"preserve_shared_contributors"`
	SurfaceStateIsolation bool     `json:"surface_state_isolation"`
}

type Provenance struct {
	Repository         string            `json:"repository"`
	RepositoryID       int64             `json:"repository_id"`
	OwnerID            int64             `json:"owner_id"`
	Release            string            `json:"release"`
	TagSHA             string            `json:"tag_sha"`
	TagVerification    Verification      `json:"tag_verification"`
	Commit             string            `json:"commit"`
	CommitParents      []string          `json:"commit_parents"`
	CommitVerification Verification      `json:"commit_verification"`
	Tree               string            `json:"tree"`
	ArchiveSHA256      string            `json:"archive_sha256"`
	SelectedSHA256     map[string]string `json:"selected_resource_sha256"`
	SnapshotSHA256     string            `json:"snapshot_sha256"`
	License            string            `json:"license"`
}

type Verification struct {
	Verified bool   `json:"verified"`
	Reason   string `json:"reason"`
}

type VersionRoute struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Kind      string   `json:"kind"`
	Migration []string `json:"migration"`
	Actions   []string `json:"actions"`
}
type AcceptanceRow struct {
	Row              int      `json:"row"`
	Gate             string   `json:"gate"`
	Fact             string   `json:"fact"`
	NegativeFact     string   `json:"negative_fact"`
	PositiveEvidence []string `json:"positive_evidence"`
	NegativeEvidence []string `json:"negative_evidence"`
}
type File struct {
	Path, Content string
	Mode          uint32
}

type Manifest struct {
	SchemaVersion int          `json:"schema_version"`
	ID            string       `json:"id"`
	Version       string       `json:"version"`
	Surfaces      []string     `json:"surfaces"`
	Provides      []string     `json:"provides"`
	Requires      Requirements `json:"requires"`
	Conflicts     []string     `json:"conflicts"`
	Resources     []Resource   `json:"resources"`
	Contract      Contract     `json:"contract"`
}
type Requirements struct {
	Capabilities []string `json:"capabilities"`
	Tools        []string `json:"tools"`
}
type Resource struct {
	Kind        string     `json:"kind"`
	ID          string     `json:"id"`
	Source      string     `json:"source"`
	Description string     `json:"description,omitempty"`
	Mode        string     `json:"mode,omitempty"`
	Tools       []string   `json:"tools,omitempty"`
	Permissions []string   `json:"permissions,omitempty"`
	Requires    []string   `json:"requires"`
	Bindings    []Binding  `json:"bindings,omitempty"`
	Arguments   *Arguments `json:"arguments,omitempty"`
	License     string     `json:"license,omitempty"`
	Attribution string     `json:"attribution,omitempty"`
}
type Binding struct {
	Surface     string `json:"surface"`
	Projection  string `json:"projection"`
	Name        string `json:"name"`
	Invocation  string `json:"invocation"`
	Mode        string `json:"mode"`
	Degradation string `json:"degradation,omitempty"`
	Sharing     string `json:"sharing"`
}
type Arguments struct {
	Mode        string `json:"mode"`
	Placeholder string `json:"placeholder,omitempty"`
}
type Contract struct {
	Exclusions    []Exclusion    `json:"exclusions"`
	OptionalModes []OptionalMode `json:"optional_modes"`
}
type Exclusion struct {
	ID          string   `json:"id"`
	SourcePaths []string `json:"source_paths"`
	Reason      string   `json:"reason"`
}
type OptionalMode struct {
	ID          string   `json:"id"`
	Authorities []string `json:"authorities"`
	Fallback    string   `json:"fallback"`
}

// Canonical returns a fresh fixture so callers cannot mutate the oracle's
// package-level slices. Its selected content is synthetic and visibly inert.
func Canonical() Fixture {
	resources := make([]Resource, 0, 44)
	for _, id := range skillIDs() {
		requires := []string{}
		if id == "using-agent-skills" {
			for _, ref := range referenceIDs() {
				requires = append(requires, "asset:"+ref)
			}
		}
		resources = append(resources, Resource{Kind: "skill", ID: id, Source: "skills/" + id, Requires: requires, Bindings: nativeBindings("skill", id)})
	}
	for _, id := range agentIDs() {
		resources = append(resources, Resource{Kind: "agent", ID: id, Source: "agents/" + id + ".md", Description: "Synthetic Addy " + id + " persona", Mode: "subagent", Tools: []string{"browser"}, Permissions: []string{"filesystem", "process"}, Requires: []string{"skill:using-agent-skills"}, Bindings: nativeBindings("agent", id)})
	}
	for _, id := range workflowIDs() {
		sourceID := id
		if id == "plan" {
			sourceID = "planning"
		}
		resources = append(resources, Resource{Kind: "command", ID: id, Source: "commands/" + sourceID + ".toml", Requires: workflowRequires(id), Bindings: commandBindings(id), Arguments: &Arguments{Mode: "freeform", Placeholder: "$ARGUMENTS"}})
	}
	for _, id := range referenceIDs() {
		resources = append(resources, Resource{Kind: "asset", ID: id, Source: "references/" + id + ".md", Requires: []string{}})
	}
	resources = append(resources, Resource{Kind: "notice", ID: "mit", Source: "LICENSE", Requires: []string{}, License: "MIT", Attribution: "Copyright 2025 Addy Osmani; synthetic Packy acceptance fixture derived from addyosmani/agent-skills"})
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Kind != resources[j].Kind {
			return resources[i].Kind < resources[j].Kind
		}
		return resources[i].ID < resources[j].ID
	})

	files := sourceFiles(resources)
	files = append(files,
		File{Path: "skills/idea-refine/examples.md", Content: "# Synthetic examples\n", Mode: 0644},
		File{Path: "skills/idea-refine/frameworks.md", Content: "# Synthetic frameworks\n", Mode: 0644},
		File{Path: "skills/idea-refine/refinement-criteria.md", Content: "# Synthetic criteria\n", Mode: 0644},
		File{Path: "skills/idea-refine/scripts/idea-refine.sh", Content: "#!/bin/sh\n# INERT TEST FIXTURE: acceptance must never execute this file.\n: > \"${ADDY_ACCEPTANCE_SENTINEL:?}\"\nexit 97\n", Mode: 0755},
	)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	provenance := Provenance{
		Repository: "addyosmani/agent-skills", RepositoryID: 871993216, OwnerID: 110953,
		Release: Release, TagSHA: "fbe226d2f1d07dff153a368790b6b97960b2da7f", TagVerification: Verification{Reason: "unsigned"},
		Commit: Commit, CommitParents: []string{"3517e072285a7b27f7a1454b8cb54cfb4fb30ac9", "27dcc32b83460aa4b186db3a31305387352a83c7"}, CommitVerification: Verification{Verified: true, Reason: "valid"},
		Tree: Tree, ArchiveSHA256: "6b9f446e2be1598fc90508d15022439737a4f85027b8d30accca316cdeccc46f", License: "MIT",
	}
	provenance.SelectedSHA256, provenance.SnapshotSHA256 = fixtureDigests(resources, files)
	return Fixture{
		Manifest:        Manifest{SchemaVersion: 2, ID: "addy", Version: PackVersion, Surfaces: []string{"codex", "opencode"}, Provides: []string{"workflow:addy"}, Requires: Requirements{Capabilities: []string{}, Tools: []string{}}, Conflicts: []string{}, Resources: resources, Contract: contract()},
		Provenance:      provenance,
		Lifecycle:       LifecycleOracle{ActivationConsent: "reversible-local", RemovalConsent: "destructive-cleanup", FreshPreflight: true, AtomicApply: true, RecoveryRequiresPlan: true, ReadinessValues: []string{"no", "unknown", "yes"}, RequireUsableRejects: []string{"no", "unknown"}, PreserveShared: true, SurfaceStateIsolation: true},
		SupportedRoutes: []VersionRoute{{From: "absent", To: "1.0.0", Kind: "introduction", Migration: []string{}, Actions: []string{"project-complete-surface"}}, {From: "1.0.0", To: "1.0.0", Kind: "exact-no-op", Migration: []string{}, Actions: []string{}}},
		AcceptanceRows:  acceptanceRows(), Files: files,
	}
}

func fixtureDigests(resources []Resource, files []File) (map[string]string, string) {
	filesByPath := make(map[string]File, len(files))
	rows := make([]string, 0, len(files))
	for _, file := range files {
		filesByPath[file.Path] = file
		sum := sha256.Sum256([]byte(file.Content))
		rows = append(rows, fmt.Sprintf("%s\x00%04o\x00%x\n", file.Path, file.Mode, sum))
	}
	sort.Strings(rows)
	snapshot := sha256.Sum256([]byte(strings.Join(rows, "")))
	selected := make(map[string]string, len(resources))
	for _, resource := range resources {
		prefix := resource.Source
		var resourceRows []string
		for path, file := range filesByPath {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				sum := sha256.Sum256([]byte(file.Content))
				resourceRows = append(resourceRows, fmt.Sprintf("%s\x00%04o\x00%x\n", path, file.Mode, sum))
			}
		}
		sort.Strings(resourceRows)
		sum := sha256.Sum256([]byte(strings.Join(resourceRows, "")))
		selected[resource.Kind+":"+resource.ID] = hex.EncodeToString(sum[:])
	}
	return selected, hex.EncodeToString(snapshot[:])
}

func nativeBindings(kind, id string) []Binding {
	return []Binding{{Surface: "codex", Projection: kind, Name: id, Invocation: "$" + id, Mode: "native", Sharing: "exclusive"}, {Surface: "opencode", Projection: kind, Name: id, Invocation: id, Mode: "native", Sharing: "exclusive"}}
}
func commandBindings(id string) []Binding {
	return []Binding{{Surface: "codex", Projection: "skill", Name: id, Invocation: "$" + id, Mode: "degraded", Degradation: "codex-command-as-workflow-skill", Sharing: "exclusive"}, {Surface: "opencode", Projection: "command", Name: id, Invocation: "/" + id, Mode: "native", Sharing: "exclusive"}}
}
func workflowRequires(id string) []string {
	r := []string{"skill:using-agent-skills", "asset:definition-of-done"}
	switch id {
	case "review":
		r = append(r, "agent:code-reviewer")
	case "ship":
		r = append(r, "agent:code-reviewer", "agent:security-auditor", "agent:test-engineer")
	case "test":
		r = append(r, "agent:test-engineer")
	case "webperf":
		r = append(r, "agent:web-performance-auditor")
	}
	sort.Strings(r)
	return r
}

func contract() Contract {
	return Contract{
		Exclusions: []Exclusion{
			{ID: "runtime-hooks", SourcePaths: []string{"hooks/sdd-cache-post.sh", "hooks/sdd-cache-pre.sh", "hooks/session-start.sh", "hooks/simplify-ignore.sh"}, Reason: "hooks are not Addy 1.0.0 runtime resources"},
			{ID: "source-maintenance", SourcePaths: []string{".agents/**", ".claude-plugin/**", ".codex-plugin/**", ".github/**", ".opencode/skills", "AGENTS.md", "evals/**", "scripts/**", "tests/**"}, Reason: "source-only setup, manifests, symlinks, evaluators, validators, tests, and CI remain inert"},
		},
		OptionalModes: []OptionalMode{
			{ID: "browser-network", Authorities: []string{"browser", "network"}, Fallback: "static evidence-only analysis"},
			{ID: "package-tools", Authorities: []string{"package-manager", "process"}, Fallback: "report commands without running them"},
			{ID: "privileged-shipping", Authorities: []string{"commit", "deploy"}, Fallback: "none"},
			{ID: "specialist-fanout", Authorities: []string{"subagent"}, Fallback: "sequential single-agent analysis"},
		},
	}
}

func sourceFiles(resources []Resource) []File {
	out := make([]File, 0, len(resources))
	for _, r := range resources {
		path := r.Source
		content := "# Synthetic Addy acceptance content: " + r.Kind + ":" + r.ID + "\n"
		if r.Kind == "skill" {
			path += "/SKILL.md"
		}
		if r.Kind == "agent" {
			content = fmt.Sprintf("---\nname: %s\ndescription: \"Synthetic Addy %s persona\"\n---\n\n# Synthetic Addy acceptance agent\n", r.ID, r.ID)
		}
		if r.Kind == "command" {
			content = fmt.Sprintf("description = \"Synthetic Addy %s command\"\nprompt = '''Run the synthetic Addy workflow with $ARGUMENTS.'''\n", r.ID)
		}
		if r.Kind == "notice" {
			content = "MIT License\n\nCopyright (c) 2025 Addy Osmani\n\nPermission is hereby granted, free of charge, to any person obtaining a copy...\n"
		}
		out = append(out, File{Path: path, Content: content, Mode: 0644})
	}
	return out
}

func acceptanceRows() []AcceptanceRow {
	names := []string{"exact-provenance", "inert-acquisition", "exclusive-source-ownership", "exact-inventory", "dependency-closure", "exclusions-inert", "schema-parity", "compatibility", "authority-disclosure", "codex-projection", "opencode-projection", "collision-alias", "deterministic-preview", "surface-isolation", "typed-consent", "atomic-recovery", "tri-state-readiness", "pending-optional-excluded", "supported-routes", "exact-ownership-removal"}
	rows := []int{1, 2, 4, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22}
	evidence := []string{
		"internal/addyacceptance:TestExactUpstreamArchiveInventoryAndSupportRemainInert",
		"internal/addyacceptance:TestExactUpstreamArchiveInventoryAndSupportRemainInert",
		"internal/addyacceptance:TestPortableCohortClosesExactInventoryOwnershipIntegrityAndEvolution",
		"internal/addyacceptance:TestCanonicalInventoryAndDeterminism",
		"internal/addyacceptance:TestPortableCohortClosesExactInventoryOwnershipIntegrityAndEvolution",
		"internal/addyacceptance:TestEvidenceEnvelopeUsesStableStructuredOracleDisposableRootsAndOneFactTwins",
		"internal/capabilitypack:TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp",
		"internal/addyacceptance:TestPortableCohortClosesExactInventoryOwnershipIntegrityAndEvolution",
		"internal/addyacceptance:TestLifecycleOracleExposesExactCountsAuthoritiesAndSurfaceBindings",
		"internal/addyacceptance:TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent/codex",
		"internal/addyacceptance:TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent/opencode",
		"internal/capabilitypack:TestCompleteAddyCollisionBlocksUntilExactSurfaceAliasReplans",
		"internal/addyacceptance:TestCompleteSurfaceCohortsAreDeterministicInertAndIndependent",
		"internal/capabilitypack:TestCompleteAddyDualSurfaceFailurePreservesAuthorizedOtherSurface",
		"internal/capabilitypack:TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp",
		"internal/capabilitypack:TestCompleteAddyAtomicAdapterFailureRecordsAttemptAndRequiresFreshRecoveryPlan",
		"internal/capabilitypack:TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct",
		"internal/capabilitypack:TestCompleteAddyReadinessKeepsUnknownPendingOptionalAndExcludedDistinct",
		"internal/capabilitypack:TestCompleteAddyCohortUsesTypedConsentFreshVerificationAndExactNoOp",
		"internal/capabilitypack:TestCompleteAddyExactOwnershipRemovalBlocksDriftWithoutEffects",
	}
	out := make([]AcceptanceRow, len(rows))
	for i, row := range rows {
		negative := fmt.Sprintf("internal/addyacceptance:TestEveryOwnedMatrixRowHasAStableOneFactNegativeCase/row-%02d", row)
		out[i] = AcceptanceRow{Row: row, Gate: names[i], Fact: "positive:" + names[i], NegativeFact: "one-fact-negative:" + names[i], PositiveEvidence: []string{evidence[i]}, NegativeEvidence: []string{negative}}
	}
	return out
}

// CanonicalJSON is the deterministic oracle encoding (two-space indentation
// and one trailing newline).
func CanonicalJSON() ([]byte, error) {
	b, err := json.MarshalIndent(Canonical(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
func SHA256() (string, error) {
	b, err := CanonicalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// NegativeTwin changes exactly one decision-relevant fact in canonical JSON.
func NegativeTwin(fact string) ([]byte, error) {
	f := Canonical()
	switch fact {
	case "missing-skill":
		f.Manifest.Resources = f.Manifest.Resources[1:]
	case "moved-tag":
		f.Provenance.Commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	case "executable-helper-selected":
		f.Manifest.Resources = append(f.Manifest.Resources, Resource{Kind: "asset", ID: "forbidden-helper", Source: "skills/idea-refine/scripts/idea-refine.sh", Requires: []string{}})
	default:
		return nil, fmt.Errorf("unknown negative twin %q", fact)
	}
	b, err := json.MarshalIndent(f, "", "  ")
	return append(b, '\n'), err
}

// WriteSnapshot writes only synthetic fixture bytes beneath root. The caller
// must supply a disposable root; an existing non-empty root is rejected.
func WriteSnapshot(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err = os.MkdirAll(root, 0700); err != nil {
			return err
		}
	}
	if len(entries) != 0 {
		return fmt.Errorf("fixture root must be empty: %s", root)
	}
	for _, f := range Canonical().Files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(f.Content), os.FileMode(f.Mode)); err != nil {
			return err
		}
	}
	return nil
}
