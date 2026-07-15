# Monitoring and evidence presentation

Load this reference after attachment or dispatch, for retries, or for explicit
human classification.

## Monitor

Poll the exact run ID with read-only `gh run view`/`gh api`; do not use external
events. Report only these distinct states: **solicitud aceptada**, **pendiente**,
**ejecución iniciada**, **bloqueada**, **sin cambios**, **PR publicada**,
**decision-ready**, **superada**, and **fusionada**. A URL-only request ends at
**solicitud aceptada**. Interrupted monitoring ends at **pendiente** with run ID,
URL, and last observed state.

Download artifacts by exact run ID into a fresh temporary directory. Validate
each claimed terminal artifact against its canonical schema fetched from remote
`main`. Logs are diagnostic only and never substitute for an artifact.

## Terminal truth

- Success is either a schema-valid no-op, or a publication artifact and live PR
  that remain decision-ready for the exact plan, candidate, base, result tree,
  head, PR identity, and validation gates. Merge is observed, never performed.
- An inspection-only human run intentionally waits without claiming success.
- A superseded run is valid only when linked to its replacement.
- A blocked/failed run requires a valid operational artifact, cause, run URL,
  and exact recovery. Missing, expired, invalid, or unexplained terminal
  evidence is **bloqueada**.
- Dispatch success and mere PR creation are intermediate. Never infer success
  from a conclusion, log line, branch, or PR alone.

Direct output contains the normalized request; preflight; current state and
links; source/selector/candidate; plan/base/head; per-pack version, mechanical
floor, classification and classifier; blockers; next action; and terminal
conclusion. Link full provenance, hashes, inventory/diff, canonical JSON, model
evidence, validation logs, brief, and artifacts rather than reproducing them.

Label every AI result `evidencia propuesta por IA; no aceptada todavía por el
mantenedor`. Present it without accepting, lowering, or rewriting it.

## Human flow

The first explicit human dispatch contains no evidence and stops after the
sealed inspection. Present immutable plan/evidence links, then collect for each
affected pack the human level, rationale, and mandatory migration/actions for a
major result. Never invent or complete rationale.

The second dispatch is a new `commit` request pinned to the inspected full SHA
and carries the original `plan_id`, base SHA, and canonical evidence. Reobserve
the plan and base before dispatch; any change discards the evidence and requires
a fresh inspection.

## Recovery

Validate an exact retry's operational artifact, pin its candidate full SHA, set
`retry_of_run`, and create a new dispatch. Never call any Actions rerun command.
Base advancement uses a fresh exact-candidate dispatch. Edited metadata, human
commits, divergence, closed PRs, unexpected identity, provenance discontinuity,
or regression are decisions for the workflow/maintainer; never repair them.
Candidate regression cannot be authorized away.

