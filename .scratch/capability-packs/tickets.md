# Tickets: Capability packs

Status: done

Tracer-bullet implementation slices for opt-in, composable capability packs across Codex and OpenCode. Source: [capability packs specification](spec.md).

Work the **frontier**: any ticket whose blockers are all done. Clear context between tickets and use `/implement` for one frontier ticket at a time.

## Discover the Matty-owned pack catalog

**What to build:** Let users discover and inspect the initial `matty` and `engram` capability packs through the deep capability-pack facade, backed by strict manifest validation and the Matty-owned bundle.

**Blocked by:** None — can start immediately.

- [x] `matty pack list` reports the initial catalog, versions, descriptions, and supported CLI surfaces without changing state, files, or external tools.
- [x] `matty pack show <pack>` reports capabilities, requirements, conflicts, and resource counts using the project glossary vocabulary.
- [x] Strict manifest decoding accepts the confirmed four resource kinds and rejects unknown fields, invalid identities, invalid composition, and unsupported surfaces with actionable errors.
- [x] The production source comes from the Matty-owned bundle; the test/dev source override remains an injected seam and never becomes a production dependency on an external tree.
- [x] Facade and CLI tests run with sandboxed environment and prove discovery is side-effect free.

## Inspect baseline pack status across surfaces

**What to build:** Let users inspect all pack/surface pairs or one targeted pair through fresh Codex and OpenCode observations before any pack is active.

**Blocked by:** Discover the Matty-owned pack catalog.

- [x] `matty pack status` renders an overview for every catalog pack across Codex and OpenCode.
- [x] `matty pack status <pack> --surface <surface>` renders detailed intent, attempt, readiness, projection, and pending-action sections.
- [x] Status performs fresh inspection through sibling Codex and OpenCode adapters and never mutates Matty state or host configuration.
- [x] Inactive packs are reported without inventing configured, authorized, or usable success.
- [x] Adapter fakes and sandboxed CLI tests verify the facade is the highest behavioral test seam.

## Activate the matty pack on Codex

**What to build:** Let a user preview, approve, apply, and verify activation of the `matty` pack on Codex as the first complete mutable capability-pack path.

**Blocked by:** Inspect baseline pack status across surfaces.

- [x] `matty pack activate matty --surface codex --dry-run` renders an immutable plan and performs no approval, state write, filesystem mutation, or external command.
- [x] Interactive activation groups reversible local projections into a typed phase and applies only the exact approved plan.
- [x] Apply rejects non-interactive input before effects and provides no generic yes-to-all bypass.
- [x] After stale checks, target activation intent and an applying journal are atomically persisted before host effects.
- [x] Fresh verification records per-projection ownership, contributor identity, and fingerprints only after the Codex outcome matches desired state.
- [x] Repeating an already-converged activation is a successful no-op with no approval or Apply.

## Activate the matty pack on OpenCode

**What to build:** Give OpenCode users the same activation contract while proving host-specific projection behavior remains inside the OpenCode adapter rather than leaking into portable policy or CLI orchestration.

**Blocked by:** Activate the matty pack on Codex.

- [x] Preview and interactive activation of `matty` work end to end for OpenCode with explicit surface selection.
- [x] OpenCode inspection and execution preserve unmanaged configuration and host syntax according to the adapter contract.
- [x] The capability-pack module owns desired state, ordering, ownership, approval validation, and readiness; the adapter owns only observed facts and approved execution.
- [x] Codex and OpenCode produce consistent portable outcomes while retaining visibly host-specific actions where required.
- [x] Sandboxed tests prove OpenCode activation does not mutate Codex and vice versa.

## Activate the engram pack with external effects

**What to build:** Let users activate `engram` on Codex and OpenCode with its global executable requirement, local projections, external setup effects, and host-owned follow-up actions clearly separated.

**Blocked by:** Activate the matty pack on OpenCode.

- [x] Preview resolves the Engram executable as a global requirement rather than a surface resource.
- [x] A missing tool is either an actionable blocker or a supported acquisition action in an executable/external phase.
- [x] Local projections and external setup use separate typed approvals bound to the exact plan and phase digests.
- [x] External effects begin only after the reversible local commit and act as barriers for later work.
- [x] Verified Apply reports pending authentication, trust, or reload without claiming the pack is authorized or usable.
- [x] Both Codex and OpenCode flows use the concrete Engram resolver through an injected seam and are verified without running real operator commands or writing real configuration.

## Plan composition, dependencies, and blockers

**What to build:** Let users understand and approve the complete combined desired state when packs share resources, require other packs, conflict, or encounter unsafe ownership.

**Blocked by:** Activate the engram pack with external effects.

- [x] Activating a pack may include inactive required packs in one plan, with requested and required activations labeled separately before approval.
- [x] Apply cannot add a dependency or effect absent from the approved plan.
- [x] Compatible shared resources merge contributors deterministically; incompatible contributions block planning without last-writer-wins behavior.
- [x] Preview reports all currently known missing requirements, capability conflicts, and ownership ambiguities together.
- [x] A blocked preview yields no applicable plan, zero actions, and no intent revision change.
- [x] Tests cover the complete desired state across both surfaces even when the request names one pack.

