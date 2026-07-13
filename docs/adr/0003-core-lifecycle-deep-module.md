# ADR 0003: Matty core lifecycle is a deep internal module

## Status

Accepted.

## Context

Matty core install, update, and uninstall policy currently lives across
`internal/cli`. Planning, persisted ownership, recovery, application, and
verification therefore share a package with Cobra flags and rendering. This
makes command tests the effective lifecycle test surface and conflicts with the
v0 architecture decision that CLI code should adapt behavior owned by internal
packages.

## Decision

Create `internal/corelifecycle` as the sole owner of the Matty core lifecycle.
It owns install, update, and uninstall planning, classic state persistence,
ownership, recovery, application, verification, and the decision to validate
the default Installed Source before update.

The operational seam is one facade with two conceptual operations:

```text
Preview(operation) -> immutable plan
Apply(plan)         -> structured result
```

`Preview` is strictly read-only. Plans are opaque and caller-immutable; the CLI
receives only a read-only view for rendering and passes the same plan to
`Apply`. The module returns structured results and actionable domain errors but
never writes to stdout or stderr.

A separate read-only observation seam exposes state presence, corruption,
recovery status, and recorded ownership to `doctor` and the future setup-health
module. It does not expose persistence or classify overall setup health.

## Ownership and seams

- `internal/cli` retains path composition, flags, rendering, and exit behavior.
  It maps resolved workstation paths into lifecycle configuration once when it
  constructs the facade.
- External dependency seams are limited to command execution and time. The
  filesystem remains an implementation detail tested through sandboxed paths;
  private fault-injection seams may protect persistence failures.
- `skillbundle`, `prompt`, `opencode`, `engrambin`, `ownedcontainer`, and
  `bootstrap` retain their existing behavior ownership. `corelifecycle`
  orchestrates when they participate.
- `bootstrap` continues to implement Installed Source Git validation;
  `corelifecycle.Preview(Update)` decides when it is required.
- `~/.matty/config.json` remains owned by Matty core lifecycle and stays
  independent from capability-pack state at `~/.matty/packs.json`.
- Capability-pack lifecycle remains in `internal/capabilitypack`; its approval,
  digest, and stale-plan guarantees are not added to Matty core lifecycle by
  this refactor.

## Compatibility

This is a behavior-preserving architectural change. It retains the classic
state path and schema, legacy reads, flags, relevant output and warnings, exit
behavior, recovery guarantees, and safe-uninstall behavior. User-visible
changes require separate justification.

Lifecycle policy tests move behind the new facade. CLI tests retain adapter
contracts and a small sandboxed end-to-end baseline. The migration may use
temporary wiring, but the final architecture deletes the old CLI lifecycle
implementations instead of retaining forwarding modules or dual ownership.

## Consequences

- Lifecycle behavior gains locality in one owning module and leverage through
  one test surface.
- Dry-run becomes a natural use of read-only `Preview`, rather than a mutation
  guard distributed through commands.
- Setup health can later deepen independently by consuming lifecycle
  observations instead of CLI state types.
- Workstation path redesign remains a separate architectural opportunity.
