#!/usr/bin/env bash
set -u -o pipefail

EVIDENCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TRANSCRIPT="$EVIDENCE_DIR/transcript.log"
RUN_ROOT="$(cd "$(mktemp -d "${TMPDIR:-/tmp}/packy-issue-64.XXXXXX")" && pwd -P)"
FAILURES=0
JQ_BIN="$(command -v jq)"

exec > >(tee "$TRANSCRIPT") 2>&1

stamp() { date -u +%Y-%m-%dT%H:%M:%SZ; }
section() { printf '\n===== %s | %s =====\n' "$(stamp)" "$1"; }

run_expect() {
  local expected="$1" label="$2"
  shift 2
  printf '\n[%s] command=' "$(stamp)"
  local arg
  for arg in "$@"; do printf '%q ' "$arg"; done
  printf '\n[%s] label=%s expected_status=%s\n' "$(stamp)" "$label" "$expected"
  "$@"
  local status=$?
  printf '[%s] label=%s exit_status=%s\n' "$(stamp)" "$label" "$status"
  if [[ "$status" -ne "$expected" ]]; then
    printf '[%s] ERROR label=%s expected=%s actual=%s\n' "$(stamp)" "$label" "$expected" "$status"
    FAILURES=$((FAILURES + 1))
  fi
  return 0
}

run_shell() {
  local expected="$1" label="$2" command="$3"
  printf '\n[%s] command=%s\n[%s] label=%s expected_status=%s\n' "$(stamp)" "$command" "$(stamp)" "$label" "$expected"
  bash -o pipefail -c "$command"
  local status=$?
  printf '[%s] label=%s exit_status=%s\n' "$(stamp)" "$label" "$status"
  if [[ "$status" -ne "$expected" ]]; then
    printf '[%s] ERROR label=%s expected=%s actual=%s\n' "$(stamp)" "$label" "$expected" "$status"
    FAILURES=$((FAILURES + 1))
  fi
  return 0
}

prepare_direction() {
  local name="$1"
  DIRECTION_ROOT="$RUN_ROOT/$name"
  HOME="$DIRECTION_ROOT/home"
  XDG_CONFIG_HOME="$DIRECTION_ROOT/xdg"
  HOMEBREW_PREFIX="$DIRECTION_ROOT/homebrew"
  HOMEBREW_CACHE="$DIRECTION_ROOT/cache"
  HOMEBREW_LOGS="$DIRECTION_ROOT/logs"
  HOMEBREW_TEMP="$DIRECTION_ROOT/temp"
  HOMEBREW_REPOSITORY="$HOMEBREW_PREFIX"
  GIT_CONFIG_GLOBAL="$DIRECTION_ROOT/gitconfig"
  GIT_CONFIG_NOSYSTEM=1
  mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$HOMEBREW_CACHE" "$HOMEBREW_LOGS" "$HOMEBREW_TEMP"
  : > "$GIT_CONFIG_GLOBAL"
  export HOME XDG_CONFIG_HOME HOMEBREW_PREFIX HOMEBREW_CACHE HOMEBREW_LOGS HOMEBREW_TEMP HOMEBREW_REPOSITORY GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM
  export HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ENV_HINTS=1
  export PATH="$HOMEBREW_PREFIX/bin:/usr/bin:/bin:/usr/sbin:/sbin"
  BREW="$HOMEBREW_PREFIX/bin/brew"

  section "$name isolation"
  printf 'DIRECTION_ROOT=%s\nHOME=%s\nXDG_CONFIG_HOME=%s\nHOMEBREW_PREFIX=%s\nHOMEBREW_REPOSITORY=%s\nHOMEBREW_CACHE=%s\nHOMEBREW_LOGS=%s\nHOMEBREW_TEMP=%s\nGIT_CONFIG_GLOBAL=%s\nGIT_CONFIG_NOSYSTEM=%s\nPATH=%s\n' \
    "$DIRECTION_ROOT" "$HOME" "$XDG_CONFIG_HOME" "$HOMEBREW_PREFIX" "$HOMEBREW_REPOSITORY" "$HOMEBREW_CACHE" "$HOMEBREW_LOGS" "$HOMEBREW_TEMP" "$GIT_CONFIG_GLOBAL" "$GIT_CONFIG_NOSYSTEM" "$PATH"
  run_expect 0 "$name clone disposable Homebrew" git clone --depth=1 https://github.com/Homebrew/brew "$HOMEBREW_PREFIX"
  run_expect 0 "$name brew version" "$BREW" --version
  run_expect 0 "$name brew repository commit" git -C "$HOMEBREW_PREFIX" rev-parse HEAD
  run_expect 0 "$name brew prefix" "$BREW" --prefix
  run_expect 0 "$name brew cache" "$BREW" --cache
  run_shell 0 "$name prefix assertion" "test \"\$(cd \"$HOMEBREW_PREFIX\" && pwd -P)\" = \"\$($BREW --prefix)\""
}

