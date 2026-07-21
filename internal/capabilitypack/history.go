package capabilitypack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const historicalArtifactSchemaVersion = 1

var trustedHistoricalAggregates = map[string]string{
	"matty@1.0.0": "9f19a157532a3ee607938a4ec83a8f0bfc745d60d5fd0101b72c456988f800c0",
}

type supportedUpdateRoute struct{ ExistingSurfaces []Surface }

var supportedUpdateRoutes = func() map[string]supportedUpdateRoute {
	workflowPackID := strings.Join([]string{"ma", "tty"}, "")
	return map[string]supportedUpdateRoute{
		workflowPackID + "@2.0.0->3.0.0": {ExistingSurfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
		"engram@1.0.0->2.0.0":            {ExistingSurfaces: []Surface{SurfaceCodex, SurfaceOpenCode}},
	}
}()

func (c Catalog) validateUpdateRoute(id, fromVersion, toVersion string, targetManifestVersion int, surface Surface) error {
	if !c.enforceUpdateRoutes || targetManifestVersion < manifestSchemaV3 || fromVersion == toVersion {
		return nil
	}
	key := id + "@" + fromVersion + "->" + toVersion
	route, ok := supportedUpdateRoutes[key]
	if !ok {
		return fmt.Errorf("capability pack %s has no supported update route from %s to %s", id, fromVersion, toVersion)
	}
	for _, existing := range route.ExistingSurfaces {
		if existing == surface {
			return nil
		}
	}
	return fmt.Errorf("capability pack %s update route from %s to %s does not add %s intent", id, fromVersion, toVersion, surface)
}

func init() {
	trustedHistoricalAggregates[strings.Join([]string{"ma", "tty@2.0.0"}, "")] = "b04e745e59562a3f0f65585af775ee22d68c4b54b43a795b2030557eb5f406c2"
	trustedHistoricalAggregates["engram@1.0.0"] = "78a02bd523d1c0383921d58507b595b4e0391023c12e666f1fd5d26dfb7c083f"
}

type historicalFileEvidence struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
}

type historicalResourceEvidence struct {
	Kind   string                   `json:"kind"`
	ID     string                   `json:"id"`
	Source string                   `json:"source"`
	Files  []historicalFileEvidence `json:"files"`
	SHA256 string                   `json:"sha256"`
}

type historicalArtifact struct {
	SchemaVersion   int                          `json:"schema_version"`
	PackID          string                       `json:"pack_id"`
	PackVersion     string                       `json:"pack_version"`
	Manifest        historicalFileEvidence       `json:"manifest"`
	Resources       []historicalResourceEvidence `json:"resources"`
	AggregateSHA256 string                       `json:"aggregate_sha256"`
}

func (c Catalog) resolveIntentPack(id, version string) (Pack, error) {
	current, err := c.catalogMetadata(id)
	if err != nil {
		return Pack{}, err
	}
	if version == "" {
		return c.Show(id)
	}
	if version == current.Version {
		return c.Show(id)
	}
	if c.allowSyntheticHistory {
		current.Version = version
		return current, nil
	}
	if c.bundleRoot == "" {
		return Pack{}, fmt.Errorf("capability pack %q targets unavailable historical version %s", id, version)
	}
	if !idPattern.MatchString(id) || !validSemver(version) {
		return Pack{}, fmt.Errorf("capability pack %q targets invalid historical version %q", id, version)
	}
	root := filepath.Join(c.bundleRoot, "history", id, version)
	var pack Pack
	err = c.withBundleLock(context.Background(), func(Catalog) error {
		var err error
		pack, err = loadHistoricalArtifact(root, c.bundleRoot, id, version)
		return err
	})
	if err != nil {
		return Pack{}, fmt.Errorf("load historical capability pack %s@%s: %w", id, version, err)
	}
	pack.Description = current.Description
	if pack.manifestVersion < manifestSchemaV3 {
		entry, ok := c.catalogEntry(id)
		if !ok {
			return Pack{}, fmt.Errorf("capability pack %q has no immutable catalog entry", id)
		}
		pack.Surfaces = append([]Surface(nil), entry.Surfaces...)
	}
	return pack, nil
}

func (c Catalog) catalogEntry(id string) (catalogEntry, bool) {
	for _, entry := range c.entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return catalogEntry{}, false
}

