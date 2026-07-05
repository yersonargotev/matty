# 05 — Add release artifact builder

Type: task
Status: resolved
Blocked by: 02

## Question

Add `scripts/build-release-artifacts.sh` for Matty, based on the dots script, to build release binaries and `checksums.txt` from a `v0.x` tag.

## Acceptance criteria

- Accepts only `v0.x` tags such as `v0.1.0` and rejects malformed/non-v0 versions before building.
- Builds raw executable artifacts named `matty_<version>_<goos>_<goarch>`.
- Injects `internal/version.Value=<tag>` with ldflags.
- Produces a standard SHA-256 `checksums.txt` manifest.
- Has tests equivalent to dots release automation tests for accepted/rejected versions and checksum completeness.


## Answer

Added `scripts/build-release-artifacts.sh` for Matty, following the dots raw-binary release pattern while enforcing exact `v0.x.y` tags before any build starts. The script cross-compiles `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `linux/arm64` artifacts named `matty_<version>_<goos>_<goarch>`, injects `github.com/yersonargotev/matty/internal/version.Value=<tag>` via ldflags, and writes a standard SHA-256 `checksums.txt` manifest.

Added `internal/release` automation tests covering accepted/rejected versions, pre-build rejection for malformed tags, ldflags injection, expected artifact names, and exact checksum completeness.
