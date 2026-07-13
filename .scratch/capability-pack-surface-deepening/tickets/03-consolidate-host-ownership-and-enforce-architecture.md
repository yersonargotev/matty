Status: ready-for-agent
Blocked by: 02

# Consolidate host ownership and enforce the architecture

## Parent

[Capability-pack surface adapter deepening specification](../spec.md)

## What to build

Complete the surface ownership contraction by making Codex and OpenCode the
two lifecycle-neutral host modules, deleting the obsolete OpenCode activation
package and remaining compatibility structure, and proving that capability-pack
and CLI code cannot redistribute host or lifecycle inspection policy.

## Acceptance criteria

- [ ] Activation-specific adapter and observation names are replaced by lifecycle-neutral surface terminology throughout productive code and focused tests.
- [ ] The OpenCode capability-pack adapter is absorbed by the existing OpenCode host module without reversing dependency direction or duplicating host projection rules.
- [ ] Codex and OpenCode remain the only concrete production adapters behind the capability-pack-owned surface seam.
- [ ] No compatibility aliases, forwarding adapters, optional capability interfaces, deprecated status inspectors, separate readiness registration, removal side channels, or lifecycle-specific inspection helpers remain.
- [ ] Capability-pack callers and tests use one complete fake surface adapter; tests that only protected partial adapters or obsolete fallbacks are deleted.
- [ ] Policy-heavy coverage remains at the capability-pack facade, host translation coverage remains in sandboxed Codex/OpenCode adapter tests, and CLI coverage remains limited to composition, rendering, exit behavior, and wiring.
- [ ] A structural architecture test forbids reintroduction of the obsolete interfaces, helpers, removal channel, package ownership, and parallel production inspection routes.
- [ ] The deletion test passes: removing obsolete surface structure does not redistribute lifecycle policy into adapters, host policy into capability-pack, or either policy into the CLI.
- [ ] User-visible status, plans, blockers, approvals, readiness, recovery, verification, filesystem effects, output, and exit behavior remain unchanged from ticket 01.
- [ ] Formatting, vetting, build, focused tests, and the complete repository test suite pass.

## Out of scope

- New host surfaces or support for partial adapters.
- New lifecycle, readiness, cleanup, manifest, state, path, or CLI behavior.
- A generic cross-host projection or lifecycle abstraction.
- Opportunistic cleanup outside surface ownership and architecture enforcement.
