# Packy Issue Delivery

Status: Active

## Goal

Turn a requested Packy GitHub issue into a verified change merged to `main`
through one predictable delivery loop:

`LOCAL[qualify -> optional bug diagnosis -> LOOP(implement -> code-review)]`
`-> NON-LOCAL[PR -> wait for CI -> green CI -> merge]`

## Skill shape

The implementation is the project-local, model-invoked skill
`deliver-packy-issue` at `.agents/skills/deliver-packy-issue/SKILL.md`. Its
model-facing description triggers only complete delivery of a named Packy issue;
consultations, isolated reviews, and releases do not trigger it.

The skill is a thin orchestrator over the two execution modes below. This
workflow is the full contract. The skill points here, to `AGENTS.md`, and to
existing diagnosis, implementation, delegation, and code-review skills instead
of copying their rules.

The primary agent retains requirements, decisions, integration, final
verification, and every GitHub mutation. Safe bounded implementation and
independent review slices may be delegated.

## Workflow

Remote reads are allowed in **LOCAL**. GitHub mutations begin only in
**NON-LOCAL**. Before every local commit, run the repository validation
authority, `./scripts/validate-packy.sh`.

### Trigger

The user identifies one Packy GitHub issue by number or URL and explicitly asks
for complete delivery. Record the immutable issue contents and the starting base
commit fetched from `origin/main` before changing project or tracker state.

### 1. LOCAL — Qualify

Read this contract and the repository instructions. Fetch the issue, confirm it
is open and labeled `status:needs-review` or `status:approved`, classify it as a
bug, feature, or non-code change, and verify that its acceptance criteria remain
current and implementable.

For a bug, apply `diagnosing-bugs` only while its reproduction, cause, or failure
boundary remains uncertain. When the issue already supplies sufficient diagnosis
and a clear reproducible regression, proceed directly to the local loop. Feature
and non-code branches do not invoke `diagnosing-bugs`.

For a needs-review issue, gather enough evidence to approve or reject it, but
record approval only as deferred intent. LOCAL performs no label or other GitHub
mutation. For an approved issue, perform the lighter currency check.

Inspect `main`, `origin/main`, and the working tree. Use the normal checkout when
it is clean and synchronized; otherwise prepare a temporary clean worktree from
the fetched `origin/main` commit without changing operator state.

**Complete when:** the issue is valid, its type and acceptance evidence are
recorded, any approval mutation is deferred, the immutable starting
`origin/main` commit is known, no exception boundary is active, and the chosen
workspace is isolated from unrelated changes. Failed validation produces an
exception brief and stops before branch creation or code edits.

### 2. LOCAL — Implement-review loop

Create `fix/issue-N-slug`, `feat/issue-N-slug`, or `chore/issue-N-slug`
according to issue type. Use CodeGraph before source discovery when the change
needs architecture, symbol, call-flow, or impact analysis.

Run these steps for every iteration:

1. Record `iteration-base-sha = HEAD` and an **iteration brief** that states the
   exact behavior or review repair this iteration must deliver.
2. Run Delegation Preflight. Delegate only a bounded local implementation slice
   with explicit file or module ownership. Keep small, cross-cutting,
   architectural, decision-dependent, or overlapping work inline. The primary
   agent inspects and integrates delegated changes and records the accepted or
   rejected handoff evidence.
3. Apply `implement` to the iteration brief. For a bug with a valid regression
   seam, apply `tdd`; for a feature, advance one vertical tracer bullet with
   public-seam tests where behavior is testable; for non-code work, use targeted
   artifact verification. Keep the delta surgical, run the required checks, and
   create one coherent local commit. Do not push it.
4. Apply `code-review` with independent Standards and Spec axes against exactly
   `iteration-base-sha...HEAD`. Give the Spec review the immutable issue and the
   iteration brief; it judges the obligations of this delta rather than treating
   earlier out-of-delta work as missing.
5. Adjudicate every finding. Rejected findings retain concise evidence. Each
   accepted finding becomes a new iteration brief and returns to step 1, so its
   repair receives its own implementation commit and review delta.

Maintain cumulative evidence that maps every issue acceptance criterion to its
implementation and focused check. Once the latest iteration has zero actionable
findings, run the final local gate on the unchanged `HEAD`: all acceptance
checks, `./scripts/validate-packy.sh`, `git diff --check`, and relevant sandboxed
real-boundary checks. Do not add a cumulative code review; every committed delta
has already received its paired review.

**Complete when:** every implementation commit has a paired review of exactly
its preceding `iteration-base-sha...HEAD` delta, every finding is adjudicated,
the latest review has zero actionable findings, every acceptance criterion has
cumulative evidence, the issue branch contains only intended commits, and every
local gate passes on its unchanged final `HEAD`.

### 3. NON-LOCAL — Deliver

Re-read the issue before its first mutation. If a needs-review issue still
matches the qualified snapshot, replace `status:needs-review` with
`status:approved`; if it changed materially or the mutation fails, stop before
the PR with an exception brief.

Push the issue branch and create a PR to `main` with `Closes #N`, the change
summary, and validation evidence. Wait for every required CI check on the exact
locally proved `HEAD`.

Classify a failed check before acting:

- Retry an infrastructure failure or flake without changing code.
- For a failure attributable to the change, return to the LOCAL
  implement-review loop, record a new `iteration-base-sha`, implement and review
  the repair, rerun the final local gate, push the new `HEAD`, and wait for CI
  again.
- If a bug failure restores uncertainty about reproduction, cause, or boundary,
  apply `diagnosing-bugs` before the repair iteration.

Merge only when every required check is green for the exact proved PR `HEAD`.
Merge through GitHub with a merge commit and delete the remote branch. Fetch
with pruning and verify that `origin/main` contains the merge. Fast-forward local
`main` only when Git can preserve the operator checkout; otherwise leave it
untouched and report that it remains behind. Then clean up the local issue
branch. For temporary-worktree runs, remove the worktree before deleting its
branch; for in-place runs, switch to `main` before deleting the branch.

**Complete when:** the PR is merged, the issue is closed, the issue branch is
absent locally and remotely, `origin/main` contains the merge commit, the
integration workspace is clean, operator changes remain untouched, and the
success brief reports the local `main` synchronization result. Release
publication is outside this workflow.

## Checkpoints

There are no routine checkpoints after successful qualification. Technical
failures, failing tests, review repairs, and red CI remain autonomous loop work.
Stop only when acceptance criteria conflict or permit materially different
behavior; no deterministic reproduction or valid regression seam exists;
implementation requires a material scope, architecture, or real-user
configuration change; the issue changes materially before its first mutation;
or a review finding needs an unstated product decision.

Failed qualification leaves issue labels and state unchanged. Every exception
presents one decision-ready brief before the workflow continues.

## Briefs

The success brief links the issue and PR; names the merge commit; summarizes the
change; maps evidence to acceptance criteria; reports local validation, every
iteration review, and CI; confirms cleanup; and notes preserved local state.

An exception brief presents evidence, explains why the workflow cannot choose
safely, lists concrete options, recommends one, and asks for exactly one
decision. Briefs link artifacts and omit raw logs.

## Definition of done

This workflow run is complete only when the NON-LOCAL criterion is satisfied or
an exception brief is waiting on the user's decision. This specification is
ready when an implementer can run `deliver-packy-issue` without asking another
question.
