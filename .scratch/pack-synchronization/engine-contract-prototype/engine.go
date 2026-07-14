package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	statusBlocked = "blocked"
	statusNoop    = "no-op"
	statusReview  = "review-required"
	statusReady   = "ready"
)

type Engine struct {
	Gateway   SourceGateway
	Failpoint string // prototype-only: before-commit or after-backup
}

func (e Engine) Check(ctx context.Context, req CheckRequest) (Plan, error) {
	if e.Gateway == nil {
		return Plan{}, errors.New("source gateway is required")
	}
	if req.Historical != "contract-snapshot" && req.Historical != "immutable-artifact" {
		return Plan{}, fmt.Errorf("unknown historical contract %q", req.Historical)
	}
	candidate, err := e.resolve(ctx, req)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{SchemaVersion: 1, SourceID: req.Source.ID, Selector: req.Selector, Candidate: candidate, Historical: req.Historical, files: map[string][]byte{}}
	plan.Preconditions.BundleSHA256, err = treeHash(filepath.Join(req.RepositoryRoot, "bundle"))
	if err != nil {
		return Plan{}, err
	}
	lockPath := filepath.Join(req.RepositoryRoot, "bundle", "sources.lock.json")
	oldLock, lockBytes, err := readLock(lockPath)
	if err != nil {
		return Plan{}, err
	}
	plan.Preconditions.LockSHA256 = bytesHash(lockBytes)
	plan.PreviousLock = oldLock
	plan.Blockers = append(plan.Blockers, validateCandidate(req.Source, oldLock, candidate, req.Selector)...)

	checkout, err := os.MkdirTemp(req.TempRoot, "clean-checkout-")
	if err != nil {
		return Plan{}, err
	}
	defer os.RemoveAll(checkout)
	if entries, _ := os.ReadDir(checkout); len(entries) != 0 {
		return Plan{}, errors.New("acquisition directory was not clean")
	}
	if err := e.Gateway.Acquire(ctx, candidate, checkout); err != nil {
		return Plan{}, fmt.Errorf("acquire immutable candidate: %w", err)
	}

	manifests, err := loadManifests(req.RepositoryRoot)
	if err != nil {
		return Plan{}, err
	}
	bindings, bindBlockers := deriveDestinations(req.Source.Bindings, manifests)
	plan.Blockers = append(plan.Blockers, bindBlockers...)
	oldByKey := mapResources(oldLock.Resources)
	newByKey := map[string]LockedResource{}
	impacts := map[string]*PackImpact{}

	for _, binding := range bindings {
		key := bindingKey(binding)
		candidateRoot := filepath.Join(checkout, filepath.FromSlash(binding.UpstreamPath))
		files, data, err := inventory(candidateRoot)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				plan.Blockers = append(plan.Blockers, "selected resource missing: "+key+" at "+binding.UpstreamPath)
				continue
			}
			return Plan{}, err
		}
		resource := LockedResource{Binding: binding, Files: files, SHA256: resourceHash(files)}
		newByKey[key] = resource
		for rel, content := range data {
			plan.files[filepath.ToSlash(filepath.Join(binding.VendoredPath, rel))] = content
		}
		old, existed := oldByKey[key]
		if existed {
			if failures := verifyVendored(req.RepositoryRoot, old); len(failures) > 0 {
				plan.Blockers = append(plan.Blockers, failures...)
			}
		}
		switch {
		case !existed:
			plan.Changes = append(plan.Changes, Change{Kind: "resource-added", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: binding.VendoredPath, After: resource.SHA256})
			addImpact(impacts, manifests[binding.PackID].Version, binding.PackID, "minor", false, "compatible optional resource added")
		case old.UpstreamPath != binding.UpstreamPath && old.SHA256 == resource.SHA256:
			plan.Changes = append(plan.Changes, Change{Kind: "upstream-path-moved", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: binding.UpstreamPath, Before: old.UpstreamPath, After: binding.UpstreamPath})
		case old.SHA256 != resource.SHA256:
			plan.Changes = append(plan.Changes, diffResource(binding, old, resource)...)
			addImpact(impacts, manifests[binding.PackID].Version, binding.PackID, "none", true, "upstream-owned content changed")
		}
	}
	for key, old := range oldByKey {
		if _, exists := newByKey[key]; exists {
			continue
		}
		plan.Changes = append(plan.Changes, Change{Kind: "resource-removed", PackID: old.PackID, ResourceID: old.ResourceID, Path: old.VendoredPath, Before: old.SHA256})
		version := "unknown"
		if manifest, ok := manifests[old.PackID]; ok {
			version = manifest.Version
		}
		addImpact(impacts, version, old.PackID, "major", false, "selected resource removed")
	}

	plan.GeneratedLock = buildLock(req.Source, req.Selector, candidate, newByKey)
	if oldLock.Commit != candidate.Commit || oldLock.Snapshot != plan.GeneratedLock.Snapshot {
		plan.Changes = append(plan.Changes, Change{Kind: "lock-replaced", Path: "bundle/sources.lock.json", Before: oldLock.Snapshot, After: plan.GeneratedLock.Snapshot})
	}
	plan.Notices = append(plan.Notices, discoverUnselected(checkout, bindings)...)
	for _, impact := range impacts {
		plan.AffectedPacks = append(plan.AffectedPacks, *impact)
	}
	sortPlan(&plan)
	switch {
	case len(plan.Blockers) > 0:
		plan.Status = statusBlocked
	case len(plan.Changes) == 0:
		plan.Status = statusNoop
	case len(plan.AffectedPacks) > 0:
		plan.Status = statusReview
	default:
		plan.Status = statusReady
	}
	plan.ID = sealPlan(plan)
	return plan, nil
}

