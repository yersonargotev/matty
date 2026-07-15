# Pack source synchronization

## Status and scope

This is the canonical operational contract for the private maintainer workflow
`.github/workflows/sync-pack-source.yml`. It exposes one manual-first operation;
it is not a public Matty command, a distributed binary, a scheduled refresh, or
authorization to merge. The implementation workflow may create or update only
the owned synchronization branch and pull request described below. It never
opens an issue, enables auto-merge, merges, or falls back from AI to human
classification.

The private `internal/tools/syncpacksource` adapter sequences domain behavior.
It must not reproduce candidate admission, compatibility floors, plan sealing,
exact version selection, evidence validation, Apply, or Recover. Those remain
owned by `internal/packsync` and `internal/packclassification` under ADR 0007
and ADR 0008.

## Canonical dispatch

Every request conforms to
`workflows/schemas/pack-source-dispatch.schema.json`. It names one configured
`source_id`, an explicit candidate selector, an explicit `ai` or `human`
classification mode, and an operator reason. There are no automatic triggers.

`latest-stable` has no selector reference. `prerelease` carries one exact
published prerelease tag. `commit` carries one full lowercase commit SHA. An
AI request cannot carry human evidence and never changes modes automatically.

Human classification is inspection-first and evidence-second:

1. The first dispatch explicitly selects `human` without evidence. Inspect
   emits the canonical sealed plan and bound inspection identity, then stops.
2. The operator inspects that artifact. A second `human` dispatch selects the
   exact candidate commit and supplies canonical evidence bound to the exact
   inspection plan ID and base SHA. Missing or stale bindings block.

## Concurrency and freshness

The concurrency group is `sync-pack-source-<source-id>` with
`cancel-in-progress: false`. GitHub therefore leaves the one active run alone,
admits at most one pending run for that source, and a newer request replaces
only the older pending request. No run resumes another run's plan. Every run
that actually starts begins at Inspect and executes a fresh canonical Check.

The concurrency key is serialization, not freshness proof. Inspect seals the
candidate, base, plan ID, provenance, configuration and selection observation.
Publish must reobserve them immediately before its first write.

## Phases and permission boundary

The workflow starts with `permissions: {}`. Every external action is pinned by
one full commit SHA.

### Inspect — `contents: read`

Inspect checks out without persisted credentials and invokes:

```text
go run ./internal/tools/syncpacksource --phase inspect ...
```

The adapter validates the dispatch, creates an isolated acquisition directory,
and calls canonical `packsync.Check`. Its output is a sealed, immutable
inspection artifact. It contains identities, reasons, changes, blockers and
digests, not copied upstream resources or credentials.

### Classify — `contents: read`, `models: read`

Classify downloads that exact inspection artifact and invokes:

```text
go run ./internal/tools/syncpacksource --phase classify ...
```

It passes the sealed plan to `packclassification`. AI mode retries only model
transport failures according to the retry policy below. Human mode accepts
only the separately dispatched, inspection-bound evidence. The classifier has
no publication authority. It emits a classified-plan artifact, never a branch
or pull-request write.

### Validate — `contents: read`

Validate downloads the exact inspection and classification artifacts, invokes
`--phase validate`, reacquires and Applies the sealed candidate in its disposable
checkout, and runs the complete Matty-owned validation authority. Its canonical
proof contains identities and booleans only, never upstream bytes.

### Publish — `contents: write`, `pull-requests: write`

Publish is gated by all three prior jobs. It downloads only their sealed artifacts
and invokes:

```text
go run ./internal/tools/syncpacksource --phase publish ...
```

Before the first Git or GitHub write, the adapter uses an isolated checkout to
reacquire the exact candidate, calls canonical Apply (and Recover if canonical
transaction evidence requires it), renders the diff, runs the complete
Matty-owned validation suite again, evaluates ownership, and freshly reobserves
the repository and GitHub state. Only a proposal whose exact identity passes
all gates can reach the write operation. A first PR is created as a blocked
draft, reobserved, finalized, converted to ready, and reobserved again before
decision readiness is recorded. Validation and publication permission
are separated by job, and publication logic remains a narrow adapter around
Matty-owned domain behavior.

All phases set sandboxed `HOME` and `XDG_CONFIG_HOME`. Acquisitions, staged
checkouts, generated state and filesystem writes remain under runner-owned
temporary or checkout paths.

