#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

# Keep this list explicit. A new Matty-owned package must be deliberately added
# here before CI or the synchronization publisher can load or execute it.
readonly packages=(
  ./cmd/matty
  ./internal/bootstrap
  ./internal/capabilitypack
  ./internal/ci
  ./internal/cli
  ./internal/codex
  ./internal/corelifecycle
  ./internal/engrambin
  ./internal/localprojection
  ./internal/opencode
  ./internal/ownedcontainer
  ./internal/prompt
  ./internal/release
  ./internal/setuphealth
  ./internal/skillbundle
  ./internal/version
  ./internal/workstation
)

# Test-only contract packages remain in the suite above but have no production
# archive for `go build` to emit.
readonly build_packages=(
  ./cmd/matty
  ./internal/bootstrap
  ./internal/capabilitypack
  ./internal/cli
  ./internal/codex
  ./internal/corelifecycle
  ./internal/engrambin
  ./internal/localprojection
  ./internal/opencode
  ./internal/ownedcontainer
  ./internal/prompt
  ./internal/setuphealth
  ./internal/skillbundle
  ./internal/version
  ./internal/workstation
)

# Formatting is likewise limited to the package directories above. The glob is
# intentionally non-recursive so an unapproved nested package is not discovered.
readonly go_dirs=(
  cmd/matty
  internal/bootstrap
  internal/capabilitypack
  internal/ci
  internal/cli
  internal/codex
  internal/corelifecycle
  internal/engrambin
  internal/localprojection
  internal/opencode
  internal/ownedcontainer
  internal/prompt
  internal/release
  internal/setuphealth
  internal/skillbundle
  internal/version
  internal/workstation
)

# Tests that exercise workstation behavior must never inherit the operator's
# real configuration roots. Preserve only Go's caches across the sandbox.
go_cache="${GOCACHE:-$(go env GOCACHE)}"
go_mod_cache="${GOMODCACHE:-$(go env GOMODCACHE)}"
go_path="${GOPATH:-$(go env GOPATH)}"
sandbox="$(mktemp -d "${TMPDIR:-/tmp}/matty-validation.XXXXXX")"
trap 'rm -rf "$sandbox"' EXIT
export HOME="$sandbox/home"
export XDG_CONFIG_HOME="$sandbox/xdg"
export GOCACHE="$go_cache"
export GOMODCACHE="$go_mod_cache"
export GOPATH="$go_path"
mkdir -p "$HOME" "$XDG_CONFIG_HOME"

shopt -s nullglob
go_files=()
for dir in "${go_dirs[@]}"; do
  files=("$root/$dir"/*.go)
  if ((${#files[@]} == 0)); then
    echo "allowlisted Go directory has no Go files: $dir" >&2
    exit 1
  fi
  go_files+=("${files[@]}")
done

echo "==> formatting"
unformatted="$(gofmt -l "${go_files[@]}")"
if [[ -n "$unformatted" ]]; then
  echo "These Matty-owned files are not gofmt-clean:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> build"
go build "${build_packages[@]}"

echo "==> vet"
go vet "${packages[@]}"

echo "==> tests"
go test "${packages[@]}"

echo "==> race"
go test -race -timeout 10m "${packages[@]}"
