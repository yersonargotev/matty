#!/usr/bin/env bash

set -euo pipefail

if (($# != 2)); then
  echo "usage: $0 <base-ref> <head-ref>" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

base_sha="$(git rev-parse --verify "$1^{commit}")"
head_sha="$(git rev-parse --verify "$2^{commit}")"
promotion_change=false
foundation_change=false
base_has_promotion_gate=false
if git show "$base_sha:.github/workflows/ci.yml" 2>/dev/null | grep -Fq 'addy-promotion-gate:'; then
  base_has_promotion_gate=true
fi

while IFS= read -r path; do
  case "$path" in
    bundle/packs/addy/pack.json | bundle/sources/addy.lock.json | bundle/history/addy/*)
      promotion_change=true
      ;;
    .github/workflows/ci.yml | internal/addyacceptance/promotion.go | internal/addyacceptance/promotion_test.go | internal/capabilitypack/catalog.go | internal/capabilitypack/manifest_v3_test.go | internal/ci/validation_test.go | internal/ci/workflow_trust_boundaries_test.go | internal/claudecode/surface.go | internal/claudecode/capabilitypack_runtime_test.go | internal/tools/addypromotiongate/* | scripts/gate-addy-promotion.sh | scripts/validate-addy-acceptance.sh)
      if [[ "$base_has_promotion_gate" == true ]]; then
        foundation_change=true
      fi
      ;;
  esac
done < <(git diff --name-only "$base_sha" "$head_sha" --)

catalog_has_addy() {
  git show "$1:internal/capabilitypack/catalog.go" |
    awk '/^var initialCatalog = / { catalog=1 } catalog { print } catalog && /^}/ { exit }' |
    grep -Eq 'ID:[[:space:]]*"addy"([,}])'
}

if [[ "$promotion_change" == false ]] && ! catalog_has_addy "$base_sha" && catalog_has_addy "$head_sha"; then
  promotion_change=true
fi
if [[ "$promotion_change" == true ]]; then
  foundation_change=false
elif [[ "$foundation_change" == true ]]; then
  ./scripts/validate-addy-acceptance.sh
fi

workflow=.github/workflows/ci.yml
if command -v sha256sum >/dev/null 2>&1; then
  workflow_digest="$(sha256sum "$workflow" | awk '{print $1}')"
else
  workflow_digest="$(shasum -a 256 "$workflow" | awk '{print $1}')"
fi

args=(
  --promotion-change="$promotion_change"
  --foundation-change="$foundation_change"
  --repository="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
  --pull-request="${GITHUB_PR_NUMBER:?GITHUB_PR_NUMBER is required}"
  --base-sha="$base_sha"
  --head-sha="$head_sha"
  --evaluated-merge-sha="${GITHUB_SHA:?GITHUB_SHA is required}"
  --workflow="$workflow"
  --workflow-digest="$workflow_digest"
  --run-id="${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
)
if [[ "$promotion_change" == true && -n "${ADDY_PROMOTION_EVIDENCE:-}" ]]; then
  args+=(--evidence="$ADDY_PROMOTION_EVIDENCE")
fi

go run ./internal/tools/addypromotiongate "${args[@]}"
