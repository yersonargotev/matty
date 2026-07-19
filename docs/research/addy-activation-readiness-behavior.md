# Addy activation and readiness behavior

## Decision question

What should preview, consent, pending-human-action, status, update, and failure
output look like for Addy's trust-sensitive workflows and declared
Codex/OpenCode projection differences?

This decision refines the existing Pack lifecycle interaction. It does not
implement, synchronize, activate, publish, or release Addy.

## Fixed contract

Addy `1.0.0` is atomic per CLI surface. A coherent activation requires all 24
skills and their dependency-closed assets, four agents, and eight logical
workflows. OpenCode preserves `/name` workflows natively; Codex exposes the
same logical workflows through the declared `$name` workflow-skill
degradation. The degradation is complete support when every required logical
capability remains coherently invocable.

Addy's four upstream hooks remain excluded from activation. Browser/network
tools, package managers, subagents, and privileged commit/deploy effects are
optional invocation-time modes. Neither group is a required projection that
Packy may silently omit.

The existing lifecycle invariants remain authoritative:

- Preview is fresh, deterministic, and side-effect free.
- Apply uses only the exact approved plan and typed effect phase.
- A changed precondition makes the plan stale and executes zero actions.
- A verified Apply may succeed before the pack becomes usable.
- Status freshly observes readiness without mutation.
- Recovery repeats the originating lifecycle verb to obtain a new plan and new
  approvals; it never replays the old plan.

## Activation preview

The preview leads with the complete per-surface observable contract before the
filesystem action list. It reports:

1. Pack version, surface, plan identity, intent revision, and disposition.
2. Required logical counts: 24 skills, four agents, eight workflows, and seven
   dependency-closed shared references.
3. The native or degraded binding for each resource class, including Codex's
   `/name` to `$name` workflow mapping.
4. Required omissions, which must be empty for an applicable plan.
5. Contract exclusions, including all four upstream hooks.
6. Invocation-time prompt authority: workflows may ask the host agent to edit
   files, run processes, use browser/network tools, spawn subagents, invoke
   package managers, or propose privileged commit/deploy effects.
7. The exact reversible-local projection actions and mandatory license notice.

The authority disclosure states that activation grants none of those later
host or tool permissions. Excluded hooks are never described as pending human
action.

Illustrative Codex dry-run:

```text
$ packy pack activate addy --surface codex --dry-run
Activation dry-run plan plan-a10
Pack: addy 1.0.0
Surface: codex
Plan disposition: applicable

Observable contract: complete on Codex
  24 skills       native
   4 agents       native
   8 workflows    declared degradation: /name -> $name
   7 shared refs  dependency-closed
  Required omissions: none
  Excluded by contract: 4 upstream hooks, maintainer/CI/eval material

Invocation-time authority:
  Workflows may request file changes, processes, network/browser tools,
  subagents, package managers, and privileged commit/deploy actions.
  Host/tool policy still governs every invocation; activation grants none.

Phase: reversible-local
  + complete Codex projection and dependency assets
  + MIT notice
Dry-run: no approvals requested; no state, files, or commands changed.
```

OpenCode uses the same structure and reports eight native `/name` workflows
instead of the Codex degradation.

## Consent

Addy activation uses one `reversible-local` approval after the mandatory prompt
authority disclosure. It does not add a `prompt-authority` consent kind.
Approval authorizes only the exact local projection actions in the sealed plan;
it cannot pre-authorize future host tool calls or privileged effects.

Because hooks are excluded, Addy activation has no executable/external hook
phase. A future contract that adds an executable effect would need its own
compatibility decision and the existing effect-appropriate consent semantics.

## Apply outcome and readiness categories

A verified Apply reports `configured`, `authorized`, and `usable` separately.
It then classifies remaining facts into three non-overlapping categories:

| Category | Meaning | Activation effect |
|---|---|---|
| Pending human action | A finite host-owned step required to advance readiness, such as trust or reload | Does not turn a verified Apply into failure; may keep `authorized` or `usable` false |
| Invocation-time mode | An optional workflow mode that is unavailable or host-policy dependent | Does not block activation; the affected invocation fails only when it has no coherent fallback |
| Contract exclusion | Material deliberately absent from the observable contract, including Addy's hooks | Requires no action and never advances readiness |

