# Publish a v0.x Release

This workflow publishes Packy release artifacts to GitHub Releases and updates
`yersonargotev/homebrew-tap` from the same `checksums.txt` manifest. Homebrew
and direct GitHub Release installs distribute the binary only; first-run users
must run `packy init` so the binary can clone the Packy Source of Truth into the
default Installed Source at `~/.local/share/packy`.

## User install path

The [README quickstart](../README.md#quickstart) is the canonical user-facing
Homebrew path. Keep the exact install/init/dry-run/apply command sequence there
so release docs do not drift from the first-run instructions users see first.

Direct GitHub Release users may download the matching `packy_<version>_<goos>_<goarch>`
asset, verify it against `checksums.txt`, put it on `PATH`, then follow the same
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

1. Confirm the release candidate passes validation:
   ```bash
   go test ./...
   ```
2. Confirm the repository has a `HOMEBREW_TAP_TOKEN` secret with write access to
   `yersonargotev/homebrew-tap`.
3. Create and push an exact v0 tag:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```
4. Watch the `Release` workflow for that tag.
5. Open the GitHub Release and verify these assets exist:
   - `packy_v0.1.0_darwin_amd64`
   - `packy_v0.1.0_darwin_arm64`
   - `packy_v0.1.0_linux_amd64`
   - `packy_v0.1.0_linux_arm64`
   - `checksums.txt`
6. Verify `yersonargotev/homebrew-tap` has a `Formula/packy.rb` commit for the
   same tag and checksums.
7. Run a sandboxed package-install smoke test before announcing the release.

## Manual dispatch

Use manual dispatch when the tag already exists but release assets or the tap
update need to be rebuilt.

1. Go to **Actions → Release → Run workflow**.
2. Enter an existing exact tag such as `v0.1.0`.
3. Run the workflow.

The workflow checks out that tag, builds artifacts and `checksums.txt`, requires
`HOMEBREW_TAP_TOKEN`, checks out the tap, regenerates and locally commits
`Formula/packy.rb` when changed, proves the tap push with `git push --dry-run`,
creates the GitHub Release if needed, uploads `dist/* --clobber`, and only then
pushes the prepared tap commit.

## `HOMEBREW_TAP_TOKEN` setup

The release workflow cannot use this repository's `GITHUB_TOKEN` to push to the
separate tap repository. Maintainers must create a token that can write to
`yersonargotev/homebrew-tap` and store it as this repository secret:
`HOMEBREW_TAP_TOKEN`.

The token should have the narrowest practical scope that allows checkout and
push access to `yersonargotev/homebrew-tap`. Configure it under this repository's
**Settings → Secrets and variables → Actions → Repository secrets**. The workflow
fails before creating or uploading release assets when the secret is missing, so
GitHub Releases and the Homebrew tap do not drift.

## Release artifact contract

`scripts/build-release-artifacts.sh` accepts exact `v0.x.y` tags and builds raw
binaries named:

```text
packy_<version>_<goos>_<goarch>
```

It currently emits Darwin and Linux assets for `amd64` and `arm64`, plus a
standard SHA-256 `checksums.txt` manifest. `scripts/generate-homebrew-formula.sh`
requires the same four checksum entries and generates `Formula/packy.rb` with
platform selectors and a `packy --version` brew test.

Packy v0 remains macOS-first. Darwin Homebrew installs are the supported user
path for the first installable release. Linux artifacts are built, checksummed,
and represented in the formula to keep the release contract ready for future
Linux support, but Linux is not part of the v0 golden-path support promise until
a Linux package-install smoke test is defined and accepted.

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

- [ ] The release candidate passed `go test ./...`.
- [ ] The tag is an exact `v0.x.y` tag, such as `v0.1.0`.
- [ ] `HOMEBREW_TAP_TOKEN` is configured with write access to
      `yersonargotev/homebrew-tap`.
- [ ] The `Release` workflow completed from the tag commit.
- [ ] Exact Claude `2.1.203` and recorded-current-stable evidence is green for
      both Darwin `amd64` and `arm64` before publication.
- [ ] All four platform artifacts and `checksums.txt` are attached to the GitHub
      Release.
- [ ] `checksums.txt` contains one SHA-256 entry for each artifact.
- [ ] `Formula/packy.rb` in `yersonargotev/homebrew-tap` points at the same tag
      and checksums.
- [ ] When explicitly requested, a real `brew install yersonargotev/tap/packy`
      in a controlled environment installs the released binary.
- [ ] Durable smoke evidence binds the tag/SHA, platform, Claude version and
      digest, commands/exits, and before/after sandbox manifests without secrets.
- [ ] A sandboxed package install can run `packy init`, `packy install --dry-run`,
      `packy install`, `packy doctor`, `packy update --dry-run`, `packy update`,
      `packy uninstall --dry-run`, `packy uninstall`, and final `packy doctor`
      without writing to real home config.
- [ ] Release notes call out that v0 is macOS-first and that Linux artifacts are
      published for future support, not the current golden path.
