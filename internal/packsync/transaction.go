package packsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/yersonargotev/matty/internal/bundletransaction"
)

var ErrRecoveryEvidence = errors.New("bundle recovery evidence is absent or invalid")

const recoveryMarkerSchema = 1

type recoveryMarker struct {
	SchemaVersion int    `json:"schema_version"`
	PlanID        string `json:"plan_id"`
	Phase         string `json:"phase"`
	Bundle        string `json:"bundle"`
	Backup        string `json:"backup"`
	Staged        string `json:"staged"`
	OldSHA256     string `json:"old_sha256"`
	NewSHA256     string `json:"new_sha256"`
	Legacy        string `json:"legacy,omitempty"`
	LegacySHA256  string `json:"legacy_sha256,omitempty"`
	Seal          string `json:"seal"`
}

func (engine Engine) Apply(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	if engine.Source == nil || engine.Validate == nil {
		return ApplyResult{}, errors.New("Apply requires source acquisition and Matty-owned bundle validation")
	}
	if request.RepositoryRoot == "" || request.AcquisitionDir == "" || request.Plan.PlanID == "" {
		return ApplyResult{}, errors.New("Apply requires repository root, acquisition directory, and exact sealed plan")
	}
	if !request.Plan.VerifySeal() {
		return ApplyResult{}, errors.New("sealed plan identity is invalid")
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return ApplyResult{}, fmt.Errorf("acquisition directory: %w", err)
	}
	candidate, err := engine.reacquireCandidate(ctx, request.Plan)
	if err != nil {
		return ApplyResult{}, err
	}
	if !reflect.DeepEqual(candidate, request.Plan.Candidate) {
		return ApplyResult{}, errors.New("exact candidate provenance changed after Check")
	}
	var result ApplyResult
	err = engine.Source.WithSnapshot(ctx, candidate, request.AcquisitionDir, func(snapshotRoot string) error {
		if err := verifySnapshot(snapshotRoot, request.Plan.ProposedLock); err != nil {
			return fmt.Errorf("reacquired candidate does not match sealed plan: %w", err)
		}
		guard, err := bundletransaction.Acquire(ctx, request.RepositoryRoot)
		if err != nil {
			return err
		}
		defer guard.Release()
		result, err = engine.applyLocked(ctx, request, candidate, snapshotRoot)
		return err
	})
	if err != nil {
		return ApplyResult{}, err
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return ApplyResult{}, fmt.Errorf("acquisition did not clean caller-supplied directory: %w", err)
	}
	return result, nil
}

func (engine Engine) reacquireCandidate(ctx context.Context, plan Plan) (Candidate, error) {
	config := SourceConfig{ID: plan.SourceID, Repository: plan.Candidate.Repository}
	if plan.Candidate.Release == nil {
		candidate, err := engine.Source.ResolveCommit(ctx, config, plan.Candidate.Commit)
		if err != nil {
			return Candidate{}, fmt.Errorf("re-resolve exact commit: %w", err)
		}
		return candidate, nil
	}
	releases, err := engine.Source.Releases(ctx, config)
	if err != nil {
		return Candidate{}, fmt.Errorf("re-list exact release: %w", err)
	}
	for _, release := range releases {
		if release.Tag == plan.Candidate.Release.Tag {
			candidate, err := engine.Source.ResolveRelease(ctx, config, release)
			if err != nil {
				return Candidate{}, fmt.Errorf("re-resolve exact release: %w", err)
			}
			return candidate, nil
		}
	}
	return Candidate{}, errors.New("sealed release is no longer published")
}