After a verified Apply, the lifecycle command exits zero even when a real
pending human action remains. `packy pack status addy --surface <surface>
--require usable` is the separate, inspection-only readiness gate for
automation.

## Targeted status

Targeted status shows the complete decision-relevant view:

- intent revision and latest reconciliation attempt;
- `configured`, `authorized`, and `usable` as explicit `yes`, `no`, or
  `unknown` values;
- projection integrity and logical resource counts;
- native bindings and declared degradation;
- contract exclusions;
- pending human actions; and
- invocation-time modes and their availability or host-policy dependence.

Human and JSON status must preserve the same three-state readiness meaning.
Unobserved authorization or usability is `unknown`, never rendered as `no`.

A surface is `complete` when every required logical capability has a coherent
native or declared degraded binding. A missing required capability, unresolved
dependency, or unresolved collision is `blocked` or `incomplete`; it is never
called a partial usable projection. Codex and OpenCode remain independent, so
one blocked surface does not falsify the other surface's status.

Illustrative targeted status:

```text
$ packy pack status addy --surface codex
addy 1.0.0 on codex
Intent: active at revision 12
Latest attempt: verified (plan-a10)
Readiness: configured=yes, authorized=yes, usable=no
Projection: complete (24 skills, 4 agents, 8 workflows, 7 shared refs)
Degradation: 8 workflows use $name instead of /name
Excluded hooks: 4
Pending human actions:
  - Reload Codex.
Invocation-time modes: browser/network unavailable; other modes host-policy dependent
```

## Update preview

An Addy update preview leads with the observable-contract diff rather than an
upstream version or file-count diff. It reports:

- prior and target Pack versions and the Pack-owned SemVer classification;
- added, changed, removed, and retained logical capabilities;
- the impact on Codex and OpenCode bindings and coherence;
- migrations and mandatory user actions;
- changed invocation-time prompt authority; and
- unchanged exclusions when they remain relevant to trust.

Upstream identity and resource counts remain supporting facts. A changed
prompt-authority boundary is a mandatory disclosure inside the exact
`reversible-local` approval, not a new consent phase. If a mandatory migration,
authorization, or user action makes the prior contract incompatible, the
classification is major.

## Failure output

Failure output is structured by lifecycle stage.

### Blocked preview

A blocker names the Addy logical identity, normalized surface binding,
competing owner or reservation, reason, and explicit resolution paths. A
collision does not produce an applicable partial plan. An alias such as
`addy-<name>` appears only as an offered surface-local choice; Packy never
selects or applies it silently.

```text
BLOCKED — complete Addy projection is not representable on OpenCode
Required capability: workflow /build
Collision: unmanaged command /build already exists
Owner: user configuration
Resolution: approve a surface-local alias such as /addy-build, then preview again
Result: no plan applied; zero actions; intent unchanged.
```

### Stale plan

Stale output names the changed precondition, states that zero actions ran and
all approvals were invalidated, and instructs the user to repeat the original
lifecycle verb for a fresh preview. It does not silently replan or carry
approval forward.

### Recovery required

Recovery output names the retained attempt, completed effects, failed effect,
not-started effects, and the repeated lifecycle command that will freshly
inspect and plan recovery. It never claims partial Addy readiness merely
because some projections were written. Contract exclusions remain absent and
are not recovery work.

## Specification implications

The final Addy specification must carry enough model data for the CLI to render
this contract without inferring it from host files:

- logical resource counts and dependency closure;
- per-surface native or degraded bindings;
- explicit exclusions and invocation-time modes;
- surface-local alias intent and collision ownership;
- observable-contract update diffs and mandatory actions; and
- three-state readiness in both human and JSON output.

Current Pack schema and adapters cannot yet encode the complete Addy resource
set, current lifecycle commands have no alias-input path, and current human
status collapses unobserved readiness to `no`. These are implementation gaps
for the final specification and validation matrix, not permission to weaken
the behavior above.

## Prototype disposition

The interaction was tested with a no-side-effect terminal transcript prototype.
The confirmed answer is fully absorbed into this document; the throwaway
prototype is deleted rather than retained as production-shaped code.
