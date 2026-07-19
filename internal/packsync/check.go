package packsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/bundletransaction"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type packManifest struct {
	SchemaVersion int                `json:"schema_version"`
	ID            string             `json:"id"`
	Version       string             `json:"version"`
	Resources     []manifestResource `json:"resources"`
}

type manifestResource struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Source string `json:"source"`
}

func (engine Engine) Check(ctx context.Context, request CheckRequest) (Plan, error) {
	if engine.Source == nil {
		return Plan{}, errors.New("source acquisition is required")
	}
	if request.RepositoryRoot == "" || request.AcquisitionDir == "" {
		return Plan{}, errors.New("repository root and caller-supplied acquisition directory are required")
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return Plan{}, fmt.Errorf("acquisition directory: %w", err)
	}
	initial, err := readCheckInputs(ctx, request, engine.allowBootstrap)
	if err != nil {
		return Plan{}, err
	}
	var releases []Release
	if initial.selector.Mode != SelectorCommit || (initial.lockPresent && initial.lock.Candidate.Release != nil) {
		releases, err = engine.Source.Releases(ctx, initial.source)
		if err != nil {
			return Plan{}, fmt.Errorf("list published releases: %w", err)
		}
	}
	candidate, err := engine.resolveFromReleases(ctx, initial.source, initial.selector, releases)
	if err != nil {
		return Plan{}, err
	}
	var continuityBlockers []string
	if initial.lockPresent {
		continuityBlockers, err = engine.lockedContinuity(ctx, initial.source, initial.lock, releases)
		if err != nil {
			return Plan{}, err
		}
	}
	var plan Plan
	err = engine.Source.WithSnapshot(ctx, candidate, request.AcquisitionDir, func(snapshotRoot string) error {
		guard, err := bundletransaction.Acquire(ctx, request.RepositoryRoot)
		if err != nil {
			return err
		}
		defer guard.Release()
		fresh, err := readCheckInputsUnlocked(request, engine.allowBootstrap)
		if err != nil {
			return err
		}
		if !bytesEqual(initial.configBytes, fresh.configBytes) || !bytesEqual(initial.lockBytes, fresh.lockBytes) || initial.lockPresent != fresh.lockPresent || initial.lockSet.LockSetSHA256 != fresh.lockSet.LockSetSHA256 || !reflect.DeepEqual(initial.source, fresh.source) || initial.selector != fresh.selector {
			return errors.New("local source configuration or provenance lock changed during Check; retry")
		}
		manifests, manifestsHash, err := loadManifests(request.RepositoryRoot)
		if err != nil {
			return err
		}
		bindings, bindingBlockers := deriveDestinations(fresh.source.Resources, manifests)
		bundleHash, err := treeHash(filepath.Join(request.RepositoryRoot, "bundle"))
		if err != nil {
			return fmt.Errorf("hash bundle: %w", err)
		}
		baseCommit, err := repositoryBase(request.RepositoryRoot)
		if err != nil {
			return fmt.Errorf("resolve repository base: %w", err)
		}
		plan = Plan{SchemaVersion: 1, Status: "blocked", Authoritative: fresh.lockPresent, SourceID: fresh.source.ID, Selector: fresh.selector, Candidate: candidate, Blockers: append([]string(nil), bindingBlockers...), Preconditions: Preconditions{BaseCommit: baseCommit, ConfigSHA256: hashBytes(fresh.configBytes), ManifestsSHA256: manifestsHash, BundleSHA256: bundleHash, SourceLockSHA256: fresh.lockSet.Digests[fresh.source.ID], LockSetSHA256: fresh.lockSet.LockSetSHA256}, LegacyEvidence: fileExists(filepath.Join(request.RepositoryRoot, "skills-lock.json"))}
		if fresh.lockPresent {
			plan.PreviousSnapshotSHA256 = fresh.lock.Snapshot
		}
		if !fresh.lockPresent {
			plan.Preconditions.SourceLockSHA256 = ""
			plan.Blockers = append(plan.Blockers, "production provenance lock is absent; this sealed bootstrap plan is non-authoritative")
		} else {
			plan.Blockers = append(plan.Blockers, validateLock(fresh.lock, fresh.source, candidate, fresh.selector)...)
			plan.Blockers = append(plan.Blockers, continuityBlockers...)
		}
		plan.Blockers = append(plan.Blockers, validateCandidate(fresh.source, candidate, fresh.selector)...)
		if err := buildPlan(snapshotRoot, request.RepositoryRoot, fresh.source, bindings, manifests, fresh.lock, fresh.lockPresent, &plan); err != nil {
			return err
		}
		_, proposedDigest, err := CanonicalSourceLock(plan.ProposedLock)
		if err != nil {
			return err
		}
		plan.SourceLockSHA256 = proposedDigest
		plan.LockSetSHA256, err = fresh.lockSet.withTarget(fresh.source.ID, proposedDigest)
		return err
	})
	if err != nil {
		return Plan{}, fmt.Errorf("inspect acquired snapshot: %w", err)
	}
	if !plan.Authoritative && len(plan.Changes) > 0 {
		plan.Blockers = append(plan.Blockers, "bootstrap selected bytes differ from the exact candidate")
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return Plan{}, fmt.Errorf("acquisition did not clean caller-supplied directory: %w", err)
	}
	sortPlan(&plan)
	if len(plan.Blockers) == 0 {
		if len(plan.Changes) == 0 {
			plan.Status = "no-op"
		} else {
			plan.Status = "review-required"
		}
	}
	plan.PlanID, err = seal(plan)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

type checkInputs struct {
	configBytes []byte
	source      SourceConfig
	selector    Selector
	lock        Lock
	lockBytes   []byte
	lockPresent bool
	lockSet     sourceLockSet
}

func readCheckInputs(ctx context.Context, request CheckRequest, allowMissing bool) (checkInputs, error) {
	guard, err := bundletransaction.Acquire(ctx, request.RepositoryRoot)
	if err != nil {
		return checkInputs{}, err
	}
	defer guard.Release()
	return readCheckInputsUnlocked(request, allowMissing)
}

func readCheckInputsUnlocked(request CheckRequest, allowMissing bool) (checkInputs, error) {
	configBytes, err := os.ReadFile(filepath.Join(request.RepositoryRoot, "bundle", "sources.json"))
	if err != nil {
		return checkInputs{}, fmt.Errorf("read source configuration: %w", err)
	}
	config, err := LoadConfig(strings.NewReader(string(configBytes)))
	if err != nil {
		return checkInputs{}, err
	}
	source, err := selectSource(config, request.SourceID)
	if err != nil {
		return checkInputs{}, err
	}
	selector := source.Selector
	if request.Selector != nil {
		selector = *request.Selector
	}
	if err := validateSelector(selector); err != nil {
		return checkInputs{}, err
	}
	lockSet, err := loadSourceLockSetForTarget(filepath.Join(request.RepositoryRoot, "bundle"), config, source.ID, allowMissing)
	if err != nil {
		return checkInputs{}, err
	}
	lock, lockPresent := lockSet.Locks[source.ID]
	var lockBytes []byte
	if lockPresent {
		lockBytes, err = os.ReadFile(sourceLockPath(request.RepositoryRoot, source.ID))
		if err != nil {
			return checkInputs{}, err
		}
	}
	return checkInputs{configBytes: configBytes, source: source, selector: selector, lock: lock, lockBytes: lockBytes, lockPresent: lockPresent, lockSet: lockSet}, nil
}

func sourceLockPath(repositoryRoot, sourceID string) string {
	return filepath.Join(repositoryRoot, "bundle", "sources", sourceID+".lock.json")
}

func bytesEqual(left, right []byte) bool { return string(left) == string(right) }

func (engine Engine) resolve(ctx context.Context, source SourceConfig, selector Selector) (Candidate, error) {
	var releases []Release
	var err error
	if selector.Mode != SelectorCommit {
		releases, err = engine.Source.Releases(ctx, source)
		if err != nil {
			return Candidate{}, fmt.Errorf("list published releases: %w", err)
		}
	}
	return engine.resolveFromReleases(ctx, source, selector, releases)
}

func (engine Engine) resolveFromReleases(ctx context.Context, source SourceConfig, selector Selector, releases []Release) (Candidate, error) {
	switch selector.Mode {
	case SelectorStableRelease:
		var stable []Release
		for _, release := range releases {
			if !release.Draft && !release.Prerelease && !release.PublishedAt.IsZero() {
				stable = append(stable, release)
			}
		}
		if len(stable) == 0 {
			return Candidate{}, errors.New("no published stable release discovered")
		}
		sort.Slice(stable, func(i, j int) bool {
			if stable[i].PublishedAt.Equal(stable[j].PublishedAt) {
				return stable[i].ID > stable[j].ID
			}
			return stable[i].PublishedAt.After(stable[j].PublishedAt)
		})
		return engine.Source.ResolveRelease(ctx, source, stable[0])
	case SelectorPrerelease:
		for _, release := range releases {
			if release.Tag == selector.Ref && release.Prerelease && !release.Draft && !release.PublishedAt.IsZero() {
				return engine.Source.ResolveRelease(ctx, source, release)
			}
		}
		return Candidate{}, fmt.Errorf("exact published prerelease %q was not found", selector.Ref)
	case SelectorCommit:
		candidate, err := engine.Source.ResolveCommit(ctx, source, selector.Ref)
		if err != nil {
			return Candidate{}, err
		}
		if candidate.Commit != selector.Ref {
			return Candidate{}, errors.New("exact commit resolution returned a different SHA")
		}
		return candidate, nil
	default:
		return Candidate{}, fmt.Errorf("floating or unknown selector %q is forbidden", selector.Mode)
	}
}

func (engine Engine) lockedContinuity(ctx context.Context, source SourceConfig, lock Lock, releases []Release) ([]string, error) {
	var observed Candidate
	var err error
	if lock.Candidate.Release != nil {
		var current *Release
		for i := range releases {
			if releases[i].Tag == lock.Candidate.Release.Tag {
				current = &releases[i]
				break
			}
		}
		if current == nil {
			return []string{"currently locked release tag is no longer published by the configured repository"}, nil
		}
		observed, err = engine.Source.ResolveRelease(ctx, source, *current)
	} else {
		observed, err = engine.Source.ResolveCommit(ctx, source, lock.Candidate.Commit)
	}
	if err != nil {
		return nil, fmt.Errorf("re-resolve currently locked provenance: %w", err)
	}
	var blockers []string
	for _, blocker := range validateCandidate(source, observed, lock.Selector) {
		blockers = append(blockers, "currently locked provenance is no longer valid: "+blocker)
	}
	if !reflect.DeepEqual(lock.Candidate, observed) {
		blockers = append(blockers, "currently locked release/tag/commit provenance changed")
	}
	return blockers, nil
}

func buildPlan(snapshotRoot, repositoryRoot string, source SourceConfig, bindings []Binding, manifests map[string]packManifest, oldLock Lock, lockPresent bool, plan *Plan) error {
	oldByKey := mapResources(oldLock.Resources)
	newByKey := map[string]ResourceEvidence{}
	for _, binding := range bindings {
		if binding.VendoredPath == "" {
			continue
		}
		candidateFiles, err := inventory(filepath.Join(snapshotRoot, filepath.FromSlash(binding.UpstreamPath)))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				plan.Blockers = append(plan.Blockers, "selected resource missing: "+bindingKey(binding))
				continue
			}
			return err
		}
		localFiles, err := inventory(filepath.Join(repositoryRoot, filepath.FromSlash(binding.VendoredPath)))
		if err != nil {
			plan.Blockers = append(plan.Blockers, "vendored resource unavailable or unsafe: "+bindingKey(binding)+": "+err.Error())
			continue
		}
		resource := ResourceEvidence{Binding: binding, SHA256: resourceHash(candidateFiles), Files: candidateFiles}
		newByKey[bindingKey(binding)] = resource
		plan.Counts.Resources++
		plan.Counts.Files += len(candidateFiles)
		changes := diffFiles(binding, localFiles, candidateFiles)
		plan.Changes = append(plan.Changes, changes...)
		for _, change := range changes {
			countChange(&plan.Counts, change.Kind)
		}
		if lockPresent {
			locked, ok := oldByKey[bindingKey(binding)]
			if !ok {
				plan.Changes = append(plan.Changes, Change{Kind: "resource-added", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: binding.VendoredPath, After: resource.SHA256})
				plan.Counts.Added++
			} else {
				if locked.UpstreamPath != binding.UpstreamPath && locked.SHA256 == resource.SHA256 {
					plan.Changes = append(plan.Changes, Change{Kind: "resource-moved", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: binding.UpstreamPath, Before: locked.UpstreamPath, After: binding.UpstreamPath})
					plan.Counts.Moved++
				} else if locked.VendoredPath != binding.VendoredPath && locked.SHA256 == resource.SHA256 {
					plan.Changes = append(plan.Changes, Change{Kind: "resource-moved", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: binding.VendoredPath, Before: locked.VendoredPath, After: binding.VendoredPath})
					plan.Counts.Moved++
				}
				if locked.SHA256 != resourceHash(locked.Files) {
					plan.Blockers = append(plan.Blockers, "locked selected-resource hash is invalid: "+bindingKey(binding))
				}
				if resourceHash(localFiles) != locked.SHA256 {
					plan.Blockers = append(plan.Blockers, "local selected-resource drift from authoritative lock: "+bindingKey(binding))
				}
			}
		}
	}
	if lockPresent {
		for key, resource := range oldByKey {
			if _, ok := newByKey[key]; !ok {
				plan.Changes = append(plan.Changes, Change{Kind: "resource-removed", PackID: resource.PackID, ResourceID: resource.ResourceID, Path: resource.VendoredPath, Before: resource.SHA256})
				plan.Counts.Removed++
			}
		}
	}
	resources := make([]ResourceEvidence, 0, len(newByKey))
	for _, resource := range newByKey {
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool { return bindingKey(resources[i].Binding) < bindingKey(resources[j].Binding) })
	plan.ProposedLock = Lock{SchemaVersion: 1, Generator: LockGeneratorName, GeneratorVersion: LockGeneratorVersion, Provider: source.Provider, ProviderAPIVersion: GitHubAPIVersion, SourceID: source.ID, Repository: plan.Candidate.Repository, RepositoryID: plan.Candidate.RepositoryID, Owner: plan.Candidate.Owner, OwnerID: plan.Candidate.OwnerID, Selector: plan.Selector, Candidate: plan.Candidate, Resources: resources}
	plan.ProposedLock.Snapshot = snapshotHash(resources)
	if lockPresent && lockDigest(oldLock) != lockDigest(plan.ProposedLock) {
		plan.Changes = append(plan.Changes, Change{Kind: "provenance-updated", Path: "bundle/sources/" + source.ID + ".lock.json", Before: lockDigest(oldLock), After: lockDigest(plan.ProposedLock)})
	}
	plan.Discoveries = discoverUnselected(snapshotRoot, bindings)
	plan.Counts.Discoveries = len(plan.Discoveries)
	plan.AffectedPacks = derivePackImpacts(plan.Changes, manifests, &plan.Blockers)
	plan.Blockers = append(plan.Blockers, compatibilityBlockers(repositoryRoot, snapshotRoot, source, bindings, manifests)...)
	return nil
}

