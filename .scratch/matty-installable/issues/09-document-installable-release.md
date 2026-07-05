# 09 — Document installable release path

Type: task
Status: open
Blocked by: 03, 04, 08

## Question

Document how maintainers publish Matty and how users install/use it from Homebrew/GitHub Releases.

## Acceptance criteria

- Adds `docs/release.md` or equivalent with first-release checklist.
- README quickstart includes `brew install yersonargotev/tap/matty`, `matty init` if required, `matty install --dry-run`, and `matty install`.
- Documents `HOMEBREW_TAP_TOKEN` maintainer setup.
- Documents sandboxed package-install smoke test expectations.
- Clarifies whether Linux artifacts are supported or merely built for future use.
