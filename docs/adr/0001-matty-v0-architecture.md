# ADR 0001: Matty v0 is a global installer/configurator

## Status

Accepted for v0.

## Decision

Matty v0 is a macOS-first Go+Cobra CLI that configures a global AI coding
workflow for Codex and OpenCode. It installs/syncs global skills, delegates
Engram setup to Engram, injects only small Matty-owned prompt/config blocks,
and stores minimal state for update/doctor/uninstall.

## Context

The target workflow combines Matt Pocock-style skills, Engram memory, and
explicit delegation conventions. Gentle AI has useful ideas, but its always-on
prompt and broader SDD-first shape are heavier than desired for Matty v0.

Matty previously carried external reference clones (`./skills`, `./engram`, and
`./gentle-ai`), but they were removed because they are not runtime material.
Matty runtime behavior stays in Matty-owned folders/packages.

## Decisions

| Topic | Decision |
| --- | --- |
| CLI framework | Use Go with Cobra for a small, testable command surface. |
| Runtime model | Do not run as an orchestrator; only install, verify, update, and uninstall configuration. |
| Config model | Prefer global user config surfaces, not repo-local copied files. |
| Skill install | Materialize the chosen skill bundle as symlinks under `~/.agents/skills`. |
| Skill source | Default source is Matty-owned `bundle/skills`; `MATTY_SKILLS_SOURCE` is only a test/dev seam. |
| Skill routing | Reuse `ask-matt` instead of creating a Matty-specific router in v0. |
| Engram | Install/update through official Homebrew path on macOS and delegate CLI wiring to `engram setup codex` and `engram setup opencode`. |
| Codex/OpenCode prompts | Manage only Matty-owned markers/entries and keep the always-on prompt small. |
| State | Store only small metadata in `~/.matty/config.json`; do not store large prompt content. |
| Coexistence | Warn about Gentle AI conflict signals, but never delete or rewrite Gentle AI content by default. |
| Safety | Tests and manual checks must sandbox `HOME`/`XDG_CONFIG_HOME`. |

## Consequences

- Matty is easy to reason about: every command plans or applies configuration changes rather than owning session runtime behavior.
- Updates and uninstall can be safe because Matty records and touches only Matty-managed artifacts.
- Token cost stays low because Matty injects pointers to global skills and routing instead of embedding large workflow instructions.
- Surface-specific behavior belongs behind narrow internal packages instead of accumulating inside the CLI command layer.
- Future adapters remain possible, but v0 avoids designing for every host CLI before Codex/OpenCode prove the model.

## Non-goals

- Managing or migrating Gentle AI installations automatically.
- Replacing Engram setup internals.
- Supporting all AI coding CLIs in v0.
- Reintroducing external project trees as runtime dependencies or production source roots.
