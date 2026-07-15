#!/usr/bin/env bash

set -euo pipefail

request="${1:?usage: dispatch.sh canonical-request.json}"
request_digest="$(jq -cS . "$request" | shasum -a 256 | cut -d ' ' -f 1)"

jq --arg request_digest "$request_digest" '
  del(.schema_version)
  | with_entries(.value |= if type == "object" or type == "array" then tojson else tostring end)
  | if has("human_evidence") then .human_evidence_json=.human_evidence | del(.human_evidence) else . end
  | .request_digest=$request_digest
' "$request" |
  gh workflow run .github/workflows/sync-pack-source.yml \
    --repo yersonargotev/matty \
    --ref main \
    --json
