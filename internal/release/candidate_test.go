package release_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/release"
)

func TestCandidateIdentityIsCanonicalDetachedAndBindsNotes(t *testing.T) {
	observation := fixtureObservation()
	candidate := mustCandidate(t, observation)
	reversed := observation
	reversed.Permissions = reversePermissions(observation.Permissions)
	reversed.Subjects = reverseSubjects(observation.Subjects)
	other := mustCandidate(t, reversed)
	if candidate.ID != other.ID || !reflect.DeepEqual(candidate, other) {
		t.Fatalf("identity depends on input order:\n%+v\n%+v", candidate, other)
	}
	observation.Permissions[0].Access = "write"
	observation.Subjects[0].SHA256 = strings.Repeat("f", 64)
	if candidate.Permissions[0].Access != "read" || candidate.Subjects[0].SHA256 == strings.Repeat("f", 64) {
		t.Fatal("candidate aliases caller-owned input")
	}
	changedNotes := fixtureObservation()
	changedNotes.ReleaseNotesSHA256 = strings.Repeat("e", 64)
	if mustCandidate(t, changedNotes).ID == candidate.ID {
		t.Fatal("release notes digest is absent from candidate identity")
	}
}

func TestCandidateEnforcesFixedPackyPolicy(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*release.Observation)
	}{
		{"version", func(o *release.Observation) { o.Version = "v1.2.3" }},
		{"repository", func(o *release.Observation) { o.Repository = "attacker/fork" }},
		{"ref", func(o *release.Observation) { o.Ref = "refs/pull/1/head" }},
		{"commit", func(o *release.Observation) { o.Commit = "main" }},
		{"workflow", func(o *release.Observation) { o.Workflow = "evil.yml" }},
		{"workflow digest", func(o *release.Observation) { o.WorkflowSHA = "short" }},
		{"notes digest", func(o *release.Observation) { o.ReleaseNotesSHA256 = "short" }},
		{"unknown permission", func(o *release.Observation) {
			o.Permissions = append(o.Permissions, release.Permission{Name: "issues", Access: "write"})
		}},
		{"excess permission", func(o *release.Observation) { o.Permissions[0].Access = "write" }},
		{"duplicate permission", func(o *release.Observation) { o.Permissions = append(o.Permissions, o.Permissions[0]) }},
		{"duplicate subject", func(o *release.Observation) { o.Subjects = append(o.Subjects, o.Subjects[0]) }},
		{"unsafe subject", func(o *release.Observation) { o.Subjects[0].Name = "../packy" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := fixtureObservation()
			test.mutate(&observation)
			if _, err := release.NewCandidate(observation); err == nil {
				t.Fatal("expected fail-closed validation")
			}
		})
	}
}

