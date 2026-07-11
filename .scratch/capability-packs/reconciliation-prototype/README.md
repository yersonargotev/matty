# Reconciliation state-machine prototype

> **THROWAWAY PROTOTYPE** — this is a discussion aid, not capability-pack implementation.

## Question under test

Can Matty keep `inspect`, `plan`, and `apply` separate while proving that it applies exactly the human-approved plan and refusing to overwrite changes made after preview?

## Candidate transaction model

```text
inspect ──> observed snapshot
                │
                v
plan ────> proposed plan ── human approval ──> approved plan
                                                  │
                              fresh inspection ───┤
                                                  v
                                      applying ──> applied
                                          │           │
                                          │           v
                                          └────> verify outcome
                                          │
                                          └────> interrupted / recovery required
```

This first slice deliberately tests only the preview/apply concurrency boundary. The later slices will add partial failure, rollback/recovery, external actions, readiness, and shared ownership after the earlier choices are agreed.

The candidate treats a plan as an immutable envelope containing:

- the requested operation;
- the desired-state revision;
- fingerprints of every inspected input the actions rely on;
- the exact ordered actions;
- a digest over all of the above.

Approval records that digest. `apply` accepts the approved envelope, re-inspects its preconditions, and either executes those exact actions or rejects the whole attempt as stale. It never silently rebuilds the plan inside `apply`.

## Run

```sh
go run ./.scratch/capability-packs/reconciliation-prototype happy
go run ./.scratch/capability-packs/reconciliation-prototype stale
go run ./.scratch/capability-packs/reconciliation-prototype partial
```

Both scenarios keep all state in memory and write only to stdout.

## Decision 1: concurrent changes after preview

**Confirmed:** reject the stale plan, execute zero actions, and require a new preview and approval. `apply` never replans silently.

The prototype currently demonstrates **hard stale-plan rejection**:

1. Preview observes host fingerprint `host-v1` and produces plan digest `P`.
2. The human approves `P`.
3. If the host is still `host-v1`, Matty applies the actions embedded in `P`.
4. If another actor changes it to `host-v2`, Matty applies nothing and requires a new preview and approval.

The competing behavior would let `apply` automatically replan after detecting drift, but then the newly computed actions would not be the plan the human approved.

## Decision 2: partial failure and atomicity

**Confirmed:** atomicity is limited to safely staged/restorable local actions. External failure preserves truthful partial state and requires a newly inspected and approved recovery plan; Matty never performs speculative rollback.

The next candidate divides approved actions into explicit **recovery classes** rather than promising impossible global atomicity:

```text
reversible local actions
    stage all outputs
    validate staged result
    commit with backups + durable journal
    on failure: restore backups

external/non-reversible actions
    execute only after local commit
    record started/completed outcome in the journal
    on failure: stop; retain truthful partial state; require recovery/retry
```

This means one reconciliation can be atomic only across the local filesystem actions an adapter can actually stage and restore. It cannot truthfully be atomic across package-manager commands, tool-owned setup, authentication, host trust, or other external effects.

The candidate plan exposes that distinction before approval. An external action with uncertain or non-reversible effects is a barrier: later actions do not start after it fails. Recovery first re-inspects reality, preserves completed owned work, and proposes only the remaining or compensating actions as a new plan requiring approval.

## Decision 3: reconciliation state versus pack readiness

**Confirmed:** reconciliation progress and pack readiness are independent. Readiness is derived from fresh inspection; pending human action records blocking reasons rather than acting as another readiness level.

The candidate keeps two state machines separate:

```text
reconciliation attempt
  proposed -> approved -> applying -> applied -> verified
                              |           |
                              +-> interrupted / recovery-required
                                          |
                                          +-> superseded by a new approved plan

pack readiness (derived from fresh inspection)
  configured?  all required Matty-owned projections match desired state
  authorized?  configured + required human trust/authentication is observed
  usable?      authorized + the host reports/verification proves it can load the capability
  pending human action = reason(s) blocking authorized or usable, not a fourth success level
```