func derivePackImpacts(changes []Change, manifests map[string]packManifest, blockers *[]string) []PackImpact {
	impacts := map[string]*PackImpact{}
	for _, change := range changes {
		var floor ClassificationLevel
		var semantic bool
		var reason string
		switch change.Kind {
		case "resource-added":
			floor, reason = LevelMinor, "selected resource added"
		case "resource-removed":
			floor, reason = LevelMajor, "selected resource removed"
		case "file-added", "file-removed", "file-modified":
			floor, semantic, reason = LevelNone, true, "upstream-owned content changed"
		default:
			continue
		}
		manifest, ok := manifests[change.PackID]
		if !ok || manifest.Version == "" {
			*blockers = append(*blockers, "affected pack manifest is missing or versionless: "+change.PackID)
			continue
		}
		impact := impacts[change.PackID]
		if impact == nil {
			impact = &PackImpact{PackID: change.PackID, CurrentVersion: manifest.Version, MechanicalFloor: floor}
			impacts[change.PackID] = impact
		}
		if classificationRank(floor) > classificationRank(impact.MechanicalFloor) {
			impact.MechanicalFloor = floor
		}
		impact.SemanticEvidenceRequired = impact.SemanticEvidenceRequired || semantic
		impact.Reasons = append(impact.Reasons, reason)
	}
	result := make([]PackImpact, 0, len(impacts))
	for _, impact := range impacts {
		sort.Strings(impact.Reasons)
		impact.Reasons = unique(impact.Reasons)
		result = append(result, *impact)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PackID < result[j].PackID })
	return result
}

