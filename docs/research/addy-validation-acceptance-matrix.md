# Addy validation and acceptance matrix

## Decision question

What deterministic, sandboxed, and surface-specific evidence must prove
provenance, inert acquisition, resource integrity, conflict handling, trust
boundaries, activation readiness, update compatibility, and safe removal before
the Addy specification is executable?

This decision defines the acceptance contract for the execution-ready Addy
specification. It does not implement, configure, synchronize, activate,
publish, or release Addy.

## Acceptance rule

The matrix is complete and blocking. Every Addy contract obligation has
positive and negative deterministic evidence separated by CLI surface and
lifecycle stage. A missing, ambiguous, non-reproducible, or unowned row blocks
the specification; representative coverage and implementation-level unit-test
counts are not substitutes.

The specification may carry red rows while Addy is still unimplemented, but
only when every row is fully specified and classified as one of:

- **Proven existing**: current Packy-owned evidence already proves the
  content-independent invariant and can be reused by Addy.
- **Implementation target**: the oracle is decided, and the final
  specification assigns the work and its blocking edges.
- **Blocked by prerequisite**: the oracle is decided, but a named earlier
  migration or contract change must land before the row can turn green.

This distinction makes the specification executable without pretending Addy
already passes. Addy cannot be published or accepted until every required row
is green.

## Evidence envelope

Every row is a reproducible contractual case with all of the following:

1. a Packy-owned canonical fixture with exact identities and digests;
2. an explicit gate, lifecycle stage, and CLI surface or portable scope;
3. an operation executed with disposable repository, `HOME`,
   `XDG_CONFIG_HOME`, state, and acquisition roots as applicable;
4. an expected structured result and stable diagnostic identity;
5. an exact permitted filesystem/state diff, or proof of zero mutation;
6. a negative twin that changes one decision-relevant fact;
7. a deterministic rerun that produces the same canonical result; and
8. separate Codex and OpenCode results, or an explicit contractual
   degradation or non-applicability reason.

A failed gate must additionally prove that later gates did not execute and
that no later mutation boundary was crossed.

Packy-owned fixtures and oracles are the sole acceptance authority. Upstream
tests, evaluators, hooks, scripts, installers, actions, and CI are inert
evidence only and are never executed during acquisition, synchronization,
validation, activation, or acceptance. Packy may encode a reviewed upstream
assertion in its own fixture. Divergence between upstream behavior and the
Packy contract blocks for explicit classification; it does not rewrite the
oracle automatically.

## Surface cohorts

Portable provenance and bundle facts are proved once. Codex and OpenCode then
run independent projection, conflict, consent, readiness, update, and removal
cohorts. Neither surface can compensate for a failure on the other.

Declared degradation has its own complete logical-capability oracle. In
particular, Codex's `/name` to `$name` workflow degradation must preserve every
required Addy workflow, while OpenCode must preserve the native `/name`
projection. A dual-surface cohort additionally proves that a collision,
failure, update, or removal on one surface does not change the authorized state
of the other or remove a still-required shared projection.

## Blocking gates and rows

The status below describes the repository at the time of this decision. The
final specification must preserve each row's oracle and assign every non-green
row to an implementation slice.

### Gate 1: source admission

