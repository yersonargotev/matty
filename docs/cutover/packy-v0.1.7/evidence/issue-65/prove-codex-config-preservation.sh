#!/bin/sh
set -eu

ENGRAM_BIN=${ENGRAM_BIN:-/opt/homebrew/bin/engram}
CODEX_BIN=${CODEX_BIN:-/opt/homebrew/bin/codex}
PYTHON_BIN=${PYTHON_BIN:-/opt/homebrew/bin/python3}
SANDBOX=$(mktemp -d "${TMPDIR:-/tmp}/packy-issue-65-codex.XXXXXX")

cleanup() {
    "$PYTHON_BIN" - "$SANDBOX" <<'PY'
import shutil
import sys

shutil.rmtree(sys.argv[1])
PY
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$SANDBOX/.codex" "$SANDBOX/.config"
cat >"$SANDBOX/.codex/config.toml" <<'TOML'
model = "gpt-5.6-sol"
model_reasoning_effort = "medium"
model_verbosity = "low"
personality = "pragmatic"
sandbox_mode = "workspace-write"
approval_policy = "never"
notify = ["/usr/bin/printf", "issue65-sentinel"]

[tui]
status_line = ["model-name", "current-dir"]
status_line_use_colors = false
theme = "ansi"

[features]
multi_agent = false
js_repl = true

[agents]
max_depth = 2
max_threads = 3

[mcp_servers.unrelated]
command = "/usr/bin/printf"
args = ["mcp-sentinel"]

[mcp_servers.unrelated.env]
ISSUE65_SENTINEL = "preserve-me"

[projects."/tmp/issue65-contributor-project"]
trust_level = "trusted"

[marketplaces.unrelated]
last_updated = "2000-01-02T03:04:05Z"
source_type = "git"
source = "https://example.invalid/contributor.git"

[plugins."unrelated@unrelated"]
enabled = false

[desktop]
followUpQueueMode = "queue"
keepRemoteControlAwakeWhilePluggedIn = false
dock-icon-preference = "default"

[notice]
hide_rate_limit_model_nudge = true

[shell_environment_policy.set]
ISSUE65_SENTINEL = "preserve-me-too"
TOML

snapshot_contributor_values() {
    "$PYTHON_BIN" - "$SANDBOX/.codex/config.toml" <<'PY'
import json
import sys
import tomllib

with open(sys.argv[1], "rb") as source:
    config = tomllib.load(source)

snapshot = {
    "root": {key: config[key] for key in (
        "model", "model_reasoning_effort", "model_verbosity", "personality",
        "sandbox_mode", "approval_policy", "notify",
    )},
    "tui": config["tui"],
    "features": config["features"],
    "agents": config["agents"],
    "unrelated_mcp": config["mcp_servers"]["unrelated"],
    "unrelated_project": config["projects"]["/tmp/issue65-contributor-project"],
    "unrelated_marketplace": config["marketplaces"]["unrelated"],
    "unrelated_plugin": config["plugins"]["unrelated@unrelated"],
    "desktop": config["desktop"],
    "notice": config["notice"],
    "shell_environment_policy": config["shell_environment_policy"],
}
print(json.dumps(snapshot, sort_keys=True, separators=(",", ":")))
PY
}

verify_engram_projection() {
    "$PYTHON_BIN" - "$SANDBOX/.codex/config.toml" "$SANDBOX" "$ENGRAM_BIN" <<'PY'
import os
import sys
import tomllib

with open(sys.argv[1], "rb") as source:
    config = tomllib.load(source)

root = sys.argv[2]
assert os.path.realpath(config["model_instructions_file"]) == os.path.realpath(os.path.join(root, ".codex", "engram-instructions.md"))
assert os.path.realpath(config["experimental_compact_prompt_file"]) == os.path.realpath(os.path.join(root, ".codex", "engram-compact-prompt.md"))
engram = config["mcp_servers"]["engram"]
assert os.path.realpath(engram["command"]) == os.path.realpath(sys.argv[3])
assert engram["args"] == ["mcp", "--tools=agent"]
PY
}

snapshot_contributor_values >"$SANDBOX/contributor-before.json"
shasum -a 256 "$SANDBOX/.codex/config.toml" | awk '{print "config_byte_sha256_before=" $1}'
shasum -a 256 "$SANDBOX/contributor-before.json" | awk '{print "contributor_semantic_sha256_before=" $1}'
printf 'engram_version='; "$ENGRAM_BIN" version
printf 'codex_version='; "$CODEX_BIN" --version

HOME="$SANDBOX" \
XDG_CONFIG_HOME="$SANDBOX/.config" \
CODEX_HOME="$SANDBOX/.codex" \
PATH="$(dirname "$CODEX_BIN"):$(dirname "$ENGRAM_BIN"):/usr/bin:/bin:/usr/sbin:/sbin" \
    "$ENGRAM_BIN" setup codex >"$SANDBOX/setup.log" 2>&1

snapshot_contributor_values >"$SANDBOX/contributor-after.json"
verify_engram_projection
cmp "$SANDBOX/contributor-before.json" "$SANDBOX/contributor-after.json"
test -f "$SANDBOX/.codex/engram-instructions.md"
test -f "$SANDBOX/.codex/engram-compact-prompt.md"

shasum -a 256 "$SANDBOX/.codex/config.toml" | awk '{print "config_byte_sha256_after=" $1}'
shasum -a 256 "$SANDBOX/contributor-after.json" | awk '{print "contributor_semantic_sha256_after=" $1}'
printf '%s\n' 'setup_output_begin'
"$PYTHON_BIN" - "$SANDBOX/setup.log" "$SANDBOX" <<'PY'
import os
import sys

text = open(sys.argv[1], encoding="utf-8").read()
root = sys.argv[2]
candidates = {root, os.path.realpath(root)}
for candidate in tuple(candidates):
    if candidate.startswith("/private/"):
        candidates.add(candidate[len("/private"):])
    elif candidate.startswith("/var/") or candidate.startswith("/tmp/"):
        candidates.add("/private" + candidate)
for candidate in sorted(candidates, key=len, reverse=True):
    text = text.replace(candidate, "$SANDBOX")
print(text, end="")
PY
printf '%s\n' 'setup_output_end' 'engram_projection=present' 'contributor_semantics=preserved' 'result=passed'