func classificationRank(level ClassificationLevel) int {
	return map[ClassificationLevel]int{LevelNone: 0, LevelPatch: 1, LevelMinor: 2, LevelMajor: 3}[level]
}

func validateCandidate(source SourceConfig, candidate Candidate, selector Selector) []string {
	var blockers []string
	owner := strings.Split(source.Repository, "/")[0]
	if !strings.EqualFold(candidate.Repository, source.Repository) || !strings.EqualFold(candidate.Owner, owner) || candidate.RepositoryID == 0 || candidate.OwnerID == 0 || candidate.RepositoryNodeID == "" || candidate.OwnerNodeID == "" {
		blockers = append(blockers, "repository or owner identity does not match the configured source")
	}
	if candidate.RepositoryHTML == "" || candidate.RepositoryClone == "" || candidate.RepositoryAPI == "" || candidate.Visibility == "" {
		blockers = append(blockers, "repository provenance evidence is incomplete")
	}
	if !candidate.Public || candidate.Archived || candidate.Disabled {
		blockers = append(blockers, "configured repository is not an active public source")
	}
	if !fullSHA(candidate.Commit) || candidate.CommitNodeID == "" || !fullSHA(candidate.Tree) {
		blockers = append(blockers, "candidate did not resolve to a complete immutable commit and tree")
	}
	for _, parent := range candidate.Parents {
		if !fullSHA(parent) {
			blockers = append(blockers, "candidate contains an invalid parent commit SHA")
			break
		}
	}
	if selector.Mode != SelectorCommit && !continuousTagChain(candidate) {
		blockers = append(blockers, "release tag-to-commit provenance is incomplete or ambiguous")
	}
	switch selector.Mode {
	case SelectorStableRelease:
		if candidate.Release == nil || candidate.Release.Prerelease {
			blockers = append(blockers, "automatic selection did not retain a published stable release")
		}
	case SelectorPrerelease:
		if candidate.Release == nil || !candidate.Release.Prerelease || candidate.Release.Tag != selector.Ref {
			blockers = append(blockers, "manual prerelease did not retain the exact published prerelease")
		}
	case SelectorCommit:
		if candidate.Release != nil || candidate.Commit != selector.Ref {
			blockers = append(blockers, "manual commit provenance is not the exact requested SHA")
		}
	}
	if selector.Mode == SelectorStableRelease && !eligibleAutomaticEvidence(candidate) {
		blockers = append(blockers, "stable release lacks eligible verification evidence")
	}
	if selector.Mode != SelectorStableRelease && invalidVerification(candidate) {
		blockers = append(blockers, "manual candidate carries invalid verification evidence")
	}
	if !completeVerification(candidate.CommitVerify) {
		blockers = append(blockers, "commit verification evidence is incomplete")
	}
	for _, tag := range candidate.TagObjects {
		if !completeVerification(tag.Verification) {
			blockers = append(blockers, "tag verification evidence is incomplete")
			break
		}
	}
	return blockers
}

