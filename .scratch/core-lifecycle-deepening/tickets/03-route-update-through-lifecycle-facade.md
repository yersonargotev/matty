Status: ready-for-agent
Blocked by: 02

# Route update through the lifecycle facade

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Run classic update end to end through the lifecycle facade, reusing install
convergence while preserving source alignment checks, Homebrew refresh,
delegated Engram setup, ownership, and recovery behavior.

## Acceptance criteria

- [ ] Update Preview is read-only and produces an opaque plan covering source validation, managed-artifact convergence, Homebrew refresh/upgrade, Engram setup, and final state without mutating or executing commands.
- [ ] The lifecycle module decides when the default Installed Source must match the running release and delegates Git validation to the existing Installed Source owner.
- [ ] Repository checkout and explicit override behavior remain distinct from default package-installed source behavior.
- [ ] Update Apply consumes the exact previewed plan, preserves install ownership/recovery guarantees, and returns structured results, warnings, and actionable errors.
- [ ] The update command retains its flags, dry-run rendering, relevant output, warnings, exit behavior, idempotency, and existing remediation guidance.
- [ ] Tests cover aligned, missing, malformed, and stale default sources; configured overrides; Homebrew failures; Engram setup failures; interrupted recovery; and successful convergence.
- [ ] Update policy tests cross the lifecycle facade while command tests remain focused on adaptation.
- [ ] Focused tests and the full repository test suite pass.

## Out of scope

- Installed Source initialization or mutation.
- Binary upgrade behavior.
- Routing uninstall through the facade.
