Status: ready-for-agent

# Workstation layout ownership specification

## Problem Statement

Matty users expect every command to operate on the same established global
state, skill, Codex, OpenCode, Installed Source, and Engram locations. That
behavior works today, but the CLI resolves one broad workstation structure
containing environment facts and roughly twenty paths owned by unrelated
domains. The structure is passed into classic lifecycle, capability packs,
setup health, host adapters, source selection, bootstrap, and executable
resolution.

This makes the CLI an accidental owner of workstation policy. A host path
change can require edits across unrelated command wiring and tests, diagnosis
can receive paths for artifacts it does not own, and multiple consumers can
reconstruct the same source or executable topology. The broad structure also
encourages new commands to depend on every path rather than the minimum facts
or capabilities they require.

The architecture must hide derived workstation layout behind the modules that
own each artifact without changing any observable command behavior. Existing
paths, source precedence, diagnostics, output, state schemas, filesystem
effects, sandbox behavior, and help/version independence must remain stable.

## Solution

Introduce a lazy, immutable Workstation snapshot containing only normalized
ambient facts and the shared Matty Home root. The process edge captures HOME,
XDG configuration home, PATH, Homebrew prefix, and current working directory
once per command invocation. Commands that do not need workstation access do
not create the snapshot.

Each owning module derives and exposes a narrow value or read-only observation
for its own layout:

- workstation owns normalization and Matty Home;
- bootstrap owns the Installed Source descriptor;
- skillbundle owns Skill Source selection and the global skill installation
  layout;
- core lifecycle and capability pack own their separate state files beneath
  Matty Home;
- Codex and OpenCode own their canonical host layouts;
- engrambin owns Engram executable topology and resolution;
- setup health consumes owner observations rather than reconstructing layout;
- the CLI remains responsible only for flags, composition, rendering, and
  command error adaptation.

Explicit Home and Installed Source overrides are translated by the CLI into
owner inputs. In particular, an explicit Home continues to isolate
configuration-home derivation from ambient XDG configuration.

Deliver the change as an expand–migrate–contract refactor. Temporary wiring may
exist while callers move, but the completed architecture deletes the shared
path structure, its resolver, mapping helpers, duplicated derivation, and all
permanent compatibility wrappers. Owner contracts and a focused structural
test prevent the old ownership model from returning.

## User Stories

