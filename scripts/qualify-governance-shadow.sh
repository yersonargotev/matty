#!/usr/bin/env bash

set -euo pipefail

usage() {
  echo "usage: $0 --repo OWNER/REPO --pr NUMBER --head SHA --output-dir DIRECTORY" >&2
  exit 2
}

repo= pr= head= output_dir=
while (($#)); do
  case "$1" in
    --repo) repo="${2:-}"; shift 2 ;;
    --pr) pr="${2:-}"; shift 2 ;;
    --head) head="${2:-}"; shift 2 ;;
    --output-dir) output_dir="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || usage
[[ "$pr" =~ ^[1-9][0-9]*$ ]] || usage
[[ "$head" =~ ^[0-9a-f]{40}$ ]] || usage
[[ -n "$output_dir" ]] || usage

command -v gh >/dev/null || { echo "gh is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

tmp="$(mktemp -d "${TMPDIR:-/tmp}/packy-shadow-qualification.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

# This script deliberately has no method-bearing gh invocation. Every request is
# a projected GET; raw API responses are retained only in the temporary folder.
gh api "repos/$repo/pulls/$pr" --jq '{number,state,head:{sha:.head.sha,ref:.head.ref,repo:.head.repo.full_name},base:{ref:.base.ref,sha:.base.sha},html_url}' >"$tmp/pr.json"
actual_head="$(jq -r '.head.sha // empty' "$tmp/pr.json")"
[[ "$actual_head" == "$head" ]] || { echo "qualification stopped: PR head does not match --head" >&2; exit 1; }
[[ "$(jq -r '.state // empty' "$tmp/pr.json")" == open ]] || { echo "qualification stopped: PR is not open" >&2; exit 1; }
[[ "$(jq -r '.base.ref // empty' "$tmp/pr.json")" == main ]] || { echo "qualification stopped: PR does not target main" >&2; exit 1; }
base_sha="$(jq -r '.base.sha // empty' "$tmp/pr.json")"
[[ "$base_sha" =~ ^[0-9a-f]{40}$ ]] || { echo "qualification stopped: PR base SHA is unavailable" >&2; exit 1; }
head_repo="$(jq -r '.head.repo // empty' "$tmp/pr.json")"
[[ "$head_repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "qualification stopped: candidate repository is unavailable" >&2; exit 1; }

gh api "repos/$repo/commits/$head/check-runs?filter=latest&per_page=100" --jq '[.check_runs[]|{name,status,conclusion,details_url,app:{id:.app.id,slug:.app.slug}}]' >"$tmp/checks.json"
gh api "repos/$repo/commits/$head/status" --jq '[.statuses[]|{context,state,target_url,created_at,updated_at}]' >"$tmp/statuses.json"

readonly names=(
  'Validate Packy-owned code'
  'Claude 2.1.203 package smoke'
  'CodeQL'
  'Dependency review'
)
readonly qualified_names=(
  'CI / Validate Packy-owned code'
  'CI / Claude 2.1.203 package smoke'
  'Security / CodeQL'
  'Security / Dependency review'
)
readonly paths=(
  '.github/workflows/ci.yml'
  '.github/workflows/ci.yml'
  '.github/workflows/security-pr.yml'
  '.github/workflows/security-pr.yml'
)

printf '[]\n' >"$tmp/qualified.json"
for i in "${!names[@]}"; do
  name="${names[$i]}"
  qualified_name="${qualified_names[$i]}"
  path="${paths[$i]}"
  count="$(jq --arg name "$name" '[.[]|select(.name==$name)]|length' "$tmp/checks.json")"
  [[ "$count" == 1 ]] || { echo "qualification stopped: expected one current check named $name" >&2; exit 1; }
  check="$(jq -c --arg name "$name" '.[]|select(.name==$name)' "$tmp/checks.json")"
  [[ "$(jq -r '.app.id' <<<"$check")" == 15368 && "$(jq -r '.app.slug' <<<"$check")" == github-actions ]] || {
    echo "qualification stopped: wrong source for $name" >&2; exit 1;
  }
  [[ "$(jq -r '.status' <<<"$check")" == completed && "$(jq -r '.conclusion' <<<"$check")" == success ]] || {
    echo "qualification stopped: $name is not a completed success" >&2; exit 1;
  }
  details="$(jq -r '.details_url // empty' <<<"$check")"
  [[ "$details" =~ /actions/runs/([0-9]+)(/job/[0-9]+)?$ ]] || { echo "qualification stopped: unrecognized workflow URL for $name" >&2; exit 1; }
  run_id="${BASH_REMATCH[1]}"
  gh api "repos/$repo/actions/runs/$run_id" --jq '{id,path,head_sha,event,status,conclusion}' >"$tmp/run.json"
  [[ "$(jq -r '.path' "$tmp/run.json")" == "$path" && "$(jq -r '.head_sha' "$tmp/run.json")" == "$head" && \
    "$(jq -r '.status' "$tmp/run.json")" == completed && "$(jq -r '.conclusion' "$tmp/run.json")" == success ]] || {
    echo "qualification stopped: stale or wrong workflow for $name" >&2; exit 1;
  }
  gh api "repos/$repo/contents/$path?ref=$head" --jq '{path,sha,type}' >"$tmp/definition.json"
  [[ "$(jq -r '.path' "$tmp/definition.json")" == "$path" && "$(jq -r '.type' "$tmp/definition.json")" == file ]] || {
    echo "qualification stopped: workflow definition unavailable for $name" >&2; exit 1;
  }
  jq --argjson check "$check" --arg qualified_name "$qualified_name" --slurpfile run "$tmp/run.json" --slurpfile definition "$tmp/definition.json" \
    '. + [$check + {name:$qualified_name,job_name:$check.name,workflow:{path:$run[0].path,run_id:$run[0].id,definition_ref:$run[0].head_sha,definition_sha:$definition[0].sha}}]' \
    "$tmp/qualified.json" >"$tmp/next.json"
  mv "$tmp/next.json" "$tmp/qualified.json"
done

governance='Governance / Validate authorization'
count="$(jq --arg context "$governance" '[.[]|select(.context==$context)]|length' "$tmp/statuses.json")"
[[ "$count" == 1 ]] || { echo "qualification stopped: expected one current status named $governance" >&2; exit 1; }
status="$(jq -c --arg context "$governance" '.[]|select(.context==$context)' "$tmp/statuses.json")"
[[ "$(jq -r '.state' <<<"$status")" == success ]] || { echo "qualification stopped: $governance is not successful" >&2; exit 1; }
target="$(jq -r '.target_url // empty' <<<"$status")"
[[ "$target" =~ /actions/runs/([0-9]+)(/job/[0-9]+)?$ ]] || { echo "qualification stopped: unrecognized Governance workflow URL" >&2; exit 1; }
run_id="${BASH_REMATCH[1]}"
gh api "repos/$repo/actions/runs/$run_id" --jq '{id,path,head_sha,event,status,conclusion,check_suite_id}' >"$tmp/run.json"
[[ "$(jq -r '.path' "$tmp/run.json")" == .github/workflows/governance.yml && "$(jq -r '.head_sha' "$tmp/run.json")" == "$head" && \
  "$(jq -r '.status' "$tmp/run.json")" == completed && "$(jq -r '.conclusion' "$tmp/run.json")" == success ]] || {
  echo "qualification stopped: stale or wrong workflow for $governance" >&2; exit 1;
}
suite_id="$(jq -r '.check_suite_id // empty' "$tmp/run.json")"
[[ "$suite_id" =~ ^[1-9][0-9]*$ ]] || { echo "qualification stopped: Governance run has no check suite" >&2; exit 1; }
gh api "repos/$repo/check-suites/$suite_id/check-runs?per_page=100" --jq '[.check_runs[]|{name,status,conclusion,details_url,app:{id:.app.id,slug:.app.slug}}]' >"$tmp/governance-checks.json"
source_count="$(jq --arg run "/actions/runs/$run_id/" '[.[]|select(.app.id==15368 and .app.slug=="github-actions" and (.details_url|contains($run)))]|length' "$tmp/governance-checks.json")"
((source_count >= 1)) || { echo "qualification stopped: Governance run has no job from the expected source" >&2; exit 1; }
gh api "repos/$repo/contents/.github/workflows/governance.yml?ref=$base_sha" --jq '{path,sha,type}' >"$tmp/definition.json"
[[ "$(jq -r '.path' "$tmp/definition.json")" == .github/workflows/governance.yml && "$(jq -r '.type' "$tmp/definition.json")" == file ]] || {
  echo "qualification stopped: Governance workflow definition unavailable" >&2; exit 1;
}
jq --argjson status "$status" --slurpfile run "$tmp/run.json" --slurpfile definition "$tmp/definition.json" \
  --arg base_sha "$base_sha" \
  '. + [{name:"Governance / Validate authorization",status_kind:"commit-status",status:$status.state,target_url:$status.target_url,source:{id:15368,slug:"github-actions"},workflow:{path:$run[0].path,run_id:$run[0].id,definition_ref:$base_sha,definition_sha:$definition[0].sha}}]' \
  "$tmp/qualified.json" >"$tmp/next.json"
mv "$tmp/next.json" "$tmp/qualified.json"

# Re-read after all observations so a synchronize event cannot leave apparently
# complete evidence for a head that ceased to be current during collection.
gh api "repos/$repo/pulls/$pr" --jq '{state,head:{sha:.head.sha},base:{ref:.base.ref,sha:.base.sha}}' >"$tmp/final-head.json"
[[ "$(jq -r '.head.sha' "$tmp/final-head.json")" == "$head" && "$(jq -r '.state' "$tmp/final-head.json")" == open && \
  "$(jq -r '.base.ref' "$tmp/final-head.json")" == main && "$(jq -r '.base.sha' "$tmp/final-head.json")" == "$base_sha" ]] || {
  echo "qualification stopped: PR identity changed during collection" >&2; exit 1;
}

mkdir -p "$output_dir"
observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
jq -n --arg schema packy-governance-shadow-v1 --arg repo "$repo" --argjson pr "$pr" --arg head "$head" --arg head_repo "$head_repo" --arg base_sha "$base_sha" \
  --arg observed_at "$observed_at" --slurpfile checks "$tmp/qualified.json" \
  '{schema:$schema,repository:$repo,pull_request:$pr,candidate:{head:$head,repository:$head_repo},base:{ref:"main",sha:$base_sha},observed_at:$observed_at,result:"qualified",checks:$checks[0]}' \
  >"$output_dir/qualification.json"

{
  echo '<!-- packy-governance-shadow-v1 -->'
  echo '## Governance shadow qualification'
  printf -- '- Repository: `%s`\n- Pull request: #%s\n- Head: `%s`\n- Observed (UTC): `%s`\n- Result: **qualified**\n\n' "$repo" "$pr" "$head" "$observed_at"
  echo '| Context | Workflow | Definition | Source | Result |'
  echo '| --- | --- | --- | --- | --- |'
  jq -r '.[]|"| `\(.name)` | `\(.workflow.path)` | `\(.workflow.definition_sha)` | `github-actions` (App 15368) | `\(.conclusion // .status)` |"' "$tmp/qualified.json"
  echo
  echo 'This is local, read-only evidence. Scenario evidence and Owner promotion sign-off remain separate and mandatory.'
} >"$output_dir/qualification.md"
