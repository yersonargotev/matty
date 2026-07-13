Status: ready-for-agent

# Capability-pack surface adapter deepening specification

## Problem Statement

Matty users rely on capability-pack status, activation, update, deactivation,
reconciliation, stale-plan rejection, and post-apply verification to observe and
change Codex and OpenCode safely. Those behaviors work today, but the
capability-pack facade discovers host inspection behavior through a base
activation adapter plus several optional lifecycle interfaces, a deprecated
status-only inspector, and a separately registered readiness inspector.

The optionality is not real: both production host adapters already implement
the complete lifecycle and readiness behavior. The fragmented interface lets
different facade paths normalize observations differently, permits invalid
adapter combinations, exposes lifecycle phases to host modules, and makes
partial adapters appear supported even though their fallbacks provide weaker
semantics.

The architecture should contract to one real Codex/OpenCode surface seam
without changing any observable capability-pack behavior. Users must receive
the same plans, blockers, consent phases, ownership protection, readiness,
status, recovery behavior, stale-plan rejection, filesystem effects, and CLI
output as before.

## Solution

Introduce one capability-pack-owned `SurfaceAdapter` interface implemented by
the Codex and OpenCode modules. It exposes one fresh, side-effect-free
inspection operation and one separate projection-application operation.

Capability-pack converts each status or lifecycle use case into normalized
transition facts: the relevant prior composition, desired composition,
residual ownership scope, and already-resolved executable facts. The adapter
does not receive an activation, update, deactivation, reconciliation, or status
label. It translates those facts into one normalized surface inspection that
contains projection observations, explicit present-or-absent goals, fresh host
readiness evidence, and pending human actions.

Capability-pack remains the sole owner of lifecycle meaning, composition,
ownership authority, blockers, consent, destructive authorization, recovery,
readiness progression, plan sealing, and verification. A single private
gateway clones inputs and outputs, validates the adapter contract, rejects
unsafe or ambiguous observations, and canonicalizes results for every caller.

This is a behavior-preserving architectural refactor. It removes fictional
adapter variability and consolidates OpenCode surface behavior into its owning
host module without adding lifecycle behavior or broadening cleanup scope.

## User Stories

