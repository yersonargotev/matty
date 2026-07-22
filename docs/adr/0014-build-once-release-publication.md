# ADR 0014: Publish one verified release candidate once

## Status

Accepted.

## Context

Packy's release workflow already retains one binary build through package
validation, GitHub Release upload, and Homebrew formula generation. Its
publication boundary is not yet immutable: the candidate has no SBOM or OIDC
provenance, a retry may rebuild it, assets are uploaded with clobber enabled,
and one job holds both GitHub Release and Homebrew authority.

Tag and release enforcement must not be enabled until publication can prove the
identity and completeness of one candidate before a version becomes public.
The same contract must also support a pre-publication retry without treating a
different build, draft, version, or set of bytes as recovery.

## Decision

One authorized, freshly checked commit from Packy's protected `main` branch
defines a release candidate. The candidate identity binds the exact version,
repository, `refs/heads/main` commit, release workflow identity and digest,
effective permissions, release notes, and the ordered names and SHA-256 digests
of every retained artifact. Packy builds the platform binaries once and adds a
complete `SHA256SUMS` manifest and SPDX SBOM without rebuilding a binary.

Deterministic release-domain code under `internal/release` owns candidate,
provenance, and publication-state validation. GitHub Actions and repository
scripts are adapters: they acquire observations and credentials, invoke the
domain contract, and stop before a write when validation fails.

OIDC build provenance is created only after all non-mutating candidate checks
pass. Its job has only `contents: read`, `id-token: write`, and
`attestations: write`, signs the exact retained subjects, and persists its
Sigstore bundle with the candidate. The bundle is verified against Packy's
repository, release workflow, workflow digest, protected-main ref, source
commit, and artifact digests before publication. A dry run completes all safe
local and read-only checks, reports the exact proposed writes, and stops before
OIDC issuance, draft creation, asset upload, publication, or tap mutation.

GitHub publication first creates or resumes one draft for the exact version and
candidate. Existing draft metadata and assets are read back; matching present
assets are retained, missing assets may be uploaded once without clobber, and
any extra, duplicate, stale, or mismatched state fails closed. The complete
draft is read back and verified before one transition to published. Automation
never moves a tag or deletes, recreates, replaces, overwrites, or reuses a
published version.

GitHub Release and Homebrew publication use separate jobs and authority.
Homebrew receives only the tap-scoped credential and begins after independently
reading back the published GitHub Release and its server-computed asset hashes.
An exact already-published release may be read back only to continue or verify
the downstream Homebrew step; it is never reopened or mutated.

## Consequences

- The release workflow consumes one retained candidate across validation,
  attestation, draft verification, GitHub publication, and Homebrew generation.
- A pre-publication retry is resumable only while its draft identity and every
  existing asset still match the original candidate.
- GitHub's OIDC and attestation permissions are a narrow reviewed exception to
  the repository-wide default denial; no other job receives them.
- Homebrew failure cannot authorize replay or replacement of GitHub Release
  state. A retry verifies the existing public bytes before continuing the tap.
- Actions artifacts remain transport, not identity. Expiry or missing retained
  bytes makes recovery fail closed rather than rebuild the version.
- Historical tags, releases, and assets remain authentic and are not rewritten.

## Non-goals

- Creating protected environments or migrating live credentials.
- Enabling protected-tag, immutable-release, or repository-rule enforcement.
- Publishing or changing a real Packy tag, version, release, attestation, or
  Homebrew formula while implementing or testing this decision.

## Enforcement

Pure fixtures cover deterministic identity, exact checksums and SBOM subjects,
provenance binding, same-draft recovery, published read-back, and fail-closed
ref, permission, digest, duplicate, extra, and divergent state. Workflow tests
require empty default permissions, isolated OIDC/GitHub/Homebrew jobs, absence
of clobber and replacement paths, draft verification before publication, and a
dry-run boundary before every external mutation. Repository validation runs
without exercising a real release destination.
