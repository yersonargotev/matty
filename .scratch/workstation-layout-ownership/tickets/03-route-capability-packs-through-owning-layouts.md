Status: ready-for-agent

# Route capability packs through owning layouts

## What to build

Route every capability-pack command through Matty Home, the resolved Skill
Source, the global skill installation layout, canonical Codex/OpenCode layouts,
and Engram resolution supplied by their owners. Capability pack continues to
own pack state and lifecycle policy; host and infrastructure modules own only
their layouts and observations.

## Blocked by

- [Route Matty core lifecycle through owning layouts](02-route-core-lifecycle-through-owning-layouts.md)

## Acceptance criteria

- [ ] Capability pack derives its separate pack-state location from Matty Home without acquiring classic-state ownership.
- [ ] Pack catalog discovery and source validation use the single resolved Skill Source.
- [ ] Pack surface adapters receive canonical host and global skill layouts from their owners.
- [ ] Executable requirements use engrambin resolution without receiving candidate paths from CLI.
- [ ] Pack CLI composition no longer consumes or reconstructs the broad shared layout.
- [ ] Pack list, show, status, activate, update, deactivate, reconcile, preview, stale-plan handling, readiness, and recovery behavior remain unchanged.
- [ ] Plans, blockers, consent, ownership protection, state schema, command execution, rendering, errors, and filesystem effects remain unchanged.
- [ ] Codex and OpenCode adapter contracts continue using sandboxed host layouts and preserve unrelated host content.
- [ ] No setup-health behavior is moved or changed.
- [ ] No duplicate host, skill, source, executable, or state layout policy remains in pack composition.
- [ ] Focused capability-pack, host, and CLI tests pass, followed by the complete repository test suite with sandboxed Home and XDG configuration.

## Out of scope

- Changing capability-pack lifecycle semantics or surface adapter contracts.
- Adding cleanup, migration, hosts, resources, or executable acquisition behavior.
- Final contraction of the CLI layout surface before setup health migrates.
