#!/usr/bin/env bash

set -euo pipefail

run="${1:?usage: result-state.sh run.json artifact-directory}"
artifacts="${2:?usage: result-state.sh run.json artifact-directory}"
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
  echo "decision-ready"
elif [[ -n "$(find_artifact inspection.json)" ]]; then
  echo "pendiente"
elif [[ -n "$(find_artifact operational-artifact.json)" ]]; then
  echo "bloqueada"
else
  echo "bloqueada"
fi
