Status: implemented

# Capability packs specification

## Problem Statement

Matty currently installs one fixed global workflow across Codex and OpenCode. Users cannot discover, opt into, compose, update, inspect, reconcile, or safely deactivate named workflow capabilities per CLI surface. Existing host configuration, external tools, shared resources, trust/authentication boundaries, concurrent changes, and partial failures make a naive installer unsafe: it could overwrite unmanaged content, apply work that was not approved, hide dependency activation, or claim a pack is usable when human action remains.

## Solution

Add a Matty-owned capability-pack system managed through `matty pack`. A strict `pack.json` manifest describes portable pack identity, capabilities, requirements, conflicts, and `skill`, `instruction`, `mcp_server`, and `lifecycle` resources. Matty core builds a complete desired state for each CLI surface, projects it through Codex and OpenCode adapters, tracks verified ownership and durable activation intent, and reconciles the host only through immutable previewed plans with typed human approvals.

The first catalog and proof include only the `matty` and `engram` packs. Discovery and status are inspection-only. Lifecycle commands present Preview and Apply as one understandable interactive flow while preserving their architectural separation; `--dry-run` previews without approval or mutation. Status reports reconciliation attempts independently from configured, authorized, and usable readiness.

## User Stories

1. As a Matty user, I want to list available capability packs, so that I can discover optional workflows before changing my setup.
2. As a Matty user, I want to inspect a pack's version, capabilities, requirements, resources, and supported surfaces, so that I understand what it contributes.
3. As a Matty user, I want every pack lifecycle command grouped under `matty pack`, so that the root CLI remains understandable.
4. As a Matty user, I want to activate a pack explicitly on Codex or OpenCode, so that activation on one surface never implies activation on another.
5. As a cautious user, I want `--dry-run` to show the exact lifecycle plan without requesting approval or mutating anything, so that I can inspect changes safely.
6. As a user applying a lifecycle change, I want to see the exact immutable plan before approval, so that Apply cannot execute unreviewed work.
7. As a user, I want reversible local changes, executable or external effects, and destructive cleanup shown as distinct phases, so that their different risks remain visible.
8. As a user, I want a separate approval for every required consent kind, so that approving local configuration never implies consent to commands or deletion.
9. As a user, I want Apply to require an interactive terminal, so that automation cannot bypass mandatory human checkpoints.
10. As an automation author, I want discovery, dry-run, and status to remain non-interactive and side-effect free, so that I can inspect Matty safely.
11. As a user activating a composed pack, I want inactive required packs explicitly included in the combined plan, so that dependency activation is convenient but never hidden.
12. As a user, I want all known dependency, conflict, and ownership blockers reported together, so that I can remediate them in one cycle.
13. As a user, I want Matty to reject capability conflicts rather than choose a last writer, so that composition is deterministic and safe.
14. As a user, I want external tools represented as global requirements, so that they are not confused with Codex or OpenCode resources.
15. As a user, I want a supported global-tool acquisition action shown as an external phase, so that installing it requires explicit executable consent.
16. As a user, I want to update a pack to the current Matty-owned catalog version, so that update has one clear meaning.
17. As a user, I want unchanged shared projections retained during update, so that other contributing packs are not disrupted.
18. As a user, I want to deactivate a pack without cascading into dependents, so that disabling a required pack is rejected rather than surprising me.
19. As a user, I want shared projections retained while any contributor remains, so that deactivating one pack does not break another.
20. As a user, I want destructive cleanup limited to unchanged, verified, last-contributor projections, so that Matty does not delete user-managed content.
21. As a user, I want drifted, ambiguous, or unmanaged targets preserved with a pending human action, so that Matty does not overwrite, delete, or silently adopt them.
22. As a user, I want targeted reconcile for one pack and bulk reconcile for a surface, so that I can choose focused repair or general maintenance.
23. As a user, I want reconcile to repair projections toward current intent without changing activation intent, so that repair is not another lifecycle transition.
24. As a user, I want an already-converged lifecycle command to succeed without approval or Apply, so that safe retries are idempotent.
25. As a user, I want a stale plan to execute zero actions and explain the changed precondition, so that concurrent host or Matty changes are never overwritten.
26. As a user, I want stale approval receipts invalidated and the command stopped, so that a replacement plan always receives fresh review.
27. As a user, I want partial failure output to distinguish completed, failed, and not-started actions, so that actual host state remains truthful.
28. As a user, I want local staged changes rolled back only when restoration is proven safe, so that Matty does not promise impossible global atomicity.
29. As a user, I want external failure to stop later phases without speculative compensation, so that recovery does not cause additional damage.
30. As a user recovering a failed lifecycle operation, I want to repeat its original verb, so that recovery follows the action I was already performing.
31. As a user, I want the repeated verb to inspect fresh state and preview a new recovery plan, so that it never replays the failed attempt.
32. As a user, I want a general status overview across all pack/surface pairs, so that I can discover problems quickly.
33. As a user, I want detailed status for one pack and surface, so that I can inspect intent, attempts, readiness, projections, and pending human actions.
34. As a user, I want configured, authorized, and usable reported independently, so that successful configuration is not mistaken for login, trust, reload, or runtime usability.
35. As an automation author, I want `status --require usable` to return nonzero until usability is observed, so that readiness can be gated independently of Apply success.
36. As a user, I want a verified Apply to exit successfully even when host-owned action remains, so that reconciliation outcome and readiness are not conflated.
37. As a maintainer, I want Matty core always available independently of the optional `matty` pack, so that pack management cannot deactivate itself.

## Implementation Decisions

