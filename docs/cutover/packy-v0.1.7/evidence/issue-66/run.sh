#!/usr/bin/env bash
set -Eeuo pipefail

EVIDENCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(git -C "$EVIDENCE_DIR" rev-parse --show-toplevel)"
RUN_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/packy-issue-66.XXXXXX")"
REAL_HOME="${HOME:?HOME must be set}"
XDG_ROOT="${XDG_CONFIG_HOME:-$REAL_HOME/.config}"
REAL_GOCACHE="$(go env GOCACHE)"
REAL_GOMODCACHE="$(go env GOMODCACHE)"
REAL_GOPATH="$(go env GOPATH)"
BREW=/opt/homebrew/bin/brew
PACKY=/opt/homebrew/bin/packy
PACKY_SOURCE="$REAL_HOME/.local/share/packy"
ARCHIVE="$REAL_HOME/Documents/dev/backups/matty-to-packy-cutover-20260717"

FROZEN_BASE=0e8971ad4ccacad5f99ec97d05ed963830b58070
ATOMIC_CANDIDATE=87718a145ecbee25556009218cff25806c67365a
MERGED_PACKY=283e726e9e1886d8b51e3222434022ac56f733eb
STARTING_MAIN=2ed52a16a88d3b150dcf0a2bbbc596eb72cb389f
TAP_SHA=ae1a2f979f073a5b07214d8f303c7ce5ff67d84d

trap 'status=$?; chmod -R u+w "$RUN_ROOT" 2>/dev/null || true; rm -rf "$RUN_ROOT"; exit "$status"' EXIT

stamp() { date -u +%Y-%m-%dT%H:%M:%SZ; }
section() { printf '\n===== %s | %s =====\n' "$(stamp)" "$1"; }
die() { printf '[%s] AUDIT_ERROR message=%s\n' "$(stamp)" "$*"; exit 1; }

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

assert_equal() {
  local label="$1" expected="$2" actual="$3"
  printf '[%s] assertion=%s expected=%q actual=%q\n' "$(stamp)" "$label" "$expected" "$actual"
  [[ "$actual" == "$expected" ]] || die "$label mismatch"
}

section "audit identity and tool versions"
printf 'issue=66\nstarting_origin_main=%s\nrun_root=%s\n' "$STARTING_MAIN" "$RUN_ROOT"
printf 'execution=./run.sh 2>&1 | tee <temporary-transcript>; gzip -n -9 <temporary-transcript> > transcript.log.gz\n'
run_expect 0 "UTC clock" date -u +%Y-%m-%dT%H:%M:%SZ
run_expect 0 "git version" git --version
run_expect 0 "Go version" go version
run_expect 0 "GitHub CLI version" gh --version
run_expect 0 "curl version" bash -o pipefail -c 'curl --version | head -1'
run_expect 0 "Homebrew version" "$BREW" --version
run_expect 0 "Packy version" "$PACKY" --version
run_expect 0 "audit harness digest" shasum -a 256 "$EVIDENCE_DIR/run.sh"
run_expect 0 "immutable issue request" cat "$EVIDENCE_DIR/issue-66.json"
run_expect 0 "audit candidate HEAD" git -C "$REPO_ROOT" rev-parse HEAD

