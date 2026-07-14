Status: ready-for-agent

# Contract the Workstation snapshot through initialization

## What to build

Introduce the narrow Workstation snapshot and prove it through the complete
initialization flow. Initialization must obtain one lazy, immutable view of
ambient workstation facts, translate explicit Home and source-root flags into
owner inputs, and use bootstrap's Installed Source descriptor. Help and version
must remain independent from workstation resolution.

This is the expand slice. It establishes the new normalization seam and one
production vertical path while leaving unrelated command families unchanged.

## Blocked by

None — can start immediately.

## Acceptance criteria

- [ ] The snapshot contains normalized Home, configuration home, executable search path, Homebrew prefix, current working directory, and Matty Home, but no domain artifact paths.
- [ ] Snapshot construction preserves missing-Home failure and absolute-versus-relative XDG configuration behavior.
- [ ] Explicit-Home mode derives configuration home from the override and ignores ambient XDG configuration.
- [ ] Snapshot resolution is lazy and reused within one command invocation.
- [ ] Help and version behavior remain successful without Home.
- [ ] Bootstrap owns one Installed Source descriptor covering the default root and bundle location.
- [ ] Explicit source-root normalization belongs to bootstrap rather than CLI path logic.
- [ ] Initialization uses the snapshot and Installed Source descriptor without duplicate Home/configuration/source derivation.
- [ ] Initialization output, errors, repository behavior, filesystem effects, and flags remain unchanged.
- [ ] Contract tests cover normal inputs, overrides, missing inputs, fallback rules, current-directory capture, and immutability using sandboxed workstation facts.
- [ ] No other command family is migrated or behaviorally changed in this ticket.
- [ ] Focused tests and the complete repository test suite pass with sandboxed Home and XDG configuration.

## Out of scope

- Migrating classic lifecycle, capability packs, or setup health.
- Moving any existing path or changing Installed Source semantics.
- Deleting the existing shared layout surface before its remaining callers migrate.
