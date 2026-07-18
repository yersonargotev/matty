#!/usr/bin/env bash
set -Eeuo pipefail

EVIDENCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(git -C "$EVIDENCE_DIR" rev-parse --show-toplevel)"
TRANSCRIPT="$EVIDENCE_DIR/transcript.log"
RUN_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/packy-issue-65.XXXXXX")"
REAL_HOME="${HOME:?HOME must be set}"
XDG_ROOT="${XDG_CONFIG_HOME:-$REAL_HOME/.config}"
BREW=/opt/homebrew/bin/brew
MATTY=/opt/homebrew/bin/matty
PACKY=/opt/homebrew/bin/packy
MATTY_STATE="$REAL_HOME/.matty"
MATTY_SOURCE="$REAL_HOME/.local/share/matty"
PACKY_STATE="$REAL_HOME/.packy"
PACKY_SOURCE="$REAL_HOME/.local/share/packy"
ARCHIVE="$REAL_HOME/Documents/dev/backups/matty-to-packy-cutover-20260717"
TAP=/opt/homebrew/Library/Taps/yersonargotev/homebrew-tap
STARTING_BASE=ad23cc0a33fe32d1f730003c24d8df87934dd9d7
MATTY_SOURCE_SHA=f348b84e50222a4eeadf5abbcedef7a24974cd88
MATTY_TAP_SHA=7603485b5071db932e83f6edf9f83b69960cd0f3
PACKY_TAP_SHA=ae1a2f979f073a5b07214d8f303c7ce5ff67d84d
PACKY_RELEASE_SHA=283e726e9e1886d8b51e3222434022ac56f733eb
PHASE=preflight
ARCHIVE_CREATED=0
ARCHIVE_READY=0

mkdir -p "$EVIDENCE_DIR"
: > "$TRANSCRIPT"
exec > >(tee -a "$TRANSCRIPT") 2>&1

stamp() { date -u +%Y-%m-%dT%H:%M:%SZ; }
section() { printf '\n===== %s | %s =====\n' "$(stamp)" "$1"; }
die() { printf '[%s] ERROR phase=%s message=%s\n' "$(stamp)" "$PHASE" "$*"; exit 1; }

on_exit() {
  local status=$?
  if [[ "$status" -ne 0 ]]; then
    printf '[%s] FAILED phase=%s exit_status=%s\n' "$(stamp)" "$PHASE" "$status"
    if [[ "$PHASE" != preflight && "$PHASE" != archive-verified ]]; then
      printf '[%s] RECOVERY_REQUIRED finish fresh Packy installation or deliberately restore historical Matty v0.1.6; never infer ownership\n' "$(stamp)"
    fi
  fi
  if [[ "$ARCHIVE_CREATED" -eq 1 && "$ARCHIVE_READY" -eq 0 && -e "$ARCHIVE" ]]; then
    printf '[%s] cleanup=remove-incomplete-new-archive path=%s\n' "$(stamp)" "$ARCHIVE"
    rm -rf -- "$ARCHIVE"
  fi
  rm -rf "$RUN_ROOT"
}
trap on_exit EXIT

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

run_capture() {
  local expected="$1" label="$2" output="$3"
  shift 3
  printf '\n[%s] command=' "$(stamp)"
  printf '%q ' "$@"
  printf '\n[%s] label=%s expected_status=%s capture=%s\n' "$(stamp)" "$label" "$expected" "$output"
  set +e
  "$@" 2>&1 | tee "$output"
  local status=${PIPESTATUS[0]}
  set -e
  printf '[%s] label=%s exit_status=%s\n' "$(stamp)" "$label" "$status"
  [[ "$status" -eq "$expected" ]] || die "$label expected $expected, got $status"
}

assert_shell() {
  local label="$1" command="$2"
  printf '\n[%s] assertion=%s command=%s\n' "$(stamp)" "$label" "$command"
  bash -o pipefail -c "$command" || die "$label"
  printf '[%s] assertion=%s status=passed\n' "$(stamp)" "$label"
}

