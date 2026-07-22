package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const qualificationHead = "0123456789abcdef0123456789abcdef01234567"
const qualificationBase = "89abcdef0123456789abcdef0123456789abcdef"

func TestGovernanceShadowQualificationContract(t *testing.T) {
	root := repositoryRoot(t)
	doc := readFile(t, filepath.Join(root, "docs", "governance", "shadow-qualification.md"))
	registry := readFile(t, filepath.Join(root, "docs", "governance", "advisory-checks.md"))
	if !strings.Contains(registry, "[Governance shadow qualification](shadow-qualification.md)") {
		t.Error("advisory registry does not link the qualification procedure")
	}
	for _, required := range []string{
		"authoritative final-HEAD record", "issue comment on #172", "Complete scenario matrix",
		"Owner PR", "Fork PR", "Explicitly delegated proposal", "Dependabot", "Synchronization",
		"Sensitive paths", "Approved issue link", "Unapproved/ambiguous link", "private-security",
		"urgent-revert", "automation", "New head", "Missing/wrong source check", "Excess permissions",
		"Unauthorized ref/environment/publication", "Pages", "Deterministic issue automation", "Installed Apps",
		"Approved substitutes", "Invalidation, reruns, and sign-off", "Only repository Owner `yersonargotev`",
	} {
		if !strings.Contains(doc, required) {
			t.Errorf("shadow qualification document lacks %q", required)
		}
	}

	script := readFile(t, filepath.Join(root, "scripts", "qualify-governance-shadow.sh"))
	for _, forbidden := range []string{"--method", "-X ", "gh pr ", "gh issue ", "gh secret ", "gh variable ", "gh release ", "gh workflow run"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("qualification collector contains mutation-capable token %q", forbidden)
		}
	}
}

func TestGovernanceShadowQualificationFailsClosed(t *testing.T) {
	root := repositoryRoot(t)
	fakeBin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh.log")
	writeQualificationGH(t, filepath.Join(fakeBin, "gh"))

	run := func(t *testing.T, head, source string, wantSuccess bool) {
		t.Helper()
		out := t.TempDir()
		cmd := exec.Command("/bin/bash", filepath.Join(root, "scripts", "qualify-governance-shadow.sh"),
			"--repo", "yersonargotev/packy", "--pr", "172", "--head", head, "--output-dir", out)
		cmd.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"), "GH_LOG="+logPath, "FIXTURE_MODE="+source)
		output, err := cmd.CombinedOutput()
		if wantSuccess && err != nil {
			t.Fatalf("qualification failed: %v\n%s", err, output)
		}
		if !wantSuccess && err == nil {
			t.Fatalf("qualification unexpectedly succeeded: %s", output)
		}
		if wantSuccess {
			for _, name := range []string{"qualification.json", "qualification.md"} {
				if _, err := os.Stat(filepath.Join(out, name)); err != nil {
					t.Fatalf("missing %s: %v", name, err)
				}
			}
		}
	}

	t.Run("status-only Governance and unqualified jobs succeed", func(t *testing.T) { run(t, qualificationHead, "valid", true) })
	t.Run("wrong head fails", func(t *testing.T) { run(t, strings.Repeat("a", 40), "valid", false) })
	t.Run("wrong ordinary source fails", func(t *testing.T) { run(t, qualificationHead, "wrong-check-source", false) })
	t.Run("wrong Governance source fails", func(t *testing.T) { run(t, qualificationHead, "wrong-governance-source", false) })
	t.Run("missing Governance status fails", func(t *testing.T) { run(t, qualificationHead, "missing-governance", false) })
	t.Run("historical Governance reruns use latest", func(t *testing.T) { run(t, qualificationHead, "historical-governance", true) })
	t.Run("forged Governance publisher fails", func(t *testing.T) { run(t, qualificationHead, "forged-status-creator", false) })
	t.Run("renamed workflow fails", func(t *testing.T) { run(t, qualificationHead, "renamed-workflow", false) })
	t.Run("missing current check fails", func(t *testing.T) { run(t, qualificationHead, "missing-check", false) })
	t.Run("duplicate current check fails", func(t *testing.T) { run(t, qualificationHead, "duplicate-check", false) })
	t.Run("closed PR fails", func(t *testing.T) { run(t, qualificationHead, "closed-pr", false) })
	t.Run("wrong base fails", func(t *testing.T) { run(t, qualificationHead, "wrong-base", false) })
	t.Run("non-successful run fails", func(t *testing.T) { run(t, qualificationHead, "failed-run", false) })

	log := readFile(t, logPath)
	if strings.Contains(log, "--method") || strings.Contains(log, " -X ") {
		t.Fatalf("collector made a method-bearing request:\n%s", log)
	}
	if !strings.Contains(log, "/commits/"+qualificationHead+"/statuses?per_page=100 --jq") {
		t.Fatalf("collector did not read projected status history:\n%s", log)
	}
	if !strings.Contains(log, "/contents/.github/workflows/governance.yml?ref="+qualificationBase) {
		t.Fatalf("collector did not bind Governance definition to the trusted base:\n%s", log)
	}
}