section "bound repository identities"
cd "$REPO_ROOT"
run_expect 0 "clean committed audit checkout" bash -c 'test -z "$(git status --porcelain --untracked-files=no)"'
run_expect 0 "Packy origin" bash -c 'test "$(git remote get-url origin)" = git@github.com:yersonargotev/packy.git'
run_expect 0 "all linked worktree remotes" bash -o pipefail -c 'git worktree list --porcelain | awk '\''/^worktree / {sub(/^worktree /, ""); print}'\'' | while IFS= read -r worktree; do printf "worktree=%s origin=%s\n" "$worktree" "$(git -C "$worktree" remote get-url origin)"; test "$(git -C "$worktree" remote get-url origin)" = git@github.com:yersonargotev/packy.git; done'
run_expect 0 "Packy repository identity" gh repo view yersonargotev/packy --json nameWithOwner,defaultBranchRef,url,isArchived
run_expect 0 "historical repository redirect" bash -o pipefail -c 'test "$(curl -fsIL -o /dev/null -w "%{url_effective}" https://github.com/yersonargotev/matty)" = https://github.com/yersonargotev/packy'
run_expect 0 "frozen base commit exists" git cat-file -e "$FROZEN_BASE^{commit}"
run_expect 0 "atomic candidate commit exists" git cat-file -e "$ATOMIC_CANDIDATE^{commit}"
run_expect 0 "merged Packy commit exists" git cat-file -e "$MERGED_PACKY^{commit}"
run_expect 0 "starting main commit exists" git cat-file -e "$STARTING_MAIN^{commit}"
assert_equal "atomic merge parents" "$FROZEN_BASE $ATOMIC_CANDIDATE" "$(git show -s --format='%P' "$MERGED_PACKY")"
assert_equal "candidate and merge tree" "$(git rev-parse "$ATOMIC_CANDIDATE^{tree}")" "$(git rev-parse "$MERGED_PACKY^{tree}")"
assert_equal "immutable v0.1.7 tag" "$MERGED_PACKY" "$(git rev-parse 'v0.1.7^{commit}')"
run_expect 0 "current starting main descends from release" git merge-base --is-ancestor "$MERGED_PACKY" "$STARTING_MAIN"
run_expect 0 "atomic PR identity" gh pr view 67 --repo yersonargotev/packy --json baseRefOid,headRefOid,mergeCommit,state,mergedAt,url
run_expect 0 "module identity" bash -c 'test "$(sed -n "1s/^module //p" go.mod)" = github.com/yersonargotev/packy && test -f cmd/packy/main.go && test ! -e cmd/matty'

section "bound evidence integrity"
run_expect 0 "issue 56 baseline manifest" shasum -a 256 -c docs/cutover/packy-v0.1.7/evidence/issue-56/SHA256SUMS
run_expect 0 "issue 64 installation manifest" bash -c 'cd docs/cutover/packy-v0.1.7/evidence/issue-64 && shasum -a 256 -c SHA256SUMS'
run_expect 0 "issue 65 cutover manifest" bash -c 'cd docs/cutover/packy-v0.1.7/evidence/issue-65 && shasum -a 256 -c SHA256SUMS'
run_expect 0 "issue 60 candidate evidence" gh issue view 60 --repo yersonargotev/packy --comments
run_expect 0 "issue 62 merge and Pages evidence" gh issue view 62 --repo yersonargotev/packy --comments
run_expect 0 "issue 63 release and tap evidence" gh issue view 63 --repo yersonargotev/packy --comments
run_expect 0 "issue 64 disposable installation state" gh issue view 64 --repo yersonargotev/packy --json state,closedAt,url
run_expect 0 "issue 65 maintainer cutover state" gh issue view 65 --repo yersonargotev/packy --json state,closedAt,url

section "repository validation and current documentation"
export HOME="$RUN_ROOT/home"
export XDG_CONFIG_HOME="$RUN_ROOT/xdg"
export GOCACHE="$REAL_GOCACHE"
export GOMODCACHE="$REAL_GOMODCACHE"
export GOPATH="$REAL_GOPATH"
mkdir -p "$HOME" "$XDG_CONFIG_HOME"
run_expect 0 "Packy repository validation" ./scripts/validate-packy.sh
run_expect 0 "Go compatibility suite" go test ./...
run_expect 0 "repository whitespace" git diff --check "$STARTING_MAIN"...HEAD
unset HOME XDG_CONFIG_HOME
export HOME="$REAL_HOME"
export XDG_CONFIG_HOME="$XDG_ROOT"