func TestCandidateValidatesExactSHA256SUMSAndSPDX(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*release.Observation)
	}{
		{"missing checksums subject", func(o *release.Observation) { o.Subjects = dropSubject(o.Subjects, release.ChecksumsName) }},
		{"missing sbom subject", func(o *release.Observation) { o.Subjects = dropSubject(o.Subjects, release.SBOMName) }},
		{"stale checksum bytes", func(o *release.Observation) { o.SHA256SUMS[0] = 'f' }},
		{"malformed checksum", func(o *release.Observation) { replaceChecksums(o, "malformed\n") }},
		{"unterminated checksum", func(o *release.Observation) { replaceChecksums(o, strings.TrimSuffix(string(o.SHA256SUMS), "\n")) }},
		{"checksum missing", func(o *release.Observation) { replaceChecksums(o, checksumLine(o.Subjects, release.SBOMName)) }},
		{"checksum extra", func(o *release.Observation) {
			replaceChecksums(o, string(o.SHA256SUMS)+strings.Repeat("e", 64)+"  extra\n")
		}},
		{"checksum duplicate", func(o *release.Observation) {
			replaceChecksums(o, string(o.SHA256SUMS)+checksumLine(o.Subjects, release.SBOMName))
		}},
		{"checksum mismatch", func(o *release.Observation) {
			replaceChecksums(o, strings.Repeat("e", 64)+"  packy_v0.1.2_linux_amd64.tar.gz\n"+checksumLine(o.Subjects, release.SBOMName))
		}},
		{"malformed sbom", func(o *release.Observation) { replaceSBOM(o, []byte("{")) }},
		{"stale sbom name", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.ReplaceAll(string(o.SBOM), "packy-v0.1.2", "packy-v0.1.1")))
		}},
		{"wrong SPDX version", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), "SPDX-2.3", "SPDX-2.2", 1)))
		}},
		{"missing document id", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), `"SPDXID":"SPDXRef-DOCUMENT",`, "", 1)))
		}},
		{"wrong data license", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), "CC0-1.0", "MIT", 1)))
		}},
		{"namespace collision", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), "releases/download/v0.1.2/sbom.spdx.json", "attacker/v0.1.2/sbom.spdx.json", 1)))
		}},
		{"invalid created", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), "2026-01-02T03:04:05Z", "yesterday", 1)))
		}},
		{"wrong creator", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), "Tool: packy-release", "Person: attacker", 1)))
		}},
		{"missing file id", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), `"SPDXID":"SPDXRef-File-packy-v0.1.2-linux-amd64.tar.gz",`, "", 1)))
		}},
		{"missing file license", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), `,"licenseConcluded":"NOASSERTION"`, "", 1)))
		}},
		{"missing copyright", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), `,"copyrightText":"NOASSERTION"`, "", 1)))
		}},
		{"missing document describes", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.Replace(string(o.SBOM), `"documentDescribes":["SPDXRef-File-packy-v0.1.2-linux-amd64.tar.gz"],`, "", 1)))
		}},
		{"sbom missing", func(o *release.Observation) {
			replaceSBOM(o, []byte(`{"spdxVersion":"SPDX-2.3","name":"packy-v0.1.2","documentNamespace":"https://packy.dev/spdx/v0.1.2","files":[]}`))
		}},
		{"sbom mismatch", func(o *release.Observation) {
			replaceSBOM(o, []byte(strings.ReplaceAll(string(o.SBOM), strings.Repeat("b", 64), strings.Repeat("e", 64))))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := fixtureObservation()
			test.mutate(&observation)
			if _, err := release.NewCandidate(observation); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
}

func TestProvenanceBindsEveryCandidateInputOffline(t *testing.T) {
	candidate := mustCandidate(t, fixtureObservation())
	if err := release.VerifyProvenance(candidate, release.ProvenanceFor(candidate)); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*release.Provenance){
		func(p *release.Provenance) { p.Repository = "attacker/fork" },
		func(p *release.Provenance) { p.Ref = "refs/tags/v0.1.2" },
		func(p *release.Provenance) { p.Commit = strings.Repeat("d", 40) },
		func(p *release.Provenance) { p.Workflow = "evil.yml" },
		func(p *release.Provenance) { p.WorkflowSHA = strings.Repeat("e", 64) },
		func(p *release.Provenance) { p.ReleaseNotesSHA256 = strings.Repeat("e", 64) },
		func(p *release.Provenance) { p.Permissions[0].Access = "write" },
		func(p *release.Provenance) { p.Subjects = p.Subjects[:2] },
		func(p *release.Provenance) { p.Subjects[0].SHA256 = strings.Repeat("e", 64) },
	}
	for i, mutate := range mutations {
		provenance := release.ProvenanceFor(candidate)
		mutate(&provenance)
		if err := release.VerifyProvenance(candidate, provenance); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
}

func TestDraftLifecycleBindsFullMetadataAndAssetsWithoutMutation(t *testing.T) {
	candidate := mustCandidate(t, fixtureObservation())
	exact := exactRelease(candidate, true, candidate.Subjects)
	tests := []struct {
		name     string
		releases []release.Release
		want     release.Lifecycle
		missing  []release.Subject
	}{
		{"absent", nil, release.ResumeDraft, candidate.Subjects},
		{"partial", []release.Release{exactRelease(candidate, true, []release.Subject{candidate.Subjects[0]})}, release.ResumeDraft, candidate.Subjects[1:]},
		{"complete draft", []release.Release{exact}, release.PublishDraft, nil},
		{"published continuation", []release.Release{exactRelease(candidate, false, reverseSubjects(candidate.Subjects))}, release.ContinuePublished, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := cloneReleases(test.releases)
			got, err := release.VerifyLifecycle(candidate, test.releases)
			if err != nil {
				t.Fatal(err)
			}
			if got.Lifecycle != test.want || !reflect.DeepEqual(got.Missing, test.missing) {
				t.Fatalf("got %+v, want %s %+v", got, test.want, test.missing)
			}
			if !reflect.DeepEqual(test.releases, before) {
				t.Fatal("verification mutated observed state")
			}
		})
	}
}

