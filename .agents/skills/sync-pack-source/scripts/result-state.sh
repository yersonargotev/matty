#!/usr/bin/env bash

set -euo pipefail

run="${1:?usage: result-state.sh run.json artifact-directory live-pr.json remote-main.json}"
artifacts="${2:?usage: result-state.sh run.json artifact-directory live-pr.json remote-main.json}"
live_pr="${3:-}"
remote_main="${4:-}"
status="$(jq -er .status "$run")"

case "$status" in
  queued|pending|requested|waiting) echo "pendiente"; exit 0 ;;
  in_progress) echo "ejecución iniciada"; exit 0 ;;
  completed) ;;
  *) echo "bloqueada"; exit 0 ;;
esac

find_artifact() {
  find "$artifacts" -type f -name "$1" -print -quit
}

if [[ -n "$(find_artifact no-op.json)" ]]; then
  echo "sin cambios"
elif publication="$(find_artifact publication.json)"; [[ -n "$publication" ]] && jq -e '.decision_ready == true' "$publication" >/dev/null; then
  number="$(jq -er .pr_number "$publication")"
  base="$(jq -er .base_sha "$publication")"
  head="$(jq -er .head_sha "$publication")"
  branch="$(jq -er .branch_name "$publication")"
  expected_metadata="$(jq -er .managed_metadata_hash "$publication")"
  expected_state="$(jq -er .pr_state_sha256 "$publication")"
  observed_metadata="$(jq -jr '
    .title, "\u0000", (.body | (rindex("<!-- matty-pack-sync:") // length) as $i | .[0:$i] | rtrimstr("\n"))
  ' "$live_pr" | shasum -a 256 | cut -d ' ' -f 1)"
  if [[ -n "$live_pr" && -n "$remote_main" && "$observed_metadata" == "$expected_metadata" && "$observed_metadata" == "$expected_state" ]] &&
    jq -e --argjson number "$number" --arg base "$base" --arg head "$head" --arg branch "$branch" '
      .number == $number and .baseRefOid == $base and .headRefOid == $head and .headRefName == $branch and
    .state == "OPEN" and .isDraft == false
    ' "$live_pr" >/dev/null && jq -e --arg base "$base" '.sha == $base' "$remote_main" >/dev/null; then
    echo "decision-ready"
  else
    echo "bloqueada"
  fi
elif [[ -n "$(find_artifact inspection.json)" ]]; then
  echo "pendiente"
elif [[ -n "$(find_artifact operational-artifact.json)" ]]; then
  echo "bloqueada"
else
  echo "bloqueada"
fi
