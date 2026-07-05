# 11 — Decide update vs upgrade semantics for package-installed Matty

Type: grilling
Status: resolved
Blocked by: 01, 03

## Question

After Matty is installed through Homebrew and initialized with a versioned source, what should `matty update` mean, and is a separate `matty upgrade` needed?

Current `matty update` refreshes Engram via Homebrew and reapplies Matty-managed skills/prompts. Package installation introduces two more moving parts: the Matty binary and the initialized source/bundle.

## Acceptance criteria

- Defines whether `matty update` updates only managed workflow artifacts or also the initialized source/bundle.
- Defines whether binary upgrades are delegated to `brew upgrade matty` or wrapped by a `matty upgrade` command.
- Explains how dry-run behaves without mutating the initialized source.
- Identifies any follow-up implementation issues created by the decision.


## Answer

`matty update` keeps its v0 meaning: it refreshes Matty-managed workflow
artifacts from the resolved skill bundle and re-runs the delegated Engram
refresh/setup path. It does not update the Matty binary and it does not mutate
the Initialized Source checkout.

Binary upgrades stay with the installer channel. Homebrew users run
`brew upgrade matty`; direct GitHub Release users replace the binary with a
newer release artifact. After changing the binary version, users run
`matty init` again so the Installed Source checkout can align to the running
release tag before `matty install --dry-run`, `matty update --dry-run`, or
`matty update` reads the bundle.

No separate `matty upgrade` command is needed for v0. Adding one would wrap
package-manager behavior without owning the package-manager state, and it would
make direct GitHub Release installs a different upgrade path from Homebrew.

Dry-run behavior: `matty update --dry-run` previews the managed workflow plan
without writing state, changing symlinks/prompts, running external commands, or
changing `~/.local/share/matty`. If the default Installed Source is missing or
stale for the running binary, the dry-run should fail with guidance to run
`matty init` rather than repairing the checkout itself.

Follow-up implementation issues: none for v0. Existing behavior already keeps
`matty update` scoped to Engram/workflow refresh (`brew update`,
`brew upgrade engram`, skill links, prompts, state) and the package-install
smoke test covers update dry-run non-mutation against the sandboxed Installed
Source. Future release-note work can document this in the first public tag
announcement, but it is not a separate implementation issue.
