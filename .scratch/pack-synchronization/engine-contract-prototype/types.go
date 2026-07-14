// PROTOTYPE — throw this package away after the engine contract is decided.
package main

import "context"

type Selector struct {
	Mode string `json:"mode"` // stable-release, prerelease, or commit
	Ref  string `json:"ref,omitempty"`
}

type SourceConfig struct {
	ID           string    `json:"id"`
	Repository   string    `json:"repository"`
	RepositoryID int64     `json:"repository_id"`
	Bindings     []Binding `json:"bindings"`
}

type Binding struct {
	PackID       string `json:"pack_id"`
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id"`
	UpstreamPath string `json:"upstream_path"`
	VendoredPath string `json:"vendored_path"`
}

type Verification struct {
	Verified bool   `json:"verified"`
	Reason   string `json:"reason"`
}

type Candidate struct {
	Repository   string       `json:"repository"`
	RepositoryID int64        `json:"repository_id"`
	Release      string       `json:"release,omitempty"`
	ReleaseID    int64        `json:"release_id,omitempty"`
	PublishedAt  string       `json:"published_at,omitempty"`
	Draft        bool         `json:"draft,omitempty"`
	Prerelease   bool         `json:"prerelease"`
	Commit       string       `json:"commit"`
	TagObject    string       `json:"tag_object,omitempty"`
	TagMoved     bool         `json:"tag_moved,omitempty"`
	Archived     bool         `json:"archived,omitempty"`
	Disabled     bool         `json:"disabled,omitempty"`
	Public       bool         `json:"public"`
	Verification Verification `json:"verification"`
}

// SourceGateway is the only provider boundary. The engine owns policy; a
// public-GitHub implementation would only observe metadata and acquire bytes.
type SourceGateway interface {
	Releases(context.Context, SourceConfig) ([]Candidate, error)
	ResolveExplicit(context.Context, SourceConfig, Selector) (Candidate, error)
	Acquire(context.Context, Candidate, string) error
}

type FileEvidence struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type LockedResource struct {
	Binding
	SHA256 string         `json:"sha256"`
	Files  []FileEvidence `json:"files"`
}

type Lock struct {
	SchemaVersion int              `json:"schema_version"`
	SourceID      string           `json:"source_id"`
	Repository    string           `json:"repository"`
	RepositoryID  int64            `json:"repository_id"`
	Selection     Selector         `json:"selection"`
	Release       string           `json:"release,omitempty"`
	TagObject     string           `json:"tag_object,omitempty"`
	Commit        string           `json:"commit"`
	Snapshot      string           `json:"snapshot_sha256"`
	Resources     []LockedResource `json:"resources"`
}

type PackManifest struct {
	ID        string             `json:"id"`
	Version   string             `json:"version"`
	Resources []ManifestResource `json:"resources"`
}

type ManifestResource struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Source string `json:"source"`
}

type Change struct {
	Kind       string `json:"kind"`
	PackID     string `json:"pack_id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	Path       string `json:"path,omitempty"`
	Before     string `json:"before,omitempty"`
	After      string `json:"after,omitempty"`
}

type PackImpact struct {
	PackID           string   `json:"pack_id"`
	CurrentVersion   string   `json:"current_version"`
	MechanicalFloor  string   `json:"mechanical_floor"`
	SemanticEvidence bool     `json:"semantic_evidence_required"`
	Reasons          []string `json:"reasons"`
}

type Preconditions struct {
	BundleSHA256 string `json:"bundle_sha256"`
	LockSHA256   string `json:"lock_sha256"`
}

type Plan struct {
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	Status        string            `json:"status"`
	SourceID      string            `json:"source_id"`
	Selector      Selector          `json:"selector"`
	Candidate     Candidate         `json:"candidate"`
	Changes       []Change          `json:"changes"`
	AffectedPacks []PackImpact      `json:"affected_packs"`
	Notices       []string          `json:"notices,omitempty"`
	Blockers      []string          `json:"blockers,omitempty"`
	Preconditions Preconditions     `json:"preconditions"`
	GeneratedLock Lock              `json:"generated_lock"`
	PreviousLock  Lock              `json:"previous_lock"`
	Historical    string            `json:"historical_contract"`
	files         map[string][]byte // sealed candidate bytes, never rendered
}

type Classification struct {
	Level           string `json:"level"`
	ProposedVersion string `json:"proposed_version"`
	Rationale       string `json:"rationale"`
	ClassifierType  string `json:"classifier_type"`
	ClassifierID    string `json:"classifier_id"`
	Migration       string `json:"migration,omitempty"`
}

type CheckRequest struct {
	Source         SourceConfig
	Selector       Selector
	RepositoryRoot string
	TempRoot       string
	Historical     string // contract-snapshot or immutable-artifact
}

type ApplyRequest struct {
	CheckRequest
	Plan            Plan
	Classifications map[string]Classification
}

type ApplyResult struct {
	Status    string   `json:"status"`
	PlanID    string   `json:"plan_id"`
	Changed   bool     `json:"changed"`
	Written   []string `json:"written,omitempty"`
	Recovered bool     `json:"recovered,omitempty"`
}
