# 01 — Decide package-installed source model

Type: grilling
Status: resolved
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

## Answer

Resolved in `docs/adr/0002-package-installed-source-model.md`.

Matty should use the `matty init` + Installed Source checkout model for the
first installable release. A package-installed binary will read
`bundle/skills` from a user-owned checkout at `~/.local/share/matty` by default,
so the skill bundle path is `~/.local/share/matty/bundle/skills`.

The package-installed first-run path is:

```bash
matty init
matty install --dry-run
```

`MATTY_SKILLS_SOURCE` remains a development/test seam that overrides the skill
bundle root directly. It does not become the production default and should not
point to external reference clones in production behavior.

Rejected for the first release:

- Binary-embedded skills, because global symlinks need stable inspectable
  filesystem targets and embedding would add an extraction ownership model.
- Homebrew Cellar resources, because raw GitHub Release binaries would not have
  the same resources and Cellar paths are versioned/package-manager-owned.
