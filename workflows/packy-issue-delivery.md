# Packy Issue Delivery

Status: Active

## Goal

Turn a requested Packy GitHub issue into a verified change merged to `main`
through one predictable delivery loop.

## Skill shape

The implementation is the project-local, model-invoked skill
`deliver-packy-issue` at `.agents/skills/deliver-packy-issue/SKILL.md`. Its
model-facing description triggers only complete delivery of a named Packy issue;
consultations, isolated reviews, and releases do not trigger it.

The skill is a thin orchestrator with four phases: **qualify**, **implement**,
**prove**, and **deliver**. Each phase ends on a checkable completion criterion.
This workflow is the full contract. The skill points here, to `AGENTS.md`, and
to existing diagnosis, TDD, and code-review skills instead of copying their
rules.

Per repository delegation policy, the primary agent retains requirements,
decisions, GitHub mutations, integration, and final verification. Safe read-only
validation, bounded non-overlapping implementation, and independent review
slices may be delegated.

## Workflow

Before every commit created by this workflow, run the repository-required
checks, currently `go test ./...`.

### Trigger

The user identifies one Packy GitHub issue by number or URL and explicitly asks
for complete delivery. Record the immutable issue contents and the starting
base commit fetched from `origin/main` before changing project or tracker state.

### 1. Qualify

Read this contract and the repository instructions. Fetch the issue, confirm it
is open and labeled `status:needs-review` or `status:approved`, classify it as a
bug, feature, or non-code change, and verify that its acceptance criteria remain
current and implementable.

For needs-review issues, investigate and reproduce the reported behavior where
applicable. Promote the issue to `status:approved` only after the evidence shows
the issue is valid. For approved issues, perform the lighter currency check.

Inspect `main`, `origin/main`, and the working tree. Use the normal checkout
when it is clean and synchronized; otherwise prepare a temporary clean worktree
from the fetched `origin/main` commit without changing operator state.

**Complete when:** the issue is valid and approved, its type and acceptance
evidence are recorded, the immutable starting `origin/main` commit is known, no
exception boundary is active, and the chosen workspace is isolated from
unrelated changes. Failed validation produces an exception brief and stops
before branch creation or code edits.

### 2. Implement

Create `fix/issue-N-slug`, `feat/issue-N-slug`, or `chore/issue-N-slug`
according to issue type. Use CodeGraph before source discovery when the
change needs architecture, symbol, call-flow, or impact analysis.

For a bug, establish the deterministic red loop and add the regression at a
valid seam before the fix. For a feature, advance in vertical tracer bullets
with public-seam tests where behavior is testable. For non-code work, use the
targeted verification appropriate to the changed artifact. Keep every diff
surgical and commit the coherent implementation.

**Complete when:** every acceptance criterion has a corresponding change and
focused check, the issue branch contains only intended commits, and those checks
pass from its current `HEAD`.

### 3. Prove

Run independent Standards and Spec reviews in parallel against the fixed
`<starting-base-sha>...HEAD` diff. Adjudicate every finding: fix accepted
findings and record evidence for rejected false positives. After the required
pre-commit checks pass, each accepted fix creates a new `HEAD` and restarts both
reviews.

When both reviews have zero actionable findings, run the final verification
gate from that same `HEAD`: acceptance checks, repository-required checks,
`git diff --check`, and relevant sandboxed real-boundary checks.

**Complete when:** both fresh review axes have zero actionable findings, every
acceptance criterion has evidence, and every local required check passes on the
unchanged final `HEAD`.

### 4. Deliver

Push the issue branch and create a PR to `main` with `Closes #N`, the change
summary, and validation evidence. Wait for every required CI check. Diagnose and
repair technical failures autonomously; any repair returns to **Prove**.

When CI is green for the proved `HEAD`, merge through GitHub with a merge commit
and delete the remote branch. Fetch with pruning and verify that `origin/main`
contains the merge. Fast-forward local `main` only when Git can preserve the
operator checkout; otherwise leave it untouched and report that it remains
behind. Then clean up the local issue branch. For temporary-worktree runs,
remove the worktree before deleting its branch; for in-place runs, switch to
`main` before deleting the branch.

**Complete when:** the PR is merged, the issue is closed, the issue branch is
absent locally and remotely, `origin/main` contains the merge commit, the
integration workspace is clean, operator changes remain untouched, and the
success brief reports the local `main` synchronization result. Release
publication is outside this workflow.

## Checkpoints

There are no routine checkpoints after successful qualification. Technical
failures, failing tests, and red CI remain autonomous repair work. Stop only
when acceptance criteria conflict or permit materially different behavior; no
deterministic reproduction or valid regression seam exists; implementation
requires a material scope, architecture, or real-user configuration change; or
a review finding needs an unstated product decision.

Failed qualification leaves issue labels and state unchanged. Every exception
presents one decision-ready brief before the workflow continues.

## Briefs

The success brief links the issue and PR; names the merge commit; summarizes
the change; maps evidence to acceptance criteria; reports tests, both review
axes, and CI; confirms cleanup; and notes preserved local state.

An exception brief presents evidence, explains why the workflow cannot choose
safely, lists concrete options, recommends one, and asks for exactly one
decision. Briefs link artifacts and omit raw logs.

## Definition of done

This workflow run is complete only when the **Deliver** criterion is satisfied
or an exception brief is waiting on the user's decision. This specification is
ready when an implementer can build `deliver-packy-issue` without asking another
question.
