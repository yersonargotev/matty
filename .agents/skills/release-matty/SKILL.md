---
name: release-matty
description: Release Matty through its certified staged npm path. Use when a new @yargote/matty version must be prepared or published.
---

# Release Matty

Release one version from the canonical repository through certification, npm
staging, human inspection, and explicit 2FA approval. Keep the repository's
workflow, policy, and release scripts as the single source of truth.

## Release workflow

Follow every step in order. Stop at a failed gate; never carry evidence or an
artifact forward from a failed or superseded run.

### 1. Resolve the live release contract

1. Work from the canonical `yersonargotev/matty` repository.
2. Read `AGENTS.md` and confirm the worktree state, current branch, remotes, and
   synchronization with `origin/main`.
3. Read these live sources before proposing commands:
   - `package.json`
   - `.github/workflows/release-candidate.yml`
   - `release/trusted-publisher.json`
   - `scripts/release/certify-candidate.mjs`
   - `scripts/release/verify-release-chain.mjs`
   - `docs/adr/0019-publish-publicly-with-oidc-and-provenance.md`
4. Confirm that the configured path is OIDC `stage-publish` from the canonical
   workflow and the `npm-stage` environment.
5. Resolve any configured `activationIssue` through live GitHub and npm
   evidence. Require the bootstrap package to be public, the trusted publisher
   to be configured, and the bootstrap token and workflow secret to be revoked.
6. Inspect the `release-candidate` and `npm-stage` environment protection rules
   read-only. Require the reviewer gates expected by the release policy.
7. Stop if activation is incomplete or if the package, repository, workflow,
   environment, protection rules, or permitted action differs. Reconcile policy
   through a reviewed repository change before releasing.

Routine releases use no npm write token and no direct `npm publish`. If the
package unexpectedly does not exist publicly, stop: bootstrap publication is
outside this workflow.

### 2. Prepare one version change

1. Query open release issues, release labels, and public versions. Require one
   approved release issue that unambiguously states the intended semver version
   and scope. If none does, stop and ask the user; never infer patch, minor,
   major, or scope from “next” or “próxima”.
2. Verify that the version is newer than the public package version and is not
   already present in the public npm registry.
3. Search the entire repository for the exact current version, excluding
   dependencies and generated build output. Classify every hit and update every
   functional assertion or status string that describes the released version.
4. Create a focused branch and change every version-specific contract revealed
   by that search. At minimum, inspect:
   - package metadata and lockfile
   - certification assertions and status output
   - artifact and release-chain assertions
   - acceptance expectations
   - release notes or issue acceptance criteria
5. Preserve only claims the current certification actually proves. Update
   obsolete bootstrap or activation text rather than carrying it into the next
   release.
6. Keep the release path stage-only. Do not add token authentication, a direct
   publish command, or an automatic public-approval step.

### 3. Verify and merge the release change

1. Read the package scripts and repository instructions, then install according
   to their exact commands.
2. Run focused checks while editing, then run the aggregate repository release
   check named by the live package scripts (`npm run check` currently). Use
   sandboxed `HOME`, npm userconfig, cache, and temporary paths for any command
   that exercises filesystem or npm configuration behavior.
3. Run the candidate certifier locally only when its host and reference-auth
   requirements can be satisfied without writing to the operator's real home.
4. Commit the focused change, open its pull request, wait for required checks
   and review, and merge it.
5. Confirm `main` contains the exact approved version change and is synchronized
   before dispatching the release workflow.

Treat failed checks or review changes as a new candidate. Never publish an
artifact built before the final merged commit.

### 4. Certify and stage the exact artifact

1. Dispatch the live `release-candidate.yml` workflow on `main` with
   `stage=true`. Prefer `gh workflow run` using the canonical repository and the
   input names read from the workflow.
2. Record the workflow run URL and source commit.
3. Tell the user that approving the `release-candidate` environment authorizes
   certification with the protected reference credential, then wait when
   GitHub requests that approval.
4. Inspect the certification job result and its uploaded tarball plus
   `SHA256SUMS`.
5. Tell the user that approving the `npm-stage` environment authorizes OIDC
   submission of that exact certified tarball to npm staging, not public
   release, then wait when GitHub requests that approval.
6. Require both jobs to succeed. Record the stage identifier and certified
   SHA-256 digest from the run evidence.

The main agent owns GitHub and npm mutations. Never ask another agent to
dispatch, approve, reject, or publish a release.

### 5. Inspect the npm-staged candidate

1. Authenticate the maintainer's npm CLI session only if npm requires it. Ask
   the user to complete browser login, password entry, or interactive
   authentication directly. Pause immediately for those prompts; never proxy
   keystrokes or request, print, or store their password, token, recovery code,
   or 2FA code.
2. Use the live npm stage CLI to list and view the staged item for
   `@yargote/matty`.
3. Download the staged tarball into a fresh temporary directory with isolated
   npm config and cache paths.
4. Verify all of the following against the certified workflow evidence:
   - package name and intended version
   - exact SHA-256 digest
   - manifest and public-access metadata
   - packed file list and absence of secrets or development residue
   - install and runtime smoke behavior required by the release contract
   - source commit and workflow identity carried by provenance evidence
5. Present the evidence and any discrepancies to the user.

If any identity, content, digest, behavior, or provenance check differs, stop.
Explain that rejection destroys the staged candidate, obtain explicit user
authorization, then use the live npm stage rejection flow with the user's
interactive 2FA. Run only the non-secret rejection command, then pause for the
user to complete any browser or terminal authentication directly. Prepare a
corrected release through a new merged commit and a new workflow run.

### 6. Obtain final publication approval

1. Resolve the intended dist-tag from the approved release issue and live npm
   stage metadata. If it is absent or ambiguous, stop and ask the user.
2. State the exact package, version, dist-tag, stage identifier, source commit,
   workflow run, and verified SHA-256 digest.
3. Explain that approving the stage makes this version public on npm and cannot
   be undone by republishing the same version.
4. Ask for explicit user authorization for final publication.
5. After authorization, run only the non-secret npm stage approval command,
   then pause immediately for the user to complete browser login, password, or
   2FA directly. Never proxy keystrokes or accept credentials in chat.
6. Confirm npm reports the stage as approved and the version as public.

Never infer final publication approval from an earlier GitHub environment
approval or from a general instruction to prepare a release.

### 7. Verify the public release

Verify from the public registry, using a fresh temporary directory:

1. The package version is public and resolves through the expected dist-tag.
2. The public tarball matches the certified and staged SHA-256 digest.
3. Registry shasum and integrity metadata match the downloaded tarball.
4. npm provenance and attestations identify the canonical repository, workflow,
   source commit, and completed run.
5. A clean install and required smoke checks succeed.

Treat delayed registry propagation as pending, not success. Recheck until the
evidence is available or report the precise outstanding item.

### 8. Record and close the release

Add the release evidence to the release issue:

- package and version
- final source commit and pull request
- workflow run
- stage identifier
- certified, staged, and public artifact digest comparison
- inspection result and user approval confirmation
- registry shasum and integrity
- provenance or attestation identity
- public package URL

Close the release issue only when every acceptance criterion is satisfied,
the approved stage is no longer pending, no other candidate from this release
remains staged, and no release cleanup remains. Finish with the local worktree
clean and synchronized with `origin/main`.

## Completion criteria

A Matty release is complete only when one artifact was certified from the
merged canonical commit, staged through the configured OIDC workflow, inspected
byte-for-byte, explicitly approved by the user with npm 2FA, independently
verified from the public registry with matching provenance, and recorded in the
release issue.
