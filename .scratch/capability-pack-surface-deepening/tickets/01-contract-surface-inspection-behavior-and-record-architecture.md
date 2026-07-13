Status: ready-for-agent
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

- [ ] A dedicated accepted architecture decision records one complete surface adapter, normalized transition facts, unified projection/readiness observations, pure inspection, separate mutation, capability-pack policy ownership, host-module ownership, behavior preservation, and obsolete seam deletion.
- [ ] The architecture decision links to the broader capability-pack specification and existing architecture without duplicating product scope.
- [ ] Facade-level characterization coverage freezes status, activation, update, deactivation, targeted and surface-wide reconciliation, stale-plan preflight, post-apply verification, recovery, and readiness behavior.
- [ ] Characterization coverage fixes current projection actions, blockers, typed consent phases, ownership protection, pending human actions, plan fingerprints, and verification outcomes.
- [ ] The contract proves status, activation, and update remain desired-only; deactivation preserves prior-to-desired inspection; reconciliation preserves ownership-residual inspection.
- [ ] The contract proves inspection remains fresh and read-only, performs no command execution, and makes no filesystem or state mutation.
- [ ] Filesystem-backed scenarios use sandboxed user and configuration paths and never inspect or write operator configuration.
- [ ] No production adapter interface, inspection route, package ownership, lifecycle behavior, readiness behavior, or user-visible output changes in this ticket.
- [ ] Focused tests and the complete repository test suite pass.

## Out of scope

- Introducing or routing through the new surface adapter.
- Removing optional interfaces or deprecated fallbacks.
- Moving the OpenCode adapter or renaming production types.
- Changing any capability-pack behavior.
