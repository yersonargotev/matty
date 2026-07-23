# Publish a v0.x Release

This workflow publishes Packy release artifacts to GitHub Releases and updates
`yersonargotev/homebrew-tap` from the same `SHA256SUMS` manifest. Homebrew
and direct GitHub Release installs distribute the binary only; first-run users
must run `packy init` so the binary can clone the Packy Source of Truth into the
default Installed Source at `~/.local/share/packy`.

## User install path

The [README quickstart](../README.md#quickstart) is the canonical user-facing
Homebrew path. Keep the exact install/init/dry-run/apply command sequence there
so release docs do not drift from the first-run instructions users see first.

Direct GitHub Release users may download the matching `packy_<version>_<goos>_<goarch>`
asset, verify it against `SHA256SUMS`, put it on `PATH`, then follow the same
first-run sequence from the README quickstart.

## User upgrade path

`packy update` is not a binary upgrade command. It refreshes Packy-managed
workflow artifacts and Engram setup from the currently resolved skill bundle.
Homebrew users upgrade Packy itself with:

```bash
brew upgrade packy
packy init
packy update --dry-run
packy update
```

Direct GitHub Release users replace the `packy` binary with the newer release
artifact, then run the same `packy init` and update dry-run/apply sequence.
`packy init` is the command that aligns the Installed Source checkout to the
running release. `packy update --dry-run` must not mutate that checkout; if the
Installed Source is missing or stale, run `packy init` first.

## Maintainer quick path

Publication is manual-only. The workflow must be dispatched from protected
`main`, and the selected exact `v0.x.y` tag must already resolve to the checked
`origin/main` commit. The workflow never creates, moves, or pushes a tag.

1. Confirm the protected-main candidate passes validation:
   ```bash
   ./scripts/validate-packy.sh
   ```
2. Review `docs/release-notes/next.md` for this exact candidate. It must contain
   exactly one `{{TAG}}` placeholder.
3. Confirm the protected `homebrew` environment's `HOMEBREW_TAP_TOKEN` has
   write access only to `yersonargotev/homebrew-tap`.
4. Create and push the exact tag through the repository's authorized process.
5. In **Actions → Release → Run workflow**, select `main`, enter the tag, and
   leave **dry_run** enabled.
6. Review the dry-run summary. It performs the build, smoke/evidence gates,
   immutable candidate sealing, and read-only release inspection, then reports
   the exact state-dependent OIDC, missing-asset, draft/publication, and tap
   effects without performing any of them.
7. Dispatch the same tag from the same protected-main commit with **dry_run**
   disabled.
8. Verify the published release has exactly these seven assets:
   - four `packy_<tag>_<goos>_<goarch>` binaries;
   - `SHA256SUMS`;
   - `sbom.spdx.json`; and
   - `attestation.bundle.jsonl`.
9. Verify `yersonargotev/homebrew-tap` has a `Formula/packy.rb` commit for the
   same immutable tag and binary hashes.
10. Run a sandboxed package-install smoke test before announcing the release.

## Manual dispatch and recovery

A dispatch is accepted only when the workflow checkout, freshly fetched
`origin/main`, and the selected tag all resolve to one commit. All platform
binaries are built once. Validation, Claude smoke, draft preparation,
publication, and Homebrew consume those retained bytes without rebuilding.

The default dry-run completes every safe, non-mutating check and read-only
inspection available before stopping. If the version already has an attestation
bundle, dry-run downloads and verifies it against the rebuilt candidate without
requesting a new token. It does not request an OIDC token or
create/change a tag, attestation, draft, release, asset, or tap commit.

A real run creates a draft only when the version is absent. If an exact draft
already exists, recovery revalidates its hidden candidate metadata, target
commit, notes, provenance, and every server-reported asset digest before
uploading only missing assets. Divergent or ambiguous state fails closed. An
already-published exact release is read and verified, never edited or recreated;
that recovery path may continue to the independently verified Homebrew stage.

## `HOMEBREW_TAP_TOKEN` setup

The release workflow cannot use this repository's `GITHUB_TOKEN` to push to the
separate tap repository. Maintainers must create a token that can write to
`yersonargotev/homebrew-tap` and store it as the `HOMEBREW_TAP_TOKEN`
environment secret in the protected `homebrew` environment.

The token should have the narrowest practical scope that allows checkout and
push access to `yersonargotev/homebrew-tap`. Configure it under this
repository's **Settings → Environments → homebrew → Environment secrets**.
Do not create a repository-level fallback secret. The token is not exposed
until the exact GitHub Release is published, independently read back, and the
Owner approves the `homebrew` deployment. If it is missing, only the tap stage
fails; rerunning the same version revalidates the immutable published release
before retrying Homebrew.

## Release artifact contract

`scripts/build-release-artifacts.sh` accepts exact `v0.x.y` tags and emits one
closed six-file candidate directory:

```text
packy_<version>_darwin_amd64
packy_<version>_darwin_arm64
packy_<version>_linux_amd64
packy_<version>_linux_arm64
SHA256SUMS
sbom.spdx.json
```

`SHA256SUMS` contains exactly the four binary digests plus the SBOM digest. The
SPDX 2.3 JSON document is deterministic for the version, commit timestamp, and
binary bytes. Verification rejects any missing, extra, duplicate, stale,
non-regular, symlinked, or mismatched file. The publication stage adds the
separately verified `attestation.bundle.jsonl` as the seventh release asset.

The sealed candidate binds the version, protected-main commit/ref, repository,
release workflow path and content digest, reviewed notes digest, exact subjects,
and the reviewed union of effective GitHub permissions. Its deterministic
provenance document and hidden draft-body metadata must round-trip exactly from
GitHub before publication. After OIDC issuance, the final release-set identity
also binds the exact attestation bundle digest and complete destination plan;
that envelope is the body verified for draft recovery and publication. The
bundle bytes are encoded in the hidden envelope so an interrupted draft can
recover the identical bundle even when failure happened before asset upload.
The destination plan identifies the tap repository/path and the exact generated
formula SHA-256, so the Homebrew mutation is part of the same release identity.

`scripts/generate-homebrew-formula.sh` accepts the complete manifest but derives
URLs and hashes only for the four binaries. Packy v0 remains macOS-first. Darwin
Homebrew installs are the supported user path; Linux artifacts remain published
for future support rather than the current golden path.

## Real-Claude package smoke gates

Packy owns two package-installed, credential-free real-Claude gates:

| Gate | Claude selection | Trigger | Effect |
| --- | --- | --- | --- |
| Exact floor | `2.1.203` | Every pull request and release | A failure blocks that pull request or release. |
| Current stable | npm's recorded stable version | Daily canary and every release | A canary failure opens compatibility work without attaching to unrelated pull requests; a release failure blocks publication. |

Release validation runs both selectors against the corresponding Darwin artifact
on Intel (`amd64`) and Apple Silicon (`arm64`) before the publication job can
create a GitHub Release, upload assets, or push the tap update. The release
workflow resolves the tag to one immutable commit, validates that commit once,
builds and checksums one candidate set, passes those same Darwin binaries and
commit SHA through smoke, and publishes that same proved artifact set without
rebuilding it. Publication stops if the tag no longer resolves to the proved
commit.

Run either contract locally from a clean checkout with:

```bash
./scripts/run-claude-smoke.sh \
  --claude-version 2.1.203 \
  --packy-ref "$(git rev-parse HEAD)" \
  --evidence-dir "$PWD/.scratch/claude-smoke-evidence"
```

Use `--claude-version stable` for the moving-stable variant. The runner acquires
Claude before restricting execution, installs a release-like Packy binary away
from the checkout, and then exposes only disposable `HOME`, `XDG_CONFIG_HOME`,
`CLAUDE_CONFIG_DIR`, cache, data, temporary, Homebrew, Installed Source, and work
roots. Its environment allowlist omits credentials and provider variables.
Homebrew and Engram are deterministic inert stubs; Claude is real.

The Claude interposer permits only version inspection and bounded user-scoped
MCP list/get/add/remove operations. It rejects login, authentication, REPL,
print/model mode, project/local MCP mutation, and malformed commands before the
real executable can observe them. The package-installed Packy sequence is:

```text
packy version
packy init --repository-url <local-checkout> --repository-ref <proved-ref>
packy install --dry-run
packy install
packy doctor
packy update --dry-run
packy update
packy uninstall --dry-run
packy uninstall
packy doctor
```

Every run retains canonical JSON evidence. It binds the Packy version, ref and
commit; OS and architecture; requested and resolved Claude version; npm
integrity and executable digest; each Packy command and normalized nested Claude
operation with its exit; deterministic before/after sandbox manifests; and
explicit assertions for disposable roots, allowlisted environment, credential
scrubbing, command confinement, unchanged source checkout, and no interactive
Claude invocation. Missing, malformed, failed, or unsafe evidence fails closed.

## Fail-closed publication gate

Build, evidence validation, attestation, GitHub publication, and Homebrew are
separate jobs with separate authority:

- build, validation, inspection, and dry-run use read-only repository access;
- attestation and GitHub publication wait on Owner approval in the protected
  `release` environment;
- Homebrew publication waits separately on Owner approval in the protected
  `homebrew` environment;
- only the attestation job receives `id-token: write` and
  `attestations: write`;
- only GitHub draft/publication receives `contents: write`; and
- only the Homebrew job receives `HOMEBREW_TAP_TOKEN`, after exact published
  GitHub bytes have been read back and verified.

The read-only build, validation, inspection, and dry-run jobs do not reference
either publication environment. Environment approval therefore gates only the
destination authority that the approved job is about to exercise.

The OIDC bundle is verified against the exact repository, fully-qualified signer
workflow, protected-main source ref, source commit, signer commit, and every
retained candidate file, including `SHA256SUMS` itself. Verification uses the retained bundle and an
explicit trusted-root document rather than fetching an arbitrary attestation.

Before the draft becomes public, the workflow reads back the complete draft,
requires the exact body metadata and seven-asset inventory, compares every
server-reported SHA-256 digest with the retained bytes, and asks the domain
verifier for the one-time `publish-draft` decision. There is no clobber, delete,
recreate, replacement, tag movement, or published-version mutation path.
The tag and protected-main refs are peeled through the Git object API and
rechecked against the retained commit immediately before draft creation, every
asset upload, OIDC issuance, publication, and the final tap push. The tap stage
also reads remote `main` and `Formula/packy.rb` back after pushing and compares
the remote commit and formula digest with the sealed destination plan.

After publication, the Homebrew job independently reads the release again,
checks its version, commit, body, exact inventory, server digests, and
`SHA256SUMS`, and only then checks out and pushes the tap. Missing, failed,
stale, duplicated, partial, or ambiguous evidence has no waiver path.

The automated local-release smoke test is:

```bash
go test ./internal/release -run TestPackageInstallSmokeLifecycleWithLocalReleaseBinary -count=1
```

That test builds a temporary release-like `./cmd/packy` binary with an injected
version, runs it from a temporary directory outside the repo checkout, clones a
local Packy Source fixture, and places stubbed `brew` and `engram` executables
ahead of the real `PATH` to verify the expected external calls without reaching
real accounts. Its exact Packy command sequence is:

```bash
packy init --repository-url <local-fixture-repo>
packy install --dry-run
packy install
packy doctor
packy update --dry-run
packy update
packy uninstall --dry-run
packy uninstall
packy doctor
```

## First v0.x checklist

- [ ] The candidate passed `./scripts/validate-packy.sh` on protected `main`.
- [ ] The exact `v0.x.y` tag, workflow checkout, and freshly fetched
      `origin/main` resolve to one commit.
- [ ] The protected `homebrew` environment's `HOMEBREW_TAP_TOKEN` is configured
      only for the dedicated tap, with no repository-level fallback secret.
- [ ] A default dry-run completed and reported the planned external mutations
      without requesting OIDC or changing tag, release, attestation, or tap state.
- [ ] Exact Claude `2.1.203` and recorded-current-stable evidence is green for
      Darwin `amd64` and `arm64`.
- [ ] `SHA256SUMS` binds exactly four binaries and `sbom.spdx.json`.
- [ ] OIDC provenance verifies the exact protected-main commit, signer workflow,
      signer digest, and all six retained candidate files.
- [ ] The draft read-back exactly matches its candidate identity, notes,
      provenance, target commit, seven assets, and server hashes before publish.
- [ ] The published release contains exactly four binaries, `SHA256SUMS`,
      `sbom.spdx.json`, and `attestation.bundle.jsonl`.
- [ ] Homebrew began only after an independent exact published-release read-back.
- [ ] `Formula/packy.rb` points at the same immutable tag and binary hashes.
- [ ] A sandboxed package install completes the documented lifecycle without
      writing to real home configuration.
- [ ] Release notes retain the macOS-first support statement and Linux limitation.
