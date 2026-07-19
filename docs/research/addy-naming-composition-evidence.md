# Addy naming, conflicts, and composition evidence

## Evidence question and boundary

This note gathers repository-local evidence for the Wayfinder question
**“Decide Addy naming, conflicts, and composition policy.”** It records current
facts and implementation gaps; it does not implement a new identity model,
activate Addy, or change any host configuration. No upstream content was
executed.

## Architectural ownership

- Capability-pack owns portable composition, ownership authority, blockers,
  lifecycle meaning, plan sealing, and verification. Codex and OpenCode own
  host syntax, paths, projection translation, inspection evidence, and
  authorized application
  ([ADR 0005](../adr/0005-capability-pack-surface-adapter.md#decision)).
- The private capability-pack gateway must reject duplicate or malformed
  projection identities and incompatible goals. Collision policy therefore
  belongs above the adapters; an adapter reports normalized surface facts but
  does not choose a winner.
- Synchronization binds source resources by `pack_id`, `kind`, and
  `resource_id`; ambiguous or divergent exact identity blocks instead of being
  overwritten
  ([ADR 0009](../adr/0009-own-manual-synchronization-orchestration.md#decision)).
- Published Pack Source schemas are exact-version immutable suites, so the new
  identity/projection split cannot be retrofitted by reinterpreting the
  existing v1 suite
  ([ADR 0011](../adr/0011-publish-versioned-pack-source-schema-suite.md#decision)).

## Current portable model

- A decoded [`Pack`](../../internal/capabilitypack/catalog.go) owns one pack ID,
  capabilities, conflicts, and resources. Each resource currently has only
  `Kind`, `ID`, source, and optional command fields; there is no distinct
  visible invocation or projection name.
- Catalog validation requires lowercase kebab-case pack and resource IDs and
  uniqueness by `kind:id` within a pack. Existing checked-in manifests
  consequently use their visible resource names as resource IDs.
- Pack Source bindings already include pack ID, kind, and resource ID and must
  resolve to a unique authoritative manifest resource. That triple is the
  closest existing provenance identity, but runtime composition drops the
  pack ID from its collision key.

The current manifest therefore cannot represent the decided distinction
between Addy's portable `(pack, kind, logical ID)` identity and a different
surface-visible invocation or alias. The execution specification must add an
explicit seam rather than infer it from paths or source bytes.

## Current composition behavior

- [`compose`](../../internal/capabilitypack/composition.go) constructs one
  complete desired composition for the requested surface, includes the other
  active intents on that surface, and closes transitive capability
  dependencies.
- A required capability with zero or multiple providers is a blocker. Declared
  capability conflicts also block; no provider is selected by precedence.
- Portable resources are currently combined by `kind:id`. Byte-identical
  declarations are deduplicated and all pack contributors are recorded;
  differing declarations with the same key block as incompatible
  contributions.
- Status preserves ambiguous shared-contributor evidence rather than hiding
  it. Deactivation retains a verified shared projection for remaining
  contributors, while later divergence blocks update.

Focused tests covering these invariants include
[`composition_test.go`](../../internal/capabilitypack/composition_test.go),
[`deactivation_test.go`](../../internal/capabilitypack/deactivation_test.go),
[`update_test.go`](../../internal/capabilitypack/update_test.go), and
[`status_test.go`](../../internal/capabilitypack/status_test.go).

Current byte equality alone is too weak for unrelated namespaced identities:
two packs can temporarily render the same bytes and later evolve independently.
An explicit mutual sharing contract is needed before contributor co-ownership
is admitted.

## Current host projection behavior

- Codex and OpenCode currently derive skill directories, instruction markers
  or files, MCP keys, and projection IDs directly from `Resource.ID`
  ([Codex adapter](../../internal/codex/surface.go),
  [OpenCode adapter](../../internal/opencode/surface.go)).
- Neither adapter has a portable-name-to-visible-name binding or alias seam.
  The Addy mapping already requires surface-asymmetric workflow invocations:
  Codex truthfully exposes `$name`, while OpenCode exposes `/name`
  ([Addy mapping](addy-capability-mapping.md)).
- Activation inspection observes the complete composition. An existing
  differing target that Packy does not own or cannot repair becomes an
  ownership blocker and is preserved. Matching bytes do not transfer external
  ownership.
- ADR 0005 requires inspection to be fresh and pure and makes projection
  application the only adapter mutation operation. Native/reserved names,
  unmanaged files, host configuration keys, and active Packy projections must
  consequently enter collision decisions through normalized inspection facts.

## Consequences for the decision

The evidence constrains the policy as follows:

1. Portable identity, visible projection, alias, and capability contract are
   separate naming layers.
2. Collision detection is per surface because composition and host namespaces
   are surface-local.
3. Capability-pack decides whether a normalized occupancy conflict blocks or
   is explicitly shared; adapters only translate and observe host facts.
4. Existing unmanaged content remains externally owned and cannot be adopted,
   overwritten, or deleted implicitly.
5. A user-selected alias must be lifecycle intent sealed into plans, not
   upstream provenance or a mutation of the Addy resource identity.
6. Supporting this contract requires explicit Pack/runtime/schema evolution;
   it is not representable by today's direct `Resource.ID` projections.

## Verification performed

The read-only exploration ran:

```text
go test ./internal/capabilitypack ./internal/codex ./internal/opencode
```

The focused packages passed. This proves the cited current behavior only; it
does not validate the future model.
