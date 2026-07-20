# Packy GitHub security and delivery authority audit

Research date: 2026-07-20

## Question and evidence boundary

What repository settings, identities, workflows, credentials, branches, tags,
releases, apps, and automation can currently mutate Packy or its delivery
artifacts; which protections are active; and where are the concrete gaps against
the destination and standing notes in
[Harden Packy's GitHub repository and delivery governance](https://github.com/yersonargotev/packy/issues/122)?

This is a read-only point-in-time audit of `yersonargotev/packy`. It uses the
GitHub REST API through `gh`, the repository at default-branch commit
[`fa522936d2f7428277729911db5982f4211f6044`](https://github.com/yersonargotev/packy/tree/fa522936d2f7428277729911db5982f4211f6044),
remote Git refs, and official GitHub documentation. No setting, issue, pull
request, workflow, secret, credential, ref, release, or external repository was
changed. Secret APIs were queried only for names and timestamps; no secret value
was requested or exposed.

## Decision-ready answer

Packy's effective mutation authority is concentrated in one human account and
the GitHub Actions tokens that account can cause GitHub to mint:

```text
yersonargotev (personal owner, sole Admin)
  |-- direct Git/API/web authority over repository settings and every ref
  |-- can dispatch Release on a selected ref
  |     |-- GITHUB_TOKEN: contents:write -> releases and release assets
  |     `-- HOMEBREW_TAP_TOKEN -> yersonargotev/homebrew-tap main
  |-- can dispatch Synchronize pack source on a selected ref
  |     `-- GITHUB_TOKEN: contents:write + pull-requests:write
  |           -> sync/* branch and draft/updated/ready PR
  `-- pushes to main
        |-- CI (contents:read only)
        `-- GitHub-managed Pages build -> public GitHub Pages site
```

There is currently no enforcement boundary between holding Admin authority and
using it routinely. `main` is unprotected, no repository ruleset exists, tags
are unprotected, published releases are mutable, and no PR, review, status
check, issue-link, signed-commit, or no-bypass requirement gates integration.
This is the central gap against the map's intended Integrator-only, protected-PR
flow and break-glass-only administrative bypass.

The automation is partly least-privileged: repository-default workflow
permissions are read-only, CI is read-only, and the synchronization workflow
starts at `permissions: {}` and grants write only to its Publish job. However,
both mutation-capable workflows can be manually dispatched on a selected ref;
neither uses a protected environment or approval gate; and the Release workflow
exposes a repository-scoped cross-repository credential to jobs that use action
tags rather than full commit SHAs. GitHub requires write access to dispatch a
workflow and explicitly permits selecting a non-default ref
([manual workflow documentation](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow)).
That means adding a future Maintain/Write principal without first installing
ref, workflow, and deployment controls would also grant that principal a path
to invoke write-capable code from a selectable branch.

## Observed authority and settings

### Repository ownership and human identities

| Evidence | Observed state | Security consequence |
|---|---|---|
| Repository metadata | Public repository, owned by personal account `yersonargotev`; default branch `main` | There is no organization/team/custom-role policy layer. |
| Collaborators | Exactly one: `yersonargotev`, role `admin`; permissions include admin, maintain, push, triage, and pull | One human identity can change settings, refs, releases, Actions, and collaborators. |
| Pending invitations | None | No additional invited human authority was found. |
| Merge settings | Merge commits, squash, and rebase are enabled; auto-merge and automatic head-branch deletion are disabled | Merge method is flexible, but none is gated by branch protection. |
| Current refs | One branch (`main`) and eight lightweight tags (`v0.1.0` through `v0.1.7`) | No automation-owned `sync/*` branch or open PR existed at audit time. |

The collaborator and repository observations come from GitHub's
[repository](https://api.github.com/repos/yersonargotev/packy),
[collaborator](https://api.github.com/repos/yersonargotev/packy/collaborators),
and [invitation](https://api.github.com/repos/yersonargotev/packy/invitations)
endpoints. The authenticated viewer was verified as `yersonargotev`, matching
the sole Admin.

### Branch, tag, merge, and commit protections

| Control | Observed state |
|---|---|
| Repository rulesets | None; the effective-rules query for `main` was also empty. |
| Classic `main` protection | Absent (`404 Branch not protected`); `main.protected` is `false`. |
| Required PR/reviews/checks | None. CI runs, but GitHub does not require it before a push or merge. |
| Admin enforcement / bypass policy | None. There is no protection to enforce or audit as bypass. |
| Force-push/deletion/create restrictions | None for `main` or tags. |
| Signed commits | Not required. The audited `main` head commit was unsigned. |
| Tag protection | No tag ruleset or legacy tag-protection configuration was found. All eight refs are lightweight tags pointing directly to commits. |

The latest seven release commits (`v0.1.1` through `v0.1.7`) currently have
valid GitHub commit verification; `v0.1.0` is unsigned. That historical fact is
useful provenance but is not an active control: an authorized writer can move,
replace, or delete the current tag refs because no tag rule protects them.
GitHub rulesets can target branches and tags and can restrict their creation,
updates, and deletion
([available rules](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets));
this audit found no such ruleset applied.

### GitHub Actions repository policy

| Setting | Observed state |
|---|---|
| Actions | Enabled. |
| Allowed actions | All actions and reusable workflows. |
| Full-SHA enforcement | Disabled. |
| Default `GITHUB_TOKEN` permission | Read-only. |
| Actions may approve PRs | Disabled. |
| Fork-PR approval | Required for first-time contributors. |
| Artifact/log retention | 90 days. |
| Active workflows | CI, Release, Synchronize pack source, and GitHub-managed `pages-build-deployment`. |

The read-only default is a useful backstop, but explicit job/workflow
permissions override it. Allowing all actions while not requiring full-SHA
pinning leaves mutable action tags usable. GitHub states that pinning an action
to a full-length commit SHA is the only immutable action reference and warns
that a compromised action can access job secrets or use the job's token
([secure use reference](https://docs.github.com/en/actions/reference/security/secure-use)).

## Mutation-capable workflows and delivery paths

### CI: validation only, not an integration gate

[`CI`](https://github.com/yersonargotev/packy/blob/fa522936d2f7428277729911db5982f4211f6044/.github/workflows/ci.yml)
runs on pull requests and pushes to `main`, grants only `contents: read`, and
runs `./scripts/validate-packy.sh`. It uses `actions/checkout@v5` and
`actions/setup-go@v6`, which are mutable tag references. CI cannot mutate the
repository with its declared token, but because `main` has no protection its
successful result is advisory rather than required.

### Release: repository releases plus an external Homebrew write

[`Release`](https://github.com/yersonargotev/packy/blob/fa522936d2f7428277729911db5982f4211f6044/.github/workflows/release.yml)
is triggered by either a pushed `v0.*.*` tag or `workflow_dispatch`. It grants
`contents: write` to the workflow and can:

1. create a GitHub Release for an existing `v0.x.y` tag;
2. upload or replace (`--clobber`) every release asset;
3. check out `yersonargotev/homebrew-tap` using `HOMEBREW_TAP_TOKEN`; and
4. commit and push `Formula/packy.rb` directly to that repository's `main`.

The only Actions repository secret is `HOMEBREW_TAP_TOKEN`, created on
2026-07-05. It is repository-scoped, not environment-scoped. There is no release
environment, required reviewer, deployment branch/tag policy, or prevent-admin-
bypass setting around the job. GitHub documents that repository secrets are
available to workflows in the repository, while environment secrets can be
limited to jobs behind environment protection and approval
([secret types](https://docs.github.com/en/code-security/reference/secret-security/secret-types),
[deployments and environments](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments)).

Release also uses mutable `actions/checkout@v5` twice and
`actions/setup-go@v6`. One checkout receives `HOMEBREW_TAP_TOKEN`; the
repository does not require full-SHA action references. Recent release runs
were tag-push runs initiated by `yersonargotev` and published by
`github-actions[bot]`.

Eight public releases exist. All are `draft: false`, `prerelease: false`, and
`immutable: false`; their assets are attributed to `github-actions[bot]`.
GitHub currently reports SHA-256 digest metadata for the `v0.1.7` assets, but
the workflow can still replace those assets and the associated tag is not
locked. GitHub's immutable-release control would prevent changing release
assets and moving or deleting the associated tag after publication and would
generate a release attestation
([immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases));
that control is not active here.

### Synchronize pack source: scoped write job, selectable workflow ref

[`Synchronize pack source`](https://github.com/yersonargotev/packy/blob/fa522936d2f7428277729911db5982f4211f6044/.github/workflows/sync-pack-source.yml)
is manual-only. It starts with `permissions: {}`; Inspect, Classify, and Validate
are read-only (Classify also has `models: read`); Publish alone receives
`contents: write` and `pull-requests: write`. Its third-party actions are pinned
to full commit SHAs, and checkout persists credentials only in Publish.

The default Publish implementation force-pushes with a lease to a stable
`sync/<source-id>` branch, creates or edits a draft PR targeting `main`, marks a
new PR ready, and finalizes managed PR metadata
([publication source](https://github.com/yersonargotev/packy/blob/fa522936d2f7428277729911db5982f4211f6044/internal/tools/syncpacksource/publish.go#L473-L571),
[push and PR creation](https://github.com/yersonargotev/packy/blob/fa522936d2f7428277729911db5982f4211f6044/internal/tools/syncpacksource/publish.go#L774-L829)).
It does not merge the PR in its intended implementation.

Recent successful and failed dispatches ran from
`feat/issue-88-admit-addy-manual-workflow`, not from `main`, proving that the
selected-ref path is active in practice. The intended code is careful about
stable-branch/PR ownership and compare-and-swap checks, but GitHub grants the
job token to the workflow code at the selected ref. Without a protected
environment or ref restriction, repository governance must treat dispatch
authority plus selectable workflow code as write authority, not merely as
permission to invoke the reviewed default implementation.

### GitHub Pages: automatic public delivery from unprotected `main`

Pages is enabled in legacy build mode, publishing the repository root from
`main` to <https://yersonargotev.github.io/packy/> with HTTPS enforced. GitHub's
managed `pages-build-deployment` workflow is active and recent runs were caused
by changes to `main`. The `github-pages` environment has no secrets, variables,
or reviewer rule; its custom branch allowlist contains `main`, `gh-pages`, and
the historical `feat/packy-atomic-cutover` branch, and `can_admins_bypass` is
true. No repository source file invokes the environment directly.

Pages therefore adds a delivery artifact whose content follows `main`. Its
branch allowlist narrows Pages deployments, but it does not protect `main`
itself or impose independent review on site changes.

## Credentials, apps, hooks, keys, and security features

### Credential and integration inventory visible to this audit

| Surface | Observed state |
|---|---|
| Actions repository secrets | `HOMEBREW_TAP_TOKEN` only; value not requested. |
| Actions repository variables | None. |
| `github-pages` environment secrets/variables | None. |
| Dependabot secrets | None. |
| Codespaces repository secrets | None. |
| Deploy keys | None. |
| Repository webhooks | None. |
| Pending collaborator invitations | None. |

`GITHUB_TOKEN`/`github.token` is not a stored secret: GitHub mints it per job
with the declared workflow/job permissions. In this repository it becomes a
mutation principal in Release and the Publish job of Synchronize pack source.
`github-actions[bot]` is the resulting actor visible on releases, assets,
automation commits, and PR operations.

Installed GitHub Apps could not be enumerated with the authenticated OAuth token:
GitHub's installation endpoints returned `401`/`403` because they require a
GitHub App-authorized token. Therefore “no installed GitHub Apps” is **not** an
audit finding. The empty webhook, deploy-key, and secret-name inventories were
successfully returned by their repository endpoints.

### Security feature state

| Feature | Observed state |
|---|---|
| Secret scanning | Enabled; zero alerts visible. |
| Secret scanning push protection | Enabled. |
| Non-provider patterns / validity checks | Disabled. |
| Dependabot alerts | Disabled (API explicitly returned that state). |
| Dependabot security updates | Disabled. |
| Code scanning | No analysis found. |
| Private vulnerability reporting | Disabled. |
| Repository security policy | No `.github/SECURITY.md`, root `SECURITY.md`, or community-profile policy was found. |
| Dependabot version-update config | No `.github/dependabot.yml` was found. |

Secret scanning and push protection are active defenses. The dependency and
code-scanning gaps mean GitHub is not currently producing vulnerability alerts
for the Go dependency graph, opening security-update PRs, or recording code
analysis. GitHub documents that Dependabot alerts scan the default branch's
dependency graph and that security updates can open remediation PRs
([Dependabot alerts](https://docs.github.com/en/code-security/concepts/supply-chain-security/dependabot-alerts),
[security updates](https://docs.github.com/en/code-security/concepts/supply-chain-security/dependabot-security-updates)).

## Concrete gaps against the map

These are observed mismatches, not rollout decisions. Later map tickets must
choose the exact controls and sequence.

1. **Protected-PR integration is not enforced.** The map says every normal
   change goes through a protected PR and only an Integrator merges. Today the
   Admin can push directly to `main`; no PR, review, CI, conversation
   resolution, or issue-closing link is required.
2. **Break-glass-only bypass has no technical boundary or audit signal.** With
   no rule to bypass, an Admin's ordinary direct push and an emergency bypass
   are indistinguishable. The provisional independent-agent review for
   Admin-authored PRs is also not recorded or required by GitHub.
3. **Adding Maintain/Write authority now would broaden mutation paths beyond
   merge.** A future Integrator with write access could dispatch either
   mutation-capable workflow on a selectable ref. The map cannot safely add the
   second role before deciding ruleset, environment, workflow-ref, secret, and
   dispatch controls.
4. **Tags and published releases are mutable.** No tag rule prevents creation,
   movement, force update, or deletion; all published releases report
   `immutable: false`; the release workflow deliberately replaces assets.
5. **The Homebrew credential is broad in placement and exposure path.** A
   repository-scoped token capable of pushing another repository's `main` is
   consumed without an environment approval gate and by a job that runs
   mutable action tags. The API cannot reveal whether the token itself is a
   fine-grained PAT or its exact external scope, so least privilege is
   unproven.
6. **Action supply-chain policy is inconsistent.** Synchronize pack source pins
   actions to full SHAs, while CI and Release use tags; repository policy allows
   all actions and does not require SHA pinning. The risk is highest in Release,
   where both a write token and external secret are present.
7. **Pages follows an unprotected source branch.** The public site is rebuilt
   from `main`; its environment has branch filtering but no reviewer and allows
   Admin bypass.
8. **Security detection and intake are incomplete.** Secret scanning/push
   protection are active, but Dependabot alerts/security updates, code scanning,
   and private vulnerability reporting are disabled, and no security policy is
   published.
9. **Issue/label governance is policy-only.** With one Admin, the desired issue
   authority happens to be concentrated, but there is no trusted automation or
   check enforcing approved-issue linkage or the declared exception categories.
10. **App authority remains an explicit evidence gap.** Repository webhooks and
    deploy keys are empty, but installed GitHub Apps were not enumerable with
    the available credential and must be checked through an App-capable API
    token or the repository installation UI before the authority inventory is
    considered exhaustive.

## Existing controls worth preserving

- Repository-default `GITHUB_TOKEN` permissions are read-only and Actions
  cannot approve PRs.
- CI explicitly grants only `contents: read`.
- Synchronize pack source is manual-only, starts from no permissions, confines
  repository writes to Publish, pins actions to full SHAs, and uses guarded
  stable-branch/PR ownership logic rather than auto-merging.
- Secret scanning and push protection are enabled, with no current secret alert
  returned.
- There are no pending invitations, deploy keys, repository webhooks,
  Dependabot/Codespaces secrets, or environment secrets in the successful API
  inventories.
- Release assets include SHA-256 digest metadata and a `checksums.txt` asset,
  even though publication immutability/attestation is not enforced.

## Limitations

- This is a 2026-07-20 point-in-time observation; settings and principals can
  change independently of the repository commit.
- GitHub App installations were not enumerable with the current OAuth token.
  OAuth applications, user PATs, SSH keys, passkeys, account recovery, and the
  personal owner's organization/account security are outside repository APIs
  and were not audited.
- Secret APIs expose metadata, never values. The type, exact scopes, expiry,
  rotation practice, and owner of `HOMEBREW_TAP_TOKEN` remain unknown.
- The audit did not inspect the settings or collaborator/ruleset state of the
  external `yersonargotev/homebrew-tap`; it proves only that Packy's Release
  workflow contains a credentialed direct-push path to that repository.
- A `404` for vulnerability alerts was accompanied by GitHub's explicit
  “Vulnerability alerts are disabled” response. “No analysis found” for code
  scanning means no result set exists; it is not proof that the code has no
  vulnerabilities.
- No workflow was executed. Mutation behavior was established from workflow
  declarations, source code, permissions, and historical run/release metadata.

## Reproducible read-only evidence commands

The audit used `gh api` with `--jq` to retain only non-secret fields. The main
command families were:

```bash
# Identity, ownership, settings, collaborators, refs and protections
gh api user
gh api repos/yersonargotev/packy
gh api repos/yersonargotev/packy/collaborators --paginate
gh api repos/yersonargotev/packy/invitations
gh api repos/yersonargotev/packy/rulesets
gh api repos/yersonargotev/packy/rules/branches/main
gh api repos/yersonargotev/packy/branches/main
gh api repos/yersonargotev/packy/branches/main/protection
gh api repos/yersonargotev/packy/branches --paginate
gh api repos/yersonargotev/packy/tags --paginate
git ls-remote --heads --tags origin

# Actions policy, workflows, run actors, environments and metadata-only secrets
gh api repos/yersonargotev/packy/actions/permissions
gh api repos/yersonargotev/packy/actions/permissions/workflow
gh api repos/yersonargotev/packy/actions/permissions/fork-pr-contributor-approval
gh api repos/yersonargotev/packy/actions/permissions/artifact-and-log-retention
gh api repos/yersonargotev/packy/actions/workflows
gh api repos/yersonargotev/packy/actions/workflows/307623509/runs
gh api repos/yersonargotev/packy/actions/workflows/313949537/runs
gh api repos/yersonargotev/packy/environments
gh api repos/yersonargotev/packy/environments/github-pages
gh api repos/yersonargotev/packy/environments/github-pages/deployment-branch-policies
gh api repos/yersonargotev/packy/actions/secrets
gh api repos/yersonargotev/packy/actions/variables
gh api repos/yersonargotev/packy/environments/github-pages/secrets
gh api repos/yersonargotev/packy/environments/github-pages/variables
gh api repos/yersonargotev/packy/dependabot/secrets
gh api repos/yersonargotev/packy/codespaces/secrets

# Keys, hooks, releases, Pages and security features
gh api repos/yersonargotev/packy/keys
gh api repos/yersonargotev/packy/hooks
gh api repos/yersonargotev/packy/releases --paginate
gh api repos/yersonargotev/packy/releases/tags/v0.1.7
gh api repos/yersonargotev/packy/pages
gh api repos/yersonargotev/packy/dependabot/alerts
gh api repos/yersonargotev/packy/code-scanning/alerts
gh api repos/yersonargotev/packy/secret-scanning/alerts
gh api repos/yersonargotev/packy/private-vulnerability-reporting
```

Repository inspection used `git status`, `git rev-parse`, `find`, `rg`, `sed`,
and targeted reads of `.github/workflows/*.yml`, `docs/release.md`, and
`internal/tools/syncpacksource/publish.go`. Symbol discovery for the
synchronization publication path used the repository's CodeGraph index before
the targeted source read.