manifest_tree() {
  local root="$1" output="$2"
  (
    cd "$root"
    find . -type f ! -name SHA256SUMS -exec shasum -a 256 {} + | LC_ALL=C sort -k2
  ) > "$output"
}

inventory_tree() {
  local root="$1" output="$2" skip_root_manifest="${3:-0}"
  python3 - "$root" "$skip_root_manifest" > "$output" <<'PY'
import hashlib, os, stat, sys
root, skip_root_manifest = sys.argv[1], sys.argv[2] == "1"
entries = []
for current, directories, files in os.walk(root, topdown=True, followlinks=False):
    directories.sort()
    files.sort()
    for name in directories + files:
        path = os.path.join(current, name)
        relative = os.path.relpath(path, root)
        if skip_root_manifest and relative == "SHA256SUMS":
            continue
        metadata = os.lstat(path)
        mode = stat.S_IMODE(metadata.st_mode)
        if stat.S_ISLNK(metadata.st_mode):
            raise SystemExit(f"recovery tree contains unsupported symlink: {relative}")
        if stat.S_ISDIR(metadata.st_mode):
            entries.append(f"directory\t{mode:04o}\t{relative}")
        elif stat.S_ISREG(metadata.st_mode):
            digest = hashlib.sha256(open(path, "rb").read()).hexdigest()
            entries.append(f"file\t{mode:04o}\t{metadata.st_size}\t{digest}\t{relative}")
        else:
            raise SystemExit(f"recovery tree contains unsupported special entry: {relative}")
print("\n".join(sorted(entries)))
PY
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

codex_agents = os.path.join(home, ".codex", "AGENTS.md")
text = open(codex_agents, encoding="utf-8").read()
for product in ("matty", "packy"):
    pattern = rf"\n?<!-- {product}:[^>]+ -->.*?<!-- /{product}:[^>]+ -->"
    text = re.sub(pattern, "", text, flags=re.S)
text = re.sub(r"\n{3,}", "\n\n", text).strip() + "\n"

opencode_path = os.path.join(xdg, "opencode", "opencode.json")
opencode = json.load(open(opencode_path, encoding="utf-8"))
opencode["instructions"] = [
    value for value in opencode.get("instructions", [])
    if not (
        isinstance(value, str)
        and os.path.normpath(value) in (
            os.path.join(xdg, "opencode", "matty.md"),
            os.path.join(xdg, "opencode", "packy.md"),
        )
    )
]

files = {
    "codex_agents_without_product_blocks": digest(text),
    "opencode_without_product_instruction": digest(json.dumps(opencode, sort_keys=True, separators=(",", ":"))),
}
for name, path in {
    "codex_config": os.path.join(home, ".codex", "config.toml"),
    "engram_binary": os.path.join(home, ".local", "bin", "engram"),
}.items():
    if os.path.isfile(path):
        files[name] = digest(open(path, "rb").read())
print(json.dumps(files, sort_keys=True, separators=(",", ":")))
PY
}

verify_packy_tap_remote() {
  local remote_head
  remote_head="$(git ls-remote https://github.com/yersonargotev/homebrew-tap.git refs/heads/main | awk 'NR == 1 {print $1}')"
  [[ "$remote_head" == "$PACKY_TAP_SHA" ]] || die "remote tap main is unavailable or moved beyond the bound Packy formula commit"
  printf '[%s] remote_tap_main=%s\n' "$(stamp)" "$remote_head"
}

section "bound execution facts"
printf 'issue=65\nstarting_base=%s\nreal_home=%s\nxdg_root=%s\narchive=%s\nrun_root=%s\n' \
  "$STARTING_BASE" "$REAL_HOME" "$XDG_ROOT" "$ARCHIVE" "$RUN_ROOT"
