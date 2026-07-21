#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Run the hermetic package-installed Packy smoke against real Claude Code.

Usage:
  scripts/run-claude-smoke.sh \
    --claude-version <2.1.203|stable> \
    --packy-ref <tag-or-full-sha> \
    --evidence-dir <directory> \
    [--packy-binary <corresponding-release-artifact>]

The runner acquires Claude before entering its restricted environment. It never
uses operator credentials, authenticates, starts a model session, or writes to
the operator's HOME/config roots.
EOF
}

claude_version=""
packy_ref=""
evidence_dir=""
packy_binary=""
while (($#)); do
  case "$1" in
    --claude-version)
      claude_version="${2:-}"
      shift 2
      ;;
    --packy-ref)
      packy_ref="${2:-}"
      shift 2
      ;;
    --evidence-dir)
      evidence_dir="${2:-}"
      shift 2
      ;;
    --packy-binary)
      packy_binary="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "$claude_version" != "2.1.203" && "$claude_version" != "stable" ]]; then
  echo "--claude-version must be 2.1.203 or stable" >&2
  exit 2
fi
if [[ -z "$packy_ref" || -z "$evidence_dir" ]]; then
  echo "--packy-ref and --evidence-dir are required" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
resolved_ref="$(git -C "$root" rev-parse --verify "${packy_ref}^{commit}")"
head="$(git -C "$root" rev-parse HEAD)"
if [[ "$resolved_ref" != "$head" ]]; then
  echo "Packy smoke ref $packy_ref resolves to $resolved_ref, but checkout HEAD is $head" >&2
  exit 1
fi

evidence_dir="$(mkdir -p "$evidence_dir" && cd "$evidence_dir" && pwd)"
build_root="$(mktemp -d "${TMPDIR:-/tmp}/packy-claude-build.XXXXXX")"
trap 'rm -rf "$build_root"' EXIT

if [[ -n "$packy_binary" ]]; then
  packy_binary="$(cd "$(dirname "$packy_binary")" && pwd)/$(basename "$packy_binary")"
  if [[ ! -x "$packy_binary" ]]; then
    echo "--packy-binary must name an executable package artifact" >&2
    exit 2
  fi
else
  packy_version="$packy_ref"
  if [[ ! "$packy_version" =~ ^v0\.[0-9]+\.[0-9]+$ ]]; then
    packy_version="v0.0.0-smoke-${resolved_ref:0:12}"
  fi
  packy_binary="$build_root/packy"
fi

(
  cd "$root"
  if [[ ! -x "$packy_binary" ]]; then
    go build \
      -ldflags "-X github.com/yersonargotev/packy/internal/version.Value=$packy_version" \
      -o "$packy_binary" \
      ./cmd/packy
  fi
  go run ./internal/tools/claudesmoke \
    --packy "$packy_binary" \
    --source-repo "$root" \
    --source-ref "$packy_ref" \
    --claude-version "$claude_version" \
    --evidence "$evidence_dir/evidence.json"
)
