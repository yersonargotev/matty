#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

# This mirrors the exhaustive validator deliberately. A package is never
# inferred into the trusted set merely because a directory happens to exist.
readonly packages=(
  ./cmd/packy
  ./internal/addyacceptance
  ./internal/bootstrap
  ./internal/bundletransaction
  ./internal/capabilitypack
  ./internal/ci
  ./internal/cli
  ./internal/codex
  ./internal/corelifecycle
  ./internal/engrambin
  ./internal/localprojection
  ./internal/opencode
  ./internal/ownedcontainer
  ./internal/packclassification
  ./internal/packsync
  ./internal/packsync/githubsource
  ./internal/packsyncworkflow
  ./internal/prompt
  ./internal/release
  ./internal/setuphealth
  ./internal/skillbundle
  ./internal/tools/syncpacksource
  ./internal/version
  ./internal/workstation
)

exhaustive() {
  echo "mode=exhaustive"
  echo "reason=$1"
  echo "changed paths=${changed_paths[*]:-(unavailable)}"
  ./scripts/validate-packy.sh
  exit
}

base="${1:-origin/main}"
changed_paths=()
if ! base_commit="$(git rev-parse --verify "${base}^{commit}" 2>/dev/null)"; then
  exhaustive "base cannot be resolved: $base"
fi
if ! git merge-base --is-ancestor "$base_commit" HEAD; then
  exhaustive "base is not an ancestor of HEAD: $base"
fi

declare -a owners=() format_files=()
documentation=false
code=false
unsafe_reason=""
records="$(mktemp "${TMPDIR:-/tmp}/packy-changed.XXXXXX")"
untracked="$(mktemp "${TMPDIR:-/tmp}/packy-changed.XXXXXX")"
sandbox=""
cleanup() { rm -f "$records" "$untracked"; [[ -z "$sandbox" ]] || rm -rf "$sandbox"; }
trap cleanup EXIT

add_owner() {
  local path="$1" dir="${1%/*}" package
  [[ "$path" == */* ]] || { unsafe_reason="Go path has no allowlisted owner: $path"; return; }
  package="./$dir"
  local allowed
  for allowed in "${packages[@]}"; do
    if [[ "$package" == "$allowed" ]]; then
      owners+=("$allowed")
      return
    fi
  done
  unsafe_reason="Go path has unknown or new package owner: $path"
}

classify_path() {
  local path="$1" current="${2:-false}"
  changed_paths+=("$path")
  case "$path" in
    go.mod|go.sum|scripts/*|.github/workflows/*|workflows/*|schemas/*|bundle/*)
      unsafe_reason="cross-cutting or dependency path changed: $path" ;;
    README.md|AGENTS.md|docs/*) documentation=true ;;
    *.go)
      code=true
      add_owner "$path"
      if [[ "$current" == true && -e "$path" ]]; then format_files+=("$root/$path"); fi ;;
    *) unsafe_reason="unknown path changed: $path" ;;
  esac
}

if ! git diff --name-status -z --find-renames "$base_commit" -- >"$records"; then
  exhaustive "Git could not enumerate the base-to-working-tree delta"
fi
while IFS= read -r -d '' status; do
  [[ -n "$status" ]] || { unsafe_reason="malformed empty Git status"; break; }
  IFS= read -r -d '' first || { unsafe_reason="malformed Git change record: $status"; break; }
  case "$status" in
    A|M) classify_path "$first" true ;;
    D) classify_path "$first" false ;;
    R[0-9]*|C[0-9]*)
      IFS= read -r -d '' second || { unsafe_reason="malformed Git rename/copy record: $status"; break; }
      classify_path "$first" false
      classify_path "$second" true ;;
    *) unsafe_reason="unknown Git status: $status" ;;
  esac
done <"$records"

if ! git ls-files --others --exclude-standard -z >"$untracked"; then
  exhaustive "Git could not enumerate untracked paths"
fi
while IFS= read -r -d '' path; do classify_path "$path" true; done <"$untracked"

if [[ -n "$unsafe_reason" ]]; then exhaustive "$unsafe_reason"; fi

if ((${#changed_paths[@]} == 0)); then
  echo "mode=focused"
  echo "reason=no changes since $base_commit"
  echo "changed paths=(none)"
  echo "scope=empty"
  echo "package scope=(none)"
  echo "WARNING: ./scripts/validate-packy.sh remains required before final delivery."
  exit 0
fi

if [[ "$code" == false ]]; then
  echo "mode=focused"
  echo "reason=only documentation paths changed"
  printf 'changed paths=%s\n' "${changed_paths[*]}"
  echo "scope=documentation-only"
  echo "package scope=(none)"
  echo "WARNING: ./scripts/validate-packy.sh remains required before final delivery."
  exit 0
fi

# Match the exhaustive validator's isolation while retaining the caller's Go caches.
go_cache="${GOCACHE:-$(go env GOCACHE)}"
go_mod_cache="${GOMODCACHE:-$(go env GOMODCACHE)}"
go_path="${GOPATH:-$(go env GOPATH)}"
sandbox="$(mktemp -d "${TMPDIR:-/tmp}/packy-validation.XXXXXX")"
export HOME="$sandbox/home" XDG_CONFIG_HOME="$sandbox/xdg"
export GOCACHE="$go_cache" GOMODCACHE="$go_mod_cache" GOPATH="$go_path"
mkdir -p "$HOME" "$XDG_CONFIG_HOME"

for owner in "${owners[@]}"; do
  if ! go list "$owner" >/dev/null 2>&1; then exhaustive "changed package cannot be resolved: $owner"; fi
done

declare -a selected=()
for candidate in "${packages[@]}"; do
  if ! dependencies="$(go list -deps -test -f '{{.ImportPath}}' "$candidate" 2>&1)"; then
    exhaustive "dependency analysis failed for $candidate"
  fi
  candidate_selected=false
  for owner in "${owners[@]}"; do
    owner_import="github.com/yersonargotev/packy/${owner#./}"
    if [[ "$candidate" == "$owner" ]] || grep -Fxq "$owner_import" <<<"$dependencies"; then
      candidate_selected=true
    fi
  done
  [[ "$candidate_selected" == false ]] || selected+=("$candidate")
done

echo "mode=focused"
echo "reason=changed Go owners and their reverse dependents"
printf 'changed paths=%s\n' "${changed_paths[*]}"
if [[ "$documentation" == true ]]; then echo "scope=go and documentation"; else echo "scope=go"; fi
printf 'package scope=%s\n' "${selected[*]}"
if ((${#format_files[@]})); then
  gofmt -w "${format_files[@]}"
fi
go test "${selected[@]}"
echo "WARNING: ./scripts/validate-packy.sh remains required before final delivery."
