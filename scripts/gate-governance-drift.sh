#!/usr/bin/env bash

set -euo pipefail

boundary=
repo=
ref=
commit=
workflow=
output_dir=
while (($#)); do
  case "$1" in
    --boundary) boundary="${2:-}"; shift 2 ;;
    --repo) repo="${2:-}"; shift 2 ;;
    --ref) ref="${2:-}"; shift 2 ;;
    --commit) commit="${2:-}"; shift 2 ;;
    --workflow) workflow="${2:-}"; shift 2 ;;
    --output-dir) output_dir="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ "$boundary" == promotion || "$boundary" == publication ]] || {
  echo "--boundary must be promotion or publication" >&2
  exit 2
}
[[ -n "$repo" && -n "$ref" && -n "$commit" && -n "$workflow" && -n "$output_dir" ]] || {
  echo "repo, ref, commit, workflow, and output-dir are required" >&2
  exit 2
}

mkdir -p "$output_dir"
workflow_sha="$(git rev-parse "HEAD:$workflow")"
./scripts/collect-governance-drift.sh \
  --repo "$repo" \
  --ref "$ref" \
  --commit "$commit" \
  --workflow-sha "$workflow_sha" \
  --output "$output_dir/observation.json"
go run ./internal/tools/governancedrift \
  --mode evaluate \
  --contract docs/governance/expected-state.v1.json \
  --observation "$output_dir/observation.json" \
  --output "$output_dir/evaluation.json"
./scripts/project-governance-drift-issues.sh \
  --repo "$repo" \
  --output "$output_dir/canonical-issues.json"
jq '[.[]|select(.open)|{
  number,
  boundaries,
  exact_evidence_human_classified
}]' "$output_dir/canonical-issues.json" >"$output_dir/blocking-issues.json"
go run ./internal/tools/governancedrift \
  --mode gate \
  --evaluation "$output_dir/evaluation.json" \
  --blocking-issues "$output_dir/blocking-issues.json" \
  --boundary "$boundary" \
  --repository "$repo" \
  --ref "$ref" \
  --commit "$commit" \
  --workflow-sha "$workflow_sha" \
  --now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --max-age 1h \
  --output "$output_dir/gate.json"