func writeQualificationGH(t *testing.T, path string) {
	t.Helper()
	fixture := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$GH_LOG"
endpoint="$2"
case "$endpoint" in
  repos/*/pulls/172)
    state=open; base_ref=main
    [[ "${FIXTURE_MODE:-valid}" == closed-pr ]] && state=closed
    [[ "${FIXTURE_MODE:-valid}" == wrong-base ]] && base_ref=develop
    printf '{"number":172,"state":"%s","head":{"sha":"` + qualificationHead + `","ref":"feat","repo":"fork/packy"},"base":{"ref":"%s","sha":"` + qualificationBase + `"}}\n' "$state" "$base_ref" ;;
  *commits/*/check-runs*)
    slug=github-actions
    [[ "${FIXTURE_MODE:-valid}" == wrong-check-source ]] && slug=untrusted
    if [[ "${FIXTURE_MODE:-valid}" == missing-check ]]; then printf '[]\n'; exit 0; fi
    if [[ "${FIXTURE_MODE:-valid}" == duplicate-check ]]; then
      printf '[{"name":"Validate Packy-owned code","status":"completed","conclusion":"success","details_url":"https://github.com/x/actions/runs/101/job/1","app":{"id":15368,"slug":"github-actions"}},{"name":"Validate Packy-owned code","status":"completed","conclusion":"success","details_url":"https://github.com/x/actions/runs/101/job/2","app":{"id":15368,"slug":"github-actions"}}]\n'
      exit 0
    fi
    cat <<JSON
[{"name":"Validate Packy-owned code","status":"completed","conclusion":"success","details_url":"https://github.com/x/actions/runs/101/job/1","app":{"id":15368,"slug":"$slug"}},{"name":"Claude 2.1.203 package smoke","status":"completed","conclusion":"success","details_url":"https://github.com/x/actions/runs/102/job/2","app":{"id":15368,"slug":"github-actions"}},{"name":"CodeQL","status":"completed","conclusion":"success","details_url":"https://github.com/x/actions/runs/104/job/4","app":{"id":15368,"slug":"github-actions"}},{"name":"Dependency review","status":"completed","conclusion":"success","details_url":"https://github.com/x/actions/runs/105/job/5","app":{"id":15368,"slug":"github-actions"}}]
JSON
    ;;
  *commits/*/statuses*)
    case "${FIXTURE_MODE:-valid}" in
      missing-governance) printf '[]\n' ;;
      historical-governance) printf '[{"id":7,"context":"Governance / Validate authorization","state":"failure","target_url":"https://github.com/x/actions/runs/99","creator":{"login":"github-actions[bot]","id":41898282,"type":"Bot","html_url":"https://github.com/apps/github-actions"}},{"id":9,"context":"Governance / Validate authorization","state":"success","target_url":"https://github.com/x/actions/runs/103","creator":{"login":"github-actions[bot]","id":41898282,"type":"Bot","html_url":"https://github.com/apps/github-actions"}}]\n' ;;
      forged-status-creator) printf '[{"id":9,"context":"Governance / Validate authorization","state":"success","target_url":"https://github.com/x/actions/runs/103","creator":{"login":"attacker[bot]","id":1,"type":"Bot","html_url":"https://github.com/apps/attacker"}}]\n' ;;
      *) printf '[{"id":9,"context":"Governance / Validate authorization","state":"success","target_url":"https://github.com/x/actions/runs/103","creator":{"login":"github-actions[bot]","id":41898282,"type":"Bot","html_url":"https://github.com/apps/github-actions","avatar_url":"https://avatars.githubusercontent.com/in/15368?v=4"}}]\n' ;;
    esac ;;
  */actions/runs/101|*/actions/runs/102)
    name=CI
    [[ "${FIXTURE_MODE:-valid}" == renamed-workflow ]] && name='Renamed CI'
    printf '{"id":%s,"name":"%s","path":".github/workflows/ci.yml","head_sha":"` + qualificationHead + `","status":"completed","conclusion":"success"}\n' "${endpoint##*/}" "$name" ;;
  */actions/runs/103)
    conclusion=success
    [[ "${FIXTURE_MODE:-valid}" == failed-run ]] && conclusion=failure
    printf '{"id":103,"name":"Governance","path":".github/workflows/governance.yml","head_sha":"` + qualificationHead + `","status":"completed","conclusion":"%s","check_suite_id":999}\n' "$conclusion" ;;
  */actions/runs/104|*/actions/runs/105) printf '{"id":%s,"name":"Security","path":".github/workflows/security-pr.yml","head_sha":"` + qualificationHead + `","status":"completed","conclusion":"success"}\n' "${endpoint##*/}" ;;
  */check-suites/999/check-runs*)
    slug=github-actions
    [[ "${FIXTURE_MODE:-valid}" == wrong-governance-source ]] && slug=untrusted
    printf '[{"name":"Validate authorization metadata (182)","status":"completed","conclusion":"success","details_url":"https://github.com/x/actions/runs/103/job/9","app":{"id":15368,"slug":"%s"}}]\n' "$slug" ;;
  */contents/*) definition="${endpoint#*/contents/}"; definition="${definition%%\?*}"; printf '{"path":"%s","sha":"blob-%s","type":"file"}\n' "$definition" "${definition##*/}" ;;
  *) echo "unexpected endpoint: $endpoint" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(path, []byte(fixture), 0o755); err != nil {
		t.Fatal(err)
	}
}
