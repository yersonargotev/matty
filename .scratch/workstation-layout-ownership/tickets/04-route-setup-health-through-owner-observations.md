Status: ready-for-agent

# Route setup health through owner observations

## What to build

Make setup health diagnose the workstation through stable read-only
observations supplied by lifecycle, skill, host, and Engram owners. Preserve
the report as the setup-health seam while removing its need for an aggregate
catalog of artifact paths.

## Blocked by

- [Route Matty core lifecycle through owning layouts](02-route-core-lifecycle-through-owning-layouts.md)

## Acceptance criteria

- [ ] Setup health consumes lifecycle state and managed-skill observations from their owners.
- [ ] Skill, Codex, OpenCode, and Engram facts come from their owning layouts, observers, or resolvers rather than setup-health path reconstruction.
- [ ] Setup health receives directly only facts intrinsic to diagnosis and report context.
- [ ] Report construction does not recreate the former broad layout under a new configuration type.
- [ ] Diagnosis remains read-only and continues after individual observation failures.
- [ ] Human context and check output remain byte-compatible.
- [ ] JSON schema, report kind, check names and order, severities, details, remediation text, summary counts, status, and error timing remain unchanged.
- [ ] Setup-health semantic tests remain at the report seam and use real owner observations against sandboxed filesystems where deterministic.
- [ ] CLI doctor tests remain limited to composition, rendering, output failures, and exit adaptation.
- [ ] Capability-pack health remains excluded.
- [ ] No permanent forwarding configuration or duplicate observation policy remains.
- [ ] Focused setup-health, owner, and CLI tests pass, followed by the complete repository test suite with sandboxed Home and XDG configuration.

## Out of scope

- Adding, removing, or reclassifying health checks.
- Repair actions, schema changes, or revised diagnostic wording.
- Capability-pack readiness or status diagnosis.
- Final contraction of the CLI layout surface before capability packs migrate.
