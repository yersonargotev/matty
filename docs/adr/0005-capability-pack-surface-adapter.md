# ADR 0005: Capability-pack uses one complete surface adapter

## Status

Accepted.

## Context

Capability-pack status and lifecycle operations currently discover host
behavior through an activation adapter, optional lifecycle-aware interfaces, a
deprecated status inspector, and a separately registered readiness inspector.
Both supported hosts already provide the complete behavior, so the fragmented
seams permit combinations that cannot preserve Matty's safety semantics.

[ADR 0001](0001-matty-v0-architecture.md) establishes Codex and
OpenCode as CLI surfaces owned by a small installer/configurator, while
[ADR 0003](0003-core-lifecycle-deep-module.md) keeps capability-pack lifecycle
separate from Matty core lifecycle. The broader behavior-preserving migration
is specified by the
[capability-pack surface adapter deepening specification](../../.scratch/capability-pack-surface-deepening/spec.md).

## Decision

Capability-pack owns one complete, lifecycle-neutral `SurfaceAdapter` contract
implemented by each supported host module. It exposes one fresh, pure
inspection operation and one separate projection-application operation.

Capability-pack supplies normalized transition facts rather than lifecycle
labels: relevant prior composition, desired composition, ownership residuals,
and already-resolved executable facts. Status, activation, and update inspect
desired projections only; deactivation compares prior and desired composition;
reconciliation additionally includes residual ownership.

Inspection returns one normalized surface observation containing projection
goals and observations, fresh authorization and usability evidence, pending
human actions, and revision facts used to seal deterministic plans. Projection
goals explicitly state presence or absence. Inspection executes no commands,
writes no files or state, and performs no mutation. Projection application is
the adapter's only mutation operation.

A private capability-pack gateway clones transition inputs and adapter output,
validates goals and observations, rejects duplicate or malformed projection
identities and incompatible goal/action combinations, and canonicalizes
ordering for every caller. It is not a second public module seam.

Capability-pack remains the sole owner of lifecycle meaning, composition,
ownership authority, blockers, typed consent, destructive authorization,
recovery, configured-to-authorized-to-usable readiness progression, plan
sealing, stale-plan rejection, and verification. Codex and OpenCode modules own
their host syntax, paths, projection translation, readiness evidence, and
authorized projection application. Only neutral filesystem, fingerprinting,
and safe-write primitives may remain shared across hosts.

## Compatibility and migration

This is a behavior-preserving refactor. Existing projection actions, blockers,
consent phases, ownership protection, pending human actions, fingerprints,
verification results, readiness, recovery, filesystem effects, CLI output, and
exit behavior must remain unchanged.

Migration must route status, preview, stale-plan preflight, post-apply
verification, and recovery through the private gateway before deleting the
obsolete seams. Once both host adapters implement the complete contract, remove
the activation-specific and optional lifecycle interfaces, deprecated status
inspector, separate readiness registration, removal-candidate side channel,
lifecycle-specific inspection helpers, compatibility fallbacks, and obsolete
OpenCode activation package. Do not retain forwarding wrappers or dual
ownership.

## Consequences

- Every supported host has one complete inspection and application contract.
- Host modules translate normalized facts but cannot acquire portable lifecycle
  policy or deletion authority.
- Freshness, validation, cloning, and canonical ordering are enforced once for
  all capability-pack use cases.
- Partial host adapters and mismatched inspection/readiness combinations are no
  longer representable after migration.
- New cleanup behavior or other user-visible lifecycle changes require a
  separate specification and decision.
