Status: ready-for-agent
Blocked by: 01

# Route install through the lifecycle facade

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Run classic install end to end through lifecycle Preview and Apply so dry-run,
managed-skill reconciliation, prompt projection, Engram setup, ownership, and
interrupted recovery no longer depend on CLI-owned policy.

## Acceptance criteria

- [ ] Install Preview returns an opaque, caller-immutable plan with a read-only action view and performs no filesystem mutation, state publication, directory creation, or external command.
- [ ] Install Apply consumes the exact previewed plan and owns managed-skill discovery, unmanaged-path preservation, container provenance, prompt projection ordering, Engram installation/setup decisions, recovery-state publication, and final confirmed state.
- [ ] The lifecycle module uses injected command lookup/execution and time while keeping filesystem behavior internal and sandboxed in tests.
- [ ] Apply returns structured results, warnings, and actionable domain errors without writing to command output streams.
- [ ] The install command retains its flags, dry-run rendering, relevant output, warnings, exit behavior, idempotency, and package/repository/override source reporting.
- [ ] Failure tests cover pre-mutation rejection, state preparation, partial container creation, symlink ownership persistence, external command failures, final state publication, and safe retry.
- [ ] Lifecycle policy is tested primarily through the facade, with command tests limited to adapter behavior and a sandboxed end-to-end install baseline.
- [ ] Focused tests and the full repository test suite pass.

## Out of scope

- Routing update or uninstall through the facade.
- Adding approvals, plan digests, serialized plans, or capability-pack stale-plan semantics.
