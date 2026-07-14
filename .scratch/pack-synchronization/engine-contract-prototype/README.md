# PROTOTYPE: deterministic synchronization engine contract

This is throwaway logic code for deciding the repository tool's deep interface. It is **not** production synchronization code and is not wired into the public Matty CLI.

Run every sandboxed contract case:

```sh
go test ./.scratch/pack-synchronization/engine-contract-prototype
```

Inspect human and structured plans:

```sh
go run ./.scratch/pack-synchronization/engine-contract-prototype --scenario success
go run ./.scratch/pack-synchronization/engine-contract-prototype --scenario invalid-provenance
go run ./.scratch/pack-synchronization/engine-contract-prototype --scenario idempotent --format json
go run ./.scratch/pack-synchronization/engine-contract-prototype --scenario rollback
go run ./.scratch/pack-synchronization/engine-contract-prototype --scenario historical
```

## Proposed deep interface

The repository-only tool is a thin renderer around two engine operations:

```text
Check(CheckRequest) -> sealed deterministic Plan
Apply(ApplyRequest{same inputs, exact Plan, compatibility evidence}) -> ApplyResult
Recover(repositoryRoot) -> recovery result
```

`Check` resolves `latest stable`, an explicit prerelease tag, or a full commit SHA; acquires the immutable commit into a new temporary directory; verifies repository identity, release/tag/commit evidence, the old lock, current vendored bytes, runtime pack manifests, and the explicit allowlist; then emits a canonical human/JSON plan. It never writes the repository.

`Apply` accepts only the exact sealed plan, rechecks repository preconditions and review evidence, materializes and verifies a complete replacement `bundle/` beside the current one, and commits the source-wide update as one bundle transaction. Pre-commit failures leave the repository untouched. Commit failures restore the old bundle; a small recovery marker makes an interrupted swap recoverable. A successful second check is a no-op.

The plan reports file additions/removals/modifications, configured resource additions/removals, upstream-path-only moves, unselected upstream discoveries, affected packs, non-reducible SemVer floors, and packs needing semantic evidence. Generated locks contain no wall-clock field: provider event timestamps may be evidence, but an observation timestamp would make identical inputs produce different plans and lock churn.

## Boundaries

- **Deterministic engine:** owns selection rules, trust admission, allowlist enforcement, byte/hash verification, diffing, SemVer floors, plan sealing, staging, atomic repository mutation, and recovery. It returns values and errors; it does not print, call `gh`, open PRs, or ask a model to classify content.
- **Public-GitHub gateway:** observes GitHub metadata and downloads one exact commit into a caller-supplied empty directory. It cannot decide trust or write the repository. The initial production implementation can be concrete rather than a speculative provider plugin system.
- **Maintainer workflow:** chooses manual refs, obtains human/AI semantic classifications, invokes Check/Apply, renders the brief, and asks a human to approve the resulting PR.
- **GitHub publication:** a later narrow job rechecks the candidate and creates/updates a branch and PR with only the permissions accepted in the trust policy. It never enters the deterministic engine.

## Historical-version experiment

The prototype makes both candidates executable:

1. `contract-snapshot` retains only old manifest/contract facts. It can explain status and compare versions, but after current bytes change it cannot reproduce the old projection for reconcile or repair.
2. `immutable-artifact` retains an internal, non-selectable artifact for the old pack version. It keeps old bytes available for status, reconcile, deactivate, and update comparison without making historical activation or downgrade public features.

The prototype therefore recommends the immutable-artifact contract. A contract-only snapshot is insufficient unless active projections independently copy and own every old resource byte, which becomes another artifact store under a different name.

## Atomicity caveat made explicit

The prototype's transaction boundary is the repository's complete `bundle/` directory because resources, pack manifests, compatibility evidence, and `sources.lock.json` must change together. A portable directory swap is failure-atomic with rollback and crash recovery, but not literally invisible to concurrent readers during the two renames. Production design must either serialize readers/writers with the transaction lock or adopt a generation pointer / platform-specific exchange primitive. The prototype prefers serialization as the smallest initial contract.

## Accepted verdict

Accepted through HITL review on 2026-07-14. Use the `Check` / `Apply` / `Recover` contract above; retain internal, immutable, non-selectable historical pack artifacts with their bytes; and serialize every repository reader and writer around the complete `bundle/` replacement transaction. Keep compatibility classification and GitHub publication outside the deterministic engine, omit wall-clock observation timestamps from deterministic locks, and require clean reacquisition plus full verification before Apply commits the sealed plan.