func validateLock(lock Lock, source SourceConfig, candidate Candidate, selector Selector) []string {
	var blockers []string
	if lock.SchemaVersion != 1 || lock.SourceID != source.ID {
		blockers = append(blockers, "production lock schema or source identity is invalid")
	}
	if lock.Generator != LockGeneratorName || lock.GeneratorVersion != LockGeneratorVersion || lock.Provider != source.Provider || lock.ProviderAPIVersion != GitHubAPIVersion {
		blockers = append(blockers, "production lock generator or provider evidence is invalid")
	}
	if lock.Repository != source.Repository || lock.RepositoryID != candidate.RepositoryID || lock.OwnerID != candidate.OwnerID || !strings.EqualFold(lock.Owner, candidate.Owner) {
		blockers = append(blockers, "repository or owner numeric identity moved from the authoritative lock")
	}
	if lock.Candidate.Repository != lock.Repository || lock.Candidate.RepositoryID != lock.RepositoryID || !strings.EqualFold(lock.Candidate.Owner, lock.Owner) || lock.Candidate.OwnerID != lock.OwnerID {
		blockers = append(blockers, "production lock candidate identity disagrees with its retained repository evidence")
	}
	if err := validateSelector(lock.Selector); err != nil {
		blockers = append(blockers, "production lock selector is invalid: "+err.Error())
	}
	for _, blocker := range validateCandidate(source, lock.Candidate, lock.Selector) {
		blockers = append(blockers, "production lock retained provenance is invalid: "+blocker)
	}
	if lock.Candidate.Release != nil && candidate.Release != nil && lock.Candidate.Release.Tag == candidate.Release.Tag && lock.Candidate.TagRefSHA != "" && lock.Candidate.TagRefSHA != candidate.TagRefSHA {
		blockers = append(blockers, "release tag ref moved for the locked candidate")
	}
	if lock.Candidate.Release != nil && candidate.Release != nil && lock.Candidate.Release.Tag == candidate.Release.Tag && !reflect.DeepEqual(lock.Candidate.Release, candidate.Release) {
		blockers = append(blockers, "release identity or publication evidence changed for the locked tag")
	}
	if lock.Snapshot != snapshotHash(lock.Resources) {
		blockers = append(blockers, "production lock snapshot hash is invalid")
	}
	blockers = append(blockers, validateLockedResources(lock.Resources)...)
	if selector.Mode == SelectorCommit && candidate.Commit != selector.Ref {
		blockers = append(blockers, "manual commit does not equal the requested full SHA")
	}
	return blockers
}

