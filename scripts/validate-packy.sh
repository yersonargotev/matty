#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

# Keep this list explicit. A new Packy-owned package must be deliberately added
# here before CI or the synchronization publisher can load or execute it.
readonly packages=(
  ./cmd/packy
  ./internal/addyacceptance
  ./internal/bootstrap
  ./internal/bundletransaction
  ./internal/capabilitypack
  ./internal/ci
  ./internal/cli
  ./internal/claudesmoke
  ./internal/codex
  ./internal/corelifecycle
  ./internal/engrambin
  ./internal/governanceauth
  ./internal/governancedrift
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
  ./internal/tools/claudesmoke
  ./internal/tools/governanceauth
  ./internal/tools/governancedrift
  ./internal/tools/syncpacksource
  ./internal/version
  ./internal/workstation
)

# Derive formatting paths and the build subset from the one package authority.
# The glob below is intentionally non-recursive. Test-only contract packages
# remain in vet/test/race but have no production archive for `go build` to emit.
go_dirs=()
build_packages=()
race_packages=()
for package in "${packages[@]}"; do
  go_dirs+=("${package#./}")
  case "$package" in
    ./internal/ci | ./internal/release) ;;
    *) build_packages+=("$package") ;;
  esac
  # Release is a test-only subprocess, cross-platform, and package-install
  # integration package. Its child commands are not race-instrumented, so the
  # ordinary exhaustive test phase covers it while the race phase excludes it.
  case "$package" in
    ./internal/release) ;;
    *) race_packages+=("$package") ;;
  esac
done

# Tests that exercise workstation behavior must never inherit the operator's
# real configuration roots. Preserve only Go's caches across the sandbox.
go_cache="${GOCACHE:-$(go env GOCACHE)}"
go_mod_cache="${GOMODCACHE:-$(go env GOMODCACHE)}"
go_path="${GOPATH:-$(go env GOPATH)}"
sandbox="$(mktemp -d "${TMPDIR:-/tmp}/packy-validation.XXXXXX")"
trap 'rm -rf "$sandbox"' EXIT
export HOME="$sandbox/home"
export XDG_CONFIG_HOME="$sandbox/xdg"
export GOCACHE="$go_cache"
export GOMODCACHE="$go_mod_cache"
export GOPATH="$go_path"
mkdir -p "$HOME" "$XDG_CONFIG_HOME"

./scripts/validate-addy-acceptance.sh

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
  echo "These Packy-owned files are not gofmt-clean:" >&2
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
go test -race -timeout 10m "${race_packages[@]}"
