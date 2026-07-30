# Give researchers two bounded write zones

The `researcher` role receives a Matty-owned file tool instead of general
`write`, `edit`, or `bash`. For each run, the tool permits multiple files only
inside `$TMPDIR/matty/research/<run-id>/` and permits one Research Report at a
path prevalidated by the parent agent. The report follows the repository's
existing research-note convention or defaults to `docs/research/<slug>.md`.
The tool rejects external absolute paths, traversal, symlink escapes, and
unauthorized overwrites, and returns both workspace and report paths. This
preserves the upstream single-report contract while allowing deep working notes
without repository-wide write access. A Research Workspace remains available
for the parent session and its path is reported to the user. Matty removes it on
clean session shutdown; after an unclean exit, a later startup may remove only
marker-bearing Matty research workspaces older than 24 hours after validating
their resolved paths. Research Reports are never part of cleanup.
