# Ship an inseparable shared skill catalog

Each Matty release ships one complete Shared Skill Catalog that users adopt as
a unit. Individual shared skills cannot be replaced, enabled, or disabled;
projects may only add non-colliding skills. This preserves the workflow's
internal assumptions and gives every Matty release one coherent, testable
behavior, at the cost of per-user customization within the shared workflow. Pi
retains authority to filter package resources, but Matty treats any resulting
Incomplete Shared Skill Catalog as failed activation rather than a supported
partial profile. If Pi filters the diagnostic bootstrap itself, Matty is simply
disabled and cannot diagnose its own absence.
