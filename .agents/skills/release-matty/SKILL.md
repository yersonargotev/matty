---
name: release-matty
description: Release Matty through its certified staged npm path. Use when a new @yargote/matty version must be prepared or published.
---

# Release Matty

Release one stable version through a protected tag, certification, OIDC npm
staging, human npm 2FA approval, and same-run public verification. The workflow,
release scripts, trusted-publisher JSON, and ADR 0019 are authoritative.

## 1. Resolve the live contract

Work only in canonical `yersonargotev/matty`. Read `AGENTS.md`, `package.json`,
the lockfile, `.github/workflows/release-candidate.yml`,
`release/trusted-publisher.json`, the scripts under `scripts/release/`, and ADR 0019. Confirm activation issue 17 is complete, the bootstrap token and secret
are revoked, and npm's trusted publisher is exactly workflow
`release-candidate.yml`, environment `npm-stage`, action `stage-publish`.

Inspect GitHub environment settings read-only. `release-candidate` must have one
protected approval for certification. `npm-stage` must have no reviewer gate;
its environment name remains solely part of the trusted-publisher identity.
Stop on any identity, protection, token, or action discrepancy.

## 2. Prepare and merge an exact version

Require an approved issue that gives an unambiguous stable SemVer. Do not infer
release scope. Ensure `@yargote/matty@X.Y.Z` is not public. Update
`package.json`, both package-lock root version fields, and every functional
version assertion. Run focused checks and `npm run check` with isolated HOME,
npm userconfig/cache, and temporary paths. Merge through normal review and
confirm the resulting commit is on `origin/main`.

Routine release code must contain no npm write token, direct `npm publish`, or
automatic npm stage approval.

## 3. Create the protected release tag

Create exactly one lightweight `vX.Y.Z` tag at the reviewed commit (for example,
`git tag vX.Y.Z <commit>`) and push that tag individually. Do not use an
annotated tag, dispatch the workflow, push several release tags together, move
an existing tag, or tag a commit outside `origin/main`. Record tag, commit, and
the resulting workflow run URL.

The unprotected validation job must pass before credentials are exposed. It
checks strict tag syntax, manifest/lock agreement, main ancestry, and that the
version is not public. Approving the `release-candidate` environment authorizes
use of the protected reference credential to certify this exact commit.

## 4. Check certification and staging

Require the macOS certifier to upload exactly one tarball, `SHA256SUMS`, and
`RELEASE-EVIDENCE.json`. Evidence must bind package/version and digest to the
tag, ref, commit, repository, workflow, and run. The reference auth path may be
visible only to T04.

The `npm-stage` job then submits that exact tarball through the configured OIDC
identity with one `npm stage publish <tarball> --tag latest`. It has the only
`id-token: write` permission, records npm's stage ID, and does not make the
version public. Require the stage job and draft-release job to succeed. The
draft job must revalidate that the live tag still targets the certified commit;
the draft GitHub Release must contain all certified and npm stage evidence.

## 5. Inspect and approve the npm stage

Use a fresh temporary HOME/config/cache. Authenticate interactively when npm
requires it; never request, proxy, print, or store a password, token, recovery
code, or 2FA code.

Run:

```sh
npm stage list @yargote/matty
npm stage view <stage-id>
npm stage download <stage-id>
```

Compare package, version, file list, stable `latest` tag, and downloaded bytes
to the certified evidence and SHA-256. Inspect install/runtime behavior where
required. On any discrepancy, stop and, only with explicit authorization, run
`npm stage reject <stage-id>` and let the maintainer complete 2FA. A corrected
release needs a new merged commit, tag, and run.

Before approval, state package/version, stage ID, tag, commit, run, dist-tag,
and digest. Explain that publication is irreversible for that version. Obtain
explicit user authorization, then run only:

```sh
npm stage approve <stage-id>
```

Pause for the maintainer's interactive npm 2FA. GitHub environment approval is
not a substitute for npm approval.

## 6. Let the same run verify and publish the release

The waiting workflow job polls npm for up to six hours. It must independently
verify the public tarball's certified SHA-256, registry shasum and integrity,
npm's signature/attestation audit, and SLSA provenance identity for repository,
workflow, tag, source commit, and run. It revalidates the live tag target and
attaches public evidence before publishing the existing draft GitHub Release.
It does not rely on a downstream
`release` event.

Do not manually publish the GitHub draft while verification is pending. On
timeout or mismatch, it remains a draft; investigate and preserve evidence.

## 7. Record completion

Record package/version, issue and PR, tag/commit, workflow run, stage ID,
human approval, certified/staged/public digest comparison, registry shasum and
integrity, provenance identity, public npm URL, and GitHub Release URL. Close
the release issue only after npm is public, the same run published the draft,
all acceptance criteria pass, and no staged candidate or cleanup remains.
