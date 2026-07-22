package release_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/release"
)

func TestCandidateIdentityIsCanonicalAndDetached(t *testing.T) {
	permissions := []release.Permission{{Name: "contents", Access: "write"}, {Name: "actions", Access: "read"}}
	subjects := fixtureSubjects()
	candidate := fixtureCandidate(t, permissions, subjects)
	reversed := fixtureCandidate(t, reversePermissions(permissions), reverseSubjects(subjects))
	if candidate.ID != reversed.ID || !reflect.DeepEqual(candidate, reversed) {
		t.Fatalf("identity depends on input order:\n%+v\n%+v", candidate, reversed)
	}
	permissions[0].Access = "none"
	subjects[0].SHA256 = strings.Repeat("f", 64)
	if candidate.Permissions[1].Access != "write" || candidate.Subjects[0].SHA256 != strings.Repeat("a", 64) {
		t.Fatal("candidate aliases caller-owned input")
	}
}

func TestCandidateRejectsUnauthorizedOrMalformedIdentity(t *testing.T) {
	authority := fixtureAuthority()
	permissions := authority.Permissions
	subjects := fixtureSubjects()
	tests := []struct {
		name        string
		version     string
		repository  string
		ref         string
		commit      string
		workflow    string
		workflowSHA string
		perms       []release.Permission
		subjects    []release.Subject
	}{
		{"version", "v1.2.3", authority.Repository, authority.Ref, strings.Repeat("c", 40), authority.Workflow, authority.WorkflowSHA, permissions, subjects},
		{"repository", "v0.1.2", "attacker/fork", authority.Ref, strings.Repeat("c", 40), authority.Workflow, authority.WorkflowSHA, permissions, subjects},
		{"ref", "v0.1.2", authority.Repository, "refs/pull/1/head", strings.Repeat("c", 40), authority.Workflow, authority.WorkflowSHA, permissions, subjects},
		{"commit", "v0.1.2", authority.Repository, authority.Ref, "main", authority.Workflow, authority.WorkflowSHA, permissions, subjects},
		{"workflow", "v0.1.2", authority.Repository, authority.Ref, strings.Repeat("c", 40), "evil.yml", authority.WorkflowSHA, permissions, subjects},
		{"workflow digest", "v0.1.2", authority.Repository, authority.Ref, strings.Repeat("c", 40), authority.Workflow, strings.Repeat("e", 64), permissions, subjects},
		{"permission", "v0.1.2", authority.Repository, authority.Ref, strings.Repeat("c", 40), authority.Workflow, authority.WorkflowSHA, append(append([]release.Permission(nil), permissions...), release.Permission{Name: "issues", Access: "write"}), subjects},
		{"duplicate subject", "v0.1.2", authority.Repository, authority.Ref, strings.Repeat("c", 40), authority.Workflow, authority.WorkflowSHA, permissions, append(subjects, subjects[0])},
		{"bad digest", "v0.1.2", authority.Repository, authority.Ref, strings.Repeat("c", 40), authority.Workflow, authority.WorkflowSHA, permissions, []release.Subject{{Name: "packy", SHA256: "short"}}},
		{"unsafe subject", "v0.1.2", authority.Repository, authority.Ref, strings.Repeat("c", 40), authority.Workflow, authority.WorkflowSHA, permissions, []release.Subject{{Name: "../packy", SHA256: strings.Repeat("a", 64)}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := release.NewCandidate(test.version, test.repository, test.ref, test.commit, test.workflow, test.workflowSHA, test.perms, test.subjects, authority); err == nil {
				t.Fatal("expected fail-closed validation")
			}
		})
	}
}

func TestProvenanceBindsEveryCandidateInputOffline(t *testing.T) {
	candidate := fixtureCandidate(t, fixtureAuthority().Permissions, fixtureSubjects())
	provenance := release.ProvenanceFor(candidate)
	if err := release.VerifyProvenance(candidate, provenance); err != nil {
		t.Fatal(err)
	}

	mutations := []func(*release.Provenance){
		func(p *release.Provenance) { p.Repository = "attacker/fork" },
		func(p *release.Provenance) { p.Workflow = "evil.yml" },
		func(p *release.Provenance) { p.WorkflowSHA = strings.Repeat("e", 64) },
		func(p *release.Provenance) { p.Ref = "refs/tags/v0.1.2" },
		func(p *release.Provenance) { p.Commit = strings.Repeat("d", 40) },
		func(p *release.Provenance) { p.Permissions[0].Access = "write" },
		func(p *release.Provenance) { p.Subjects[0].SHA256 = strings.Repeat("f", 64) },
		func(p *release.Provenance) { p.Subjects = p.Subjects[:1] },
		func(p *release.Provenance) {
			p.Subjects = append(p.Subjects, release.Subject{Name: "extra", SHA256: strings.Repeat("e", 64)})
		},
	}
	for i, mutate := range mutations {
		p := release.ProvenanceFor(candidate)
		mutate(&p)
		if err := release.VerifyProvenance(candidate, p); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
	}
}

