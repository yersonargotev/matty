# Wayfinder map: Make Matty installable

## Notes

Goal: make Matty installable from GitHub Releases and `yersonargotev/homebrew-tap`, then make the package-installed binary usable with a first-run `matty init` path.

Use the `dots` release system as the local reference, especially:

- `/Users/argote/Documents/dev/yersonargotev/dots/.github/workflows/release.yml`
- `/Users/argote/Documents/dev/yersonargotev/dots/.github/workflows/ci.yml`
- `/Users/argote/Documents/dev/yersonargotev/dots/scripts/build-release-artifacts.sh`
- `/Users/argote/Documents/dev/yersonargotev/dots/scripts/generate-homebrew-formula.sh`
- `/Users/argote/Documents/dev/yersonargotev/dots/internal/release/release_automation_test.go`
- `/Users/argote/Documents/dev/yersonargotev/dots/internal/cli/init.go`
- `/Users/argote/Documents/dev/yersonargotev/dots/internal/bootstrap/bootstrap.go`
- `/Users/argote/Documents/dev/yersonargotev/dots/docs/release.md`

Current Matty gotcha: `ResolvePaths` defaults `SkillSourceRoot` by walking upward from the current working directory to find `bundle/skills`. That works in repo/dev checkouts, but a Homebrew-installed binary run from arbitrary directories will not have a reliable `bundle/skills` beside the process. The first architectural question is whether package-installed Matty should clone a Source of Truth via `matty init`, embed the bundle into the binary, or install a separate bundle resource.

Standing constraints:

- Keep Matty-owned runtime behavior in Matty-owned folders/packages.
- `./skills`, `./engram`, and `./gentle-ai` remain external reference projects only.
- Tests and manual checks must sandbox `HOME`, `XDG_CONFIG_HOME`, and any default installed source path.
- Prefer small deep modules: release, bootstrap/init, and source-resolution behavior should not accumulate inside `internal/cli`.
- `go test ./...` remains required before reporting implementation success.

## Decisions so far

- v0 lifecycle exists — Matty already supports `install`, `doctor`, `update`, and `uninstall`, and sandbox smoke testing has passed.
- Dots reference pattern — build raw cross-platform binaries, publish `checksums.txt`, generate a Homebrew formula from that manifest, prepare/dry-run tap update before GitHub Release mutation, then push the tap after release assets exist.
- Package-installed source model — use `matty init` to clone the Matty Source of Truth into `~/.local/share/matty`, then resolve the default skill bundle from `~/.local/share/matty/bundle/skills`; `MATTY_SKILLS_SOURCE` stays a direct dev/test override. See `docs/adr/0002-package-installed-source-model.md`.
- Release-injectable version package — `internal/version.Value` defaults to `dev` and can be overridden with `go build -ldflags "-X github.com/yersonargotev/matty/internal/version.Value=v0.x.y"`; `internal/cli` consumes that value for `matty --version` and state metadata. See [02](issues/02-add-version-package.md).
- `matty init` implemented — package-installed users can clone/update the Matty Source of Truth into `~/.local/share/matty`; source resolution now falls back to `~/.local/share/matty/bundle/skills` outside repo checkouts while preserving `MATTY_SKILLS_SOURCE` and local repo discovery. See [03](issues/03-implement-matty-init.md).
- Package-installed command resolution hardened — outside repo checkouts, `matty init` plus `install`/`doctor`/`update`/`uninstall` now uses `~/.local/share/matty/bundle/skills`; missing default Installed Source errors name the missing path and suggest `matty init`. See [04](issues/04-resolve-skill-source-for-package-installs.md).
- Release artifact builder implemented — `scripts/build-release-artifacts.sh` accepts exact `v0.x.y` tags, builds raw `matty_<version>_<goos>_<goarch>` binaries for supported Darwin/Linux architectures, injects `internal/version.Value` via ldflags, and writes `checksums.txt`. See [05](issues/05-add-release-artifact-builder.md).
- Homebrew formula generator implemented — `scripts/generate-homebrew-formula.sh` emits the Matty formula from `checksums.txt`, validates supported release artifacts and checksums, installs the raw binary as `matty`, and includes a `matty --version` brew test. See [06](issues/06-add-homebrew-formula-generator.md).
- CI workflow implemented — `.github/workflows/ci.yml` runs on pull requests and pushes to `main`, uses Go from `go.mod`, verifies Matty-owned Go formatting, runs `go vet ./...`, builds, and runs `go test ./...`. See [07](issues/07-add-ci-workflow.md).
- Release workflow implemented — `.github/workflows/release.yml` publishes `v0.*` tags or manually selected existing tags by building `dist/*`, proving token-backed Homebrew tap update access, creating the GitHub Release with generated notes if needed, uploading assets, then pushing the prepared tap commit. See [08](issues/08-add-release-workflow-and-tap-update.md).
- Installable release docs added — `docs/release.md` documents maintainer publishing, `HOMEBREW_TAP_TOKEN`, first-release checklist, sandboxed package-install smoke expectations, and Linux artifact support boundaries; README quickstart documents the user Homebrew path through `matty init` and `matty install`. See [09](issues/09-document-installable-release.md).
- Package-install smoke implemented — `internal/release/package_install_smoke_test.go` builds a local release-like binary, runs it outside the repo with temp `HOME`/`XDG_CONFIG_HOME`, initializes a local Source fixture, verifies install/update/uninstall dry-run and apply paths plus doctor, and stubs `brew`/`engram` calls. See [10](issues/10-package-install-smoke-test.md).
- Update vs upgrade semantics decided — `matty update` remains a managed workflow/Engram refresh and never upgrades the Matty binary or mutates the Installed Source; package users upgrade via Homebrew or GitHub Release artifact replacement, then rerun `matty init` to align the Installed Source. No `matty upgrade` command is needed for v0. See [11](issues/11-decide-update-vs-upgrade-semantics.md).

## Frontier

Tickets 01, 02, 03, 04, 05, 06, 07, 08, 09, 10, and 11 are resolved. The next frontier is the lowest-numbered remaining open, unblocked installable-roadmap ticket.

## Fog

- Whether generated GitHub release notes are enough, or whether Matty needs a changelog/release-note convention before first public tag.
- Whether the Homebrew formula should live only in `yersonargotev/homebrew-tap` or whether this repo also keeps a generated snapshot for review.