section "CI workflows and freeze boundary"
run_expect 0 "candidate CI" bash -o pipefail -c 'gh run view 29632325438 --repo yersonargotev/packy --json headSha,conclusion,status,url | jq -e '\''(.headSha == "87718a145ecbee25556009218cff25806c67365a") and (.conclusion == "success") and (.status == "completed")'\'''
run_expect 0 "merged Packy CI" bash -o pipefail -c 'gh run view 29642069661 --repo yersonargotev/packy --json headSha,conclusion,status,url | jq -e '\''(.headSha == "283e726e9e1886d8b51e3222434022ac56f733eb") and (.conclusion == "success") and (.status == "completed")'\'''
run_expect 0 "merged Packy Pages" bash -o pipefail -c 'gh run view 29642312499 --repo yersonargotev/packy --json headSha,conclusion,status,url | jq -e '\''(.headSha == "283e726e9e1886d8b51e3222434022ac56f733eb") and (.conclusion == "success") and (.status == "completed")'\'''
run_expect 0 "release workflow" bash -o pipefail -c 'gh run view 29644676900 --repo yersonargotev/packy --json headSha,conclusion,status,url | jq -e '\''(.headSha == "283e726e9e1886d8b51e3222434022ac56f733eb") and (.conclusion == "success") and (.status == "completed")'\'''
run_expect 0 "starting main CI" bash -o pipefail -c 'gh run view 29651231126 --repo yersonargotev/packy --json headSha,conclusion,status,url | jq -e '\''(.headSha == "2ed52a16a88d3b150dcf0a2bbbc596eb72cb389f") and (.conclusion == "success") and (.status == "completed")'\'''
run_expect 0 "active workflow inventory" bash -o pipefail -c 'gh api repos/yersonargotev/packy/actions/workflows | jq -e '\''[.workflows[] | select(.state != "active")] | length == 0'\'''
run_expect 0 "no conflicting active runs" bash -o pipefail -c 'gh api "repos/yersonargotev/packy/actions/runs?status=in_progress&per_page=100" | jq -e '\''.total_count == 0'\'' && gh api "repos/yersonargotev/packy/actions/runs?status=queued&per_page=100" | jq -e '\''.total_count == 0'\'''

