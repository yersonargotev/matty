# Packy governance rollout readiness

- Issue: [#168](https://github.com/yersonargotev/packy/issues/168)
- Baseline commit: `7255ffb48a58d82c5e1de408f272e57a68928f51`
- REST/GraphQL/Git observation: `2026-07-22T03:24:25Z`
- Owner UI observation: `2026-07-22T03:26:25Z`–`2026-07-22T03:47:49Z`
- Observer: `yersonargotev` through a recently authenticated Owner session

## Evidence boundary

This is the canonical readiness record for Packy's governance rollout. It is a
read-only point-in-time observation. No enforcement, credential, workflow,
release, tag, protected ref, environment, App permission, or repository setting
was changed while capturing it.

The filtered machine snapshot is
[`evidence/issue-168/baseline.json`](evidence/issue-168/baseline.json). The
snapshot and this record are bound by
[`evidence/issue-168/SHA256SUMS`](evidence/issue-168/SHA256SUMS).

### Sanitization

- Record credential names, type/scope assertions, and timestamps only; never
  values, tokens, recovery codes, emails, phone numbers, device names, session
  data, or screenshots of recovery material.
- Record App names, repository selection, permission categories, and pending
  expansions; never authentication artifacts.
- Preserve command output only after projecting explicit non-secret fields.
- Treat `unverified`, `absent`, and API authorization failures as evidence, not
  as proof of a safe empty state.

## Readiness result

**State: REPOSITORY PREREQUISITES AUTHORIZED.** The Owner authorizes only the
independently reversible repository work in #169–#171. No check or control may
become required, and no credential or environment may be created, moved, or
consumed, while its gate below remains unresolved.

- [x] The Owner attests that at least two independent recovery paths were
  inspected, recovery material is stored outside GitHub, and the documented
  non-destructive restoration tabletop is viable.
- [ ] Before credential migration, the Owner records the type, destination allowlist, scopes, expiry,
  rotation owner, and revocation path of `HOMEBREW_TAP_TOKEN` without recording
  its value or a provider-side secret identifier.
- [x] A recently authenticated human Owner session was preserved during capture.
- [x] All pending App permission expansions remain unapproved.
- [x] The Owner signs the repository-prerequisite stage-entry statement at the
  end of this document.

The remaining credential gap blocks #173 and any credential-consuming
promotion. The absent/unproven checks block enforcement. Neither gap is treated
as implied approval merely because repository prerequisite work may proceed.

## Verified baseline

| Surface | Current state | Independent evidence | Consequence |
| --- | --- | --- | --- |
| Ownership | Public personal repository; `yersonargotev` is the sole collaborator and Admin; no pending invitations | REST collaborator inventory, GraphQL `viewerPermission=ADMIN`, and Owner Settings UI agree | One human identity holds routine and break-glass authority. |
| Merge policy | Merge, squash, and rebase enabled; auto-merge and automatic head deletion disabled | REST, GraphQL, and Owner General Settings UI agree | Merge methods exist without an enforcement gate. |
| `main` | One branch at `7255ffb48a58d82c5e1de408f272e57a68928f51`; unprotected | Branch REST and `git ls-remote` agree; classic protection returns `404` | Direct update, force-push, and deletion are not prevented. |
| Tags | `v0.1.0`–`v0.1.7`; no tag rule | Tag REST and `git ls-remote` agree | Version refs remain mutable. |
| Rules | No repository rulesets and no effective rule for `main` | Ruleset and effective-rules endpoints both return empty sets | No PR, review, check, signed-commit, or bypass rule is enforced. |
| Actions policy | Enabled; all Actions allowed; full-SHA enforcement disabled; default token read-only; workflow tokens may approve PR reviews; first-time-contributor fork approval; 90-day retention | Actions policy endpoints and Owner Actions Settings UI agree | `can_approve_pull_request_reviews` drifted from `false` on 2026-07-20 to `true`; later work must use live state. |
| Workflows | CI, Claude stable canary, Release, Synchronize pack source, and GitHub-managed Pages are active | Workflow REST matches four checked-in definitions plus the managed Pages workflow | Release and synchronization contain scoped write paths; canary can write issues. |
| Environments | Only `github-pages`; Admin bypass enabled; custom branches `main`, `gh-pages`, and stale `feat/packy-atomic-cutover`; no environment secrets or reviewers | Environment/branch-policy endpoints and Owner Environment Settings UI agree | No protected `release` or `homebrew` authority boundary exists. |
| Credentials | Repository secret `HOMEBREW_TAP_TOKEN`; no repository variables, Dependabot secrets, Codespaces secrets, or environment secrets | Secret-name API and Release workflow reference agree | Token value was not queried; type and external scope remain unverified. |
| Releases | Eight published releases, five assets each, all mutable | Release REST and `gh release list` agree | Existing assets and tags lack immutable-release enforcement. |
| Pages | Legacy build from `main:/`; public and HTTPS-enforced | Pages endpoint and Owner Pages Settings UI agree | Pages follows unprotected `main`. |
| Security | Secret scanning and push protection enabled; zero visible secret alerts; dependency graph, Dependabot alerts/updates, and automated fixes disabled; no CodeQL analysis; private vulnerability reporting disabled | Security feature endpoints and Owner Advanced Security Settings UI agree on enablement | Three future security checks do not yet exist. |
| Keys/hooks | No deploy keys or repository webhooks | Each REST inventory agrees with its independent Owner Settings UI | Installed Apps and workflow tokens remain separate authority surfaces. |

### Check identity and source registry

The five stable contexts are namespaced in policy. GitHub's check-runs API
reports the job name, while GitHub displays the workflow-qualified name.

| Required identity after qualification | Current observation | Expected source | Promotion rule |
| --- | --- | --- | --- |
| `CI / Validate Packy-owned code` | `Validate Packy-owned code` succeeded at the baseline head | GitHub Actions, app `15368`, slug `github-actions` | Re-prove at current head before requiring it. |
| `CI / Claude 2.1.203 package smoke` | Present but skipped on the baseline `main` run | GitHub Actions, app `15368`, slug `github-actions` | Require successful PR evidence; a skipped result is not qualification. |
| `Governance / Validate authorization` | Absent | Intended GitHub Actions app `15368`; unproven | Create advisory-only, then prove its exact name/source. |
| `Security / CodeQL` | Absent; API reports no analysis | Intended GitHub Actions app `15368`; unproven | Create advisory-only, then prove its exact name/source. |
| `Security / Dependency review` | Absent | Intended GitHub Actions app `15368`; unproven | Create advisory-only, then prove its exact name/source. |

A new or renamed check is never made required in the same change that creates
it. A source mismatch, unavailable context, skipped required scenario, or name
drift stops promotion.

### Installed Apps

The current OAuth token cannot enumerate installations. The authenticated Owner
Settings inventory and each installation's configuration view agree on eleven
Apps, all configured for **all current and future repositories** owned by the
account.

| App | Effective permission summary | Pending expansion |
| --- | --- | --- |
| AWS Amplify (us-east-1) | Write `amplify.yml`; read code/metadata; read-write checks, PRs, hooks | Remove check-run/check-suite access; unapproved |
| AWS Amplify (us-west-2) | Write `amplify.yml`; read code/metadata; read-write checks, PRs, hooks | None |
| Claude | Read Actions/checks/metadata; read-write code, discussions, issues, PRs, workflows | Members, webhooks, commit statuses, and higher Actions/checks access; unapproved |
| Cloudflare Workers and Pages | Read metadata; read-write administration, checks, code, deployments, PRs | None |
| Google AI Studio | Read statuses/issues/metadata; read-write administration, code, PRs | Read-write Actions/workflows; unapproved |
| Google Cloud Build | Read code/issues/metadata/PRs; read-write checks/statuses | None |
| Google Labs Jules | Read metadata; read-write Actions, code, issues, PRs, workflows | Read administration, artifacts, checks, deployments, email addresses, members, and code-scanning alerts; unapproved |
| Linear | Read checks/metadata; read-write issues/PRs | None |
| Railway App | Read metadata; read-write Actions, administration, checks, code, statuses, deployments, PRs, workflows | None |
| Render | Read Dependabot alerts/code/metadata; read-write Actions, checks, statuses, deployments, environments, issues, PRs, hooks, workflows | None |
| Vercel | Read Actions/metadata; read-write administration, checks, code, statuses, deployments, issues, PRs, hooks, workflows | None |

No pending expansion is approved by this record. A later App audit must justify
continued installation and minimum selected-repository access without a
destructive probe.

### Owner access and recovery

Owner/Admin access was verified by REST, GraphQL, and an authenticated Settings
session. The UI showed one verified email, a configured password, one passkey,
enabled 2FA, a configured authenticator, two GitHub Mobile devices, and recovery
codes previously viewed. GitHub warned that recovery codes had not been
downloaded or printed in the last year.

Those facts do not prove independent recovery or off-account storage. The Owner
must attest only the yes/no facts in the readiness gate. Never add recovery
material or identifying details to this repository, an issue, a PR, or CI.

The tabletop path is: preserve an authenticated session; confirm a first and an
independent second recovery method; confirm off-account recovery material;
identify the safe account-restoration route; and stop before logging out,
revoking a method, rotating a credential, or simulating actual account loss.

## Actor/path matrix

This matrix defines the scenarios that later shadow and enforcement stages must
prove. `Allow` means the narrowly named path; everything else is a denial case.

| Actor | Allowed path | Must be denied | Evidence and safe negative test |
| --- | --- | --- | --- |
| Owner/Admin | Propose/review/integrate through the protected PR path; execute separately approved settings changes; invoke break-glass only for a catalogued incident | Routine direct protected-ref update, self-review where independence is required, secret disclosure, silent bypass | Disposable PR for normal flow; settings/API query; break-glass remains tabletop-only. |
| Explicitly delegated agent | Create an issue-bound branch/PR and merge only under explicit end-to-end delegation using the Owner session | Self-review, control mutation, credential access, publication approval, break-glass | Disposable issue branch with allowed PR operation and denied settings/secret probes. |
| Fork contributor | Fork, propose a PR, and run secretless read-only checks after policy approval | Repository write, secrets, protected environments, merge, publication | Fork fixture proves checkout/tests and absence of secrets/write token. |
| Dependabot | Propose dependency updates and run secretless advisory checks | Merge, arbitrary branch/ref writes, environments, secrets, release | Dependabot fixture PR; negative metadata checks for token/environment access. |
| Synchronization automation | Inspect/classify/validate read-only; Publish writes only its owned `sync/*` branch and PR | `main` update, merge, release, environment secret access, unrelated branch overwrite | Sandbox source proposal plus wrong-branch/identity denial. |
| Deterministic issue/canary automation | Read repository and create/update only its canonical issue evidence | Code/ref write, PR merge, environment/secret access, release | Disposable canary result and denied contents-write probe. |
| Release automation | After approval, consume one proved build and publish through protected release/Homebrew environments | Unprotected ref, rebuild after proof, asset clobber, version reuse, Pages/control mutation | Sandbox release rehearsal; Packy dry-run stops before real publication. |
| GitHub Pages | Deploy checked-in Pages content from its approved protected source | General repository write, secrets, non-approved source branch | Disposable source-policy denial and environment/API verification. |
| Installed GitHub Apps | Perform only reviewed, minimum installation-specific operations | Unapproved expansion, unexpected repo selection, control/secret/publication access outside declared need | Owner UI permission review and non-destructive event/check observation; never approve as a test. |

Every scenario records actor, UTC time, ref and commit, workflow-definition SHA,
effective permissions, accessible environments/secret names, result, and denial
proof. A head update invalidates prior scenario evidence.

## Independently reversible rollout units

Each unit restores only its own object to the captured baseline. Repository file
rollback is a protected `git revert`; settings rollback uses the normal Owner
path and then repeats API, independent-view, and safe negative verification.

| Unit | Later issue/stage | Captured prior state | Object-only rollback | Verification and safe negative test |
| --- | --- | --- | --- | --- |
| CODEOWNERS and SECURITY policy | #169 / prerequisites | Absent | Protected revert of those files | File/ownership validation; non-owner fixture cannot satisfy ownership. |
| Validate/go test/govulncheck context | #169 | CI validation exists; full required identity not enforced | Revert its workflow unit | Current-head success plus intentionally failing fixture. |
| Claude floor context | #169 | Context exists but is advisory and may skip | Revert its workflow unit | Exact-floor PR success plus unsupported/missing-floor denial. |
| Governance authorization context | #169 | Absent | Revert policy, fixtures, and workflow together | Approved/denied metadata fixtures; no issue mutation. |
| CodeQL context | #169 | No analysis | Revert CodeQL workflow/config | Analysis appears from expected App; failing fixture blocks only shadow qualification. |
| Dependency-review context | #169 | Absent; Dependabot alerts disabled | Revert dependency-review workflow/config | Allowed dependency PR plus vulnerable-change fixture. |
| Repository Actions policy | #170 | All Actions allowed; SHA enforcement off; default read; PR approval on | Restore only captured Actions settings | API/UI agreement; mutable-action fixture rejected. |
| CI/canary hardening | #170 | CI read-only; canary read plus issues-write; mutable Actions present | Revert affected workflow only | Fork/Owner/canary matrix and denied write/secret probes. |
| Synchronization hardening | #170 | Manual selectable ref; publish has contents/PR write | Revert synchronization workflow only | Owned `sync/*` proposal succeeds; wrong ref/branch denied. |
| Release hardening | #170 | Workflow-wide contents-write; mutable Actions; repository secret | Revert release trust-boundary change only; never restore broader authority | Unprotected ref and secretless unauthorized job denied. |
| Build-once release redesign | #171 | Build/validation artifacts exist; publication can clobber assets | Revert release-flow redesign only | Artifact/hash/SBOM/attestation parity and same-version recovery rehearsal. |
| `release` environment | #173 | Absent | Delete empty environment or restore captured object | Unauthorized deployment denied before any secret is added. |
| `homebrew` environment and destination credential | #173 | Absent; repository secret scope unknown | Disable consumer, revoke new token, restore empty environment; never restore broad secret | Destination-only write succeeds; unrelated repository write denied. |
| `github-pages` environment | #173 | Admin bypass on; three custom branches; no secrets/reviewer | Restore captured Pages environment object | Approved protected source succeeds; stale/unapproved source denied. |
| Merge settings | #174 | Three merge methods; auto-merge/head deletion off | Restore only captured merge flags | REST/UI agreement and disposable PR method checks. |
| Main ruleset | #174 | No rules/protection | Restore captured absence only through prepared break-glass if rule blocks its repair | Owner/fork/automation PR matrix; disposable direct-update denial. |
| Version-tag ruleset | #175 | No tag rule | Restore captured absence for the tag object only | Sandbox tag create/update/delete matrix; do not probe real release tags destructively. |
| Immutable-release setting | #175 | Eight releases mutable; repository immutability off | Disable only before a publication cutover if safely reversible; never mutate old releases as a test | Sandbox draft/publish/retry and mutation/deletion denial. |
| Drift detection and cadence | #176 | No canonical automation | Revert drift workflow/policy only | Seeded read-only drift opens evidence; automation cannot self-correct. |

## Stop and rollback protocol

Stop promotion immediately on any unexpected authorization or denial,
unavailable/renamed/wrong-source check, unplanned permission/secret/bypass,
API disagreement with an independent view/query, failed safe negative test,
stale head/workflow identity, or repair-lockout risk.

Freeze the current stage, retain sanitized evidence, and restore one object at a
time in reverse change order to the last verified state. Repeat the primary
query, independent query/view, and negative test after each restoration. Never
remove all protections, restore broad credentials, use mutable references, or
weaken unrelated controls as a shortcut. If a control deadlocks its own repair,
use the prepared break-glass path; if complete restoration fails, stop
non-essential operations and retain the most restrictive safe state.

## Reproduction commands

Run from a clean checkout. Keep the `--jq` projections; do not persist raw
responses from secret, installation, credential, or recovery surfaces.

```bash
repo=yersonargotev/packy
sha=$(gh api repos/$repo/commits/main --jq .sha)

gh api user --jq '{login}'
gh api repos/$repo --jq \
  '{owner:.owner.login,owner_type:.owner.type,visibility,default_branch,archived,allow_merge_commit,allow_squash_merge,allow_rebase_merge,allow_auto_merge,delete_branch_on_merge,web_commit_signoff_required,security_and_analysis,permissions}'
gh api graphql -f query='query { repository(owner:"yersonargotev", name:"packy") { owner { login } visibility defaultBranchRef { name target { ... on Commit { oid } } } viewerPermission mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed autoMergeAllowed deleteBranchOnMerge } }'
gh api repos/$repo/collaborators --paginate \
  --jq '[.[]|{login,role_name,permissions}]'
gh api repos/$repo/invitations --jq '[.[]|{login:.invitee.login,permissions}]'

gh api repos/$repo/rulesets --jq '[.[]|{id,name,target,enforcement,conditions,rules}]'
gh api repos/$repo/rules/branches/main --jq \
  '[.[]|{type,source_type,source,parameters}]'
gh api repos/$repo/branches --paginate \
  --jq '[.[]|{name,protected,sha:.commit.sha}]'
gh api repos/$repo/tags --paginate --jq '[.[]|{name,sha:.commit.sha}]'
git ls-remote --heads --tags origin

gh api repos/$repo/actions/permissions
gh api repos/$repo/actions/permissions/workflow
gh api repos/$repo/actions/permissions/fork-pr-contributor-approval
gh api repos/$repo/actions/permissions/artifact-and-log-retention
gh api repos/$repo/actions/workflows \
  --jq '[.workflows[]|{id,name,path,state}]'
gh api repos/$repo/commits/$sha/check-runs \
  --jq '[.check_runs[]|{name,status,conclusion,app:{id:.app.id,slug:.app.slug,name:.app.name}}]'

gh api repos/$repo/environments \
  --jq '[.environments[]|{name,protection_rules,deployment_branch_policy,can_admins_bypass}]'
gh api repos/$repo/environments/github-pages/deployment-branch-policies \
  --jq '[.branch_policies[]|{name,type}]'
gh api repos/$repo/actions/secrets \
  --jq '{total_count,secrets:[.secrets[]|{name,created_at,updated_at}]}'
gh api repos/$repo/actions/variables --jq '{total_count,variables:[.variables[]|{name}]}'
gh api repos/$repo/dependabot/secrets --jq '{total_count,secrets:[.secrets[]|{name}]}'
gh api repos/$repo/codespaces/secrets --jq '{total_count,secrets:[.secrets[]|{name}]}'

gh api repos/$repo/releases --paginate \
  --jq '[.[]|{tag_name,draft,prerelease,immutable,published_at,author:.author.login,asset_count:(.assets|length)}]'
gh release list --repo $repo --limit 100
gh api repos/$repo/pages \
  --jq '{status,build_type,source,public,https_enforced}'
gh api repos/$repo/keys --jq '[.[]|{title,verified,read_only,created_at}]'
gh api repos/$repo/hooks --jq '[.[]|{name,active,events,created_at}]'
```

Installed Apps and account recovery require Owner-authenticated UI inspection
because the available OAuth token cannot enumerate installations and repository
APIs cannot prove recovery. Record only the sanitized fields defined above.

## Owner stage-entry sign-off

- Owner: `yersonargotev`
- Decision: **REPOSITORY PREREQUISITES ONLY**
- UTC time: `2026-07-22T03:40:53Z`
- Durable evidence: explicit confirmation in the issue-delivery conversation
  and this committed record

The Owner sign-off statement is:

> I verified the sanitized baseline, independent recovery and off-account
> recovery-material assertions, and the non-destructive restoration tabletop.
> Pending App expansions remain unapproved. I authorize only the independently
> reversible repository-prerequisite issues #169–#171; no control is authorized
> for enforcement, and credential/environment promotion remains blocked until
> `HOMEBREW_TAP_TOKEN` metadata is verified.

This sign-off does not authorize #172–#176, enforcement, or any credential,
environment, release, tag, protected-ref, or repository-setting mutation except
the independently reversible repository Actions policy prerequisite scoped to
#170.
