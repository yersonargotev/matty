# Addy 1.1.0 promotion specification

## Purpose and authority

This is the decision-complete implementation handoff for promoting the exact
current Addy content to `addy@1.1.0`: one immutable manifest-v3 capability
Pack with complete Claude Code compatibility, unchanged Codex/OpenCode
behavior, durable `addy@1.0.0` history, and protected atomic publication in the
selectable catalog.

It assembles the decisions from
[Promote Addy to Claude Code and the selectable catalog](https://github.com/yersonargotev/packy/issues/192)
without reopening or weakening them. A linked resolution or prototype remains
authoritative where it contains a more detailed table or transcript than this
handoff.

Implementation is complete only when the original Addy 24-row matrix and the
new 14-row promotion matrix are green for the exact evaluated merge, the
package-installed real-Claude boundary passes, `./scripts/validate-packy.sh`
passes, and the protected promotion aggregate admits the exact atomic catalog
delta. A failing row is implementation work, never permission for a waiver.

This specification plans the work. It does not authorize this planning session
to change Addy bytes, run `sync/addy`, activate a Pack, write real operator
configuration, install or run upstream content, authenticate, call a model, or
publish a Packy release.

## Immutable decision inputs

| Decision | Contract carried into implementation |
| --- | --- |
| [Define Addy Claude skill and command projections](https://github.com/yersonargotev/packy/issues/193) | All 24 skills and eight logical commands become exclusive native Claude personal skills. Deterministic composite trees preserve source bytes, close dependencies, carry all seven references, translate TOML commands canonically, preserve `$ARGUMENTS`, and use explicit surface-local aliases. |
| [Define Addy Claude agent authority and readiness](https://github.com/yersonargotev/packy/issues/194) | Four exclusive user agents use deterministic generated frontmatter and exact source bodies, least-authority tool lists, explicit native/fallback/guarded authority outcomes, host-governed permissions, and tri-state readiness without turning optional authority into compatibility. |
| [Define Addy catalog, history, and lifecycle promotion](https://github.com/yersonargotev/packy/issues/195) | `1.1.0` is the sole selectable current v3 artifact; `1.0.0` remains immutable v2 history. Intents and aliases are surface-local. Activation, update, reconcile, deactivation, withdrawal, and recovery remain plan-bound, atomic, historical-artifact-aware, and fail closed. |
| [Prototype the Addy Claude catalog and activation experience](https://github.com/yersonargotev/packy/issues/196) | The approved operator transcript fixes discovery, show, activation, readiness, update/no-op, collision/alias, failure/remediation, and withdrawal behavior. A withdrawn known Pack is omitted from normal list output but remains addressable through `pack show`. |
| [Design the Addy Claude validation and publication gate](https://github.com/yersonargotev/packy/issues/197) | Fourteen additive blocking rows across six sequential gates preserve the original matrix, prove unchanged source/history and complete Claude behavior, require package-installed real-host safety, and bind protected PR/release publication to fresh canonical aggregate evidence. |

The approved concrete UX is
[Addy 1.1.0 Claude catalog and activation prototype](https://gist.github.com/yersonargotev/eb6dac0d835ea506bb87132618a7dd6c).
Implementations may improve formatting only when human and JSON facts, ordering,
terminology, exits, and remediation remain equivalent to that prototype.

Accepted architecture continues to govern ownership:

- [ADR 0002](../adr/0002-package-installed-source-model.md): the running
  binary and same-tag Installed Source form one package contract.
- [ADR 0005](../adr/0005-capability-pack-surface-adapter.md):
  `internal/capabilitypack` owns portable lifecycle policy; one complete
  `internal/claudecode.SurfaceAdapter` owns Claude translation and effects.
- [ADR 0006](../adr/0006-own-workstation-layout-by-domain.md): paths derive
  from injected workstation facts, never ambient operator configuration.
- [ADR 0007](../adr/0007-serialize-complete-bundle-transactions.md):
  `internal/bundletransaction` remains the only complete-bundle lock and
  publication recovery owner.
- [ADR 0014](../adr/0014-build-once-release-publication.md): release
  publication reuses one retained candidate and separates GitHub/Homebrew
  authority.
- [ADR 0015](../adr/0015-detect-governance-drift-without-repair-authority.md):
  fresh governance observation may block promotion or publication but cannot
  repair external state.

No new ADR is required. This promotion extends accepted seams. If
implementation discovers a contradiction, it must stop for an explicit
architecture decision rather than introduce a generic host framework, a
second catalog/history registry, a second bundle lock, or workflow-owned
domain policy.

## Scope and exclusions

In scope:

- one strict current `bundle/packs/addy/pack.json` v3 contract for `1.1.0`;
- immutable self-contained `1.0.0` and `1.1.0` history;
- current/list/show/update-route/withdrawn catalog metadata;
- native Claude projections for 24 skills, four agents, and eight commands;
- all seven inherited assets and the non-projecting MIT notice;
- surface-local intent, alias, lifecycle, ownership, readiness, recovery, and
  structured-output behavior;
- the 14-row promotion cohort, package-installed Claude smoke, aggregate
  evidence, required PR check, exact-tag replay, and release admission;
- operator and maintainer documentation needed to keep those claims exact.

Out of scope:

- synchronizing a newer `addyosmani/agent-skills` revision or changing any
  selected source byte, path, mode, dependency, exclusion, optional mode, or
  notice;
- changing Addy's existing Codex/OpenCode observable behavior;
- implicit Claude intent from listing, history, Packy update, another surface,
  or an existing `1.0.0` intent;
- historical-version selectors, reverse routes, automatic downgrade, or
  rewrite of a published version;
- browser/MCP installation, authentication, model calls, REPL/print sessions,
  upstream scripts/hooks/installers/tests, or implicit runtime permission;
- a generic projection compiler or another public surface abstraction;
- real operator activation or Packy release publication while implementing
  planning artifacts.

## Shared vocabulary and invariants

- **Portable identity** is `(addy, kind, logical-id)` and never changes when an
  effective alias changes.
- **Effective name** is the default Claude name or an explicitly approved
  surface-local alias sealed into intent and the current plan.
- **Compatibility complete** means every required Addy resource has its exact
  native Claude representation and its dependency closure is representable.
  Optional authority availability is not compatibility.
- **Configured** means every exact required projection and fingerprint is
  present without collision, ambiguity, drift, or missing dependency.
- **Authorized** means configured plus supported Claude version, applicable
  observed policy, and no observed higher-precedence replacement. Unobservable
  policy is `unknown`, not `yes`.
- **Usable** means authorized plus explicit current runtime evidence bound to
  resource identity/fingerprint, Addy version, Claude version, and policy.
- **Optional authority** is `available`, `unavailable`, or `unknown`, with the
  exact declared fallback or guarded fail-closed behavior. It is reported
  separately from readiness.
- **Withdrawal** makes a known current Pack non-selectable without deleting its
  identity, history, or existing intents.
- **Recovery** always starts from fresh observation and a newly sealed plan; it
  never replays historical effects.

Universal invariants:

1. Every preview and inspection is inert. It may resolve paths and bounded
   version facts but may not write, authenticate, invoke a model, start Claude,
   install packages, run upstream content, grant permission, spawn an agent,
   commit, deploy, or publish.
2. Required collisions, malformed sources, incomplete authority translation,
   unknown fields/tools, missing dependencies, ambiguous ownership, stale
   observations, or unexpected fingerprints block the complete surface plan
   before effects.
3. Equal bytes never authorize adoption, ownership transfer, overwrite, or
   cleanup.
4. Approval is typed, plan-bound, and effect-specific. `reversible-local`
   covers only enumerated local projections/ownership; exact last-contributor
   removal also requires `destructive-cleanup`.
5. An active intent is written only after every required projection applies
   and verifies. An interrupted attempted application is
   `recovery-required`, never partial success.
6. Human and JSON renderings consume the same domain result. `unknown` remains
   explicit; secrets, raw upstream bytes, credentials, and ambient real paths
   never enter canonical evidence.

## Exact catalog and immutable artifact contract

### Current manifest

The selectable artifact is exactly:

```text
schema_version: 3
id: addy
version: 1.1.0
surfaces: [claude, codex, opencode]
resources: 24 skills, 4 agents, 8 commands, 7 assets, 1 notice
```

It retains the exact portable `1.0.0` logical contract: IDs, source paths,
source repository and lock, selected files and modes, dependencies, provides,
requires, conflicts, exclusions, optional modes, and MIT notice. Every skill,
agent, and command has exactly one binding for each declared surface.
Codex/OpenCode binding names, invocation forms, modes, sharing, and observable
behavior remain unchanged.

Assets materialize only through consumer dependency closure. The notice is
display-only, non-projecting, and excluded from readiness. Any missing binding,
unknown field, unclosed dependency, invalid authority record, source-lock
change, or selected-byte drift blocks decoding or promotion.

### History, routes, and catalog visibility

Publication includes trusted self-contained artifacts for both versions:

- `bundle/history/addy/1.0.0/` contains the byte-identical v2 manifest and
  selected resource tree that defined accepted Codex/OpenCode behavior.
- `bundle/history/addy/1.1.0/` contains the exact v3 manifest and the same
  selected source bytes.
- Each `artifact.json` canonically records path, size, mode, SHA-256,
  per-resource digest, and aggregate digest.
- Trusted aggregate registration is explicit and immutable.

`pack list` advertises only selectable current `addy@1.1.0` with Claude, Codex,
and OpenCode. `pack show addy` reports current or withdrawn status, both exact
versions, the sole route `1.0.0 -> 1.1.0`, source identity, resource counts,
surface contracts, and surface-local intent facts. Fresh activation always
selects `1.1.0`; no command accepts a historical version selector.

Historical status, reconcile, deactivation, composition, and recovery resolve
the exact verified historical artifact without depending on current manifest
bytes, upstream availability, or current source validation.

Withdrawal removes Addy from normal list results and blocks fresh
activation/update. `pack show addy`, status, reconcile, recovery, and
deactivation remain available for known historical intents. Re-advertisement
requires byte-identical `1.1.0` and a fresh complete gate. A correction uses a
higher SemVer.

## Claude projection contract

### Skills and command-as-skill projections

The 24 skill IDs remain those in the current Addy inventory. Each default
target is `~/.claude/skills/<effective-name>` and invocation is
`/<effective-name>`.

Each skill and command uses a deterministic Packy-owned composite tree. It
contains the exact source tree plus the complete selected reference set under
`references/`:

- `accessibility-checklist.md`
- `definition-of-done.md`
- `observability-checklist.md`
- `orchestration-patterns.md`
- `performance-checklist.md`
- `security-checklist.md`
- `testing-patterns.md`

The eight logical commands are native personal skills:

| Command | Invocation | Required logical dependencies |
| --- | --- | --- |
| `build` | `/build` | `skill:using-agent-skills`, `asset:definition-of-done` |
| `code-simplify` | `/code-simplify` | `skill:using-agent-skills`, `asset:definition-of-done` |
| `plan` | `/plan` | `skill:using-agent-skills`, `asset:definition-of-done` |
| `review` | `/review` | `agent:code-reviewer`, `skill:using-agent-skills`, `asset:definition-of-done` |
| `ship` | `/ship` | `agent:code-reviewer`, `agent:security-auditor`, `agent:test-engineer`, `skill:using-agent-skills`, `asset:definition-of-done` |
| `spec` | `/spec` | `skill:using-agent-skills`, `asset:definition-of-done` |
| `test` | `/test` | `agent:test-engineer`, `skill:using-agent-skills`, `asset:definition-of-done` |
| `webperf` | `/webperf` | `agent:web-performance-auditor`, `skill:using-agent-skills`, `asset:definition-of-done` |

Command sources decode as strict UTF-8 TOML containing only the expected string
`description` and `prompt`. Generated `SKILL.md` has canonical YAML
frontmatter, an exact resolved dependency contract, a fixed verbatim
`$ARGUMENTS` preamble, and then the unchanged prompt string. Packy never
interpolates, tokenizes, executes, or rewrites command input or prompt bytes.

Skills and commands share one exclusive personal-skill namespace. An unresolved
collision blocks all 36 user-facing projections. Explicit resolution uses a
freshly checked alias, normally `addy-<logical-id>`, bound to the portable
identity and used by generated dependencies.

### Native agents and authority

The four native user-agent files are:

```text
~/.claude/agents/code-reviewer.md
~/.claude/agents/security-auditor.md
~/.claude/agents/test-engineer.md
~/.claude/agents/web-performance-auditor.md
```

Each source decodes as strict UTF-8 Markdown with exactly `name` and
`description` in source frontmatter. Generated frontmatter contains effective
name, exact description, exact sorted tool allowlist,
`permissionMode: default`, and the effective `using-agent-skills` dependency.
It adds no model, effort, memory, background, hooks, MCP, isolation, or initial
prompt. The generated dependency/authority contract precedes the unchanged
upstream body.

| Agent | Exact Claude tools |
| --- | --- |
| `code-reviewer` | `Bash`, `Glob`, `Grep`, `Read` |
| `security-auditor` | `Bash`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch` |
| `test-engineer` | `Bash`, `Edit`, `Glob`, `Grep`, `Read`, `Write` |
| `web-performance-auditor` | `Bash`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch` |

Tool presence is not permission. Effective Claude deny/ask/allow policy and
parent permission mode govern invocation. No subagent receives `Agent`, a
general `Skill` tool, arbitrary MCP tools, or tools outside its list.

Each portable authority has one canonical v3 record:

```text
portable
declarations[]  # sorted source tool/permission/optional-mode declarations
outcome          # native | fallback | guarded
claude_tools[]   # sorted unique; empty for fallback
fallback         # exact declared fallback or none
```

`permission_mode` is explicit. Coverage is exact: missing, duplicate, dangling,
unknown, unsorted, or contradictory declarations/tools/outcomes block the
manifest.

| Authority | Outcome and behavior |
| --- | --- |
| browser | `fallback`; install/adopt no browser or browser MCP; use static evidence and report metrics as not measured |
| network | native `WebFetch`/`WebSearch` only for security/web-performance agents; otherwise static fallback |
| process | native `Bash`; run only under effective host authorization, otherwise report exact commands |
| package-manager | native through `Bash` with report-only fallback; never an implicit grant |
| commit/deploy | `guarded` through `Bash`, fallback `none`; fail the requested branch before its first privileged effect when authority is absent/denied/unknown |
| subagent | main-session authority only; use exact installed personas or sequential exact-persona fallback; missing/drifted persona blocks invocation |

Optional-authority availability is disclosed in preview/status/invocation but
never changes compatibility or Pack readiness. A runtime denial after valid
activation uses the exact fallback or fails only the requested invocation
branch; it does not retroactively invalidate verified local activation.

### Ownership, application, and removal

Ownership for every projection records portable identity, effective name,
target path/type, canonical composite-tree target where applicable, Pack
version, expected definition/source/tree fingerprints, contributors, and
deletion authority.

Claude translation, strict source decoding, composite-tree generation,
frontmatter rendering, host inspection, and host application remain private
implementation in `internal/claudecode` behind the existing complete
`SurfaceAdapter`. Portable composition, intent, aliases, consent, blockers,
ownership policy, readiness aggregation, and recovery remain in
`internal/capabilitypack`.

Apply constructs and verifies the complete desired tree/documents before
changing host-visible paths. Replacement is allowed only for an exact target
matching recorded ownership. Local publication is coherent and verified;
obsolete composite trees are removed only when no contributor retains them.
Foreign, ambiguous, modified, higher-precedence, or unexpected content is
preserved and blocks mutation. Absence converges.

## Surface-local lifecycle and operator contract

Each intent owns exactly one Pack, surface, version, revision, and alias set.
One workstation may retain Codex `1.0.0`, OpenCode `1.1.0`, and Claude
`1.1.0` independently.

### Activation and update

Activation requires explicit `--surface claude|codex|opencode`. Preview seals
Pack/version, surface, intent revision, aliases, observations, fingerprints,
dependencies, preservation, authority outcomes/fallbacks, readiness
expectation, actions, and typed phase digests. `--dry-run` requests no approval
and mutates nothing.

Updating an active Codex/OpenCode `1.0.0` intent shows the exact historical
v2/current v3 observable diff and preserves only compatible aliases. It never
creates Claude intent. Version and affected definition/binding/name/fingerprint
changes invalidate affected usability evidence. `1.1.0 -> 1.1.0` is a verified
no-op with no approval, action, revision, alias, or evidence change.

### Reconcile, deactivation, and recovery

Reconcile targets the exact version and aliases in durable intent, never
catalog current. A converged plan needs no approval; repair uses
`reversible-local`.

Deactivation resolves the intended historical artifact, recomposes remaining
active Packs on that surface, preserves foreign/drifted/ambiguous content, and
removes only exact unchanged last-contributor projections after both required
consents. Intent becomes inactive only after verified cleanup.

Lifecycle journals retain completed, failed, and not-started actions plus exact
ownership/fingerprints/outcome. Repeating the originating verb performs fresh
inspection and produces a fresh plan that may converge, request approval, or
preserve residuals with explicit human action.

### Discovery, status, and remediation

Canonical human and JSON v2 output expose the same ordered facts:

- list/show: selection or withdrawal, exact history/routes, compatibility,
  counts, source identity, bindings, and per-surface intent;
- preview/result: all effective projections, dependencies, aliases/collisions,
  preservation, authority/fallbacks, expected readiness, fingerprints, typed
  actions, blockers, pending actions, and recovery;
- status: intent/attempt, per-resource ownership and health, precedence,
  configured/authorized/usable, optional-authority availability/fallbacks,
  evidence, blockers, and remediation;
- update/no-op/deactivation: exact old/new contract, retained/removed
  contributors, consents, revisions, evidence invalidation, and zero-effect
  convergence.

`pack status --require usable` emits the complete status document before its
nonzero gate error. Drift directs explicit reconcile. `recovery-required`
directs repeating the originating verb. No output suggests implicit adoption,
automatic alias execution, automatic downgrade, or historical-plan replay.

## Validation and protected publication

### Promotion rows and owning authority

`internal/addyacceptance` remains the single Addy-specific acceptance module.
It preserves the original 24 stable rows and adds the 14 canonical
`ADDY-CLAUDE-PROMOTION-ROW-XX` rows. It owns canonical fixtures, independent
base/head/tag/history reconstruction, stable row results, the
`addy-promotion-evidence.v1.json` aggregate validator, and deterministic
negative twins. It does not own GitHub, branch protection, release, or host
effect policy.

`scripts/validate-addy-acceptance.sh` and the GitHub workflows are adapters:
they invoke exact registered top-level tests, collect sanitized evidence, and
stop before a write when domain validation fails. They do not duplicate row
meaning.

The six sequential gates are:

1. source/content invariance, immutable `1.0.0`, and exact atomic delta;
2. strict v3 inventory, complete Claude projection/authority, and
   surface-local compatibility/intent;
3. deterministic discovery/lifecycle output;
4. plan/approval/apply/recovery atomicity and collision/ownership/removal
   safety;
5. package-installed parity and real Claude Addy smoke;
6. protected promotion PR and exact-tag release publication.

Every row has a stable fixture, owning package/test names, disposable roots,
canonical result/blocked diagnostic, exact permitted diff or zero-mutation
proof, one-fact negative twin, identical rerun, later-gate suppression proof,
and evidence hashes. Independent reconstruction never trusts candidate hashes
as their own oracle and proves `sync/addy` did not participate.

### Package-installed real-Claude boundary

The final host rows use one built binary and Installed Source prepared outside
the checkout from the exact clean commit/archive. They may not use `go run`,
development paths, untracked files, or direct fixture access.

PR smoke uses Claude `2.1.203`. Exact-tag release smoke uses the supported
floor and the recorded stable version on Darwin Intel and Apple Silicon. Claude
is acquired before the restricted boundary. Evidence records requested and
resolved versions, npm integrity, executable digest, exact argv/exits/process
log, and before/after manifests.

Sandboxed `HOME`, `XDG_CONFIG_HOME`, `CLAUDE_CONFIG_DIR`, state, package,
repository, and acquisition roots are mandatory. Provider credentials are
scrubbed; nonessential traffic and upstream execution are denied. No login,
authentication, REPL, `--print`, model call, or outside-root write is allowed.

### Aggregate, PR gate, and release gate

The sole admission document, `addy-promotion-evidence.v1.json`, binds:

- repository, PR, base, head, evaluated merge, or exact tag;
- workflow identity/digest and matrix version;
- all 14 unique row results and evidence hashes;
- independent source/history/diff proof;
- package candidate identity and Claude identities; and
- atomicity/process/filesystem manifests.

Missing, duplicate, unknown, stale, ambiguous, cross-commit, cross-workflow, or
cross-run evidence is rejected.

The always-emitted strict required check is named
`Addy 1.1.0 promotion gate`. Unrelated changes return canonical
`not_applicable`; a promotion change cannot. Its protected rollout is explicit:

1. land the always-emitted check and its stable app/context identity;
2. update Packy's protected expected-state contract through a protected PR;
3. have a human Owner add the live required check to protected `main`; and
4. collect fresh read-only governance evidence proving exact app/context
   identity, strict/up-to-date evaluation, admin enforcement, and evaluated
   merge binding.

ADR 0015 automation never repairs this external state. Missing, stale,
unclassified, or divergent rollout evidence blocks the promotion PR. The check
and verified governance configuration exist before catalog advertisement, so
there is no interval in which partial Addy publication is selectable.

The evaluated-merge aggregate is generated and retained by the promotion
workflow; it is not committed into the candidate whose identity it proves.
Candidate-controlled trusted artifact-registration metadata may be committed,
but it never substitutes for fresh out-of-tree row evidence.

PR and effect-free post-merge `main` replay evidence is retained for 90 days.
Release publication accepts only evidence produced in the same exact-tag
workflow run. The exact protected-main tag reuses one retained candidate and
passes repository validation, package parity, complete Addy evidence, and four
floor/stable architecture smokes. The final aggregate and protected
environment approval precede GitHub Release, OIDC, or Homebrew authority.

Recovery is a higher SemVer fix or temporary catalog withdrawal. It is never a
rewrite, partial release, automatic downgrade, or bypass.

## Architecture ownership and impact guide

This guide fixes seams; it does not require every listed file to change.

- `internal/capabilitypack/catalog.go`, `history.go`, `activation.go`,
  `composition.go`, `status.go`, lifecycle structured-result files, and their
  focused tests own manifest decoding, catalog/history/routes/withdrawal,
  portable composition, intents, aliases, consent, lifecycle, readiness
  aggregation, and recovery.
- `internal/claudecode/surface.go`, `capabilitypack_ownership.go`, host
  observation/application files, and focused tests own strict Addy source
  translation, composite skill trees, command documents, agent documents,
  Claude namespaces, fingerprints, and host effects.
- `internal/localprojection` remains the neutral staged local-write primitive;
  it must not acquire Addy or lifecycle policy.
- `internal/cli/pack.go` and CLI tests remain adapters for flags, human
  rendering, JSON selection, and exits. No catalog or readiness policy moves
  there.
- `internal/addyacceptance` owns both exact Addy matrices, fixtures,
  independent invariance proof, and aggregate validation.
- `internal/claudesmoke`, `scripts/run-claude-smoke.sh`, and package/release
  tests own sandboxed package-installed host evidence without redefining Addy
  compatibility.
- `internal/release`, `scripts/verify-release-evidence.sh`, and
  `.github/workflows/release.yml` consume the Addy aggregate inside ADR 0014's
  one-candidate publication contract.
- `internal/bundletransaction` remains the unchanged repository-local lock for
  complete bundle observations; `internal/packsync` alone owns local
  transactional replacement/recovery during synchronization. Neither module
  publishes the promotion or owns protected-merge atomicity.
- `bundle/packs/addy/pack.json`, `bundle/history/addy/**`, current catalog
  metadata, trusted aggregate registration, schemas, canonical JSON v2
  fixtures, docs, and checksums form one reviewed promotion delta.
- `bundle/sources/addy.lock.json`, `bundle/sources.json`, and every selected
  Addy source byte are invariance inputs and must not change.
- The exact reviewed Git tree and protected commit/merge are the repository
  catalog-publication boundary. Package-installed readers consume one exact
  Installed Source commit; synchronization's local bundle swap remains
  separate from this promotion.

## Ordered protected tracer-bullet plan

Each slice is a protected pull request with focused tests and
`./scripts/validate-packy.sh` green. Slices before the final promotion use
synthetic/canonical fixtures and keep Addy non-selectable at `1.0.0`; no
intermediate merge may advertise partial `1.1.0`.

| Slice | Smallest independently verifiable outcome | Blocked by |
| --- | --- | --- |
| A. Establish Addy promotion evidence foundations | Strict v3 authority records, immutable-history fixtures, the 14 stable promotion row identities, independent invariance/diff inputs, canonical aggregate validation, and an always-emitted non-publishing promotion check exist without changing current Addy catalog selection. | None |
| B. Build Addy Claude skill and command projections | One canonical Addy workflow proves exact composite skills, command translation, all seven references, aliases/collisions, ownership, apply/verify/remove, and negative twins behind the complete Claude adapter. | A |
| C. Build Addy Claude agent authority and readiness | Extending the same central adapter path after B, one canonical persona proves generated agents, exact tools, complete native/fallback/guarded translation, precedence, optional-authority reporting, readiness/evidence invalidation, ownership, and negative twins. | B |
| D. Add immutable Addy history and surface-local lifecycle semantics | Synthetic exact `1.0.0`/`1.1.0` artifacts prove current/history/show/list/withdrawn behavior, the sole update route, independent intents/aliases, update/no-op/reconcile/deactivation/recovery, and atomic bundle observation without advertising Addy. | A |
| E. Stabilize the complete Addy operator and structured-output contract | The complete 36-projection Addy fixture passes activation, update, status, collision/alias, no-op, failure/recovery, deactivation, withdrawal, remediation, canonical human/JSON v2, redaction, and exit contracts across all surfaces. | C, D |
| F. Qualify the Addy promotion and real-host gate harness | Both matrix runners, sequential suppression, fault injection, aggregate generation/validation, package-parity harness, exact-floor/four-release-smoke harnesses, retention, and release consumers pass synthetic/pre-candidate qualification. Rows requiring the production evaluated merge remain unsatisfied and catalog publication remains absent. | E |
| G. Roll out and verify the Addy promotion required check | A protected PR adds the check to expected governance state; a human Owner configures the exact live required check; fresh read-only evidence proves exact app/context identity, strict/up-to-date and admin enforcement, with no catalog change. | F |
| H. Publish the protected atomic Addy 1.1.0 catalog promotion | One exact-delta PR supplies the evaluated candidate inputs: current v3 artifact, immutable history, current selection, route, trusted registration metadata, and final docs/schemas/fixtures/checksums. Its CI run builds/package-installs that evaluated merge, executes both matrices and real-host rows, generates and retains the fresh aggregate out of tree, admits the merge, and is followed by effect-free `main` replay. | G |

Native blocking edges:

```text
A -> B
A -> D
B -> C
C -> E
D -> E
E -> F
F -> G
G -> H
```

B and D are the first parallel implementation frontier after A. C is
serialized after B because skills, commands, and agents currently share the
central Claude adapter inspection/application/ownership path. E owns the first
all-36 atomic integration. H is the only slice allowed to make Addy selectable,
and its bundle/catalog/history changes are inseparable. G is a human-owned
external-governance task; automation observes but never repairs it.

The approved implementation issues are:

1. [Establish Addy promotion evidence foundations](https://github.com/yersonargotev/packy/issues/201)
2. [Build Addy Claude skill and command projections](https://github.com/yersonargotev/packy/issues/202)
3. [Build Addy Claude agent authority and readiness](https://github.com/yersonargotev/packy/issues/203)
4. [Add immutable Addy history and surface-local lifecycle semantics](https://github.com/yersonargotev/packy/issues/204)
5. [Stabilize the complete Addy operator and structured-output contract](https://github.com/yersonargotev/packy/issues/205)
6. [Qualify the Addy promotion and real-host gate harness](https://github.com/yersonargotev/packy/issues/206)
7. [Roll out and verify the Addy promotion required check](https://github.com/yersonargotev/packy/issues/207)
8. [Publish the protected atomic Addy 1.1.0 catalog promotion](https://github.com/yersonargotev/packy/issues/208)

## Decision-closure audit

No product, architecture, safety, compatibility, naming, lifecycle, history,
intent, authority, readiness, UX, validation, packaging, publication,
recovery, or delivery-order decision remains before slice A begins.

The following are execution observations, not open decisions:

- exact file lists and aggregate hashes are derived from the implemented
  candidate by the independent oracle;
- optional host authority may truthfully remain unavailable or unknown;
- runtime usability may truthfully remain `unknown` until explicit current
  evidence exists;
- branch-protection or governance drift blocks the affected boundary until
  fresh classified evidence exists;
- a failed exact-floor or release-architecture smoke is implementation or
  environment work, not a reason to narrow the matrix; and
- a correction after publication uses a new monotonic version or withdrawal.

If implementation discovers a fact that contradicts a fixed decision, it must
stop and open a new decision ticket. It must not reinterpret this document,
rewrite `addy@1.0.0` or `addy@1.1.0`, silently omit a resource, weaken a gate,
or broaden an existing module seam.