func TestDraftLifecycleRecoveryAndPublication(t *testing.T) {
	candidate := fixtureCandidate(t, fixtureAuthority().Permissions, fixtureSubjects())
	tests := []struct {
		name     string
		releases []release.Release
		want     release.Lifecycle
		missing  []release.Subject
	}{
		{"absent", nil, release.ResumeDraft, candidate.Subjects},
		{"partial draft", []release.Release{{Version: candidate.Version, CandidateID: candidate.ID, Draft: true, Assets: []release.Subject{candidate.Subjects[1]}}}, release.ResumeDraft, []release.Subject{candidate.Subjects[0]}},
		{"complete draft order independent", []release.Release{{Version: candidate.Version, CandidateID: candidate.ID, Draft: true, Assets: reverseSubjects(candidate.Subjects)}}, release.PublishDraft, nil},
		{"complete published", []release.Release{{Version: candidate.Version, CandidateID: candidate.ID, Assets: candidate.Subjects}}, release.ContinuePublished, nil},
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
				t.Fatal("verification mutated observed releases")
			}
		})
	}
}

func TestDraftLifecycleRejectsDivergenceAmbiguityAndBadAssets(t *testing.T) {
	candidate := fixtureCandidate(t, fixtureAuthority().Permissions, fixtureSubjects())
	exact := release.Release{Version: candidate.Version, CandidateID: candidate.ID, Draft: true, Assets: candidate.Subjects}
	tests := []release.Release{
		{Version: candidate.Version, CandidateID: strings.Repeat("f", 64), Draft: true},
		{Version: candidate.Version, CandidateID: candidate.ID, Draft: true, Assets: []release.Subject{{Name: "packy_checksums.txt", SHA256: strings.Repeat("f", 64)}}},
		{Version: candidate.Version, CandidateID: candidate.ID, Draft: true, Assets: append(append([]release.Subject(nil), candidate.Subjects...), release.Subject{Name: "extra", SHA256: strings.Repeat("e", 64)})},
		{Version: candidate.Version, CandidateID: candidate.ID, Draft: true, Assets: []release.Subject{candidate.Subjects[0], candidate.Subjects[0]}},
		{Version: candidate.Version, CandidateID: candidate.ID, Draft: false, Assets: candidate.Subjects[:1]},
	}
	for i, observed := range tests {
		if _, err := release.VerifyLifecycle(candidate, []release.Release{observed}); err == nil {
			t.Fatalf("divergent state %d accepted", i)
		}
	}
	if _, err := release.VerifyLifecycle(candidate, []release.Release{exact, exact}); err == nil {
		t.Fatal("ambiguous duplicate releases accepted")
	}
	published := exact
	published.Draft = false
	if _, err := release.VerifyDraftPreparation(candidate, []release.Release{published}); err == nil {
		t.Fatal("draft preparation accepted an already-published release")
	}
	if err := release.VerifyPublishedContinuation(candidate, []release.Release{published}); err != nil {
		t.Fatalf("exact published continuation rejected: %v", err)
	}
	if err := release.VerifyPublishedContinuation(candidate, []release.Release{exact}); err == nil {
		t.Fatal("published continuation accepted a draft")
	}
}

func fixtureAuthority() release.Authorization {
	return release.Authorization{Repository: "yersonargotev/packy", Ref: "refs/heads/main", Workflow: ".github/workflows/release.yml", WorkflowSHA: strings.Repeat("d", 64), Permissions: []release.Permission{{Name: "actions", Access: "read"}, {Name: "contents", Access: "write"}}}
}

func fixtureSubjects() []release.Subject {
	return []release.Subject{{Name: "packy_checksums.txt", SHA256: strings.Repeat("a", 64)}, {Name: "packy_v0.1.2_linux_amd64.tar.gz", SHA256: strings.Repeat("b", 64)}}
}

func fixtureCandidate(t *testing.T, permissions []release.Permission, subjects []release.Subject) release.Candidate {
	t.Helper()
	a := fixtureAuthority()
	c, err := release.NewCandidate("v0.1.2", a.Repository, a.Ref, strings.Repeat("c", 40), a.Workflow, a.WorkflowSHA, permissions, subjects, a)
	if err != nil {
		t.Fatal(err)
	}
	return c
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