## Update packs to catalog-current

**What to build:** Let users update one active pack and surface toward the current Matty-owned catalog version while preserving shared resources and showing every local or external effect.

**Blocked by:** Plan composition, dependencies, and blockers.

- [x] `matty pack update <pack> --surface <surface>` targets only the catalog-current version and offers no explicit version or remote-fetch behavior.
- [x] Preview shows old and target versions, intent revision, changed projections, retained shared resources, and external phases.
- [x] Apply persists the approved target version in durable intent before effects and verifies the resulting projections.
- [x] Unchanged shared projections retain their contributor sets without unnecessary rewrites.
- [x] A catalog-current, drift-free pack returns a successful no-op.
- [x] Tests cover both CLI surfaces, conflicts introduced by a new contribution, and exact-plan approval.

## Deactivate packs with contributor-safe cleanup

**What to build:** Let users deactivate a pack safely without cascading into dependents or deleting shared, drifted, ambiguous, or unmanaged content.

**Blocked by:** Plan composition, dependencies, and blockers.

- [x] Deactivation of a pack still required by another active pack is rejected with actionable dependency information and no cascade.
- [x] Removing one contributor retains a shared projection while any contributor remains.
- [x] Last-contributor deletion is planned only for a projection whose observed fingerprint matches Matty's last verified ownership record.
- [x] Destructive cleanup has its own typed, descriptive approval and lists every deletion before Apply.
- [x] Drifted, ambiguous, and unmanaged targets are preserved and reported as pending human actions.
- [x] Repeating deactivation of an already-inactive, converged pack is a successful no-op.

## Reconcile drift and reject stale plans

**What to build:** Let users repair safe drift toward current activation intent, either for one pack or every active pack on a surface, while refusing to apply obsolete plans.

**Blocked by:** Update packs to catalog-current; Deactivate packs with contributor-safe cleanup.

- [x] Targeted and surface-wide reconcile both compute against complete desired state and contributor sets without changing activation intent.
- [x] Reversible, external, and destructive repair phases remain visibly separated with their required approvals.
- [x] Drifted or ambiguous unmanaged replacements are preserved rather than adopted, overwritten, or deleted.
- [x] A drift-free reconcile is a successful no-op with no approval or Apply.
- [x] Changed host observations or intent revision after Preview terminate the attempt as stale, invalidate receipts, execute zero actions, and leave intent unchanged.
- [x] Stale output identifies the changed precondition and instructs the user to rerun the original command without automatically previewing a replacement.

## Recover truthful partial attempts

**What to build:** Let users understand and recover from interrupted or partially failed lifecycle operations without speculative rollback or replaying an obsolete plan.

**Blocked by:** Reconcile drift and reject stale plans.

- [x] Local actions stage and validate outputs, retain backups, and roll back only within a proven restorable transaction.
- [x] External failure stops later actions at the barrier and records completed, failed, and not-started facts truthfully.
- [x] Recovery-required preserves approved durable intent and does not claim unverified ownership or readiness.
- [x] Repeating the originating activate, update, or deactivate verb recognizes the target intent and previews a fresh recovery plan.
- [x] Recovery preserves already verified owned work, proposes only safe remaining or compensating actions, and requires new typed approvals.
- [x] Crash and failure tests prove the old plan is retained for history but never mutated or replayed.

## Gate independently derived readiness

**What to build:** Let users and automation distinguish successful reconciliation from configured, authorized, and usable pack readiness using fresh host inspection.

**Blocked by:** Activate the engram pack with external effects.

- [x] Status derives configured, authorized, and usable independently from fresh adapter observations rather than intent or Apply success.
- [x] Pending authentication, trust, reload, or runtime action is reported with an actionable follow-up and never treated as an Apply receipt.
- [x] A verified Apply exits successfully even when authorization or usability remains pending.
- [x] `matty pack status <pack> --surface <surface> --require usable` exits nonzero until usable is freshly observed.
- [x] Overview status highlights pending and recovery-required pack/surface pairs without conflating attempt state and readiness.
- [x] Tests cover configured-but-unauthorized, authorized-but-not-loaded, usable, and recovery-required outcomes.

## Validate rollout and manual transition

**What to build:** Give existing Matty users a documented, sandbox-verified path into the opt-in pack model and prove the complete `matty`/`engram` lifecycle across both supported surfaces.

**Blocked by:** Recover truthful partial attempts; Gate independently derived readiness.

- [x] Document the manual transition from pre-pack Matty state without automatic migration or writes to real user configuration.
- [x] CLI help and examples cover discovery, explicit surface activation, dry-run, typed approvals, status/readiness, update, deactivation, reconcile, stale retry, and recovery.
- [x] End-to-end sandbox scenarios prove `matty` and `engram` activate, update, deactivate, reconcile, and report readiness correctly on Codex and OpenCode.
- [x] The rollout preserves Matty core independently of the optional `matty` pack and does not reintroduce external source trees as runtime dependencies or default install targets.
- [x] `web`, `mobile`, remote packs, marketplaces, signing, multi-version selection, unattended Apply, and automatic migration remain absent.
- [x] Focused checks, filesystem/external-command assertions, and the full repository test suite pass.