func (engine Engine) applyLocked(ctx context.Context, request ApplyRequest, candidate Candidate, snapshotRoot string) (ApplyResult, error) {
	plan := request.Plan
	if !plan.VerifySeal() || !reflect.DeepEqual(plan.Candidate, candidate) {
		return ApplyResult{}, errors.New("sealed plan identity changed while acquiring the transaction lock")
	}
	if err := verifySnapshot(snapshotRoot, plan.ProposedLock); err != nil {
		return ApplyResult{}, fmt.Errorf("exact candidate changed while acquiring the transaction lock: %w", err)
	}
	bundle := filepath.Join(request.RepositoryRoot, "bundle")
	legacy := filepath.Join(request.RepositoryRoot, "skills-lock.json")
	if converged, err := convergedBootstrap(bundle, legacy, plan); err != nil {
		return ApplyResult{}, err
	} else if converged {
		if err := engine.validateStaged(ctx, request.RepositoryRoot, bundle, snapshotRoot, plan); err != nil {
			return ApplyResult{}, fmt.Errorf("validate converged bundle: %w", err)
		}
		return ApplyResult{Status: "no-op", PlanID: plan.PlanID}, nil
	}
	if !applicablePlan(plan) {
		return ApplyResult{}, fmt.Errorf("plan status %q is not applicable", plan.Status)
	}
	if err := validatePreconditions(request.RepositoryRoot, plan.Preconditions); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.Validate.ValidateBundle(ctx, request.RepositoryRoot, bundle); err != nil {
		return ApplyResult{}, fmt.Errorf("fresh Matty-owned validation: %w", err)
	}
	staged, backup := transactionPaths(request.RepositoryRoot, plan.PlanID)
	markerPath := recoveryMarkerPath(request.RepositoryRoot)
	for _, path := range []string{staged, backup, markerPath} {
		if _, err := os.Lstat(path); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return ApplyResult{}, fmt.Errorf("%w: unexpected transaction path %s", ErrRecoveryEvidence, path)
		}
	}
	if err := copyTreeExact(bundle, staged); err != nil {
		return ApplyResult{}, fmt.Errorf("stage complete bundle: %w", err)
	}
	cleanupStaged := true
	defer func() {
		if cleanupStaged {
			_ = os.RemoveAll(staged)
		}
	}()
	currentLock, _, currentLockPresent, err := readLock(filepath.Join(bundle, "sources.lock.json"))
	if err != nil {
		return ApplyResult{}, err
	}
	if err := materializeSelectedResources(staged, snapshotRoot, currentLock, currentLockPresent, plan.ProposedLock); err != nil {
		return ApplyResult{}, err
	}
	if err := writeCanonicalLock(filepath.Join(staged, "sources.lock.json"), plan.ProposedLock); err != nil {
		return ApplyResult{}, err
	}
	oldHash, err := treeHash(bundle)
	if err != nil {
		return ApplyResult{}, err
	}
	newHash, err := treeHash(staged)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := engine.validateStaged(ctx, request.RepositoryRoot, staged, snapshotRoot, plan); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.inject(FaultBeforeSwap); err != nil {
		return ApplyResult{}, err
	}
	marker := recoveryMarker{SchemaVersion: recoveryMarkerSchema, PlanID: plan.PlanID, Phase: "prepared", Bundle: bundle, Backup: backup, Staged: staged, OldSHA256: oldHash, NewSHA256: newHash}
	if data, err := os.ReadFile(legacy); err == nil {
		marker.Legacy, marker.LegacySHA256 = legacy, hashBytes(data)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ApplyResult{}, err
	}
	if err := writeRecoveryMarker(markerPath, &marker); err != nil {
		return ApplyResult{}, err
	}
	if err := os.Rename(bundle, backup); err != nil {
		_ = os.Remove(markerPath)
		return ApplyResult{}, fmt.Errorf("first bundle rename: %w", err)
	}
	if err := syncDirectory(request.RepositoryRoot); err != nil {
		return ApplyResult{}, err
	}
	cleanupStaged = false
	marker.Phase = "old-renamed"
	if err := writeRecoveryMarker(markerPath, &marker); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.inject(FaultAfterFirstRename); err != nil {
		return ApplyResult{}, err
	}
	if err := os.Rename(staged, bundle); err != nil {
		return ApplyResult{}, fmt.Errorf("second bundle rename: %w", err)
	}
	if err := syncDirectory(request.RepositoryRoot); err != nil {
		return ApplyResult{}, err
	}
	marker.Phase = "new-installed"
	if err := writeRecoveryMarker(markerPath, &marker); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.inject(FaultAfterSecondRename); err != nil {
		return ApplyResult{}, err
	}
	if err := verifyTreeHash(bundle, newHash); err != nil {
		return ApplyResult{}, err
	}
	marker.Phase = "cleanup"
	if err := writeRecoveryMarker(markerPath, &marker); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.inject(FaultDuringCleanup); err != nil {
		return ApplyResult{}, err
	}
	if err := cleanupCommitted(marker); err != nil {
		return ApplyResult{}, err
	}
	if err := os.Remove(markerPath); err != nil {
		return ApplyResult{}, fmt.Errorf("remove recovery marker: %w", err)
	}
	return ApplyResult{Status: "applied", PlanID: plan.PlanID, Changed: true}, nil
}