| # | Required evidence | Positive oracle | Negative twin | Current status |
|---|---|---|---|---|
| 1 | Exact Addy provenance | The stable selector resolves once to the retained repository/owner IDs, release and tag chain, full commit/tree/parents, verification facts, archive digest, selected-resource digests, and snapshot digest. | A moved tag, changed owner/repository identity, regression, discontinuous tag chain, or changed bytes blocks with zero admitted plan. | Proven existing (`internal/packsync`) |
| 2 | Safe inert acquisition | An exact archive is acquired into an empty disposable root; selected bytes and safe modes are inventoried, no candidate content runs, and the root is cleaned on success or failure. | Traversal, absolute paths, symlink/hardlink, setuid/world-writable mode, hostile executable, hook, or installer is rejected without a sentinel firing. | Proven existing (`internal/packsync/githubsource`) |
| 3 | Per-source lock topology | `sources.json` has an exact bijection with canonical `bundle/sources/<source-id>.lock.json` documents; the target lock and ordered complete lock set reproduce `source_lock_sha256` and `lock_set_sha256`. | Missing, orphaned, duplicate, path-unsafe, or stale locks—including an unrelated-source generation change—block. | Blocked by the singular-lock migration and new schema suite |
| 4 | Exclusive complete source ownership | Every `(pack_id, kind, resource_id)` has one Pack Source owner and Addy's exact candidate replaces its complete configured contribution across all affected Packs. | Equal-byte cross-source duplication, a partial Pack/resource update, or an implicit ownership transfer blocks. | Blocked by per-source provenance and new resource intents |
| 5 | Migration and source registration continuity | A separate clean cutover moves the existing lock without changing candidate, snapshot, selected bytes, resources, or digests; later Addy registration atomically adds configuration, first lock, resources, Packs, and evidence. | Mixed singular/per-source topology, configured-without-lock state, dual writes, fallback reads, or partial registration blocks. | Blocked by the singular-lock migration and new schema suite |

### Gate 2: bundle integrity

| # | Required evidence | Positive oracle | Negative twin | Current status |
|---|---|---|---|---|
| 6 | Exact Addy inventory and attribution | Addy `1.0.0` contains exactly 24 skills, four agents, eight logical workflows, seven shared references, their dependency-closed support files, and retained MIT/source attribution. | A missing or extra required logical capability, helper, reference, or notice blocks the complete Pack. | Implementation target: Pack/resource intents and manifest |
| 7 | Dependency closure and resource integrity | Every selected relative/shared reference resolves within its declared resource closure; canonical inventories reproduce file, resource, and complete snapshot hashes with safe modes. | A dangling or escaping reference, missing helper, unexpected symlink projection, byte drift, unsafe mode, or undeclared file blocks. | Implementation target: Addy bundle fixture and validators |
| 8 | Exclusions remain inert | The four hooks and source-only setup, plugin, symlink, evaluator, validator, test, and CI material are either absent from selected resources or retained only as non-executed evidence; `idea-refine.sh` is content, never an activation or synchronization action. | Any excluded path becomes a Pack resource, projection action, pending action, Go package input, or executed sentinel. | Implementation target: exclusions and dependency-asset model |
| 9 | Schema, producer, validator, and runtime parity | One complete immutable Pack Source schema suite describes every new Addy resource and provenance artifact; all producers, offline validators, fixtures, and runtime domain checks accept and reject the same instances. | Partial suite adoption, a network schema lookup, old/new dual format, unknown resource kind, or producer/runtime disagreement blocks. | Blocked by a new complete schema suite |
| 10 | Compatibility and version evidence | The engine computes the exact next Pack SemVer; evidence covers both surfaces, dependencies, composition, invocation syntax/degradation, requirements, exclusions, aliases, and mandatory actions. A major includes a concrete migration and nonempty actions. | Below-floor or arbitrary version, incomplete affected-Pack evidence, concealed mandatory action, patch/minor migration, or major without migration blocks. | Proven existing engine; Addy coverage is an implementation target |

### Gate 3: planning by surface