run_expect 0 "UTC clock" date -u +%Y-%m-%dT%H:%M:%SZ
run_expect 0 "kernel" uname -a
run_expect 0 "architecture" uname -m
run_expect 0 "macOS version" sw_vers
run_expect 0 "git version" git --version
run_expect 0 "brew version" "$BREW" --version
run_expect 0 "repository starting base" git -C "$REPO_ROOT" merge-base --is-ancestor "$STARTING_BASE" HEAD

section "pre-mutation ownership audit"
[[ -x "$MATTY" ]] || die "Matty binary is absent"
[[ ! -e "$PACKY" && ! -e "$PACKY_STATE" && ! -e "$PACKY_SOURCE" ]] || die "Packy already owns operator state"
[[ -f "$MATTY_STATE/config.json" && -d "$MATTY_STATE/backups" ]] || die "Matty state or recovery material is absent"
[[ ! -e "$MATTY_STATE/packs.json" && ! -e "$MATTY_STATE/packs.lock" ]] || die "Matty capability-pack state exists; deactivate exact pairs before continuing"
[[ -d "$MATTY_SOURCE" ]] || die "Matty Installed Source is absent"
[[ ! -e "$ARCHIVE" ]] || die "approved recovery destination already exists"
[[ -x "$BREW" && -d "$TAP/.git" ]] || die "Homebrew or tap checkout is unavailable"

run_capture 0 "Matty version" "$RUN_ROOT/matty-version.log" "$MATTY" --version
grep -qx 'matty version v0.1.6' "$RUN_ROOT/matty-version.log" || die "unexpected Matty version"
run_capture 0 "Matty doctor JSON" "$RUN_ROOT/matty-doctor.json" "$MATTY" doctor --json
jq -e '.summary.status == "healthy" and all(.checks[]; .severity == "PASS")' "$RUN_ROOT/matty-doctor.json" >/dev/null || die "Matty doctor is not fully healthy"
run_expect 0 "Matty formula inventory" "$BREW" list --versions matty
run_expect 1 "Packy formula absent" "$BREW" list --versions packy
run_expect 0 "Matty tap formula ownership" "$BREW" info --json=v2 matty
[[ "$(git -C "$TAP" rev-parse HEAD)" == "$MATTY_TAP_SHA" ]] || die "local tap no longer contains the bound Matty formula"
[[ -z "$(git -C "$TAP" status --porcelain)" && -f "$TAP/Formula/matty.rb" && ! -e "$TAP/Formula/packy.rb" ]] || die "local tap is dirty or has unexpected formulas"
verify_packy_tap_remote
run_expect 0 "prefetch bound Packy tap commit without changing worktree" git -C "$TAP" fetch origin "$PACKY_TAP_SHA"
run_expect 0 "bound Packy tap commit is local" git -C "$TAP" cat-file -e "$PACKY_TAP_SHA^{commit}"
[[ "$(git -C "$MATTY_SOURCE" rev-parse HEAD)" == "$MATTY_SOURCE_SHA" ]] || die "Matty Installed Source SHA drifted"
[[ "$(git -C "$MATTY_SOURCE" describe --tags --exact-match)" == v0.1.4 ]] || die "Matty Installed Source tag drifted"
[[ -z "$(git -C "$MATTY_SOURCE" status --porcelain)" ]] || die "Matty Installed Source is dirty"

python3 - "$MATTY_STATE/config.json" "$MATTY_SOURCE" <<'PY'
import json, os, sys
state, source = sys.argv[1:]
data = json.load(open(state))
skills = data.get("managed_skills", [])
if len(skills) != 23:
    raise SystemExit(f"managed skill count is {len(skills)}, want 23")
for skill in skills:
    link, recorded = skill["link_path"], skill["source_path"]
    if not recorded.startswith(source + os.sep) or not os.path.islink(link):
        raise SystemExit(f"invalid ownership record for {skill.get('name')}")
    target = os.path.realpath(link)
    if target != os.path.realpath(recorded):
        raise SystemExit(f"link target drift for {skill.get('name')}")
