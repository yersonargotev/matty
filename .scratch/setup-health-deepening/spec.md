Status: ready-for-agent

# Matty setup health deepening specification

## Problem Statement

Matty users rely on `doctor` to explain whether their base installation is
healthy and how to recover when lifecycle state, managed skill links, Engram,
Codex, or OpenCode have drifted. The command works today, but the CLI adapter
owns the cross-domain health semantics as well as fact collection, summary
calculation, rendering, and exit behavior. This makes the command package the
effective domain test surface, exposes private check builders as the useful
seam, and forces future consumers of setup health to depend on CLI-owned types.

The architecture should change without changing the observable `doctor`
contract. Users must receive the same checks in the same order, with the same
names, severities, details, remediation, summary, human output, JSON schema,
and exit behavior.

## Solution

Introduce one deep setup health module as the sole owner of base-installation
health orchestration. It obtains fresh read-only observations from the existing
domain owners, converts those facts into stable diagnoses, calculates the
summary, and returns a self-contained report snapshot through one Diagnose
operation.

The report contains structured context, ordered checks, complete diagnostic
text, and a summary. Observation failures become WARN or FAIL checks rather
than aborting diagnosis, so every completed call returns the most complete
report possible. The CLI remains a thin adapter that supplies resolved
configuration, renders the report as human or JSON output, and translates the
summary into command exit behavior.

This is a behavior-preserving architectural refactor. It covers only the base
Matty setup; capability-pack convergence and readiness remain owned by the
capability-pack status model.

## User Stories

1. As a Matty user, I want `doctor` to preserve its current behavior, so that an architectural refactor does not disrupt diagnosis of my workstation.
2. As a Matty user, I want lifecycle state health reported, so that I know whether the base installation is present, missing, corrupt, or requires recovery.
3. As a Matty user, I want managed skill-link health reported, so that missing, changed, unmanaged, or healthy links are explained consistently.
4. As a Matty user, I want Engram executable health reported, so that PATH resolution and canonical Homebrew ownership remain visible.
5. As a Matty user, I want Engram version inspection reported, so that unreadable or mismatched executable versions remain actionable.
6. As a Matty user, I want Engram PATH shadowing reported, so that an unexpected executable taking precedence is easy to identify.
7. As a Matty user, I want local Engram compatibility-link health reported, so that stale or incorrect links remain visible.
8. As a Matty user, I want active Engram runtime processes diagnosed, so that a running process from the wrong executable can be remediated.
9. As a Matty user, I want delegated Engram setup expectations diagnosed from lifecycle state, so that missing Codex or OpenCode setup can be repaired.
10. As a Codex user, I want Matty prompt markers and external managed-block conflicts diagnosed, so that global instructions remain understandable.
11. As an OpenCode user, I want config, prompt, instruction reference, malformed JSONC, and overlay conflicts diagnosed, so that setup drift is actionable.
12. As a user with several simultaneous setup problems, I want one complete report, so that one failed observation does not hide unrelated diagnoses.
13. As a user with an unreadable file, I want the failure represented as a diagnosis, so that permissions problems are explained rather than causing an early abort.
14. As a user when process inspection fails, I want the rest of the report preserved, so that filesystem and configuration health remain visible.
15. As a cautious user, I want diagnosis to be strictly read-only, so that running `doctor` never repairs, installs, removes, writes, or terminates anything.
16. As a user, I want active read-only inspection to remain available, so that PATH, executable versions, and active processes are diagnosed from fresh facts.
17. As an automation author, I want JSON schema version 1 preserved, so that existing parsers continue to work.
18. As an automation author, I want check names, order, severities, details, summary fields, and exit behavior preserved, so that existing assertions and gates do not regress.
19. As a terminal user, I want human output and its context header preserved, so that current troubleshooting instructions remain familiar.
20. As a maintainer, I want setup health behind one deep interface, so that callers and tests do not need to coordinate individual domain checks.
21. As a maintainer, I want every report to be a self-contained point-in-time snapshot, so that renderers never reobserve or reinterpret workstation state.
22. As a maintainer, I want setup health to own diagnostic names, severities, details, remediation, ordering, and summary policy, so that those semantics have one owner.
23. As a maintainer, I want domain-specific probes and artifact parsing to remain with their existing owner modules, so that setup health orchestrates without absorbing unrelated implementations.
24. As a maintainer, I want only nondeterministic external facts substituted in tests, so that the real filesystem behavior is exercised safely through sandboxed paths.
25. As a maintainer, I want executable lookup isolated behind a least-authority seam, so that diagnosis cannot receive arbitrary command-execution capability.
26. As a maintainer, I want report construction to return a report rather than an ambiguous report-plus-error pair, so that partial-observation policy is consistent for every caller.
27. As a maintainer, I want human and JSON rendering to remain adapters, so that presentation and output failures stay separate from health semantics.
28. As a maintainer, I want CLI tests to cover only rendering, exit behavior, and command wiring, so that the CLI no longer acts as the domain test surface.
29. As a maintainer, I want the old CLI-owned report types and check builders deleted after migration, so that the codebase has one health owner rather than compatibility wrappers.
30. As a capability-pack maintainer, I want pack status and readiness to remain independent, so that base setup health does not conflate installation health with activation convergence.
31. As a future caller, I want a structured setup health report independent of Cobra, so that another adapter can consume the same diagnoses without importing CLI types.
32. As a contributor, I want the architecture recorded independently from core lifecycle, so that ownership and seam decisions remain durable and traceable.