| # | Required evidence | Positive oracle | Negative twin | Current status |
|---|---|---|---|---|
| 11 | Prompt-authority disclosure | Preview names filesystem, process, network/browser, subagent, package-manager, and privileged commit/deploy authority that workflows may later request, while granting none of it during activation. | An omitted authority class or any preview/activation effect treated as future tool authorization blocks the plan. | Implementation target: Addy contract rendering |
| 12 | Coherent Codex projection | Codex plans 24 native skills, four native agents, eight complete `$name` workflow-skill degradations, seven shared references, no required omission, and the mandatory notice. | A missing agent/workflow/reference, silent `/name` claim, incomplete logical workflow, or unmodelled degradation blocks the whole Codex surface. | Blocked by agent, command/degradation, asset, and notice intents |
| 13 | Coherent OpenCode projection | OpenCode plans 24 skills, four agents, eight native `/name` commands, seven shared references, no required omission, and the mandatory notice. | A required command represented only by a generic fallback, a missing native projection, or incomplete logical workflow blocks the whole OpenCode surface. | Blocked by agent, command, asset, and notice intents |
| 14 | Collision and alias policy | Fresh occupancy inspection finds no implicit precedence or overwrite; an explicitly selected surface-local `addy-<name>` alias triggers a new sealed preview and retains upstream provenance. | Required collisions, implicit renames, stale alias facts, cross-surface alias leakage, or alias-modified provenance block. | Implementation target: alias intent and CLI input |
| 15 | Pure deterministic preview | Repeated inspection over the same intent, catalog, host observation, and executable facts yields the same canonical plan while repository/home/state snapshots remain unchanged and dry-run asks for no approval. | Any relied-on fact changes before Apply, making the plan stale and executing zero actions with all approvals invalidated. | Proven existing gateway/lifecycle; Addy fixture is an implementation target |
| 16 | Cross-surface and shared-resource isolation | Each surface can plan independently; the dual-surface fixture retains shared projections while any contributor remains and preserves the unaffected surface across collision or failure. | State bleed, one surface compensating for the other's missing capability, or removal of a still-contributed projection blocks verification. | Implementation target: Addy composition fixture |

### Gate 4: application and readiness

| # | Required evidence | Positive oracle | Negative twin | Current status |
|---|---|---|---|---|
| 17 | Typed consent and stale preflight | One exact `reversible-local` receipt follows authority disclosure and authorizes only the sealed projection actions; fresh preflight precedes the first effect. | Missing/wrong receipt, changed intent, host fact, projection fingerprint, executable fact, or catalog identity produces zero effects. | Proven existing lifecycle; Addy receipt fixture is an implementation target |
| 18 | Atomic Apply, verification, and recovery | A surface writes its complete coherent projection or none; fresh post-Apply inspection verifies every goal before `configured=yes`; interruption records an attempt whose originating verb creates a new recovery plan and approvals. | Partial adapter failure, unverified goal, blind replay, or inference of partial readiness leaves recovery required and never claims complete Addy. | Proven existing lifecycle; new Addy projections are implementation targets |
| 19 | Fresh three-state readiness | `configured`, `authorized`, and `usable` are independently observed as `yes`, `no`, or `unknown`, bound to current plan/projection/host revisions, and identical in human and JSON output. Files prove only configured; `--require usable` fails on `no` and `unknown`. | Filesystem presence inferred as usability, unobserved state rendered `no`, stale host evidence, or human/JSON disagreement blocks the readiness claim. | Implementation target: human tri-state output and host usability evidence |
| 20 | Distinct pending, optional, and excluded facts | Finite required human steps are pending actions; unavailable optional browser/network/package/subagent/privileged modes affect only invocations without coherent fallback; contract exclusions require no action. | An optional tool blocks activation, an excluded hook appears pending, or absent usability evidence is hidden as an optional mode. | Implementation target: Addy readiness observation |

### Gate 5: evolution and removal

| # | Required evidence | Positive oracle | Negative twin | Current status |
|---|---|---|---|---|
| 21 | Supported update routes and exact no-op | Every still-supported active Addy version has a tested direct route or explicit mandatory migration chain to the candidate across both surfaces, aliases, shared resources, and mandatory actions; an identical exact candidate produces a schema-valid no-op. | An unsupported gap, incomplete migration, stale precondition, hidden mandatory action, or changed exact no-op input blocks without effects. | Implementation target: versioned Addy history fixtures |
| 22 | Exact-ownership safe removal | Preview lists each candidate, contributor set, and expected fingerprint; only unchanged Packy-owned artifacts no longer required by any active Pack are removed under separate exact destructive consent. | Modified, unknown, ambiguous, residual-owned, or still-shared artifacts are preserved and block complete removal; one surface's removal cannot affect the other. | Proven existing ownership lifecycle; Addy/shared fixtures are implementation targets |
| 23 | Full disposable publication tracer | Inspect, Classify, Validate, and Publish run in disposable clones/homes; Validate and Publish independently reacquire the same candidate, reproduce locks/diff/result tree, and run `./scripts/validate-packy.sh` without changing the sealed diff. Success is one decision-ready non-draft manual-merge PR or a verified no-op. | A different reacquisition, validation mutation/failure, or unexplained terminal success never crosses the first write boundary. | Proven existing workflow tracer; Addy fixture is an implementation target |
| 24 | Freshness, decision-readiness, and artifact safety | The proposal remains bound to exact candidate, base, configuration, manifests, target lock, complete lock set, result tree, branch/head, and managed PR metadata; operational artifacts contain neither secrets nor upstream bytes. | Mutating any binding before the first/final write blocks or invalidates readiness; evidence is never rebased or patched forward. | Proven existing workflow; per-source digests are blocked by Gate 1 |