func continuousTagChain(candidate Candidate) bool {
	if candidate.Release == nil || candidate.TagRefSHA == "" {
		return false
	}
	if !fullSHA(candidate.TagRefSHA) || candidate.TagRefName != "refs/tags/"+candidate.Release.Tag || candidate.Release.ID <= 0 || candidate.Release.NodeID == "" || candidate.Release.Tag == "" || candidate.Release.Target == "" || candidate.Release.CreatedAt.IsZero() || candidate.Release.PublishedAt.IsZero() || candidate.Release.Draft || !validActor(candidate.Release.Author) {
		return false
	}
	if len(candidate.TagObjects) == 0 {
		return candidate.TagRefType == "commit" && candidate.TagRefSHA == candidate.Commit
	}
	if candidate.TagRefType != "tag" || candidate.TagRefSHA != candidate.TagObjects[0].SHA {
		return false
	}
	for i, tag := range candidate.TagObjects {
		if !fullSHA(tag.SHA) || tag.Name == "" || (i == 0 && tag.Name != candidate.Release.Tag) || !fullSHA(tag.TargetSHA) {
			return false
		}
		want := candidate.Commit
		wantType := "commit"
		if i+1 < len(candidate.TagObjects) {
			want = candidate.TagObjects[i+1].SHA
			wantType = "tag"
		}
		if tag.TargetSHA != want || tag.TargetType != wantType {
			return false
		}
	}
	return true
}

