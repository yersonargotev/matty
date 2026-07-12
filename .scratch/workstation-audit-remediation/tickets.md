# Tickets: Workstation audit remediation

These tickets address the capability-pack defects and cleanup improvement discovered during sandboxed build, automated, release, and real-tool validation on macOS.

Work the **frontier**: any ticket whose blockers are all done. Four tickets can start immediately; automation-visible blocked plans follows managed-drift repair.

## Converge Engram activation on Codex

**What to build:** Make Engram activation on Codex converge against the real Engram CLI by establishing one authoritative writer for Engram-owned configuration, while preserving Matty's immutable preview, consent, verification, and recovery guarantees.

**Blocked by:** None — can start immediately.

- [x] Activating Engram on Codex with a compatible real Engram binary completes all approved phases and fresh verification without projection fingerprint conflict.
- [x] The resulting MCP configuration and Engram-owned companion settings match the real Engram setup contract rather than a competing Matty representation.
- [x] A failed or interrupted external setup produces truthful recovery state, and repeating the lifecycle verb presents an actionable fresh plan that can converge.
- [x] Sandboxed compatibility coverage exercises the real external command boundary closely enough to detect changes in Engram's emitted Codex configuration.
- [x] Engram activation on OpenCode and Matty activation on both surfaces remain working.

## Isolate readiness by pack and surface

**What to build:** Derive readiness independently for each pack/surface pair so drift or failure belonging to one active pack cannot degrade another pack whose own projections and runtime evidence remain valid.

**Blocked by:** None — can start immediately.

- [x] Engram-specific unmanaged or drifted Codex projections do not make Matty/Codex report unconfigured when all Matty-owned projections verify.
- [x] Each configured, authorized, and usable result is derived only from evidence relevant to the requested pack/surface pair.
- [x] Surface-wide status still reports each affected pair truthfully without hiding shared-host conflicts.
- [x] Tests cover two active packs on one surface with one healthy pair and one failed or drifted pair.

## Repair drift in Matty-owned projections

**What to build:** Let reconcile repair drift in projections whose current ownership record proves they are Matty-managed, while continuing to preserve genuinely unmanaged or contributor-owned content.

**Blocked by:** None — can start immediately.

- [ ] Editing a Matty-owned projection produces a reconcile preview that explicitly proposes restoring the catalog-current desired content.
- [ ] Applying the approved repair restores the projection and returns the affected pack/surface pair to configured readiness.
- [ ] Content without valid Matty ownership remains protected and is never silently overwritten or deleted.
- [ ] Shared projections retain contributor-safe behavior when one pack is repaired.
- [ ] Coverage includes Codex and OpenCode drift plus dry-run non-mutation.

## Expose blocked lifecycle plans to automation

**What to build:** Give scripts and humans an unambiguous result when update, reconcile, or recovery cannot produce an applicable plan because ownership protection blocks every required change.

**Blocked by:** Repair drift in Matty-owned projections.

- [ ] An applicable dry-run remains successful and clearly describes the actions that could be approved.
- [ ] A fully ownership-blocked or otherwise non-actionable lifecycle request is distinguishable through stable output and a non-success exit status.
- [ ] Mixed plans clearly separate applicable actions from preserved or blocked projections without overstating that repair succeeded.
- [ ] Interactive Apply cannot claim success when required changes were blocked or no verified desired state was reached.
- [ ] CLI tests cover update, targeted reconcile, surface-wide reconcile, and recovery previews.

## Clean up Matty-created empty containers

**What to build:** Make uninstall return a fresh installation closer to its original filesystem state by removing empty containers created exclusively by Matty, without broadening deletion authority over pre-existing or contributor-owned paths.

**Blocked by:** None — can start immediately.

- [ ] Matty records or otherwise proves which empty container files and directories it created before removing them.
- [ ] Uninstall removes newly-created empty Matty state, skill, Codex, and OpenCode containers when they contain no unmanaged content.
- [ ] Pre-existing containers, non-empty files, and contributor-owned content are preserved byte-for-byte.
- [ ] Dry-run reports cleanup candidates without mutating the filesystem.
- [ ] A sandboxed install/uninstall lifecycle verifies both pristine cleanup and preservation cases.
