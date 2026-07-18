# Structured CLI output

`packy doctor --json` and `packy pack status [pack] [--surface surface] --json` emit one JSON document on stdout. Human output remains the default. Every document has `schema_version: 1`; incompatible changes require a new version.

Doctor uses `report: "doctor"`, check severities `PASS`, `WARN`, or `FAIL`, and summary status `healthy`, `warnings`, or `failures`. WARN exits successfully. A completed report containing FAIL is written in full before the command returns `ErrDoctorUnhealthy`.

Pack overview uses `report: "pack-status-overview"`; targeted status uses `report: "pack-status"`. Both use an `entries` array. Intent is `{state:"absent",active:null,revision:null}` when no intent exists, otherwise `state:"known"`. A missing attempt is `null`; attempt outcomes are `applying`, `verified`, `recovery-required`, or `unknown` for an unrecognized persisted value. Each readiness dimension is `{state:"known",value:true|false}` when freshly observed and `{state:"unknown",value:null}` otherwise. Blockers, evidence, and pending actions are always arrays, including when empty.

Entries are ordered by pack then surface. Checks retain Packy's defined diagnostic order. Blockers, evidence, and pending actions are lexically sorted. Pre-report validation/inspection errors follow normal CLI error handling and may produce no JSON. Pack `--require usable` writes the complete report, then preserves the existing readiness-gate exit result.
