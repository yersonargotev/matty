# 08 — Add release workflow and tap update

Type: task
Status: resolved
Blocked by: 05, 06

## Question

Add GitHub Actions release automation for Matty, borrowing the dots release workflow ordering so GitHub Releases and the Homebrew tap cannot drift.

## Acceptance criteria

- Triggers on `v0.*` tags and manual dispatch with an existing tag input.
- Checks out the release tag with full history and builds artifacts/checksums.
- Requires `HOMEBREW_TAP_TOKEN` before creating/updating release assets.
- Checks out `yersonargotev/homebrew-tap` with the token and writes `Formula/matty.rb`.
- Prepares and dry-run pushes the tap commit before mutating the GitHub Release.
- Creates GitHub Release with generated notes if missing, uploads `dist/* --clobber`, then pushes the prepared tap commit.
- Release automation tests verify the required ordering.

## Answer

Resolved by `.github/workflows/release.yml`. The release workflow publishes Matty from existing `v0.x.y` release tags, builds `dist/*` artifacts and checksums from the checked-out release tag, requires `HOMEBREW_TAP_TOKEN`, prepares and dry-run verifies the `yersonargotev/homebrew-tap` update for `Formula/matty.rb`, creates the GitHub Release with generated notes if needed, uploads assets with `--clobber`, and only then pushes the prepared tap commit. Release automation tests in `internal/release/release_automation_test.go` verify the required ordering.
