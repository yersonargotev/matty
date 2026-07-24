package claudesmoke

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateReleaseEvidenceMatrixUsesCanonicalEvidence(t *testing.T) {
	root := t.TempDir()
	version := "v0.99.0"
	sha := strings.Repeat("a", 40)
	for _, arch := range []string{"amd64", "arm64"} {
		for _, selector := range []string{ExactFloor, "stable"} {
			evidence := validEvidence()
			evidence.PackyVersion = version
			evidence.PackyRef = sha
			evidence.PackySHA = sha
			evidence.InstalledSourceSHA = sha
			evidence.OS = "darwin"
			evidence.Arch = arch
			evidence.RequestedClaudeVersion = selector
			path := filepath.Join(root, arch+"-"+selector, "evidence.json")
			writeReleaseEvidence(t, path, evidence)
		}
	}
	if err := ValidateReleaseEvidenceMatrix(root, version, sha); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "amd64-stable", "evidence.json")
	tampered := validEvidence()
	tampered.PackyVersion = version
	tampered.PackyRef = sha
	tampered.PackySHA = sha
	tampered.InstalledSourceSHA = strings.Repeat("b", 40)
	tampered.OS = "darwin"
	tampered.Arch = "amd64"
	tampered.RequestedClaudeVersion = "stable"
	writeReleaseEvidence(t, path, tampered)
	if err := ValidateReleaseEvidenceMatrix(root, version, sha); err == nil {
		t.Fatal("accepted release matrix with a cross-commit Installed Source")
	}
}

func TestValidateReleaseAddyQualificationMatrixRequiresOneSyntheticRun(t *testing.T) {
	root := t.TempDir()
	version := "v0.99.0"
	sha := strings.Repeat("a", 40)
	for _, arch := range []string{"amd64", "arm64"} {
		for _, selector := range []string{ExactFloor, "stable"} {
			qualification := validAddyQualification()
			qualification.Synthetic, qualification.Tag, qualification.Commit = true, version, sha
			qualification.InstalledSourceCommit = sha
			qualification.Smoke.PackyVersion, qualification.Smoke.PackyRef = version, version
			qualification.Smoke.PackySHA, qualification.Smoke.InstalledSourceSHA = sha, sha
			qualification.Smoke.OS, qualification.Smoke.Arch = "darwin", arch
			qualification.Smoke.RequestedClaudeVersion = selector
			path := filepath.Join(root, arch+"-"+selector, "addy-qualification.json")
			data, err := CanonicalAddyQualificationJSON(qualification)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := ValidateReleaseAddyQualificationMatrix(root, version, sha, false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReleaseAddyQualificationMatrix(root, version, sha, true); err == nil {
		t.Fatal("synthetic pre-candidate matrix crossed the production boundary")
	}

	path := filepath.Join(root, "amd64-stable", "addy-qualification.json")
	var changed AddyQualification
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &changed) != nil {
		t.Fatal(err)
	}
	changed.RunID = "other-run"
	data, _ = CanonicalAddyQualificationJSON(changed)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReleaseAddyQualificationMatrix(root, version, sha, false); err == nil {
		t.Fatal("cross-run Addy release qualification accepted")
	}
}

func writeReleaseEvidence(t *testing.T, path string, evidence Evidence) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
