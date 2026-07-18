# ADR 0011: Publish the versioned pack-source schema suite

## Status

Accepted.

## Context

Packy's five synchronization schemas were repository-local workflow files with
repository URLs as their identifiers. External consumers need stable,
dereferenceable identities, while Packy validation must remain deterministic
and must not depend on network access. Producers, validators, fixtures,
runtime behavior, and documentation also need to move as one contract rather
than adopting independently drifting schema identities.

## Decision

The five pack-source schemas form one immutable suite. The initial suite is
`v1.0.0` and its sole canonical checked-in publication tree is
`schemas/pack-source/v1.0.0/`. Every schema `$id` is its exact GitHub Pages URL
under
`https://yersonargotev.github.io/packy/schemas/pack-source/v1.0.0/`, preserving
the existing five filenames.

Repository validation registers all five checked-in documents together by
canonical `$id` and compiles or resolves them locally. Packy runtime validates
domain values directly; it does not resolve schemas or fetch them from the
network. GitHub Pages provides the same checked-in bytes for external
retrieval; deployment is not a second schema source.

Machine consumers always select an exact suite version. There is no machine
`latest` alias, old Matty-ID alias or redirect, runtime registry, or network
fallback. Published suites are complete and immutable. Patch versions may
change annotations without changing accepted instances, minor versions may
expand validation compatibly, and major versions are incompatible. Suite major
`vN` aligns with instance `schema_version: N`; suite versions are independent
of Packy application releases.

## Consequences

- Repository checks enforce the exact complete five-file suite, unique exact
  IDs, valid Draft 2020-12 JSON, offline compilation and resolution, and
  runtime/producer/schema parity.
- Pages publication and post-deployment verification compare hosted resources
  byte-for-byte with the canonical repository files.
- Producers and external consumers use versioned identities and must adopt a
  future complete suite explicitly.
- The clean Packy cutover does not preserve or redirect old schema identities.