1. As a Matty user, I want capability-pack commands to preserve their current behavior, so that an architectural refactor does not change my workstation.
2. As a Codex user, I want status to inspect Codex projections freshly, so that its report reflects the current host state.
3. As an OpenCode user, I want status to inspect OpenCode projections freshly, so that its report reflects the current host state.
4. As a capability-pack user, I want targeted status to remain limited to the requested pack and dependency closure, so that inspection does not become an implicit cleanup operation.
5. As a capability-pack user, I want activation previews to preserve their current actions and blockers, so that approval remains predictable.
6. As a capability-pack user, I want updates to preserve their current convergence scope, so that the refactor does not introduce automatic obsolete-resource cleanup.
7. As a capability-pack user, I want deactivation to compare the prior and desired compositions, so that disappearing projections remain visible and safely handled.
8. As a capability-pack user, I want reconciliation to inspect obsolete owned projections, so that residual Matty state remains repairable.
9. As a capability-pack user, I want targeted and surface-wide reconciliation to preserve their existing scopes, so that intent is not changed accidentally.
10. As a cautious user, I want unmanaged or drifted projections preserved, so that Matty never treats adapter output as deletion authority.
11. As a cautious user, I want destructive cleanup to retain explicit approval, so that unapproved host content is never removed.
12. As a user with a stale plan, I want preflight to reinspect the host through the same contract used by preview, so that changed facts invalidate the plan reliably.
13. As a user applying a plan, I want Matty to verify the resulting host state freshly, so that successful execution is not confused with convergence.
14. As a user recovering from a partial attempt, I want recovery facts and verification semantics preserved, so that the refactor does not weaken truthful recovery.
15. As a user whose pack requires an executable, I want already-resolved executable facts translated consistently by each host, so that host projections use the approved command path.
16. As a user, I want inspection to execute no commands, so that status and preview remain read-only.
17. As a user, I want inspection to write no files or state, so that observation cannot mutate my workstation.
18. As a user, I want projection application to remain the adapter's only mutation operation, so that effects remain explicit and approved.
19. As a user, I want configured, authorized, and usable readiness to preserve their current meaning, so that automation gates remain stable.
20. As a user, I want pending host actions and readiness evidence preserved, so that trust, reload, and runtime limitations remain actionable.
21. As an automation author, I want status output and require-usable behavior preserved, so that existing automation does not regress.
22. As a maintainer, I want one real surface seam, so that all facade paths use the same host inspection contract.
23. As a maintainer, I want every supported surface adapter to implement complete inspection and application behavior, so that partial fallbacks cannot hide missing safety semantics.
24. As a maintainer, I want adapters to receive transition facts rather than lifecycle labels, so that host modules do not own portable lifecycle policy.
25. As a maintainer, I want projection goals to state present or absent explicitly, so that a missing fingerprint cannot be interpreted as destructive intent.
26. As a maintainer, I want absent goals validated against removal actions, so that invalid adapter output is rejected before planning.
27. As a maintainer, I want present goals validated against desired fingerprints and non-removal actions, so that incomplete observations fail safely.
28. As a maintainer, I want projection observations and readiness evidence returned together, so that incompatible host adapters cannot be composed accidentally.
29. As a maintainer, I want one normalized projection collection, so that removal candidates are not a lifecycle-specific side channel.
30. As a maintainer, I want adapter results cloned and canonically ordered centrally, so that plan fingerprints remain deterministic.
31. As a maintainer, I want duplicate or malformed projection IDs rejected centrally, so that every caller receives the same contract enforcement.
32. As a maintainer, I want capability-pack to retain ownership, blocker, consent, recovery, and readiness policy, so that adapters remain host translators rather than lifecycle owners.
33. As a maintainer, I want Codex-specific marker and configuration rules localized in the Codex module, so that portable policy does not absorb host syntax.
34. As a maintainer, I want OpenCode-specific JSONC, instruction, and configuration rules localized in the OpenCode module, so that host behavior has one owner.
35. As a maintainer, I want only neutral filesystem and fingerprint primitives shared across hosts, so that superficial duplication does not create a shallow cross-host abstraction.
36. As a maintainer, I want the deprecated status inspector removed, so that status cannot bypass lifecycle-quality observations.
37. As a maintainer, I want the separate readiness inspector removed, so that readiness evidence cannot come from a different host adapter.
38. As a maintainer, I want optional resolution, deactivation, and reconciliation interfaces removed, so that fictional variability does not return.
39. As a maintainer, I want the OpenCode surface adapter in the OpenCode owning module, so that package names reflect hosts rather than lifecycle phases.
40. As a contributor, I want the architecture recorded in an accepted ADR, so that seam placement and safety invariants remain durable.
41. As a contributor, I want facade tests to exercise the complete use-case matrix through one fake adapter, so that the interface is the test surface.
42. As a contributor, I want sandboxed Codex and OpenCode adapter contracts, so that host translation is verified without touching operator configuration.
43. As a contributor, I want a structural deletion test, so that obsolete interfaces, helpers, and package ownership cannot be reintroduced silently.
44. As a CLI maintainer, I want command tests limited to composition, rendering, exit behavior, and wiring, so that CLI tests do not duplicate capability-pack policy.
45. As a future host-adapter author, I want one complete contract to implement, so that supported behavior and invalid states are explicit.

## Implementation Decisions

- Keep capability-pack as the owner of the surface interface; Codex and OpenCode remain its two concrete adapters.
- Replace the activation-specific adapter family with one lifecycle-neutral `SurfaceAdapter` interface.
- Expose one pure inspection operation and keep projection application as a separate mutating operation.
- Require every supported surface adapter to implement the complete contract. Do not retain partial-adapter fallbacks or optional capability discovery.
- Represent inspection input as normalized transition facts: relevant prior composition, desired composition, residual ownership supplied for inspection, and resolved executable facts.
- Do not expose status, activate, update, deactivate, or reconcile labels to host adapters.
- Construct transition facts according to existing behavior: status, activation, and update inspect desired projections only; deactivation compares prior and desired compositions; reconciliation additionally supplies residual ownership.
- Keep targeted status free of residual cleanup inspection. Capability-pack may still use complete ownership facts to classify desired projection health.
- Return one `SurfaceInspection` containing a single normalized projection collection, fresh authorization/usability evidence, pending human actions, and the revision facts required for deterministic plan fingerprints.
- Remove the separate removal-candidate collection. Every projection carries an explicit, nonzero present-or-absent goal.
- Require present goals to include valid desired fingerprints and compatible non-removal actions. Require absent goals to include compatible removal actions and no desired fingerprint.
- Reject zero goals, duplicate IDs, malformed observations, and incompatible goal/action combinations before planning.
- Route status, preview, stale-plan preflight, and post-apply verification through one private inspection gateway.
- Make the private gateway clone transition inputs, clone adapter output, enforce the contract, and canonicalize ordering. It is an implementation detail rather than another module interface.
- Require inspection to observe the host freshly, remain side-effect-free, execute no commands, write no files, and persist no state.
- Keep projection application as the only mutating operation exposed by a surface adapter.
- Integrate fresh readiness evidence into surface inspection and remove separate readiness-adapter registration.
- Keep configured readiness derived by capability-pack from verified projection state. Capability-pack continues enforcing configured-to-authorized-to-usable progression.
- Keep ownership authority, contributor rules, blocker classification, typed consent, destructive authorization, lifecycle intent, recovery, plan sealing, and verification inside capability-pack.
- Keep host-specific transition translation inside each host module. Do not introduce a generic cross-host lifecycle module.
- Continue sharing only neutral projection primitives such as fingerprinting and safe filesystem mechanics.
- Rename activation-specific interface and observation concepts to lifecycle-neutral surface terminology.
- Consolidate the OpenCode capability-pack adapter into the existing OpenCode host module. Preserve dependency direction: host adapters depend on capability-pack contracts, not the reverse.
- Delete the deprecated baseline status inspector, optional lifecycle-aware interfaces, separate readiness interface, lifecycle-specific inspection helpers, and compatibility wrappers after migration.
- Treat this work as a behavior-preserving refactor. New cleanup behavior, including obsolete-resource removal during update, requires a separate specification.
- Record the final seam, invariants, ownership, behavior-preservation constraint, and deletions in a dedicated accepted architecture decision.
- Deliver the work through three independently green slices: contract and ADR, unified routing, then ownership/naming contraction and architecture enforcement.

