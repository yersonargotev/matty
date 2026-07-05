# 03 — Implement matty init

Type: task
Status: resolved
Blocked by: 01, 02

## Question

Implement `matty init` for package-installed users, borrowing the safe clone/validate shape from `dots init` while adapting it to Matty's bundle contract.

## Acceptance criteria

- `matty init` initializes the chosen installed source location without touching real HOME in tests.
- It supports test/dev flags equivalent to `--home`, `--source-root`, `--repository-url`, and optional `--repository-ref` if the source model uses Git.
- Release builds default the repository ref to the current `v0.x.y` tag when appropriate.
- Existing valid initialized state is idempotent.
- Invalid non-empty destination fails with actionable guidance rather than deleting user data.
- Tests use sandboxed HOME/git config and pass with `go test ./...`.

## Answer

Implemented `matty init` with Matty-owned Installed Source bootstrapping behind `internal/bootstrap` and a thin `internal/cli` command adapter.

- Default Installed Source root: `~/.local/share/matty`.
- Default skill bundle fallback outside a repo checkout: `~/.local/share/matty/bundle/skills`.
- Dev override remains highest priority via `MATTY_SKILLS_SOURCE`.
- Repo checkout discovery remains supported for local development.
- `matty init` supports `--home`, `--source-root`, `--repository-url`, and `--repository-ref`.
- Release versions beginning with `v` default `--repository-ref` to the running binary version.
- Existing valid checkouts are idempotent; invalid non-empty destinations fail with guidance to move them aside or pass `--source-root`.
- Tests create local git fixtures with sandboxed `HOME`/`XDG_CONFIG_HOME`; final verification: `go test ./...`.
