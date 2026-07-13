Status: resolved
Blocked by: 02

# Contract the CLI health adapter and verify the architecture

## Parent

[Matty setup health deepening specification](../spec.md)

## What to build

Complete the ownership transfer by deleting the former CLI health model and
leaving the command layer responsible only for composition, human and JSON
rendering, output errors, and unhealthy exit mapping. Prove the final
architecture and behavior through the setup health report seam and a small CLI
adapter suite.

## Acceptance criteria

- [ ] CLI-owned report and summary types, report builders, per-domain classifiers, summary logic, and obsolete helper tests are deleted.
- [ ] No aliases, forwarding builders, duplicate policy, or second production diagnosis route remain after contraction.
- [ ] The CLI contains only setup health construction, resolved-configuration adaptation, human rendering, JSON version 1 encoding, output handling, and exit-error mapping for `doctor`.
- [ ] Policy-heavy coverage resides with the setup health module and crosses Diagnose-to-Report; CLI coverage is limited to rendering, schema encoding, output failures, exit behavior, command wiring, and a small sandboxed baseline.
- [ ] The exact contract from ticket 01 still passes for check order, names, severities, details, remediation, summary, human output, JSON output, and exit behavior.
- [ ] Read-only verification proves that `doctor` does not mutate sandboxed files or invoke mutating command capability.
- [ ] The deletion test passes: removing the former CLI health implementation does not redistribute diagnosis policy among renderers, command construction, domain probes, or test helpers.
- [ ] Setup health remains limited to base installation and neither reads nor reclassifies capability-pack status or readiness.
- [ ] Workstation path design, core lifecycle behavior, owner probes, and user-facing diagnosis remain unchanged.
- [ ] Formatting, vetting, build, focused tests, and the complete repository test suite pass.

## Out of scope

- New health behavior, schema versions, commands, flags, or repair operations.
- Capability-pack status integration or workstation path redesign.
- Opportunistic cleanup outside the setup health ownership refactor.


## Answer

Removed the obsolete CLI-owned doctor report model, summary, builders, domain
classifiers, remediation and ordering policy, and their private-helper tests.
The command now has one production route through
`setuphealth.New(...).Diagnose(config) -> Report`; `internal/cli` only adapts
resolved paths, renders human and JSON v1 output, propagates writer failures,
maps report failures to `ErrDoctorUnhealthy`, and wires Cobra.

Policy-heavy CLI scenarios were deleted in favor of the existing
`internal/setuphealth` report-level semantic matrix. CLI coverage now consists
of focused adapter tests, command wiring/configuration checks, the sandboxed
read-only baseline, and the exact ticket-01 human/JSON/exit contract. A setup
health architecture deletion test proves the former files and symbols are gone,
no domain observation or diagnosis policy has moved into CLI production or test
helpers, and only one construction and invocation route remains.

`internal/setuphealth` was unchanged. Focused tests, the deletion test,
`go vet ./...`, `go build ./...`, and `go test ./...` pass. Two-axis review
against `7d35199`, this ticket, the specification, and ADR 0004 returned no
Standards or Spec findings.
