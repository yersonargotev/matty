# 04 — Resolve skill source for package installs

Type: task
Status: resolved
Blocked by: 01, 03

## Question

Update Matty path resolution so package-installed commands use the initialized installed source by default, while repo checkouts and `MATTY_SKILLS_SOURCE` remain safe dev/test seams.

## Acceptance criteria

- `matty install`, `doctor`, `update`, and `uninstall` behave correctly when launched outside the repo after `matty init`.
- Missing initialized source errors suggest `matty init` and name the missing path.
- Repo checkout behavior remains convenient for development.
- `MATTY_SKILLS_SOURCE` remains an explicit override and production defaults do not point at external clones.
- Tests cover outside-repo execution with sandboxed HOME and no real config writes.

## Answer

Resolved in `internal/cli/skills.go` by validating the resolved skill source before bundle discovery. When package-installed execution falls back to `~/.local/share/matty/bundle/skills` and that path is missing, `matty install`/`matty update` now fail with an actionable error that names the missing path and tells the user to run `matty init`. Repository checkout discovery and `MATTY_SKILLS_SOURCE` override behavior remain intact.

Added sandboxed outside-repo coverage proving `matty init` followed by `install`, `doctor`, `update`, and `uninstall` uses the initialized Installed Source without touching the operator's real HOME, and proving missing Installed Source dry-runs do not mutate sandbox state.
