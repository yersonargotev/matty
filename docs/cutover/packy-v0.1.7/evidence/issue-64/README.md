# Disposable Matty and Packy installation proof

Status: passed

Execution window: 2026-07-18T13:08:25Z–2026-07-18T13:13:32Z

Source ticket: [#64](https://github.com/yersonargotev/packy/issues/64)

Starting Packy `origin/main`: `283e726e9e1886d8b51e3222434022ac56f733eb`

The exact executed harness is [`run.sh`](run.sh). Its gzip-compressed complete timestamped
stdout/stderr and per-command exit statuses are in
[`transcript.log.gz`](transcript.log.gz). The run finished with
`overall_failures=0` and deleted both disposable roots.

## Isolation boundary

The two directions used independent temporary clones of Homebrew, each with its
own prefix, repository, `HOME`, `XDG_CONFIG_HOME`, cache, logs, temp directory,
and Git configuration. Their `PATH` values contained only the temporary
Homebrew `bin` and Apple system directories; `/opt/homebrew` was never an
executable source. Prefix assertions matched Homebrew's canonical prefix in both
directions.

All lifecycle commands ran outside the repository checkout so source discovery
selected the package Installed Source, not the development checkout. The only
operator-state observation was the read-only doctor warning that an existing
Engram process used `/opt/homebrew`; no operator process or file was changed.

## Historical Matty v0.1.6

- The isolated tap was detached at preserved commit
  `5168baccc0aa16d3d4a7a1bac1ca1c00b11158a3`.
- `Formula/matty.rb` SHA-256 was
  `dfad3d15a29130a7d2debb36c9196bff6cb64361e9033c23e474d1f933d30271`.
- The downloaded Darwin arm64 asset matched the release manifest at
  `be0cc89a5e950e486103959c0018acf54a7eb6c2e8bb8176d125faa419451d6e`.
- Real `brew install yersonargotev/tap/matty` installed formula `0.1.6`; the
  installed binary had the same digest and reported `matty version v0.1.6`.
- `matty init` selected exact tag `v0.1.6`, commit
  `68aec8969374fa9e9a6ea86b33e6719646b999f8`.
- The package-source lifecycle passed: install dry-run/apply, doctor human/JSON,
  update dry-run/apply, pack discovery, uninstall dry-run/apply, final doctor,
  cleanup assertions, and resulting filesystem observation.
- The isolated Engram tap was explicitly trusted only inside the disposable XDG
  root so historical `matty update` could complete under current Homebrew trust
  rules.

## Fresh Packy v0.1.7

- Real `brew install yersonargotev/tap/packy` used tap commit
  `ae1a2f979f073a5b07214d8f303c7ce5ff67d84d`.
- `Formula/packy.rb` SHA-256 was
  `f448f50cca97b6768e53e42da74aa613e2c8d0bfafc130f502d5f8e032bb7729`.
- The downloaded Darwin arm64 asset matched the release manifest at
  `8e95ed2888845aa06caca336f4ad70153fc2cfb7b45c21177a4d07877cccfd8b`.
- The installed binary had the same digest, reported `packy version v0.1.7`,
  and `packy init` selected exact tag `v0.1.7`, commit
  `283e726e9e1886d8b51e3222434022ac56f733eb`.
- The package-source lifecycle passed: install dry-run/apply, doctor human/JSON,
  uninstall dry-run/apply, final doctor, cleanup assertions, and resulting
  filesystem observation.

## Disjoint ownership and semantic `matty` pack

Before Packy ran, the fresh sandbox received `.matty` and
`.local/share/matty` sentinel files plus a deliberately invalid
`MATTY_SKILLS_SOURCE`. Packy ignored the legacy variable, created and recorded
only Packy source/state/markers, and all managed skill source paths were under
`.local/share/packy`. The legacy sentinel SHA-256 values were unchanged after
uninstall.

The installed Packy binary listed and showed the semantic `matty` pack,
reported its Codex and OpenCode status in JSON, and produced an actionable Codex
activation dry-run for capability `workflow:matty`. This proves the surviving
capability-pack identity without activating it or creating mixed ownership.

## Evidence integrity

[`SHA256SUMS`](SHA256SUMS) binds this index, the exact harness, and the complete
transcript. Any change requires regenerating the manifest and re-reviewing the
evidence.
