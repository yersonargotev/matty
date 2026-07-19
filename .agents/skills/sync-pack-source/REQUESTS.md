# Request normalization and preflight

Load this reference for normalization, preflight, attachment, or dispatch.

## Authority

- Work only inside a checkout whose canonical remote resolves to
  `yersonargotev/packy`, but read authority with `gh api` from remote `main`.
- Require `git`, `gh`, `jq`, active authentication, read access to repository
  contents/Actions/branches/pull requests, and actual workflow-dispatch access.
- Read `bundle/sources.json`, both versioned dispatch schemas, and
  `.github/workflows/sync-pack-source.yml` through GitHub's contents API at
  `ref=main`. Confirm the workflow is active and remote `main` resolves.
- Download `scripts/request.sh`, `attach.sh`, `dispatch.sh`, and
  `result-state.sh` from that same observed remote-main commit into a fresh
  temporary directory and execute only those copies. Checkout-local script
  bytes are never operational authority.
- Observe per-source active/pending workflow runs, `sync/<source-id>`, and its
  open PR. These are preflight facts only; the workflow owns deep ownership,
  regression, provenance, divergence, and readiness decisions.
- Never checkout, pull, clone, execute upstream content, or read synchronization
  authority from the local tree. Never change permissions or handle secrets.

Materialize the remote runtime without touching the checkout:

```sh
remote_main_sha="$(gh api repos/yersonargotev/packy/branches/main --jq .commit.sha)"
remote_skill_runtime="$(mktemp -d)"
for script in request.sh attach.sh dispatch.sh result-state.sh; do
  gh api -H 'Accept: application/vnd.github.raw+json' \
    "repos/yersonargotev/packy/contents/.agents/skills/sync-pack-source/scripts/$script?ref=$remote_main_sha" \
    > "$remote_skill_runtime/$script"
  chmod 700 "$remote_skill_runtime/$script"
done
```

Use that same `remote_main_sha` for every subsequent remote contents read.

## Normalize intent

Use schema version 1 for `synchronize` requests. Use schema version 2 with
`operation: register` only when the maintainer explicitly requests admission
of an absent source. A registration request must contain the complete strict
`SourceConfig` as `registration`; its `id` must equal `source_id`, its selector
must agree with the request selector, and its resources must bind the complete
intended source contribution. Canonicalize the configuration by the runtime
rules (including binding order), encode it as two-space-indented JSON with one
trailing newline, and set `registration_sha256` to the lowercase SHA-256 of
those exact bytes. Never infer an absent source registration from an update
request.

Infer `source_id` for synchronization only when an explicitly named configured
source, repository, or pack has exactly one match in remote
`bundle/sources.json`. Ask when zero or multiple sources match. For explicit
registration, require the named source to be absent and derive its complete
configuration from an already reviewed, Packy-owned manifest or specification;
ambiguity in any binding blocks before dispatch.

| Intent | Canonical selector |
| --- | --- |
| stable, generic unambiguous update | `latest-stable`, no `selector_ref` |
| exact published prerelease | `prerelease` plus the exact tag |
| exact commit | `commit` plus one full lowercase 40-character SHA |
| explicit human inspection | requested selector plus `human` mode, no evidence |
| human evidence publication | `commit` plus full resolved SHA, `human`, exact plan/base, canonical evidence |
| exact retry | `commit` plus artifact candidate SHA and `retry_of_run` |

Default classification is `ai`. Preserve the maintainer's reason faithfully in
`request_reason`; do not embellish it. A retry is exact only after validating
the named run's operational artifact. A pre-resolution failure can become a
new, explicitly labelled stable selection, never an exact retry.

Reject versions without an exact prerelease tag, releases that are not
prereleases, branches, abbreviated/uppercase SHAs, floating or unpublished
refs, and arbitrary tags. Reject pack IDs as dispatch inputs and all branch,
PR, base, version, provenance, validation, permission, credential, secret,
auto-merge, upstream-byte, executable, repair, or bypass inputs.

Build JSON with `jq`, omitting absent optional properties. Its allowed keys are
exactly those in the matching versioned dispatch schema. Validate it against
that checked-in schema by its canonical `$id`; do not resolve it over the
network. Show the exact JSON before dispatch. Map `human_evidence` to the
workflow transport input `human_evidence_json` and `registration` to
`registration_json` without changing either canonical JSON value.

## Attach or dispatch

Compute `request_digest` as the lowercase SHA-256 of `jq -cS .` output, including
its trailing newline. For every active or pending run of the canonical workflow
on `main`, compare that digest with its exact run-name identity. For a started
run, also download `request.json`, recompute its digest, and require both values
to agree. Attach only on equality; a malformed or absent identity blocks rather
than permitting a guessed duplicate. The digest is transport identity, not a
canonical request field or synchronization authority.

A distinct admitted request may be dispatched; GitHub's non-cancelling
per-source concurrency owns queueing and pending supersession. Report the
observed active and pending URLs and never manipulate that queue.

List canonical runs with `databaseId`, `displayTitle`, `status`, and `url`,
download any started run's `request.json` as `<databaseId>-request.json`, then
invoke the remote `attach.sh`. Exit 0 attaches, exit 1 admits dispatch, and exit
2 is ambiguous and blocks. Pending attachment relies on the verified run-name
digest because it cannot yet own an artifact.

Submit stdin JSON exactly once with the remote-main renderer, which adds
only the required transport digest and executes the accepted primary command:

```sh
"$remote_skill_runtime/dispatch.sh" canonical-request.json
```

Require the returned run URL; do not rediscover the run by time or actor. If
dispatch is unavailable, report **bloqueada** and show the exact `gh workflow
run .github/workflows/sync-pack-source.yml --repo yersonargotev/packy --ref
main --json` command plus equivalent Actions UI fields. Instructions are not
success.
