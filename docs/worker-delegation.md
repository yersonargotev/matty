# Worker delegation

The `worker` Matty Role is the delegated implementation path. Before a child
starts, Matty validates a Capability Contract with the exact worker tool
allowlist, canonical trusted working tree, validated temporary paths, and a
writer cardinality of one.

A worker may read, create, and edit files in the trusted working tree, write to
the validated temporary paths, install project-local dependencies, and run
project checks. Single Writer uses a repository-keyed lease so separate Matty
parent processes sharing a repository and system temporary root cannot run
workers concurrently. The Worker Guard reserves Matty's lease path from worker
mutation. Parallel-writer contracts fail Capability Preflight.

The Worker Guard blocks recognized `gh` access, Git index and reference
mutation, global installation, writes outside the validated roots, and writes
to real user-configuration paths. It is a best-effort command and path policy,
not a security sandbox or a boundary against a malicious child process.

The parent remains responsible for reviewing the worker's filesystem changes
and for all integration and external-state operations, including commits,
pushes, pull requests, review submission, merges, releases, and user
configuration.
