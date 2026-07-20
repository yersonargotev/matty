---
name: deliver-packy-issue
description: Deliver a named Packy GitHub issue end to end through a local implementation-review loop, pull request, green CI, merge, and cleanup. Use when the user explicitly asks for complete issue delivery.
---

# Deliver Packy Issue

Read the complete [workflow contract](../../../workflows/packy-issue-delivery.md)
and [repository instructions](../../../AGENTS.md) before mutating project or
tracker state. The contract owns delivery behavior; keep this skill as its thin
orchestrator.

## 1. LOCAL — Qualify

Run **LOCAL — Qualify** from the contract.

**Complete when:** the contract's LOCAL Qualify criterion is satisfied.

## 2. LOCAL — Implement-review loop

Run the contract's **LOCAL — Implement-review loop**, applying `delegation`,
`implement`, and `code-review` at their declared points.

**Complete when:** the contract's LOCAL Implement-review criterion is
satisfied.

## 3. NON-LOCAL — Deliver

Run **NON-LOCAL — Deliver** from the contract.

**Complete when:** the contract's NON-LOCAL Deliver criterion is satisfied.
