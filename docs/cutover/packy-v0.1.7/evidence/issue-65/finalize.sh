#!/usr/bin/env bash
set -Eeuo pipefail

EVIDENCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
TRANSCRIPT="$EVIDENCE_DIR/transcript.log"
REAL_HOME="${HOME:?HOME must be set}"
XDG_ROOT="${XDG_CONFIG_HOME:-$REAL_HOME/.config}"
RUN_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/packy-issue-65-finalize.XXXXXX")"
BREW=/opt/homebrew/bin/brew
PACKY=/opt/homebrew/bin/packy
PACKY_SOURCE="$REAL_HOME/.local/share/packy"
ARCHIVE="$REAL_HOME/Documents/dev/backups/matty-to-packy-cutover-20260717"
PACKY_RELEASE_SHA=283e726e9e1886d8b51e3222434022ac56f733eb

[[ -f "$TRANSCRIPT" ]] || { echo "missing cutover transcript" >&2; exit 1; }
exec > >(tee -a "$TRANSCRIPT") 2>&1
trap 'status=$?; rm -rf "$RUN_ROOT"; exit "$status"' EXIT

stamp() { date -u +%Y-%m-%dT%H:%M:%SZ; }
section() { printf '\n===== %s | %s =====\n' "$(stamp)" "$1"; }
die() { printf '[%s] FINALIZE_ERROR message=%s\n' "$(stamp)" "$*"; exit 1; }

run_expect() {
  local expected="$1" label="$2"
  shift 2
  printf '\n[%s] command=' "$(stamp)"
  printf '%q ' "$@"
  printf '\n[%s] label=%s expected_status=%s\n' "$(stamp)" "$label" "$expected"
  set +e
  "$@"
  local status=$?
  set -e
  printf '[%s] label=%s exit_status=%s\n' "$(stamp)" "$label" "$status"
  [[ "$status" -eq "$expected" ]] || die "$label expected $expected, got $status"
}

normalized_host_fingerprint() {
  local output="$1"
  python3 - "$REAL_HOME" "$XDG_ROOT" > "$output" <<'PY'
import hashlib, json, os, re, sys
home, xdg = sys.argv[1:]
def digest(data):
    if isinstance(data, str):
        data = data.encode()
    return hashlib.sha256(data).hexdigest()
agents = open(os.path.join(home, ".codex", "AGENTS.md"), encoding="utf-8").read()
for product in ("matty", "packy"):
    agents = re.sub(rf"\n?<!-- {product}:[^>]+ -->.*?<!-- /{product}:[^>]+ -->", "", agents, flags=re.S)
agents = re.sub(r"\n{3,}", "\n\n", agents).strip() + "\n"
opencode_path = os.path.join(xdg, "opencode", "opencode.json")
opencode = json.load(open(opencode_path, encoding="utf-8"))
owned = {
    os.path.join(xdg, "opencode", "matty.md"),
    os.path.join(xdg, "opencode", "packy.md"),
}
opencode["instructions"] = [value for value in opencode.get("instructions", []) if not (isinstance(value, str) and os.path.normpath(value) in owned)]
engram = os.path.join(home, ".local", "bin", "engram")
print(json.dumps({
    "codex_agents_without_product_blocks": digest(agents),
    "engram_binary": digest(open(engram, "rb").read()),
    "opencode_without_product_instruction": digest(json.dumps(opencode, sort_keys=True, separators=(",", ":"))),
}, sort_keys=True, separators=(",", ":")))
PY
}

section "finalize accepted delegated Engram rewrite"
grep -Fq 'ERROR phase=packy-install message=contributor-owned host configuration or Engram binary changed' "$TRANSCRIPT" || die "transcript does not contain the adjudicated final-gate false positive"
grep -m1 '^normalized_host_before=' "$TRANSCRIPT" | sed 's/^normalized_host_before=//' | jq -c 'del(.codex_config)' > "$RUN_ROOT/host-before.json"
normalized_host_fingerprint "$RUN_ROOT/host-after.json"
printf 'adjudication=Packy delegated Engram setup through the external Homebrew-owned binary; Codex rewrote its config serialization and Engram marketplace timestamp while contributor surfaces and Engram state remained intact\n'
printf 'normalized_host_before_without_delegated_codex_config='; cat "$RUN_ROOT/host-before.json"; printf '\n'
printf 'normalized_host_after_without_delegated_codex_config='; cat "$RUN_ROOT/host-after.json"; printf '\n'
cmp "$RUN_ROOT/host-before.json" "$RUN_ROOT/host-after.json" || die "normalized contributor surfaces or Engram binary changed"

