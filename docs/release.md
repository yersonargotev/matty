# Publish a v0.x Release

This workflow publishes Matty release artifacts to GitHub Releases and updates
`yersonargotev/homebrew-tap` from the same `checksums.txt` manifest. Homebrew
and direct GitHub Release installs distribute the binary only; first-run users
must run `matty init` so the binary can clone the Matty Source of Truth into the
default Installed Source at `~/.local/share/matty`.

## User install path

The [README quickstart](../README.md#quickstart) is the canonical user-facing
Homebrew path. Keep the exact install/init/dry-run/apply command sequence there
so release docs do not drift from the first-run instructions users see first.

Direct GitHub Release users may download the matching `matty_<version>_<goos>_<goarch>`
asset, verify it against `checksums.txt`, put it on `PATH`, then follow the same
first-run sequence from the README quickstart.

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
   - `matty_v0.1.0_darwin_amd64`
   - `matty_v0.1.0_darwin_arm64`
   - `matty_v0.1.0_linux_amd64`
   - `matty_v0.1.0_linux_arm64`
   - `checksums.txt`
6. Verify `yersonargotev/homebrew-tap` has a `Formula/matty.rb` commit for the
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
`Formula/matty.rb` when changed, proves the tap push with `git push --dry-run`,
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
matty_<version>_<goos>_<goarch>
```

It currently emits Darwin and Linux assets for `amd64` and `arm64`, plus a
standard SHA-256 `checksums.txt` manifest. `scripts/generate-homebrew-formula.sh`
requires the same four checksum entries and generates `Formula/matty.rb` with
platform selectors and a `matty --version` brew test.

Matty v0 remains macOS-first. Darwin Homebrew installs are the supported user
path for the first installable release. Linux artifacts are built, checksummed,
and represented in the formula to keep the release contract ready for future
Linux support, but Linux is not part of the v0 golden-path support promise until
a Linux package-install smoke test is defined and accepted.

## Sandboxed package-install smoke expectations

Never validate package-installed Matty against the operator's real `HOME` or
`XDG_CONFIG_HOME`. A release smoke test must point both variables at disposable
temporary directories before running Matty lifecycle commands, for example:

```bash
sandbox="$(mktemp -d)"
export HOME="$sandbox/home"
export XDG_CONFIG_HOME="$sandbox/xdg"
mkdir -p "$HOME" "$XDG_CONFIG_HOME"

matty --version
matty init
matty install --dry-run
matty install
matty doctor
```

For Homebrew-specific verification, install the released formula in a disposable
or explicitly controlled test environment, then run the Matty commands above
with sandboxed `HOME` and `XDG_CONFIG_HOME`. The smoke test should prove that a
package-installed binary can initialize its Installed Source, read
`bundle/skills` from that source, preview installation, and apply the golden-path
setup without touching the maintainer's real home config. If external tools such
as Homebrew or Engram are not intentionally exercised against real accounts,
stub or otherwise control those calls.

## First v0.x checklist

- [ ] The release candidate passed `go test ./...`.
- [ ] The tag is an exact `v0.x.y` tag, such as `v0.1.0`.
- [ ] `HOMEBREW_TAP_TOKEN` is configured with write access to
      `yersonargotev/homebrew-tap`.
- [ ] The `Release` workflow completed from the tag commit.
- [ ] All four platform artifacts and `checksums.txt` are attached to the GitHub
      Release.
- [ ] `checksums.txt` contains one SHA-256 entry for each artifact.
- [ ] `Formula/matty.rb` in `yersonargotev/homebrew-tap` points at the same tag
      and checksums.
- [ ] `brew install yersonargotev/tap/matty` installs the released binary.
- [ ] A sandboxed package install can run `matty init`, `matty install --dry-run`,
      `matty install`, and `matty doctor` without writing to real home config.
- [ ] Release notes call out that v0 is macOS-first and that Linux artifacts are
      published for future support, not the current golden path.