func TestDraftLifecycleRejectsAlteredMetadataAndAmbiguity(t *testing.T) {
	candidate := mustCandidate(t, fixtureObservation())
	mutations := []func(*release.Release){
		func(r *release.Release) { r.CandidateID = strings.Repeat("e", 64) },
		func(r *release.Release) { r.Repository = "attacker/fork" },
		func(r *release.Release) { r.Ref = "refs/tags/v0.1.2" },
		func(r *release.Release) { r.TargetCommit = strings.Repeat("e", 40) },
		func(r *release.Release) { r.Workflow = "evil.yml" },
		func(r *release.Release) { r.WorkflowSHA = strings.Repeat("e", 64) },
		func(r *release.Release) { r.ReleaseNotesSHA256 = strings.Repeat("e", 64) },
		func(r *release.Release) { r.Assets[0].SHA256 = strings.Repeat("e", 64) },
		func(r *release.Release) {
			r.Assets = append(r.Assets, release.Subject{Name: "extra", SHA256: strings.Repeat("e", 64)})
		},
		func(r *release.Release) { r.Assets = append(r.Assets, r.Assets[0]) },
	}
	for i, mutate := range mutations {
		observed := exactRelease(candidate, true, candidate.Subjects)
		mutate(&observed)
		if _, err := release.VerifyLifecycle(candidate, []release.Release{observed}); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
	provenanceMutations := []func(*release.Provenance){
		func(p *release.Provenance) { p.CandidateID = strings.Repeat("e", 64) },
		func(p *release.Provenance) { p.Version = "v0.1.1" },
		func(p *release.Provenance) { p.Repository = "attacker/fork" },
		func(p *release.Provenance) { p.Ref = "refs/tags/v0.1.2" },
		func(p *release.Provenance) { p.Commit = strings.Repeat("e", 40) },
		func(p *release.Provenance) { p.Workflow = "evil.yml" },
		func(p *release.Provenance) { p.WorkflowSHA = strings.Repeat("e", 64) },
		func(p *release.Provenance) { p.ReleaseNotesSHA256 = strings.Repeat("e", 64) },
		func(p *release.Provenance) { p.Permissions[0].Access = "write" },
		func(p *release.Provenance) { p.Subjects[0].SHA256 = strings.Repeat("e", 64) },
	}
	for i, mutate := range provenanceMutations {
		observed := exactRelease(candidate, true, candidate.Subjects)
		mutate(&observed.Provenance)
		if observed.CandidateID != candidate.ID {
			t.Fatal("fixture changed apparent candidate ID")
		}
		if _, err := release.VerifyLifecycle(candidate, []release.Release{observed}); err == nil {
			t.Fatalf("provenance mutation %d accepted", i)
		}
	}
	exact := exactRelease(candidate, true, candidate.Subjects)
	if _, err := release.VerifyLifecycle(candidate, []release.Release{exact, exact}); err == nil {
		t.Fatal("ambiguous releases accepted")
	}
	published := exactRelease(candidate, false, candidate.Subjects)
	if _, err := release.VerifyDraftPreparation(candidate, []release.Release{published}); err == nil {
		t.Fatal("published release accepted for draft preparation")
	}
	if err := release.VerifyPublishedContinuation(candidate, []release.Release{published}); err != nil {
		t.Fatal(err)
	}
}

func fixtureObservation() release.Observation {
	binary := release.Subject{Name: "packy_v0.1.2_linux_amd64.tar.gz", SHA256: strings.Repeat("b", 64)}
	sbom := []byte(fmt.Sprintf(`{"spdxVersion":"SPDX-2.3","SPDXID":"SPDXRef-DOCUMENT","dataLicense":"CC0-1.0","name":"packy-v0.1.2","documentNamespace":"https://github.com/yersonargotev/packy/releases/download/v0.1.2/sbom.spdx.json","creationInfo":{"created":"2026-01-02T03:04:05Z","creators":["Tool: packy-release"]},"documentDescribes":["SPDXRef-File-packy-v0.1.2-linux-amd64.tar.gz"],"files":[{"fileName":%q,"SPDXID":"SPDXRef-File-packy-v0.1.2-linux-amd64.tar.gz","checksums":[{"algorithm":"SHA256","checksumValue":%q}],"licenseConcluded":"NOASSERTION","copyrightText":"NOASSERTION"}]}`, binary.Name, binary.SHA256))
	sbomSubject := release.Subject{Name: release.SBOMName, SHA256: digest(sbom)}
	checksums := []byte(binary.SHA256 + "  " + binary.Name + "\n" + sbomSubject.SHA256 + "  " + sbomSubject.Name + "\n")
	return release.Observation{
		Version: "v0.1.2", Repository: release.PackyRepository, Ref: release.PackyMainRef,
		Commit: strings.Repeat("c", 40), Workflow: release.PackyReleaseWorkflow,
		WorkflowSHA: strings.Repeat("d", 64), ReleaseNotesSHA256: strings.Repeat("c", 64),
		Permissions: []release.Permission{{Name: "actions", Access: "read"}, {Name: "contents", Access: "write"}},
		Subjects:    []release.Subject{binary, sbomSubject, {Name: release.ChecksumsName, SHA256: digest(checksums)}},
		SHA256SUMS:  checksums, SBOM: sbom,
	}
}

func exactRelease(c release.Candidate, draft bool, assets []release.Subject) release.Release {
	return release.Release{
		Version: c.Version, CandidateID: c.ID, Provenance: release.ProvenanceFor(c), Repository: c.Repository, Ref: c.Ref,
		TargetCommit: c.Commit, Workflow: c.Workflow, WorkflowSHA: c.WorkflowSHA,
		ReleaseNotesSHA256: c.ReleaseNotesSHA256, Draft: draft, Assets: append([]release.Subject(nil), assets...),
	}
}
func replaceChecksums(o *release.Observation, content string) {
	o.SHA256SUMS = []byte(content)
	for i := range o.Subjects {
		if o.Subjects[i].Name == release.ChecksumsName {
			o.Subjects[i].SHA256 = digest(o.SHA256SUMS)
		}
	}
}
func replaceSBOM(o *release.Observation, content []byte) {
	o.SBOM = content
	for i := range o.Subjects {
		if o.Subjects[i].Name == release.SBOMName {
			o.Subjects[i].SHA256 = digest(content)
		}
	}
	// Keep the checksum manifest internally consistent so the SBOM validator is the failing gate.
	var lines []string
	for _, s := range o.Subjects {
		if s.Name != release.ChecksumsName {
			lines = append(lines, s.SHA256+"  "+s.Name)
		}
	}
	sort.Strings(lines)
	replaceChecksums(o, strings.Join(lines, "\n")+"\n")
}
func checksumLine(subjects []release.Subject, name string) string {
	for _, s := range subjects {
		if s.Name == name {
			return s.SHA256 + "  " + s.Name + "\n"
		}
	}
	return ""
}
func dropSubject(subjects []release.Subject, name string) []release.Subject {
	var out []release.Subject
	for _, s := range subjects {
		if s.Name != name {
			out = append(out, s)
		}
	}
	return out
}
func digest(content []byte) string { sum := sha256.Sum256(content); return hex.EncodeToString(sum[:]) }
func mustCandidate(t *testing.T, observation release.Observation) release.Candidate {
	t.Helper()
	candidate, err := release.NewCandidate(observation)
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}
func reversePermissions(in []release.Permission) []release.Permission {
	out := append([]release.Permission(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
func reverseSubjects(in []release.Subject) []release.Subject {
	out := append([]release.Subject(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
func cloneReleases(in []release.Release) []release.Release {
	out := append([]release.Release(nil), in...)
	for i := range out {
		out[i].Assets = append([]release.Subject(nil), out[i].Assets...)
	}
	return out
}