func (e Engine) resolve(ctx context.Context, req CheckRequest) (Candidate, error) {
	switch req.Selector.Mode {
	case "stable-release":
		if req.Selector.Ref != "" {
			return Candidate{}, errors.New("stable-release does not accept a ref")
		}
		releases, err := e.Gateway.Releases(ctx, req.Source)
		if err != nil {
			return Candidate{}, err
		}
		var stable []Candidate
		for _, release := range releases {
			if release.Release != "" && !release.Draft && !release.Prerelease {
				stable = append(stable, release)
			}
		}
		if len(stable) == 0 {
			return Candidate{}, errors.New("no published stable release discovered")
		}
		sort.Slice(stable, func(i, j int) bool {
			if stable[i].PublishedAt == stable[j].PublishedAt {
				return stable[i].ReleaseID > stable[j].ReleaseID
			}
			return stable[i].PublishedAt > stable[j].PublishedAt
		})
		return stable[0], nil
	case "prerelease", "commit":
		if req.Selector.Ref == "" {
			return Candidate{}, fmt.Errorf("%s selection requires an explicit ref", req.Selector.Mode)
		}
		if req.Selector.Mode == "commit" && (len(req.Selector.Ref) != 40 || strings.Trim(req.Selector.Ref, "0123456789abcdef") != "") {
			return Candidate{}, errors.New("commit selection requires a full lowercase 40-character SHA")
		}
		candidate, err := e.Gateway.ResolveExplicit(ctx, req.Source, req.Selector)
		if err != nil {
			return Candidate{}, err
		}
		if req.Selector.Mode == "commit" && candidate.Commit != req.Selector.Ref {
			return Candidate{}, errors.New("explicit commit did not resolve to the requested SHA")
		}
		if req.Selector.Mode == "prerelease" && (candidate.Release != req.Selector.Ref || !candidate.Prerelease || candidate.Draft) {
			return Candidate{}, errors.New("explicit prerelease did not resolve to the requested published prerelease")
		}
		return candidate, nil
	default:
		return Candidate{}, fmt.Errorf("floating or unknown selector %q is forbidden", req.Selector.Mode)
	}
}

func validateCandidate(source SourceConfig, old Lock, c Candidate, selector Selector) []string {
	var failures []string
	if c.Repository != source.Repository || c.RepositoryID != source.RepositoryID || (old.RepositoryID != 0 && c.RepositoryID != old.RepositoryID) {
		failures = append(failures, "repository identity or canonical locator changed")
	}
	if !c.Public || c.Archived || c.Disabled {
		failures = append(failures, "repository is not an eligible active public source")
	}
	if c.TagMoved {
		failures = append(failures, "tag provenance moved since the locked observation")
	}
	if c.Commit == "" {
		failures = append(failures, "candidate did not resolve to an immutable commit")
	}
	if selector.Mode == "stable-release" {
		if c.Release == "" || c.Prerelease {
			failures = append(failures, "automatic selection did not resolve a stable release")
		}
		if !c.Verification.Verified || c.Verification.Reason != "valid" {
			failures = append(failures, "stable release lacks eligible verified tag-or-commit evidence")
		}
	} else if !c.Verification.Verified && c.Verification.Reason != "unsigned" {
		failures = append(failures, "explicit candidate carries present-but-invalid verification evidence")
	}
	return failures
}

