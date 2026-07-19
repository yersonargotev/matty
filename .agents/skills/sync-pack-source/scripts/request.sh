#!/usr/bin/env bash

request_digest() {
  jq -cS . "$1" | shasum -a 256 | cut -d ' ' -f 1
}

workflow_inputs() {
  local request="$1"
  local digest
  digest="$(request_digest "$request")"
  jq --arg request_digest "$digest" '
    del(.schema_version)
    | with_entries(.value |= if type == "object" or type == "array" then tojson else tostring end)
    | if has("human_evidence") then .human_evidence_json=.human_evidence | del(.human_evidence) else . end
    | if has("registration") then .registration_json=.registration | del(.registration) else . end
    | .request_digest=$request_digest
  ' "$request"
}
