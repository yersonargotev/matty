# 05 — Add release artifact builder

Type: task
Status: open
Blocked by: 02

## Question

Add `scripts/build-release-artifacts.sh` for Matty, based on the dots script, to build release binaries and `checksums.txt` from a `v0.x` tag.

## Acceptance criteria

- Accepts only `v0.x` tags such as `v0.1.0` and rejects malformed/non-v0 versions before building.
- Builds raw executable artifacts named `matty_<version>_<goos>_<goarch>`.
- Injects `internal/version.Value=<tag>` with ldflags.
- Produces a standard SHA-256 `checksums.txt` manifest.
- Has tests equivalent to dots release automation tests for accepted/rejected versions and checksum completeness.
