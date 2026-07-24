package claudesmoke

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func validAddyQualification() AddyQualification {
	commit := strings.Repeat("a", 40)
	e := validEvidence()
	e.PackyRef, e.PackySHA, e.InstalledSourceSHA = commit, commit, commit
	processLog, _ := json.Marshal(e.Commands)
	processDigest := sha256.Sum256(processLog)
	return AddyQualification{
		SchemaVersion: 1, Repository: "yersonargotev/packy", Workflow: ".github/workflows/addy.yml",
		WorkflowDigest: strings.Repeat("1", 64), RunID: "12345", Commit: commit,
		Checkout: "/checkout", PackyExecutable: "/candidate/packy", PackyExecutableDigest: strings.Repeat("2", 64),
		InstalledSource: "/sandbox/installed-source", InstalledSourceCommit: commit, InstalledSourceClean: true,
		Sandbox: "/sandbox", WritableRoots: AddyWritableRoots{Home: "/sandbox/home", XDGConfig: "/sandbox/config", ClaudeConfig: "/sandbox/claude", State: "/sandbox/state", Package: "/sandbox/package", Repository: "/sandbox/repository", Acquisition: "/sandbox/acquisition"},
		ProcessLogDigest: hex.EncodeToString(processDigest[:]), Smoke: e,
	}
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
		{"duplicate writable root", func(q *AddyQualification) { q.WritableRoots.State = q.WritableRoots.Home }},
		{"go run", func(q *AddyQualification) { q.UsedGoRun = true }},
		{"development path", func(q *AddyQualification) { q.UsedDevelopmentPath = true }},
		{"direct fixture", func(q *AddyQualification) { q.UsedDirectFixture = true }},
		{"untracked input", func(q *AddyQualification) { q.UsedUntrackedInput = true }},
		{"authentication", func(q *AddyQualification) { q.Authenticated = true }},
		{"model", func(q *AddyQualification) { q.ModelInvoked = true }},
		{"print", func(q *AddyQualification) { q.PrintInvoked = true }},
		{"repl", func(q *AddyQualification) { q.REPLInvoked = true }},
		{"upstream execution", func(q *AddyQualification) { q.UpstreamExecuted = true }},
		{"credentials", func(q *AddyQualification) { q.CredentialsObserved = true }},
		{"outside write", func(q *AddyQualification) { q.OutsideWriteObserved = true }},
		{"workflow digest", func(q *AddyQualification) { q.WorkflowDigest = "bad" }},
		{"process log", func(q *AddyQualification) { q.ProcessLogDigest = "bad" }},
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
