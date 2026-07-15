# ADR 0007: Serialize complete bundle transactions

## Status

Accepted.

## Context

The Matty bundle contains runtime pack manifests, selected resources,
compatibility evidence, immutable history, source configuration, and generated
provenance. Replacing those artifacts independently would let concurrent
readers combine facts from different bundle generations. Portable directory
replacement also requires two renames, leaving no `bundle/` path between them.

The accepted synchronization engine and source-provenance contracts require
one full-bundle transaction without a second lock owner, compatibility reader,
or dual-write migration.

## Decision

`internal/bundletransaction` exclusively owns one repository-local exclusive
lock for complete bundle observations and mutations. It locks the stable parent
directory inode that owns `bundle/`; it creates no lock file or read-time
filesystem state. `packsync` Check holds it only while materializing one local
observation. `skillbundle` and `capabilitypack` hold the same lock while
materializing their bundle observations. Apply and Recover hold it throughout
mutation, verification, cleanup, or repair.

`internal/packsync` owns transactional replacement. Apply reacquires the exact
candidate outside the lock, freshly revalidates the sealed plan and all local
authority under the lock, constructs and validates a complete sibling-staged
bundle, and records a durable sealed recovery marker. The bundle swap consists
of exactly two directory renames: current to backup, then staged to current.
Backup and marker remain until the installed or restored tree hash and
Matty-owned validation succeed.

Recover uses only the canonical marker paths, recorded phase, and observed
old/new tree hashes. It rolls back a one-rename state, completes cleanup for an
installed new bundle, and blocks on absent, ambiguous, manipulated, or
hash-incompatible evidence. It does not reconstruct intent from partial
content.

The first production provenance transaction is the sole bootstrap exception:
it may generate `bundle/sources.lock.json` and remove informational
`skills-lock.json` only when every configured selected byte already equals the
exact candidate and no selection or pack-version change is pending. No public
CLI or maintainer synchronization operation is introduced by this decision.

## Consequences

- Readers observe one complete old or new bundle generation, never a mixture.
- Read-only operations create no lock artifacts and preserve sandbox purity.
- Portable recovery does not depend on platform-specific directory exchange.
- Interrupted swaps fail closed with sufficient durable evidence for one
  deterministic repair.
- Later classification and publication slices can compose the internal seam
  without acquiring another lock or bypassing its validation boundary.

## Enforcement

Focused tests cover lock participation, exact-candidate acquisition outside the
lock, stale-plan boundaries, the four transaction fault points, marker and hash
tampering, incomplete or unexpected siblings, recovery cleanup, concurrency,
and Apply/Check idempotence. The Matty validation allowlist includes
`internal/bundletransaction`; vendored or hostile upstream content remains
outside the build, vet, test, and race package set.