func (e Engine) Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error) {
	if req.Plan.Status == statusBlocked {
		return ApplyResult{}, errors.New("blocked plans cannot be applied")
	}
	if req.Plan.Status == statusNoop {
		return ApplyResult{Status: statusNoop, PlanID: req.Plan.ID}, nil
	}
	if err := validateClassifications(req.Plan, req.Classifications); err != nil {
		return ApplyResult{}, err
	}
	bundle := filepath.Join(req.RepositoryRoot, "bundle")
	got, err := treeHash(bundle)
	if err != nil {
		return ApplyResult{}, err
	}
	if got != req.Plan.Preconditions.BundleSHA256 {
		return ApplyResult{}, errors.New("stale plan: bundle changed after check")
	}
	if req.Plan.ID != sealPlan(req.Plan) {
		return ApplyResult{}, errors.New("plan seal is invalid")
	}
	plan, err := e.reacquirePlanBytes(ctx, req)
	if err != nil {
		return ApplyResult{}, err
	}
	transaction, err := os.MkdirTemp(filepath.Dir(bundle), ".sync-transaction-")
	if err != nil {
		return ApplyResult{}, err
	}
	defer os.RemoveAll(transaction)
	staged := filepath.Join(transaction, "bundle")
	if err := copyTree(bundle, staged); err != nil {
		return ApplyResult{}, err
	}
	if err := materialize(staged, plan, req.Classifications); err != nil {
		return ApplyResult{}, err
	}
	if e.Failpoint == "before-commit" {
		return ApplyResult{}, errors.New("injected failure before commit")
	}
	if err := verifyStaged(staged, plan); err != nil {
		return ApplyResult{}, err
	}
	backup := filepath.Join(transaction, "bundle.backup")
	marker := filepath.Join(req.RepositoryRoot, ".sync-transaction.json")
	markerBytes, _ := json.Marshal(map[string]string{"bundle": bundle, "backup": backup, "staged": staged})
	if err := os.WriteFile(marker, markerBytes, 0o600); err != nil {
		return ApplyResult{}, err
	}
	defer os.Remove(marker)
	if err := os.Rename(bundle, backup); err != nil {
		return ApplyResult{}, err
	}
	if e.Failpoint == "after-backup" {
		_ = os.Rename(backup, bundle)
		return ApplyResult{}, errors.New("injected failure after backup; original restored")
	}
	if err := os.Rename(staged, bundle); err != nil {
		_ = os.Rename(backup, bundle)
		return ApplyResult{}, fmt.Errorf("commit staged bundle: %w", err)
	}
	written := []string{"bundle/sources.lock.json"}
	for _, change := range req.Plan.Changes {
		if change.Kind != "lock-replaced" {
			written = append(written, change.Path)
		}
	}
	sort.Strings(written)
	return ApplyResult{Status: "applied", PlanID: req.Plan.ID, Changed: true, Written: unique(written)}, nil
}

func (e Engine) reacquirePlanBytes(ctx context.Context, req ApplyRequest) (Plan, error) {
	checkout, err := os.MkdirTemp(req.TempRoot, "apply-checkout-")
	if err != nil {
		return Plan{}, err
	}
	defer os.RemoveAll(checkout)
	if err := e.Gateway.Acquire(ctx, req.Plan.Candidate, checkout); err != nil {
		return Plan{}, fmt.Errorf("reacquire sealed candidate: %w", err)
	}
	plan := req.Plan
	plan.files = map[string][]byte{}
	for _, resource := range plan.GeneratedLock.Resources {
		files, data, err := inventory(filepath.Join(checkout, filepath.FromSlash(resource.UpstreamPath)))
		if err != nil || resourceHash(files) != resource.SHA256 {
			return Plan{}, fmt.Errorf("reacquired bytes do not match sealed plan for %s", bindingKey(resource.Binding))
		}
		for path, content := range data {
			plan.files[filepath.ToSlash(filepath.Join(resource.VendoredPath, path))] = content
		}
	}
	return plan, nil
}

