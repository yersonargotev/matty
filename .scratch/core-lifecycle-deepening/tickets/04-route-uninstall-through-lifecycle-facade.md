Status: ready-for-agent
Blocked by: 01

# Route uninstall through the lifecycle facade

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Run classic uninstall end to end through lifecycle Preview and Apply so
ownership verification, marker-owned prompt removal, safe container cleanup,
and interrupted-install cleanup are owned by the lifecycle module.

## Acceptance criteria

- [ ] Uninstall Preview is read-only and returns an opaque plan describing only verified Matty-owned removal and cleanup candidates.
- [ ] Uninstall Apply verifies previewed cleanup preconditions before mutation and removes only managed skill links, marker-owned prompt/config content, classic state, and unchanged empty containers whose provenance is proven.
- [ ] Missing state, corrupt state, interrupted install, changed containers, unmanaged symlinks, pre-existing containers, and contributor-owned bytes preserve the current safety behavior.
- [ ] A converged uninstall reports no work without mutating the filesystem.
- [ ] Apply returns a structured result or actionable domain error and never writes directly to command output streams.
- [ ] The uninstall command retains its flags, dry-run rendering, relevant output, exit behavior, and preservation guarantees.
- [ ] Facade and sandboxed end-to-end tests cover pristine cleanup, unmanaged preservation, recovery cleanup, preview/apply change detection, and repeated uninstall.
- [ ] Focused tests and the full repository test suite pass.

## Out of scope

- Removing Installed Source data.
- Reading or deleting capability-pack state or projections.
- General workstation path redesign.
