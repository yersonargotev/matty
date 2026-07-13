Status: resolved
Blocked by: None — can start immediately

# Contract surface inspection behavior and record the architecture

## Parent

[Capability-pack surface adapter deepening specification](../spec.md)

## What to build

Establish a durable architectural decision and a high-seam regression contract
for existing capability-pack surface behavior before changing production
inspection. Users must see no behavior change in this ticket, while the
complete status and lifecycle contract becomes reviewable through the facade.

## Acceptance criteria

- [x] A dedicated accepted architecture decision records one complete surface adapter, normalized transition facts, unified projection/readiness observations, pure inspection, separate mutation, capability-pack policy ownership, host-module ownership, behavior preservation, and obsolete seam deletion.
- [x] The architecture decision links to the broader capability-pack specification and existing architecture without duplicating product scope.
- [x] Facade-level characterization coverage freezes status, activation, update, deactivation, targeted and surface-wide reconciliation, stale-plan preflight, post-apply verification, recovery, and readiness behavior.
- [x] Characterization coverage fixes current projection actions, blockers, typed consent phases, ownership protection, pending human actions, plan fingerprints, and verification outcomes.
- [x] The contract proves status, activation, and update remain desired-only; deactivation preserves prior-to-desired inspection; reconciliation preserves ownership-residual inspection.
- [x] The contract proves inspection remains fresh and read-only, performs no command execution, and makes no filesystem or state mutation.
- [x] Filesystem-backed scenarios use sandboxed user and configuration paths and never inspect or write operator configuration.
- [x] No production adapter interface, inspection route, package ownership, lifecycle behavior, readiness behavior, or user-visible output changes in this ticket.
- [x] Focused tests and the complete repository test suite pass.

## Out of scope

- Introducing or routing through the new surface adapter.
- Removing optional interfaces or deprecated fallbacks.
- Moving the OpenCode adapter or renaming production types.
- Changing any capability-pack behavior.

## Answer

Accepted [ADR 0005](../../../docs/adr/0005-capability-pack-surface-adapter.md),
which records the complete lifecycle-neutral `SurfaceAdapter` seam, normalized
transition facts, inspection/application separation, ownership boundaries,
behavior-preservation constraint, and required deletion of obsolete seams after
migration.

Added facade-level characterization coverage in
`internal/capabilitypack/surface_inspection_contract_test.go`. One complete fake
implements every current inspection capability and freezes desired-only status,
activation, and update; prior-to-desired deactivation; ownership-residual
targeted and surface-wide reconciliation; actions, blockers, consent,
ownership protection, pending actions, deterministic plan identity, stale
preflight, post-apply verification, recovery, and readiness. Purity assertions
cover state/application/command sentinels plus empty sandboxed `HOME` and
`XDG_CONFIG_HOME` paths.

No production code, interface, route, or package ownership changed. Focused
capability-pack tests and sandboxed `go test ./... -count=1` passed. Independent
standards and specification re-reviews reported no remaining findings.