func (engine Engine) validateStaged(ctx context.Context, repositoryRoot, staged, snapshotRoot string, plan Plan) error {
	view, err := os.MkdirTemp(repositoryRoot, ".matty-bundle-validation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(view)
	viewBundle := filepath.Join(view, "bundle")
	if err := copyTreeExact(staged, viewBundle); err != nil {
		return err
	}
	configBytes, err := os.ReadFile(filepath.Join(viewBundle, "sources.json"))
	if err != nil || hashBytes(configBytes) != plan.Preconditions.ConfigSHA256 {
		return errors.New("staged source configuration changed from the sealed plan")
	}
	config, err := LoadConfig(bytes.NewReader(configBytes))
	if err != nil {
		return err
	}
	source, err := selectSource(config, plan.SourceID)
	if err != nil {
		return err
	}
	manifests, _, err := loadManifests(view)
	if err != nil {
		return err
	}
	bindings, blockers := deriveDestinations(source.Resources, manifests)
	lock, _, present, err := readLock(filepath.Join(viewBundle, "sources.lock.json"))
	if err != nil || !present || lockDigest(lock) != lockDigest(plan.ProposedLock) {
		return errors.New("staged production provenance lock does not match the sealed plan")
	}
	stagedPlan := Plan{Candidate: plan.Candidate, Selector: plan.Selector}
	if err := buildPlan(snapshotRoot, view, source, bindings, manifests, lock, true, &stagedPlan); err != nil {
		return err
	}
	blockers = append(blockers, stagedPlan.Blockers...)
	if len(blockers) != 0 || len(stagedPlan.Changes) != 0 || lockDigest(stagedPlan.ProposedLock) != lockDigest(plan.ProposedLock) {
		return fmt.Errorf("staged bundle validation blocked: %s", strings.Join(blockers, "; "))
	}
	if err := engine.Validate.ValidateBundle(ctx, view, viewBundle); err != nil {
		return fmt.Errorf("validate staged bundle with Matty-owned suite: %w", err)
	}
	return nil
}

func (engine Engine) Recover(ctx context.Context, request RecoverRequest) (ApplyResult, error) {
	if engine.Validate == nil || request.RepositoryRoot == "" {
		return ApplyResult{}, errors.New("Recover requires repository root and Matty-owned bundle validation")
	}
	guard, err := bundletransaction.Acquire(ctx, request.RepositoryRoot)
	if err != nil {
		return ApplyResult{}, err
	}
	defer guard.Release()
	markerPath := recoveryMarkerPath(request.RepositoryRoot)
	marker, err := readRecoveryMarker(markerPath)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := validateRecoveryPaths(request.RepositoryRoot, marker); err != nil {
		return ApplyResult{}, err
	}
	bundleHash, bundlePresent, err := observedTreeHash(marker.Bundle)
	if err != nil {
		return ApplyResult{}, err
	}
	backupHash, backupPresent, err := observedTreeHash(marker.Backup)
	if err != nil {
		return ApplyResult{}, err
	}
	stagedHash, stagedPresent, err := observedTreeHash(marker.Staged)
	if err != nil {
		return ApplyResult{}, err
	}
	rollbackPending := !bundlePresent && backupPresent && backupHash == marker.OldSHA256 && stagedPresent && stagedHash == marker.NewSHA256 && (marker.Phase == "prepared" || marker.Phase == "old-renamed" || marker.Phase == "rolling-back")
	prepared := bundlePresent && bundleHash == marker.OldSHA256 && !backupPresent && stagedPresent && stagedHash == marker.NewSHA256 && marker.Phase == "prepared"
	rollbackInstalled := bundlePresent && bundleHash == marker.OldSHA256 && !backupPresent && (!stagedPresent || stagedHash == marker.NewSHA256) && (marker.Phase == "rolling-back" || marker.Phase == "rolled-back")
	completed := bundlePresent && bundleHash == marker.NewSHA256 && backupPresent && backupHash == marker.OldSHA256 && !stagedPresent && (marker.Phase == "prepared" || marker.Phase == "old-renamed" || marker.Phase == "new-installed" || marker.Phase == "cleanup")
	cleaned := bundlePresent && bundleHash == marker.NewSHA256 && !backupPresent && !stagedPresent && (marker.Phase == "new-installed" || marker.Phase == "cleanup") && legacyClean(marker)
	switch {
	case prepared:
		marker.Phase = "rolled-back"
		if err := writeRecoveryMarker(markerPath, &marker); err != nil {
			return ApplyResult{}, err
		}
		return engine.finishRollback(ctx, request.RepositoryRoot, markerPath, marker)
	case rollbackPending:
		marker.Phase = "rolling-back"
		if err := writeRecoveryMarker(markerPath, &marker); err != nil {
			return ApplyResult{}, err
		}
		if err := os.Rename(marker.Backup, marker.Bundle); err != nil {
			return ApplyResult{}, fmt.Errorf("restore old bundle: %w", err)
		}
		if err := syncDirectory(request.RepositoryRoot); err != nil {
			return ApplyResult{}, err
		}
		marker.Phase = "rolled-back"
		if err := writeRecoveryMarker(markerPath, &marker); err != nil {
			return ApplyResult{}, err
		}
		return engine.finishRollback(ctx, request.RepositoryRoot, markerPath, marker)
	case rollbackInstalled:
		return engine.finishRollback(ctx, request.RepositoryRoot, markerPath, marker)
	case completed:
		if err := engine.Validate.ValidateBundle(ctx, request.RepositoryRoot, marker.Bundle); err != nil {
			return ApplyResult{}, err
		}
		marker.Phase = "cleanup"
		if err := writeRecoveryMarker(markerPath, &marker); err != nil {
			return ApplyResult{}, err
		}
		if err := cleanupCommitted(marker); err != nil {
			return ApplyResult{}, err
		}
		if err := os.Remove(markerPath); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Status: "completed", PlanID: marker.PlanID, Changed: true, Recovered: true}, nil
	case cleaned:
		if err := engine.Validate.ValidateBundle(ctx, request.RepositoryRoot, marker.Bundle); err != nil {
			return ApplyResult{}, err
		}
		if err := os.Remove(markerPath); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Status: "completed", PlanID: marker.PlanID, Changed: true, Recovered: true}, nil
	default:
		return ApplyResult{}, fmt.Errorf("%w: phase %q is incompatible with observed old/new hashes", ErrRecoveryEvidence, marker.Phase)
	}
}

