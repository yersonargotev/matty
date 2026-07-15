# Request normalization and preflight

Load this reference for normalization, preflight, attachment, or dispatch.

## Authority

- Work only inside a checkout whose canonical remote resolves to
  `yersonargotev/matty`, but read authority with `gh api` from remote `main`.
- Require `git`, `gh`, `jq`, active authentication, read access to repository
  contents/Actions/branches/pull requests, and actual workflow-dispatch access.
- Read `bundle/sources.json`, the dispatch schema, and
  `.github/workflows/sync-pack-source.yml` through GitHub's contents API at
  `ref=main`. Confirm the workflow is active and remote `main` resolves.
- Observe per-source active/pending workflow runs, `sync/<source-id>`, and its
  open PR. These are preflight facts only; the workflow owns deep ownership,
  regression, provenance, divergence, and readiness decisions.
- Never checkout, pull, clone, execute upstream content, or read synchronization
  authority from the local tree. Never change permissions or handle secrets.

## Normalize intent

Infer `source_id` only when an explicitly named configured source, repository,
or pack has exactly one match in remote `bundle/sources.json`. Ask when zero or
multiple sources match.

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
exactly those in `workflows/schemas/pack-source-dispatch.schema.json`. Validate
it against the remote schema and show the exact JSON before dispatch. Map
`human_evidence` to the workflow transport input `human_evidence_json` without
changing its canonical JSON.

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

Submit stdin JSON exactly once with the repository-owned renderer, which adds
only the required transport digest and executes the accepted primary command:

```sh
./.agents/skills/sync-pack-source/scripts/dispatch.sh canonical-request.json
```

Require the returned run URL; do not rediscover the run by time or actor. If
dispatch is unavailable, report **bloqueada** and show the exact `gh workflow
run .github/workflows/sync-pack-source.yml --repo yersonargotev/matty --ref
main --json` command plus equivalent Actions UI fields. Instructions are not
success.
