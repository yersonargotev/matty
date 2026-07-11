## Destination

Reach an implementation-ready product and architecture specification for opt-in, composable Matty capability packs, proven by the real `matty` and `engram` packs across Codex and OpenCode.

## Notes

- This map plans the capability-pack system; it does not implement it.
- Consult `domain-modeling`, `codebase-design`, `research`, `prototype`, and `grilling` as each ticket requires.
- Use CodeGraph before architecture or symbol discovery. Verify host behavior, filesystem writes, configuration, installers, and CLI behavior with official documentation and sandboxed real commands or tests.
- Matty core is always available; every capability pack is opt-in and activated globally per CLI surface.
- Packs compose additively, declare provided/required/conflicting capabilities, and never resolve conflicts through silent last-writer-wins behavior.
- The first catalog is Matty-owned. Its format is a Matty-native manifest with per-surface adapters.
- The first proof uses only `matty` and `engram`; `web` and `mobile` are deferred until their concrete contents are requested.
- Disabling a required pack is rejected without cascading; shared Matty-owned resources remain while any active pack needs them.

## Decisions so far

<!-- Resolved ticket pointers are appended here. -->

- [Map host capability surfaces](tickets/01-map-host-capability-surfaces.md) — Verified the shared skill/MCP concepts and the host-specific adapters required for every lifecycle and non-portable surface.
- [Define pack manifest contract](tickets/02-define-pack-manifest-contract.md) — Fixed the strict portable manifest, four-resource vocabulary, and complete `matty`/`engram` proof cases.
- [Define trust and consent boundaries](tickets/03-define-trust-and-consent-boundaries.md) — Separated previewed reconciliation from host trust, authentication, executable-code consent, and destructive human checkpoints.
- [Place pack domain and surface seams](tickets/04-place-pack-domain-and-surface-seams.md) — Placed portable planning and ownership behind one deep capability-pack facade with sibling host adapters and narrow bundle/tool seams.
- [Prototype reconciliation state machine](tickets/05-prototype-reconciliation-state-machine.md) — Fixed exact-plan approval, stale rejection, bounded atomicity, recovery, readiness, and shared ownership semantics.

## Not yet specified

- The final implementation slices and blocking graph depend on the resolved module, state, lifecycle, and verification contracts.

## Out of scope

- Implementing the capability-pack system during this wayfinding effort.
- Third-party packs, remote sources, marketplaces, signing, and public ecosystem policy.
- Defining or validating `web` and `mobile` packs until their concrete skills, MCPs, and other contents are requested.
- Repository-scoped or per-session activation and turning Matty into a runtime launcher/orchestrator.
- Automatic migration from the pre-pack Matty state; rollout will use a documented, sandbox-verified manual transition.
