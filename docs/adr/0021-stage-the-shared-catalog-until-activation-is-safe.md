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