func (engine Engine) finishRollback(ctx context.Context, repositoryRoot, markerPath string, marker recoveryMarker) (ApplyResult, error) {
	if err := verifyTreeHash(marker.Bundle, marker.OldSHA256); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.Validate.ValidateBundle(ctx, repositoryRoot, marker.Bundle); err != nil {
		return ApplyResult{}, err
	}
	if marker.Phase != "rolled-back" {
		marker.Phase = "rolled-back"
		if err := writeRecoveryMarker(markerPath, &marker); err != nil {
			return ApplyResult{}, err
		}
	}
	if err := os.RemoveAll(marker.Staged); err != nil {
		return ApplyResult{}, err
	}
	if err := os.Remove(markerPath); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Status: "rolled-back", PlanID: marker.PlanID, Recovered: true}, nil
}

func applicablePlan(plan Plan) bool {
	if plan.Status == "review-required" && plan.Authoritative && len(plan.Blockers) == 0 {
		return true
	}
	return plan.Status == "blocked" && !plan.Authoritative && len(plan.Changes) == 0 && len(plan.Blockers) == 1 && strings.Contains(plan.Blockers[0], "production provenance lock is absent")
}

func materializeSelectedResources(staged, snapshotRoot string, current Lock, currentPresent bool, next Lock) error {
	nextByKey := mapResources(next.Resources)
	if currentPresent {
		for _, resource := range current.Resources {
			if _, retained := nextByKey[bindingKey(resource.Binding)]; retained {
				continue
			}
			target, err := stagedResourcePath(staged, resource.VendoredPath)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(target); err != nil {
				return err
			}
		}
	}
	for _, resource := range next.Resources {
		target, err := stagedResourcePath(staged, resource.VendoredPath)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		source := filepath.Join(snapshotRoot, filepath.FromSlash(resource.UpstreamPath))
		if err := copyTreeExact(source, target); err != nil {
			return fmt.Errorf("materialize selected resource %s: %w", bindingKey(resource.Binding), err)
		}
	}
	return nil
}

