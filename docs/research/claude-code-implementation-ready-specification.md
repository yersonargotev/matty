# Claude Code implementation-ready specification

## Purpose and authority

This specification is the implementation handoff for making Claude Code a
first-class third Packy CLI surface. It assembles the decisions made by the
Wayfinder map
[Specify first-class Claude Code support](https://github.com/yersonargotev/packy/issues/112)
without reopening or weakening them. When this document is shorter than a
linked resolution or research asset, that linked artifact remains
authoritative.

Implementation is complete only when every acceptance row in this document is
green, `./scripts/validate-packy.sh` passes, both required real-Claude smoke
variants pass, and the documentation and release evidence agree with the code.
A failing row is implementation work, not permission for an exception.

This specification plans the work. It does not authorize this planning session
to install Claude Code, write real user configuration, authenticate, call a
model, activate a Pack, publish an artifact, or release Packy.

## Immutable decision inputs

| Decision | Contract carried into implementation |
| --- | --- |
| [Inventory Claude Code's official global integration surface](https://github.com/yersonargotev/packy/issues/113) | Use personal skills, one marked global instruction block, CLI-managed user MCP, and selected typed command hooks. Require Claude Code 2.1.203+ for shared skill-directory symlinks. Do not use Claude plugins initially. |
| [Map a third CLI surface across Packy's architecture](https://github.com/yersonargotev/packy/issues/114) | Extend the existing host seam. Add `internal/claudecode`; keep classic lifecycle, setup health, capability-pack, and CLI ownership in their current deep modules. Do not create a generic host framework. |
| [Decide the Claude Code host integration contract](https://github.com/yersonargotev/packy/issues/115) | Separate inert observation, authorized application, and runtime evidence. Own exact fragments, preserve foreign authority, use official user-scoped MCP commands, and never write `~/.claude.json`. |
| [Decide Claude Code classic lifecycle and health behavior](https://github.com/yersonargotev/packy/issues/116) | Claude is desired classic intent. Missing or old Claude is pending; collisions block without recovery; attempted effect failures require recovery; uninstall preserves residual ownership; doctor is inert and tiered. |
| [Decide Claude Code capability-pack behavior and exclusions](https://github.com/yersonargotev/packy/issues/117) | New Packs use manifest v3 with an explicit Claude binding or exclusion per runtime resource. Compatibility and readiness are independent. Command hooks are typed and consented. |
| [Prototype the Claude Code classic operator journey](https://github.com/yersonargotev/packy/issues/118) | Preserve six distinct outcomes: pending prerequisite, verified migration, inert health, preserved collision, fresh-plan recovery, and residual-safe uninstall. |
| [Prototype Claude Code pack activation and readiness](https://github.com/yersonargotev/packy/issues/119) | Show compatibility, intent, readiness, evidence, consent, preservation, and cleanup separately. Update invalidates affected runtime evidence. |
| [Decide Claude Code compatibility and validation gates](https://github.com/yersonargotev/packy/issues/120) | Use one 2.1.203+ rule, owner-layered tests, a permanent regression corpus, exact-floor PR smoke, moving-stable release smoke, synchronized docs/packaging, and fail-closed publication. |

Accepted ADRs continue to govern ownership:

- [ADR 0002](../adr/0002-package-installed-source-model.md): the running
  binary and same-tag Installed Source are one release contract.
- [ADR 0003](../adr/0003-core-lifecycle-deep-module.md): classic planning,
  state, application, verification, recovery, and ownership stay in
  `internal/corelifecycle`; CLI remains an adapter.
- [ADR 0004](../adr/0004-setup-health-deep-module.md): diagnostic meaning and
  ordering stay in `internal/setuphealth`; host observations are detached and
  read-only.
- [ADR 0005](../adr/0005-capability-pack-surface-adapter.md): capability-pack
  owns lifecycle meaning while one complete adapter owns each host translation.
- [ADR 0006](../adr/0006-own-workstation-layout-by-domain.md): paths derive
  from injected workstation facts, never ambient HOME or XDG state.
- [ADR 0011](../adr/0011-publish-versioned-pack-source-schema-suite.md): new
  manifest contracts and their fixtures are versioned, immutable, and complete.

No new ADR is required: this plan extends those accepted seams. If
implementation discovers a contradiction with an accepted ADR, it must stop
and request an explicit architecture decision rather than adding a wrapper,
second registry, or implicit exception.

## Scope and exclusions

The implementation covers user-global Claude Code support on Packy's existing
Darwin architectures and acquisition mechanisms. It covers classic install,
update, uninstall, doctor, skills, global instructions, Engram MCP setup,
capability Packs, typed command hooks, ownership, recovery, documentation,
structured output, package-installed smoke, and release gates.

The following are explicitly excluded:

- Linux, Windows, or a new Packy installer or artifact shape;
- installing or upgrading the Claude Code executable;
- authentication, REPL or print/model invocation, paid API use, or model calls;
- repository-local `CLAUDE.md`, `.claude/`, settings, hooks, agents, skills, or
  MCP configuration;
- direct writes to `~/.claude.json`;
- adoption or migration of foreign Claude configuration;
- Claude Desktop, Claude web, Anthropic API/SDK, or custom server integration;
- Claude plugins, marketplaces, caches, or plugin lifecycle;
- opaque JSON injection, prompt/agent/http/MCP-tool hooks, or automatic
  translation of generic lifecycle resources;
- automatic Claude compatibility for historical manifests or accepted Packs;
- runtime usability claims inferred from filesystem correctness alone.

## Shared vocabulary and invariants

- **CLI surface**: one host projection target: `codex`, `opencode`, or
  `claude`.
- **Desired classic intent**: the surfaces Packy should converge. It is durable.
- **Configured**: every exact required projection is present. It is observed.
- **Authorized**: configured plus supported version and observable policy/tool
  permission. It is observed.
- **Usable**: authorized plus an explicit current runtime loading, connection,
  or firing signal. It is observed and invalidatable.
- **Compatibility**: whether a Pack's declared Claude contract is `complete`,
  `degraded`, or `blocked` before readiness is considered.
- **Projection ownership**: Packy's exact fragment identity, observed
  fingerprint, contributors, and deletion authority. Equal bytes alone never
  transfer ownership.
- **Pending prerequisite**: work cannot yet use a user-managed executable or
  host feature; no attempted Packy effect failed.
- **Blocker**: an observed collision, invalid shared document, stale plan, or
  mandatory exclusion that prevents safe application; it is not recovery.
- **Recovery-required**: an attempted effect failed after Packy could not prove
  the complete intended prior or desired state.
- **Uninstall-incomplete**: residual ownership remains but no attempted cleanup
  failed.

Every inspect and dry-run path is inert. It may read named files, inspect
symlinks, resolve `claude`, and execute only bounded `claude --version`. It may
not write, authenticate, start a Claude session, invoke a model, or use
`claude mcp list/get` because those commands may start approved servers.

Every apply path uses fresh evidence sealed into a plan. Shared files are
reread immediately before publication. Stale evidence executes no unstarted
effects. A failed plan is never replayed; repeating the originating command
performs fresh inspection and requires fresh approval where consent applies.

## Compatibility authority

`internal/claudecode` owns one exported stable compatibility authority:

```go
const MinimumSupportedVersion = "2.1.203"

func ObserveVersion(ctx context.Context, executable string, runner Runner) VersionObservation
func ClassifyVersion(VersionObservation) Compatibility
```

`Compatibility` distinguishes supported stable, missing, below-floor,
prerelease, unreadable, failed, and timed-out observations. Only a stable
semantic version greater than or equal to 2.1.203 is supported; there is no
upper bound. Every other classification is a pending prerequisite with exact
remediation unless another independent blocker or failed effect exists.

Classic lifecycle, setup health, capability-pack, tests, docs, smoke scripts,
and release evidence consume this authority. No duplicate literal or comparator
is permitted. Packy never writes Claude's own minimum-version setting.

## `internal/claudecode` host module

### Layout and dependencies

`claudecode.NewCanonicalLayout(home)` returns an immutable user-global layout:

```text
~/.claude/
~/.claude/skills/
~/.claude/agents/
~/.claude/CLAUDE.md
~/.claude/settings.json
```

The constructor receives the resolved home directory. Tests and smoke pass a
disposable home and `CLAUDE_CONFIG_DIR`; production composition supplies the
workstation snapshot. The module never calls `os.UserHomeDir` or derives policy
from an ambient test runner home.

Executable discovery and execution use injected interfaces:

```go
type LookPath func(string) (string, error)

type Runner interface {
    Run(context.Context, Command) Result
}
```

`Command` contains executable, argv, environment additions, timeout, and a
stable redacted description. Results retain exit/error facts but never place
MCP environment values in errors, plans, output, state, or logs.

### Detached observations

The module returns values, not policy decisions:

- `VersionObservation` for executable discovery and `claude --version`;
- `SkillObservation` for path type, target, source, and tree fingerprint;
- `InstructionObservation` for document revision, marker cardinality,
  contribution fingerprints, and foreign-content preservation facts;
- `AgentObservation` for exact file identity and fingerprint;
- `HookObservation` for settings parseability, canonical matching entries,
  disabled or shadowed policy, and entry fingerprint;
- `MCPObservation` for the minimum named user entry statically readable from
  `~/.claude.json`, including exact redacted definition identity;
- `SetupObservation` aggregating those detached facts for setup health; and
- `RuntimeEvidence` only when an explicit caller supplies a separately obtained
  loading, connection, or firing signal.

The mixed `~/.claude.json` store is a minimum named read surface only. Failure
to read or classify it is a per-entry observation error. It is never a write
target or a source from which Packy adopts ownership.

### Exact projection actions

The host module validates and applies only these sealed action kinds:

| Kind | Exact effect |
| --- | --- |
| `claude-skill-link` | Create, replace, or remove one owned directory symlink beneath `skills/`; the complete source tree remains in the Installed Source. |
| `claude-instruction-contribution` | Merge or remove one deterministic contribution inside one outer Packy block in global `CLAUDE.md`. |
| `claude-agent-file` | Atomically publish or remove one exact owned Markdown file beneath `agents/`. |
| `claude-command-hook` | Merge or remove one canonical typed command-hook entry in valid `settings.json`. |
| `claude-user-mcp` | Execute one official `claude mcp add/remove ... --scope user` operation and statically reobserve the named entry. |

Before the first effect the module validates the complete batch for duplicate
identities, overlapping exclusive paths, invalid markers, invalid JSON,
noncanonical hooks, missing executables for required CLI effects, and stale
document revisions. Filesystem writes stage siblings and publish atomically
where the storage permits. MCP effects are individually journaled and cannot
be represented as part of the filesystem transaction.

The module exposes narrow merge/validate/apply primitives to classic lifecycle
and the accepted complete capability-pack adapter methods:

```go
InspectSurface(context.Context, capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error)
ApplyProjections(context.Context, []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError
```

It never constructs a classic or Pack lifecycle plan and never owns consent,
diagnostic severity, state publication, recovery classification, or CLI output.

Classic and capability-pack retain their separate authoritative state files,
but the composition root supplies the adapter with a read-only
`OwnershipSnapshot` assembled from both owners before planning. The snapshot is
not a third registry: each record keeps its original state owner and contributor
identity. All Claude mutations acquire one host-effect lock beneath Packy's
resolved state root, reread both state files and the target immediately before
the first effect, and reject stale ownership. An owner removing its contribution
may remove shared bytes only when the fresh composite snapshot proves it is the
last contributor. If either state is missing, corrupt, stale, or ambiguous,
cleanup preserves the projection and reports a blocker.

### Ownership identities

- Skill identity: surface, projection ID, path, symlink type, resolved target,
  expected source, and source-tree fingerprint.
- Instruction identity: global document, one outer Packy marker pair,
  contributor ID (`classic` or `pack:<pack>:<resource>`), and normalized exact
  contribution fingerprint.
- Agent identity: surface, projection ID, path, and file fingerprint.
- Hook identity: event, matcher, canonical command and arguments, timeout,
  blocking/failure behavior, authorities, and entry fingerprint.
- MCP identity: scope `user`, server name, command, ordered arguments, sorted
  environment keys, and a non-rendered fingerprint of the canonical complete
  environment definition.

Only a fragment that still matches recorded ownership can be updated or
removed. Missing is converged for removal. Changed, duplicate, foreign, or
ambiguous fragments are preserved. Shared instruction contributions and other
declared shared projections remain until the last contributor leaves.

## Classic lifecycle and state schema v2

### State wire contract

`internal/corelifecycle` raises `SchemaVersion` from 1 to 2. Version 2 removes
`configured_surfaces` and writes `desired_surfaces`, canonically ordered as
`codex`, `opencode`, `claude`. It retains current path and created-container
ownership and adds exact Claude projection and attempt evidence:

```json
{
  "schema_version": 2,
  "packy_version": "<running version>",
  "desired_surfaces": ["codex", "opencode", "claude"],
  "managed_skills": [],
  "claude_ownership": [
    {
      "id": "<stable projection id>",
      "kind": "skill|instruction|agent|hook|mcp",
      "target": "<path or user MCP name>",
      "fingerprint": "sha256:<hex>",
      "contributors": ["classic"],
      "source_path": "<skill source only>",
      "link_target": "<skill link only>",
      "command": "<hook or MCP only>",
      "args": [],
      "environment_keys": [],
      "environment_fingerprint": "<MCP only, never rendered>",
      "deletion_authorized": true
    }
  ],
  "paths": {},
  "last_install_check": "<timestamp when present>",
  "created_containers": [],
  "install_status": "confirmed|recovery-required|uninstall-incomplete",
  "latest_attempt": {
    "operation": "install|update|uninstall",
    "outcome": "verified|blocked|partially-applied|rolled-back|recovery-required|uninstall-incomplete",
    "completed_effects": ["<stable effect id>"],
    "failed_effect": "<stable effect id or empty>",
    "not_started_effects": ["<stable effect id>"]
  }
}
```

`partially-applied` is used only when verified effects and an independent
blocker coexist without a failed effect.

`rolled-back` records a failed local Claude effect whose exact prior state was
restored. It returns nonzero without setting recovery-required; a legacy v1
state remains authoritative instead of being migrated by that failed attempt.

Fields in a projection that do not apply to its kind are omitted. Arrays are
non-null and sorted where they are sets. Raw environment values are forbidden.
Live Claude version, current compatibility, readiness, policy, and runtime
evidence are never persisted as authority.

The v1 decoder remains exact. A valid v1 state with the historical
`configured_surfaces=[codex,opencode]` is **legacy provenance**, not corrupt or
unhealthy. `packy update` is the canonical migration. It derives the v2 desired
intent, performs fresh three-surface planning, applies and verifies safe work,
then atomically publishes v2. The v1 file remains authoritative if migration
does not verify. Idempotent install may converge the same migration but reports
that update is canonical. Unknown schemas fail closed.

Legacy migration is deliberately stricter than an already-v2 reconciliation:
if inspection finds any blocker, migration executes no new Claude effect. This
prevents an applied projection from existing without v2 ownership. Independent
safe partial convergence after a known blocker is allowed only when a v2 state
can remain authoritative and atomically record every verified new ownership
fact. A runtime failure during legacy migration follows normal rollback and
recovery rules; if exact rollback succeeds v1 remains authoritative, otherwise
minimal recovery ownership is published in v2 before returning the error.

### Install and update

Fresh install and update desire all three surfaces. Action order is:

1. inspect all surfaces and current state;
2. classify prerequisites, collisions, and stale evidence;
3. seal the complete plan and intended state transition;
4. apply and verify reversible local projections;
5. execute and statically verify each journaled Claude MCP effect;
6. publish state only after the attempted contract is verified.

Missing, old, prerelease, unreadable, or timed-out Claude leaves safe inert
filesystem work applicable, keeps Claude-dependent MCP work pending, converges
Codex/OpenCode, exits zero with warnings, and recommends installing stable
Claude Code 2.1.203+ followed by `packy update`. Pending MCP ownership is not
recorded.

A foreign collision or invalid shared document preserves the foreign artifact.
Independent safe classic actions may converge, but the overall result is
`blocked` or `partially-applied`, exits nonzero, and does not set recovery when
no effect failed.

A local failure that restores the exact prior state exits nonzero without
recovery. Failed restoration, or an MCP failure after committed verified local
effects, records exact completed/failed/not-started effects and sets
recovery-required. Repeating the verb performs fresh inspection and planning.

### Uninstall

Uninstall removes only exact recorded fragments. It continues independent safe
cleanup, preserves drifted or foreign fragments, and retains state until every
residual owner and pending removal is gone. Unavailable Claude permits local
cleanup but leaves MCP residual ownership and returns nonzero
`uninstall-incomplete` without recovery when no MCP removal was attempted. A
failed attempted removal is recovery-required. Packy never removes credentials,
Engram memory, foreign configuration, or a shared container it cannot prove it
created and emptied.

### Dry-run and exit contract

Install, update, and uninstall dry-run reports global classification, desired
surfaces, ordered actions, preserved/skipped fragments, blockers, pending
prerequisites, consent where applicable, and planned state transition. It
performs no mutation and never reveals MCP environment values.

| Outcome | Exit |
| --- | --- |
| `converged`, `applied`, `applied-with-pending-prerequisite` | zero |
| `blocked`, `blocked-dry-run`, `partially-applied`, `rolled-back`, `recovery-required`, `uninstall-incomplete` | nonzero |

## Setup health contract

`internal/setuphealth` consumes `claudecode.SetupObservation` and appends these
stable checks in this exact order after the existing common and host checks:

1. `claude-binary`
2. `claude-version`
3. `claude-skills`
4. `claude-instructions`
5. `claude-hooks`
6. `claude-mcp`
7. `claude-readiness`

Severity rules are exact:

- WARN: executable missing; version below 2.1.203; prerelease, parse, timeout,
  or observation failure; valid legacy v1 state awaiting migration; runtime
  usability unknown.
- FAIL: recovery-required or uninstall-incomplete ownership that should be
  present; missing/drifted recorded ownership; blocking collision; unreadable
  or invalid desired shared document; observable policy disabling a desired
  integration.
- PASS: the named static prerequisite or exact ownership/authorization fact is
  positively observed.

Checks do not collapse into a host-module health status. Warnings alone exit
zero; any failure exits nonzero. Human and JSON reports contain the same check
names, order, severity, remediation, summary counts, and overall status.
Doctor never starts an MCP server or runtime session.

## Manifest schema v3 and catalog contract

### Version preservation and first Pack versions

Manifest v1 and v2 decoders and historical files remain byte-exact and never
infer Claude support. Manifest v3 is the only schema that may declare the
`claude` surface. The first v3 catalog versions are:

- <code>ma&#116;ty</code> **3.0.0**: Claude compatibility `complete`; and
- `engram` **2.0.0**: Claude compatibility `degraded` but activatable.

`addy` 1.0.0 remains its exact v2 Codex/OpenCode contract. No accepted source
or historical activation gains Claude intent implicitly.

Before replacing a current catalog entry, preserve today's exact
<code>ma&#116;ty</code> 2.0.0
and `engram` 1.0.0 manifests and selected artifacts in immutable history with
their normal artifact metadata. The supported routes are
<code>ma&#116;ty 2.0.0 -&gt; 3.0.0</code> and
<code>engram 1.0.0 -&gt; 2.0.0</code>. Existing Codex/OpenCode activations remain
pinned to their recorded historical version and surface intent until an
explicit Pack update. Updating them may select the v3 version for those existing
surfaces, but never adds Claude intent implicitly; Claude activation is a
separate explicit surface choice. Deactivation of any historical intent must
continue to resolve its exact historical contract.

### V3 wire additions

V3 retains the v2 top-level Pack and resource fields, adds a required sorted,
non-null top-level `surfaces` array, and adds `claude` to the surface enum. V1
and v2 continue to receive their surfaces from their immutable catalog entry;
only v3 decodes this field from the manifest. Each v3 runtime resource
(`skill`, `instruction`, `mcp_server`, `lifecycle`, `agent`, `command`) contains
required non-null `bindings` and `surface_exclusions` arrays and must declare
exactly one entry across those two arrays for every top-level surface. `asset`
inherits through consumers and `notice` has both arrays empty.

A v3 surface exclusion is:

```json
{
  "surface": "claude",
  "mode": "optional|mandatory",
  "code": "<stable-kebab-case-code>",
  "reason": "<operator-visible explanation>"
}
```

An optional exclusion makes compatibility degraded. A mandatory exclusion, or
an excluded dependency of a mandatory resource, makes compatibility blocked
and the plan non-applicable.

The v2 binding fields remain required where meaningful:
`surface`, `projection`, `name`, `invocation`, `mode`, `degradation`, and
`sharing`. V3 adds these optional discriminated objects, legal only for the
matching projection:

```json
{
  "agent_authority": {
    "tools": [{"portable": "<id>", "claude": "<documented native id>"}],
    "permissions": [{"portable": "<id>", "claude": "<documented native id>"}]
  },
  "hook": {
    "event": "<documented Claude event>",
    "matcher": "<explicit matcher or empty only when event permits>",
    "command": "<canonical executable>",
    "args": [],
    "timeout_seconds": 0,
    "blocking": true,
    "failure": "block|warn",
    "authorities": []
  }
}
```

`projection` admits `skill`, `instruction`, `mcp_server`, `agent`, and
`command_hook` for Claude. Unknown fields, projections, events, failures,
authorities, duplicate surface outcomes, unsorted producer sets, missing
translations, and binding/exclusion gaps fail closed. A generic lifecycle
resource is never translated without a typed `command_hook` binding. Hook
`timeout_seconds` must be positive; only `type: command` is supported.

### Claude resource mapping

| Resource | Claude projection |
| --- | --- |
| `skill` | Complete source-tree symlink at `~/.claude/skills/<name>`; minimum version applies. |
| `instruction` | Deterministic pack/resource contribution inside the shared outer Packy block in global `CLAUDE.md`. |
| `mcp_server` | Official `claude mcp ... --scope user`; identity includes name, command, args, and environment. |
| `command` | Native personal skill, never legacy `.claude/commands`; `/name` invocation and `$ARGUMENTS` semantics are preserved. |
| `agent` | User-global `~/.claude/agents/<name>.md` only with explicit documented tool and permission translations. |
| `lifecycle` | Only an explicit typed command hook. The current opaque `lifecycle:engram-memory` is an optional exclusion. |
| `asset` | Materialized only inside the dependency closure of a compatible skill, command, or agent. |
| `notice` | Display/attribution metadata only; no projection or readiness effect. |

Skills and commands share Claude's personal-skill namespace. Agents, MCP names,
and hook identities have separate namespaces. A projection may be shared only
when all contributors declare sharing and canonical content is identical;
otherwise an explicit alias is required or composition blocks. Foreign and
higher-precedence authority is preserved and reported.

### Initial Pack contracts

<code>ma&#116;ty</code> 3.0.0 declares native Claude bindings for every skill and instruction,
has no Claude exclusion, and is compatibility-complete. Its runtime usability
may remain unknown after successful projection.

`engram` 2.0.0 declares a native instruction contribution and exact user MCP
binding. It declares `lifecycle:engram-memory` as an optional Claude exclusion
with a stable code explaining that generic lifecycle translation is
unsupported. It does not execute `engram setup claude-code`. The Pack is
degraded but activatable; its instruction and MCP still require their normal
consent and evidence.

## Capability-pack lifecycle contract

`internal/capabilitypack` adds `SurfaceClaude`, registers exactly one complete
Claude adapter, and retains ownership of composition, aliases, blockers,
consent, intent, journaling, recovery, stale-plan rejection, verification, and
readiness normalization.

Compatibility is computed before application:

- `complete`: every required runtime resource has a native Claude binding and
  there is no declared degradation or exclusion;
- `degraded`: every mandatory resource remains coherent, but at least one
  accepted deliberately degraded binding or independent optional exclusion
  exists;
- `blocked`: a mandatory outcome is absent, excluded, collided, stale, or
  otherwise non-applicable.

Preview shows compatibility, every binding/exclusion, exact projections,
preserved artifacts, blockers, expected readiness, pending evidence, and
plan-bound consent. A typed command hook requires executable/external consent;
last-contributor hook or MCP cleanup requires destructive-cleanup consent.
Environment values are always redacted. Any preview blocker executes zero
effects.

Readiness is the minimum across included resources:

- skills, commands, agents, and instructions require an explicit loading signal
  for usable;
- MCP requires explicit current connection evidence;
- hooks require explicit current firing evidence;
- assets inherit their consumer; notices and exclusions do not participate;
- a version, projection, definition, policy, or Pack-version change invalidates
  the affected usability evidence.

`pack show` owns declared bindings, degradations, and exclusions. Preview owns
planned compatibility, actions, consent, and preservation. `pack status [pack]
--surface claude` owns intent, projections, ownership health, blockers,
compatibility, readiness, evidence, and pending actions. `--require usable`
exits nonzero until every required usable signal is freshly known true.

Deactivation recomputes remaining composition and removes only exact unchanged
last-contributor projections. It preserves foreign text, other contributors,
changed or ambiguous artifacts, credentials, Engram memory, and all external
data. Unavailable Claude leaves user MCP residual ownership until official
removal can run.

## CLI and structured-output contract

`internal/cli` adds only composition wiring, the `claude` surface spelling,
help/examples, rendering, and exit mapping. It must not implement merge,
compatibility, readiness, recovery, or version policy.

All affected JSON reports increment to schema version 2 rather than silently
adding fields to version 1:

- classic install/update/uninstall preview and result add `desired_surfaces`,
  `pending_prerequisites`, `preserved`, `blockers`, `recovery`, and the planned
  or committed state transition;
- setup-health adds the seven stable Claude checks without exposing filesystem
  secrets;
- Pack show/preview/status add `compatibility`, `bindings`, `exclusions`,
  redacted effects, and readiness evidence/pending actions.

JSON arrays that represent sets are sorted; result/action arrays retain
execution order. Optional readiness remains the existing explicit
known/unknown representation. Environment values, authentication material,
and raw mixed-store content are forbidden in human output, JSON, errors, and
fixtures. Human output is tested by stable labels, ordering, essential facts,
and remediation rather than whole-transcript snapshots.

## Validation and permanent evidence

### Owner-layered automated matrix

The following rows are blocking and live with their owning module:

| Owner | Required rows |
| --- | --- |
| `internal/claudecode` | floor-1/floor/floor+1, prerelease and malformed versions; timeout/missing binary; inert inspection; skill/instruction/agent/hook/MCP projection; exact fingerprints; duplicate markers; invalid JSON; foreign collisions; higher-precedence/disabled policy; redaction; idempotence; stale shared documents; exact cleanup; proof that observation never calls effectful MCP commands. |
| `internal/corelifecycle` | fresh supported install; missing/old pending install; v1-to-v2 dry-run and verified migration; collision partial convergence without recovery; local rollback success/failure; MCP failure after local commit; fresh-plan retry; drift; residual-safe uninstall; unavailable-Claude uninstall; attempted MCP removal failure; repeated convergence and state-publication atomicity. |
| `internal/setuphealth` | exact check names/order; every WARN/FAIL/PASS boundary; legacy migration warning; inertness; summary and exit behavior. |
| `internal/capabilitypack` | v1/v2 exact preservation; v3 strict decoding; binding/exclusion completeness; complete/degraded/blocked compatibility; skill/command namespace collision; sharing and aliases; typed-hook validation and consent; <code>ma&#116;ty</code> 3.0.0 and engram 2.0.0 contracts; readiness minima; evidence invalidation; stale plan; cleanup and recovery. |
| `internal/cli` | composition wiring; `claude` parsing/help; human field order; JSON v2 schemas; redaction; stable exit mapping; no duplicated semantic matrix. |
| Catalog/history/docs | immutable historical fixtures; exact new Pack versions; no implicit Addy Claude support; current floor and three-surface claims agree everywhere. |

The two approved prototypes become permanent executable fixtures. Foreign
content is compared byte for byte before and after every relevant action.
Negative fixtures include unreadable documents, invalid JSON, duplicate
markers, changed symlinks/fragments, same-name/different-definition MCP,
managed-policy blockers, timeouts, and stale plans. Tests never manufacture a
usable observation.

### Real-Claude smoke

Every pull request runs a package-installed smoke against exact Claude Code
2.1.203. A scheduled canary and every release run against the exact current
stable version, recording the resolved version and digest. A moving-stable
failure opens compatibility work and blocks releases but does not retroactively
fail unrelated pull requests.

Both variants:

1. acquire Claude before restricting the execution boundary;
2. isolate `HOME`, `XDG_CONFIG_HOME`, and `CLAUDE_CONFIG_DIR` in disposable
   roots;
3. remove credentials and provider variables, disable updates and
   nonessential traffic, and install an inert deterministic local MCP server;
4. run only bounded version and controlled user-scoped MCP operations;
5. run package-installed Packy version, init, install dry-run/apply, doctor,
   update dry-run/apply, uninstall dry-run/apply, and final doctor;
6. never invoke REPL, print/model mode, login, authentication, or a model;
7. assert exact projections, redaction, foreign preservation, expected exits,
   residual-safe cleanup, and no write outside the sandbox; and
8. retain durable Packy tag/SHA, OS/architecture, Claude requested/resolved
   version/digest, commands, exits, and before/after sandbox manifests.

The exact-floor smoke blocks pull requests and releases. Release validation
also runs moving stable against the corresponding Darwin artifact on both Intel
and Apple Silicon.

### Validation authority and packaging

`./scripts/validate-packy.sh` remains the repository authority and includes
every new package, schema/fixture, structured-output, documentation, smoke
contract, and allowlist. `go test ./...` stays green. Focused tests may run
during implementation, but passing a subset is never completion evidence.

Artifact names, supported platforms, checksums, and Homebrew dependencies do
not change. The formula tests only `packy --version`. The binary carries the
adapter and compatibility policy; the same-tag Installed Source carries v3
manifests, resources, schemas, fixtures, and docs. Package smoke runs outside
the checkout and proves alignment.

No release asset or tap change may be published until the exact tag commit has:

1. complete repository validation;
2. all four Packy artifacts and verified checksums;
3. green v3/history/docs/JSON validation;
4. exact-floor and recorded-current-stable package smoke;
5. both Darwin architectures;
6. durable sandbox and version/digest evidence;
7. proof of no credential, authentication, model call, secret exposure, or
   outside-sandbox write; and
8. exact parity among tag, Installed Source, binaries, checksums, formula, and
   release notes.

Build/validation and publication remain separate. Missing, failed, stale, or
ambiguous evidence blocks publication without waiver or partial release.

## Documentation contract

- `docs/claude-code.md`: canonical guide for prerequisite, layout, projections,
  exclusions, compatibility, readiness, migration, preservation, recovery,
  cleanup, and the no-auth/no-model evidence boundary.
- `README.md`: three-surface golden-path summary and 2.1.203+ prerequisite,
  linking the guide.
- `docs/product/packy-v0.md` and `docs/roadmap.md`: three-surface product promise.
- `docs/capability-packs.md`: v3 bindings/exclusions, compatibility, consent,
  and readiness.
- `docs/structured-output.md`: exact JSON v2 reports and Claude checks.
- `docs/release.md`: dual smoke matrix and fail-closed publication gates.
- first supporting release notes: version floor, state migration, new Pack
  versions, degraded Engram lifecycle exclusion, and limitations.

Documentation validation rejects stale two-surface claims and any version-floor
literal that disagrees with `claudecode.MinimumSupportedVersion`.

## Ordered tracer-bullet delivery plan

The smallest dependency-aware implementation route is:

1. **Establish manifest v3 and Claude catalog foundations.** Add strict v3
   decoding, `SurfaceClaude`, explicit outcomes, typed bindings, immutable
   current/history fixtures, and supported update routes while preserving v1/v2
   exactly. Keep current catalog entries non-Claude until an adapter tracer can
   validate them.
2. **Build the Claude Code host adapter foundation.** Add layout, compatibility,
   inert observations, exact merge/fingerprint primitives, sealed actions,
   redaction, and injected execution without lifecycle policy.
3. **Carry Claude through classic lifecycle and state migration.** Implement
   state v2, pending prerequisites, ordered local/MCP application, recovery,
   uninstall residuals, and the permanent classic prototype scenarios.
4. **Add inert Claude setup-health diagnostics.** Consume detached host and
   lifecycle observations and implement the seven exact checks and exits.
5. **Deliver and publish the Claude capability-pack tracer.** Register the
   adapter, carry one <code>ma&#116;ty</code> skill plus instruction through show, preview,
   activate, status, update, and deactivate with compatibility/readiness
   separation, then publish the complete <code>ma&#116;ty</code> 3.0.0 current contract.
6. **Complete Claude capability-pack projections and exclusions.** Add commands,
   agents, typed hooks, MCP, assets, notices, composition, consent, evidence
   invalidation, publish the degraded engram 2.0.0 current contract, and retain
   the permanent Pack scenarios.
7. **Stabilize Claude CLI and structured output.** Wire every command, publish
   JSON v2 schemas/fixtures, lock human ordering/remediation, redaction, and exit
   mapping.
8. **Build the Claude regression and real-host smoke cohort.** Close the full
   owner-layered matrix, package-installed exact-floor PR smoke, moving-stable
   canary/release smoke, both Darwin architectures, and durable evidence.
9. **Document and gate first-class Claude Code support.** Publish the canonical
   guide and scoped docs, validate claims/floor, and enforce fail-closed release
   parity without changing the artifact or Homebrew dependency surface.

Slices 1 and 2 are the initial parallel frontier. Slice 3 is blocked by 2.
Slice 4 is blocked by 2 and 3. Slice 5 is blocked by 1 and 2. Slice 6 is blocked
by 5. Slice 7 is blocked by 3, 4, and 6. Slice 8 is blocked by 3, 4, 6, and 7.
Slice 9 is blocked by 7 and 8. A slice closes only when its focused tests and
the repository authority are green; later slices may not hide a failing earlier
contract.

## Definition of implementation-ready

The route is ready to execute because this specification fixes:

- the minimum and comparison rule;
- every allowed global path and effect boundary;
- exact ownership, collision, preservation, and cleanup behavior;
- classic state v2 and v1 migration semantics;
- install, update, uninstall, doctor, exit, and recovery classifications;
- manifest v3 outcomes, wire additions, initial Pack versions, and explicit
  resource mappings/exclusions;
- capability compatibility, consent, readiness, and evidence invalidation;
- human/JSON reporting and redaction;
- owner-layered and real-host validation;
- packaging, docs, release gates, delivery order, and blocking edges; and
- every explicit exclusion from the first release.

No product, architecture, safety, compatibility, naming, lifecycle, state,
migration, UX, validation, documentation, packaging, release, or delivery-order
decision remains before the first implementation slice begins.
