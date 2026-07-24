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
	sample := validAddyQualification()
	trust := AddyReleaseTrust{
		Repository: sample.Repository, Workflow: sample.Workflow,
		WorkflowDigest: sample.WorkflowDigest, RunID: sample.RunID,
	}
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
	if err := ValidateReleaseAddyQualificationMatrix(root, version, sha, trust, false); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*AddyReleaseTrust){
		"repository":      func(t *AddyReleaseTrust) { t.Repository = "other/repository" },
		"workflow":        func(t *AddyReleaseTrust) { t.Workflow = ".github/workflows/other.yml" },
		"workflow digest": func(t *AddyReleaseTrust) { t.WorkflowDigest = strings.Repeat("b", 64) },
		"run":             func(t *AddyReleaseTrust) { t.RunID = "other-run" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := trust
			mutate(&changed)
			if err := ValidateReleaseAddyQualificationMatrix(root, version, sha, changed, false); err == nil {
				t.Fatal("release matrix accepted evidence outside the trusted workflow run")
			}
		})
	}
	if err := ValidateReleaseAddyQualificationMatrix(root, version, sha, trust, true); err == nil {
		t.Fatal("synthetic pre-candidate matrix crossed the production boundary")
	}

	for _, arch := range []string{"amd64", "arm64"} {
		for _, selector := range []string{ExactFloor, "stable"} {
			path := filepath.Join(root, arch+"-"+selector, "addy-qualification.json")
			var changed AddyQualification
			data, err := os.ReadFile(path)
			if err != nil || json.Unmarshal(data, &changed) != nil {
				t.Fatal(err)
			}
			changed.RunID = "stale-run"
			data, _ = CanonicalAddyQualificationJSON(changed)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := ValidateReleaseAddyQualificationMatrix(root, version, sha, trust, false); err == nil {
		t.Fatal("mutually consistent stale-run Addy release qualification accepted")
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
