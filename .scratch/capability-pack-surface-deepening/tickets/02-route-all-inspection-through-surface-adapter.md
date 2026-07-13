Status: resolved
Blocked by: 01

# Route all inspection through SurfaceAdapter

## Parent

[Capability-pack surface adapter deepening specification](../spec.md)

## What to build

Route status, lifecycle preview, stale-plan preflight, and post-apply
verification through one complete surface adapter. Codex and OpenCode users
must receive the exact behavior contracted by ticket 01 while optional
lifecycle discovery, status-only inspection, separate readiness registration,
and removal side channels disappear from production.

## Acceptance criteria

- [x] Capability-pack owns one complete surface adapter interface with one fresh, side-effect-free inspection operation and one separate projection-application operation.
- [x] Inspection input contains normalized prior composition, desired composition, residual ownership scope, and resolved executable facts without exposing lifecycle operation labels to host adapters.
- [x] Inspection returns one normalized projection collection with explicit nonzero present-or-absent goals, fresh authorization/usability evidence, pending human actions, and deterministic revision facts.
- [x] A single private gateway clones inputs and outputs, rejects duplicate or malformed projection observations, validates goal/action compatibility, and canonicalizes results for every caller.
- [x] Status, preview, stale-plan preflight, and post-apply verification all cross the same gateway.
- [x] Status, activation, and update preserve desired-only inspection; deactivation preserves prior-to-desired inspection; reconciliation preserves residual-ownership inspection.
- [x] Capability-pack remains the owner of composition, ownership authority, blockers, typed consent, destructive authorization, lifecycle intent, recovery, readiness progression, plan sealing, and verification.
- [x] Codex and OpenCode adapters both satisfy the complete contract while retaining their host-specific projection, shared-file, and surgical-removal rules.
- [x] The deprecated status inspector, optional resolution/deactivation/reconciliation interfaces, separate readiness interface and registration, lifecycle-specific inspection helpers, and removal-candidate result channel have no production use and are removed rather than wrapped.
- [x] Inspection performs no writes, state persistence, or command execution; projection application remains the adapter's only mutation operation.
- [x] Facade and sandboxed adapter tests cover valid transitions, malformed adapter results, readiness, deterministic fingerprints, stale plans, and host isolation.
- [x] The ticket 01 behavior contract remains unchanged and the complete repository test suite passes.

## Out of scope

- Moving the OpenCode adapter into its final owning package; ticket 03 performs that contraction.
- Broad package cleanup or structural deletion enforcement reserved for ticket 03.
- New cleanup behavior, commands, flags, output, state, resource kinds, or lifecycle policy.


## Answer

Introduced the capability-pack-owned `SurfaceAdapter` with one pure
`InspectSurface` operation and separate `ApplyProjections` mutation.
`SurfaceTransition` carries prior and desired composition, residual ownership,
and resolved executable facts without lifecycle labels; `SurfaceInspection`
returns explicit present/absent goals, fresh readiness evidence, pending human
actions, and deterministic revision facts.

All status, preview, stale-plan preflight, and post-apply verification paths now
cross the same private gateway. The gateway clones inputs and outputs, rejects
zero goals, duplicate or malformed identities, missing present fingerprints,
and incompatible goal/action combinations, then canonicalizes projection,
pending-action, and evidence ordering. The legacy fingerprint payload remains
pinned so the new goal/readiness fields do not alter stale-plan behavior.

Codex and OpenCode implement the complete contract while retaining their host
projection and surgical-removal rules. Production optional resolution,
deactivation, reconciliation, status-only, and separate readiness seams plus
the removal-candidate result channel were deleted rather than wrapped. The
OpenCode adapter remains in its current package for ticket 03.

Focused capability-pack, Codex, OpenCode, and CLI tests passed, followed by a
full sandboxed `go test ./...`. Independent Standards and Spec re-reviews
reported no remaining findings.
