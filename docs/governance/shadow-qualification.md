# Governance shadow qualification

Issue [#172](https://github.com/yersonargotev/packy/issues/172) qualifies the
advisory controls defined by the rollout plan. It does not promote, require, or
change a control. The **authoritative final-HEAD record is an Owner-reviewed
issue comment on #172** containing sanitized evidence (or durable links to it)
and the candidate PR/head. Committed examples, CI artifacts, local output, and
earlier comments are supporting evidence only.

Run the non-destructive collector with an explicit candidate identity:

```bash
./scripts/qualify-governance-shadow.sh \
  --repo yersonargotev/packy --pr NUMBER --head FULL_SHA --output-dir /tmp/packy-shadow
```

It performs local evidence validation over projected, read-only `gh api` GETs;
it does not run the scenario test matrices. The operator separately supplies
the focused sandboxed test evidence named below. The collector checks the five
names in [the registry](advisory-checks.md), GitHub Actions App ID
`15368` and slug `github-actions`, the expected workflow paths, their definition
blob identities, and the current PR head. It emits sanitized JSON and Markdown;
posting or editing the issue comment is deliberately outside the script.

Before collecting the final record, run the focused local matrices with
sandboxed configuration roots and attach their summarized result:

```bash
sandbox=$(mktemp -d)
HOME="$sandbox/home" XDG_CONFIG_HOME="$sandbox/xdg" \
  go test ./internal/governanceauth ./internal/ci
rm -rf "$sandbox"
```

The four ordinary Actions check runs expose job names; the collector qualifies
them only after fetching and verifying the exact `CI` or `Security` workflow
display name and path. Governance is a legacy commit status:
the collector follows its Actions target URL, verifies the current-head
`Governance` run and trusted-base definition, verifies the latest status record's
concrete `github-actions[bot]` publisher identity, and corroborates it with a job
in that run from the expected GitHub Actions App. Older pending, failed, or
successful status-history entries do not invalidate a later successful rerun;
the latest record is selected by its numeric status ID.

## Complete scenario matrix

Every row records actor, UTC time, repository/ref/head, workflow path and
definition SHA, effective permissions, accessible environment and secret
**names/classes only**, expected/result, denial proof, and rollback. `Allow` is
limited to the named behavior; all additional authority is a denial.

| Scenario | Expected shadow result and safe proof |
| --- | --- |
| Owner PR | All five exact current-head contexts succeed from the registered source; protected-PR flow only. |
| Fork PR | Read-only, secretless checks run; repository writes, merge, environments, secrets, and publication are denied. |
| Explicitly delegated proposal | Issue-bound proposal is allowed; self-review, control/credential access, publication approval, and break-glass are denied. |
| Dependabot | `app/dependabot` on `dependabot/*` may propose; merge, arbitrary refs, environments, secrets, and release are denied. |
| Synchronization | Protected dispatch may create its bound `sync/*` proposal; wrong actor, run, head, ref, branch, or PR binding is denied. |
| Sensitive paths | CODEOWNERS/security ownership is observed; an unowned or unapproved change cannot gain authorization. |
| Approved issue link | Open, same-repository, unambiguous closing issue with exactly `status:approved` succeeds for the current head. |
| Unapproved/ambiguous link | Missing, cross-repository, closed, stale, multiply-statused, inaccessible, or ambiguous evidence is denied. |
| `private-security` exception | Deterministic denial, per ADR 0013; no advisory lookup is attempted. |
| `urgent-revert` exception | Only the canonical open, same-repository, PR-bound retrospective inside 24 hours succeeds; malformed/stale records fail. |
| `automation` exception | Only the canonical PR comment, protected successful run, actor, branch, PR, and current-head binding succeeds. |
| New head | Every prior scenario and context observation is stale and unusable; collect a complete replacement set. |
| Missing/wrong source check | Missing, duplicate, renamed, non-successful, wrong-App, wrong-path, or stale check stops qualification. |
| Excess permissions | Any authority beyond documented job permissions stops qualification; no live write probe is made. |
| Unauthorized ref/environment/publication | Metadata/fixtures prove denial before access; never create a real tag, release, package, formula, deployment, or environment. |
| Pages | Only its approved protected source may deploy; no general contents write or publication secret authority. |
| Deterministic issue automation | Only its canonical issue evidence may be updated; contents, merge, environment, secret, and release authority are denied. |
| Installed Apps | Owner performs a read-only installation/repository/permission audit; no approval, expansion, reinstall, or destructive probe. |

## Approved substitutes and safe boundaries

Use a disposable PR or fork for positive check execution. Static workflow
inspection, committed validator fixtures, projected API metadata, job logs with
secrets redacted, and local dry-runs are approved substitutes for unsafe denial
probes. A protected workflow definition plus its definition blob SHA and an
API-observed current-head run is the approved read-only workflow-identity proof.
Permission declarations and API/UI metadata substitute for attempts to use a
forbidden token. Owner UI inspection is the only approved installed-App check.
`private-security`, recovery, break-glass, credential compromise, publication,
and destructive settings tests are tabletop/static only.

Never record secret values, tokens, cookies, headers, URLs containing
credentials, recovery material, personal recovery details, raw installation
responses, or unredacted logs. Stop on unexpected authority, a secret value,
API/independent-view disagreement, renamed/unavailable identity, stale head or
definition, unsafe probe, or inability to sanitize. Preserve only the minimum
projected evidence and report the blocked row.

## Invalidation, reruns, and sign-off

A candidate-head change invalidates the entire matrix. A relevant workflow,
validator, fixture, permission, environment, App installation, or policy change
invalidates every affected row and all five identity observations when its
scope is uncertain. A false failure is never crossed out in place: correct it,
rerun the affected row plus its positive/negative counterpart on the corrected
head and definition, and publish a replacement final-HEAD comment that links
the superseded record. Partial results cannot be promoted.

Only repository Owner `yersonargotev` may make the final promotion decision,
after reviewing the complete authoritative comment. The sign-off must state the
exact head and evidence comment, either explicitly authorizing the next stage or
recording why it remains blocked. Collection, a green matrix, or this document
is not Owner sign-off and changes no required check, setting, credential, or
environment.
