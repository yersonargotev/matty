#!/usr/bin/env bash

set -euo pipefail

if (($# != 3)); then
  echo "usage: $0 <base-ref> <head-ref> <new-evidence-directory>" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
base="$(git rev-parse --verify "$1^{commit}")"
head="$(git rev-parse --verify "$2^{commit}")"
[[ "$(git rev-parse HEAD)" == "$head" ]] || {
  echo "replay checkout does not match the exact protected-main head" >&2
  exit 1
}
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
  echo "replay requires a clean checkout" >&2
  exit 1
}

output="$3"
if [[ "$output" != /* ]]; then
  output="$(pwd)/$output"
fi
output="$(dirname "$output")/$(basename "$output")"
case "$output/" in
  "$root/"*) echo "replay evidence must be written outside the checkout" >&2; exit 1 ;;
esac
[[ ! -e "$output" ]] || {
  echo "replay evidence directory already exists" >&2
  exit 1
}
mkdir -m 700 "$output"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

matrix_rows="$output/matrix-inputs"
for path in \
  internal/addyacceptance/promotion.go \
  internal/addyacceptance/harness.go \
  scripts/validate-addy-acceptance.sh; do
  printf '%s %s\n' "$(sha256_file "$path")" "$path"
done > "$matrix_rows"
matrix_digest="$(sha256_file "$matrix_rows")"

run_acceptance() {
  local name="$1" sandbox="$output/$1"
  mkdir -m 700 "$sandbox"
  mkdir -m 700 "$sandbox/home" "$sandbox/xdg" "$sandbox/claude" "$sandbox/tmp" "$sandbox/go-cache"
  env \
    -u ANTHROPIC_API_KEY -u CLAUDE_API_KEY -u OPENAI_API_KEY -u GITHUB_TOKEN -u GH_TOKEN \
    HOME="$sandbox/home" \
    XDG_CONFIG_HOME="$sandbox/xdg" \
    CLAUDE_CONFIG_DIR="$sandbox/claude" \
    TMPDIR="$sandbox/tmp" \
    GOCACHE="$sandbox/go-cache" \
    ./scripts/validate-addy-acceptance.sh > "$sandbox/validation.log" 2>&1
  printf '%s\n' passed > "$sandbox/result"
}

run_acceptance first
run_acceptance second

[[ "$(git rev-parse HEAD)" == "$head" && -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
  echo "effect-free replay changed the checkout" >&2
  exit 1
}

workflow=.github/workflows/ci.yml
workflow_digest="$(sha256_file "$workflow")"
jq -cnS \
  --arg repository "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}" \
  --arg base "$base" \
  --arg head "$head" \
  --arg workflow "$workflow" \
  --arg workflow_digest "$workflow_digest" \
  --arg matrix_version "addy-claude-promotion.v1" \
  --arg matrix_digest "$matrix_digest" \
  --arg run_id "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}" \
  '{schema:"addy-promotion-main-replay.v1",disposition:"qualified",
    repository:$repository,base_sha:$base,head_sha:$head,
    workflow:$workflow,workflow_digest:$workflow_digest,
    matrix_version:$matrix_version,matrix_digest:$matrix_digest,run_id:$run_id,
    runs:[{id:"first",result:"passed"},{id:"second",result:"passed"}],
    identical_inputs:true,zero_checkout_mutation:true,production_admissible:false}' \
  > "$output/addy-promotion-main-replay.v1.json"
