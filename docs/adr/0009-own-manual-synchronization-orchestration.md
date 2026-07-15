# ADR 0009: Own manual synchronization orchestration outside packsync

## Status

Accepted.

## Context

The manual-first synchronization operation must sequence Inspect, Classify and
Publish across GitHub Actions permission boundaries. It also needs policy for
dispatch schemas, per-source concurrency, transient retry, operational failure
artifacts, stable branch and pull-request ownership, and decision-readiness
invalidation.

Those concerns do not belong to the deterministic synchronization engine or to
compatibility classification. ADR 0007 makes `internal/packsync` the owner of
sealed plans, exact candidates, Apply and Recover. ADR 0008 makes
`internal/packclassification` the owner of AI and human classification
orchestration while reserving floors, versions and evidence admission to
`packsync`.

## Decision

`internal/packsyncworkflow` owns the private manual synchronization operation:
dispatch admission, phase sequencing, failure classification and bounded retry,
operational artifacts, GitHub branch and PR ownership decisions, publication
admission, and exact-identity decision readiness and invalidation.

The GitHub Actions YAML is a least-privilege adapter. Inspect has only
`contents: read`; Classify has `contents: read` and `models: read`; Validate has
only `contents: read`; Publish has only `contents: write` and
`pull-requests: write`. The workflow has no automatic
trigger, starts with empty permissions, pins actions by full SHA, serializes by
source without cancelling an active run, and completes classification and
Matty-owned validation before entering the publication job.

`internal/tools/syncpacksource` is a private repository adapter for those four
phases and is not a public or distributed Matty command. Publish performs exact
candidate reacquisition, canonical Apply, diff and complete validation in its
sandbox, then freshly reobserves publication state before its first write.

`internal/packsyncworkflow` does not derive affected packs, mechanical floors,
versions or plans; validate classifier evidence; mutate the bundle; or recover
a transaction. It composes `internal/packsync` and
`internal/packclassification` without moving their authority. It similarly
does not grant the classifier GitHub or mutation authority.

Publication owns only `sync/<source-id>` and at most one open PR for that
source. Edited or human-owned state, divergence, unexpected or ambiguous
identity, stale bindings, moved provenance, regression and closed PR state all
block without overwrite or competing publication. Readiness is bound to the
exact validated identity, auto-merge remains disabled, and merge remains a
manual maintainer decision.

## Consequences

- Workflow and GitHub policy have one Matty-owned module rather than living as
  deterministic rules in YAML.
- The write-capable phase remains narrow and cannot admit an unclassified or
  incompletely validated proposal.
- A promoted pending run performs a new Check; artifacts from superseded runs
  never establish freshness.
- Terminal pre-PR failures are recoverable from a canonical 30-day artifact
  without leaking credentials or upstream bytes or opening an issue.
- Sandboxed fakes can prove publication and concurrency policy without a real
  dispatch, source branch, PR, merge, model call or refresh.

## Non-goals

- A scheduled synchronization or real refresh.
- A public Matty command or new distributed binary.
- Automatic merge, issue publication, or implicit AI-to-human fallback.
- The maintainer skill delivered by the later implementation slice.

## Enforcement

Structural tests require the manual-only trigger, empty default permissions,
full action SHAs, per-source non-cancelling concurrency, three distinct jobs,
and the accepted job permissions. Domain and sandbox lifecycle tests cover the
retry boundary, human two-dispatch flow, operational artifact safety,
publication ownership, stale and divergent states, active/pending
supersession, fresh Check, and readiness invalidation. The repository's
validation authority runs before delivery.