func Recover(repositoryRoot string) (ApplyResult, error) {
	marker := filepath.Join(repositoryRoot, ".sync-transaction.json")
	data, err := os.ReadFile(marker)
	if errors.Is(err, fs.ErrNotExist) {
		return ApplyResult{Status: "clean"}, nil
	}
	if err != nil {
		return ApplyResult{}, err
	}
	var tx map[string]string
	if err := json.Unmarshal(data, &tx); err != nil {
		return ApplyResult{}, err
	}
	if _, err := os.Stat(tx["bundle"]); errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(tx["backup"], tx["bundle"]); err != nil {
			return ApplyResult{}, fmt.Errorf("restore interrupted transaction: %w", err)
		}
	}
	_ = os.RemoveAll(filepath.Dir(tx["backup"]))
	_ = os.Remove(marker)
	return ApplyResult{Status: "recovered", Recovered: true}, nil
}

func validateClassifications(plan Plan, got map[string]Classification) error {
	for _, impact := range plan.AffectedPacks {
		c, ok := got[impact.PackID]
		if !ok || c.Rationale == "" || c.ClassifierID == "" || (c.ClassifierType != "human" && c.ClassifierType != "ai-agent") {
			return fmt.Errorf("pack %s requires complete reviewable compatibility evidence", impact.PackID)
		}
		if rank(c.Level) < rank(impact.MechanicalFloor) {
			return fmt.Errorf("pack %s classification %s is below mechanical floor %s", impact.PackID, c.Level, impact.MechanicalFloor)
		}
		want, err := bump(impact.CurrentVersion, c.Level)
		if err != nil || want != c.ProposedVersion {
			return fmt.Errorf("pack %s proposed version must be exact next %s bump (%s)", impact.PackID, c.Level, want)
		}
		if c.Level == "major" && c.Migration == "" {
			return fmt.Errorf("pack %s major classification requires migration evidence", impact.PackID)
		}
	}
	return nil
}