## Implementation Decisions

- Create one independent setup health module as the sole owner of base-installation health orchestration, diagnoses, ordered checks, remediation text, and summary calculation.
- Keep setup health separate from both the core lifecycle module and the CLI adapter. It consumes the core lifecycle read-only observation seam without owning lifecycle persistence or recovery behavior.
- Limit the module to base setup: lifecycle state, managed skill links, Engram, Codex, and OpenCode. Capability-pack status, projections, blockers, and readiness remain outside it.
- Present one deep interface with a single Diagnose operation. External dependencies are supplied when the module is constructed; per-domain diagnosis and summary helpers are implementation details.
- Make Diagnose return only a structured Report. Observation failures are diagnostic facts represented by checks, not a second error channel.
- Include structured context metadata, ordered checks, and summary in the report so it is a self-contained point-in-time snapshot. Renderers must not reobserve or reinterpret workstation state.
- Give each check a stable name, severity, and complete actionable detail. The setup health module owns remediation language, including suggested Matty commands.
- Preserve the existing diagnostic contract exactly: JSON schema version, report kind, check names and order, severities, detail text, summary fields and status rules, human output, and exit behavior.
- Keep warnings non-fatal and derive unhealthy command behavior from report failures, as today.
- Permit active but strictly read-only observations: filesystem and symlink reads, PATH lookup, process inspection, and bounded non-mutating executable version queries.
- Prohibit report construction from writing files, creating directories, repairing state, installing or removing artifacts, changing configuration, executing lifecycle actions, or terminating processes.
- Continue diagnosis after individual observation failures and produce the most complete report possible. Preserve the current WARN-versus-FAIL classification for each failure mode.
- Define minimal setup-health configuration containing only the paths and environment values required by base health diagnosis. The CLI adapts its resolved workstation paths into this configuration.
- Do not move or redesign the broader workstation path model as part of this work.
- Reuse stable observations and diagnoses from the lifecycle, Engram, prompt, and OpenCode owner modules. Use the real filesystem through sandboxed paths rather than introducing one interface per domain.
- Own a least-authority executable-lookup seam that exposes only PATH resolution. Reuse the existing Engram fact provider for bounded version and process observations.
- Do not depend on the CLI runner or grant setup health arbitrary command execution.
- Keep human rendering, JSON encoding, output writers, and command exit-error adaptation in the CLI.
- Remove CLI-owned report and summary types, report builders, check classifiers, and compatibility aliases after callers and tests migrate to the setup health report.
- Replace the old ownership rather than layering a forwarding module or preserving duplicate diagnosis policy.
- Record the setup health module as a new accepted architecture decision linked to, but independent from, the core lifecycle decision.

