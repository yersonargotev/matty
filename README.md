# Packy

Packy is a lightweight macOS-first installer/configurator for a global AI coding workflow. It installs a curated Matt Pocock skill bundle, wires Engram through its official setup commands, and adds small Packy-owned prompt layers for Codex and OpenCode.

Packy is not a runtime orchestrator and does not copy workflow files into every project.

## Quickstart

Install Packy from the Homebrew tap, initialize the package-installed source checkout, preview the setup, then apply it:

```sh
brew install yersonargotev/tap/packy
packy init
packy install --dry-run
packy install
```

`packy init` is required for Homebrew/GitHub Release installs because package managers install the binary only; Packy reads its default skill bundle from the initialized source at `~/.local/share/packy/bundle/skills`. To upgrade Packy itself later, use `brew upgrade packy` (or replace the GitHub Release binary), then rerun `packy init` before `packy update --dry-run`. Maintainer release docs live in [docs/release.md](docs/release.md).

## v0 scope

Packy v0 manages:

- global skill symlinks under `~/.agents/skills`
- small Packy state at `~/.packy/config.json`
- Codex prompt markers in `~/.codex/AGENTS.md`
- an OpenCode prompt file and reference under `$XDG_CONFIG_HOME/opencode`
- Engram install/update/setup by delegating to the Homebrew-managed Engram binary (`<brew-prefix>/bin/engram setup ...`)

Packy v0 is macOS-first. Linux and other agent adapters may be added later, but they are outside v0.

## Commands

```sh
packy init             # initialize the package-installed source checkout
packy install          # apply the golden-path setup
packy install --dry-run
packy doctor           # read-only setup health checks
packy update           # refresh Engram, skill links, prompts, and state; does not upgrade the binary
packy update --dry-run
packy uninstall        # remove only Packy-managed artifacts
packy uninstall --dry-run
```

## Opt-in capability packs

Packy core remains available even when the optional `matty` capability pack is
inactive. The initial Packy-owned catalog contains only `matty` and `engram`, and
supports only the `codex` and `opencode` surfaces. Existing installations are not
automatically migrated or adopted.

Before opting in, inspect the catalog and current host state without mutation:

```sh
packy pack list
packy pack show matty
packy pack status
packy pack status matty --surface codex
```

Then follow the explicit [manual capability-pack transition](docs/capability-packs.md).
It documents dry-run, typed approvals, readiness gating, update, reconcile,
recovery, and contributor-safe deactivation for both supported surfaces.

## Global paths

| Path | Purpose |
| --- | --- |
| `~/.agents/skills` | Packy-managed skill symlinks |
| `~/.packy/config.json` | Packy ownership/state metadata |
| `~/.codex/AGENTS.md` | Codex prompt file containing Packy markers |
| `$XDG_CONFIG_HOME/opencode/opencode.json` | OpenCode config containing the Packy prompt reference |
| `$XDG_CONFIG_HOME/opencode/packy.md` | Packy-owned OpenCode prompt |

If `XDG_CONFIG_HOME` is unset or relative, Packy uses `~/.config`.

## Safety model

- `doctor` is read-only and reports which Engram binary is on `PATH`, whether it is Homebrew-managed, any `engram serve` daemon executable it can see, and whether a `~/.local/bin/engram` compatibility entry is a symlink to Homebrew.
- `--dry-run` reports planned actions without writing files or running external commands.
- Packy-owned prompt content is wrapped in `packy:*` markers and only those blocks are updated or removed.
- `uninstall` removes Packy-managed symlinks, Packy prompt blocks/references, the Packy OpenCode prompt, and Packy state.
- Packy warns about `gentle-ai:*` content but does not delete or rewrite Gentle AI-managed content.
- Tests use sandboxed `HOME` and `XDG_CONFIG_HOME`; they must not write to the operator's real home config.

## Out of scope for v0

- TUI installer or model picker
- runtime profile manager
- SDD workflow installation or SDD orchestrators
- repo-local docs/config by default
- Claude Code, Antigravity, GitHub Copilot CLI, Gemini, Cursor, or other adapters
- automatic Gentle AI cleanup or migration
- vendoring the Engram binary or installing a second copy under `~/.local/bin`; Homebrew owns Engram, and Packy only delegates setup/configuration
- installing only a tiny skill subset

## Verification

The repository validation authority uses an explicit allowlist of Packy-owned
Go packages and paths, so vendored or temporary upstream content is never
discovered or executed:

```sh
./scripts/validate-packy.sh
```

For a faster, non-authoritative local check, validate only the impact of the
complete base-to-working-tree change set:

```sh
./scripts/validate-changed.sh             # base defaults to origin/main
./scripts/validate-changed.sh <base>      # optional branch, tag, or commit
```

The command reports `mode=focused` when it can safely format changed Packy Go
files and test their owning packages and reverse dependents (or skip tests for
documentation-only or empty changes). It reports `mode=exhaustive` and
delegates to `./scripts/validate-packy.sh` whenever the base or impact cannot be
established safely, or a cross-cutting/unknown path changed. The focused command
is only a local feedback aid: `./scripts/validate-packy.sh` remains required
before final delivery and is the command used by CI.

Until vendored upstream Go content exists, `go test ./...` also remains a
supported compatibility check.
