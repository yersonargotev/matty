# ADR 0010: Cut over the product identity to Packy

## Status

Accepted.

## Context

The repository historically shipped the Matty product while also owning a
capability pack whose stable semantic ID is `matty`. Reusing one name for both
made product-owned commands, state, automation, distribution, and maintainer
surfaces ambiguous. The Packy v0.1.7 cutover needs one current product identity
without rewriting the accepted decisions and immutable evidence that explain
how the system reached this point.

## Decision

The current product and repository identity is **Packy**. Its module, executable,
state and source roots, host markers, automation, synchronization ownership,
release artifacts, Homebrew formula, maintainer surfaces, and current product
documentation use Packy names only.

The cutover is deliberately incompatible. Packy does not expose a legacy Matty
binary or environment fallback, read legacy product state or markers, or migrate
or delete legacy product-owned files automatically.

The capability pack ID `matty` remains unchanged, including `workflow:matty`,
its resource IDs, bindings, contributors, compatibility tuple, and immutable
history. Accepted ADRs 0001–0009, completed planning artifacts, old tags,
releases, and assets remain authentic Matty history and are not rewritten.

## Consequences

- Current operators and maintainers use only Packy commands, paths, automation,
  repository references, archives, and `Formula/packy.rb`.
- Mixed product/pack surfaces must be edited according to ownership rather than
  by global textual replacement.
- Remaining Matty terms in current code and documentation must denote the
  surviving `matty` pack or explicit legacy-isolation behavior.
- External repository, release, tap, and installation cutover steps must be
  coordinated atomically with the Packy candidate.
