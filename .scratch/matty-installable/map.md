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

## Frontier

The first unblocked ticket is [01 — Decide package-installed source model](issues/01-decide-package-installed-source-model.md). Resolve it before implementing `matty init` or release plumbing because it determines what the Homebrew formula installs and what `install/update/doctor` read by default.

## Fog

- Whether Matty should support Linux release artifacts immediately or stay macOS-only for package install while keeping scripts structurally ready for Linux.
- Whether `matty update` should keep meaning “refresh managed workflow” only, or whether a package-installed Matty needs a separate `matty upgrade` for the binary/source bundle.
- Whether generated GitHub release notes are enough, or whether Matty needs a changelog/release-note convention before first public tag.
- Whether the Homebrew formula should live only in `yersonargotev/homebrew-tap` or whether this repo also keeps a generated snapshot for review.