print("classic_state_links=23 all_recorded_links_match=true")
PY

run_capture 0 "Matty uninstall dry-run" "$RUN_ROOT/matty-uninstall-plan.log" "$MATTY" uninstall --dry-run
[[ "$(grep -c 'remove managed skill' "$RUN_ROOT/matty-uninstall-plan.log")" -eq 23 ]] || die "uninstall plan does not contain exactly 23 managed links"
grep -Fq "remove Matty state metadata ($MATTY_STATE/config.json)" "$RUN_ROOT/matty-uninstall-plan.log" || die "uninstall plan omits classic state"
grep -Fq "remove Codex Matty prompt markers" "$RUN_ROOT/matty-uninstall-plan.log" || die "uninstall plan omits Codex ownership"
grep -Fq "remove OpenCode Matty prompt reference" "$RUN_ROOT/matty-uninstall-plan.log" || die "uninstall plan omits OpenCode ownership"
! grep -Eiq 'backups|\.local/share/matty|engram' "$RUN_ROOT/matty-uninstall-plan.log" || die "uninstall plan reaches non-owned recovery/source/Engram state"

section "capability-pack intent audit"
mkdir -p "$RUN_ROOT/matty-v0.1.6"
git -C "$REPO_ROOT" archive v0.1.6 | tar -x -C "$RUN_ROOT/matty-v0.1.6"
MATTY_CATALOG="$RUN_ROOT/matty-v0.1.6/bundle/skills"
run_capture 1 "Matty aggregate pack status known external Engram blocker" "$RUN_ROOT/matty-pack-status.log" env MATTY_SKILLS_SOURCE="$MATTY_CATALOG" "$MATTY" pack status
grep -Fq 'OpenCode MCP server "engram" already exists with unmanaged settings' "$RUN_ROOT/matty-pack-status.log" || die "aggregate pack failure is not the accepted external Engram ownership"
for surface in codex opencode; do
  run_capture 0 "Matty semantic pack intent absent on $surface" "$RUN_ROOT/matty-pack-$surface.json" env MATTY_SKILLS_SOURCE="$MATTY_CATALOG" "$MATTY" pack status matty --surface "$surface" --json
  jq -e '.entries | length == 1 and .[0].intent.state == "absent"' "$RUN_ROOT/matty-pack-$surface.json" >/dev/null || die "Matty pack intent is not absent on $surface"
done
run_capture 0 "Engram Codex intent absent" "$RUN_ROOT/engram-codex.json" env MATTY_SKILLS_SOURCE="$MATTY_CATALOG" "$MATTY" pack status engram --surface codex --json
jq -e '.entries | length == 1 and .[0].intent.state == "absent"' "$RUN_ROOT/engram-codex.json" >/dev/null || die "Engram Codex intent is not absent"
run_capture 1 "Engram OpenCode accepted external ownership" "$RUN_ROOT/engram-opencode.log" env MATTY_SKILLS_SOURCE="$MATTY_CATALOG" "$MATTY" pack status engram --surface opencode --json
grep -Fq 'already exists with unmanaged settings' "$RUN_ROOT/engram-opencode.log" || die "Engram OpenCode failure is not the accepted external ownership"

normalized_host_fingerprint "$RUN_ROOT/host-before.json"
printf 'normalized_host_before='; cat "$RUN_ROOT/host-before.json"; printf '\n'
run_expect 0 "Engram version before" "$REAL_HOME/.local/bin/engram" version
run_capture 0 "Engram process before" "$RUN_ROOT/engram-process-before.log" pgrep -lf '/engram'

