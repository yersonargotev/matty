Status: ready-for-agent

# Matty core lifecycle deepening specification

## Problem Statement

Matty users rely on install, update, and uninstall to reconcile global workflow
artifacts safely, preserve unmanaged content, recover from interruption, and
report actionable failures. Those behaviors work today, but their policy is
spread through the CLI adapter alongside flags and rendering. As a result,
maintainers must test lifecycle behavior through command construction, changes
to recovery or ownership require knowledge of several shallow modules, and a
future setup-health module would have to depend on CLI-owned state.

The architecture should change without changing the user's commands, persisted
state, safety guarantees, or observable lifecycle behavior.

## Solution

Introduce one deep Matty core lifecycle module that owns install, update, and
uninstall planning, classic state persistence, ownership, recovery,
application, verification, and update-time Installed Source policy. The CLI
will resolve workstation configuration, invoke the module, and render its
plans, structured results, warnings, and errors.

The operational interface will consist of read-only Preview and mutating Apply.
Preview produces an opaque, caller-immutable plan; Apply consumes that exact
plan. A separate read-only observation interface will expose classic state facts
to doctor without transferring persistence or health-classification ownership.

This is a behavior-preserving architectural refactor. Matty core lifecycle and
capability-pack lifecycle remain independent.

## User Stories

1. As a Matty user, I want install to preserve its current behavior, so that an architectural refactor does not disrupt my setup.
2. As a Matty user, I want update to preserve its current behavior, so that managed tools and workflow artifacts still refresh safely.
3. As a Matty user, I want uninstall to remove only verified Matty-owned artifacts, so that unmanaged content remains protected.
4. As a cautious user, I want every lifecycle dry-run to remain read-only, so that previewing changes cannot alter my workstation.
5. As a Matty user, I want an interrupted install or update to retain truthful recovery state, so that retry and cleanup remain safe.
6. As a Matty user, I want corrupt classic state rejected before mutation, so that Matty never guesses ownership.
7. As a Matty user, I want the existing classic state schema and location preserved, so that no state migration is required.
8. As a package-installed user, I want update to reject a stale default Installed Source before mutation, so that binary and workflow sources stay aligned.
9. As a developer using an explicit source override, I want update to preserve development-source behavior, so that test and development workflows remain usable.
10. As a repository contributor, I want repository checkout source behavior preserved, so that local development remains predictable.
11. As a user without canonical Homebrew Engram, I want the same actionable failure before unsafe setup execution, so that remediation remains clear.
12. As a user with unmanaged skill paths or symlinks, I want Matty to preserve them and report warnings, so that it never silently adopts or overwrites them.
13. As a user uninstalling after a partial install, I want only proven ownership removed, so that recovery metadata cannot broaden deletion authority.
14. As an automation author, I want lifecycle exit behavior and relevant messages preserved, so that existing scripts do not regress.
15. As an automation author, I want doctor to observe classic lifecycle state without mutation, so that health checks remain safe.
16. As a maintainer, I want lifecycle behavior tested through one high-level interface, so that tests exercise the same seam as callers.
17. As a maintainer, I want plans immutable to callers, so that the CLI cannot add, remove, or reorder lifecycle actions.
18. As a maintainer, I want structured lifecycle results and errors, so that presentation remains independent from domain behavior.
19. As a maintainer, I want external command execution replaceable in tests, so that tests never invoke real Homebrew or Engram.
20. As a maintainer, I want time replaceable in tests, so that persisted checks and results remain deterministic.
21. As a maintainer, I want filesystem behavior tested in sandboxed directories, so that production file semantics are exercised without touching real user configuration.
22. As a maintainer, I want existing skill, prompt, OpenCode, Engram, container, and bootstrap modules to retain their ownership, so that the lifecycle module deepens orchestration without absorbing unrelated implementations.
23. As a maintainer, I want classic state independent from capability-pack state, so that one lifecycle cannot mutate or remove the other's intent or ownership.
24. As a maintainer, I want the old CLI lifecycle policy deleted after migration, so that the codebase has one owner rather than forwarding modules or dual behavior.
25. As a future health-module author, I want a stable read-only lifecycle observation, so that setup health can deepen without importing CLI types or persistence internals.

## Implementation Decisions

