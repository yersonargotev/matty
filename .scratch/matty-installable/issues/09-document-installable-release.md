# 09 — Document installable release path

Type: task
Status: resolved
Blocked by: 03, 04, 08

## Question

Document how maintainers publish Matty and how users install/use it from Homebrew/GitHub Releases.

## Acceptance criteria

- Adds `docs/release.md` or equivalent with first-release checklist.
- README quickstart includes `brew install yersonargotev/tap/matty`, `matty init` if required, `matty install --dry-run`, and `matty install`.
- Documents `HOMEBREW_TAP_TOKEN` maintainer setup.
- Documents sandboxed package-install smoke test expectations.
- Clarifies whether Linux artifacts are supported or merely built for future use.

## Answer

Resolved by `docs/release.md` and the README quickstart. The release docs give maintainers a first-release checklist, tag/manual-dispatch flow, `HOMEBREW_TAP_TOKEN` setup, release artifact contract, and sandboxed package-install smoke expectations. The README now shows the package-installed user path: `brew install yersonargotev/tap/matty`, `matty init`, `matty install --dry-run`, and `matty install`. The docs clarify that Matty v0 is macOS-first: Linux artifacts are built, checksummed, and represented in the formula for future support, but Linux is not the first installable release golden path.