section "preserve non-owned recovery material"
mkdir -p "$(dirname "$ARCHIVE")" "$ARCHIVE"
ARCHIVE_CREATED=1
inventory_tree "$MATTY_STATE/backups" "$RUN_ROOT/source-inventory"
cp -a "$MATTY_STATE/backups/." "$ARCHIVE/"
inventory_tree "$ARCHIVE" "$RUN_ROOT/destination-inventory"
cmp "$RUN_ROOT/source-inventory" "$RUN_ROOT/destination-inventory" || die "recovery copy typed inventory differs from source"
manifest_tree "$MATTY_STATE/backups" "$RUN_ROOT/source-SHA256SUMS"
manifest_tree "$ARCHIVE" "$RUN_ROOT/destination-SHA256SUMS"
cmp "$RUN_ROOT/source-SHA256SUMS" "$RUN_ROOT/destination-SHA256SUMS" || die "recovery copy differs from source"
cp "$RUN_ROOT/source-SHA256SUMS" "$ARCHIVE/SHA256SUMS"
(cd "$ARCHIVE" && shasum -a 256 -c SHA256SUMS) || die "recovery archive manifest verification failed"
[[ "$(find "$MATTY_STATE/backups" -type f | wc -l | tr -d ' ')" -eq 3 ]] || die "unexpected recovery source file count"
[[ "$(find "$ARCHIVE" -type f ! -name SHA256SUMS | wc -l | tr -d ' ')" -eq 3 ]] || die "unexpected recovery destination file count"
run_expect 0 "recovery archive inventory" find "$ARCHIVE" -maxdepth 4 -type f -print
ARCHIVE_READY=1
PHASE=archive-verified

section "remove Matty-owned projections"
run_capture 0 "Matty uninstall final dry-run" "$RUN_ROOT/matty-uninstall-final-plan.log" "$MATTY" uninstall --dry-run
cmp "$RUN_ROOT/matty-uninstall-plan.log" "$RUN_ROOT/matty-uninstall-final-plan.log" || die "Matty uninstall plan changed after archive creation"
verify_packy_tap_remote
PHASE=matty-cleanup
run_expect 0 "Matty uninstall apply" "$MATTY" uninstall
[[ ! -e "$MATTY_STATE/config.json" ]] || die "Matty classic state remains"
[[ "$(find "$REAL_HOME/.agents/skills" -type l -exec sh -c 'for p do case "$(readlink "$p")" in *"/.local/share/matty"*) printf x;; esac; done' sh {} + | wc -c | tr -d ' ')" -eq 0 ]] || die "global links still target Matty Installed Source"
! grep -Fq '<!-- matty:' "$REAL_HOME/.codex/AGENTS.md" || die "Matty Codex product marker remains"
[[ ! -e "$XDG_ROOT/opencode/matty.md" ]] || die "Matty OpenCode prompt remains"
! grep -Fq 'matty.md' "$XDG_ROOT/opencode/opencode.json" || die "Matty OpenCode reference remains"
normalized_host_fingerprint "$RUN_ROOT/host-after-matty-uninstall.json"
cmp "$RUN_ROOT/host-before.json" "$RUN_ROOT/host-after-matty-uninstall.json" || die "Matty uninstall changed contributor-owned host configuration or Engram binary"

section "remove Matty formula and approved residuals"
run_expect 0 "fully qualified Matty formula uninstall without tap refresh" env HOMEBREW_NO_AUTO_UPDATE=1 "$BREW" uninstall yersonargotev/tap/matty
[[ -z "$(git -C "$MATTY_SOURCE" status --porcelain)" && "$(git -C "$MATTY_SOURCE" rev-parse HEAD)" == "$MATTY_SOURCE_SHA" ]] || die "Matty Installed Source changed before approved deletion"
rm -rf -- "$MATTY_SOURCE"
manifest_tree "$MATTY_STATE/backups" "$RUN_ROOT/source-after-uninstall-SHA256SUMS"
cmp "$RUN_ROOT/source-SHA256SUMS" "$RUN_ROOT/source-after-uninstall-SHA256SUMS" || die "original recovery material changed before legacy state deletion"
inventory_tree "$MATTY_STATE/backups" "$RUN_ROOT/source-after-uninstall-inventory"
cmp "$RUN_ROOT/source-inventory" "$RUN_ROOT/source-after-uninstall-inventory" || die "original recovery typed inventory changed before legacy state deletion"
if find "$MATTY_STATE" -mindepth 1 -maxdepth 1 ! -name backups -print -quit | grep -q .; then
  die "unapproved Matty state residual exists"
