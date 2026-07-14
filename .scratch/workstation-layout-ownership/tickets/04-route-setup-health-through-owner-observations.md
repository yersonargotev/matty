Status: resolved

# Route setup health through owner observations

## What to build

Make setup health diagnose the workstation through stable read-only
observations supplied by lifecycle, skill, host, and Engram owners. Preserve
the report as the setup-health seam while removing its need for an aggregate
catalog of artifact paths.

## Blocked by

- [Route Matty core lifecycle through owning layouts](02-route-core-lifecycle-through-owning-layouts.md)

## Acceptance criteria

- [x] Setup health consumes lifecycle state and managed-skill observations from their owners.
- [x] Skill, Codex, OpenCode, and Engram facts come from their owning layouts, observers, or resolvers rather than setup-health path reconstruction.
- [x] Setup health receives directly only facts intrinsic to diagnosis and report context.
- [x] Report construction does not recreate the former broad layout under a new configuration type.
- [x] Diagnosis remains read-only and continues after individual observation failures.
- [x] Human context and check output remain byte-compatible.
- [x] JSON schema, report kind, check names and order, severities, details, remediation text, summary counts, status, and error timing remain unchanged.
- [x] Setup-health semantic tests remain at the report seam and use real owner observations against sandboxed filesystems where deterministic.
- [x] CLI doctor tests remain limited to composition, rendering, output failures, and exit adaptation.
- [x] Capability-pack health remains excluded.
- [x] No permanent forwarding configuration or duplicate observation policy remains.
- [x] Focused setup-health, owner, and CLI tests pass, followed by the complete repository test suite with sandboxed Home and XDG configuration.

## Out of scope

- Adding, removing, or reclassifying health checks.
- Repair actions, schema changes, or revised diagnostic wording.
- Capability-pack readiness or status diagnosis.
- Final contraction of the CLI layout surface before capability packs migrate.
