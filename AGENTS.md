# AGENTS.md

## Project-specific architecture rule

`./skills`, `./engram`, and `./gentle-ai` are external reference projects only. Do not use them as Matty runtime/source dependencies, default install targets, or production source roots.

Matty-owned behavior must live in Matty-owned folders/packages. For the v0 skill bundle, use `bundle/skills` as the default source tree and keep bundle discovery behind `internal/skillbundle`; `internal/cli` should only adapt that behavior to commands/state.

`MATTY_SKILLS_SOURCE` is allowed as a test/dev seam, but production defaults must not point at external clones.

## Development guardrails

- Keep changes scoped to the requested issue or follow-up.
- Tests and manual checks must sandbox `HOME`/`XDG_CONFIG_HOME`; never validate by writing to the operator's real home config.
- Prefer small, deep modules with narrow interfaces over CLI packages that know every detail.
- Run `go test ./...` before reporting success or committing.

## Delegation approval

This repo pre-approves safe Codex subagent delegation for non-trivial implementation, exploration, test/log triage, and review work. Use the dots delegation policy from the session instructions: keep requirements, decisions, integration, and final verification with the main agent; assign bounded non-overlapping slices; and report delegated work, accepted/rejected findings, and main-agent verification.
