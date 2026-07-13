Status: resolved
Blocked by: 02

# Route update through the lifecycle facade

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Run classic update end to end through the lifecycle facade, reusing install
convergence while preserving source alignment checks, Homebrew refresh,
delegated Engram setup, ownership, and recovery behavior.

## Acceptance criteria

- [x] Update Preview is read-only and produces an opaque plan covering source validation, managed-artifact convergence, Homebrew refresh/upgrade, Engram setup, and final state without mutating or executing commands.
- [x] The lifecycle module decides when the default Installed Source must match the running release and delegates Git validation to the existing Installed Source owner.
- [x] Repository checkout and explicit override behavior remain distinct from default package-installed source behavior.
- [x] Update Apply consumes the exact previewed plan, preserves install ownership/recovery guarantees, and returns structured results, warnings, and actionable errors.
- [x] The update command retains its flags, dry-run rendering, relevant output, warnings, exit behavior, idempotency, and existing remediation guidance.
- [x] Tests cover aligned, missing, malformed, and stale default sources; configured overrides; Homebrew failures; Engram setup failures; interrupted recovery; and successful convergence.
- [x] Update policy tests cross the lifecycle facade while command tests remain focused on adaptation.
- [x] Focused tests and the full repository test suite pass.

## Out of scope

- Installed Source initialization or mutation.
- Binary upgrade behavior.
- Routing uninstall through the facade.

## Answer

Implemented classic update through `corelifecycle.Facade.Preview(Update)` and
`Apply(plan)`. The lifecycle module now owns the default Installed Source
alignment decision while delegating Git validation to `bootstrap`, and it
orchestrates Homebrew refresh/upgrade, managed-artifact convergence, canonical
Engram setup, recovery publication, final confirmation, structured warnings,
and actionable errors through the same opaque-plan seam established for
install.

The CLI now only maps resolved paths and source facts into lifecycle config,
renders the detached plan/result, and preserves the existing update command
contract. Update policy and persistence-failure coverage moved to sandboxed
facade tests; CLI coverage retains adapter contracts plus a sandboxed
idempotent lifecycle baseline. Focused tests and sandboxed `go test ./...`
pass, and the initial two-axis review findings were corrected before closure.