fi
rm -rf -- "$MATTY_STATE"

section "zero-active-Matty-residual gate"
run_expect 1 "Matty formula absent" "$BREW" list --versions matty
[[ ! -e "$MATTY" && ! -e "$MATTY_STATE" && ! -e "$MATTY_SOURCE" ]] || die "Matty binary, state, or Installed Source remains"
hash -r
! command -v matty >/dev/null 2>&1 || die "a live Matty executable remains on PATH"
[[ "$(find "$REAL_HOME/.agents/skills" -type l -exec sh -c 'for p do case "$(readlink "$p")" in *"/.local/share/matty"*) printf x;; esac; done' sh {} + | wc -c | tr -d ' ')" -eq 0 ]] || die "link into Matty Installed Source remains"
! grep -Fq '<!-- matty:' "$REAL_HOME/.codex/AGENTS.md" || die "live Matty Codex marker remains"
[[ ! -e "$XDG_ROOT/opencode/matty.md" ]] || die "live Matty OpenCode file remains"
! grep -Fq 'matty.md' "$XDG_ROOT/opencode/opencode.json" || die "live Matty OpenCode reference remains"
! grep -Eiq '\.local/share/matty|\.matty/config\.json' "$REAL_HOME/.codex/AGENTS.md" "$XDG_ROOT/opencode/opencode.json" || die "live Matty product path reference remains"
(cd "$ARCHIVE" && shasum -a 256 -c SHA256SUMS) || die "external recovery archive changed"

section "refresh tap and create fresh Packy ownership"
PHASE=packy-install
run_expect 0 "reset clean tap to published Packy formula" git -C "$TAP" reset --hard "$PACKY_TAP_SHA"
[[ "$(git -C "$TAP" rev-parse HEAD)" == "$PACKY_TAP_SHA" ]] || die "Packy tap commit is not the published v0.1.7 input"
[[ -z "$(git -C "$TAP" status --porcelain)" && ! -e "$TAP/Formula/matty.rb" && -f "$TAP/Formula/packy.rb" ]] || die "tap formula ownership is mixed or dirty"
run_expect 0 "fully qualified Packy formula install" env HOMEBREW_NO_AUTO_UPDATE=1 "$BREW" install yersonargotev/tap/packy
run_expect 0 "Packy formula inventory" "$BREW" list --versions packy
run_capture 0 "Packy version" "$RUN_ROOT/packy-version.log" "$PACKY" --version
grep -qx 'packy version v0.1.7' "$RUN_ROOT/packy-version.log" || die "unexpected Packy version"

