# Packy v0.1.7 final acceptance audit

Status: passed

Execution window: 2026-07-18T17:38:25Z–2026-07-18T17:42:40Z

Source ticket: [#66](https://github.com/yersonargotev/packy/issues/66)

Starting Packy `origin/main`:
`2ed52a16a88d3b150dcf0a2bbbc596eb72cb389f`

Exact audit candidate:
`9f0ed789fea4d078526984ad4c6fe4e878778e11`

The exact read-only harness is [`run.sh`](run.sh). Its gzip-compressed complete
timestamped stdout/stderr and per-command exit statuses are in
[`transcript.log.gz`](transcript.log.gz). The record ends with
`overall_failures=0`, `final_audit=passed`, `availability_candidate=ready`, and
`automation_candidate=ready`.

## Bound identity set

| Identity | Exact value |
| --- | --- |
| Frozen pre-rename base | `0e8971ad4ccacad5f99ec97d05ed963830b58070` |
| Accepted atomic candidate | `87718a145ecbee25556009218cff25806c67365a` |
| Atomic Packy merge and immutable `v0.1.7` tag | `283e726e9e1886d8b51e3222434022ac56f733eb` |
| Current evidence-bearing starting main | `2ed52a16a88d3b150dcf0a2bbbc596eb72cb389f` |
| Exact final-audit candidate | `9f0ed789fea4d078526984ad4c6fe4e878778e11` |
| Published tap | `ae1a2f979f073a5b07214d8f303c7ce5ff67d84d` |

The atomic merge parents are the frozen base and accepted candidate, and its
tree is byte-identical to the candidate tree. The tag resolves to that merge
without movement. The current main and audit candidate descend from it. The
repository, old-name redirect, active checkout, and every linked worktree all
resolve to `yersonargotev/packy`.

## Repository, automation, and Pages

- `./scripts/validate-packy.sh`, including race, and separate `go test ./...`
  passed on the exact audit candidate under disposable `HOME` and
  `XDG_CONFIG_HOME`.
- Exact candidate CI `29632325438`, merge CI `29642069661`, merge Pages
  `29642312499`, release run `29644676900`, and current-starting-main CI
  `29651231126` are complete and successful at their bound SHAs.
- Every repository workflow is active, with no queued or in-progress run during
  the final probe. No ordinary release or synchronization was dispatched while
  the cutover freeze remained in force.
- Pages is built from `main:/` with HTTPS enforced. Each hosted schema is valid
  JSON, has the requested canonical `$id`, and is byte-identical to the atomic
  candidate, tagged merge, current starting main, and checkout:
  - dispatch: `c759176f7cc20bed520104ce7a5d732b2318b29c0442f80caa1b54318f13b571`
  - no-op: `596c81c047cea8160190b06e531bda474d748bfc534b01bf44594f701ab26b99`
  - operational artifact: `34f3b4e29e69b2f3f0c4e4dd65d2c216a94043ffaa358d25cda659e64ef0224e`
  - publication: `e6ec28082e88ad20eb32a5a9ee4142164fd77784278bea9f596e61bf2ae22931`
  - validation: `04d2ab6ba1394faab4bfe9d9347c0bcfb5a2ce57622a4ebba8982fbdd36a0da8`

## Release and tap

The release contains exactly `checksums.txt` and four Packy binaries. Fresh
downloads matched the manifest, and the native asset reported `v0.1.7`:

| Asset | SHA-256 |
| --- | --- |
| `packy_v0.1.7_darwin_amd64` | `d22c42c464deca8e2f574f8a86d1913e6c9dd38d976740f427bfc92503c2d293` |
| `packy_v0.1.7_darwin_arm64` | `8e95ed2888845aa06caca336f4ad70153fc2cfb7b45c21177a4d07877cccfd8b` |
| `packy_v0.1.7_linux_amd64` | `ea0073925a2147def3c997cd0ca390eeb3783c8d3bdc625d1d0f7da9dd23678e` |
| `packy_v0.1.7_linux_arm64` | `1212a53f764a54fe2158f49471d4972ced4a471122c907ff31c5a863f2b38e48` |

The remote and installed tap are clean at the bound tap commit. It contains
`Formula/packy.rb`, no legacy formula or rename metadata, and its version, URLs,
homepage, binary name, test, and four checksums match the release.

## Maintainer installation and preservation

- Homebrew owns Packy `0.1.7`; its installed binary digest equals the release
  and formula. No legacy formula, binary, state root, Installed Source, product
  marker/reference, or link into the old source remains active.
- The Installed Source is clean at exact `v0.1.7`/`283e726e...`. All 23 classic
  state links resolve to sources under `~/.local/share/packy`, and all eight
  doctor checks pass.
- The semantic `matty` pack remains version `2.0.0`, is observable on Codex and
  OpenCode, and produces an applicable activation dry-run without changing
  intent.
- Contributor-owned Codex configuration, the Engram `1.19.0` binary/process,
  the typed 6-directory/3-file recovery archive and its manifest, and authentic
  historical tag/release availability all passed fresh read-only checks.
- The checksum-bound #56, #64, and #65 evidence manifests remain intact; the
  durable #60, #62, and #63 exact-SHA evidence is linked in the transcript.

## Availability and automation ordering

This audit makes Packy v0.1.7 ready for availability and ordinary automation,
but does not itself dispatch either workflow. The historical baseline remains
immutable and records the freeze as it existed when the window opened.
Automation stays operationally frozen until this evidence is merged, exact
merge-SHA CI and Pages succeed, and the final issue #66 announcement explicitly
opens ordinary release and pack-source synchronization. A failed delivery gate
keeps the freeze in force.

## Evidence integrity

[`SHA256SUMS`](SHA256SUMS) binds this index, the immutable issue request, exact
harness, and complete compressed transcript. Any edit requires regenerating the
manifest and repeating review.