func materialize(staged string, plan Plan, classifications map[string]Classification) error {
	if err := retainHistorical(staged, plan); err != nil {
		return err
	}
	for _, change := range plan.Changes {
		if change.Kind == "resource-removed" {
			if err := os.RemoveAll(filepath.Join(filepath.Dir(staged), filepath.FromSlash(change.Path))); err != nil {
				return err
			}
		}
	}
	for path, content := range plan.files {
		rel := strings.TrimPrefix(path, "bundle/")
		target := filepath.Join(staged, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
	}
	lock, _ := json.MarshalIndent(plan.GeneratedLock, "", "  ")
	if err := os.WriteFile(filepath.Join(staged, "sources.lock.json"), append(lock, '\n'), 0o644); err != nil {
		return err
	}
	for _, impact := range plan.AffectedPacks {
		manifestPath := filepath.Join(staged, "packs", impact.PackID, "pack.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return err
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		raw["version"] = classifications[impact.PackID].ProposedVersion
		data, _ = json.MarshalIndent(raw, "", "  ")
		if err := os.WriteFile(manifestPath, append(data, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func retainHistorical(staged string, plan Plan) error {
	for _, impact := range plan.AffectedPacks {
		manifest := filepath.Join(staged, "packs", impact.PackID, "pack.json")
		data, err := os.ReadFile(manifest)
		if err != nil {
			return err
		}
		if plan.Historical == "contract-snapshot" {
			history := filepath.Join(staged, ".history", "contracts")
			if err := os.MkdirAll(history, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(history, impact.PackID+"-"+impact.CurrentVersion+".json"), data, 0o644); err != nil {
				return err
			}
			continue
		}
		history := filepath.Join(staged, ".history", "packs", impact.PackID, impact.CurrentVersion)
		if err := os.MkdirAll(history, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(history, "pack.json"), data, 0o644); err != nil {
			return err
		}
		for _, resource := range plan.PreviousLock.Resources {
			if resource.PackID != impact.PackID {
				continue
			}
			source := filepath.Join(filepath.Dir(staged), filepath.FromSlash(resource.VendoredPath))
			destination := filepath.Join(history, "resources", resource.Kind, resource.ResourceID)
			if err := copyTree(source, destination); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyStaged(staged string, plan Plan) error {
	for _, r := range plan.GeneratedLock.Resources {
		root := filepath.Join(filepath.Dir(staged), filepath.FromSlash(r.VendoredPath))
		files, _, err := inventory(root)
		if err != nil || resourceHash(files) != r.SHA256 {
			return fmt.Errorf("staged bytes failed verification for %s", bindingKey(r.Binding))
		}
	}
	return nil
}

func VerifyHistorical(repositoryRoot, packID, version, mode string) error {
	if mode == "contract-snapshot" {
		path := filepath.Join(repositoryRoot, "bundle", ".history", "contracts", packID+"-"+version+".json")
		if _, err := os.Stat(path); err != nil {
			return err
		}
		return errors.New("contract snapshot describes the old pack but cannot reproduce its old resource bytes")
	}
	root := filepath.Join(repositoryRoot, "bundle", ".history", "packs", packID, version)
	if _, err := os.Stat(filepath.Join(root, "pack.json")); err != nil {
		return err
	}
	entries, err := filepath.Glob(filepath.Join(root, "resources", "*", "*"))
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return errors.New("historical artifact has no retained resource bytes")
	}
	return nil
}

func readLock(path string) (Lock, []byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Lock{}, nil, nil
	}
	if err != nil {
		return Lock{}, nil, err
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return Lock{}, nil, err
	}
	return lock, data, nil
}

func loadManifests(root string) (map[string]PackManifest, error) {
	paths, err := filepath.Glob(filepath.Join(root, "bundle", "packs", "*", "pack.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]PackManifest{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var manifest PackManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		out[manifest.ID] = manifest
	}
	return out, nil
}

func deriveDestinations(bindings []Binding, manifests map[string]PackManifest) ([]Binding, []string) {
	out := append([]Binding(nil), bindings...)
	var blockers []string
	for i := range out {
		manifest, ok := manifests[out[i].PackID]
		if !ok {
			blockers = append(blockers, "binding references unknown pack: "+out[i].PackID)
			continue
		}
		found := false
		for _, resource := range manifest.Resources {
			if resource.Kind == out[i].Kind && resource.ID == out[i].ResourceID {
				out[i].VendoredPath = filepath.ToSlash(filepath.Join("bundle", resource.Source))
				found = true
				break
			}
		}
		if !found {
			blockers = append(blockers, "binding is not present in runtime manifest: "+bindingKey(out[i]))
		}
	}
	return out, blockers
}

func inventory(root string) ([]FileEvidence, map[string][]byte, error) {
	var files []FileEvidence
	data := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		files = append(files, FileEvidence{Path: rel, Size: int64(len(content)), SHA256: bytesHash(content)})
		data[rel] = content
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, data, err
}

func verifyVendored(root string, locked LockedResource) []string {
	files, _, err := inventory(filepath.Join(root, filepath.FromSlash(locked.VendoredPath)))
	if err != nil {
		return []string{"locked vendored resource is unavailable: " + bindingKey(locked.Binding)}
	}
	if resourceHash(files) != locked.SHA256 {
		return []string{"local vendored bytes drifted from the lock: " + bindingKey(locked.Binding)}
	}
	return nil
}

func diffResource(binding Binding, old, next LockedResource) []Change {
	oldFiles, newFiles := mapFiles(old.Files), mapFiles(next.Files)
	var changes []Change
	for path, before := range oldFiles {
		after, ok := newFiles[path]
		switch {
		case !ok:
			changes = append(changes, Change{Kind: "file-removed", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: filepath.ToSlash(filepath.Join(binding.VendoredPath, path)), Before: before.SHA256})
		case before.SHA256 != after.SHA256:
			changes = append(changes, Change{Kind: "file-modified", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: filepath.ToSlash(filepath.Join(binding.VendoredPath, path)), Before: before.SHA256, After: after.SHA256})
		}
	}
	for path, after := range newFiles {
		if _, ok := oldFiles[path]; !ok {
			changes = append(changes, Change{Kind: "file-added", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: filepath.ToSlash(filepath.Join(binding.VendoredPath, path)), After: after.SHA256})
		}
	}
	return changes
}

func buildLock(source SourceConfig, selector Selector, c Candidate, resources map[string]LockedResource) Lock {
	lock := Lock{SchemaVersion: 1, SourceID: source.ID, Repository: c.Repository, RepositoryID: c.RepositoryID, Selection: selector, Release: c.Release, TagObject: c.TagObject, Commit: c.Commit}
	for _, resource := range resources {
		lock.Resources = append(lock.Resources, resource)
	}
	sort.Slice(lock.Resources, func(i, j int) bool {
		return bindingKey(lock.Resources[i].Binding) < bindingKey(lock.Resources[j].Binding)
	})
	lock.Snapshot = snapshotHash(lock.Resources)
	return lock
}

func discoverUnselected(checkout string, bindings []Binding) []string {
	selected := map[string]bool{}
	for _, b := range bindings {
		selected[filepath.ToSlash(b.UpstreamPath)] = true
	}
	var notices []string
	entries, _ := filepath.Glob(filepath.Join(checkout, "skills", "*"))
	for _, entry := range entries {
		rel, _ := filepath.Rel(checkout, entry)
		rel = filepath.ToSlash(rel)
		if info, err := os.Stat(entry); err == nil && info.IsDir() && !selected[rel] {
			notices = append(notices, "unselected upstream resource discovered: "+rel)
		}
	}
	sort.Strings(notices)
	return notices
}

func addImpact(m map[string]*PackImpact, version, pack, floor string, semantic bool, reason string) {
	impact := m[pack]
	if impact == nil {
		impact = &PackImpact{PackID: pack, CurrentVersion: version, MechanicalFloor: floor}
		m[pack] = impact
	}
	if rank(floor) > rank(impact.MechanicalFloor) {
		impact.MechanicalFloor = floor
	}
	impact.SemanticEvidence = impact.SemanticEvidence || semantic
	impact.Reasons = append(impact.Reasons, reason)
}

func sortPlan(plan *Plan) {
	sort.Slice(plan.Changes, func(i, j int) bool {
		a, b := plan.Changes[i], plan.Changes[j]
		return a.PackID+a.ResourceID+a.Path+a.Kind < b.PackID+b.ResourceID+b.Path+b.Kind
	})
	sort.Slice(plan.AffectedPacks, func(i, j int) bool { return plan.AffectedPacks[i].PackID < plan.AffectedPacks[j].PackID })
	for i := range plan.AffectedPacks {
		sort.Strings(plan.AffectedPacks[i].Reasons)
	}
	sort.Strings(plan.Notices)
	sort.Strings(plan.Blockers)
}

func sealPlan(plan Plan) string {
	copy := plan
	copy.ID = ""
	copy.files = nil
	data, _ := json.Marshal(copy)
	return "sync-" + bytesHash(data)[:16]
}

func bindingKey(b Binding) string { return b.PackID + "/" + b.Kind + "/" + b.ResourceID }
func mapResources(in []LockedResource) map[string]LockedResource {
	out := map[string]LockedResource{}
	for _, r := range in {
		out[bindingKey(r.Binding)] = r
	}
	return out
}
func mapFiles(in []FileEvidence) map[string]FileEvidence {
	out := map[string]FileEvidence{}
	for _, f := range in {
		out[f.Path] = f
	}
	return out
}
func bytesHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func resourceHash(files []FileEvidence) string {
	h := sha256.New()
	ordered := append([]FileEvidence(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	for _, f := range ordered {
		fmt.Fprintf(h, "%s\x00%d\x00%s\n", f.Path, f.Size, f.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func snapshotHash(resources []LockedResource) string {
	h := sha256.New()
	for _, r := range resources {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", bindingKey(r.Binding), r.UpstreamPath, r.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func treeHash(root string) (string, error) {
	files, _, err := inventory(root)
	if err != nil {
		return "", err
	}
	return resourceHash(files), nil
}
func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(source, path)
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
func rank(level string) int {
	return map[string]int{"none": 0, "patch": 1, "minor": 2, "major": 3}[level]
}
func bump(version, level string) (string, error) {
	var major, minor, patch int
	if _, err := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		return "", err
	}
	switch level {
	case "patch":
		patch++
	case "minor":
		minor++
		patch = 0
	case "major":
		major++
		minor = 0
		patch = 0
	default:
		return "", fmt.Errorf("invalid level %q", level)
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}
func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
