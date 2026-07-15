# ADR 0008: Orchestrate compatibility classification outside packsync

## Status

Accepted.

## Context

Synchronization can determine mechanical compatibility floors from selected
resource changes, but semantic compatibility still requires untrusted AI or
explicit human evidence. Classification must compose the canonical Check and
transaction seams without giving a classifier authority over plans, versions,
repository mutation, or recovery. The later maintainer workflow also needs one
internal classification capability that has no GitHub publication knowledge.

[ADR 0007](0007-serialize-complete-bundle-transactions.md) makes
`internal/packsync` the owner of complete bundle Apply and Recover. This
decision specifies the classification boundary that composes that transaction.

## Decision

`internal/packclassification` owns compatibility-classification orchestration.
It derives exactly one deterministic request per affected pack from a canonical
sealed `packsync` Check plan. Requests carry the exact plan, candidate and base
identity plus the engine-derived current version, mechanical floor, reasons and
changes.

AI mode retries the same per-pack request a bounded number of times. Model
unavailability or invalid evidence remains blocked, and orchestration never
switches to human evidence implicitly. Human mode is explicit and consists of
two bound operations: an inspection dispatch followed by an evidence-supply
dispatch carrying the inspection identity.

`internal/packsync` remains the sole owner of affected-pack derivation,
mechanical floors, canonical-plan admission, exact next-version calculation,
evidence validation, classified manifest materialization, Apply and Recover.
Classifier output is evidence only. Apply validates the complete evidence set
before acquisition and again under the shared bundle lock, then uses the
existing sibling-staged transaction and Matty-owned validation seam.

Evidence is canonical per pack and records classifier type and identity,
rationale, exact current and proposed versions, changed observable-contract
aspects, mechanical floor, final level, and mandatory migration and actions for
major changes. The complete set is bound to one candidate, plan ID and base SHA;
human sets additionally bind the inspection ID. Mixed modes, missing or
duplicate packs, stale bindings, arbitrary versions and below-floor results
fail closed.

## Consequences

- Classification orchestration can later be adapted by the maintainer workflow
  without importing GitHub, branch, PR or publication policy into the domain.
- AI and human fixtures exercise the same deterministic engine validation and
  transactional Apply boundary.
- Adding a classifier does not grant it authority to lower floors, choose an
  arbitrary version, apply to a checkout, or recover a transaction.

## Non-goals

- A public Matty command or additional distributed binary.
- GitHub writes, workflow publication, maintainer dispatch or real refresh.
- Applying a synchronization candidate to the real Matty checkout.
