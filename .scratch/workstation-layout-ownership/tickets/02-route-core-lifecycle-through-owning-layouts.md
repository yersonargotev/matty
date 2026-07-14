Status: ready-for-agent

# Route Matty core lifecycle through owning layouts

## What to build

Route install, update, and uninstall through the Workstation snapshot and the
narrow values owned by core lifecycle, bootstrap, skillbundle, Codex,
OpenCode, and engrambin. Resolve Skill Source once, reuse the global skill and
host layouts, and derive classic state from Matty Home while preserving the
existing core lifecycle facade and all observable command behavior.

This slice establishes the shared owner APIs needed by capability packs and
setup health. Temporary legacy wiring may remain only for command families that
have not migrated yet.

## Blocked by

- [Contract the Workstation snapshot through initialization](01-contract-workstation-snapshot-through-initialization.md)

## Acceptance criteria

- [ ] Core lifecycle derives its classic state location from Matty Home and retains sole ownership of classic state behavior.
- [ ] Skill Source precedence is resolved once by skillbundle and the same resolved value is used throughout one lifecycle invocation.
- [ ] Bootstrap's Installed Source descriptor is reused for default-source validation.
- [ ] Skillbundle owns and exposes the global skill installation layout used by core lifecycle.
- [ ] Codex and OpenCode own and expose their canonical host layouts used by core lifecycle.
- [ ] Engrambin owns executable candidates, precedence, resolution, and observation used by core lifecycle.
- [ ] CLI lifecycle composition no longer maps a broad path value into the core lifecycle facade.
- [ ] Install, update, and uninstall preserve plans, warnings, errors, rendering, command execution, state schemas, ownership, recovery, and filesystem effects.
- [ ] Sandboxed lifecycle tests use owner values rather than independently deriving canonical paths.
- [ ] Capability-pack and setup-health behavior are not migrated or changed in this ticket.
- [ ] No permanent compatibility wrapper or duplicate owner policy is introduced.
- [ ] Focused owner, lifecycle, and CLI tests pass, followed by the complete repository test suite with sandboxed Home and XDG configuration.

## Out of scope

- Capability-pack composition and lifecycle behavior.
- Setup-health report construction or rendering.
- Final deletion of the shared layout surface while downstream callers remain.