func eligibleAutomaticEvidence(candidate Candidate) bool {
	eligible := validVerification(candidate.CommitVerify)
	if !completeVerification(candidate.CommitVerify) {
		return false
	}
	for _, tag := range candidate.TagObjects {
		if validVerification(tag.Verification) {
			eligible = true
		}
		if !completeVerification(tag.Verification) {
			return false
		}
	}
	return eligible
}

func invalidVerification(candidate Candidate) bool {
	if !completeVerification(candidate.CommitVerify) {
		return true
	}
	for _, tag := range candidate.TagObjects {
		if !completeVerification(tag.Verification) {
			return true
		}
	}
	return false
}

func completeVerification(value Verification) bool {
	return validVerification(value) || unsignedVerification(value)
}

func validVerification(value Verification) bool {
	return value.Verified && value.Reason == "valid" && value.VerifiedAt != nil && value.SignatureSHA256 != nil && value.PayloadSHA256 != nil && fullDigest(*value.SignatureSHA256) && fullDigest(*value.PayloadSHA256)
}

func unsignedVerification(value Verification) bool {
	return !value.Verified && value.Reason == "unsigned" && value.VerifiedAt == nil && value.SignatureSHA256 == nil && value.PayloadSHA256 == nil
}

func validActor(value Actor) bool {
	return value.Login != "" && value.ID > 0 && value.NodeID != ""
}

