Status: resolved
Blocked by: 02, 03, 04

# Contract the CLI lifecycle and verify the architecture

## Parent

[Matty core lifecycle deepening specification](../spec.md)

## What to build

Complete the architectural cutover by leaving the CLI as composition and
presentation only, removing the old lifecycle policy, and proving behavior and
ownership through the new facade and a small command-level integration suite.

## Acceptance criteria

- [x] Install, update, and uninstall commands construct the lifecycle facade from resolved configuration, invoke Preview and Apply, and only render their plans, results, warnings, and errors.
- [x] No lifecycle planning, state persistence, ownership, recovery, Engram acquisition/setup policy, managed-skill policy, or safe-cleanup policy remains implemented in the CLI package.
- [x] Old lifecycle implementations and transitional forwarding modules are deleted; there is one production path for each operation.
- [x] Policy-heavy tests reside with the lifecycle module and primarily cross its facade; CLI tests cover flags, rendering, relevant messages, warnings, exit behavior, and a small sandboxed integration baseline.
- [x] The deletion test passes: removing the former CLI lifecycle modules does not redistribute their complexity among commands or test helpers.
- [x] Classic state and capability-pack state remain independent, and no lifecycle command reads or mutates pack intent or ownership.
- [x] User-visible behavior remains compatible with the source spec and ADR 0003.
- [x] Formatting, vet, focused tests, the full repository test suite, and any repository-required checks pass.

## Out of scope

- Setup-health deepening beyond the already-established lifecycle observation.
- Workstation path redesign.
- New lifecycle features or user-visible behavior changes.

## Answer

Completed the final CLI contraction by deleting `internal/cli/plan.go` and
`internal/cli/skills.go`, removing their unused path/state helpers and
policy-heavy CLI assertions, and leaving install, update, and uninstall on one
production route each: resolved paths/configuration, facade construction,
`Preview`, `Apply`, and presentation.

Doctor still owns health classification and messages, but now consumes
detached read-only managed-link facts from `corelifecycle`; filesystem
inspection and expected-skill discovery remain with the lifecycle owner. The
deletion test scans the complete CLI package for obsolete structures and
forbidden production effects, while facade tests explicitly prove that classic
lifecycle operations neither read nor mutate `packs.json` or capability-pack
artifacts.

Focused tests, gofmt, vet, build, the sandboxed full suite, and the sandboxed
race suite pass. The initial two-axis review found two Spec issues (doctor
inspection-error compatibility and incomplete deletion-test scope); both were
corrected and the final Standards and Spec re-reviews report zero findings.
