#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Generate a Homebrew formula for Packy from a release checksum manifest.

Usage:
  scripts/generate-homebrew-formula.sh --version <v0.x.y> --checksums <SHA256SUMS> --out <Formula/packy.rb> [options]

Options:
  --version   Release tag used in artifact filenames, for example v0.1.0.
  --checksums Path to the complete release SHA256SUMS manifest.
  --out       Output formula path.
  --repo      GitHub repository in owner/name form. Defaults to yersonargotev/packy.
  --homepage  Formula homepage. Defaults to https://github.com/<repo>.
  --desc      Formula description. Defaults to AI coding workflow installer.
  -h, --help  Show this help.
USAGE
}

fail() {
  echo "generate-homebrew-formula: $*" >&2
  exit 1
}

ruby_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "$value"
}

version="${RELEASE_VERSION:-}"
checksums_path=""
out_path=""
repo="yersonargotev/packy"
homepage=""
desc="AI coding workflow installer"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      version="${2:-}"
      shift 2
      ;;
    --checksums)
      checksums_path="${2:-}"
      shift 2
      ;;
    --out)
      out_path="${2:-}"
      shift 2
      ;;
    --repo)
      repo="${2:-}"
      shift 2
      ;;
    --homepage)
      homepage="${2:-}"
      shift 2
      ;;
    --desc)
      desc="${2:-}"
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
  fail "release version must be a v0.x.y tag such as v0.1.0; got '${version:-<empty>}'"
fi
if [[ -z "$checksums_path" ]]; then
  fail "--checksums is required"
fi
if [[ ! -f "$checksums_path" ]]; then
  fail "checksums file not found: $checksums_path"
fi
if [[ -z "$out_path" ]]; then
  fail "--out is required"
fi
if [[ "$repo" != */* ]]; then
  fail "--repo must be in owner/name form; got '$repo'"
fi
if [[ -z "$homepage" ]]; then
  homepage="https://github.com/$repo"
fi

formula_version="${version#v}"
asset_base="https://github.com/${repo}/releases/download/${version}"

is_expected_artifact() {
  local artifact="$1"
  local expected
  for expected in "${expected_artifacts[@]}"; do
    if [[ "$artifact" == "$expected" ]]; then
      return 0
    fi
  done
  return 1
}

validate_checksum_manifest() {
  local line checksum artifact extra expected
  local seen_artifacts=""

  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ -z "${line//[[:space:]]/}" ]]; then
      continue
    fi

    checksum=""
    artifact=""
    extra=""
    read -r checksum artifact extra <<< "$line"
    if [[ -z "$checksum" || -z "$artifact" || -n "$extra" ]]; then
      fail "checksum line should be '<sha256>  <artifact>'; got '$line'"
    fi
    if [[ ! "$checksum" =~ ^[0-9A-Fa-f]{64}$ ]]; then
      fail "checksum entry for $artifact is not a SHA-256 hex digest"
    fi
    if ! is_expected_artifact "$artifact"; then
      fail "unexpected checksum entry for $artifact"
    fi

    if [[ " $seen_artifacts " == *" $artifact "* ]]; then
      fail "duplicate checksum entry for $artifact"
    fi
    seen_artifacts="${seen_artifacts} ${artifact}"

    case "$artifact" in
      "$darwin_amd64_artifact")
        darwin_amd64_sha="$checksum"
        ;;
      "$darwin_arm64_artifact")
        darwin_arm64_sha="$checksum"
        ;;
      "$linux_amd64_artifact")
        linux_amd64_sha="$checksum"
        ;;
      "$linux_arm64_artifact")
        linux_arm64_sha="$checksum"
        ;;
    esac
  done < "$checksums_path"

  for expected in "${expected_artifacts[@]}"; do
    if [[ " $seen_artifacts " != *" $expected "* ]]; then
      fail "missing checksum entry for $expected"
    fi
  done
}

darwin_amd64_artifact="packy_${version}_darwin_amd64"
darwin_arm64_artifact="packy_${version}_darwin_arm64"
linux_amd64_artifact="packy_${version}_linux_amd64"
linux_arm64_artifact="packy_${version}_linux_arm64"
sbom_artifact="sbom.spdx.json"

expected_artifacts=(
  "$darwin_amd64_artifact"
  "$darwin_arm64_artifact"
  "$linux_amd64_artifact"
  "$linux_arm64_artifact"
  "$sbom_artifact"
)

darwin_amd64_sha=""
darwin_arm64_sha=""
linux_amd64_sha=""
linux_arm64_sha=""
validate_checksum_manifest

mkdir -p "$(dirname "$out_path")"
cat > "$out_path" <<FORMULA
class Packy < Formula
  desc "$(ruby_escape "$desc")"
  homepage "$(ruby_escape "$homepage")"
  version "$formula_version"

  on_macos do
    if Hardware::CPU.arm?
      url "${asset_base}/${darwin_arm64_artifact}", using: :nounzip
      sha256 "$darwin_arm64_sha"
    else
      url "${asset_base}/${darwin_amd64_artifact}", using: :nounzip
      sha256 "$darwin_amd64_sha"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${asset_base}/${linux_arm64_artifact}", using: :nounzip
      sha256 "$linux_arm64_sha"
    else
      url "${asset_base}/${linux_amd64_artifact}", using: :nounzip
      sha256 "$linux_amd64_sha"
    end
  end

  def install
    downloaded_binary = Dir["packy_*"].first
    odie "downloaded packy binary not found" if downloaded_binary.nil?
    bin.install downloaded_binary => "packy"
  end

  test do
    system "#{bin}/packy", "--version"
  end
end
FORMULA

echo "wrote Homebrew formula to $out_path"