func stagedResourcePath(staged, vendoredPath string) (string, error) {
	prefix := "bundle/"
	if !strings.HasPrefix(vendoredPath, prefix) || !safeSlashPath(vendoredPath) {
		return "", fmt.Errorf("unsafe staged resource path %q", vendoredPath)
	}
	return filepath.Join(staged, filepath.FromSlash(strings.TrimPrefix(vendoredPath, prefix))), nil
}

func convergedBootstrap(bundle, legacy string, plan Plan) (bool, error) {
	if _, err := os.Stat(legacy); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	lock, _, present, err := readLock(filepath.Join(bundle, "sources.lock.json"))
	if err != nil || !present || lockDigest(lock) != lockDigest(plan.ProposedLock) {
		return false, err
	}
	for _, resource := range lock.Resources {
		files, err := inventory(filepath.Join(filepath.Dir(bundle), filepath.FromSlash(resource.VendoredPath)))
		if err != nil || resourceHash(files) != resource.SHA256 {
			return false, err
		}
	}
	return true, nil
}

func validatePreconditions(repositoryRoot string, expected Preconditions) error {
	if base, err := repositoryBase(repositoryRoot); err != nil || base != expected.BaseCommit {
		return fmt.Errorf("stale plan: repository base changed after Check")
	}
	config, err := os.ReadFile(filepath.Join(repositoryRoot, "bundle", "sources.json"))
	if err != nil || hashBytes(config) != expected.ConfigSHA256 {
		return errors.New("stale plan: source configuration changed after Check")
	}
	_, manifests, err := loadManifests(repositoryRoot)
	if err != nil || manifests != expected.ManifestsSHA256 {
		return errors.New("stale plan: runtime manifests changed after Check")
	}
	_, lockBytes, present, err := readLock(filepath.Join(repositoryRoot, "bundle", "sources.lock.json"))
	if err != nil {
		return err
	}
	lockHash := ""
	if present {
		lockHash = hashBytes(lockBytes)
	}
	if lockHash != expected.LockSHA256 {
		return errors.New("stale plan: production provenance lock changed after Check")
	}
	bundle, err := treeHash(filepath.Join(repositoryRoot, "bundle"))
	if err != nil || bundle != expected.BundleSHA256 {
		return errors.New("stale plan: bundle, history, or compatibility evidence changed after Check")
	}
	return nil
}

func repositoryBase(repositoryRoot string) (string, error) {
	repository, err := git.PlainOpen(repositoryRoot)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	head, err := repository.Head()
	if err != nil {
		return "", err
	}
	return head.Hash().String(), nil
}

func verifySnapshot(snapshotRoot string, lock Lock) error {
	for _, resource := range lock.Resources {
		files, err := inventory(filepath.Join(snapshotRoot, filepath.FromSlash(resource.UpstreamPath)))
		if err != nil || resourceHash(files) != resource.SHA256 {
			return fmt.Errorf("selected resource %s changed", bindingKey(resource.Binding))
		}
	}
	return nil
}