section "bound execution facts"
printf 'issue=64\nstarting_base=283e726e9e1886d8b51e3222434022ac56f733eb\nrun_root=%s\n' "$RUN_ROOT"
run_expect 0 "UTC clock" date -u +%Y-%m-%dT%H:%M:%SZ
run_expect 0 "kernel" uname -a
run_expect 0 "architecture" uname -m
run_expect 0 "macOS version" sw_vers
run_expect 0 "git version" git --version
run_expect 0 "curl version" curl --version
run_expect 0 "jq version" jq --version
run_expect 0 "shasum version" shasum --version

prepare_direction historical-matty-v0.1.6
MATTY_CHECKSUMS="$DIRECTION_ROOT/checksums.txt"
MATTY_ASSET="$DIRECTION_ROOT/matty_v0.1.6_darwin_arm64"
run_expect 0 "tap historical formula repository" "$BREW" tap yersonargotev/tap
MATTY_TAP="$($BREW --repository yersonargotev/tap)"
run_expect 0 "pin tap to preserved Matty formula commit" git -C "$MATTY_TAP" checkout --detach 5168baccc0aa16d3d4a7a1bac1ca1c00b11158a3
MATTY_FORMULA="$MATTY_TAP/Formula/matty.rb"
run_expect 0 "historical tap commit" git -C "$MATTY_TAP" rev-parse HEAD
run_expect 0 "download Matty v0.1.6 checksums" curl -fL --retry 3 -o "$MATTY_CHECKSUMS" https://github.com/yersonargotev/packy/releases/download/v0.1.6/checksums.txt
run_expect 0 "download Matty v0.1.6 arm64 asset" curl -fL --retry 3 -o "$MATTY_ASSET" https://github.com/yersonargotev/packy/releases/download/v0.1.6/matty_v0.1.6_darwin_arm64
run_expect 0 "Matty preserved input digests" shasum -a 256 "$MATTY_FORMULA" "$MATTY_CHECKSUMS" "$MATTY_ASSET"
run_shell 0 "Matty asset matches release manifest" "cd \"$DIRECTION_ROOT\" && grep '  matty_v0.1.6_darwin_arm64$' checksums.txt | shasum -a 256 -c -"
run_expect 0 "real historical fully qualified formula install" "$BREW" install yersonargotev/tap/matty
run_expect 0 "Matty formula inventory" "$BREW" list --versions matty
pushd "$DIRECTION_ROOT" >/dev/null
run_expect 0 "Matty installed binary version" "$HOMEBREW_PREFIX/bin/matty" --version
run_expect 0 "Matty installed binary digest" shasum -a 256 "$HOMEBREW_PREFIX/bin/matty"
run_expect 0 "Matty init" "$HOMEBREW_PREFIX/bin/matty" init
run_expect 0 "Matty Installed Source exact tag" git -C "$HOME/.local/share/matty" describe --tags --exact-match
run_expect 0 "Matty Installed Source SHA" git -C "$HOME/.local/share/matty" rev-parse HEAD
run_expect 0 "Matty install dry-run" "$HOMEBREW_PREFIX/bin/matty" install --dry-run
run_expect 0 "Matty install apply" "$HOMEBREW_PREFIX/bin/matty" install
run_expect 0 "Matty doctor" "$HOMEBREW_PREFIX/bin/matty" doctor
run_expect 0 "Matty doctor JSON" "$HOMEBREW_PREFIX/bin/matty" doctor --json
run_expect 0 "trust isolated Engram tap for historical update" "$BREW" trust gentleman-programming/tap
run_expect 0 "Matty update dry-run" "$HOMEBREW_PREFIX/bin/matty" update --dry-run
run_expect 0 "Matty update apply" "$HOMEBREW_PREFIX/bin/matty" update
run_expect 0 "Matty pack list" "$HOMEBREW_PREFIX/bin/matty" pack list
run_expect 0 "Matty semantic pack show" "$HOMEBREW_PREFIX/bin/matty" pack show matty
run_expect 0 "Matty uninstall dry-run" "$HOMEBREW_PREFIX/bin/matty" uninstall --dry-run
run_expect 0 "Matty uninstall apply" "$HOMEBREW_PREFIX/bin/matty" uninstall
run_expect 0 "Matty final doctor reports expected warnings after state removal" "$HOMEBREW_PREFIX/bin/matty" doctor
run_shell 0 "Matty resulting filesystem observation" "find \"$HOME\" \"$XDG_CONFIG_HOME\" -mindepth 1 -maxdepth 5 -print | LC_ALL=C sort"
run_shell 0 "Matty lifecycle cleanup assertions" "test ! -e \"$HOME/.matty/config.json\" && test -d \"$HOME/.local/share/matty\" && test ! -e \"$XDG_CONFIG_HOME/opencode/matty.md\""
popd >/dev/null

