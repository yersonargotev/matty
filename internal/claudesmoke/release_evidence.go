package claudesmoke

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// ValidateReleaseAddyQualificationMatrix proves that all four Addy smoke
// qualifications came from one exact-tag workflow run. Synthetic mode
// qualifies the pre-candidate harness but remains production-inadmissible.
func ValidateReleaseAddyQualificationMatrix(root, packyVersion, packySHA string, production bool) error {
	if root == "" || packyVersion == "" || len(packySHA) != 40 {
		return errors.New("Addy release qualification root, Packy version, and full SHA are required")
	}
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "addy-qualification.json" {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("discover Addy release qualification: %w", err)
	}
	sort.Strings(paths)
	if len(paths) != 4 {
		return fmt.Errorf("release requires exactly four Addy qualification documents; got %d", len(paths))
	}
	want := map[string]bool{
		"amd64|" + ExactFloor: true,
		"amd64|stable":        true,
		"arm64|" + ExactFloor: true,
		"arm64|stable":        true,
	}
	var identity struct{ repository, workflow, workflowDigest, runID string }
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var qualification AddyQualification
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&qualification); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return fmt.Errorf("decode %s: trailing JSON", path)
		}
		if production {
			err = ValidateProductionAddyQualification(qualification)
		} else {
			err = ValidateAddyQualification(qualification)
			if err == nil && !qualification.Synthetic {
				err = errors.New("pre-candidate Addy release qualification must remain synthetic")
			}
		}
		if err != nil {
			return fmt.Errorf("invalid %s: %w", path, err)
		}
		if qualification.Commit != packySHA || qualification.Tag != packyVersion {
			return fmt.Errorf("Addy release identity mismatch in %s", path)
		}
		if i == 0 {
			identity.repository, identity.workflow = qualification.Repository, qualification.Workflow
			identity.workflowDigest, identity.runID = qualification.WorkflowDigest, qualification.RunID
		} else if qualification.Repository != identity.repository || qualification.Workflow != identity.workflow ||
			qualification.WorkflowDigest != identity.workflowDigest || qualification.RunID != identity.runID {
			return fmt.Errorf("Addy release qualification is cross-workflow or cross-run in %s", path)
		}
		key := qualification.Smoke.Arch + "|" + qualification.Smoke.RequestedClaudeVersion
		if !want[key] {
			return fmt.Errorf("duplicated or unexpected Addy release smoke %s in %s", key, path)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		return fmt.Errorf("Addy release qualification matrix is incomplete: %v", sortedEvidenceKeys(want))
	}
	return nil
}

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
		return fmt.Errorf("release smoke matrix is incomplete: %v", sortedEvidenceKeys(want))
	}
	return nil
}

func sortedEvidenceKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
