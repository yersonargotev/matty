package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type contract struct {
	SchemaVersion int      `json:"schema_version"`
	Sources       []source `json:"sources"`
}

type source struct {
	ID         string      `json:"id"`
	Provider   string      `json:"provider"`
	Repository any         `json:"repository"`
	Selector   selector    `json:"selector"`
	Selection  selection   `json:"selection"`
	Release    release     `json:"release"`
	Git        gitEvidence `json:"git"`
	Snapshot   string      `json:"snapshot_sha256"`
	Resources  []resource  `json:"resources"`
}

type selector struct {
	Mode string `json:"mode"`
}

type selection struct {
	Mode string `json:"mode"`
}

type repositoryEvidence struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type release struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

type gitEvidence struct {
	TagRef     tagRef      `json:"tag_ref"`
	TagObjects []tagObject `json:"tag_objects"`
	Commit     commit      `json:"commit"`
}

type tagRef struct {
	ObjectSHA string `json:"object_sha"`
}

type tagObject struct {
	SHA          string       `json:"sha"`
	TargetSHA    string       `json:"target_sha"`
	Verification verification `json:"verification"`
}

type commit struct {
	SHA          string       `json:"sha"`
	Verification verification `json:"verification"`
}

type verification struct {
	Verified bool   `json:"verified"`
	Reason   string `json:"reason"`
}

type resource struct {
	PackID       string `json:"pack_id"`
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id"`
	UpstreamPath string `json:"upstream_path"`
	VendoredPath string `json:"vendored_path"`
	ResourceHash string `json:"resource_sha256"`
	Files        []file `json:"files"`
}

type file struct {
	Path   string `json:"path"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type observed struct {
	RepositoryID int64
	TagObjectSHA string
	Files        map[string]map[string]file
}

type result struct {
	ConfigSources int
	Bindings      int
	LockedFiles   int
	ByteIdentical int
	Drifted       int
	Missing       int
	Unexpected    int
	Failures      []string
}

func evaluate(config, lock contract, actual observed) result {
	r := result{ConfigSources: len(config.Sources)}
	if config.SchemaVersion != 1 || lock.SchemaVersion != 1 {
		r.Failures = append(r.Failures, "unsupported schema version")
	}
	if len(config.Sources) != len(lock.Sources) || len(config.Sources) != 1 {
		r.Failures = append(r.Failures, "prototype expects one matching configured and locked source")
		return r
	}
	cfg, locked := config.Sources[0], lock.Sources[0]
	r.Bindings = len(cfg.Resources)
	repo, ok := locked.Repository.(map[string]any)
	if !ok || repo["full_name"] != cfg.Repository {
		r.Failures = append(r.Failures, "configured repository does not match locked canonical repository")
	}
	lockedRepoID, _ := repo["id"].(float64)
	if int64(lockedRepoID) != actual.RepositoryID {
		r.Failures = append(r.Failures, "repository numeric identity changed")
	}
	if locked.Git.TagRef.ObjectSHA != actual.TagObjectSHA {
		r.Failures = append(r.Failures, "locked tag moved")
	}
	if !continuousChain(locked.Git) {
		r.Failures = append(r.Failures, "locked tag-to-commit chain is discontinuous")
	}
	if cfg.Selector.Mode != "stable-release" || locked.Selection.Mode != cfg.Selector.Mode {
		r.Failures = append(r.Failures, "selection mode changed")
	}
	if locked.Release.Draft || locked.Release.Prerelease || locked.Release.TagName == "" {
		r.Failures = append(r.Failures, "locked release is not a published stable release")
	}
	if !eligibleEvidence(locked.Git) {
		r.Failures = append(r.Failures, "signature evidence is not eligible")
	}
	if got := snapshotHash(locked.Resources); got != locked.Snapshot {
		r.Failures = append(r.Failures, "source snapshot aggregate does not match its resources")
	}

	configured := map[string]resource{}
	for _, binding := range cfg.Resources {
		configured[bindingKey(binding)] = binding
	}
	for _, want := range locked.Resources {
		r.LockedFiles += len(want.Files)
		binding, exists := configured[bindingKey(want)]
		if !exists || binding.UpstreamPath != want.UpstreamPath {
			r.Failures = append(r.Failures, "lock contains a resource outside the explicit selection: "+bindingKey(want))
			continue
		}
		if resourceHash(want.Files) != want.ResourceHash {
			r.Failures = append(r.Failures, "resource aggregate is invalid: "+bindingKey(want))
		}
		gotFiles := actual.Files[bindingKey(want)]
		wantFiles := map[string]file{}
		for _, f := range want.Files {
			wantFiles[f.Path] = f
			got, exists := gotFiles[f.Path]
			switch {
			case !exists:
				r.Missing++
			case got.Size != f.Size || got.SHA256 != f.SHA256:
				r.Drifted++
			default:
				r.ByteIdentical++
			}
		}
		for path := range gotFiles {
			if _, exists := wantFiles[path]; !exists {
				r.Unexpected++
			}
		}
	}
	if len(configured) != len(locked.Resources) {
		r.Failures = append(r.Failures, "configured and locked resource sets differ")
	}
	if r.Drifted+r.Missing+r.Unexpected > 0 {
		r.Failures = append(r.Failures, "vendored bytes do not match the atomic locked snapshot")
	}
	return r
}

func eligibleEvidence(g gitEvidence) bool {
	valid := g.Commit.Verification.Verified && g.Commit.Verification.Reason == "valid"
	if !g.Commit.Verification.Verified && g.Commit.Verification.Reason != "unsigned" {
		return false
	}
	for _, t := range g.TagObjects {
		if t.Verification.Verified && t.Verification.Reason == "valid" {
			valid = true
		}
		if !t.Verification.Verified && t.Verification.Reason != "unsigned" {
			return false
		}
	}
	return valid
}

func continuousChain(g gitEvidence) bool {
	if len(g.TagObjects) == 0 || g.TagRef.ObjectSHA != g.TagObjects[0].SHA {
		return false
	}
	for i, tag := range g.TagObjects {
		want := g.Commit.SHA
		if i+1 < len(g.TagObjects) {
			want = g.TagObjects[i+1].SHA
		}
		if tag.TargetSHA != want {
			return false
		}
	}
	return true
}

func bindingKey(r resource) string {
	return r.PackID + "/" + r.Kind + "/" + r.ResourceID
}

func resourceHash(files []file) string {
	h := sha256.New()
	ordered := append([]file(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	for _, f := range ordered {
		fmt.Fprintf(h, "%s\x00%d\x00%s\n", f.Path, f.Size, f.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func snapshotHash(resources []resource) string {
	h := sha256.New()
	for _, r := range resources {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\n", r.PackID, r.Kind, r.ResourceID, r.UpstreamPath, r.VendoredPath, r.ResourceHash)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func status(r result) string {
	if len(r.Failures) == 0 {
		return "ELIGIBLE FOR HUMAN REVIEW"
	}
	return "BLOCKED"
}

func failureText(r result) string {
	if len(r.Failures) == 0 {
		return "none"
	}
	return strings.Join(r.Failures, "\n  - ")
}
