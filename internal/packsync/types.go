// Package packsync owns deterministic repository-local source inspection and
// transactional replacement of the complete Matty bundle.
// Upstream content is data only: this package inventories, hashes, and compares
// it, but never executes it.
package packsync

import (
	"context"
	"time"
)

type SelectorMode string

const (
	SelectorStableRelease SelectorMode = "stable-release"
	SelectorPrerelease    SelectorMode = "prerelease"
	SelectorCommit        SelectorMode = "commit"
	LockGeneratorName                  = "matty-packsync"
	LockGeneratorVersion               = "1"
	GitHubAPIVersion                   = "2022-11-28"
)

type Selector struct {
	Mode SelectorMode `json:"mode"`
	Ref  string       `json:"ref,omitempty"`
}

type Config struct {
	SchemaVersion int            `json:"schema_version"`
	Sources       []SourceConfig `json:"sources"`
}

type SourceConfig struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"`
	Repository string    `json:"repository"`
	Selector   Selector  `json:"selector"`
	Resources  []Binding `json:"resources"`
}

type Binding struct {
	PackID       string `json:"pack_id"`
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id"`
	UpstreamPath string `json:"upstream_path"`
	VendoredPath string `json:"vendored_path,omitempty"`
}

type Release struct {
	ID          int64     `json:"id"`
	NodeID      string    `json:"node_id"`
	Tag         string    `json:"tag"`
	Name        string    `json:"name"`
	Target      string    `json:"target_commitish"`
	Immutable   bool      `json:"immutable"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Author      Actor     `json:"author"`
}

type Actor struct {
	Login  string `json:"login"`
	ID     int64  `json:"id"`
	NodeID string `json:"node_id"`
}

type Verification struct {
	Verified        bool       `json:"verified"`
	Reason          string     `json:"reason"`
	VerifiedAt      *time.Time `json:"verified_at"`
	SignatureSHA256 *string    `json:"signature_sha256"`
	PayloadSHA256   *string    `json:"payload_sha256"`
}

type TagObject struct {
	SHA          string       `json:"sha"`
	Name         string       `json:"name"`
	TargetSHA    string       `json:"target_sha"`
	TargetType   string       `json:"target_type"`
	Verification Verification `json:"verification"`
}

type Candidate struct {
	Repository       string       `json:"repository"`
	RepositoryID     int64        `json:"repository_id"`
	RepositoryNodeID string       `json:"repository_node_id"`
	RepositoryHTML   string       `json:"repository_html_url"`
	RepositoryClone  string       `json:"repository_clone_url"`
	RepositoryAPI    string       `json:"repository_api_url"`
	Visibility       string       `json:"visibility"`
	Owner            string       `json:"owner"`
	OwnerID          int64        `json:"owner_id"`
	OwnerNodeID      string       `json:"owner_node_id"`
	Public           bool         `json:"public"`
	Archived         bool         `json:"archived"`
	Disabled         bool         `json:"disabled"`
	Release          *Release     `json:"release,omitempty"`
	TagRefName       string       `json:"tag_ref_name,omitempty"`
	TagRefType       string       `json:"tag_ref_type,omitempty"`
	TagRefSHA        string       `json:"tag_ref_sha,omitempty"`
	TagObjects       []TagObject  `json:"tag_objects,omitempty"`
	Commit           string       `json:"commit"`
	CommitNodeID     string       `json:"commit_node_id"`
	Tree             string       `json:"tree"`
	Parents          []string     `json:"parents"`
	CommitVerify     Verification `json:"commit_verification"`
	ArchiveSHA256    string       `json:"archive_sha256,omitempty"`
}

// Source is the acquisition boundary. WithSnapshot must accept an empty,
// caller-owned directory, expose inert files only during visit, and leave the
// supplied directory empty on every return path.
type Source interface {
	Releases(context.Context, SourceConfig) ([]Release, error)
	ResolveRelease(context.Context, SourceConfig, Release) (Candidate, error)
	ResolveCommit(context.Context, SourceConfig, string) (Candidate, error)
	WithSnapshot(context.Context, Candidate, string, func(string) error) error
}

type FileEvidence struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
}

type ResourceEvidence struct {
	Binding
	SHA256 string         `json:"sha256"`
	Files  []FileEvidence `json:"files"`
}

type Lock struct {
	SchemaVersion      int                `json:"schema_version"`
	Generator          string             `json:"generator"`
	GeneratorVersion   string             `json:"generator_version"`
	Provider           string             `json:"provider"`
	ProviderAPIVersion string             `json:"provider_api_version"`
	SourceID           string             `json:"source_id"`
	Repository         string             `json:"repository"`
	RepositoryID       int64              `json:"repository_id"`
	Owner              string             `json:"owner"`
	OwnerID            int64              `json:"owner_id"`
	Selector           Selector           `json:"selector"`
	Candidate          Candidate          `json:"candidate"`
	Snapshot           string             `json:"snapshot_sha256"`
	Resources          []ResourceEvidence `json:"resources"`
}

type Change struct {
	Kind       string `json:"kind"`
	PackID     string `json:"pack_id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	Path       string `json:"path,omitempty"`
	Before     string `json:"before,omitempty"`
	After      string `json:"after,omitempty"`
}

