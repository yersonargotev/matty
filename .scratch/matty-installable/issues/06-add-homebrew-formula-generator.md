# 06 — Add Homebrew formula generator

Type: task
Status: open
Blocked by: 05

## Question

Add `scripts/generate-homebrew-formula.sh` for Matty, based on the dots generator, so the release workflow can update `yersonargotev/homebrew-tap/Formula/matty.rb` from the same checksum manifest uploaded to GitHub Releases.

## Acceptance criteria

- Generates `class Matty < Formula` with correct desc/homepage/version.
- Selects the correct artifact for supported OS/arch combinations.
- Uses SHA-256 entries from `checksums.txt` and rejects missing, duplicate, or unexpected artifacts.
- Installs the raw executable as `matty` and includes a `brew test` command using `matty --version`.
- Tests assert formula content and validation failure modes.
