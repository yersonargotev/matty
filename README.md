# Matty

Matty is a lightweight macOS-first installer/configurator for a global AI coding workflow. It installs a curated Matt Pocock skill bundle, wires Engram through its official setup commands, and adds small Matty-owned prompt layers for Codex and OpenCode.

Matty is not a runtime orchestrator and does not copy workflow files into every project.

## Quickstart

Install Matty from the Homebrew tap, initialize the package-installed source checkout, preview the setup, then apply it:

```sh
brew install yersonargotev/tap/matty
matty init
matty install --dry-run
matty install
```

`matty init` is required for Homebrew/GitHub Release installs because package managers install the binary only; Matty reads its default skill bundle from the initialized source at `~/.local/share/matty/bundle/skills`. To upgrade Matty itself later, use `brew upgrade matty` (or replace the GitHub Release binary), then rerun `matty init` before `matty update --dry-run`. Maintainer release docs live in [docs/release.md](docs/release.md).

## v0 scope

Matty v0 manages:

- global skill symlinks under `~/.agents/skills`
- small Matty state at `~/.matty/config.json`
- Codex prompt markers in `~/.codex/AGENTS.md`
- an OpenCode prompt file and reference under `$XDG_CONFIG_HOME/opencode`
- Engram install/update/setup by delegating to Homebrew and `engram setup`

Matty v0 is macOS-first. Linux and other agent adapters may be added later, but they are outside v0.

## Commands

```sh
matty init             # initialize the package-installed source checkout
matty install          # apply the golden-path setup
matty install --dry-run
matty doctor           # read-only setup health checks
matty update           # refresh Engram, skill links, prompts, and state; does not upgrade the binary
matty update --dry-run
matty uninstall        # remove only Matty-managed artifacts
matty uninstall --dry-run
```

## Global paths

| Path | Purpose |
| --- | --- |
| `~/.agents/skills` | Matty-managed skill symlinks |
| `~/.matty/config.json` | Matty ownership/state metadata |
| `~/.codex/AGENTS.md` | Codex prompt file containing Matty markers |
| `$XDG_CONFIG_HOME/opencode/opencode.json` | OpenCode config containing the Matty prompt reference |
| `$XDG_CONFIG_HOME/opencode/matty.md` | Matty-owned OpenCode prompt |

If `XDG_CONFIG_HOME` is unset or relative, Matty uses `~/.config`.

## Safety model

- `doctor` is read-only.
- `--dry-run` reports planned actions without writing files or running external commands.
- Matty-owned prompt content is wrapped in `matty:*` markers and only those blocks are updated or removed.
- `uninstall` removes Matty-managed symlinks, Matty prompt blocks/references, the Matty OpenCode prompt, and Matty state.
- Matty warns about `gentle-ai:*` content but does not delete or rewrite Gentle AI-managed content.
- Tests use sandboxed `HOME` and `XDG_CONFIG_HOME`; they must not write to the operator's real home config.

## Out of scope for v0

- TUI installer or model picker
- runtime profile manager
- SDD workflow installation or SDD orchestrators
- repo-local docs/config by default
- Claude Code, Antigravity, GitHub Copilot CLI, Gemini, Cursor, or other adapters
- automatic Gentle AI cleanup or migration
- vendoring the Engram binary
- installing only a tiny skill subset

## Verification

The final v0 verification command is:

```sh
go test ./...
```
