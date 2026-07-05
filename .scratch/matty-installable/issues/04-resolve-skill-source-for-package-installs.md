# 04 — Resolve skill source for package installs

Type: task
Status: open
Blocked by: 01, 03

## Question

Update Matty path resolution so package-installed commands use the initialized installed source by default, while repo checkouts and `MATTY_SKILLS_SOURCE` remain safe dev/test seams.

## Acceptance criteria

- `matty install`, `doctor`, `update`, and `uninstall` behave correctly when launched outside the repo after `matty init`.
- Missing initialized source errors suggest `matty init` and name the missing path.
- Repo checkout behavior remains convenient for development.
- `MATTY_SKILLS_SOURCE` remains an explicit override and production defaults do not point at external clones.
- Tests cover outside-repo execution with sandboxed HOME and no real config writes.
