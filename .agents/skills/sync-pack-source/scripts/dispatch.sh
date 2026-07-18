#!/usr/bin/env bash

set -euo pipefail

request="${1:?usage: dispatch.sh canonical-request.json}"
. "$(dirname "${BASH_SOURCE[0]}")/request.sh"

workflow_inputs "$request" |
  gh workflow run .github/workflows/sync-pack-source.yml \
    --repo yersonargotev/packy \
    --ref main \
    --json