prepare_direction fresh-packy-v0.1.7
PACKY_CHECKSUMS="$DIRECTION_ROOT/checksums.txt"
PACKY_ASSET="$DIRECTION_ROOT/packy_v0.1.7_darwin_arm64"
mkdir -p "$HOME/.matty" "$HOME/.local/share/matty"
printf 'legacy-state-sentinel\n' > "$HOME/.matty/legacy-sentinel"
printf 'legacy-source-sentinel\n' > "$HOME/.local/share/matty/legacy-sentinel"
LEGACY_STATE_BEFORE="$(shasum -a 256 "$HOME/.matty/legacy-sentinel" | awk '{print $1}')"
LEGACY_SOURCE_BEFORE="$(shasum -a 256 "$HOME/.local/share/matty/legacy-sentinel" | awk '{print $1}')"
export MATTY_SKILLS_SOURCE="$DIRECTION_ROOT/must-not-be-read"
run_expect 0 "download Packy v0.1.7 checksums" curl -fL --retry 3 -o "$PACKY_CHECKSUMS" https://github.com/yersonargotev/packy/releases/download/v0.1.7/checksums.txt
run_expect 0 "download Packy v0.1.7 arm64 asset" curl -fL --retry 3 -o "$PACKY_ASSET" https://github.com/yersonargotev/packy/releases/download/v0.1.7/packy_v0.1.7_darwin_arm64
run_expect 0 "Packy published input digests" shasum -a 256 "$PACKY_CHECKSUMS" "$PACKY_ASSET"
run_shell 0 "Packy asset matches release manifest" "cd \"$DIRECTION_ROOT\" && grep '  packy_v0.1.7_darwin_arm64$' checksums.txt | shasum -a 256 -c -"
run_expect 0 "real fully qualified Packy tap install" "$BREW" install yersonargotev/tap/packy
PACKY_TAP="$($BREW --repository yersonargotev/tap)"
run_expect 0 "Packy tap commit" git -C "$PACKY_TAP" rev-parse HEAD
run_expect 0 "Packy formula digest" shasum -a 256 "$PACKY_TAP/Formula/packy.rb"
run_expect 0 "Packy formula inventory" "$BREW" list --versions packy
pushd "$DIRECTION_ROOT" >/dev/null
run_expect 0 "Packy installed binary version" "$HOMEBREW_PREFIX/bin/packy" --version
run_expect 0 "Packy installed binary digest" shasum -a 256 "$HOMEBREW_PREFIX/bin/packy"
run_expect 0 "Packy init" "$HOMEBREW_PREFIX/bin/packy" init
run_expect 0 "Packy Installed Source exact tag" git -C "$HOME/.local/share/packy" describe --tags --exact-match
run_expect 0 "Packy Installed Source SHA" git -C "$HOME/.local/share/packy" rev-parse HEAD
run_expect 0 "Packy install dry-run" "$HOMEBREW_PREFIX/bin/packy" install --dry-run
run_expect 0 "Packy install apply" "$HOMEBREW_PREFIX/bin/packy" install
run_expect 0 "Packy doctor" "$HOMEBREW_PREFIX/bin/packy" doctor
run_expect 0 "Packy doctor JSON" "$HOMEBREW_PREFIX/bin/packy" doctor --json
run_expect 0 "Packy semantic pack list" "$HOMEBREW_PREFIX/bin/packy" pack list
run_expect 0 "Packy semantic matty pack show" "$HOMEBREW_PREFIX/bin/packy" pack show matty
run_expect 0 "Packy semantic matty pack status Codex" "$HOMEBREW_PREFIX/bin/packy" pack status matty --surface codex --json
run_expect 0 "Packy semantic matty pack status OpenCode" "$HOMEBREW_PREFIX/bin/packy" pack status matty --surface opencode --json
run_expect 0 "Packy semantic matty activation preview Codex" "$HOMEBREW_PREFIX/bin/packy" pack activate matty --surface codex --dry-run
run_shell 0 "Packy fresh ownership assertions" "$JQ_BIN -e '(.managed_skills | length > 0) and all(.managed_skills[]; .source_path | startswith(\"$HOME/.local/share/packy/\"))' \"$HOME/.packy/config.json\" >/dev/null && ! grep -R -E 'matty:(start|end)|\\.local/share/matty|\\.matty/config.json' \"$HOME/.packy\" \"$HOME/.codex\" \"$XDG_CONFIG_HOME/opencode\""
run_expect 0 "Packy uninstall dry-run" "$HOMEBREW_PREFIX/bin/packy" uninstall --dry-run
run_expect 0 "Packy uninstall apply" "$HOMEBREW_PREFIX/bin/packy" uninstall
run_expect 0 "Packy final doctor reports expected warnings after state removal" "$HOMEBREW_PREFIX/bin/packy" doctor
run_shell 0 "Packy resulting filesystem observation" "find \"$HOME\" \"$XDG_CONFIG_HOME\" -mindepth 1 -maxdepth 5 -print | LC_ALL=C sort"
run_shell 0 "Packy disjoint legacy sentinel assertions" "test \"\$(shasum -a 256 \"$HOME/.matty/legacy-sentinel\" | awk '{print \$1}')\" = \"$LEGACY_STATE_BEFORE\" && test \"\$(shasum -a 256 \"$HOME/.local/share/matty/legacy-sentinel\" | awk '{print \$1}')\" = \"$LEGACY_SOURCE_BEFORE\""
run_shell 0 "Packy lifecycle cleanup assertions" "test ! -e \"$HOME/.packy/config.json\" && test -d \"$HOME/.local/share/packy\" && test ! -e \"$XDG_CONFIG_HOME/opencode/packy.md\""
popd >/dev/null

section "cleanup"
run_expect 0 "remove both disposable directions" chmod -R u+w "$RUN_ROOT"
run_expect 0 "delete both disposable directions" rm -rf "$RUN_ROOT"
run_shell 0 "disposable root absent" "test ! -e \"$RUN_ROOT\""
printf '\n[%s] overall_failures=%s\n' "$(stamp)" "$FAILURES"
if [[ "$FAILURES" -ne 0 ]]; then
  exit 1
fi
