# 10 — Package install smoke test

Type: task
Status: resolved
Blocked by: 03, 04, 08, 09

## Question

Define and run the first package-install smoke test proving a released or locally generated Matty binary can initialize its source, install, update, uninstall, and doctor in a sandbox.

## Acceptance criteria

- Uses temporary HOME/XDG_CONFIG_HOME and does not write real user config.
- Exercises package-installed execution outside the repo checkout.
- Verifies `matty init`, `install --dry-run`, `install`, `doctor`, `update --dry-run`, `update`, `uninstall --dry-run`, `uninstall`, and final `doctor`.
- Verifies expected external calls via stubs or controlled tools unless explicitly running a real release account test.
- Captures the exact smoke command sequence in docs.


## Answer

Added `internal/release/package_install_smoke_test.go` with `TestPackageInstallSmokeLifecycleWithLocalReleaseBinary`. The smoke test builds a temporary release-like Matty binary from `./cmd/matty` with an injected `v0.99.0` version, creates sandboxed `HOME` and `XDG_CONFIG_HOME`, runs from a temp directory outside the repo checkout, initializes a local Matty Source fixture via `matty init --repository-url`, and verifies the full package-installed lifecycle:

1. `matty init --repository-url <local fixture repo>`
2. `matty install --dry-run`
3. `matty install`
4. `matty doctor`
5. `matty update --dry-run`
6. `matty update`
7. `matty uninstall --dry-run`
8. `matty uninstall`
9. final `matty doctor`

The test puts stubbed `brew` and `engram` binaries ahead of the real `PATH`, asserts dry-run commands do not mutate the sandbox, verifies install/uninstall artifacts, keeps the initialized Installed Source after uninstall, and checks the expected external call log. `docs/release.md` now documents the exact manual smoke command sequence and the focused automated smoke test command.