1. As a Matty user, I want commands to keep using the same global paths, so that an architectural refactor does not move or duplicate my state.
2. As a Matty user, I want install behavior to remain unchanged, so that my managed skills and host configuration are reconciled as before.
3. As a Matty user, I want update behavior to remain unchanged, so that source validation and managed artifact refresh remain predictable.
4. As a Matty user, I want uninstall behavior to remain unchanged, so that owned artifacts are removed safely without affecting unmanaged content.
5. As a capability-pack user, I want pack state to remain separate from classic Matty state, so that the two lifecycle models do not acquire shared ownership.
6. As a capability-pack user, I want pack commands to preserve plans, status, effects, and output, so that layout refactoring does not alter lifecycle semantics.
7. As a Codex user, I want all Matty features to agree on Codex's canonical files, so that classic and pack lifecycle cannot diverge.
8. As an OpenCode user, I want all Matty features to agree on OpenCode's canonical files, so that classic and pack lifecycle cannot diverge.
9. As a user of global skills, I want classic and pack lifecycle to target the same installation surface, so that skill projections remain composable.
10. As a package-installed user, I want the Installed Source to remain at its established default, so that existing checkouts continue working.
11. As a package-installed user, I want bootstrap and update validation to observe the same Installed Source descriptor, so that initialization and validation cannot disagree.
12. As a developer using an explicit skill-source override, I want it to retain highest precedence, so that local development remains deterministic.
13. As a developer running inside the Matty repository, I want repository discovery to preserve its current precedence, so that local bundle development still works.
14. As a user without an override or repository checkout, I want Skill Source selection to fall back to Installed Source, so that packaged operation remains reliable.
15. As a user, I want every subsystem in one command to observe the same Skill Source, so that diagnosis and reconciliation cannot disagree.
16. As a user, I want missing-source guidance to remain unchanged, so that existing remediation instructions remain useful.
17. As an Engram user, I want executable lookup and fallback behavior to remain unchanged, so that Matty continues finding the same binary.
18. As an Engram user, I want lifecycle, packs, and setup health to share one executable resolution policy, so that their results remain consistent.
19. As a user with no HOME value, I want workstation-dependent commands to retain the established error, so that failure remains clear and compatible.
20. As a user with a relative XDG configuration home, I want Matty to retain its existing fallback, so that path normalization does not change silently.
21. As a user invoking help, I want it to work without a valid HOME, so that documentation remains available in restricted environments.
22. As a user invoking version output, I want it to work without a valid HOME, so that package checks remain independent from workstation setup.
23. As a user invoking one command, I want every participating module to observe one immutable workstation snapshot, so that environmental changes cannot create inconsistent paths mid-operation.
24. As a user passing an explicit Home to initialization, I want ambient XDG configuration ignored as before, so that the alternate Home remains isolated.
25. As a user passing an explicit source root, I want bootstrap to normalize and use it exactly as before, so that custom Installed Sources remain supported.
26. As a cautious user, I want tests and manual verification to use sandboxed Home and configuration paths, so that development never mutates my real setup.
27. As a maintainer, I want workstation normalization to have one owner, so that HOME and configuration-home rules cannot diverge.
28. As a maintainer, I want the Workstation snapshot to exclude domain artifact paths, so that it does not become a renamed global path catalog.
29. As a maintainer, I want Matty Home shared as a root without sharing ownership of files beneath it, so that classic and pack state remain independent.
30. As a maintainer, I want bootstrap to own Installed Source layout, so that checkout initialization and validation use one descriptor.
31. As a maintainer, I want skillbundle to own Skill Source selection, so that precedence is applied exactly once.
32. As a maintainer, I want skillbundle to own the global skill installation layout, so that lifecycle modules only decide which skills to reconcile.
33. As a maintainer, I want Codex to own its host paths, so that consumers do not encode Codex conventions.
34. As a maintainer, I want OpenCode to own its host paths, so that consumers do not encode OpenCode conventions.
35. As a maintainer, I want engrambin to own executable candidates and precedence, so that consumers request executable facts instead of paths.
36. As a setup-health maintainer, I want diagnosis to consume owner observations, so that its configuration does not recreate the broad path structure.
37. As a CLI maintainer, I want command code limited to composition and adaptation, so that adding a command does not expose every workstation path.
38. As a CLI maintainer, I want explicit flags translated into owner inputs, so that flag handling does not acquire layout policy.
39. As a contributor, I want owner layout contracts, so that path semantics are verified where they are defined.
40. As a contributor, I want CLI end-to-end compatibility tests, so that the refactor proves existing user-visible behavior.
41. As a contributor, I want a focused structural architecture test, so that the shared path model and ambient reads cannot be reintroduced silently.
42. As a contributor, I want structural checks to target concrete ownership violations, so that enforcement does not become a fragile global literal scan.
43. As a test author, I want an aggregate fixture available only to CLI end-to-end tests, so that setup remains readable without creating production layout policy.
44. As a test author, I want the aggregate fixture assembled from owner APIs, so that tests do not become a second source of path truth.
45. As a maintainer, I want the migration to remain green in bounded slices, so that the wide refactor is reviewable and reversible during development.
46. As a maintainer, I want all temporary compatibility wiring deleted at closure, so that the final architecture has one owner for every layout decision.
47. As a future maintainer changing a host path, I want that change localized to the host owner and its contract tests, so that unrelated lifecycle code remains untouched.
48. As a future maintainer changing Matty's state namespace, I want the shared-root decision localized to workstation, so that state owners receive one consistent root.

## Implementation Decisions

- Add an internal workstation module as the exclusive owner of ambient-input normalization, Home validation, configuration-home normalization, Matty Home derivation, and immutable snapshot creation.
- Capture Home, XDG configuration-home input, executable search path, Homebrew prefix, and current working directory once at the process edge.
- Resolve the Workstation snapshot lazily and at most once per command invocation. Help and version operations do not require it.
- Do not include state files, skill paths, host projection paths, Installed Source paths, or executable candidate paths in the Workstation snapshot.
- Support an explicit Home override as part of snapshot construction. Override mode derives configuration home from the supplied Home and ignores ambient XDG configuration.
- Keep the CLI as composition root and flag adapter. It supplies substitutable process inputs and explicit overrides but contains no artifact-layout rules.
- Make bootstrap own an immutable Installed Source descriptor containing the checkout root and bundle location.
- Make bootstrap normalize explicit source-root overrides. Consumers receive its descriptor rather than recomputing Installed Source paths.
- Make skillbundle the single owner of Skill Source selection across explicit override, repository checkout, and Installed Source candidates.
- Resolve Skill Source once per invocation and distribute the same value, including its origin and missing-source guidance, to all consumers.
- Make skillbundle own the global skill installation layout and skill-name projection rules.
- Make core lifecycle derive its classic state file from Matty Home while retaining complete ownership of classic state and lifecycle policy.
- Make capability pack derive its pack state file from Matty Home while retaining complete ownership of pack state and lifecycle policy.
- Keep classic state and pack state separate even though they share Matty Home.
- Make Codex and OpenCode modules derive and expose narrow canonical host-layout values.
- Let lifecycle modules decide when host projections participate; let host modules decide where those projections live.
- Make engrambin own Engram executable candidate locations, precedence, resolution, and read-only observation.
- Supply consumers with Engram resolvers or observations rather than PATH, Homebrew, or local-bin candidate paths.
- Make setup health consume stable read-only observations from lifecycle, skill, host, and Engram owners.
- Give setup health directly only facts intrinsic to diagnosis and report context; do not recreate an aggregate layout configuration.
- Forbid owning modules from rediscovering ambient layout through direct environment, user-home, or current-directory calls.
- Preserve exact current paths, precedence, error text, CLI output, state schemas, filesystem effects, and sandbox behavior.
- Preserve missing-Home failure, relative-XDG fallback, explicit-Home isolation, and explicit source-root behavior.
- Do not introduce state migration, new XDG state placement, or revised user-facing diagnostics.
- Use an expand–migrate–contract sequence. Temporary dual wiring is allowed only while callers migrate and must be deleted before completion.
- Delete the shared path structure, shared resolver, default-source helper, owner-config mapping helpers, duplicated CLI derivation, and obsolete compatibility wrappers at contraction.
- Record durable ownership in the accepted workstation-layout ADR and use the glossary terms Workstation snapshot, Matty Home, Installed Source, and Skill Source consistently.