## Testing Decisions

- Use the capability-pack facade as the primary behavioral seam. Tests and production callers cross the same Preview, Apply, and Status interface.
- Before production changes, freeze the current observable matrix for status, activation, update, deactivation, reconciliation, stale-plan preflight, post-apply verification, recovery, readiness, blockers, consent, and ownership protection.
- Exercise facade behavior with one complete fake `SurfaceAdapter`; do not create different fakes for optional lifecycle capabilities.
- Verify operation-specific transition scope indirectly through facade outcomes: desired-only status/activation/update, prior-to-desired deactivation, and ownership-aware reconciliation.
- Verify malformed adapter output centrally: zero goals, duplicate IDs, missing desired fingerprints, removal actions for present goals, and non-removal actions for absent goals must fail safely.
- Verify deterministic cloning and ordering through stable plan fingerprints and stale-plan behavior rather than by directly testing private helpers.
- Test Codex and OpenCode adapters through sandboxed host files with scenarios for present, missing, drifted, unmanaged, prior-composition residual, ownership residual, shared-file updates, and surgical removal.
- Verify fresh readiness evidence and configured-to-authorized-to-usable normalization through facade status and apply results.
- Verify inspection purity by asserting no sandboxed file, state, or command execution changes during status, preview, preflight, or verification.
- Keep adapter application tests focused on executing already-authorized host actions and preserving unrelated host content.
- Keep CLI tests limited to adapter composition, rendering, output failures, exit behavior, require-usable behavior, and command wiring.
- Add a structural architecture test that forbids the optional lifecycle-aware interfaces, deprecated status inspector, separate readiness inspector, removal-candidate field, lifecycle-specific inspection helpers, and the obsolete OpenCode activation package.
- Delete tests that exist only to protect partial adapters, fallback behavior, or obsolete private decomposition after the replacement contract covers them.
- Use sandboxed HOME and XDG configuration paths for every filesystem-backed test. Never read or write the operator's real host configuration.
- Run focused package tests while iterating and the complete repository test suite for every ticket.

## Out of Scope

- Changing capability-pack commands, flags, rendering, JSON shape, exit behavior, or require-usable semantics.
- Changing plan actions, blockers, consent phases, approval receipts, ownership authority, recovery behavior, or stale-plan rules.
- Adding automatic obsolete-resource cleanup to activation or update.
- Changing targeted or bulk reconciliation scope or lifecycle intent semantics.
- Adding another host surface or designing for hypothetical partial adapters.
- Changing portable pack manifests, resource kinds, dependency resolution, conflict handling, catalog behavior, or state schema.
- Changing executable acquisition, external effects, command execution, or compensation policy.
- Combining capability-pack readiness with base setup health diagnosis.
- Redesigning workstation path resolution or moving host paths into capability-pack policy.
- Creating a generic cross-host lifecycle or projection-policy abstraction.
- Opportunistic cleanup outside surface inspection ownership and naming.

## Further Notes

- Two production adapters prove that the surface seam is real; their complete implementations prove that lifecycle-specific optionality is not.
- The deletion test is decisive: removing the host adapters must force Codex and OpenCode projection rules into portable policy, while removing any of the obsolete optional interfaces should remove no supported behavior.
- The accepted ADR should describe the durable architecture and link to the broader capability-pack product specification rather than duplicating it.
- Work the approved tickets in dependency order, one frontier ticket per fresh implementation session.
