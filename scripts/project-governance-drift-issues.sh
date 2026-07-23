#!/usr/bin/env bash

set -euo pipefail

repo=
output=
while (($#)); do
  case "$1" in
    --repo) repo="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[[ -n "$repo" && -n "$output" ]] || {
  echo "--repo and --output are required" >&2
  exit 2
}

GH_BIN="${GH_BIN:-gh}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
title='Packy governance drift detected'
"$GH_BIN" issue list --repo "$repo" --state all \
  --search "\"$title\" in:title" --limit 100 \
  --json number,state,body >"$tmp/issues.json"
: >"$tmp/projected.jsonl"

while IFS= read -r issue; do
  number="$(jq -r .number <<<"$issue")"
  state="$(jq -r .state <<<"$issue")"
  body="$(jq -r .body <<<"$issue")"
  canonical_key="$(jq -r 'try (.body | capture("key: (?<value>[^\\n]+)").value) catch ""' <<<"$issue")"
  evidence_digest="$(jq -r 'try (.body | capture("evidence: (?<value>sha256:[0-9a-f]{64})").value) catch ""' <<<"$issue")"
  boundaries_text="$(jq -r 'try (.body | capture("boundaries: (?<value>[^\\n]+)").value) catch ""' <<<"$issue")"
  if [[ "$boundaries_text" == promotion || "$boundaries_text" == publication ]]; then
    boundaries="$(jq -cn --arg value "$boundaries_text" '[$value]')"
  elif [[ "$boundaries_text" == promotion,publication || "$boundaries_text" == publication,promotion ]]; then
    boundaries='["promotion","publication"]'
  else
    boundaries='["promotion","publication"]'
  fi

  classified=false
  if [[ "$state" == OPEN && "$canonical_key" == packy-governance-drift-v1 && -n "$evidence_digest" ]]; then
    "$GH_BIN" api "repos/$repo/issues/$number/comments?per_page=100" --paginate \
      --jq '[.[]|{body,author_association}]' >"$tmp/comments-$number.json"
    marker="$(printf '<!-- packy-governance-classification\nevidence: %s\nclassification: reviewed\n-->' "$evidence_digest")"
    classified="$(jq --arg marker "$marker" 'any(.[]; .author_association=="OWNER" and .body==$marker)' "$tmp/comments-$number.json")"
  fi

  jq -cn \
    --argjson number "$number" \
    --arg canonical_key "$canonical_key" \
    --argjson open "$([[ "$state" == OPEN ]] && echo true || echo false)" \
    --arg evidence_digest "$evidence_digest" \
    --argjson boundaries "$boundaries" \
    --argjson classified "$classified" \
    '{
      number:$number,
      canonical_key:$canonical_key,
      open:$open,
      evidence_digest:$evidence_digest,
      boundaries:$boundaries,
      exact_evidence_human_classified:$classified
    }' >>"$tmp/projected.jsonl"
done < <(jq -c '.[]' "$tmp/issues.json")

mkdir -p "$(dirname "$output")"
jq -s 'sort_by(.number)' "$tmp/projected.jsonl" >"$output"
