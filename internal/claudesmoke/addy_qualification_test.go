package claudesmoke

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validAddyQualification() AddyQualification {
	commit := strings.Repeat("a", 40)
	e := validEvidence()
	e.PackyRef, e.PackySHA, e.InstalledSourceSHA = commit, commit, commit
	processLog, _ := json.Marshal(e.Commands)
	processDigest := sha256.Sum256(processLog)
	e.Qualification = AddyQualificationObservation{
		InstalledSource: "/sandbox/installed-source", InstalledSourceCommit: commit, InstalledSourceClean: true,
		WritableRoots:    AddyWritableRoots{Home: "/sandbox/home", XDGConfig: "/sandbox/config", ClaudeConfig: "/sandbox/home", State: "/sandbox/data", Package: "/sandbox/npm", Repository: "/sandbox/source-repository", Acquisition: "/sandbox/acquisition"},
		ProcessLogDigest: hex.EncodeToString(processDigest[:]), CollectedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Safety: AddyObservedSafety{NoGoRun: true, NoDevelopmentPath: true, NoDirectFixture: true, NoUntrackedInput: true, NoAuthentication: true, NoModelInvocation: true, NoPrint: true, NoREPL: true, NoUpstreamExecute: true, NoCredentials: true, NoOutsideWrite: true},
	}
	q, err := BindAddyQualification(AddyQualification{
		SchemaVersion: 1, Repository: "yersonargotev/packy", Workflow: ".github/workflows/addy.yml",
		WorkflowDigest: strings.Repeat("1", 64), RunID: "12345", Commit: commit,
		Checkout: "/checkout", PackyExecutable: "/candidate/packy", PackyExecutableDigest: strings.Repeat("2", 64),
	}, e)
	if err != nil {
		panic(err)
	}
	return q
}

func TestAddyQualificationProductionBoundary(t *testing.T) {
	q := validAddyQualification()
	if err := ValidateProductionAddyQualification(q); err != nil {
		t.Fatal(err)
	}
	q.Synthetic = true
	if err := ValidateAddyQualification(q); err != nil {
		t.Fatalf("synthetic harness qualification rejected: %v", err)
	}
	if err := ValidateProductionAddyQualification(q); err == nil {
		t.Fatal("synthetic qualification admitted to production")
	}
}

