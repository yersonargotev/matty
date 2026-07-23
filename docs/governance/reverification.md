# Governance drift and recurring re-verification

Packy's governance controls are compared with the versioned
[`expected-state.v1.json`](expected-state.v1.json) contract. The comparison is
read-only. It never repairs repository settings, refs, environments,
credentials, Apps, releases, packages, Pages, workflows, or protection rules.
The architectural contract is [ADR 0015](../adr/0015-detect-governance-drift-without-repair-authority.md).

## Automated cadence

`Governance drift` runs every Monday at 08:43 UTC and may also be dispatched
manually from protected `main`. Its observer has only read permissions and
retains:

- the sanitized observation bound to repository, ref, commit, workflow blob,
  and UTC time; and
- the deterministic evaluation: `clean`, `confirmed-drift`,
  `unclassifiable-drift`, or `collection-failure`.

A separate reporter has `issues: write` and may mutate only the canonical
`Packy governance drift detected` issue. It creates, updates, deduplicates, or
resolves that signal; it has no control-repair authority.

Release and pack-source synchronization repeat the same collection immediately
before their first affected action. Publication drift stops Release before
build, OIDC, draft creation, upload, publication, or Homebrew mutation.
Promotion drift stops synchronization before inspection or proposal writes.
An unaffected boundary remains available.

## Canonical issue and classification

The issue records the affected control and boundary, actor, UTC time,
repository/ref/commit, workflow definition SHA, expected and observed sanitized
state, collection commands/APIs, classification, owner, durable run, and rerun
result.

A clean rerun does not erase an unclassified signal. The Owner classifies the
exact evidence digest by posting only this marker to the canonical issue:

```text
<!-- packy-governance-classification
evidence: sha256:EXACT_DIGEST_FROM_THE_ISSUE
classification: reviewed
-->
```

Only an `OWNER` comment with the exact current digest is accepted. A different
digest, a non-Owner comment, or general issue discussion does not classify the
evidence. The affected boundary resumes only after both exact-evidence
classification and a clean current rerun.

## Immediate re-validation

Do not wait for the weekly schedule. Dispatch `Governance drift` immediately
after any:

- repository, Actions, merge, branch-protection, tag-rule, environment, Pages,
  release, package, or workflow control change;
- incident, suspected compromise, break-glass use, or restoration attempt;
- installed-App permission, pending-expansion, or repository-selection change;
- credential creation, rotation, scope, destination, expiry, revocation, or
  metadata change; or
- required-check name, source App, workflow identity, or protected-ref change.

Stop on collection ambiguity, unexpected authorization or denial, stale
identity, wrong-source checks, unsafe account/recovery probing, or any API and
independent-view disagreement. Classify and assign every gap before proceeding.

## Quarterly Owner review

Before the `review_due` time in
[`owner-attestation.json`](evidence/issue-176/owner-attestation.json), the Owner
reviews and replaces the sanitized attestation through a protected PR. The
review covers:

1. repository and Actions policy, protected `main`, required check identities
   and GitHub Actions App ID, version-tag policy, and immutable releases;
2. `release`, `homebrew`, and `github-pages` protection, branch policies, and
   secret-name metadata;
3. installed-App inventory, repository selection, effective authority classes,
   continued need, and every pending expansion (none is approved by silence);
4. at least two independent recovery methods and off-account recovery material,
   recording yes/no readiness only;
5. safe negative fixtures for Owner, delegated agent, fork, Dependabot,
   synchronization, canary/issue automation, release, Pages, and installed-App
   boundaries; and
6. residual Owner authority to edit release title/notes and the Owner-only
   break-glass whole-release deletion action that permanently burns the tag.

An absent, expired, malformed, or incomplete attestation is unclassifiable
drift. Never record secret values, tokens, recovery material, personal recovery
details, or raw installation responses.

## Tabletop-only scenarios

Break-glass, destructive publication recovery, account loss, credential
compromise, failed restoration, immutable-release deletion, and recovery-method
loss remain sanitized tabletop exercises unless a real catalogued incident
exists. Preserve an authenticated session and stop before logout, revocation,
rotation, tag/release mutation, protection weakening, or simulated lockout.

## Pre-release review

The release gate must prove the authorized `refs/heads/main` commit, current
governance result, protected environments, retained artifact hashes, SBOM,
provenance/attestation, protected tag policy, immutable-release state, and the
residual-authority policy. Drift detection does not rebuild, replace, reopen,
delete, or repair release state and never enables `private-security`
authorization through polling.