section "GitHub Pages schema suite"
run_expect 0 "Pages configuration" bash -o pipefail -c 'gh api repos/yersonargotev/packy/pages | jq -e '\''(.status == "built") and (.https_enforced == true) and (.source.branch == "main") and (.source.path == "/")'\'''
for local_schema in schemas/pack-source/v1.0.0/*.json; do
  name="$(basename "$local_schema")"
  url="$(jq -r '.["$id"]' "$local_schema")"
  hosted="$RUN_ROOT/$name"
  run_expect 0 "download hosted schema $name" curl -fsSL "$url" -o "$hosted"
  run_expect 0 "valid hosted JSON $name" jq -e . "$hosted"
  assert_equal "hosted schema id $name" "$url" "$(jq -r '.["$id"]' "$hosted")"
  run_expect 0 "hosted bytes match $name" cmp "$local_schema" "$hosted"
  run_expect 0 "hosted bytes match candidate $name" bash -c 'git show "$1:$2" | cmp - "$3"' _ "$ATOMIC_CANDIDATE" "$local_schema" "$hosted"
  run_expect 0 "hosted bytes match merged Packy $name" bash -c 'git show "$1:$2" | cmp - "$3"' _ "$MERGED_PACKY" "$local_schema" "$hosted"
  run_expect 0 "hosted bytes match starting main $name" bash -c 'git show "$1:$2" | cmp - "$3"' _ "$STARTING_MAIN" "$local_schema" "$hosted"
  run_expect 0 "hosted digest $name" shasum -a 256 "$hosted"
done

section "release downloads and Homebrew tap"
mkdir -p "$RUN_ROOT/release"
run_expect 0 "release metadata" gh release view v0.1.7 --repo yersonargotev/packy --json tagName,isDraft,isPrerelease,publishedAt,assets,url
run_expect 0 "download exact release" gh release download v0.1.7 --repo yersonargotev/packy --dir "$RUN_ROOT/release"
run_expect 0 "exact release asset names" bash -o pipefail -c 'cd "$1" && printf "%s\n" * | diff -u - "$2"' _ "$RUN_ROOT/release" <(printf '%s\n' checksums.txt packy_v0.1.7_darwin_amd64 packy_v0.1.7_darwin_arm64 packy_v0.1.7_linux_amd64 packy_v0.1.7_linux_arm64)
run_expect 0 "release checksum manifest" bash -c 'cd "$1" && shasum -a 256 -c checksums.txt' _ "$RUN_ROOT/release"
run_expect 0 "make downloaded native asset executable" chmod u+x "$RUN_ROOT/release/packy_v0.1.7_darwin_arm64"
run_expect 0 "released Packy executable" "$RUN_ROOT/release/packy_v0.1.7_darwin_arm64" --version
run_expect 0 "tap commit" bash -o pipefail -c 'gh api repos/yersonargotev/homebrew-tap/commits/main | jq -e '\''.sha == "ae1a2f979f073a5b07214d8f303c7ce5ff67d84d"'\'''
run_expect 0 "tap tree" bash -o pipefail -c 'gh api "repos/yersonargotev/homebrew-tap/git/trees/ae1a2f979f073a5b07214d8f303c7ce5ff67d84d?recursive=1" | jq -e '\''([.tree[].path | select(. == "Formula/packy.rb")] | length == 1) and ([.tree[].path | select(. == "Formula/matty.rb" or . == "formula_renames.json")] | length == 0)'\'''
run_expect 0 "download tap formula" bash -c 'gh api '\''repos/yersonargotev/homebrew-tap/contents/Formula/packy.rb?ref=ae1a2f979f073a5b07214d8f303c7ce5ff67d84d'\'' -H '\''Accept: application/vnd.github.raw+json'\'' > "$1"' _ "$RUN_ROOT/packy.rb"
run_expect 0 "formula release parity" python3 - "$RUN_ROOT/packy.rb" "$RUN_ROOT/release/checksums.txt" <<'PY'
import pathlib, re, sys
formula = pathlib.Path(sys.argv[1]).read_text()
checksums = {}
for line in pathlib.Path(sys.argv[2]).read_text().splitlines():
    digest, name = line.split()
    checksums[name] = digest
required = [
    "packy_v0.1.7_darwin_amd64", "packy_v0.1.7_darwin_arm64",
    "packy_v0.1.7_linux_amd64", "packy_v0.1.7_linux_arm64",
]
if 'homepage "https://github.com/yersonargotev/packy"' not in formula or 'version "0.1.7"' not in formula:
    raise SystemExit("formula identity/version mismatch")
if 'bin.install downloaded_binary => "packy"' not in formula or '"#{bin}/packy", "--version"' not in formula:
    raise SystemExit("formula binary/test mismatch")
for name in required:
    url = f"https://github.com/yersonargotev/packy/releases/download/v0.1.7/{name}"
    pattern = re.escape(f'url "{url}", using: :nounzip') + r'\s+sha256 "([0-9a-f]{64})"'
    match = re.search(pattern, formula)
    if not match or match.group(1) != checksums[name]:
        raise SystemExit(f"formula checksum mismatch: {name}")
print("formula_release_parity=passed assets=4")
PY

section "maintainer installation and preservation"
run_expect 0 "installed Packy formula" "$BREW" list --versions packy
run_expect 1 "Matty formula absent" "$BREW" list --versions matty
run_expect 0 "installed Packy binary digest" bash -o pipefail -c 'test "$(shasum -a 256 /opt/homebrew/bin/packy | awk '\''{print $1}'\'')" = "$(awk '\''$2 == "packy_v0.1.7_darwin_arm64" {print $1}'\'' "$1")"' _ "$RUN_ROOT/release/checksums.txt"
run_expect 0 "local tap exact and clean" bash -c 'tap="$(/opt/homebrew/bin/brew --repo yersonargotev/tap)" && test "$(git -C "$tap" rev-parse HEAD)" = "$1" && test -z "$(git -C "$tap" status --porcelain)"' _ "$TAP_SHA"
run_expect 0 "zero direct Matty residuals" bash -c 'test ! -e /opt/homebrew/bin/matty && test ! -e "$1/.matty" && test ! -e "$1/.local/share/matty" && test ! -e "$2/opencode/matty.md"' _ "$REAL_HOME" "$XDG_ROOT"
run_expect 0 "fresh Packy source" bash -c 'test "$(git -C "$1" describe --tags --exact-match)" = v0.1.7 && test "$(git -C "$1" rev-parse HEAD)" = "$2" && test -z "$(git -C "$1" status --porcelain)"' _ "$PACKY_SOURCE" "$MERGED_PACKY"
run_expect 0 "fresh Packy ownership links" python3 - "$REAL_HOME/.packy/config.json" "$PACKY_SOURCE" <<'PY'
import json, os, sys
state, source = sys.argv[1:]
skills = json.load(open(state)).get("managed_skills", [])
if len(skills) != 23:
    raise SystemExit(f"managed skill count is {len(skills)}, want 23")
for skill in skills:
    if not skill["source_path"].startswith(source + os.sep) or not os.path.islink(skill["link_path"]):
        raise SystemExit(f"invalid ownership for {skill.get('name')}")
    if os.path.realpath(skill["link_path"]) != os.path.realpath(skill["source_path"]):
        raise SystemExit(f"link drift for {skill.get('name')}")
print("fresh_classic_state_links=23 all_sources_are_packy=true")
PY
run_expect 0 "healthy Packy doctor" bash -o pipefail -c '/opt/homebrew/bin/packy doctor --json | jq -e '\''(.summary.status == "healthy") and all(.checks[]; .severity == "PASS")'\'''
run_expect 0 "semantic Matty pack list" "$PACKY" pack list
run_expect 0 "semantic Matty pack show" "$PACKY" pack show matty
run_expect 0 "semantic Matty Codex status" "$PACKY" pack status matty --surface codex --json
run_expect 0 "semantic Matty OpenCode status" "$PACKY" pack status matty --surface opencode --json
run_expect 0 "semantic Matty activation preview" "$PACKY" pack activate matty --surface codex --dry-run
run_expect 0 "contributor Codex config" bash -c 'test -f "$1/.codex/config.toml" && grep -Eq '\''^\[mcp_servers\.engram\]$'\'' "$1/.codex/config.toml" && grep -Eq '\''^\[marketplaces\.engram\]$'\'' "$1/.codex/config.toml" && grep -Eq '\''^\[plugins\."engram@engram"\]$'\'' "$1/.codex/config.toml"' _ "$REAL_HOME"
run_expect 0 "Codex config digest" shasum -a 256 "$REAL_HOME/.codex/config.toml"
run_expect 0 "Engram version" "$REAL_HOME/.local/bin/engram" version
run_expect 0 "Engram process" pgrep -lf '/engram'
run_expect 0 "recovery manifest" bash -c 'cd "$1" && shasum -a 256 -c SHA256SUMS' _ "$ARCHIVE"
run_expect 0 "recovery typed inventory" python3 - "$ARCHIVE" <<'PY'
import os, stat, sys
root = sys.argv[1]
directories = files = 0
for current, names, filenames in os.walk(root, topdown=True, followlinks=False):
    for name in names + filenames:
        path = os.path.join(current, name)
        if os.path.relpath(path, root) == "SHA256SUMS":
            continue
        mode = os.lstat(path).st_mode
        if stat.S_ISLNK(mode) or not (stat.S_ISDIR(mode) or stat.S_ISREG(mode)):
            raise SystemExit(f"unsupported recovery entry: {path}")
        directories += stat.S_ISDIR(mode)
        files += stat.S_ISREG(mode)
if (directories, files) != (6, 3):
    raise SystemExit(f"recovery inventory is {directories}/{files}, want 6/3")
print("recovery_typed_inventory=6_directories,3_regular_files,0_special_entries")
PY
run_expect 0 "historical Matty tag" bash -o pipefail -c 'git ls-remote https://github.com/yersonargotev/matty.git refs/tags/v0.1.6 | grep -q "^68aec8969374fa9e9a6ea86b33e6719646b999f8"'
run_expect 0 "historical Matty release asset" curl -fsIL -o /dev/null https://github.com/yersonargotev/packy/releases/download/v0.1.6/matty_v0.1.6_darwin_arm64

section "final result"
printf '[%s] overall_failures=0 final_audit=passed availability_candidate=ready automation_candidate=ready\n' "$(stamp)"
