Status: ready-for-agent
Blocked by: 02, 03, 04

# Contract the CLI lifecycle and verify the architecture

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Complete the architectural cutover by leaving the CLI as composition and
presentation only, removing the old lifecycle policy, and proving behavior and
ownership through the new facade and a small command-level integration suite.

## Acceptance criteria

- [ ] Install, update, and uninstall commands construct the lifecycle facade from resolved configuration, invoke Preview and Apply, and only render their plans, results, warnings, and errors.
- [ ] No lifecycle planning, state persistence, ownership, recovery, Engram acquisition/setup policy, managed-skill policy, or safe-cleanup policy remains implemented in the CLI package.
- [ ] Old lifecycle implementations and transitional forwarding modules are deleted; there is one production path for each operation.
- [ ] Policy-heavy tests reside with the lifecycle module and primarily cross its facade; CLI tests cover flags, rendering, relevant messages, warnings, exit behavior, and a small sandboxed integration baseline.
- [ ] The deletion test passes: removing the former CLI lifecycle modules does not redistribute their complexity among commands or test helpers.
- [ ] Classic state and capability-pack state remain independent, and no lifecycle command reads or mutates pack intent or ownership.
- [ ] User-visible behavior remains compatible with the source spec and ADR 0003.
- [ ] Formatting, vet, focused tests, the full repository test suite, and any repository-required checks pass.

## Out of scope

- Setup-health deepening beyond the already-established lifecycle observation.
- Workstation path redesign.
- New lifecycle features or user-visible behavior changes.
