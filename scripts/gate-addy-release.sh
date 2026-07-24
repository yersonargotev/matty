#!/usr/bin/env bash

set -euo pipefail

if (($# < 2 || $# > 3)); then
  echo "usage: $0 <exact-tag> <commit> [promotion-evidence]" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

tag="$1"
commit="$(git rev-parse --verify "$2^{commit}")"
evidence="${3:-}"
[[ "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ ]] || {
  echo "exact release tag must be v0.x.y" >&2
  exit 2
}
tag_commit="$(git rev-parse --verify "${tag}^{commit}")"
[[ "$tag_commit" == "$commit" ]] || {
  echo "exact release tag and candidate commit diverge" >&2
  exit 1
}
base="$(git rev-parse --verify "${commit}^1")"

promotion_change=false
while IFS= read -r path; do
  case "$path" in
    bundle/packs/addy/pack.json | bundle/sources/addy.lock.json | bundle/history/addy/*)
      promotion_change=true
      ;;
  esac
done < <(git diff --name-only "$base" "$commit" --)

catalog_has_addy() {
  git show "$1:internal/capabilitypack/catalog.go" |
    awk '/^var initialCatalog = / { catalog=1 } catalog { print } catalog && /^}/ { exit }' |
    grep -Eq 'ID:[[:space:]]*"addy"([,}])'
}
if [[ "$promotion_change" == false ]] && ! catalog_has_addy "$base" && catalog_has_addy "$commit"; then
  promotion_change=true
fi
if [[ "$promotion_change" == true && -z "$evidence" ]]; then
  echo "exact-tag Addy promotion requires same-run evidence" >&2
  exit 1
fi
if [[ "$promotion_change" == false && -n "$evidence" ]]; then
  echo "candidate evidence is not accepted without an exact-tag Addy promotion" >&2
  exit 1
fi

workflow=.github/workflows/release.yml
if command -v sha256sum >/dev/null 2>&1; then
  workflow_digest="$(sha256sum "$workflow" | awk '{print $1}')"
else
  workflow_digest="$(shasum -a 256 "$workflow" | awk '{print $1}')"
fi

args=(
  --promotion-change="$promotion_change"
  --foundation-change=false
  --repository="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
  --tag="$tag"
  --base-sha="$base"
  --head-sha="$commit"
  --workflow="$workflow"
  --workflow-digest="$workflow_digest"
  --run-id="${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
)
if [[ -n "$evidence" ]]; then
  [[ -f "$evidence" && ! -L "$evidence" ]] || {
    echo "promotion evidence must be a regular same-run artifact" >&2
    exit 1
  }
  args+=(--evidence="$evidence")
fi

go run ./internal/tools/addypromotiongate "${args[@]}"
