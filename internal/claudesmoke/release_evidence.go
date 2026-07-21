package claudesmoke

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// ValidateReleaseEvidenceMatrix proves the exact two-selector, two-architecture
// release matrix against the canonical smoke-evidence validator.
func ValidateReleaseEvidenceMatrix(root, packyVersion, packySHA string) error {
	if root == "" || packyVersion == "" || len(packySHA) != 40 {
		return errors.New("release evidence root, Packy version, and full SHA are required")
	}
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "evidence.json" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("discover release evidence: %w", err)
	}
	sort.Strings(paths)
	if len(paths) != 4 {
		return fmt.Errorf("release requires exactly four Claude smoke evidence documents; got %d", len(paths))
	}
	want := map[string]bool{
		"amd64|" + ExactFloor: true,
		"amd64|stable":        true,
		"arm64|" + ExactFloor: true,
		"arm64|stable":        true,
	}
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		var evidence Evidence
		if decodeErr := json.Unmarshal(data, &evidence); decodeErr != nil {
			return fmt.Errorf("decode %s: %w", path, decodeErr)
		}
		if validateErr := ValidateEvidence(evidence); validateErr != nil {
			return fmt.Errorf("invalid %s: %w", path, validateErr)
		}
		if evidence.PackyVersion != packyVersion || evidence.PackyRef != packySHA || evidence.PackySHA != packySHA || evidence.InstalledSourceSHA != packySHA || evidence.OS != "darwin" {
			return fmt.Errorf("release identity mismatch in %s", path)
		}
		key := evidence.Arch + "|" + evidence.RequestedClaudeVersion
		if !want[key] {
			return fmt.Errorf("duplicated or unexpected release smoke %s in %s", key, path)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		return fmt.Errorf("release smoke matrix is incomplete: %v", want)
	}
	return nil
}
