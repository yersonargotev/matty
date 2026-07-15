---
name: sync-pack-source
description: Synchronize one configured Matty pack source through the canonical manual workflow. Use when a Matty maintainer asks to update, retry, or monitor pack-source synchronization.
---

# Synchronize Pack Source

This is a repository-local maintainer skill. Read the canonical
[operational contract](../../../workflows/pack-source-synchronization.md) before
acting. The workflow and its artifacts own synchronization semantics; this
skill only translates intent, performs remote preflight, dispatches, monitors,
and presents evidence.

## 1. Normalize

Follow [REQUESTS.md](REQUESTS.md). Resolve the source and selector exclusively
from remote `main`, render the exact canonical JSON request, and reject every
ambiguity or forbidden override before any write.

**Complete when:** one schema-valid request is shown verbatim, or the operation
is explicitly blocked with the missing decision named.

## 2. Preflight and attach

Run every remote preflight in [REQUESTS.md]. Ignore the local branch and dirty
state except as informational. If an active or pending run exposes an identical
canonical request, attach to it. Never dispatch when equality cannot be proved.

**Complete when:** repository, access, remote `main`, workflow, configuration,
source, selector, queue, branch, and PR observations all come from GitHub, and
the operation is either attached or admitted for one dispatch.

## 3. Dispatch once

Unless attached, submit the rendered request once to
`.github/workflows/sync-pack-source.yml` on remote `main`. Return the owner-
produced run URL. Do not switch, pull, edit, repair, rerun, cancel, or reorder
anything locally or on GitHub.

**Complete when:** exactly one new run URL or one pre-existing identical run URL
is recorded; dispatch acceptance is reported only as **solicitud aceptada**.

## 4. Monitor and conclude

Follow [RESULTS.md](RESULTS.md). Monitor by default, validate mandatory artifacts
against the schemas on remote `main`, and report their facts without deriving
new synchronization decisions. Stop early only for an explicit URL-only request
or interruption, reporting **pendiente** rather than success.

**Complete when:** the result is a verified **sin cambios**, a PR that remains
**decision-ready** for its exact plan/candidate/base/head identity, an intentional
human-inspection wait, a linked supersession, or **bloqueada** with owner-produced
evidence and an exact next action.

