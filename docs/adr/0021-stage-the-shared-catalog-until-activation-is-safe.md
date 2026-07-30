# Stage the Shared Skill Catalog until activation is safe

Matty imports, validates, and packs the complete Shared Skill Catalog, but does
not advertise it to Pi `0.83.0` through either `pi.skills` or
`resources_discover`. The catalog remains staged and Matty activation remains
degraded while the Activation Safety Gate from ADR 0002 is blocked.

Declaring the catalog through `pi.skills` would load skills before Matty can
validate the active set. Pi may also filter individual package resources, but
does not expose the resulting skill diagnostics or a public removal operation.
Matty could therefore observe neither an atomic catalog nor reliably roll back
a partial one.

Returning the catalog later from `resources_discover` would let Matty validate
its own files first, but those paths bypass normal package-resource filtering
and are appended after external skills have already won first-name collisions.
Pi exposes only the winning commands at that point. This route would violate
Pi's filtering authority without satisfying the universal pre-model collision
guarantee, so Matty does not use it as an approximation.

The package instead contains:

- the verbatim 68-file upstream snapshot;
- a manifest recording all 22 members, upstream repository and commit, original
  snapshot digest, and current Matty release digest;
- one build-time validator for missing, extra, duplicate, renamed, modified, or
  unsupported `ask-matt` routes.

This preserves the imported source and makes future activation work
reproducible without claiming that the workflow is available today. Activation
requires a Pi seam that preserves filtering authority and lets Matty prove
all-or-none reserved-name ownership before any model path can consume a
colliding definition, or a deliberate revision of the product contract with
equivalent safety evidence.

## Rejected extension-owned expansion

Matty cannot recover the required guarantee by registering its own
`skill:<name>` extension commands and expanding the packaged files itself.
Pi tries an extension command before native skill expansion in the simple
invocation path, but that apparent precedence is not stable:

- Pi still serializes loaded external skills into the system prompt, including
  their names, descriptions, and paths. Command shadowing does not remove a
  colliding definition from model context.
- If another extension registers the same command, Pi renames every duplicate
  with numeric suffixes. The unsuffixed `/skill:<name>` then remains available
  to native skill expansion.
- Loading the catalog only as extension-owned files would bypass Pi's
  per-resource filtering authority instead of detecting an individually
  filtered catalog.

This design can validate Matty's own files atomically, but it cannot reserve
their names or prove that colliding external content stays outside every model
path. It is therefore not an equivalent Activation Safety Gate.

## Host capability re-audit

On 2026-07-30, `@earendil-works/pi-coding-agent@0.83.0` remained the latest
public package at commit `845d6ff1f6643aba440341cce877ce1c43ebbc39`.
The public repository head
`6bcc29a06cee8b9ac1643df901468e3c51d6a48f` contained no relevant coding-agent
source or documentation change after that release.

Pi internally records first-wins skill-collision diagnostics, but its extension
interface still exposes only the winning commands and no removal operation.
`resources_discover` remains additive and late, resource filtering remains
individual, and a custom provider's `streamSimple` can still receive context
without a universal Matty interception point. The SDK-level `skillsOverride`
option is available only to code constructing the Pi host; an installed Matty
extension does not control that seam.
