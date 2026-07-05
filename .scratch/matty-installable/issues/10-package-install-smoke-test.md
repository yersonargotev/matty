# 10 — Package install smoke test

Type: task
Status: open
Blocked by: 03, 04, 08, 09

## Question

Define and run the first package-install smoke test proving a released or locally generated Matty binary can initialize its source, install, update, uninstall, and doctor in a sandbox.

## Acceptance criteria

- Uses temporary HOME/XDG_CONFIG_HOME and does not write real user config.
- Exercises package-installed execution outside the repo checkout.
- Verifies `matty init`, `install --dry-run`, `install`, `doctor`, `update --dry-run`, `update`, `uninstall --dry-run`, `uninstall`, and final `doctor`.
- Verifies expected external calls via stubs or controlled tools unless explicitly running a real release account test.
- Captures the exact smoke command sequence in docs.
