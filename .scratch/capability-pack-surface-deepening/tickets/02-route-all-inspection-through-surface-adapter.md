Status: ready-for-agent
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

- [ ] Capability-pack owns one complete surface adapter interface with one fresh, side-effect-free inspection operation and one separate projection-application operation.
- [ ] Inspection input contains normalized prior composition, desired composition, residual ownership scope, and resolved executable facts without exposing lifecycle operation labels to host adapters.
- [ ] Inspection returns one normalized projection collection with explicit nonzero present-or-absent goals, fresh authorization/usability evidence, pending human actions, and deterministic revision facts.
- [ ] A single private gateway clones inputs and outputs, rejects duplicate or malformed projection observations, validates goal/action compatibility, and canonicalizes results for every caller.
- [ ] Status, preview, stale-plan preflight, and post-apply verification all cross the same gateway.
- [ ] Status, activation, and update preserve desired-only inspection; deactivation preserves prior-to-desired inspection; reconciliation preserves residual-ownership inspection.
- [ ] Capability-pack remains the owner of composition, ownership authority, blockers, typed consent, destructive authorization, lifecycle intent, recovery, readiness progression, plan sealing, and verification.
- [ ] Codex and OpenCode adapters both satisfy the complete contract while retaining their host-specific projection, shared-file, and surgical-removal rules.
- [ ] The deprecated status inspector, optional resolution/deactivation/reconciliation interfaces, separate readiness interface and registration, lifecycle-specific inspection helpers, and removal-candidate result channel have no production use and are removed rather than wrapped.
- [ ] Inspection performs no writes, state persistence, or command execution; projection application remains the adapter's only mutation operation.
- [ ] Facade and sandboxed adapter tests cover valid transitions, malformed adapter results, readiness, deterministic fingerprints, stale plans, and host isolation.
- [ ] The ticket 01 behavior contract remains unchanged and the complete repository test suite passes.

## Out of scope

- Moving the OpenCode adapter into its final owning package; ticket 03 performs that contraction.
- Broad package cleanup or structural deletion enforcement reserved for ticket 03.
- New cleanup behavior, commands, flags, output, state, resource kinds, or lifecycle policy.
