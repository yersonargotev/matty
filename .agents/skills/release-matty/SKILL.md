---
name: release-matty
description: Release Matty safely. Use when the user asks to release Matty, publish GitHub artifacts, update the Homebrew formula, or recover a failed release workflow.
---

# Release Matty

Run a **release gate**: no tag or success announcement passes it until every
required validation and publication check is observable. Treat
[`docs/release.md`](../../../docs/release.md) and
[`.github/workflows/release.yml`](../../../.github/workflows/release.yml) as
the current release contract; do not duplicate their version, asset, or
workflow details here.

## 1. Establish the candidate

1. Read the release contract and inspect the release workflow before making a
   mutation.
2. Fetch `origin` with tags and pruning. Confirm local `main` is clean and at
   the same commit as `origin/main`.
3. Find the latest valid `v0.x.y` tag and GitHub release. Use a user-specified
   version when supplied; otherwise select the next patch version and state it
   before tagging. Stop for clarification if the requested version is not a
   valid, monotonically newer release tag.
4. Confirm GitHub authentication and the workflow's required release secret.

**Complete when:** the exact version and immutable `main` commit are known, and
the release contract can run from that commit.

## 2. Prove the candidate

1. Run `go test ./...`.
2. Run the package-install lifecycle smoke test named in the release contract.
   It must use disposable `HOME` and `XDG_CONFIG_HOME`; never validate against
   the operator's real configuration.
3. Inspect the latest merged PR checks when the candidate contains a recent PR.

**Complete when:** every required test and applicable CI check passes. Do not
create or move a release tag after a failed validation.

## 3. Publish once

1. Confirm the chosen tag does not already exist locally or on `origin`.
2. Create the exact tag at the verified `main` commit and push only that tag.
3. Locate the tag-triggered Release workflow and wait for a terminal result.
4. If it fails, preserve the run URL and failed step, diagnose the cause, and
   repair through a new commit/tag or the documented manual-dispatch recovery
   path. Never force-move or reuse a published tag.

**Complete when:** the workflow reports `completed/success` for the exact tag.

## 4. Verify the published contract

1. Confirm the GitHub release targets the chosen tag and has every artifact and
   checksum entry required by the release contract.
2. Download the checksum manifest and validate the host-platform release binary
   against it. Run that binary's `--version` and require the chosen version.
3. Read `yersonargotev/homebrew-tap/Formula/matty.rb` through GitHub and confirm
   its version, URLs, and checksums match the release manifest.
4. Confirm the working tree remains clean. Report the tag, commit, release URL,
   workflow URL/run ID, validations, and tap commit.

Use a real `brew install` only when the user explicitly requests that external
package-manager check or supplies a controlled test environment; the required
local package-install smoke test is the default release gate.

**Complete when:** artifacts, checksums, release binary, and Homebrew formula
all agree on one version, with evidence recorded in the final report.