An attempt can therefore end `recovery-required` while a pack remains partly projected and **not configured**. Conversely, a plan can be fully applied and verified as **configured** while the pack is still pending login, trust, restart/reload, or another human action. Neither `apply` success nor persisted intent alone may claim `authorized` or `usable`.

## Decision 4: ownership and shared resources

**Confirmed:** ownership is verified per projection; contributor sets protect shared resources, and drift or ambiguity blocks destructive mutation rather than being silently adopted.

The candidate ledger records ownership per projected resource, not merely per pack:

```text
projection identity: surface + resource kind + logical key + host target
desired contributors: set(pack id + pack version + resource digest)
last verified projection: content/semantic fingerprint
recovery metadata: kept in the attempt journal, not presented as verified ownership
```

Lifecycle operations all compile to a new complete desired state and diff it against fresh inspection plus this ledger:

- **activate:** add the pack as a contributor; create a projection only if absent from the combined desired state;
- **update:** replace that pack's contribution; reject incompatible shared contributions rather than select a winner;
- **deactivate:** remove that contributor; retain the projection while any contributor remains;
- **reconcile:** repair verified Matty-owned projections toward the combined desired state without changing activation intent.

Deletion is planned only when the last contributor disappears **and** the observed projection still matches the last verified Matty fingerprint. Drifted or ambiguous targets are preserved and become pending human action; Matty does not overwrite, delete, or silently adopt unmanaged content. Ownership becomes verified only after outcome inspection; an interrupted attempt records completed facts in its recovery journal without pretending the whole desired ownership transition committed.

## Decision 5: durable activation intent and concurrent Matty commands

**Confirmed:** after both stale checks, apply atomically persists the approved target intent and applying journal before side effects. Recovery continues toward that intent, and obsolete Matty revisions require a new preview and approval.

The candidate distinguishes durable **activation intent** from readiness and guards it with a monotonically increasing revision:

```text
preview:  read intent revision R + inspect hosts -> plan bound to R and observations
approval: approve exact plan digest
apply:    acquire state lock
          require current intent revision == R
          re-inspect and require host preconditions still match
          atomically persist new intent revision R+1 + applying journal
          release lock only after the durable transition is safe
          execute approved actions and verify
```

The new intent is persisted only after both stale checks pass, but before the first side effect. Therefore a crash or partial failure does not forget what the user approved:

- failed **activate** remains active in intent but not configured, with recovery required;
- failed **update** retains the approved target version in intent and recovery converges toward it;
- failed **deactivate** remains absent from intent while recovery completes safe cleanup;
- **reconcile** changes no activation intent and repairs toward the current revision.

A second Matty process cannot apply a plan built from revision `R` after another operation has advanced it. It must preview and obtain approval again. The state lock protects only Matty's short durable state transition; host drift is handled by fingerprints because Matty cannot lock external hosts.

## Decision 6: non-skippable human checkpoints

**Confirmed:** approvals are typed receipts bound to exact plan phases. No consent kind implies another, stale plans invalidate receipts, and unattended flags cannot bypass required human checkpoints.

The candidate plan is an ordered set of phases. Each phase declares its effect class and required consent:

```text
plan digest P
  phase A: reversible local projections       -> activation/reconciliation approval
  phase B: executable or external effects     -> executable-action approval
  phase C: destructive owned cleanup, if any  -> destructive-cleanup approval
```

An approval receipt names the plan digest, phase digest, and consent kind. `apply` may execute a phase only with its matching receipt; approval for phase A never implies approval for B or C. A stale observation or changed plan invalidates every outstanding receipt. The CLI may gather adjacent approvals during one interaction, but it cannot collapse them into a generic consent or let unattended flags bypass a required human checkpoint.

Authentication, host trust, and runtime authorization are not apply receipts: Matty reports them as pending human actions and later inspection observes whether they have been completed in the owning host.

