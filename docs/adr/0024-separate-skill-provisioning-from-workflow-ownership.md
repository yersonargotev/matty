# Separate skill provisioning from workflow ownership

Packy remains the supported provisioner of one authoritative global Matty Skill
Pack, while `@yargote/matty` owns and executes Matty Workflows without bundling
competing copies of Matt Skills. Matt Skills are unchanged reviewed snapshots
with upstream repository, commit, digest, and license; Matty validates the
identity, provenance, content, and availability of the subset required by a
workflow without depending on Packy's runtime receipt or trusting names alone.
Setup and remediation are visible and user-directed, and a missing or mismatched
skill blocks only the affected workflow or gate. Matty reports real upstream
limitations for human decision instead of silently forking, instrumenting, or
redefining a skill.
