Status: resolved
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

- [x] Activation-specific adapter and observation names are replaced by lifecycle-neutral surface terminology throughout productive code and focused tests.
- [x] The OpenCode capability-pack adapter is absorbed by the existing OpenCode host module without reversing dependency direction or duplicating host projection rules.
- [x] Codex and OpenCode remain the only concrete production adapters behind the capability-pack-owned surface seam.
- [x] No compatibility aliases, forwarding adapters, optional capability interfaces, deprecated status inspectors, separate readiness registration, removal side channels, or lifecycle-specific inspection helpers remain.
- [x] Capability-pack callers and tests use one complete fake surface adapter; tests that only protected partial adapters or obsolete fallbacks are deleted.
- [x] Policy-heavy coverage remains at the capability-pack facade, host translation coverage remains in sandboxed Codex/OpenCode adapter tests, and CLI coverage remains limited to composition, rendering, exit behavior, and wiring.
- [x] A structural architecture test forbids reintroduction of the obsolete interfaces, helpers, removal channel, package ownership, and parallel production inspection routes.
- [x] The deletion test passes: removing obsolete surface structure does not redistribute lifecycle policy into adapters, host policy into capability-pack, or either policy into the CLI.
- [x] User-visible status, plans, blockers, approvals, readiness, recovery, verification, filesystem effects, output, and exit behavior remain unchanged from ticket 01.
- [x] Formatting, vetting, build, focused tests, and the complete repository test suite pass.

## Out of scope

- New host surfaces or support for partial adapters.
- New lifecycle, readiness, cleanup, manifest, state, path, or CLI behavior.
- A generic cross-host projection or lifecycle abstraction.
- Opportunistic cleanup outside surface ownership and architecture enforcement.

## Answer

Renamed the concrete Codex and OpenCode implementations and focused tests to
lifecycle-neutral surface terminology. The OpenCode implementation now lives
directly in `internal/opencode`; the obsolete `internal/opencodeactivation`
package was deleted without aliases, wrappers, or forwarding adapters. CLI
wiring constructs the two host-owned adapters behind the unchanged
capability-pack `SurfaceAdapter` contract.

Capability-pack facade, status, and contract coverage now share one complete
fake surface adapter. A structural architecture test pins Codex and OpenCode as
the only concrete implementations, the private gateway as the only direct
production inspection route, the absence of obsolete interfaces and package
ownership, and the boundary between lifecycle, host, and CLI policy. The only
remaining `RemovalCandidates` spelling is the fixed historical fingerprint
JSON key required to preserve existing plan digests; the structural test
rejects any additional production occurrence.

Focused package tests and the final sandboxed formatting, `go vet ./...`,
`go build ./...`, and `go test ./...` checks passed. Standards and Spec
re-reviews against `b85512babc03e75da4ee9bdc57987a7916a3ec52` reported no
remaining findings.
