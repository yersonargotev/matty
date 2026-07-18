# Maintainer installation cutover to Packy

Status: passed

Execution window: 2026-07-18T14:36:04Z–2026-07-18T14:48:16Z

Source ticket: [#65](https://github.com/yersonargotev/packy/issues/65)

Starting Packy `origin/main`: `ad23cc0a33fe32d1f730003c24d8df87934dd9d7`

The exact clean-cut harness is [`run.sh`](run.sh). Its final read-only recovery
verification is [`finalize.sh`](finalize.sh). The gzip-compressed complete
cutover/finalization stdout and stderr are in
[`transcript.log.gz`](transcript.log.gz). A bounded diagnostic record for the
only adjudicated gate is in
[`diagnostic-adjudication.log`](diagnostic-adjudication.log). The final record
ends with `overall_failures=0`, `cutover=passed`, and `finalization=passed`.
The separate, sandboxed public-boundary proof
[`prove-codex-config-preservation.sh`](prove-codex-config-preservation.sh) and
its captured [`codex-config-preservation.log`](codex-config-preservation.log)
verify that the delegated Engram setup preserves contributor-owned Codex values.

## Pre-mutation gate

- Homebrew Matty `0.1.6`, its binary, healthy doctor, clean Installed Source at
  exact `v0.1.4`/`f348b84e50222a4eeadf5abbcedef7a24974cd88`, and all 23
  state-recorded links were reverified.
- `packs.json` and its lock were absent. Targeted `matty` intent was absent on
  Codex and OpenCode. Aggregate status failed only for the already classified,
  externally owned OpenCode Engram MCP settings.
- The stale clean tap remained at exact Matty formula commit
  `7603485b5071db932e83f6edf9f83b69960cd0f3`; the published Packy tap commit
  `ae1a2f979f073a5b07214d8f303c7ce5ff67d84d` was remotely verified and
  prefetched without changing the worktree before the destructive boundary.

## Recovery preservation and Matty removal

- The complete recovery tree was copied to the approved external path
  `~/Documents/dev/backups/matty-to-packy-cutover-20260717/`.
- Source and destination matched by a typed inventory: six directories, three
  regular files, exact modes/sizes/SHA-256 values, and no symlinks or special
  entries. The retained `SHA256SUMS` verified before and after deletion.
- The unchanged uninstall plan removed only 23 recorded links, classic state,
  Matty Codex markers, and the exact OpenCode reference/file.
- The fully qualified Matty formula was uninstalled with Homebrew auto-update
  disabled before the tap changed. Only the reverified-clean Installed Source
  and backup-only legacy state root were then removed.
- The zero-active-residual gate found no Matty formula/binary, live state,
  Installed Source, link into it, or live product marker/reference.

## Fresh Packy ownership

- The tap was reset to the prefetched published commit and fully qualified
  `yersonargotev/tap/packy` installed Packy `v0.1.7`.
- `packy init` selected exact tag `v0.1.7`, commit
  `283e726e9e1886d8b51e3222434022ac56f733eb`; the source remained clean.
- Install dry-run/apply created fresh `~/.packy/config.json`, 23 links whose
  recorded sources are all under `~/.local/share/packy`, and exact Packy
  Codex/OpenCode projections. Human and JSON doctor reports passed all eight
  checks.
- The semantic `matty` pack remained version `2.0.0`, absent by intent on both
  surfaces, and produced an applicable activation dry-run without activation.

## External ownership adjudication

Packy explicitly delegated setup to the Homebrew-owned Engram `1.19.0` binary.
That command caused Codex to reserialize `config.toml` paths and refresh Engram
marketplace metadata, so the initial whole-file digest gate stopped after the
otherwise successful Packy install. This was not contributor loss: normalized
Codex prompt content, normalized OpenCode content, and the Engram binary matched
their pre-cutover digests; the contributor Codex config and its unrelated
sections remained; and the sandbox proof exercised the same installed
`engram setup codex` boundary with unrelated root values, nested tables, MCP,
project, marketplace, plugin, desktop, notice, and shell-policy sentinels. Their
canonical semantic digest matched before and after while the Engram projection
was added. Engram projections, binary, and running MCP processes passed fresh
checks. [`finalize.sh`](finalize.sh) records that adjudication and the unchanged
final product/archive/history gates. No Matty restoration was needed.

## Authentic history

The old repository endpoint still resolves immutable Matty tag `v0.1.6`; the
Packy repository resolves immutable tag `v0.1.7`; and both historical Matty and
current Packy release assets remain downloadable. The external recovery archive
is outside Packy ownership.

## Evidence integrity

[`SHA256SUMS`](SHA256SUMS) binds this index, both exact cutover harnesses, the
sandbox proof, the complete compressed transcript, and both diagnostic records.
Any change requires regenerating the manifest and re-reviewing issue #65
evidence.
