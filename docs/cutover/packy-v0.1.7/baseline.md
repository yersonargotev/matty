# Packy v0.1.7 cutover baseline

Status: frozen

Window opened: 2026-07-18T03:00:48Z

Source ticket: [#56](https://github.com/yersonargotev/matty/issues/56)

This document opens the controlled Packy cutover window and binds its initial
inputs before any product identity changes. The command-level record, including
timestamps, tool versions, exit statuses, and complete outputs, is in
[`evidence/issue-56/baseline.log`](evidence/issue-56/baseline.log).

## Freeze contract

- Ordinary releases are frozen. The only permitted release path is the
  controlled `v0.1.7` publication in issue #63, from the exact Packy `main` SHA
  accepted by issues #60 and #62.
- Pack-source synchronization is fully frozen. No manual dispatch may start
  until issue #66 completes the final audit and explicitly reopens automation.
- The release workflow must not be dispatched and no `v0.*.*` tag may be
  created before issue #63. Same-tag recovery is permitted only for idempotent
  completion at the unchanged `v0.1.7` tag SHA.
- Any conflicting run, namespace collision, invalidated SHA, or changed
  publication input stops the cutover and requires fresh evidence.

The workflows are intentionally still enabled: pack-source synchronization is
manual-only, while disabling the release workflow would also disable the one
permitted `v0.1.7` path. The freeze is the explicit operational allowlist above,
recorded in the repository and on issue #56. At the opening probe there were no
queued or in-progress runs.

## Bound inputs

| Input | Bound value |
| --- | --- |
| Pre-rename base | `0e8971ad4ccacad5f99ec97d05ed963830b58070` |
| Base relationship | `HEAD`, local `main`, and `origin/main` were equal before worktree creation |
| Candidate branch | `feat/packy-atomic-cutover` |
| Candidate starting HEAD | `0e8971ad4ccacad5f99ec97d05ed963830b58070` |
| Migration batches | #57 executable/local ownership; #58 automation/distribution/docs; #59 schema suite |
| Integration gate | #60 binds and proves the final unchanged candidate SHA |
| Accepted inputs | #50 identity classification; #51 external constraints; #52 install/state contract; #53 ordering; #54 acceptance contract; #55 schema identity |
| Planned PR boundary | One atomic identity PR containing #57-#60; no partial identity merge |

There is no candidate SHA yet. Issue #60 must record it after all three migration
batches and acceptance automation are assembled; this baseline must not be
misread as binding a future, unknown commit.

## Immediate external recheck

The opening probes found no blocking collision:

- `yersonargotev/packy` did not exist; `yersonargotev/matty` remained the public
  source repository with default branch `main`.
- `yersonargotev/homebrew-tap` contained `Formula/matty.rb`, no
  `Formula/packy.rb`, and no `formula_renames.json`.
- Homebrew/core contained neither an expected-path Matty formula nor an
  expected-path Packy formula.
- `https://yersonargotev.github.io/` and `/packy/` both returned HTTP 404.
- The authenticated GitHub probe found no
  `yersonargotev/yersonargotev.github.io` source repository.
- GitHub reported no queued or in-progress Matty Actions run.

The expected absence probes return non-zero status for GitHub 404 responses;
those statuses are evidence of a free namespace, not ignored command failures.
All namespace and workflow probes must run again immediately before issue #61.

## Historical Matty recovery proof

The historical public inputs are mutually consistent:

- tag `v0.1.6` resolves to commit
  `68aec8969374fa9e9a6ea86b33e6719646b999f8`;
- the published release is neither draft nor prerelease and exposes
  `checksums.txt` plus the four expected raw Matty executables;
- all four downloaded files passed the published SHA-256 manifest;
- every tap formula URL and SHA-256 matched the corresponding downloaded file;
- the downloaded Darwin arm64 executable ran in isolated `HOME` and
  `XDG_CONFIG_HOME` roots and reported `matty version v0.1.6`;
- the current tap formula is version `0.1.6`, installs `matty`, and tests
  `matty --version`.

These checks establish obtainability and byte consistency. The full disposable
historical installation lifecycle remains the separate acceptance gate in #64.

## Maintainer recovery boundary

Only read-only observations were made against the maintainer installation:

- Homebrew Matty `0.1.6` is installed and `/opt/homebrew/bin/matty` reports
  `v0.1.6`.
- Direct filesystem inspection found `~/.matty/config.json` and
  `~/.matty/backups/`; pack intent and lock files are absent. The state records
  23 managed links, all resolving to their recorded sources, and the three known
  recovery files are still present.
- `~/.local/share/matty` is a clean detached checkout at tag `v0.1.4`, commit
  `f348b84e50222a4eeadf5abbcedef7a24974cd88`.
- The approved destination
  `~/Documents/dev/backups/matty-to-packy-cutover-20260717/` and its parent do
  not exist yet. Issue #65, not this ticket, creates, hashes, and verifies that
  copy before any legacy deletion.

Direct inspection also confirms that OpenCode contains its existing enabled
Engram MCP configuration. This is accepted baseline external ownership under
issue #52: Packy and Matty must preserve it, must not adopt or rewrite it, and
must re-audit it before #65. It is not Matty-owned state.

No product CLI command was run against the operator's real `HOME` or
`XDG_CONFIG_HOME`. Real recovery material was observed only with direct,
read-only filesystem/configuration tools; executable behavior used isolated
roots.

## Evidence integrity

`evidence/issue-56/SHA256SUMS` binds the durable files created for this gate.
Any edit to this baseline or its raw log requires regenerating that manifest and
re-reviewing issue #56 evidence. Later tickets append their own evidence rather
than rewriting this opening observation.