func writeCanonicalLock(path string, lock Lock) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func transactionPaths(repositoryRoot, planID string) (string, string) {
	suffix := strings.TrimPrefix(planID, "pack-sync-")
	return filepath.Join(repositoryRoot, ".matty-bundle-"+suffix+".staged"), filepath.Join(repositoryRoot, ".matty-bundle-"+suffix+".backup")
}

func recoveryMarkerPath(repositoryRoot string) string {
	return filepath.Join(repositoryRoot, ".matty-bundle-recovery.json")
}

func (engine Engine) inject(point FaultPoint) error {
	if engine.Fault == nil {
		return nil
	}
	if err := engine.Fault(point); err != nil {
		return fmt.Errorf("bundle transaction %s: %w", point, err)
	}
	return nil
}

func markerSeal(marker recoveryMarker) string {
	marker.Seal = ""
	data, _ := json.Marshal(marker)
	return hashBytes(data)
}

func writeRecoveryMarker(path string, marker *recoveryMarker) error {
	marker.Seal = markerSeal(*marker)
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".matty-bundle-marker-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func readRecoveryMarker(path string) (recoveryMarker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return recoveryMarker{}, fmt.Errorf("%w: read marker: %v", ErrRecoveryEvidence, err)
	}
	var marker recoveryMarker
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil {
		return recoveryMarker{}, fmt.Errorf("%w: decode marker: %v", ErrRecoveryEvidence, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return recoveryMarker{}, fmt.Errorf("%w: marker has trailing data", ErrRecoveryEvidence)
	}
	if marker.SchemaVersion != recoveryMarkerSchema || marker.PlanID == "" || marker.Seal == "" || marker.Seal != markerSeal(marker) || !fullDigest(marker.OldSHA256) || !fullDigest(marker.NewSHA256) {
		return recoveryMarker{}, fmt.Errorf("%w: marker identity or seal is invalid", ErrRecoveryEvidence)
	}
	return marker, nil
}

func validateRecoveryPaths(repositoryRoot string, marker recoveryMarker) error {
	bundle := filepath.Join(repositoryRoot, "bundle")
	staged, backup := transactionPaths(repositoryRoot, marker.PlanID)
	if marker.Bundle != bundle || marker.Staged != staged || marker.Backup != backup || (marker.Legacy != "" && marker.Legacy != filepath.Join(repositoryRoot, "skills-lock.json")) {
		return fmt.Errorf("%w: marker paths are not canonical siblings", ErrRecoveryEvidence)
	}
	matches, err := filepath.Glob(filepath.Join(repositoryRoot, ".matty-bundle-*"))
	if err != nil {
		return err
	}
	allowed := map[string]bool{markerPathClean(staged): true, markerPathClean(backup): true}
	for _, match := range matches {
		if filepath.Base(match) == ".matty-bundle-recovery.json" {
			continue
		}
		if !allowed[markerPathClean(match)] {
			return fmt.Errorf("%w: unexpected or ambiguous transaction sibling %s", ErrRecoveryEvidence, match)
		}
	}
	return nil
}

func markerPathClean(path string) string { return filepath.Clean(path) }

func observedTreeHash(path string) (string, bool, error) {
	hash, err := treeHash(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	return hash, err == nil, err
}

func verifyTreeHash(path, expected string) error {
	actual, err := treeHash(path)
	if err != nil {
		return fmt.Errorf("hash bundle tree: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("bundle tree hash is %s, want %s", actual, expected)
	}
	return nil
}

func cleanupCommitted(marker recoveryMarker) error {
	if marker.Legacy != "" {
		data, err := os.ReadFile(marker.Legacy)
		if err == nil && hashBytes(data) != marker.LegacySHA256 {
			return fmt.Errorf("%w: legacy evidence changed during transaction", ErrRecoveryEvidence)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err == nil {
			if err := os.Remove(marker.Legacy); err != nil {
				return err
			}
		}
	}
	return os.RemoveAll(marker.Backup)
}

func legacyClean(marker recoveryMarker) bool {
	if marker.Legacy == "" {
		return true
	}
	_, err := os.Stat(marker.Legacy)
	return errors.Is(err, fs.ErrNotExist)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func copyTreeExact(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe symlink in bundle: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
