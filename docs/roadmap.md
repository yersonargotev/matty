# Matty roadmap

Matty v0 is an installable installer/configurator for Codex and OpenCode. It
ships through GitHub Releases and Homebrew, supports package-installed use
through `matty init`, and manages opt-in `matty` and `engram` capability packs.

## Next checkpoint

Prepare the accumulated post-v0.1.5 behavior and architecture hardening for the
next verified v0.x release. The release gate and package-install smoke contract
are defined in [the release guide](release.md); the user install and upgrade
paths remain canonical in the [README](../README.md).

No unresolved implementation frontier remains in the current tracker indexes.
The completed work includes:

- GitHub Release and Homebrew distribution with a version-aligned Installed
  Source;
- the opt-in capability-pack lifecycle for Codex and OpenCode;
- structured doctor and pack-status output;
- deep internal ownership for core lifecycle, setup health, host surfaces, and
  workstation layout.

## Near-term follow-ups

| Topic | Question to answer |
| --- | --- |
| Token budget | What measurement proves Matty is materially lighter than Gentle AI at session start? |
| Review workflow | Is Matt Pocock `review`/`code-review` sufficient, or does Matty need a distinct review layer later? |
| Engram ambiguity | What user-facing guidance is needed when Engram project detection is ambiguous? |
| Next adapter | What evidence should be required before adding another host beyond Codex and OpenCode? |

## Future adapters

These remain outside v0. Adding one requires an explicit product decision and
evidence for its host-specific paths, projections, trust model, and readiness:

- Claude Code.
- Antigravity.
- GitHub Copilot CLI.
- Gemini, Cursor, or other host CLIs.

When adding adapters, keep the same boundary: Matty should configure host-specific prompts/state through narrow adapters and avoid growing the core prompt.

## Historical planning source

The completed maps and tickets under `.scratch/` preserve planning history; they
are not active roadmaps or runtime documentation. Durable product behavior lives
in the README and product docs, while accepted architecture lives in
[`docs/adr/`](adr/).
