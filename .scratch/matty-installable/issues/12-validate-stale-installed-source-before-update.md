# 12 — Validate stale Installed Source before update dry-runs

Type: task
Status: resolved
Blocked by: 11

## Question

`matty update` intentionally does not mutate or upgrade the Installed Source, but
package-installed runs should still detect when the default Installed Source is
missing or stale relative to the running release before previewing or applying a
managed workflow refresh.

## Acceptance criteria

- `matty update --dry-run` does not mutate `~/.local/share/matty`.
- When the default Installed Source is stale relative to a release-version binary,
  `matty update --dry-run` fails with guidance to run `matty init` instead of
  silently planning from the stale bundle.
- `matty update` applies the same stale-source guard before running external
  commands or writing managed artifacts.
- Explicit dev/test seams such as `MATTY_SKILLS_SOURCE` remain usable without
  requiring release-tag validation.
- Tests sandbox `HOME`, `XDG_CONFIG_HOME`, and the default Installed Source.

## Answer

`matty update` now validates the default package-installed Source before it
loads state, builds a plan, runs external commands, or writes managed artifacts.
For release-version binaries (`v*`), the guard compares the default Installed
Source checkout's `HEAD` with the running release tag and fails stale or invalid
checkouts with guidance to run `matty init`.

The guard is read-only. `matty update --dry-run` fails without mutating
`~/.local/share/matty`, and `matty update` applies the same guard before any
`brew`, `engram`, symlink, prompt, or state work can run.

The behavior is scoped to the default package-installed fallback. Explicit
`MATTY_SKILLS_SOURCE` usage and repository checkout discovery remain
development/test seams and do not require release-tag validation.

Implementation notes:

- `internal/cli/paths.go` records whether skill resolution used the default
  Installed Source fallback.
- `internal/bootstrap/bootstrap.go` exposes the read-only
  `ValidateInstalledSourceRef` seam so `internal/cli` does not learn git details.
- `internal/cli/root.go` calls the guard from `matty update` before planning.
- `internal/cli/root_test.go` covers stale default source failures for
  `update --dry-run` and `update`, plus the `MATTY_SKILLS_SOURCE` bypass.
- `internal/release/package_install_smoke_test.go` exercises a release-like
  binary initialized at an older tag and proves stale `update --dry-run` fails
  without mutating the sandboxed home or running external commands.
