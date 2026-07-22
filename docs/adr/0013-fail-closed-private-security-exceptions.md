# ADR 0013: Fail closed on private-security exceptions

## Status

Accepted.

## Context

Packy's protected Governance workflow must revoke authorization when its
evidence changes. GitHub Actions can react to issue and workflow-run changes,
but it does not expose `repository_advisory` as a workflow trigger. A
repository-only implementation that accepts a private security advisory could
therefore leave a successful authorization status on a pull request after the
advisory is withdrawn, closed, or otherwise changed.

Polling would narrow but not eliminate that stale-authorization window. A
GitHub App or webhook could provide the missing event, but installing and
authorizing external infrastructure is outside issue #169.

## Decision

The repository-only Governance workflow recognizes the `private-security`
declaration grammar but denies every such exception. Both the protected adapter
and the pure validator enforce the denial, and the workflow has no permission
or code path for reading private advisory records.

Approved issue authorization, urgent-revert exceptions, and canonical
automation exceptions remain supported. Private-security support may be added
only with separately authorized infrastructure that immediately recomputes
every affected pull request on every relevant advisory change.

## Consequences

- A private security fix cannot use the declared exception until external
  recomputation infrastructure is approved and deployed.
- Repository-only governance cannot publish or retain a green result from
  private advisory evidence.
- Polling is not an acceptable substitute because it permits stale success.

## Enforcement

Governance fixtures require deterministic private-security denial. Repository
validation forbids advisory-read permission and advisory API access in the
Governance workflow. The adapter fails before collecting exception metadata,
and the domain validator rejects even forged trusted metadata.
