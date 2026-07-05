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

## Frontier

Tickets 01, 02, 03, and 04 are resolved. The next frontier is the lowest-numbered remaining open, unblocked installable-roadmap ticket.

## Fog

- Whether Matty should support Linux release artifacts immediately or stay macOS-only for package install while keeping scripts structurally ready for Linux.
- Whether `matty update` should keep meaning “refresh managed workflow” only, or whether a package-installed Matty needs a separate `matty upgrade` for the binary/source bundle.
- Whether generated GitHub release notes are enough, or whether Matty needs a changelog/release-note convention before first public tag.
- Whether the Homebrew formula should live only in `yersonargotev/homebrew-tap` or whether this repo also keeps a generated snapshot for review.
