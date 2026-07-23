package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluateAndGateCleanCurrentEvidence(t *testing.T) {
	root := t.TempDir()
	sha := strings.Repeat("a", 40)
	workflow := strings.Repeat("b", 40)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	contract := writeFixture(t, root, "contract.json", `{
  "schema_version": 1,
  "controls": [{"id":"main","boundaries":["promotion"],"expected":{"protected":true}}]
}`)
	observation := writeFixture(t, root, "observation.json", `{
  "schema_version": 1,
  "identity": {
    "repository": "yersonargotev/packy",
    "ref": "refs/heads/main",
    "commit_sha": "`+sha+`",
    "workflow_sha": "`+workflow+`",
    "collected_at": "`+now.Format(time.RFC3339)+`"
  },
  "controls": [{"id":"main","state":"observed","actual":{"protected":true}}]
}`)
	evaluation := filepath.Join(root, "evaluation.json")
	blockingIssues := writeFixture(t, root, "blocking-issues.json", `[]`)
	if err := run([]string{
		"--mode", "evaluate",
		"--contract", contract,
		"--observation", observation,
		"--output", evaluation,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var gate bytes.Buffer
	if err := run([]string{
		"--mode", "gate",
		"--evaluation", evaluation,
		"--blocking-issues", blockingIssues,
		"--boundary", "promotion",
		"--repository", "yersonargotev/packy",
		"--ref", "refs/heads/main",
		"--commit", sha,
		"--workflow-sha", workflow,
		"--now", now.Add(time.Hour).Format(time.RFC3339),
		"--max-age", "192h",
	}, &gate); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gate.String(), `"allowed": true`) {
		t.Fatalf("gate output = %s", gate.String())
	}
}

func TestGateReturnsDecisionAndErrorForAffectedDrift(t *testing.T) {
	root := t.TempDir()
	sha := strings.Repeat("a", 40)
	workflow := strings.Repeat("b", 40)
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	evaluation := writeFixture(t, root, "evaluation.json", `{
  "identity": {
    "repository": "yersonargotev/packy",
    "ref": "refs/heads/main",
    "commit_sha": "`+sha+`",
    "workflow_sha": "`+workflow+`",
    "collected_at": "`+now.Format(time.RFC3339)+`"
  },
  "state": "confirmed-drift",
  "findings": [{
    "control_id": "release",
    "state": "confirmed-drift",
    "boundaries": ["publication"],
    "expected": {"immutable":true},
    "observed": {"immutable":false}
  }]
}`)
	blockingIssues := writeFixture(t, root, "blocking-issues.json", `[]`)
	var output bytes.Buffer
	err := run([]string{
		"--mode", "gate",
		"--evaluation", evaluation,
		"--blocking-issues", blockingIssues,
		"--boundary", "publication",
		"--repository", "yersonargotev/packy",
		"--ref", "refs/heads/main",
		"--commit", sha,
		"--workflow-sha", workflow,
		"--now", now.Format(time.RFC3339),
	}, &output)
	if err == nil || !strings.Contains(output.String(), `"allowed": false`) {
		t.Fatalf("err=%v output=%s", err, output.String())
	}
}

func writeFixture(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