func loadHistoricalArtifact(root, bundleRoot, packID, version string) (Pack, error) {
	if err := validateHistoricalRoot(root, bundleRoot); err != nil {
		return Pack{}, err
	}
	artifactPath := filepath.Join(root, "artifact.json")
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return Pack{}, fmt.Errorf("read artifact.json: %w", err)
	}
	var artifact historicalArtifact
	if err := strictDecode(data, &artifact); err != nil {
		return Pack{}, fmt.Errorf("decode artifact.json: %w", err)
	}
	canonical, err := canonicalHistoricalArtifact(artifact)
	if err != nil {
		return Pack{}, err
	}
	if !bytes.Equal(data, canonical) {
		return Pack{}, fmt.Errorf("artifact.json is not the exact canonical artifact evidence")
	}
	if artifact.SchemaVersion != historicalArtifactSchemaVersion || artifact.PackID != packID || artifact.PackVersion != version {
		return Pack{}, fmt.Errorf("artifact identity does not match %s@%s", packID, version)
	}
	trustedAggregate, trusted := trustedHistoricalAggregates[packID+"@"+version]
	if !trusted {
		return Pack{}, fmt.Errorf("historical artifact %s@%s has no trusted immutable aggregate", packID, version)
	}
	if artifact.AggregateSHA256 != trustedAggregate {
		return Pack{}, fmt.Errorf("artifact aggregate hash does not match the trusted immutable aggregate")
	}
	if artifact.Manifest.Path != "pack.json" {
		return Pack{}, fmt.Errorf("artifact manifest path must be pack.json")
	}
	manifestPath := filepath.Join(root, artifact.Manifest.Path)
	if err := verifyHistoricalFile(root, manifestPath, artifact.Manifest); err != nil {
		return Pack{}, fmt.Errorf("verify historical manifest: %w", err)
	}
	pack, err := decodeManifest(manifestPath, root)
	if err != nil {
		return Pack{}, err
	}
	if pack.ID != packID || pack.Version != version {
		return Pack{}, fmt.Errorf("historical manifest identity is %s@%s, want %s@%s", pack.ID, pack.Version, packID, version)
	}
	expected, err := inspectHistoricalArtifact(root, pack)
	if err != nil {
		return Pack{}, err
	}
	expected.SchemaVersion = artifact.SchemaVersion
	expected.PackID = artifact.PackID
	expected.PackVersion = artifact.PackVersion
	expected.AggregateSHA256 = historicalAggregateHash(expected)
	if digestJSON(expected) != digestJSON(artifact) {
		return Pack{}, fmt.Errorf("artifact evidence or retained bytes changed")
	}
	if artifact.AggregateSHA256 != historicalAggregateHash(artifact) {
		return Pack{}, fmt.Errorf("artifact aggregate hash changed")
	}
	prefix, err := filepath.Rel(bundleRoot, root)
	if err != nil || !safeHistoricalPath(filepath.ToSlash(prefix)) {
		return Pack{}, fmt.Errorf("historical artifact root is not contained in the bundle")
	}
	for i := range pack.Resources {
		if pack.Resources[i].Source != "" {
			pack.Resources[i].Source = filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(pack.Resources[i].Source)))
		}
	}
	return pack, nil
}

func inspectHistoricalArtifact(root string, pack Pack) (historicalArtifact, error) {
	manifest, err := inspectHistoricalFile(root, filepath.Join(root, "pack.json"))
	if err != nil {
		return historicalArtifact{}, fmt.Errorf("inspect historical manifest: %w", err)
	}
	artifact := historicalArtifact{SchemaVersion: historicalArtifactSchemaVersion, PackID: pack.ID, PackVersion: pack.Version, Manifest: manifest}
	seenFiles := map[string]bool{"pack.json": true}
	for _, resource := range pack.Resources {
		if resource.Source == "" {
			continue
		}
		files, err := inspectHistoricalResource(root, resource)
		if err != nil {
			return historicalArtifact{}, fmt.Errorf("inspect historical resource %s:%s: %w", resource.Kind, resource.ID, err)
		}
		for _, file := range files {
			if seenFiles[file.Path] {
				return historicalArtifact{}, fmt.Errorf("historical file %q belongs to multiple resources", file.Path)
			}
			seenFiles[file.Path] = true
		}
		artifact.Resources = append(artifact.Resources, historicalResourceEvidence{Kind: resource.Kind, ID: resource.ID, Source: resource.Source, Files: files, SHA256: historicalFilesHash(files)})
	}
	actualFiles, err := historicalTreeFiles(root)
	if err != nil {
		return historicalArtifact{}, err
	}
	for _, path := range actualFiles {
		if path == "artifact.json" {
			continue
		}
		if !seenFiles[path] {
			return historicalArtifact{}, fmt.Errorf("unreferenced file %q is present in the historical artifact", path)
		}
	}
	if len(actualFiles)-1 != len(seenFiles) {
		return historicalArtifact{}, fmt.Errorf("historical artifact evidence does not cover every retained file")
	}
	artifact.AggregateSHA256 = historicalAggregateHash(artifact)
	return artifact, nil
}