type Counts struct {
	Resources   int `json:"resources"`
	Files       int `json:"files"`
	Added       int `json:"added"`
	Removed     int `json:"removed"`
	Moved       int `json:"moved"`
	Modified    int `json:"modified"`
	Discoveries int `json:"discoveries"`
}

type Preconditions struct {
	BaseCommit      string `json:"base_commit,omitempty"`
	ConfigSHA256    string `json:"config_sha256"`
	ManifestsSHA256 string `json:"manifests_sha256"`
	BundleSHA256    string `json:"bundle_sha256"`
	LockSHA256      string `json:"lock_sha256,omitempty"`
}

type Plan struct {
	SchemaVersion  int           `json:"schema_version"`
	PlanID         string        `json:"plan_id"`
	Status         string        `json:"status"`
	Authoritative  bool          `json:"authoritative"`
	SourceID       string        `json:"source_id"`
	Selector       Selector      `json:"selector"`
	Candidate      Candidate     `json:"candidate"`
	Counts         Counts        `json:"counts"`
	Changes        []Change      `json:"changes"`
	Discoveries    []string      `json:"unselected_discoveries"`
	Blockers       []string      `json:"blockers"`
	Preconditions  Preconditions `json:"preconditions"`
	ProposedLock   Lock          `json:"proposed_lock"`
	LegacyEvidence bool          `json:"legacy_root_lock_present"`
}

type CheckRequest struct {
	RepositoryRoot string
	SourceID       string
	Selector       *Selector
	AcquisitionDir string
}

type Engine struct {
	Source   Source
	Validate BundleValidator
	Fault    FaultInjector
}

type BundleValidator interface {
	ValidateBundle(context.Context, string, string) error
}

type BundleValidatorFunc func(context.Context, string, string) error

func (validate BundleValidatorFunc) ValidateBundle(ctx context.Context, repositoryRoot, bundleRoot string) error {
	return validate(ctx, repositoryRoot, bundleRoot)
}

type FaultPoint string

const (
	FaultBeforeSwap        FaultPoint = "before-swap"
	FaultAfterFirstRename  FaultPoint = "after-first-rename"
	FaultAfterSecondRename FaultPoint = "after-second-rename"
	FaultDuringCleanup     FaultPoint = "during-cleanup"
)

type FaultInjector func(FaultPoint) error

type ApplyRequest struct {
	CheckRequest
	Plan Plan
}

type ApplyResult struct {
	Status    string `json:"status"`
	PlanID    string `json:"plan_id,omitempty"`
	Changed   bool   `json:"changed"`
	Recovered bool   `json:"recovered,omitempty"`
}

type RecoverRequest struct {
	RepositoryRoot string
}
