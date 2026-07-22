package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/release"
)

func TestCreateAndVerifyLifecycleOffline(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	output := filepath.Join(root, "metadata")
	os.Mkdir(dist, 0o700)
	notes := filepath.Join(root, "notes.md")
	os.WriteFile(notes, []byte("release notes\n"), 0o600)
	writeDistFixture(t, dist)
	var stdout bytes.Buffer
	args := []string{"create", "--version", "v0.1.2", "--repository", release.PackyRepository, "--ref", release.PackyMainRef, "--commit", strings.Repeat("c", 40), "--workflow", release.PackyReleaseWorkflow, "--workflow-sha", strings.Repeat("d", 64), "--release-notes", notes, "--dist", dist, "--output-dir", output, "--permission", "actions=read", "--permission", "contents=write"}
	if err := run(args, &stdout); err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(output, "candidate.json")
	provenancePath := filepath.Join(output, "provenance.json")
	if got, want := stdout.String(), string(mustRead(t, candidatePath)); got != want {
		t.Fatalf("stdout differs from candidate file")
	}
	stdout.Reset()
	if err := run([]string{"verify-provenance", "--candidate", candidatePath, "--provenance", provenancePath}, &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "{\"verified\":true}\n" {
		t.Fatalf("unexpected verification: %q", stdout.String())
	}
	var candidate release.Candidate
	json.Unmarshal(mustRead(t, candidatePath), &candidate)
	statePath := filepath.Join(root, "state.json")
	state := releaseState{Version: candidate.Version, Repository: candidate.Repository, Ref: candidate.Ref, TargetCommit: candidate.Commit, Workflow: candidate.Workflow, WorkflowSHA: candidate.WorkflowSHA, ReleaseNotesSHA256: candidate.ReleaseNotesSHA256, Draft: true}
	for _, subject := range candidate.Subjects {
		state.Assets = append(state.Assets, serverAsset{Name: subject.Name, Digest: "sha256:" + subject.SHA256})
	}
	writeJSON(t, statePath, state)
	stdout.Reset()
	if err := run([]string{"verify-state", "--candidate", candidatePath, "--provenance", provenancePath, "--state", statePath, "--mode", "draft"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "{\"decision\":\"publish-draft\",\"missing_assets\":[]}\n" {
		t.Fatalf("unexpected decision: %s", stdout.String())
	}
}

func TestCreateRejectsUnsafeFilesystemAndOverlap(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	os.Mkdir(dist, 0o700)
	notes := filepath.Join(root, "notes")
	os.WriteFile(notes, []byte("notes"), 0o600)
	writeDistFixture(t, dist)
	if err := os.Symlink(filepath.Join(dist, "packy_v0.1.2_linux_amd64.tar.gz"), filepath.Join(dist, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := run(createArgs(notes, dist, filepath.Join(root, "out")), ioDiscard{}); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink error = %v", err)
	}
	os.Remove(filepath.Join(dist, "linked"))
	if err := run(createArgs(notes, dist, filepath.Join(dist, "metadata")), ioDiscard{}); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlap error = %v", err)
	}
}

func TestStrictEvidenceAndStateJSONRejectDuplicateAndUnknownFields(t *testing.T) {
	root := t.TempDir()
	candidatePath := filepath.Join(root, "candidate.json")
	os.WriteFile(candidatePath, []byte(`{"id":"first","id":"last"}`), 0o600)
	var candidate release.Candidate
	if err := strictReadJSON(candidatePath, &candidate, candidateSchema); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
	statePath := filepath.Join(root, "state.json")
	os.WriteFile(statePath, []byte(`{"Version":"v0.1.2"}`), 0o600)
	var state releaseState
	if err := strictReadJSON(statePath, &state, stateSchema); err == nil || !strings.Contains(err.Error(), "incorrectly cased") {
		t.Fatalf("case error = %v", err)
	}
}

func TestVerifyProvenanceRejectsTamperedCandidateID(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	output := filepath.Join(root, "out")
	os.Mkdir(dist, 0o700)
	notes := filepath.Join(root, "notes")
	os.WriteFile(notes, []byte("notes"), 0o600)
	writeDistFixture(t, dist)
	if err := run(createArgs(notes, dist, output), ioDiscard{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(output, "candidate.json")
	var value map[string]any
	json.Unmarshal(mustRead(t, path), &value)
	value["id"] = strings.Repeat("e", 64)
	writeJSON(t, path, value)
	if err := run([]string{"verify-provenance", "--candidate", path, "--provenance", filepath.Join(output, "provenance.json")}, ioDiscard{}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("tamper error = %v", err)
	}
}

func createArgs(notes, dist, output string) []string {
	return []string{"create", "--version", "v0.1.2", "--repository", release.PackyRepository, "--ref", release.PackyMainRef, "--commit", strings.Repeat("c", 40), "--workflow", release.PackyReleaseWorkflow, "--workflow-sha", strings.Repeat("d", 64), "--release-notes", notes, "--dist", dist, "--output-dir", output, "--permission", "actions=read", "--permission", "contents=write"}
}
func writeDistFixture(t *testing.T, dir string) {
	t.Helper()
	name := "packy_v0.1.2_linux_amd64.tar.gz"
	binary := []byte("binary")
	binaryDigest := testDigest(binary)
	id := "SPDXRef-File-" + hex.EncodeToString([]byte(name))
	sbom := []byte(`{"spdxVersion":"SPDX-2.3","SPDXID":"SPDXRef-DOCUMENT","dataLicense":"CC0-1.0","name":"packy-v0.1.2","documentNamespace":"https://github.com/yersonargotev/packy/releases/download/v0.1.2/sbom.spdx.json","creationInfo":{"created":"2026-01-02T03:04:05Z","creators":["Tool: packy-release"]},"documentDescribes":["` + id + `"],"files":[{"fileName":"` + name + `","SPDXID":"` + id + `","checksums":[{"algorithm":"SHA256","checksumValue":"` + binaryDigest + `"}],"licenseConcluded":"NOASSERTION","copyrightText":"NOASSERTION"}]}`)
	checksums := []byte(binaryDigest + "  " + name + "\n" + testDigest(sbom) + "  " + release.SBOMName + "\n")
	for name, data := range map[string][]byte{name: binary, release.SBOMName: sbom, release.ChecksumsName: checksums} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
func testDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := canonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
