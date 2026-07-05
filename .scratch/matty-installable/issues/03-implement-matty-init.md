# 03 — Implement matty init

Type: task
Status: open
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