section "fresh Packy ownership"
run_expect 0 "Packy version" "$PACKY" --version
run_expect 0 "Packy formula inventory" "$BREW" list --versions packy
run_expect 1 "Matty formula absent" "$BREW" list --versions matty
[[ ! -e /opt/homebrew/bin/matty && ! -e "$REAL_HOME/.matty" && ! -e "$REAL_HOME/.local/share/matty" ]] || die "active Matty product residual remains"
[[ -f "$REAL_HOME/.packy/config.json" && -d "$PACKY_SOURCE" ]] || die "fresh Packy state/source is absent"
[[ "$(git -C "$PACKY_SOURCE" describe --tags --exact-match)" == v0.1.7 ]] || die "Packy source tag is unexpected"
[[ "$(git -C "$PACKY_SOURCE" rev-parse HEAD)" == "$PACKY_RELEASE_SHA" ]] || die "Packy source SHA is unexpected"
[[ -z "$(git -C "$PACKY_SOURCE" status --porcelain)" ]] || die "Packy source is dirty"
python3 - "$REAL_HOME/.packy/config.json" "$PACKY_SOURCE" <<'PY'
import json, os, sys
state, source = sys.argv[1:]
skills = json.load(open(state)).get("managed_skills", [])
if len(skills) != 23:
    raise SystemExit(f"managed skill count is {len(skills)}, want 23")
for skill in skills:
    if not skill["source_path"].startswith(source + os.sep) or not os.path.islink(skill["link_path"]):
        raise SystemExit(f"invalid Packy ownership for {skill.get('name')}")
    if os.path.realpath(skill["link_path"]) != os.path.realpath(skill["source_path"]):
        raise SystemExit(f"link drift for {skill.get('name')}")
print("fresh_classic_state_links=23 all_sources_are_packy=true")
PY
run_expect 0 "Packy doctor JSON" bash -o pipefail -c 'packy doctor --json | jq -e '\''(.summary.status == "healthy") and all(.checks[]; .severity == "PASS")'\'' >/dev/null'
run_expect 0 "semantic Matty pack list" "$PACKY" pack list
run_expect 0 "semantic Matty pack show" "$PACKY" pack show matty
run_expect 0 "semantic Matty Codex status" "$PACKY" pack status matty --surface codex --json
run_expect 0 "semantic Matty OpenCode status" "$PACKY" pack status matty --surface opencode --json
run_expect 0 "semantic Matty activation preview" "$PACKY" pack activate matty --surface codex --dry-run

section "external ownership preservation"
[[ -f "$REAL_HOME/.codex/config.toml" ]] || die "contributor-owned Codex config is absent"
run_expect 0 "current Codex config digest" shasum -a 256 "$REAL_HOME/.codex/config.toml"
for pattern in \
  '^model_instructions_file = ' \
  '^experimental_compact_prompt_file = ' \
  '^\[marketplaces\.engram\]$' \
  '^\[plugins\."engram@engram"\]$' \
  '^\[mcp_servers\.engram\]$'; do
  grep -Eq "$pattern" "$REAL_HOME/.codex/config.toml" || die "delegated Engram Codex projection is incomplete: $pattern"
done
printf 'codex_config_sections='; grep -E '^\[[^]]+\]$' "$REAL_HOME/.codex/config.toml" | sed 's/^\[//; s/\]$//' | LC_ALL=C sort | tr '\n' ','; printf '\n'
run_expect 0 "Engram version" "$REAL_HOME/.local/bin/engram" version
run_expect 0 "Engram process" pgrep -lf '/engram'

section "recovery archive and authentic history"
[[ -f "$ARCHIVE/SHA256SUMS" ]] || die "recovery manifest is absent"
(cd "$ARCHIVE" && shasum -a 256 -c SHA256SUMS) || die "recovery archive digest verification failed"
python3 - "$ARCHIVE" <<'PY'
import os, stat, sys
root=sys.argv[1]; directories=files=0
for current, names, filenames in os.walk(root, topdown=True, followlinks=False):
    for name in names + filenames:
        path=os.path.join(current,name); relative=os.path.relpath(path,root)
        if relative == "SHA256SUMS": continue
        mode=os.lstat(path).st_mode
        if stat.S_ISLNK(mode) or not (stat.S_ISDIR(mode) or stat.S_ISREG(mode)):
            raise SystemExit(f"unsupported recovery entry: {relative}")
        directories += stat.S_ISDIR(mode)
        files += stat.S_ISREG(mode)
if (directories, files) != (6, 3):
    raise SystemExit(f"recovery typed inventory is directories={directories} files={files}, want 6/3")
print("recovery_typed_inventory=6_directories,3_regular_files,0_special_entries")
PY
run_expect 0 "historical Matty tag remains" bash -c "git ls-remote https://github.com/yersonargotev/matty.git refs/tags/v0.1.6 | grep -q '^68aec8969374fa9e9a6ea86b33e6719646b999f8'"
run_expect 0 "Packy release tag remains" bash -c "git ls-remote https://github.com/yersonargotev/packy.git refs/tags/v0.1.7 | grep -q '^283e726e9e1886d8b51e3222434022ac56f733eb'"
run_expect 0 "historical Matty release asset remains" curl -fsIL https://github.com/yersonargotev/packy/releases/download/v0.1.6/matty_v0.1.6_darwin_arm64
run_expect 0 "Packy release asset remains" curl -fsIL https://github.com/yersonargotev/packy/releases/download/v0.1.7/packy_v0.1.7_darwin_arm64

section "final result"
printf '[%s] overall_failures=0 cutover=passed finalization=passed archive=%s\n' "$(stamp)" "$ARCHIVE"