func inspectHistoricalResource(root string, resource Resource) ([]historicalFileEvidence, error) {
	if !safeHistoricalPath(resource.Source) {
		return nil, fmt.Errorf("unsafe source path %q", resource.Source)
	}
	path := filepath.Join(root, filepath.FromSlash(resource.Source))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("source %q is a symlink", resource.Source)
	}
	if historicalFileResource(resource.Kind) {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("source %q has the wrong resource type", resource.Source)
		}
		file, err := inspectHistoricalFile(root, path)
		if err != nil {
			return nil, err
		}
		return []historicalFileEvidence{file}, nil
	}
	if resource.Kind != "skill" || !info.IsDir() {
		return nil, fmt.Errorf("source %q has the wrong resource type", resource.Source)
	}
	var files []historicalFileEvidence
	err = filepath.WalkDir(path, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == path {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := validateHistoricalMode(name, info.Mode(), entry.IsDir()); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		file, err := inspectHistoricalFile(root, name)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("source %q contains no files", resource.Source)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func historicalFileResource(kind string) bool {
	switch kind {
	case "instruction", "agent", "command", "asset", "notice":
		return true
	default:
		return false
	}
}

func historicalTreeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := validateHistoricalMode(name, info.Mode(), entry.IsDir()); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !safeHistoricalPath(relative) {
			return fmt.Errorf("unsafe historical path %q", relative)
		}
		files = append(files, relative)
		return nil
	})
	sort.Strings(files)
	return files, err
}

func inspectHistoricalFile(root, path string) (historicalFileEvidence, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return historicalFileEvidence{}, err
	}
	if err := validateHistoricalMode(path, info.Mode(), false); err != nil {
		return historicalFileEvidence{}, err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return historicalFileEvidence{}, err
	}
	relative = filepath.ToSlash(relative)
	if !safeHistoricalPath(relative) {
		return historicalFileEvidence{}, fmt.Errorf("unsafe historical file path %q", relative)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return historicalFileEvidence{}, err
	}
	return historicalFileEvidence{Path: relative, Size: int64(len(data)), Mode: uint32(info.Mode().Perm()), SHA256: historicalHash(data)}, nil
}

func verifyHistoricalFile(root, path string, expected historicalFileEvidence) error {
	actual, err := inspectHistoricalFile(root, path)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("file %q size, mode, or SHA-256 does not match artifact evidence", expected.Path)
	}
	return nil
}

func validateHistoricalRoot(root, bundleRoot string) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect artifact root: %w", err)
	}
	if err := validateHistoricalMode(root, rootInfo.Mode(), true); err != nil {
		return err
	}
	resolvedBundle, err := filepath.EvalSymlinks(bundleRoot)
	if err != nil {
		return fmt.Errorf("resolve bundle root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve artifact root: %w", err)
	}
	relative, err := filepath.Rel(resolvedBundle, resolvedRoot)
	if err != nil || !safeHistoricalPath(filepath.ToSlash(relative)) {
		return fmt.Errorf("historical artifact resolves outside the bundle root")
	}
	return nil
}

func validateHistoricalMode(path string, mode fs.FileMode, directory bool) error {
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe symlink in historical artifact: %s", path)
	}
	if mode.Perm()&0o022 != 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("unsafe permissions %04o in historical artifact: %s", mode.Perm(), path)
	}
	if directory {
		if !mode.IsDir() {
			return fmt.Errorf("historical artifact path is not a directory: %s", path)
		}
		return nil
	}
	if !mode.IsRegular() || mode.Perm()&0o600 != 0o600 {
		return fmt.Errorf("historical artifact path is not a safe regular file: %s", path)
	}
	return nil
}

func safeHistoricalPath(path string) bool {
	return path != "" && path != "." && !strings.Contains(path, "\\") && !filepath.IsAbs(path) && filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) == path && path != ".." && !strings.HasPrefix(path, "../")
}

func historicalFilesHash(files []historicalFileEvidence) string {
	hash := sha256.New()
	for _, file := range files {
		fmt.Fprintf(hash, "%s\x00%d\x00%04o\x00%s\n", file.Path, file.Size, file.Mode, file.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func historicalAggregateHash(artifact historicalArtifact) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%d\x00%s\x00%s\n", artifact.SchemaVersion, artifact.PackID, artifact.PackVersion)
	fmt.Fprintf(hash, "manifest\x00%s\x00%d\x00%04o\x00%s\n", artifact.Manifest.Path, artifact.Manifest.Size, artifact.Manifest.Mode, artifact.Manifest.SHA256)
	for _, resource := range artifact.Resources {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\n", resource.Kind, resource.ID, resource.Source, resource.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func historicalHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func canonicalHistoricalArtifact(artifact historicalArtifact) ([]byte, error) {
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode canonical artifact evidence: %w", err)
	}
	return append(data, '\n'), nil
}

func hasTrustedHistoricalArtifact(packID, version string) bool {
	_, ok := trustedHistoricalAggregates[packID+"@"+version]
	return ok
}