func validateLockedResources(resources []ResourceEvidence) []string {
	var blockers []string
	seenResources := map[string]bool{}
	for _, resource := range resources {
		key := bindingKey(resource.Binding)
		if resource.PackID == "" || resource.Kind == "" || resource.ResourceID == "" || seenResources[key] {
			blockers = append(blockers, "production lock has an incomplete or duplicate selected resource: "+key)
			continue
		}
		seenResources[key] = true
		if !safeSlashPath(resource.UpstreamPath) || !safeSlashPath(resource.VendoredPath) || !strings.HasPrefix(resource.VendoredPath, "bundle/") {
			blockers = append(blockers, "production lock has unsafe selected-resource paths: "+key)
		}
		if len(resource.Files) == 0 || resource.SHA256 != resourceHash(resource.Files) {
			blockers = append(blockers, "production lock has invalid selected-resource bytes: "+key)
		}
		seenFiles := map[string]bool{}
		for _, file := range resource.Files {
			if (file.Path == fileResourceRootPath && len(resource.Files) != 1) || (file.Path != fileResourceRootPath && !safeSlashPath(file.Path)) || seenFiles[file.Path] || file.Size < 0 || !fullDigest(file.SHA256) || file.Mode&0o600 != 0o600 || file.Mode&0o022 != 0 || file.Mode&^uint32(0o777) != 0 {
				blockers = append(blockers, "production lock has unsafe or invalid file evidence: "+key+"/"+file.Path)
				break
			}
			seenFiles[file.Path] = true
		}
	}
	return blockers
}

func fullSHA(value string) bool {
	return len(value) == 40 && strings.Trim(value, "0123456789abcdef") == ""
}

func fullDigest(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func lockDigest(lock Lock) string {
	data, _ := json.Marshal(lock)
	return hashBytes(data)
}

func diffFiles(binding Binding, local, candidate []FileEvidence) []Change {
	localMap, candidateMap := mapFiles(local), mapFiles(candidate)
	var changes []Change
	for name, before := range localMap {
		after, ok := candidateMap[name]
		path := filepath.ToSlash(filepath.Join(binding.VendoredPath, name))
		switch {
		case !ok:
			changes = append(changes, Change{Kind: "file-removed", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: path, Before: before.SHA256})
		case before.SHA256 != after.SHA256 || before.Mode != after.Mode:
			changes = append(changes, Change{Kind: "file-modified", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: path, Before: before.SHA256, After: after.SHA256})
		}
	}
	for name, after := range candidateMap {
		if _, ok := localMap[name]; !ok {
			changes = append(changes, Change{Kind: "file-added", PackID: binding.PackID, ResourceID: binding.ResourceID, Path: filepath.ToSlash(filepath.Join(binding.VendoredPath, name)), After: after.SHA256})
		}
	}
	return changes
}

func loadManifests(root string) (map[string]packManifest, string, error) {
	paths, err := filepath.Glob(filepath.Join(root, "bundle", "packs", "*", "pack.json"))
	if err != nil {
		return nil, "", err
	}
	sort.Strings(paths)
	result := map[string]packManifest{}
	var framed []byte
	for _, name := range paths {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, "", err
		}
		var manifest packManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, "", fmt.Errorf("decode runtime manifest %s: %w", name, err)
		}
		if (manifest.SchemaVersion != 1 && manifest.SchemaVersion != 2) || manifest.ID == "" || result[manifest.ID].ID != "" {
			return nil, "", fmt.Errorf("invalid or duplicate runtime manifest %s", name)
		}
		if manifest.SchemaVersion == 2 {
			portable, err := capabilitypack.LoadPortableManifest(name, filepath.Join(root, "bundle"))
			if err != nil {
				return nil, "", fmt.Errorf("runtime manifest %s disagrees with capability-pack contract: %w", name, err)
			}
			if portable.ID != manifest.ID || portable.Version != manifest.Version {
				return nil, "", fmt.Errorf("runtime manifest %s identity disagrees with capability-pack contract", name)
			}
		}
		result[manifest.ID] = manifest
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return nil, "", err
		}
		framed = append(framed, []byte(filepath.ToSlash(relative)+"\x00"+hashBytes(data)+"\n")...)
	}
	return result, hashBytes(framed), nil
}

