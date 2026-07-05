# 01 — Decide package-installed source model

Type: grilling
Status: open
Blocked by:

## Question

When Matty is installed by Homebrew/GitHub Release, where does the managed skill bundle come from by default?

Options to compare:

1. `matty init` clones the Matty Source of Truth into a default installed source root such as `~/.local/share/matty`, similar to `dots init`.
2. The Go binary embeds `bundle/skills` and writes/syncs from embedded content.
3. Homebrew installs binary plus bundle resources into the Cellar and Matty resolves them relative to the executable.

The answer should produce an ADR-level decision covering default paths, dev/test override behavior, version pinning, update semantics, uninstall expectations, and package-manager tradeoffs.

## Acceptance criteria

- Documents the chosen source model in `docs/adr/` or `docs/product/`.
- Explains why the rejected options are not chosen for the first installable release.
- Defines the default installed source path if cloning is chosen.
- States how `MATTY_SKILLS_SOURCE` behaves after the change.
- States what a package-installed user should run first: e.g. `matty init`, then `matty install --dry-run`.
