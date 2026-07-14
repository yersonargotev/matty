# PROTOTYPE: source configuration and provenance lock contract

This throwaway logic prototype asks whether one source-owned snapshot plus explicit pack/resource bindings is the smallest model that can represent multi-source synchronization without putting maintenance metadata in runtime `pack.json` files. It makes the proposed JSON concrete for the current `matty` pack and exposes the important failure cases before production code exists.

Run it from the repository root:

```sh
go run ./.scratch/pack-synchronization/source-contract-prototype
```

Use `n` and `p` to move through the current bundle, a byte-identical candidate, a moved tag, a replaced repository, and a missing selected file. For an automated sanity check, run:

```sh
go run ./.scratch/pack-synchronization/source-contract-prototype --check
```

## Concrete files

- `example.sources.json` is the hand-maintained input. One source declares its provider, reviewable repository locator, update selector, and explicit bindings to `(pack_id, kind, resource_id)` plus an upstream path. The runtime pack manifest remains the authority for the vendored destination.
- `example.sources.lock.json` is generated and reviewed with vendored bytes. It records repository identity, release/tag/commit/signature evidence, the resolved destination, every file's size and SHA-256, per-resource aggregates, and one source-snapshot aggregate.

The proposed production names are `bundle/sources.json` and `bundle/sources.lock.json`. The source configuration is maintainer-owned; the lock is generator-owned and must never be hand-edited. A single source may bind resources in multiple packs, but all its bindings advance atomically to one commit.

## What the prototype says

- The config needs only source identity/selection and explicit resource bindings. It does not duplicate the destination path already owned by each runtime pack manifest.
- The lock repeats resolved upstream and vendored paths deliberately: it is self-contained review evidence and can verify both sides without consulting mutable provider state.
- Aggregate hashes are SHA-256 over sorted, framed child records. A resource hashes `path NUL size NUL sha256 LF`; a source snapshot hashes `pack_id NUL kind NUL resource_id NUL upstream_path NUL vendored_path NUL resource_sha256 LF` in configured order.
- Admission and byte verification are one fail-closed decision. Valid provenance with local drift is still blocked; no partial resource or lock update is representable.
- The current fixture reproduces the audit: 23 selected resources, 45 locked files, 40 byte-identical files, and 5 locally modified files.
- Migration replaces the partial root `skills-lock.json` only after the five adaptations are moved, adopted upstream, or rejected. There is no compatibility reader or dual-write period: generate the complete new lock and remove the old informational lock in the same implementation change.

## Verdict to capture after the human review

Accepted on 2026-07-14. Use `bundle/sources.json` for maintainer-owned configuration and `bundle/sources.lock.json` for generator-owned evidence. Keep `(pack_id, kind, resource_id)` plus upstream path as the explicit binding, derive the vendored destination from the runtime pack manifest, lock one atomic snapshot per source, and migrate from `skills-lock.json` without a compatibility reader or dual writes.
