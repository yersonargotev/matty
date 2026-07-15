#!/usr/bin/env bash

set -euo pipefail

request="${1:?usage: attach.sh canonical-request.json runs.json}"
runs="${2:?usage: attach.sh canonical-request.json runs.json}"
source_id="$(jq -er .source_id "$request")"
request_digest="$(jq -cS . "$request" | shasum -a 256 | cut -d ' ' -f 1)"
run_name="sync-pack-source / $source_id / $request_digest"

matches="$(jq -c --arg name "$run_name" '[.[] | select(
  .displayTitle == $name and
  (.status == "queued" or .status == "in_progress" or .status == "pending" or
   .status == "requested" or .status == "waiting")
)]' "$runs")"
count="$(jq length <<<"$matches")"
if [[ "$count" -gt 1 ]]; then
  echo "multiple active or pending runs expose the same request identity" >&2
  exit 2
fi
if [[ "$count" -eq 0 ]]; then
  exit 1
fi
jq -er '.[0].url' <<<"$matches"