unset MATTY_SKILLS_SOURCE PACKY_SKILLS_SOURCE
pushd "$RUN_ROOT" >/dev/null
run_expect 0 "Packy init" "$PACKY" init
[[ "$(git -C "$PACKY_SOURCE" describe --tags --exact-match)" == v0.1.7 ]] || die "Packy Installed Source is not v0.1.7"
[[ "$(git -C "$PACKY_SOURCE" rev-parse HEAD)" == "$PACKY_RELEASE_SHA" ]] || die "Packy Installed Source SHA is unexpected"
[[ -z "$(git -C "$PACKY_SOURCE" status --porcelain)" ]] || die "fresh Packy Installed Source is dirty"
run_capture 0 "Packy install dry-run" "$RUN_ROOT/packy-install-plan.log" "$PACKY" install --dry-run
run_expect 0 "Packy install apply" "$PACKY" install
run_expect 0 "Packy doctor" "$PACKY" doctor
run_capture 0 "Packy doctor JSON" "$RUN_ROOT/packy-doctor.json" "$PACKY" doctor --json
jq -e '.summary.status == "healthy" and all(.checks[]; .severity == "PASS")' "$RUN_ROOT/packy-doctor.json" >/dev/null || die "Packy doctor is not fully healthy"
run_expect 0 "Packy semantic pack list" "$PACKY" pack list
run_expect 0 "Packy semantic Matty pack show" "$PACKY" pack show matty
for surface in codex opencode; do
  run_capture 0 "Packy semantic Matty status on $surface" "$RUN_ROOT/packy-matty-$surface.json" "$PACKY" pack status matty --surface "$surface" --json
  jq -e '.entries | length == 1 and .[0].intent.state == "absent"' "$RUN_ROOT/packy-matty-$surface.json" >/dev/null || die "fresh semantic Matty intent is not absent on $surface"
done
run_expect 0 "Packy semantic Matty activation preview" "$PACKY" pack activate matty --surface codex --dry-run
popd >/dev/null

python3 - "$PACKY_STATE/config.json" "$PACKY_SOURCE" <<'PY'
import json, os, sys
state, source = sys.argv[1:]
data = json.load(open(state))
skills = data.get("managed_skills", [])
if len(skills) != 23:
    raise SystemExit(f"fresh managed skill count is {len(skills)}, want 23")
for skill in skills:
    link, recorded = skill["link_path"], skill["source_path"]
    if not recorded.startswith(source + os.sep) or not os.path.islink(link):
        raise SystemExit(f"invalid fresh ownership for {skill.get('name')}")
    if os.path.realpath(link) != os.path.realpath(recorded):
        raise SystemExit(f"fresh link target drift for {skill.get('name')}")
print("fresh_classic_state_links=23 all_sources_are_packy=true")
PY

section "preservation and authentic-history gates"
normalized_host_fingerprint "$RUN_ROOT/host-after.json"
printf 'normalized_host_after='; cat "$RUN_ROOT/host-after.json"; printf '\n'
cmp "$RUN_ROOT/host-before.json" "$RUN_ROOT/host-after.json" || die "contributor-owned host configuration or Engram binary changed"
run_expect 0 "Engram version after" "$REAL_HOME/.local/bin/engram" version
run_capture 0 "Engram process after" "$RUN_ROOT/engram-process-after.log" pgrep -lf '/engram'
(cd "$ARCHIVE" && shasum -a 256 -c SHA256SUMS) || die "external recovery archive failed final verification"
inventory_tree "$ARCHIVE" "$RUN_ROOT/archive-final-inventory" 1
cmp "$RUN_ROOT/source-inventory" "$RUN_ROOT/archive-final-inventory" || die "external recovery archive typed inventory changed"
run_expect 0 "historical Matty tag remains" bash -c "git ls-remote https://github.com/yersonargotev/matty.git refs/tags/v0.1.6 | grep -q '^68aec8969374fa9e9a6ea86b33e6719646b999f8'"
run_expect 0 "Packy release tag remains" bash -c "git ls-remote https://github.com/yersonargotev/packy.git refs/tags/v0.1.7 | grep -q '^283e726e9e1886d8b51e3222434022ac56f733eb'"
run_expect 0 "historical Matty release asset remains" curl -fsIL https://github.com/yersonargotev/packy/releases/download/v0.1.6/matty_v0.1.6_darwin_arm64
run_expect 0 "Packy release asset remains" curl -fsIL https://github.com/yersonargotev/packy/releases/download/v0.1.7/packy_v0.1.7_darwin_arm64

PHASE=complete
section "result"
printf '[%s] overall_failures=0 cutover=passed archive=%s\n' "$(stamp)" "$ARCHIVE"