## Testing Decisions

- Use the Workstation snapshot resolver as the primary normalization contract. Verify normal mode, explicit-Home mode, missing Home, relative and absolute XDG configuration, PATH, Homebrew prefix, current directory, Matty Home, and immutability.
- Test bootstrap's Installed Source descriptor through default and explicit-root initialization behavior, including normalization and validation reuse.
- Test Skill Source precedence and missing-source guidance at the skillbundle seam, not through CLI-owned path helpers.
- Test the global skill installation layout at the skillbundle seam with sandboxed Home facts.
- Test classic and pack state derivation independently from the same sandboxed Matty Home.
- Test Codex and OpenCode canonical layouts in their host packages and reuse those values in lifecycle tests.
- Test Engram candidate selection, resolution, and observation at the engrambin seam.
- Test setup health through its report seam using stable owner observers and the established semantic scenario matrix.
- Keep CLI tests at the highest seam for init, install, update, uninstall, pack commands, doctor, help, and version compatibility.
- Assert external behavior: rendered text and JSON, errors, state schemas, created or preserved files, symlinks, command execution, and exit adaptation. Avoid assertions against private mapping helpers.
- Allow a test-only CLI aggregate fixture assembled exclusively from owner APIs. It may prepare and inspect the sandbox but must derive no paths independently.
- Add focused structural enforcement forbidding the production shared path type/resolver, known CLI artifact derivation, and unauthorized ambient reads.
- Prefer positive owner contracts over broad source-text checks. Structural tests cover ownership boundaries only.
- Delete tests whose sole purpose is protecting the obsolete shared path model once equivalent owner and CLI contracts exist.
- Use sandboxed Home and XDG configuration paths for all filesystem-backed tests and manual checks.
- Run focused package tests during each migration slice and the complete repository test suite before resolving every ticket.

## Out of Scope

- Changing any established global path or moving Matty state to an XDG data directory.
- Migrating, merging, or changing the schema of classic state or capability-pack state.
- Changing install, update, uninstall, init, pack, or doctor command behavior.
- Changing CLI flags, output formats, JSON schemas, errors, exit behavior, or remediation language.
- Changing Skill Source precedence, repository discovery, missing-source guidance, or Installed Source lifecycle semantics.
- Changing capability-pack planning, ownership, readiness, consent, recovery, or projection behavior.
- Changing Codex or OpenCode syntax, projection contents, authorization, or runtime behavior.
- Changing Engram installation, setup delegation, acquisition, execution, or process-observation semantics.
- Adding automatic repair, cleanup, migration, or new setup-health checks.
- Adding support for another host, skill destination, source type, package manager, or executable.
- Creating a generic filesystem abstraction or a broad per-domain mock layer.
- Exposing the Workstation snapshot as another catalog of every derived path.
- Retaining production forwarding wrappers or dual layout ownership after migration.
- Opportunistic cleanup unrelated to workstation layout ownership.

## Further Notes

- ADR 0006 is the durable authority for the ownership model; this specification
  supplies delivery scope and compatibility detail.
- The resolver earns its place by normalizing shared process facts and Matty
  Home. It must remain narrow enough that deleting any owner would remove that
  owner's layout knowledge from the system.
- Setup health owns diagnosis, not the location or parsing policy of the
  artifacts it observes.
- Work the approved tickets one frontier ticket at a time in fresh
  implementation sessions.
