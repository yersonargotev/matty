#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Build Packy release artifacts, an SPDX SBOM, and a SHA-256 manifest.

Usage:
  scripts/build-release-artifacts.sh --version <v0.x.y> [--out-dir <dir>]

Options:
  --version  Release tag embedded in artifact filenames, for example v0.1.0.
  --out-dir  Output directory for release assets. Defaults to dist.
  -h, --help Show this help.
USAGE
}

version="${RELEASE_VERSION:-}"
out_dir="${RELEASE_OUT_DIR:-dist}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      version="${2:-}"
      shift 2
      ;;
    --out-dir)
      out_dir="${2:-}"
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

release_version_regex='^v0\.[0-9]+\.[0-9]+$'
if [[ ! "$version" =~ $release_version_regex ]]; then
  echo "Release version must be a v0.x.y tag such as v0.1.0; got '${version:-<empty>}'" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
mkdir -p "$out_dir"
out_dir="$(cd "$out_dir" && pwd)"
if find "$out_dir" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
  echo "release output directory must be empty: $out_dir" >&2
  exit 1
fi

platforms=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
)

artifacts=()
cd "$repo_root"
for platform in "${platforms[@]}"; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  artifact="packy_${version}_${goos}_${goarch}"
  echo "building ${artifact}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w -X github.com/yersonargotev/packy/internal/version.Value=${version}" -o "$out_dir/$artifact" ./cmd/packy
  artifacts+=("$artifact")
done

metadata_dir="$(mktemp -d "$(dirname "$out_dir")/.packy-release-metadata.XXXXXX")"
trap 'rm -rf "$metadata_dir"' EXIT
created="$(git show -s --format=%cI HEAD)"
go run ./internal/tools/releasesbom \
  --version "$version" \
  --created "$created" \
  --dist "$out_dir" \
  --out "$metadata_dir/sbom.spdx.json" >/dev/null
ln "$metadata_dir/sbom.spdx.json" "$out_dir/sbom.spdx.json"
artifacts+=("sbom.spdx.json")
IFS=$'\n' artifacts=($(printf '%s\n' "${artifacts[@]}" | sort))
unset IFS

(
  cd "$out_dir"
  : > SHA256SUMS
  for artifact in "${artifacts[@]}"; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$artifact" >> SHA256SUMS
    else
      shasum -a 256 "$artifact" >> SHA256SUMS
    fi
  done
)

echo "wrote four binaries, sbom.spdx.json, and SHA256SUMS to $out_dir"