## Decision 7: inspect, plan, and apply seams

**Confirmed:** the facade exposes preview/apply/status while preserving strict internal inspect, pure plan, and exact apply stages. The CLI and adapters do not absorb portable reconciliation policy.

The candidate keeps three semantic stages strict without making `internal/cli` orchestrate them:

```text
internal/cli
  facade.Preview(request) -> immutable plan + status
  facade.Apply(approved plan, receipts) -> attempt result + status
  facade.Status() -> freshly inspected status

internal/capabilitypack implementation
  inspect: adapters return side-effect-free observed facts and fingerprints
  plan: pure computation from request + manifest catalog + intent + ownership + observations
  apply: validate plan/receipts, re-inspect preconditions, persist journal/intent, execute exact actions, verify
```

`Preview` is the public use case that composes inspection and pure planning; it never writes host or Matty state. `Apply` accepts the immutable plan value returned by preview rather than a request from which it could silently recompute actions. It may re-inspect only to validate preconditions and verify outcomes. If validation fails, it returns a stale result; it does not invoke planning.

Adapters expose host-specific inspection and execution behind the seam owned by `internal/capabilitypack`. They do not decide ownership, global action ordering, readiness, or recovery. The canonical plan encoding and digest are owned by `internal/capabilitypack`, so the CLI cannot accidentally approve one representation and execute another.

## Consolidated transition table

| From | Event | Guard | To | Durable effect |
| --- | --- | --- | --- | --- |
| inspected | preview | manifests, requirements, conflicts, ownership and observations valid | proposed | none |
| proposed | approve phase(s) | human receipts bind exact plan/phase digests | approved | receipts may be held by the caller |
| approved | begin apply | intent revision and all relied-on observations unchanged | applying | target intent + journal committed atomically |
| approved | begin apply | revision or observation changed | stale | no intent change; zero actions |
| applying | local failure | safe backups available | recovery-required | rollback local transaction; record outcome |
| applying | external failure/crash | no speculative compensation | recovery-required | preserve completed facts and approved intent |
| applying | all actions complete | exact approved phases executed | applied | journal records outcomes |
| applied | inspect outcome | projections match desired state | verified | commit verified ownership/fingerprints |
| applied | inspect outcome | mismatch or ambiguous outcome | recovery-required | preserve journal; do not claim ownership/readiness |
| recovery-required | preview recovery | fresh inspection can compute safe convergence | proposed | none; old attempt retained for audit/recovery context |

`stale`, `verified`, and `recovery-required` terminate one attempt. Recovery is always a new proposed plan and approval, never mutation or replay of the old plan.

## Scenario matrix

| Scenario | Expected result |
| --- | --- |
| Activate `matty` on a clean surface | Intent adds pack; owned projections verify; configured may become true independently of authorization/usability. |
| Activate `engram` without its global tool | Preview reports unsatisfied requirement/acquisition action and required executable consent; it never treats the tool as a surface resource. |
| Update a pack | Intent targets the approved version; unchanged shared projections retain contributors; incompatible shared contribution rejects planning. |
| Deactivate one of two contributors | Remove only that contributor; retain the shared projection unchanged. |
| Deactivate the last contributor | Delete only with destructive approval and matching verified fingerprint; otherwise preserve and request human action. |
| Reconcile drifted owned content | Preview a repair when safe; ambiguity or unmanaged replacement is preserved rather than adopted. |
| Host changes after preview | Apply returns stale and executes zero actions. |
| Another Matty process changes intent | Revision guard returns stale and executes zero actions. |
| Local write fails | Roll back the staged/restorable local transaction and enter recovery-required. |
| External setup fails after local commit | Preserve committed facts, stop at the barrier, and require a newly approved recovery plan. |
| Apply succeeds but login/trust/reload remains | Pack may be configured, while authorized/usable remain false with pending-human-action reasons. |
