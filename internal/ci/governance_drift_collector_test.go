package ci_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGovernanceDriftCollectorIsReadOnlyAndSanitized(t *testing.T) {
	root := repositoryRoot(t)
	fake := filepath.Join(t.TempDir(), "gh")
	log := filepath.Join(t.TempDir(), "gh.log")
	writeFile(t, fake, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$GH_LOG"
endpoint="$2"; filter="$4"
case "$endpoint" in
  repos/*/actions/permissions/workflow) raw='{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}' ;;
  repos/*/actions/permissions) [[ "${FAIL_ACTIONS:-}" == 1 ]] && exit 1; raw='{"enabled":true,"allowed_actions":"selected","sha_pinning_required":true,"selected_actions_url":"https://secret"}' ;;
  repos/*/actions/workflows*) raw='{"workflows":[{"id":99,"name":"CI","path":".github/workflows/ci.yml","state":"active","url":"https://secret"}]}' ;;
  repos/*/branches/main/protection) raw='{"required_status_checks":{"strict":true,"checks":[{"context":"CI","app_id":15368}]},"required_pull_request_reviews":{"dismiss_stale_reviews":true,"require_code_owner_reviews":true,"required_approving_review_count":1,"require_last_push_approval":true},"enforce_admins":{"enabled":true},"required_conversation_resolution":{"enabled":true},"restrictions":null,"allow_force_pushes":{"enabled":false},"allow_deletions":{"enabled":false}}' ;;
  repos/*/rulesets/44) raw='{"id":44,"name":"tags","target":"tag","enforcement":"active","conditions":{"ref_name":{"include":["~ALL"],"exclude":[]}},"rules":[{"type":"deletion"}],"bypass_actors":[],"_links":{"self":{"href":"https://secret"}}}' ;;
  repos/*/rulesets*) raw='[{"id":44,"name":"tags","target":"tag","enforcement":"active","_links":{"self":{"href":"https://secret"}}}]' ;;
  repos/*/environments*) raw='{"total_count":0,"environments":[]}' ;;
  repos/*/actions/secrets*) raw='{"total_count":1,"secrets":[{"name":"TOKEN","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","value":"raw-secret","url":"https://secret"}]}' ;;
  repos/*/immutable-releases) raw='{"enabled":true,"enforced_by_owner":true}' ;;
  repos/*/releases/latest) raw='{"id":7,"tag_name":"v1","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-01-01T00:00:00Z","author":{"login":"owner","id":3,"avatar_url":"https://secret"},"assets":[{"id":8,"url":"https://secret"}]}' ;;
  repos/*) raw='{"visibility":"public","default_branch":"main","archived":false,"allow_merge_commit":false,"allow_squash_merge":true,"allow_rebase_merge":false,"allow_auto_merge":true,"delete_branch_on_merge":true,"web_commit_signoff_required":true,"token":"raw-secret","owner":{"avatar_url":"https://secret"}}' ;;
  *) exit 1 ;;
esac
printf '%s\n' "$raw" | jq -c "$filter"
`)
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, fail bool) map[string]any {
		t.Helper()
		out := filepath.Join(t.TempDir(), "observation.json")
		cmd := exec.Command("/bin/bash", filepath.Join(root, "scripts", "collect-governance-drift.sh"), "--repo", "owner/repo", "--ref", "refs/heads/main", "--commit", strings.Repeat("a", 40), "--workflow-sha", strings.Repeat("b", 40), "--output", out)
		cmd.Env = append(os.Environ(), "GH_BIN="+fake, "GH_LOG="+log)
		if fail {
			cmd.Env = append(cmd.Env, "FAIL_ACTIONS=1")
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("collector: %v\n%s", err, output)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "raw-secret") || strings.Contains(string(data), "avatar_url") || strings.Contains(string(data), "https://secret") || strings.Contains(string(data), `"id": 99`) || strings.Contains(string(data), `"id": 44`) {
			t.Fatalf("sensitive/raw field escaped projection:\n%s", data)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	clean := run(t, false)
	controls := clean["controls"].([]any)
	if len(controls) != 10 {
		t.Fatalf("controls=%d want 10", len(controls))
	}
	for _, raw := range controls {
		if raw.(map[string]any)["state"] != "observed" {
			t.Fatalf("unclean observation: %+v", raw)
		}
	}
	if !strings.Contains(mustJSON(t, controls), `"type":"deletion"`) {
		t.Fatalf("tag rule detail was not collected: %+v", controls)
	}
	failed := run(t, true)
	failures := 0
	for _, raw := range failed["controls"].([]any) {
		if raw.(map[string]any)["state"] == "collection-failure" {
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("collection failures=%d want 1", failures)
	}
	requests, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"--method", " -X ", "gh issue", "gh secret", "gh release", "PATCH", "POST", "PUT", "DELETE"} {
		if strings.Contains(string(requests), forbidden) {
			t.Fatalf("mutation-capable request %q:\n%s", forbidden, requests)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
