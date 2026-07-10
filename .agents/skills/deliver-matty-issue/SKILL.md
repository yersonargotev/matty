---
name: deliver-matty-issue
description: Deliver a named Matty GitHub issue end to end through validation, implementation, review, pull request, merge, and cleanup. Use when the user explicitly asks for complete issue delivery.
---

# Deliver Matty Issue

Read the complete [workflow contract](../../../workflows/matty-issue-delivery.md)
and [repository instructions](../../../AGENTS.md) before mutating project or
tracker state. The contract owns delivery behavior; keep this skill as its thin
orchestrator.

## 1. Qualify

Run **Qualify** from the contract. Apply `diagnosing-bugs` for the bug branch.

**Complete when:** the contract's Qualify criterion is satisfied.

## 2. Implement

Run **Implement** from the contract. Apply `tdd` for the bug branch when it has
a valid regression seam.

**Complete when:** the contract's Implement criterion is satisfied.

## 3. Prove

Run the contract's **Prove** phase through `code-review`.

**Complete when:** the contract's Prove criterion is satisfied.

## 4. Deliver

Run **Deliver** from the contract.

**Complete when:** the contract's Deliver criterion is satisfied.
