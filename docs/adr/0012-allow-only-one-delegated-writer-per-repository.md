# Allow only one delegated writer per repository

Matty v1 enforces a Single Writer for delegated implementation: at most one
`worker` may be active for a repository, while non-writing roles may execute in
parallel. No imported workflow currently requires parallel writers, so Matty
rejects such a Delegation Contract instead of sharing one working tree between
writers. Parallel implementation may be added later only with explicit
isolation and integration semantics, such as dedicated worktrees.

The active worker may edit the trusted working tree, install project-local
dependencies, and run checks, but a best-effort Worker Guard blocks Git index
or reference mutation, `gh`, global installation, external-path writes, and
real user-configuration writes. The main agent remains responsible for
reviewing and integrating the worker's changes.
