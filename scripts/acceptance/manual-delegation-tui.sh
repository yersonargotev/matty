#!/bin/sh
set -eu

# Runs the packed candidate on the certified Pi/target with an isolated HOME.
# The generated Markdown records artifact/version/target evidence but leaves all
# subjective observations unchecked for the human operator to complete.
repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
evidence=${1:-"$repository_root/docs/acceptance/delegation-tui-manual.md"}

MATTY_MANUAL_TUI=1 \
MATTY_MANUAL_EVIDENCE="$evidence" \
node "$repository_root/scripts/acceptance/t10-delegation-tui.mjs"