## Testing Decisions

- Make Diagnose-to-Report the primary behavioral test seam. Tests and production callers cross the same interface.
- Test complete reports rather than exporting or directly testing individual state, skill, Engram, Codex, OpenCode, or summary helpers.
- Build a semantic scenario matrix covering healthy setup, missing state, corrupt state, recovery-required state, missing or drifted links, unmanaged expected links, and zero recorded skills.
- Cover Engram absent from PATH, canonical and noncanonical resolution, version inspection failure, version mismatch, PATH shadowing, local compatibility links, no active runtime, matching runtime, mismatched runtime, and process-inspection failure.
- Cover Codex prompt absence, unreadable prompt, complete and incomplete Matty markers, and detected external managed-block conflicts.
- Cover OpenCode config absence, missing Matty instruction, missing prompt, valid setup, malformed or unreadable config, and detected overlay conflicts.
- Verify best-effort behavior by combining failures from one domain with healthy or unhealthy observations from the others and asserting that the report remains complete.
- Verify exact check ordering, severity, detail, remediation, summary counts, and summary status through the report seam.
- Verify that warnings alone remain healthy for command exit purposes and that one or more failures produce the existing unhealthy error behavior in the CLI adapter.
- Use sandboxed HOME, XDG configuration, state, prompt, OpenCode, skill-link, local-bin, and Homebrew-prefix paths. Never inspect or write the operator's real configuration.
- Use the real sandboxed filesystem and existing domain observers for local-substitutable behavior.
- Substitute only executable lookup, executable-version facts, and process-listing facts. Never invoke the operator's real Engram or process state in tests.
- Retain focused CLI adapter tests for human rendering, JSON schema version 1 encoding, output failures, unhealthy exit mapping, and Cobra wiring.
- Retain a small sandboxed command-level baseline that proves the CLI maps resolved paths into setup health and renders the resulting report.
- Delete tests that exist only to protect the old private check decomposition or obsolete CLI-owned report builders.
- Run focused module and CLI tests while iterating, then the complete repository test suite before reporting success.

## Out of Scope

- Changing diagnostic check names, order, severities, details, remediation language, summary policy, human output, JSON schema, or exit behavior.
- Adding new health checks, report fields, commands, flags, repair actions, or automatic remediation.
- Combining setup health with capability-pack status, convergence, blockers, projections, or readiness.
- Changing core lifecycle planning, application, persistence, ownership, recovery, or observation semantics.
- Redesigning workstation path resolution or the shared path structure.
- Redesigning Engram, Codex prompt, or OpenCode owner probes beyond the minimum required to reuse their existing observations without behavioral change.
- Introducing broad filesystem abstractions or one mock interface per diagnosed domain.
- Giving diagnosis arbitrary command execution or performing mutating commands.
- Preserving deprecated CLI report builders or compatibility wrappers after migration.
- Opportunistic cleanup unrelated to setup health ownership.

## Further Notes

- The accepted core lifecycle architecture already provides the read-only state and managed-skill observation seams required by setup health and explicitly keeps overall health classification separate.
- The implementation should observe shared lifecycle state once per diagnosis and reuse that snapshot across state, skills, delegated Engram setup, and report context so one report is internally consistent.
- Check evaluation should remain deterministic and preserve current ordering even if implementation details later change.
- The existing bounded Engram version observation remains compatible with the read-only active-probe decision.
- The deletion test is decisive: removing the setup health module should force orchestration, diagnosis policy, remediation, ordering, and summary behavior back into adapters or individual probes.
