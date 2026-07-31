# Publish publicly with OIDC and provenance

Matty uses the public canonical repository `yersonargotev/matty` and public npm
package `@yargote/matty`. Package metadata declares public access, provenance,
and the case-exact repository.

Stable releases are initiated only by individually pushing a protected,
lightweight tag named strictly `vX.Y.Z`. Before protected credentials are
available, the workflow
checks that the tag, `package.json`, and both package-lock root versions agree;
that the tagged commit is an ancestor of `origin/main`; and that the version is
not already public. A failed check performs no certification or registry write.

The macOS certification job has the workflow's single `release-candidate`
environment approval. It builds and tests one dynamically versioned tarball and
emits `SHA256SUMS` plus structured `RELEASE-EVIDENCE.json`, binding the artifact
to its tag, ref, commit, repository, workflow, and run. The protected reference
credential is written to a temporary file and exposed only to T04; neither
`PI_AUTH_JSON` nor its path is inherited by unrelated subprocesses.

The exact certified tarball is passed to a minimal Ubuntu `npm-stage` job. That
environment is retained solely because it is part of npm's configured trusted
publisher identity; it has no GitHub reviewer gate in live settings. This is the
only job with `id-token: write`, and it runs exactly one
`npm stage publish <tarball> --tag latest`. Routine releases have no npm token,
no direct `npm publish`, and no automated npm approval. The trusted publisher
contract remains stage-only and is tracked by activation issue 17.

The workflow records npm's returned stage identifier as structured evidence.
After staging, a separate `contents: write` job revalidates that the live tag
still targets the certified commit, creates a draft GitHub Release for that tag,
and attaches the certified tarball, checksum, certification evidence, and stage
evidence. Its body explains how a maintainer uses `npm stage list`,
`view`, and `download`, compares the staged tarball, then runs `npm stage
approve <stage-id>` and completes npm 2FA. Approval makes the version public and
is an explicit human decision.

A later Ubuntu job in the same workflow run polls the public registry for up to
six hours. It does not depend on a downstream GitHub release event. Once the
version appears, it independently downloads the public tarball and requires:

- the certified SHA-256 digest;
- the registry SHA-1 shasum and Subresource Integrity digest;
- npm attestations containing SLSA provenance for the canonical repository,
  `release-candidate.yml`, tag, source commit, and originating run.

It also runs npm's signature and attestation audit in an isolated temporary
project. It revalidates the live tag target, attaches all public verification
evidence to the draft, and only then changes the existing GitHub Release from
draft to published. Timeout,
metadata mismatch, artifact mismatch, failed signature verification, missing
provenance, or identity mismatch fails before that final mutation, so the
GitHub Release safely remains a draft.

The initial `0.1.0` bootstrap publication was a one-time prerequisite because
npm requires an existing package before trusted-publisher configuration. Its
token path is not part of routine releases and must never be restored.