## Readiness oracle

`configured`, `authorized`, and `usable` remain separate `yes`/`no`/`unknown`
observations. File presence cannot prove host authorization or runtime loading.
Every assertion is bound to the current plan, projection, and host observation.
If a host has no verifiable usability signal, the accepted result is `unknown`,
not simulated readiness. Human and JSON output preserve identical values, and
`--require usable` fails for both `no` and `unknown`.

Pending human actions, optional invocation-time modes, and contract exclusions
are disjoint. A required omission is a blocker, never a partial usable Addy
projection.

## Update and removal oracle

Compatibility classification against the immediate prior Pack version selects
SemVer, but acceptance additionally proves reachability from every
still-supported active version. A route may be direct or a declared migration
chain; Packy never assumes every workstation is on `N-1`.

Removal is ownership reconciliation, not snapshot restoration or best effort.
It deletes only freshly observed, exact-fingerprint Packy-owned artifacts with
no remaining contributor. Modified or ambiguous artifacts are preserved and
block complete removal. Destructive approval is distinct from activation
consent and binds the exact removal plan. Interrupted removal starts from a
fresh observation and never blindly replays the old plan.

## Final publication proof

The Addy acceptance cohort culminates in one disposable
Inspect -> Classify -> Validate -> Publish tracer. Validate and Publish
reacquire the exact candidate independently. The tracer proves sealed source
and lock identities, expected complete-bundle diff and result tree, sandboxed
Packy validation, credential stripping, immutable operational evidence, and
fresh repository and pull-request state before every write boundary.

Every decision-relevant mutation has a negative case. No failed case writes a
branch or pull request, and no later mutation retains decision-readiness. The
only successful outcomes are a schema-valid exact no-op or one open,
non-draft, auto-merge-disabled pull request awaiting human merge.

## Existing evidence and specification gaps

Current reusable foundations include:

- the sandboxed repository validation authority in
  `scripts/validate-packy.sh` and its structural tests in `internal/ci`;
- exact inert acquisition, inventory, sealing, Apply, and Recover in
  `internal/packsync`;
- classification admission in `internal/packsync` and
  `internal/packclassification`;
- sandboxed Validate/Publish orchestration in `internal/packsyncworkflow` and
  `internal/tools/syncpacksource`;
- pure surface inspection, stale-plan rejection, ownership reconciliation,
  and recovery in `internal/capabilitypack`; and
- host-owned projection translation in `internal/codex` and
  `internal/opencode`.

The execution-ready specification must assign the current gaps rather than
weaken their rows:

- the catalog has no Addy Pack;
- Pack schema v1 cannot express agents, commands, dependency assets, notices,
  exclusions, modes, or aliases;
- host adapters cannot yet project Addy agents or commands;
- production synchronization still reads the singular source lock;
- human status collapses unobserved readiness while JSON preserves it;
- neither host currently supplies Addy-specific fresh usability evidence; and
- the immutable current Pack Source schema suite cannot be extended in place.

These are implementation prerequisites, not bootstrap exceptions. The first
Addy `1.0.0` proposal and every later proposal pass the same complete matrix.
