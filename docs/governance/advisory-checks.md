# Advisory governance and security checks

Issue [#169](https://github.com/yersonargotev/packy/issues/169) introduces the
repository policy surface and five check identities below. They are informative
only: no branch protection or repository ruleset requires them in this stage.

## Stable identity and source registry

| Qualified check identity | Workflow / job | Expected source | Advisory behavior |
| --- | --- | --- | --- |
| `CI / Validate Packy-owned code` | `CI` / `Validate Packy-owned code` | GitHub Actions; App ID `15368`, slug `github-actions` | Runs repository validation, the complete Go test suite (through validation), and advisory `govulncheck`; vulnerability findings remain visible without becoming required in this stage. |
| `CI / Claude 2.1.203 package smoke` | `CI` / `Claude 2.1.203 package smoke` | GitHub Actions; App ID `15368`, slug `github-actions` | Runs the exact supported Claude floor on pull requests. |
| `Governance / Validate authorization` | Protected `Governance` workflow commit status | GitHub Actions; App ID `15368`, slug `github-actions` | Accepts open, same-repository closing issues with exactly `status:approved`; every absent, ambiguous, stale, or cross-repository state is denied. |
| `Security / CodeQL` | `Security` / `CodeQL` | GitHub Actions; App ID `15368`, slug `github-actions` | Uploads Go analysis without becoming a merge requirement. |
| `Security / Dependency review` | `Security` / `Dependency review` | GitHub Actions; App ID `15368`, slug `github-actions` | Reports dependency risk with warning semantics; operational errors remain visible in the step result. |

The expected source is a policy binding, not proof. Issue #172 must observe each
exact name and App identity on current-head runs before any later ruleset can
require it. A rename or source mismatch stops promotion.

## Authorization boundary

The Governance workflow uses `pull_request_target` only to read pull-request and
issue metadata. It checks out the exact base SHA, never the proposed head, and
grants read-only metadata access plus the minimum `statuses: write` needed to
bind its result to the current PR head. Per-PR concurrency cancels stale runs so
an older approved snapshot cannot overwrite a later revocation. The checked-in
validator has no network or mutation capability.

The trusted workflow queries GitHub for the pull request's closing issues and
passes the projected fields to `internal/governanceauth`. The validator fails
closed unless the PR targets the default branch and every same-repository issue
that closes it is open and carries exactly one approved delivery status. Issue
label, close, and reopen events recompute affected open PRs so revoked evidence
cannot leave a stale successful result.

An ordinary PR uses GitHub-recognized closing references. A policy exception
uses exactly these two lines in the PR body:

```text
Authorization-Exception: private-security|urgent-revert|automation
Authorization-Record: https://github.com/yersonargotev/packy/<canonical-record>
```

`private-security` is denied fail-closed in this repository-only workflow.
GitHub Actions has no `repository_advisory` trigger, so accepting private
advisories could leave a stale successful status after an advisory changes.
Supporting that exception requires separately authorized external infrastructure
that can recompute every affected PR immediately. `urgent-revert` accepts an
open same-repository retrospective whose body links the PR and whose creation is
no later than 24 hours after the PR. The protected adapter projects only the
binding fact; it never persists the retrospective body.

`automation` accepts the PR itself for a current `app/dependabot` proposal on a
`dependabot/*` branch, or a successful completed `workflow_dispatch` run of the
protected `Synchronize pack source`/`sync/*` or `Release`/`release/*` proposal
path, initiated by `yersonargotev` and proposed by `app/github-actions`.

The validator rejects unknown or duplicate headers, mixed issue/exception
evidence, cross-repository records, inaccessible records, noncanonical URLs,
unbound records, and failed or stale record state. Issue and approved
proposal-workflow events search exact record URLs in open PR bodies and
recompute every match; the search result is only target discovery, never proof.

CI and Security also run weekly. Dependency Review compares the current commit
with its first parent outside pull-request events, so scheduled and `main` runs
remain metadata-only and advisory.

Positive and negative JSON fixtures live under
`internal/governanceauth/testdata/`. Run them with:

```bash
go test ./internal/governanceauth ./internal/tools/governanceauth
```

Secret Scanning and Push Protection remain GitHub platform controls. These
workflows do not read secrets, approve pull requests, change issues, write
repository contents, or alter repository settings.