## Retries and failures

A single operation has at most three attempts. Attempt delays use bounded
exponential backoff. A valid server `Retry-After` delay replaces the computed
delay when it is longer. Only transport, rate-limit and service-unavailable
failures classified as transient retry.

Provenance, integrity, classification, validation, ownership and divergence
failures are terminal. Invalid evidence and AI unavailability after the bound
attempts remain blocked. Retrying never changes candidate or classification
mode and never implies AI-to-human fallback.

Before a PR exists, every terminal failure emits
`pack-source-operational-artifact.schema.json` as canonical JSON. It contains
the exact known source/plan/base/candidate identities, blockers, and concrete
recovery steps. It contains no credential, environment dump, request header,
model prompt, upstream file content, patch, archive, or other upstream bytes.
The workflow retains this artifact for 30 days and does not publish an issue.

## Publication ownership and fail-closed checks

The operation owns exactly `sync/<source-id>` and at most one open PR from that
branch to `main`. The automation identity is `github-actions[bot]`. A pristine
first publication may create both. A pristine advancing candidate may update
the same branch and PR. An exact already-published identity is a no-op.

Immediately before writing, Publish revalidates all of the following together:

- candidate commit and non-regressive relation;
- current base SHA and sealed plan ID;
- exact source provenance and proposed provenance digest;
- automation identity and the ownership record;
- stable branch name, head, ancestry and commit authorship;
- sole open PR identity, base/head/state, managed metadata and content; and
- the exact validated result tree and every decision-readiness gate.

Publish fails closed, without a force-push, metadata overwrite or competing
PR, when metadata was edited, a human commit is present, the branch diverged,
identity is unexpected, base or plan is stale, the candidate regresses,
provenance moved, the PR was closed, or automation ownership is absent or
ambiguous. A closed owned PR is an explicit blocker; automation does not create
a replacement. Reviewer-authored content is never normalized away.

## Decision readiness

The publication record conforms to
`pack-source-publication.schema.json`. A PR may be non-draft and marked
decision-ready only when these gates passed for one exact plan/base/head/
candidate/provenance/PR-state identity:

1. provenance;
2. classification;
3. exact candidate reacquisition;
4. canonical Apply;
5. expected diff;
6. automation ownership; and
7. the complete Matty-owned validation suite.

Auto-merge is false and manual merge remains required. A later change to base,
candidate, provenance, head, managed PR state, or the PR's open identity makes
the readiness record invalid. The next operation must start again with fresh
Inspect; readiness is not patched forward.

## Canonical proposal brief

Publish renders one canonical JSON proposal into Markdown without recomputing
domain facts. The JSON and brief carry the same information:

1. request actor and reason, source, selector, workflow run and attempt,
   candidate, plan ID, base/result head and branch/PR identity;
2. repository and owner identity, release, exact tag-to-commit resolution,
   verification, tree and parent identity, and provenance hashes;
3. selected resource and file additions, modifications, removals and moves,
   unselected discoveries, and old/new snapshot hashes;
4. affected packs, old and proposed versions, mechanical floors, final
   classifications, classifier identities and rationales, plus mandatory
   migrations and actions;
5. exact reacquisition, Apply, diff and every validation result, including the
   explicit fact that no upstream content was executed; and
6. blockers, decision-readiness state, invalidation conditions, and exact
   retry or recovery instructions.

The managed title/body markers and their canonical hash form part of ownership
revalidation. Logs are not the operational record, and the Markdown renderer
cannot add authority absent from the sealed JSON inputs.

## Sandboxed acceptance tracer

Deterministic fake source, GitHub, clock, sleeper, and concurrency fixtures
exercise: pristine creation; pristine update; exact no-op; base advancement;
candidate regression; divergent branch; edited metadata; human commits;
ambiguous ownership; closed PR; unexpected identity; human inspection then
evidence; unavailable AI; three-attempt exponential backoff; `Retry-After`;
non-retryable blockers; secret-free and upstream-byte-free failure artifacts;
active-run preservation and pending supersession; fresh Check when the promoted
pending run starts; valid readiness and later invalidation; and the permission
boundary between validation and publication.

The tracer uses no GitHub Models request, workflow dispatch, real source branch,
real synchronization PR, real merge, real refresh, or real bundle update. It
also preserves the seals, provenance bindings, shared lock and transaction
contracts delivered before this workflow.
