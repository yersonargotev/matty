# Upstream trust and provenance policy

Research date: 2026-07-13 (America/Bogota)

## Question and recommendation

This research defines the authenticity and provenance guarantees Matty must require before accepting a stable release or an explicitly selected commit from a public GitHub repository.

Matty should use a **pinned-source, recorded-evidence, human-merge** policy. Trust begins with an explicitly configured repository identity, not with a repository name discovered at runtime. Every candidate must resolve to a full commit SHA, retain the GitHub repository and release/tag identities that produced it, record GitHub's signature-verification evidence without overstating it, hash every selected file, and stop on any identity or provenance discontinuity. Automation may prepare a pull request, but only a human merge accepts the candidate.

Signatures strengthen this policy but are not a universal admission gate. A verified tag or commit is positive evidence. An object whose verification reason is exactly `unsigned` has no signature evidence and may be accepted only from an already trusted repository under the rules below. A present-but-unverified signature is not equivalent to an unsigned object and blocks acceptance. This distinction keeps the first validation source usable: `mattpocock/skills` release `v1.1.0` has an unsigned annotated tag but a GitHub-verified peeled commit.

## Evidence and limits

- A Git tag reference can be force-updated through GitHub's API. A tag name alone is therefore mutable provenance ([Git references API](https://docs.github.com/en/rest/git/refs#update-a-reference)).
- GitHub immutable releases lock their associated tag and assets, create a release attestation, and prevent reuse of the release tag after repository deletion and recreation. This is stronger evidence when an upstream enables it, but it is not universal ([Immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases)).
- GitHub exposes `verified`, `reason`, `signature`, `payload`, and `verified_at` for commit and annotated-tag objects. `unsigned` is one enumerated reason among invalid, malformed, unavailable, and identity-related failures ([Git commits API](https://docs.github.com/en/rest/git/commits#signature-verification-object), [Git tags API](https://docs.github.com/en/rest/git/tags#signature-verification-object)).
- GitHub's persistent commit verification records the verified state and time within a repository network; it does not continuously re-evaluate later key revocation or expiry. Matty must record it as GitHub-observed evidence, not as timeless proof of maintainer intent ([About commit signature verification](https://docs.github.com/en/authentication/managing-commit-signature-verification/about-commit-signature-verification#persistent-commit-signature-verification)).
- A repository transfer redirects old web and Git URLs, but creating a repository or fork at the old location permanently removes those redirects. A readable `owner/name` is therefore insufficient to distinguish a transfer from later name reuse ([Transferring a repository](https://docs.github.com/en/repositories/creating-and-managing-repositories/transferring-a-repository#about-repository-transfers)).
- Archiving makes releases, commits, tags, and branches read-only but can be reversed. Deletion makes the repository unavailable and some deleted repositories can be restored for 90 days ([Archiving repositories](https://docs.github.com/en/repositories/archiving-a-github-repository/archiving-repositories), [Deleting a repository](https://docs.github.com/en/repositories/creating-and-managing-repositories/deleting-a-repository), [Restoring a deleted repository](https://docs.github.com/en/repositories/creating-and-managing-repositories/restoring-a-deleted-repository)).
- A workflow's `GITHUB_TOKEN` is a short-lived installation token scoped to its repository. GitHub recommends declaring only the required permissions; omitted permissions become `none` when any are specified ([`GITHUB_TOKEN`](https://docs.github.com/en/actions/concepts/security/github_token), [Workflow syntax: permissions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions)).
- GitHub warns that untrusted workflow input and third-party code can expose secrets or write tokens, and recommends full-SHA pins for actions ([Secure use reference](https://docs.github.com/en/actions/reference/security/secure-use)). The synchronizer must treat upstream bytes as data and never execute them.

GitHub does not provide a general transparency log proving that an ordinary mutable tag was never moved away and later moved back. Matty can detect a tag whose current target differs from its recorded target, but cannot reconstruct an unobserved move-and-return. Immutable-release status or external attestations can improve that guarantee; absent those, the lock records exactly what Matty observed and approved.

## Admission policy

### 1. Establish the source identity explicitly

The hand-maintained source configuration must name the expected public GitHub repository. On first resolution, Matty records:

- GitHub's numeric repository `id` and GraphQL `node_id`;
- canonical `full_name` and clone/API URLs;
- owner login, numeric owner `id`, and owner `node_id`; and
- visibility, archived/disabled state, and observation time.

The repository ID is the primary continuity key; `owner/name` remains the reviewable locator. The ID is an API identity signal, not an independent cryptographic identity. If Matty has no prior lock for the source, the first candidate requires normal human PR review to establish trust on first use.

### 2. Resolve every candidate to immutable Git objects

For an automatic stable release, Matty must:

1. select a published, non-draft, non-prerelease GitHub release;
2. retain its release `id`, `node_id`, exact `tag_name`, timestamps, author identity, `target_commitish`, and `immutable` flag;
3. resolve `refs/tags/<exact-name>` and retain its object type and SHA;
4. recursively peel annotated tag objects to a commit and retain every tag-object SHA; and
5. fetch content only by the resulting full commit SHA.

For a manual exact-commit selection, Matty must require a full SHA, verify that GitHub serves that commit through the configured repository, record any requested/observed ref separately, and fetch content by the full SHA. It must never retain a branch, abbreviated SHA, release `target_commitish`, or tag name as the effective lock.

### 3. Evaluate signatures without inventing trust

Matty records GitHub's complete verification result for every annotated tag object and the peeled commit: `verified`, `reason`, `verified_at`, and hashes of any returned signature and signed payload. Raw signature material need not be duplicated in the lock.

| Observed evidence | Automatic stable release | Manual exact commit |
| --- | --- | --- |
| At least one relevant tag/commit object has `verified: true` and `reason: valid`; no object has a present-but-unverified signature | Eligible | Eligible |
| All relevant objects report exactly `reason: unsigned` | Block automatic acceptance; produce a candidate requiring an explicit source-policy exception and human merge | Eligible only when the operator explicitly selected that SHA and the PR calls out the absence of signature evidence |
| A signature is present but GitHub does not verify it (`invalid`, `malformed_signature`, unknown/untrusted identity/key reasons, and similar) | Block | Block |
| Verification is unavailable because of a GitHub service error | Retry; then block without changing the lock | Retry; then block without changing the lock |

An unsigned annotated tag may still lead to an automatically eligible release when the peeled commit is verified, as in the initial `mattpocock/skills` case. A signature proves control of a signing identity over an object; it does not prove that the change is safe, so byte comparison, validation, and human merge remain mandatory.

### 4. Detect moved tags and changed release evidence

Before proposing any update, Matty re-resolves the currently locked tag. If its ref target, annotated-tag chain, or peeled commit differs from the lock, synchronization fails closed and preserves the old lock and vendored bytes. The same rule applies if an ordinary release changes identity or becomes a draft/prerelease.

A maintainer may not silently "refresh" the lock to the moved tag. Recovery requires an explicit review that treats the new target as a new provenance decision and preserves the old evidence in Git history. An immutable release is recorded as stronger evidence, but Matty still pins and verifies its commit and content hashes.

### 5. Fail closed across repository lifecycle changes

| Change | Required behavior |
| --- | --- |
| Same repository ID, new canonical owner/name | Treat as a transfer/rename; stop automatic updates and require explicit re-trust of the new canonical identity and owner before changing source configuration |
| Configured name resolves to a different repository ID | Treat as repository replacement/resurrection; block completely and never inherit the old trust |
| Repository is archived | Existing vendored bytes and lock remain valid; block new automatic updates until a human confirms whether the archived source should remain trusted or be replaced |
| Repository is disabled, private, inaccessible, or deleted | Preserve the lock and vendored bytes; report the source as unavailable; do not fall back to a fork, mirror, redirect target with a different ID, cached branch, or same-name repository |
| Repository later returns | Require the recorded repository ID and provenance chain to match; otherwise treat it as a new source requiring explicit trust |

Repository state changes do not invalidate bytes already committed to Matty when their hashes still match. They do prevent Matty from claiming that a fresh candidate has continuous upstream provenance.

## Generated lock evidence

The generated provenance lock should retain enough evidence to reproduce the accepted snapshot and explain why it was admitted:

- lock schema and generator versions;
- provider plus repository numeric/GraphQL IDs, canonical name and URLs;
- owner numeric/GraphQL IDs and login observed at resolution;
- selection mode (`stable-release` or `exact-commit`) and requested selector;
- release identity and state when applicable: numeric/GraphQL IDs, exact tag, author ID/login, draft/prerelease/immutable flags, and creation/publication timestamps;
- exact tag ref target, each annotated-tag object SHA, and peeled full commit SHA;
- commit tree SHA and parent SHAs;
- tag and commit verification fields, `verified_at`, and SHA-256 hashes of returned signature/payload material;
- repository visibility plus archived/disabled state at observation;
- GitHub API version and resolution timestamp;
- every selected resource path and every contained file's path, byte size, and SHA-256; and
- deterministic aggregate hashes for each resource and the complete source snapshot.

Fields derived from mutable GitHub state are evidence of an observation, not substitutes for the immutable commit and byte hashes. The lock is accepted only in the same review that accepts the vendored bytes; failed runs never partially rewrite it.

## Credential and execution boundaries

Public upstream discovery and Git fetches require no upstream credential. The workflow must not use a maintainer PAT, SSH deploy key, upstream token, repository secret, or cloud credential for acquisition. It must not pass the destination repository's `GITHUB_TOKEN` to Git, HTTP clients, subprocesses, generated files, or commands that process upstream paths/content.

Upstream content is inert input: inspect, copy, hash, and diff it, but never execute upstream scripts, actions, hooks, generators, package-manager lifecycle scripts, or binaries. Disable Git hooks and submodule/LFS recursion unless a later source contract explicitly adds and reviews them. Pin all workflow actions to full commit SHAs.

For GitHub Actions, keep the repository default read-only and grant permissions at job scope:

```yaml
permissions: {}

jobs:
  inspect:
    permissions:
      contents: read

  publish:
    permissions:
      contents: write
      pull-requests: write
```

The inspect job performs selection, fetching, hashing, comparison, and validation without write credentials. The publish job receives only a deterministic, validated candidate, re-verifies its manifest/hashes, creates or updates the synchronization branch, and opens the pull request. It needs no `actions`, `checks`, `issues`, `id-token`, `packages`, `secrets`, or administration permission. Matty's current repository default is already read-only and Actions cannot approve pull requests; keep both settings.

The workflow-created PR may require a human to approve its CI run because GitHub limits recursive runs created with `GITHUB_TOKEN`. That is compatible with the standing human merge checkpoint and is not a reason to introduce a PAT or broader GitHub App credential.

## Validation against `mattpocock/skills` `v1.1.0`

First-party REST responses observed on the research date establish the initial case:

- repository `mattpocock/skills` has numeric ID `1148788086`, node ID `R_kgDORHkddg`, owner ID `28293365`, and is public, enabled, and unarchived ([repository API](https://api.github.com/repos/mattpocock/skills));
- release `v1.1.0` has ID `350942193`, is published/stable and not immutable ([release API](https://api.github.com/repos/mattpocock/skills/releases/tags/v1.1.0));
- `refs/tags/v1.1.0` points to annotated tag object `eabea89380927aadb93abf6e290a19334d249292` ([tag ref API](https://api.github.com/repos/mattpocock/skills/git/ref/tags/v1.1.0));
- that tag object is unsigned and peels to commit `d574778f94cf620fcc8ce741584093bc650a61d3` ([tag object API](https://api.github.com/repos/mattpocock/skills/git/tags/eabea89380927aadb93abf6e290a19334d249292)); and
- GitHub reports the peeled commit as cryptographically verified, with `reason: valid` and `verified_at: 2026-07-08T13:20:40Z` ([commit API](https://api.github.com/repos/mattpocock/skills/commits/d574778f94cf620fcc8ce741584093bc650a61d3)).

Under this policy the release is eligible for a synchronization candidate despite its unsigned tag because its peeled commit is verified, its complete identity is retained, and a human must still merge the byte-level change. The policy does not retroactively declare the current Matty bundle synchronized; the existing local drift and partial lock found by the bundle audit remain separate blockers for the future dry run.

## Contract for later tickets

The source-configuration and lock contract must represent repository identity continuity, selection mode, the full tag-to-commit chain, signature evidence, immutable-release state, and complete content hashes. The event-triggered workflow must stop on moved tags, repository identity/lifecycle changes, unverifiable signatures, or unavailable evidence; separate read-only inspection from narrowly privileged publication; and surface every exception in the PR brief. No later ticket should weaken these failures into warnings or add an automatic fallback source.