- Create one deep module named `corelifecycle` as the sole owner of Matty core install, update, and uninstall behavior.
- Exclude Installed Source initialization, setup-health classification, and capability-pack lifecycle operations.
- Present one operational facade with conceptual Preview and Apply operations covering install, update, and uninstall.
- Make Preview strictly read-only: it may inspect state, managed artifacts, source facts, and executable availability, but cannot create, repair, execute, or persist.
- Make lifecycle plans opaque and caller-immutable. Rendering uses a read-only action view; Apply receives the unchanged plan produced by Preview.
- Return structured results, warnings, and actionable domain errors without writing directly to command output streams.
- Own classic state types, schema compatibility, loading, atomic saving, ownership records, recovery status, and desired-state derivation inside the lifecycle module.
- Provide a separate read-only observation interface for state presence, corruption, recovery status, and recorded ownership. Do not expose the internal store or health classification.
- Keep workstation path resolution outside this refactor. The CLI maps already-resolved locations and source facts into lifecycle configuration once during facade construction.
- Limit external dependency seams to command lookup/execution and time. Keep filesystem behavior internal and exercise it through sandboxed paths, with private failure seams only where persistence tests require them.
- Retain existing owner modules for Skill bundle discovery, Codex projections, OpenCode projections, Engram executable behavior, owned-container cleanup, and Installed Source Git validation.
- Let the lifecycle module decide when update requires default Installed Source validation while the bootstrap module retains the validation implementation.
- Keep classic lifecycle state independent from capability-pack state and projections.
- Do not add capability-pack approval, digest, serialized-plan, or stale-plan guarantees as part of this refactor.
- Preserve the existing user-visible contract: state schema and location, legacy reads, flags, relevant output, warnings, exit behavior, recovery, idempotency, and safe uninstall.
- Migrate incrementally while keeping checks green, but finish with one architecture: no forwarding modules, duplicate policy, or old CLI lifecycle implementations.
- Treat ADR 0003 as the durable architecture source for this work.

## Testing Decisions

- Make the lifecycle facade the primary behavioral test seam for planning, ownership, recovery, application, persistence, and verification.
- Test Preview as read-only for every operation by asserting no filesystem writes, state publication, directory creation, or external commands.
- Test Apply against exact opaque plans and verify structured results, warnings, actionable failures, and persisted outcomes.
- Exercise state creation, replacement, legacy reads, corruption, atomic-publication failures, recovery-required state, and final confirmation through the lifecycle module.
- Exercise install, update, and uninstall with sandboxed HOME, XDG configuration, source roots, PATH, and Homebrew prefixes.
- Use fake command adapters and an injected clock; never depend on the operator's Homebrew, Engram, processes, or configuration.
- Use the real sandboxed filesystem rather than a broad filesystem mock. Retain focused internal failure injection for atomic persistence and rollback cases.
- Preserve command-level tests for flags, dry-run rendering, relevant messages, warnings, and exit behavior.
- Retain a small end-to-end sandbox lifecycle covering install, update, doctor observation, interrupted recovery, and uninstall preservation.
- Move policy-heavy tests out of the CLI package as each operation migrates; delete tests that only protect obsolete forwarding structure.
- Finish with the deletion test: removing the former CLI lifecycle modules must not redistribute policy among commands.
- Run focused tests during each slice and the full repository suite before reporting completion.

## Out of Scope

- Redesigning workstation path resolution or the shared path structure.
- Deepening setup-health diagnosis; only the lifecycle observation it will consume is included.
- Changing Installed Source initialization behavior.
- Combining classic lifecycle state with capability-pack state.
- Changing capability-pack activation, update, deactivation, reconciliation, approval, or readiness behavior.
- Adding new user-facing lifecycle commands, flags, approval prompts, plan serialization, or migrations.
- Changing the classic state path or schema except to preserve already-supported legacy reads.
- Broad filesystem abstractions or adapters without a second real implementation.
- Opportunistic cleanup outside the lifecycle ownership refactor.

## Further Notes

- ADR 0003 records the accepted architecture and its rationale.
- The migration order already confirmed is state and observation, then lifecycle operations, CLI wiring, test migration, old-policy deletion, and full verification.
- Setup-health deepening should be grilled after the lifecycle observation interface is implemented or stable.
