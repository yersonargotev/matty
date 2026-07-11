Type: prototype
Status: resolved
Blocked by: 02, 04

## Question

What desired-state and ownership state machine makes enable, disable, update, conflict rejection, shared-resource retention, partial failure, retry, and recovery predictable without requiring Matty to become an always-on runtime?

## Answer

Use the throwaway [reconciliation state-machine prototype](../reconciliation-prototype/README.md) as the concrete contract.

Keep inspection, planning, and application semantically strict behind the deep `internal/capabilitypack` facade. Preview composes side-effect-free adapter inspection with pure planning and returns an immutable, canonically encoded plan. Human approvals bind the exact plan digest and its typed phases. Apply accepts that approved plan, never the original request, and may re-inspect only to reject stale preconditions and verify outcomes; it never silently replans.

Bind every plan to both the Matty intent revision and fingerprints of all relied-on host observations. Apply first locks Matty state and rejects changed intent or host state with zero actions. Once both checks pass, it atomically persists the approved target intent and an applying journal before executing side effects, so crashes and partial failures cannot forget the approved destination. Recovery always begins with fresh inspection and produces a new plan requiring approval; it is never blind replay or mutation of the failed plan.

Guarantee atomicity only for local actions an adapter can stage, validate, back up, and safely restore. External or non-reversible actions are explicit ordered barriers after local commit. Their failure stops later work, preserves truthful completed facts, and enters `recovery-required`; Matty performs no speculative compensation. Typed approval receipts separately cover activation/reconciliation, executable/external effects, and destructive cleanup. No generic or unattended consent bypasses required human checkpoints.

Track ownership per verified host projection, including its stable identity, contributor set, and last verified fingerprint. Activate and update add or replace contributions; incompatible shared contributions reject planning. Deactivate removes a contributor but retains shared projections while any contributor remains. Last-contributor deletion requires destructive approval and an unchanged verified fingerprint. Reconcile repairs toward current intent without changing activation. Drifted, ambiguous, or unmanaged targets are preserved for human action rather than overwritten, removed, or silently adopted.

Keep the reconciliation attempt separate from readiness. Attempts progress through proposed, approved, applying, applied, and verified, or terminate stale/recovery-required. Fresh inspection derives configured, authorized, and usable independently; pending human action records the reasons blocking authorization or usability. A verified apply may therefore leave a pack configured but still awaiting host trust, authentication, or reload.

`internal/cli` crosses only conceptual Preview, Apply, and Status operations. `internal/codex` and `internal/opencode` inspect and execute host-specific actions behind the interface owned by `internal/capabilitypack`; they do not decide ownership, global ordering, readiness, or recovery. This remains an on-demand installer/configurator state machine and requires no always-on Matty runtime.