- Use a strict Matty-native `pack.json`; reject unknown fields and invalid manifests rather than guessing.
- Support only `skill`, `instruction`, `mcp_server`, and `lifecycle` portable resources initially. Host-native artifacts are adapter projections, not manifest fields.
- Treat external executables such as Engram as global pack requirements. Acquisition and inspection remain Matty core behavior.
- Keep the initial Matty-owned catalog and proof limited to `matty` and `engram` across Codex and OpenCode.
- Put manifest validation, catalog construction, dependency/conflict resolution, desired-state computation, activation intent, ownership, planning, application, recovery, and readiness behind one deep capability-pack module.
- Expose only conceptual `Preview`, `Apply`, and `Status` operations to the CLI. The CLI renders interactions but does not orchestrate portable policy or adapters.
- Use sibling Codex and OpenCode adapters behind interfaces owned by the capability-pack module. Adapters inspect and execute host projections but do not decide ownership, global ordering, readiness, or recovery.
- Keep physical bundle discovery behind the skill-bundle locator and concrete Engram executable resolution behind the Engram binary resolver.
- Persist activation intent with a monotonically increasing revision. After stale checks, atomically persist the approved target intent and applying journal before side effects.
- Make a reconciliation plan an immutable envelope over the operation, intent revision, relied-on observations, ordered actions, typed phases, and digest. Apply receives that plan and never rebuilds it.
- Track ownership per projection identity with contributor sets and last verified fingerprints. Interrupted journals record facts without claiming verified ownership.
- Order plans into reversible local, executable/external, and destructive-cleanup phases. Atomicity is limited to local actions that can be staged, validated, backed up, and restored.
- Bind approvals to the plan digest, phase digest, and consent kind. No approval kind implies another, and stale inputs invalidate all receipts.
- Require an interactive terminal for Apply. Do not initially support generic `--yes`, serialized plan files, unattended Apply, or transported approval receipts.
- Group commands under `matty pack`: list, show, status, activate, update, deactivate, and reconcile.
- Compose Preview and Apply into one interactive lifecycle command; `--dry-run` calls only Preview and preserves Matty's existing preview convention.
- Require explicit `--surface codex|opencode` for lifecycle mutations. Status without a target provides an overview; targeted status provides details.
- Allow activation to include explicitly displayed required packs in one combined plan. Reject disabling a required pack without cascading.
- Update only to the current version in the Matty-owned catalog; omit explicit version selection initially.
- Allow both targeted reconcile and bulk reconcile per surface. Reconcile never changes activation intent.
- Treat already-converged lifecycle requests as successful no-ops with no approval or Apply.
- On stale Apply, report the changed precondition, execute zero actions, invalidate approvals, and stop without automatically previewing a replacement.
- On recovery-required, preserve truthful completed/failed/not-started facts. Repeating the originating lifecycle verb performs fresh inspection and presents a newly approved plan.
- Keep reconciliation-attempt outcomes separate from readiness. Verified Apply exits zero; pending trust/authentication/reload is reported, and `status --require usable` supplies a separate readiness gate.

## Testing Decisions

- Test external behavior through the capability-pack facade wherever possible; the primary seam is Preview, Apply, and Status rather than internal planners or adapter helpers.
- Test strict manifest decoding, validation, catalog resolution, dependency closure, conflicts, desired state, ownership contributors, stale guards, typed receipts, phase ordering, journals, recovery, and readiness through facade results.
- Use replaceable Codex/OpenCode adapters and tool-resolution seams to supply observed fingerprints and capture approved actions without touching real host configuration.
- Add CLI command tests around rendered plans, prompts, exit codes, no-op results, consolidated blockers, status tables, stale output, pending actions, and recovery guidance. Follow existing Cobra command tests that inject environment and runner dependencies.
- Sandbox `HOME`, `XDG_CONFIG_HOME`, `PATH`, and any Matty source override in every filesystem or CLI test. Never write to the operator's real configuration.
- Verify `--dry-run`, list, show, and status perform no state, filesystem, or external-command mutation.
- Verify non-TTY Apply fails before effects, while inspection-only operations remain scriptable.
- Verify stale intent or host fingerprints produce zero actions and no intent revision change.
- Verify local rollback only for staged/restorable actions, external barriers stop later actions, and recovery plans are computed from fresh inspection.
- Verify shared projections remain with contributors, last-contributor cleanup requires matching fingerprints and destructive approval, and drifted/unmanaged targets are preserved.
- Verify readiness using fresh adapter inspection rather than persisted intent or Apply success, including `--require usable` exit behavior.
- Run the full repository test suite after focused tests.

## Out of Scope

- Third-party packs, remote sources, marketplaces, signing, and public ecosystem policy.
- `web` and `mobile` pack definitions or validation.
- Repository-scoped or per-session activation.
- Turning Matty into an always-running runtime launcher or orchestrator.
- Automatic migration from pre-pack Matty state; rollout uses a documented sandbox-verified manual transition.
- Multiple retained catalog versions, explicit version selection, downgrade policy, or fetching unlisted versions.
- Unattended Apply, generic yes-to-all behavior, serialized plan exchange, or approval-receipt transport.
- Global rollback across external tools, authentication, trust, or other non-reversible effects.
- Automatic host trust, authentication, authorization, or reload completion.

## Further Notes

- The confirmed throwaway lifecycle prototype supplies the interaction contract and scenario matrix; it is not production code.
- Existing Matty `--dry-run` behavior and injected environment/runner test patterns are the prior art for CLI safety.
- Product vocabulary follows the project glossary: capability pack, pack resource, pack requirement, pack activation, pack desired state, pack ownership, pack readiness, reconciliation plan, and reconciliation attempt.
- Durable product and architecture decisions should move from `.scratch` into product documentation and ADRs before temporary planning artifacts are removed.
