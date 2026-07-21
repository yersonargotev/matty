# Structured CLI output

Packy emits versioned JSON when `--json` is present. The Claude Code cutover uses
`schema_version: 2` for every report below; version 1 is not extended with
Claude fields.

| Command | `report` |
| --- | --- |
| `packy install|update|uninstall --dry-run --json` | `classic-lifecycle-preview` |
| `packy install|update|uninstall --json` | `classic-lifecycle-result` |
| `packy doctor --json` | `doctor` |
| `packy pack show PACK --json` | `pack-show` |
| Pack activate/update/deactivate/reconcile preview | `pack-lifecycle-preview` |
| successful Pack apply | `pack-lifecycle-apply` |
| completed structured Pack failure | `pack-lifecycle-failure` |
| `packy pack status --json` | `pack-status-overview` |
| targeted `packy pack status PACK --surface SURFACE --json` | `pack-status` |

The exact offline schemas are under `schemas/cli/v2/`. Canonical redacted
fixtures are under `internal/cli/testdata/structured-output/v2/`; repository
validation compiles every schema and validates both the fixtures and live
producer examples.

## Classic lifecycle

Preview and result reports carry the operation and outcome plus these common
facts: `desired_surfaces`, `pending_prerequisites`, `preserved`, `blockers`,
`recovery`, and `state_transition`. Preview adds the ordered redacted `actions`
and `dry_run: true`. Result adds `committed`, ordered `completed_effects` and
`not_started_effects`, the optional `failed_effect`, sorted `warnings`, and the
managed skill count.

`desired_surfaces` is the sorted set `claude`, `codex`, `opencode`. A dry-run or
result is written before the existing outcome-to-exit mapping is applied:
`converged`, `applied`, and `applied-with-pending-prerequisite` exit zero; all
other completed outcomes exit nonzero.

## Doctor

Doctor contains ordered `checks` and a `summary`. The Claude checks retain this
exact order after the existing common and host checks:

1. `claude-binary`
2. `claude-version`
3. `claude-skills`
4. `claude-instructions`
5. `claude-hooks`
6. `claude-mcp`
7. `claude-readiness`

Each check uses `PASS`, `WARN`, or `FAIL` and includes remediation in `detail`.
Warnings exit zero. A complete report containing a failure is written before
`ErrDoctorUnhealthy` produces the nonzero process exit. Workstation context and
raw shared documents are not part of doctor JSON.

## Capability packs

`pack-show` publishes declared surfaces and, per surface, compatibility,
bindings, exclusions, optional modes, prompt authorities, and aliases. It does
not claim observed readiness.

Lifecycle preview publishes the sealed ordered phases and actions, contract,
compatibility, consent, preservation, blockers, expected readiness, observed
evidence, pending evidence, and recovery. Apply and failure reports retain that
redacted plan. Status publishes intent, projection health, compatibility,
readiness, evidence, and pending actions. Each readiness dimension remains
explicitly `{state:"known",value:true|false}` or
`{state:"unknown",value:null}`. `--require usable` writes the complete status
report before preserving the readiness-gate exit result.

## Ordering and redaction

Arrays representing sets are lexically sorted; examples include surfaces,
capabilities, blockers, evidence, aliases, bindings, exclusions, and
contributors. Arrays representing work retain sealed execution order; examples
include lifecycle phases, actions, completed effects, and not-started effects.
Checks retain diagnostic order, and status entries are ordered by pack then
surface.

Reports never include action payload content, raw mixed-store documents,
authentication material, or MCP environment values. Environment-bearing command
arguments preserve the key and replace the value with `<redacted>`. Human output
uses the same owner-provided facts and stable label order without reconstructing
compatibility, readiness, recovery, or version policy in `internal/cli`.
