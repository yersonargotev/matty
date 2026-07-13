Status: ready-for-agent
Blocked by: None — can start immediately

# Move classic state behind lifecycle ownership

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Make the Matty core lifecycle module the sole owner of classic state,
persistence, recovery facts, and read-only observation while preserving every
current command and doctor result. This establishes the first complete path
through the new owner without changing install, update, or uninstall behavior.

## Acceptance criteria

- [ ] Classic state types, schema compatibility, legacy reads, atomic saving, ownership records, recovery status, and desired-state derivation are owned by the lifecycle module rather than the CLI adapter.
- [ ] The lifecycle module exposes a read-only observation that distinguishes missing, valid, corrupt, and recovery-required state and reports recorded ownership without exposing save operations or its store.
- [ ] Doctor consumes lifecycle observation while retaining its existing health classification, human output, JSON output, exit behavior, and read-only guarantee.
- [ ] Existing install, update, and uninstall commands continue to read and persist the same state contract during the migration.
- [ ] State and observation tests use sandboxed paths and cover initial publication, complete replacement, write/publication failures, corrupt data, legacy data, recovery state, and ownership facts.
- [ ] Focused tests and the full repository test suite pass without writing to real user configuration.

## Out of scope

- Moving install, update, or uninstall planning and application behind the facade.
- General setup-health deepening beyond consuming the state observation.
- Changing the state schema, location, or capability-pack state.
