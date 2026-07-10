# ADR 0002: Package-installed Matty uses an Installed Source checkout

## Status

Accepted for the first installable release.

## Decision

When Matty is installed from a GitHub Release or Homebrew tap, the binary will
read its managed skill bundle from a user-owned Installed Source checkout.
Users initialize that checkout with:

```bash
matty init
matty install --dry-run
```

The default Installed Source root is:

```text
~/.local/share/matty
```

Matty then reads the default skill bundle from:

```text
~/.local/share/matty/bundle/skills
```

This mirrors the `dots init` model: package managers install the executable,
while the first-run init command clones the repository Source of Truth into a
stable user-data path that the executable can read from any current working
directory. The repository intentionally contains only Matty-owned runtime
material; historical external reference trees are not part of the Installed
Source.

## Why this model

The current development default discovers `bundle/skills` by walking upward from
the caller's working directory. That is correct for repository checkouts, but it
is not reliable for a Homebrew- or GitHub-Release-installed binary launched from
an arbitrary project directory.

An Installed Source checkout keeps one source model across distribution paths:

| Topic | Decision |
| --- | --- |
| Source of Truth | `https://github.com/yersonargotev/matty.git` is cloned by `matty init`. |
| Default root | `~/.local/share/matty` for the first installable release. |
| Skill bundle path | `<installed-source>/bundle/skills`. |
| First command | `matty init`, then `matty install --dry-run`. |
| Package contents | GitHub Release and Homebrew install the binary only. |
| Dev/test seam | `MATTY_SKILLS_SOURCE` continues to override the skill bundle root directly. |
| Tests/checks | Any test or manual package smoke check must sandbox `HOME` and `XDG_CONFIG_HOME`, and therefore the default Installed Source path. |

## Version pinning

`matty init` should pin the Installed Source to the running binary version when
the binary version is a release tag such as `v0.1.0`. Development binaries
(`0.0.0-dev`) may use the repository default branch unless the user passes an
explicit repository ref.

This gives each released binary a matching copy of `bundle/skills` without
requiring Homebrew to manage non-binary resources. It also keeps GitHub Release
and Homebrew installs equivalent: both need the same `matty init` step and both
resolve the same default checkout.

## Update semantics

`matty init` owns the Installed Source checkout. It should be idempotent:

- If the default path is missing, clone it.
- If the path already contains a valid Matty checkout at the expected ref,
  report that it is already initialized.
- If the checkout is clean but stale for the running release, update it to the
  expected ref.
- If the checkout is dirty, missing Git metadata, or not a valid Matty Source of
  Truth, fail with guidance to move it aside or pass an explicit source root.

`matty update` keeps its v0 meaning: refresh Matty-managed workflow artifacts
from the resolved skill bundle and rerun the delegated Engram refresh/setup
path. It should not upgrade the binary and it should not mutate the Installed
Source. For package-installed Matty, users upgrade the binary with Homebrew
(`brew upgrade matty`) or by installing a newer GitHub Release artifact, then
run `matty init` again to align the Installed Source before running
`matty update` or `matty install --dry-run`.

Matty does not need a separate `matty upgrade` command for v0. The package
manager owns binary upgrades, and `matty init` owns Source checkout alignment.

Dry-run commands must not mutate the Installed Source. If a dry-run detects that
the default Installed Source is missing or stale relative to the running release,
it should explain that the user needs `matty init` first.

## Uninstall expectations

`matty uninstall` removes only Matty-managed workflow artifacts: skill symlinks,
Matty prompt/config blocks, and Matty state. It should not delete
`~/.local/share/matty` by default because the Installed Source is user-owned
data and may contain local inspection state.

Users who want a full cleanup can remove the Installed Source manually:

```bash
rm -rf ~/.local/share/matty
```

A future explicit cleanup flag can make that safer, but it is not required for
the first installable release.

## Rejected options

### Embed `bundle/skills` in the Go binary

Embedding would make the binary self-contained, but it is the wrong first
release model:

- Skill files would be hidden inside the executable instead of inspectable as a
  normal checkout.
- Global skill symlinks need stable filesystem targets; embedded files would
  require extraction/copying and another ownership model.
- Updating skill content would require replacing the binary even when the user
  only needs source/bundle alignment.
- It would create a different runtime model from the repository checkout path.

### Install bundle resources into the Homebrew Cellar

Homebrew-managed resources would work only for Homebrew and would make GitHub
Release installs different:

- Raw GitHub Release binaries do not have a Cellar resource path.
- Symlinking global skills into a versioned Cellar path is fragile across
  `brew upgrade` and `brew cleanup`.
- The Cellar is package-manager-owned, not a good place for Matty's user-data
  Source of Truth checkout.
- Release scripts and formulas would need to publish/install extra resources
  when the Dots reference path proves a binary-only formula is enough.

## Consequences

- `internal/cli` should not know clone/update details. Put `matty init` source
  bootstrapping behind a small internal package, similar to `dots/internal/bootstrap`.
- Path resolution should keep `MATTY_SKILLS_SOURCE` as the highest-priority
  development override, keep repository checkout discovery for local builds, and
  fall back to `~/.local/share/matty/bundle/skills` for package-installed runs.
- The Homebrew formula can stay simple: select the platform-specific raw binary,
  install it as `matty`, and test `matty --version`.
- Package smoke tests should prove the first-run sequence with sandboxed
  `HOME`/`XDG_CONFIG_HOME`: install binary, run `matty init`, run
  `matty install --dry-run`, then continue the existing lifecycle baseline.