func TestAddyQualificationCanonicalOutput(t *testing.T) {
	q := validAddyQualification()
	one, err := CanonicalAddyQualificationJSON(q)
	if err != nil {
		t.Fatal(err)
	}
	two, err := CanonicalAddyQualificationJSON(q)
	if err != nil || string(one) != string(two) {
		t.Fatalf("non-deterministic output: %v", err)
	}
	var decoded AddyQualification
	if err := json.Unmarshal(one, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProductionAddyQualification(decoded); err != nil {
		t.Fatal(err)
	}
}

func TestBindAddyQualificationUsesInSandboxObservations(t *testing.T) {
	q := validAddyQualification()
	if q.WritableRoots.ClaudeConfig != "/sandbox/home" {
		t.Fatalf("CLAUDE_CONFIG_DIR mapping was invented: %q", q.WritableRoots.ClaudeConfig)
	}
	if q.WritableRoots.Repository != "/sandbox/source-repository" {
		t.Fatalf("repository root mapping was invented: %q", q.WritableRoots.Repository)
	}
	e := q.Smoke
	e.Qualification.InstalledSourceClean = false
	dirty, err := BindAddyQualification(AddyQualification{
		SchemaVersion: 1, Repository: q.Repository, Workflow: q.Workflow,
		WorkflowDigest: q.WorkflowDigest, RunID: q.RunID, Commit: q.Commit,
		Checkout: q.Checkout, PackyExecutable: q.PackyExecutable, PackyExecutableDigest: q.PackyExecutableDigest,
	}, e)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAddyQualification(dirty); err == nil {
		t.Fatal("qualification adapter certified a dirty Installed Source")
	}
	e.Qualification = AddyQualificationObservation{}
	if _, err := BindAddyQualification(AddyQualification{}, e); err == nil {
		t.Fatal("qualification adapter certified absent observations")
	}
}

func TestProductionAddyQualificationRejectsStaleCollection(t *testing.T) {
	q := validAddyQualification()
	q.CollectedAt = time.Now().UTC().Add(-25 * time.Hour).Format(time.RFC3339Nano)
	q.Smoke.Qualification.CollectedAt = q.CollectedAt
	if err := ValidateAddyQualification(q); err != nil {
		t.Fatalf("well-formed historical qualification rejected structurally: %v", err)
	}
	if err := ValidateProductionAddyQualification(q); err == nil {
		t.Fatal("stale qualification admitted to production")
	}
}

func TestAddyQualificationRejectsOneFactSafetyFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AddyQualification)
	}{
		{"packy in checkout", func(q *AddyQualification) { q.PackyExecutable = "/checkout/bin/packy" }},
		{"source in checkout", func(q *AddyQualification) { q.InstalledSource = "/checkout/source" }},
		{"sandbox in checkout", func(q *AddyQualification) { q.Sandbox = "/checkout/sandbox" }},
		{"source outside sandbox", func(q *AddyQualification) { q.InstalledSource = "/candidate/source" }},
		{"dirty source", func(q *AddyQualification) { q.InstalledSourceClean = false }},
		{"cross commit source", func(q *AddyQualification) { q.InstalledSourceCommit = strings.Repeat("b", 40) }},
		{"outside writable root", func(q *AddyQualification) { q.WritableRoots.State = "/state" }},
		{"go run unobserved", func(q *AddyQualification) { q.Safety.NoGoRun = false }},
		{"development path unobserved", func(q *AddyQualification) { q.Safety.NoDevelopmentPath = false }},
		{"direct fixture unobserved", func(q *AddyQualification) { q.Safety.NoDirectFixture = false }},
		{"untracked input unobserved", func(q *AddyQualification) { q.Safety.NoUntrackedInput = false }},
		{"authentication unobserved", func(q *AddyQualification) { q.Safety.NoAuthentication = false }},
		{"model unobserved", func(q *AddyQualification) { q.Safety.NoModelInvocation = false }},
		{"print unobserved", func(q *AddyQualification) { q.Safety.NoPrint = false }},
		{"repl unobserved", func(q *AddyQualification) { q.Safety.NoREPL = false }},
		{"upstream execution unobserved", func(q *AddyQualification) { q.Safety.NoUpstreamExecute = false }},
		{"credentials unobserved", func(q *AddyQualification) { q.Safety.NoCredentials = false }},
		{"outside write unobserved", func(q *AddyQualification) { q.Safety.NoOutsideWrite = false }},
		{"workflow digest", func(q *AddyQualification) { q.WorkflowDigest = "bad" }},
		{"process log", func(q *AddyQualification) { q.ProcessLogDigest = "bad" }},
		{"malformed collection time", func(q *AddyQualification) { q.CollectedAt = "today" }},
		{"invalid tag", func(q *AddyQualification) { q.Tag = "latest" }},
		{"requested Claude", func(q *AddyQualification) { q.Smoke.RequestedClaudeVersion = "latest" }},
		{"resolved Claude", func(q *AddyQualification) { q.Smoke.ResolvedClaudeVersion = "2.2.0" }},
		{"npm integrity", func(q *AddyQualification) { q.Smoke.ClaudeIntegrity = "" }},
		{"executable digest", func(q *AddyQualification) { q.Smoke.ClaudeDigest = "bad" }},
		{"argv", func(q *AddyQualification) { q.Smoke.Commands[0].Args = []string{"--print"} }},
		{"exit", func(q *AddyQualification) { q.Smoke.Commands[0].ExitCode = 1 }},
		{"filesystem before", func(q *AddyQualification) { q.Smoke.Before = nil }},
		{"filesystem after", func(q *AddyQualification) { q.Smoke.After = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := validAddyQualification()
			tc.mutate(&q)
			if err := ValidateAddyQualification(q); err == nil {
				t.Fatal("unsafe qualification accepted")
			}
		})
	}
}
