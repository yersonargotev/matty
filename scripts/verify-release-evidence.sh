#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Verify one complete Packy release candidate before publication.

Usage: scripts/verify-release-evidence.sh \
  --tag <v0.x.y> --commit <40-hex-sha> --dist <directory> \
  --evidence-root <directory> --formula <file> \
  --notes-template <file> --notes-output <file>
EOF
}

tag=""
commit=""
dist=""
evidence_root=""
formula=""
notes_template=""
notes_output=""
while (($#)); do
  case "$1" in
    --tag) tag="${2:-}"; shift 2 ;;
    --commit) commit="${2:-}"; shift 2 ;;
    --dist) dist="${2:-}"; shift 2 ;;
    --evidence-root) evidence_root="${2:-}"; shift 2 ;;
    --formula) formula="${2:-}"; shift 2 ;;
    --notes-template) notes_template="${2:-}"; shift 2 ;;
    --notes-output) notes_output="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ ! "$tag" =~ ^v0\.[0-9]+\.[0-9]+$ ]]; then
  echo "release evidence tag must be v0.x.y; got '${tag:-<empty>}'" >&2
  exit 2
fi
if [[ ! "$commit" =~ ^[0-9a-f]{40}$ ]]; then
  echo "release evidence commit must be a full lowercase SHA" >&2
  exit 2
fi
for value in "$dist" "$evidence_root" "$formula" "$notes_template" "$notes_output"; do
  if [[ -z "$value" ]]; then
    echo "all release evidence paths are required" >&2
    exit 2
  fi
done
for command in go sed grep find sort cmp; do
  command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
done

scratch="$(mktemp -d "${TMPDIR:-/tmp}/packy-release-evidence.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT
platforms=(darwin_amd64 darwin_arm64 linux_amd64 linux_arm64)
{
  printf '%s\n' checksums.txt
  for platform in "${platforms[@]}"; do
    printf 'packy_%s_%s\n' "$tag" "$platform"
  done
} | sort > "$scratch/expected-entries"
find "$dist" -mindepth 1 -maxdepth 1 -type f -exec basename {} \; | sort > "$scratch/actual-entries"
if ! cmp -s "$scratch/expected-entries" "$scratch/actual-entries"; then
  echo "release candidate artifact set is incomplete or unexpected" >&2
  diff -u "$scratch/expected-entries" "$scratch/actual-entries" >&2 || true
  exit 1
fi

if ! awk 'NF != 2 || length($1) != 64 || $1 !~ /^[0-9a-f]+$/ { exit 1 }' "$dist/checksums.txt"; then
  echo "checksums.txt is malformed" >&2
  exit 1
fi
if [[ "$(wc -l < "$dist/checksums.txt" | tr -d ' ')" != 4 ]]; then
  echo "checksums.txt must contain exactly four Packy artifacts" >&2
  exit 1
fi
for platform in "${platforms[@]}"; do
  name="packy_${tag}_${platform}"
  matches="$(awk -v name="$name" '$2 == name { print $1 }' "$dist/checksums.txt")"
  [[ "$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d ' ')" == 1 ]] || { echo "checksums.txt missing or duplicates $name" >&2; exit 1; }
  want="$matches"
  if command -v sha256sum >/dev/null; then
    got="$(sha256sum "$dist/$name" | awk '{print $1}')"
  else
    got="$(shasum -a 256 "$dist/$name" | awk '{print $1}')"
  fi
  [[ "$got" == "$want" ]] || { echo "checksum mismatch for $name" >&2; exit 1; }
done

[[ -f "$formula" ]] || { echo "proved Homebrew formula is missing" >&2; exit 1; }
grep -Fq "version \"${tag#v}\"" "$formula" || { echo "formula version does not match $tag" >&2; exit 1; }
for platform in "${platforms[@]}"; do
  name="packy_${tag}_${platform}"
  grep -Fq "$name" "$formula" || { echo "formula missing $name" >&2; exit 1; }
  digest="$(awk -v name="$name" '$2 == name { print $1 }' "$dist/checksums.txt")"
  grep -Fq "$digest" "$formula" || { echo "formula checksum does not match $name" >&2; exit 1; }
done
grep -Fq '"#{bin}/packy", "--version"' "$formula" || { echo "formula test surface changed" >&2; exit 1; }

go run ./internal/tools/claudesmoke verify-release \
  --evidence-root "$evidence_root" \
  --packy-version "$tag" \
  --packy-sha "$commit"

[[ -f "$notes_template" ]] || { echo "release-note template is missing" >&2; exit 1; }
if [[ "$(grep -Fo '{{TAG}}' "$notes_template" | wc -l | tr -d ' ')" != 1 ]]; then
  echo "release-note template must contain exactly one {{TAG}} placeholder" >&2
  exit 1
fi
for required in "2.1.203" "state schema v2" "matty 3.0.0" "engram 2.0.0" "degraded" "Limitations"; do
  grep -Fqi "$required" "$notes_template" || { echo "release notes missing required Claude support fact: $required" >&2; exit 1; }
done
mkdir -p "$(dirname "$notes_output")"
sed "s/{{TAG}}/$tag/g" "$notes_template" > "$notes_output"
grep -Fq "$tag" "$notes_output" || { echo "rendered release notes do not bind $tag" >&2; exit 1; }
if grep -Fq '{{TAG}}' "$notes_output"; then
  echo "rendered release notes retain an unresolved tag" >&2
  exit 1
fi

echo "release evidence verified: tag=$tag commit=$commit artifacts=4 claude_smokes=4"
