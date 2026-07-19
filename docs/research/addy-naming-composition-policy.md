# Addy naming, conflicts, and composition policy

## Decision question

How should `addy`-namespaced internal identities preserve conflict-free
upstream invocations while detecting and resolving collisions with native host
capabilities and resources contributed by other active packs?

This decision consumes the repository-local
[evidence](addy-naming-composition-evidence.md), the decided
[Addy observable contract](addy-observable-contract.md), and the
[capability mapping](addy-capability-mapping.md). It plans the contract; it does
not implement the required model seams or activate Addy.

## Separate naming layers

Every Addy resource has one portable identity:

```text
(pack_id=addy, resource_kind, upstream_logical_id)
```

The upstream logical ID follows the selected upstream capability's stable
logical name. It is not a host path, invocation spelling, projected filename,
or alias. Codex and OpenCode adapters translate this identity into
surface-specific projections.

Addy-provided and Addy-required capability contracts are also namespaced, for
example `addy.workflow.build`. An unnamespaced capability is valid only when
Packy deliberately defines it as a cross-pack contract with explicit
compatibility semantics. A coincidentally equal generic label cannot satisfy
an Addy dependency.

## Default user-facing projection

A surface preserves the upstream user-facing name when that name is
conflict-free. The previously decided surface distinction remains truthful:
Codex exposes workflow invocation as `$name`, while OpenCode exposes `/name`.
The portable resource identity is the same even though the host syntax differs.

There is no precedence rule. Addy never wins because it was activated later,
because its pack ID sorts first, or because its bytes match existing content.
Packy never silently overrides or automatically renames another owner.

## Collision domain and detection

Each complete surface adapter freshly reports its normalized occupied
namespace during inspection. The report includes, where relevant:

- native or reserved invocations;
- paths, filenames, configuration keys, and other host-visible targets;
- unmanaged external resources; and
- projections contributed by active packs.

The adapter applies the host's path, case, and name normalization and reports
which resource kinds share a namespace. Capability-pack compares desired
projection bindings after translation. It detects cross-kind collisions when
the host maps different portable kinds into the same visible namespace.

An unresolved collision on any required Addy projection blocks the whole Addy
activation or update on that surface. It does not invalidate a coherent Addy
projection on the other surface. The blocker identifies the Addy logical
resource, normalized target, other owner or reservation, and available explicit
resolution paths.

## Explicit alias resolution

The user may resolve a collision with a surface-local qualified alias. The
default proposed form is `addy-<upstream-name>`, rendered in native syntax—for
example `$addy-build` on Codex and `/addy-build` on OpenCode.

Packy never selects the alias merely because a collision exists. The user must
explicitly approve the mapping. The mapping is stored against `(surface,
portable Addy resource identity)` in desired activation intent and is sealed
into preview, approval, apply, verification, recovery, status, and update
plans. It changes only the projection binding, not the Pack resource identity,
source binding, or Addy version.

The alias is normalized and collision-checked like any other projection. If it
also collides, the operation remains blocked until the user chooses another
valid alias or removes one contributor from the desired composition.

## Sharing and contributor ownership

Matching bytes do not make two resources shared. Two packs may intentionally
co-own one visible projection only when all of the following hold:

1. both pack contracts explicitly declare the projection shareable;
2. the normalized surface projection identity is identical;
3. the rendered bytes and relevant behavioral declaration are identical; and
4. Packy records every contributor in ownership state.

Otherwise, the overlap is a collision and requires an alias or removal of one
contributor. Later divergence in any sharing precondition blocks update; Packy
does not choose a winner. Deactivation removes one contributor and retains the
projection while a verified contributor remains.

## External ownership boundary

An unmanaged host resource remains externally owned even when its normalized
target and bytes match Addy's desired projection. Addy activation may use an
approved alias or wait for the user to remove the external resource. It never
adopts, overwrites, or deletes that resource implicitly.

Any ownership-transfer facility is a separate explicit workflow outside this
Addy pack specification. The existence of such a future workflow cannot weaken
the Addy activation blocker or grant authority during inspection.

## Alias lifecycle and upgrades

Alias mappings are local activation intent, not source-derived pack content.
They are excluded from Addy's Pack Source provenance lock, source manifest,
and observable-contract version.

An alias remains attached to its portable logical resource across compatible
Addy upgrades. Every lifecycle preview freshly revalidates its normalized host
target and all ownership/sharing assumptions. If upstream removes or changes
the logical identity, Packy blocks with a migration requirement rather than
guessing a successor. A changed invocation promised by the Addy observable
contract remains subject to the previously decided compatibility/versioning
policy even when an existing local alias masks the visible spelling.

## Required model evolution

The current runtime keys resources and host projections directly by `kind:id`.
Implementing this decision therefore requires an explicit separation between:

- portable pack resource identity;
- logical capability contracts;
- per-surface desired projection binding and optional alias;
- normalized observed host occupancy; and
- contributor ownership and explicit sharing declarations.

Capability-pack owns composition, blocker, sharing, lifecycle, and ownership
policy. Codex and OpenCode own normalization, host translation, inspection,
and authorized application. Published schema evolution must follow ADR 0011;
no existing schema version may silently acquire new meaning.

## Answer

Addy resources use `addy`-namespaced portable identities while retaining their
upstream invocation names wherever each surface reports those names as free.
Packy evaluates fresh normalized surface occupancy without precedence or
implicit ownership. Required collisions block the surface atomically and can
be resolved only by explicit composition change, normally a user-approved
surface-local `addy-<name>` alias. Sharing requires mutual declaration plus
identical rendered behavior and recorded contributors. Aliases remain local
lifecycle intent, survive compatible upgrades by logical identity, and never
change source provenance or Pack identity.
