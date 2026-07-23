package ci_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGovernanceDriftIssueProjectionRequiresExactOwnerClassification(t *testing.T) {
	root := repositoryRoot(t)
	fake := filepath.Join(t.TempDir(), "gh")
	digest := "sha256:" + strings.Repeat("a", 64)
	writeFile(t, fake, `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == issue && "$2" == list ]]; then
  cat <<'JSON'
[{"number":7,"state":"OPEN","body":"<!-- packy-governance-drift\nkey: packy-governance-drift-v1\nevidence: `+digest+`\nboundaries: publication\nclassification: unclassified\nowner: yersonargotev\n-->"}]
JSON
  exit 0
fi
if [[ "$1" == api ]]; then
  cat <<'JSON' | jq -c "$5"
[
  {"author_association":"CONTRIBUTOR","body":"<!-- packy-governance-classification\nevidence: `+digest+`\nclassification: reviewed\n-->"},
  {"author_association":"OWNER","body":"<!-- packy-governance-classification\nevidence: `+digest+`\nclassification: reviewed\n-->"}
]
JSON
  exit 0
fi
exit 1
`)
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "issues.json")
	cmd := exec.Command(filepath.Join(root, "scripts", "project-governance-drift-issues.sh"),
		"--repo", "owner/repo", "--output", output)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GH_BIN="+fake)
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("project issues: %v\n%s", err, combined)
	}
	var issues []struct {
		Number                       int      `json:"number"`
		Boundaries                   []string `json:"boundaries"`
		ExactEvidenceHumanClassified bool     `json:"exact_evidence_human_classified"`
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Number != 7 ||
		!issues[0].ExactEvidenceHumanClassified ||
		len(issues[0].Boundaries) != 1 || issues[0].Boundaries[0] != "publication" {
		t.Fatalf("projected issues = %+v", issues)
	}
}