func deriveDestinations(bindings []Binding, manifests map[string]packManifest) ([]Binding, []string) {
	result := append([]Binding(nil), bindings...)
	var blockers []string
	for i := range result {
		manifest, ok := manifests[result[i].PackID]
		if !ok {
			blockers = append(blockers, "binding references unknown runtime pack: "+result[i].PackID)
			continue
		}
		found := false
		for _, resource := range manifest.Resources {
			if resource.Kind == result[i].Kind && resource.ID == result[i].ResourceID {
				if !safeSlashPath(resource.Source) {
					blockers = append(blockers, "runtime manifest has unsafe vendored source: "+bindingKey(result[i]))
					break
				}
				result[i].VendoredPath = filepath.ToSlash(filepath.Join("bundle", resource.Source))
				found = true
				break
			}
		}
		if !found {
			blockers = append(blockers, "binding is absent from authoritative runtime manifest: "+bindingKey(result[i]))
		}
	}
	return result, blockers
}

func discoverUnselected(root string, bindings []Binding) []string {
	selected := map[string]bool{}
	for _, binding := range bindings {
		selected[binding.UpstreamPath] = true
	}
	var discoveries []string
	categories, _ := os.ReadDir(filepath.Join(root, "skills"))
	for _, category := range categories {
		if !category.IsDir() || category.Type()&os.ModeSymlink != 0 {
			continue
		}
		resources, _ := os.ReadDir(filepath.Join(root, "skills", category.Name()))
		for _, resource := range resources {
			relative := filepath.ToSlash(filepath.Join("skills", category.Name(), resource.Name()))
			if resource.IsDir() && resource.Type()&os.ModeSymlink == 0 && !selected[relative] {
				discoveries = append(discoveries, relative)
			}
		}
	}
	sort.Strings(discoveries)
	return discoveries
}

func readLock(name string) (Lock, []byte, bool, error) {
	data, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return Lock{}, nil, false, nil
	}
	if err != nil {
		return Lock{}, nil, false, err
	}
	var lock Lock
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lock); err != nil {
		return Lock{}, nil, false, fmt.Errorf("decode production lock: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Lock{}, nil, false, err
	}
	return lock, data, true, nil
}

func selectSource(config Config, id string) (SourceConfig, error) {
	if id == "" && len(config.Sources) == 1 {
		return config.Sources[0], nil
	}
	for _, source := range config.Sources {
		if source.ID == id {
			return source, nil
		}
	}
	return SourceConfig{}, fmt.Errorf("configured source %q was not found", id)
}

func requireEmptyDirectory(name string) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("must be a real directory")
	}
	entries, err := os.ReadDir(name)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("must be empty")
	}
	return nil
}

func sortPlan(plan *Plan) {
	sort.Slice(plan.Changes, func(i, j int) bool {
		a, b := plan.Changes[i], plan.Changes[j]
		return a.Kind+a.PackID+a.ResourceID+a.Path+a.Before+a.After < b.Kind+b.PackID+b.ResourceID+b.Path+b.Before+b.After
	})
	sort.Strings(plan.Discoveries)
	sort.Slice(plan.AffectedPacks, func(i, j int) bool { return plan.AffectedPacks[i].PackID < plan.AffectedPacks[j].PackID })
	for i := range plan.AffectedPacks {
		sort.Strings(plan.AffectedPacks[i].Reasons)
		plan.AffectedPacks[i].Reasons = unique(plan.AffectedPacks[i].Reasons)
	}
	sort.Strings(plan.Blockers)
	plan.Blockers = unique(plan.Blockers)
}

func mapResources(resources []ResourceEvidence) map[string]ResourceEvidence {
	result := map[string]ResourceEvidence{}
	for _, resource := range resources {
		result[bindingKey(resource.Binding)] = resource
	}
	return result
}

func countChange(counts *Counts, kind string) {
	switch kind {
	case "file-added":
		counts.Added++
	case "file-removed":
		counts.Removed++
	case "file-modified":
		counts.Modified++
	}
}

func unique(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}
