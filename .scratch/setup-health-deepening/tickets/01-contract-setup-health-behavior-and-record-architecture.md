Status: ready-for-agent
Blocked by: None — can start immediately

# Contract setup health behavior and record the architecture

## Parent

[Matty setup health deepening specification](../spec.md)

## What to build

Establish a durable architectural decision and a high-seam regression contract
for the existing `doctor` command before moving diagnosis ownership. The
contract must make exact behavior equivalence reviewable while leaving the
production diagnosis route unchanged.

## Acceptance criteria

- [ ] A dedicated accepted architecture decision records the setup health module's ownership, single Diagnose-to-Report interface, self-contained report, read-only active observations, best-effort semantics, minimal dependencies, CLI rendering responsibility, compatibility requirements, and explicit exclusions.
- [ ] The architecture decision links to the existing core lifecycle observation decision without moving health classification into lifecycle or changing lifecycle behavior.
- [ ] Command-level regression coverage fixes the existing human and JSON version 1 contracts, including context, check names and order, severities, details, remediation, summary counts and status, and unhealthy exit behavior.
- [ ] Regression coverage proves warnings remain non-fatal, failures remain fatal only after the complete report is rendered, and a failed observation does not hide unrelated checks.
- [ ] Regression coverage proves diagnosis remains read-only while using substituted executable lookup, version, and process facts rather than the operator's real workstation state.
- [ ] All filesystem-backed checks run with sandboxed user and configuration paths and do not write to real user configuration.
- [ ] No production diagnosis behavior or user-visible output changes in this ticket.
- [ ] Focused tests and the complete repository test suite pass.

## Out of scope

- Creating or wiring the setup health module.
- Moving any diagnosis implementation out of the CLI.
- Changing checks, messages, schema, paths, capability-pack status, or repair behavior.
