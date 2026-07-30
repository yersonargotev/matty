# Matty Product Requirements Document

## Status

Shared product understanding confirmed for a new greenfield project.

Matty is not Pilly V2. It has no compatibility obligation to Pilly's package,
CLI, storage, release lifecycle, or user configuration. This document is the
canonical product handoff for the greenfield Matty repository.

## Product summary

Matty is a batteries-included Pi package that loads a fixed, opinionated
engineering workflow whenever the user starts `pi`. It combines a curated set
of shared skills, deterministic skill precedence, workflow-aware subagent
delegation, and optional web access.

The initial Shared Skill Catalog is copied verbatim from the complete
`skills/engineering` and `skills/productivity` trees in
[`mattpocock/skills`](https://github.com/mattpocock/skills), pinned to an exact
upstream commit. After import, those files are Matty-owned source and may
diverge. Later upstream improvements enter Matty only through a deliberate
maintainer review and a new Matty release.

The initial Skill Import targets upstream commit
`2ab958093e83e0ec752e6c1c5932da465bf23e0c`.

The initial Subagent Runtime is imported from Pi `0.83.0`'s official subagent
extension example at commit
`845d6ff1f6643aba440341cce877ce1c43ebbc39`, then maintained as Matty-owned
source.

The canonical onboarding and daily experience is:

```sh
pi install npm:@yargote/matty
pi
```

Before the first engineering flow in a repository, the user explicitly runs
`/skill:setup-matt-pocock-skills`. This Repository Preparation configures the
project-owned workflow policy required by those skills; it never runs at Pi
startup.

Pi remains the host, interactive entry point, execution authority, and owner of
project trust. Matty contributes a coherent workflow without becoming a
separate launcher or a second configuration system.

## Problem Statement

Pi can load skills and extensions from several sources, but a user who wants
the same opinionated engineering workflow across projects must currently
assemble, version, prioritize, configure, and diagnose those pieces manually.
This creates drift between projects, ambiguous skill collisions, inconsistent
delegation behavior, and repeated integration setup.

The previous Pilly effort attempted to solve this through an external CLI,
internal releases, activation pointers, compatibility certificates, and an
isolated `run` command. That architecture made reproducibility central while
failing to deliver the basic user experience: open Pi normally and have the
shared workflow available.

The user needs one native Pi package that:

- loads automatically during normal Pi startup;
- supplies a known shared workflow consistently across projects;
- protects shared skill names from accidental replacement;
- allows projects to extend the workflow without copying shared skills;
- makes subagent delegation a dependable workflow capability;
- includes web access without making provider configuration a startup
  dependency;
- remains diagnosable and non-disruptive when Matty or an integration fails.

## Solution

Publish Matty as the native Pi package `@yargote/matty`. The supported v1 path
is a global package installation through Pi, making one workflow available
across repositories. Pi owns package installation, updates, removal, and
startup. One installed npm package version is one complete Matty release.

At Pi startup, Matty first loads a minimal diagnostic bootstrap. The bootstrap
checks the running Pi version and the requirements needed to activate the
shared workflow. If compatible, Matty transactionally activates its fixed
shared skill catalog, subagent support, project-skill composition, and optional
integration adapters. If core activation cannot complete safely, Matty loads
none of those workflow capabilities, leaves Pi usable, and retains only its
diagnostic surface.

The normal active experience includes:

- a fixed shared catalog led by `ask-matt`;
- reserved shared skill names with deterministic precedence;
- additive trusted project skills from `.agents/skills`;
- a shared subagent surface and roles;
- workflow declarations of `required`, `optional`, or `none` delegation;
- independently degradable web access;
- `/matty status` and `/matty doctor`.

Matty does not fetch skills at runtime, own provider credentials, copy shared
skills into repositories, mutate projects during startup, or create a release
lifecycle separate from Pi's package lifecycle.

The MVP Web Capability is supplied by the exact candidate dependency
`pi-web-access@0.15.0`, without a semver range. Phase 0 must prove its
ChatGPT/Codex subscription path with the Certified Pi Version, Certified
Target, and Reference Model Path before Matty claims that no separate API key
is required.

## Goals

1. Make `pi` the only interactive entry point for a Matty-enabled workflow.
2. Deliver the same curated engineering workflow across projects.
3. Make shared skill precedence deterministic and observable.
4. Support additive project customization without shared-skill drift.
5. Provide real subagent delegation with workflow-specific obligations.
6. Include useful integrations without coupling core availability to provider
   configuration.
7. Fail open for Pi and fail closed for Matty's workflow activation.
8. Certify Pi `0.83.0` on macOS Apple Silicon for v1 without claiming an
   unverified version or platform range; expand the tested matrix in later
   releases.
9. Keep installation, updates, and removal native to Pi.
10. Keep startup free of project writes and runtime skill downloads.
11. Require no Matty-owned user configuration or persistent runtime state.
12. Emit no Matty-owned telemetry, crash reports, update probes, or background
    network requests.
13. Publish a prebuilt package that executes no Matty-owned installation
    lifecycle scripts.
14. Make Matty's source and npm package publicly auditable and publish routine
    releases through OIDC trusted publishing with provenance.

## Non-goals

Matty is not:

- a replacement executable or launcher for Pi;
- a compatibility layer for Pilly;
- a global CLI required for onboarding;
- a second package manager;
- a Matty-specific user configuration system;
- a telemetry, analytics, or crash-reporting service;
- a credential or secret store;
- a system for per-user combinations of shared skills;
- an automatic project migration tool;
- a guarantee that every Pi `0.x.x` version is compatible;
- an owner of project trust or agent execution authority.

## User Stories

1. As a developer, I want to install Matty globally with Pi's package command,
   so that I get one workflow across repositories without a separate global
   CLI.
2. As a developer, I want to start my normal session with `pi`, so that I have
   one obvious interactive entry point.
3. As a developer, I want Matty to load automatically, so that the shared
   workflow is present in every session without extra flags.
4. As a developer, I want `ask-matt` available after startup, so that I can
   select the appropriate engineering workflow.
5. As a developer, I want one fixed catalog per Matty version, so that sessions
   and projects share the same workflow assumptions.
6. As a developer, I want Matty's shared skill names to be reserved, so that a
   global or project skill cannot silently alter the shared workflow.
7. As a developer, I want collisions to identify the conflicting skill and
   source, so that I can rename or remove it.
8. As a developer, I want non-colliding global skills to keep working, so that
   Matty does not monopolize Pi's skill system.
9. As a project owner, I want to add project-specific skills under
   `.agents/skills`, so that my repository can extend the shared workflow.
10. As a project owner, I want project skills loaded only after Pi trusts the
    project, so that Matty does not bypass Pi's authority.
11. As a project owner, I want project skills to remain owned by the project,
    so that installing or removing Matty does not mutate them.
12. As a project owner, I want shared skills to remain in the Matty package, so
    that repositories do not accumulate drifting copies.
13. As a developer, I want Matty to expose a shared subagent tool and roles, so
    that workflows can delegate bounded work consistently.
14. As a workflow author, I want to declare delegation as required, optional,
    or unnecessary, so that runtime behavior matches the workflow's actual
    independence needs.
15. As a workflow author, I want required delegation to block only the affected
    workflow when unavailable, so that Matty does not pretend an essential
    property was preserved.
16. As a workflow author, I want optional delegation to fall back inline with
    a recorded reason, so that useful work can continue transparently.
17. As a reviewer, I want required parallel reviewers to execute independently,
    so that sequential work is not reported as parallel validation.
18. As a developer, I want web capability available when its provider is
    configured, so that workflows can research current information.
19. As a provider owner, I want my canonical configuration and credential
    mechanisms to remain authoritative, so that Matty does not duplicate or
    reinterpret sensitive state.
20. As a developer, I want Matty diagnostics to redact sensitive information,
    so that status output is safe to inspect and share.
21. As a developer, I want Pi to open when Matty is incompatible, so that an
    optional workflow package cannot lock me out of the host agent.
22. As a developer, I want an incompatible Matty activation to be all-or-none,
    so that I do not work with a partially loaded shared workflow.
23. As a developer, I want `/matty status` available in degraded mode, so that I
    can see package, Pi, activation, and capability state.
24. As a developer, I want `/matty doctor` to explain actionable remediation,
    so that I can repair incompatibility or missing provider configuration.
25. As a developer, I want only one startup warning for a degraded activation,
    so that diagnostics remain visible without becoming noisy.
26. As a maintainer, I want each Matty release to declare exact certified Pi
    versions, so that support claims are evidence-backed.
27. As a maintainer, I want Matty v1 to certify Pi `0.83.0`, so that the first
    release proves one complete real-world path before expanding its matrix.
28. As a maintainer, I want capability detection and internal adapters, so that
    Pi differences can be contained behind Matty's activation interface.
29. As a maintainer, I want an uncertified Pi version to remain unsupported
    until it passes the complete suite, so that a semantic-version range does
    not become an untested promise.
30. As a developer, I want Matty to avoid network fetches during skill loading,
    so that one installed version has stable contents.
31. As a maintainer, I want upstream skill changes reviewed and published as a
    new Matty version, so that changes are intentional and testable.
32. As a developer, I want to update Matty through Pi, so that package lifecycle
    has one owner.
33. As a developer, I want to remove or disable Matty through Pi, so that I can
    return to a normal Pi environment.
34. As a project owner, I want Pi startup to perform no project writes, so that
    merely opening a repository cannot create unreviewed changes.
35. As a new Matty maintainer, I want a greenfield implementation, so that
    Pilly's lifecycle abstractions do not constrain the new product.
36. As a project owner, I want Matty to identify when Repository Preparation is
    missing, so that I know why a project-dependent workflow cannot start.
37. As a project owner, I want to invoke `setup-matt-pocock-skills` explicitly,
    so that repository policy changes remain visible and consented.
38. As a maintainer or support engineer, I want versioned JSON from Matty's
    diagnostic commands, so that tests and automation do not parse human text.
39. As a developer, I want workflows that require current information to stop
    when web access is unavailable, so that model knowledge is not presented as
    fresh research.
40. As a developer, I want workflows with optional web research to continue
    with an explicit disclosure, so that useful work can proceed transparently.
41. As a developer, I want Matty to work without its own configuration file, so
    that Pi and each integration remain the sole owners of their settings.
42. As a developer, I want Matty to inherit my active Pi model without enforcing
    GPT-5.6 globally, so that a non-reference model does not disable the
    workflow.
43. As an explorer, I want inspection-only shell access, so that I can use Git,
    CodeGraph, and diagnostic commands without general write access.
44. As a designer, I want enough local inspection access to produce an
    evidence-based alternative without modifying the repository.
45. As a worker, I want to edit and validate the trusted working tree without
    receiving authority over Git history, GitHub, global installs, or user
    configuration.
46. As a developer, I want delegated web research concentrated in the
    `researcher` role, so that other subagents do not acquire implicit research
    capabilities.
47. As a developer, I want Matty diagnostics and startup to remain local, so
    that no data leaves my environment without a visible user or workflow
    action.
48. As a developer, I want Matty installation to unpack a reviewed, prebuilt
    artifact without executing Matty-owned lifecycle code.
49. As a user or contributor, I want Matty-owned source clearly licensed under
    MIT while third-party notices retain their original attribution.
50. As a user, I want a public package linked by provenance to its public source
    and build workflow.
51. As a maintainer, I want the unavoidable first-publication credential to be
    temporary, narrowly scoped, and removed immediately after OIDC setup.
52. As a maintainer, I want routine CI to stage rather than publish a release,
    so that the exact tarball requires my review and 2FA approval.

## Functional Requirements

### Native package lifecycle

1. The canonical package name is `@yargote/matty`.
2. The canonical install command is `pi install npm:@yargote/matty`.
3. Pi's package mechanisms own update, disablement, and removal.
4. Matty must not require a globally installed executable for daily use.
5. One npm package version is one complete Matty release.
6. Matty must not implement internal prepared releases, active pointers,
   promotion, rollback, or user-managed compatibility certificates.
7. The Supported Installation for Matty v1 is a global Pi package installation.
8. A project-local installation may function under Pi's package and trust
   behavior, but Matty v1 must not claim, test, or document it as supported.
9. Matty must not block a project-local installation or rewrite project
   configuration to convert it into a global installation.
10. The published package must be an Install-Safe Artifact.
11. `@yargote/matty` must not declare `preinstall`, `install`, `postinstall`,
    `prepare`, or another installation lifecycle script.
12. Build, validation, and artifact generation must finish before publication;
    installation must not compile or generate Matty files.
13. Release CI must inventory lifecycle scripts across the resolved production
    dependency tree.
14. Any new, changed, or unreviewed dependency lifecycle script must block
    publication until its package, exact version, command, effects, and
    justification are explicitly recorded.
15. `@yargote/matty` must be a public scoped package on the public npm registry.
16. The canonical public source repository must be
    `https://github.com/yersonargotev/matty`.
17. `package.json` must declare `publishConfig.access` as `public` and a
    case-exact `repository` URL of
    `https://github.com/yersonargotev/matty`.
18. Routine publication must use a Staged Release from a GitHub-hosted runner
    through npm trusted publishing with OIDC and no long-lived write token.
19. The trusted publisher must be restricted to the canonical release workflow
    and grant `npm stage publish` permission only, never direct `npm publish`.
20. Every functional release, including `0.1.0`, must carry npm provenance that
    links the public package to its public source and build instructions.
21. Release CI must use an npm and Node.js toolchain meeting npm's current
    trusted-publishing requirements and grant only the workflow permissions
    required to mint the OIDC identity.
22. Initial npm package registration must use a Bootstrap Publication because
    npm requires the package to exist before a trusted publisher can be
    configured.
23. The Bootstrap Publication must publish `@yargote/matty@0.1.0` from the
    canonical GitHub Actions workflow using
    `npm publish --provenance --access public`.
24. The bootstrap credential must be a granular npm token with the minimum
    available scope, permissions, and expiration, stored only as a protected
    workflow secret.
25. The bootstrap workflow and logs must not expose the token.
26. After publication, maintainers must verify `0.1.0`'s public provenance,
    configure the canonical workflow as the npm OIDC trusted publisher,
    disallow token-based publication, revoke the bootstrap token, and remove
    its workflow secret.
27. Completion evidence for each bootstrap step must be recorded in the
    `0.1.0` release record.
28. Matty must not publish a placeholder or reservation version before `0.1.0`.
29. No later Matty release may use the bootstrap credential path.
30. For every release after `0.1.0`, CI must submit the exact validated artifact
    with `npm stage publish`.
31. A maintainer must download and inspect the exact staged tarball before
    approval.
32. Only an interactive maintainer approval with 2FA may make a staged release
    public.
33. A rejected or mismatched staged artifact must never be published; release
    CI must rebuild under a new reviewed staging attempt.
34. The release record must retain the staged package identifier, artifact
    digest, inspection result, approver, and public registry result.

### Release versioning

1. The first functional Matty release is `0.1.0`.
2. A patch release must not intentionally change workflow behavior.
3. A minor `0.x` release may change skills, Matty Rules, roles, contracts,
   bundled integration versions, or certified Pi targets, with documented
   incompatibilities.
4. Matty reserves `1.0.0` until its product contract has demonstrated stability
   beyond one Certified Pi Version and one Certified Target.
5. Every version remains a complete, inseparable release snapshot.

### Configuration and state ownership

1. Matty v1 must not create or read a user or project configuration file of its
   own.
2. Paths such as `~/.matty`, `.mattyrc`, and project `.matty` files must have no
   runtime meaning in v1.
3. Matty Rules, compatibility manifests, role definitions, Delegation
   Contracts, and Web Contracts must be immutable Package Contract Data shipped
   in the npm artifact.
4. Pi configuration, project instructions, Repository Preparation outputs, and
   `pi-web-access` configuration remain owned by their existing authorities and
   must not be reinterpreted as Matty configuration.
5. Matty must not persist activation, first-run, compatibility, or diagnostic
   state.
6. Session-scoped Research Workspaces and explicit project-owned Research
   Reports are operational artifacts, not Matty configuration.
7. Adding user-facing Matty configuration after v1 requires an explicit minor
   release with documented precedence, ownership, migration, and removal
   behavior.

### Diagnostic bootstrap and activation

1. The diagnostic bootstrap must be the smallest surface loaded before
   workflow activation.
2. The bootstrap must determine the running Pi version and required extension
   capabilities.
3. A compatible activation must register the complete core workflow.
4. A failed activation must register none of the shared skills, subagent
   runtime, or integration adapters.
5. A failed activation must not prevent Pi from becoming usable.
6. Degraded mode should retain `/matty status` and `/matty doctor` whenever Pi's
   extension contract permits command registration.
7. Startup must emit at most one actionable degraded warning.
8. Activation must not write project files or fetch shared skills.
9. If the bootstrap loads but Pi admits an Incomplete Shared Skill Catalog,
   activation must fail closed and diagnostics must identify each excluded
   resource.
10. If Pi excludes the bootstrap itself, Matty is disabled and cannot promise
    diagnostics.
11. Successful activation must show exactly one informational Startup Hint:
    `Matty active · /skill:ask-matt · /matty status`.
12. A degraded warning must replace, not accompany, the Startup Hint.
13. Matty must not persist first-run state, open a tutorial, or invoke
    `ask-matt` automatically.

### Shared skill catalog

1. The initial import must copy the complete contents of `skills/engineering`
   and `skills/productivity` verbatim from one exact upstream commit, including
   the support files used by those skills.
2. The package source must record the upstream repository and commit selected
   for each deliberate import.
3. Imported skills become Matty-owned source and may change independently.
4. Matty must not automatically track, merge, fetch, or synchronize upstream
   skill changes.
5. The catalog must be fixed for the package version.
6. `ask-matt` must be included and must route only to skills guaranteed by that
   catalog or to explicitly optional capabilities.
7. Shared skill names must be reserved.
8. A colliding project, global, or third-party skill must not replace a shared
   skill.
9. A collision must produce an actionable diagnostic identifying the name and
   source.
10. Non-colliding external skills must remain available.
11. Matty must not support individually enabled or disabled shared skills as a
    valid workflow state.
12. Pi retains authority to filter package resources; Matty must never rewrite
    Pi configuration to restore excluded resources.
13. Matty package disablement is the supported opt-out.

### Project skills and trust

1. Matty must rely on Pi's project trust decision.
2. Matty must not create a separate project approval store or grant.
3. Trusted project skills are discovered only from `.agents/skills`.
4. Project skills compose additively and remain project-owned.
5. Shared skills must never be copied into the project.
6. Removing Matty must not remove or modify project skills.

### Repository preparation

1. Matty installation and startup must not perform Repository Preparation.
2. Engineering workflows whose project-policy prerequisites are absent must
   direct the user to `setup-matt-pocock-skills`.
3. Repository Preparation must run only through explicit skill invocation.
4. The preparation skill must inspect and present its proposed changes before
   writing.
5. Preparation must rely on Pi project trust and preserve unrelated files.
6. Preparation artifacts remain project-owned and reviewable through Git.

### Subagent delegation

1. Matty must provide a shared subagent tool and curated roles as a core
   capability.
2. The initial Subagent Runtime must copy Pi `0.83.0`'s official subagent
   example and record its exact source commit.
3. After import, the runtime becomes Matty-owned source and must not depend on
   the example remaining a stable upstream API.
4. Every child process must inherit the active session's provider, model,
   authentication, and reasoning configuration.
5. Matty roles may vary prompts and allowed tools but must not silently select a
   different model or require another provider account.
6. Matty v1 must provide exactly five curated roles: `explorer`, `reviewer`,
   `designer`, `researcher`, and `worker`.
7. Each role must receive only the tools required for its declared
   responsibility. `worker` receives `read`, `write`, `edit`, `grep`, `find`,
   `ls`, and `bash`; it may modify the trusted working tree and validated
   temporary paths, install project-local dependencies, and run project checks.
8. `explorer`, `designer`, and `reviewer` must receive `read`, `grep`, `find`,
   `ls`, and `bash`. Explorer and designer shell access is intended for local
   Git history, CodeGraph, and diagnostic inspection; reviewer shell access
   also supports remote inspection through inherited `gh` authentication.
9. An Inspection Guard must allow inspection commands while blocking recognized
   mutating Git, GitHub, filesystem, and shell command families. It must block
   `gh` entirely for `explorer` and `designer`.
10. Diagnostics and documentation must state that the Inspection Guard is
    best-effort and not a security sandbox.
11. A GitHub-backed review must preflight `gh` availability and authentication
    when remote evidence is required.
12. A Worker Guard must block `gh`, Git index and reference mutations including
    commit, push, checkout, reset, and merge, global installations, writes
    outside the trusted working tree and validated temporary paths, and writes
    to real user configuration. It is best-effort rather than a security
    sandbox. Only the main agent may perform Git or GitHub publication,
    integration, comments, approvals, merges, or other external-state
    mutations.
13. Matty v1 must permit at most one active `worker` per repository.
14. Non-writing roles may execute concurrently within the runtime's bounded
    concurrency.
15. A contract requiring parallel workers must fail Delegation Preflight as
    unsupported rather than sharing one working tree between writers.
16. A subagent call must accept at most eight tasks and run at most four child
    Pi processes simultaneously.
17. Tasks beyond the active limit must be reported as queued and must not be
    described as simultaneously parallel.
18. Matty v1 must not expose user configuration for these limits.
19. `researcher` must not receive general `write`, `edit`, or `bash`; it receives
   a Matty-owned bounded research file tool and the certified `web_search`,
   `source_check`, `fetch_content`, and `get_search_content` tools. No other
   Matty Role receives those Web Capability tools.
20. Each research run must have a unique Research Workspace under
   `$TMPDIR/matty/research/<run-id>/` and exactly one prevalidated Research
   Report path in the repository.
21. The Research Report path must follow an existing project convention or
    default to `docs/research/<slug>.md`.
22. The bounded file tool must reject external absolute paths, traversal,
    symlink escapes, and unauthorized overwrites, and must return both workspace
    and report paths.
23. A Research Workspace must remain available for the complete parent session
    and be removed on clean session shutdown.
24. Startup cleanup may remove only marker-bearing Matty research workspaces
    older than 24 hours after validating their resolved paths.
25. Research Reports must never be included in workspace cleanup.
26. Matty Rules and the Delegation Contract must map upstream `Explore`,
   `background agent`, and `general-purpose` references to a concrete Matty
   Role.
27. Matty must not provide an unrestricted general-purpose role.
28. A Matty-owned Delegation Contract, stored separately from imported skill
   files, must classify each skill or delegation-dependent scenario as
   `required`, `optional`, or `none`.
29. The contract must declare required cardinality, independence, role,
   parallelism, and unavailable-capability behavior.
30. An explicit upstream requirement to delegate, execute in parallel, or
   isolate context must map to `required`.
31. `optional` may be used only when the skill permits semantically equivalent
   inline execution; absence of delegation instructions maps to `none`.
32. Ambiguous delegation language must require maintainer review rather than an
   inferred fallback.
33. Matty must run a Delegation Preflight before a classified skill or
   delegation-dependent branch produces effects.
34. Required delegation must fail the affected workflow clearly when its
   cardinality, independence, role, or parallelism cannot be satisfied.
35. Optional delegation may fall back to main-agent execution only with an
   observable skip reason.
36. Matty must not report sequential or same-process execution as independent
    parallel delegation.
37. A failed preflight must block only the affected skill or branch while
    leaving it discoverable and diagnosable.
38. A required multi-agent Delegation Group must succeed atomically.
39. Failure of any required member must fail the group, cancel work that can no
    longer satisfy the contract, and expose partial outputs only as diagnostics.
40. Matty must not invisibly retry an entire failed group or replace a failed
    member with inline or sequential execution.
41. Subagent failure must not prevent Pi or unrelated non-delegating workflows
    from operating.

### Matty rules

1. Matty must store host-compatibility rules as package-owned versioned data.
2. The Matty extension must inject those rules through `before_agent_start`
   into both parent and child Pi processes.
3. The injected block must use `<!-- matty:rules -->` and
   `<!-- /matty:rules -->` markers and must never appear more than once in a
   system prompt.
4. Matty Rules must translate imported host-specific delegation language to the
   Subagent Runtime without editing imported skill files.
5. The `subagent` tool must also provide tool-specific `promptGuidelines`
   describing its exact batching and role schema.
6. Matty must not write or replace global, parent-directory, or project
   `AGENTS.md` files to install these rules.
7. Matty Rules must govern only Matty workflow invariants; external
   `AGENTS.md` files retain authority over project conventions and policy.
8. Project instructions may extend Matty behavior but must not silently
   redefine a Matty invariant.
9. A detected direct conflict must block and diagnose only the affected
   workflow path.

### Diagnostic redaction

1. Status, doctor, and error output must be produced from one shared closed
   allowlist.
2. Allowed fields are package and Pi versions, state and error codes, skill and
   role names, source kinds, active provider and model identifiers, the web
   provider used, normalized paths, remediation text, and bounded concurrency
   state.
3. Paths must replace sensitive roots with `<home>`, `<project>`, or `<tmp>`.
4. Tokens, cookies, headers, environment values, provider configuration
   contents, prompts, research content, file contents, sensitive URL
   components, raw external stderr, and revealing absolute paths are forbidden.
5. Fields not explicitly allowed must be omitted by default.
6. External errors must be mapped into Matty-owned codes and safe summaries
   before rendering or persistence.
7. Human and JSON command output must be rendered from the same Redacted
   Diagnostic snapshot.
8. The Diagnostic Schema must include a top-level `schemaVersion`, beginning at
   `1`, and a top-level `command` identifying `status` or `doctor`.
9. JSON output must contain valid JSON only, with no ANSI sequences, banners,
   or surrounding prose.
10. A breaking change to the Diagnostic Schema requires a new schema version;
    additive fields must still satisfy the closed allowlist.

### Optional web access

1. Matty `0.1.0` must bundle exactly `pi-web-access@0.15.0`, without a semver
   range, for workflows that need current information.
2. The dependency must be bundled complete and unmodified rather than maintained
   as a Matty fork.
3. Matty v1 certifies only `web_search`, `source_check`, `fetch_content`, and
   `get_search_content`.
4. Other `pi-web-access` capabilities remain available under that package's own
   contract but are outside Matty's v1 guarantees.
5. Matty must not enable browser-cookie access or write `pi-web-access`
   configuration automatically.
6. The integration must be tested with Pi authenticated through a ChatGPT/Codex
   subscription and every Reference Model Path Matty claims.
7. Matty must not claim subscription-backed OpenAI search until the packed
   package test proves a real search and cited result without a separate OpenAI
   API key.
8. Web access must expose `available`, `degraded`, or `unavailable` state.
9. Web failures must be isolated from the core workflow.
10. `pi-web-access` configuration, credentials, sessions, and stored state must
   remain outside Matty ownership.
11. `pi-web-access` may use its internal Exa MCP fallback without Matty exposing
   a general MCP integration or configuration surface.
12. Search results and diagnostics must identify the provider actually used.
13. Provider-owned configuration must allow a user to pin OpenAI and disable
   automatic fallback without Matty rewriting that configuration.
14. Matty must not delete or migrate provider-owned state during package
   lifecycle operations.
15. Diagnostic output must follow a closed redaction contract.
16. Phase 0 may replace the `0.15.0` candidate before Matty `0.1.0` is
    published if complete compatibility validation fails.
17. After `0.1.0` is published, changing the bundled `pi-web-access` version
    requires a new Matty minor release and complete compatibility validation.
18. A Matty-owned Web Contract, stored separately from imported skill files,
    must classify every skill or web-dependent branch as `required`, `optional`,
    or `none`.
19. Matty must run a Web Preflight before a classified skill or branch produces
    effects.
20. If a `required` Web Capability is unavailable, Matty must block only the
    affected path with actionable remediation.
21. Matty must not silently substitute model knowledge for a `required` Web
    Capability.
22. If an `optional` Web Capability is unavailable, the path may continue only
    with an explicit disclosure that no web research was performed.
23. A `none` classification must not introduce a web availability dependency.
24. A required web operation that becomes unavailable after preflight must fail
    the affected path clearly; an optional operation may continue only with the
    same explicit disclosure.
25. The parent agent and `researcher` role must receive the four certified Web
    Capability tools.
26. `explorer`, `designer`, `reviewer`, and `worker` must not receive those
    tools; external research for those roles must be assigned to `researcher`
    or performed by the parent.
27. This tool assignment must not redefine the reviewer's explicitly allowed
    read-only `gh` inspection or the worker's explicitly allowed
    project-local dependency installation as web research.

### Updates and runtime network behavior

1. Runtime startup must not fetch or update shared skills.
2. Upstream skill changes must enter through source review and a new package
   publication.
3. Each source review must identify the exact upstream commit and affected
   catalog files.
4. Upstream reconciliation must preserve Matty changes by default and accept or
   replace differences skill by skill.
5. Package updates must be explicit through Pi's supported package lifecycle.
6. Matty may report update availability only if Pi exposes that information
   locally without Matty making a network request or creating a second updater.
7. Matty must not emit telemetry, analytics, usage metrics, automatic crash
   reports, or Matty-owned update checks.
8. Startup, `/matty status`, and `/matty doctor` must perform no network
   requests or live provider probes.
9. Matty-owned network initiation is permitted only for a User-Directed Network
   Operation required by the active workflow.
10. Permitted operations include normal parent and child Pi model requests, Web
    Capability calls, reviewer read-only GitHub inspection, and worker
    project-local dependency installation.
11. Pi-owned package installation, update, disablement, and removal remain
    outside Matty's network ownership.
12. Third-party services used during a User-Directed Network Operation retain
    their own data-processing contracts; Matty must not add a separate
    reporting request.

### Third-party provenance

1. The package must retain the MIT copyright and permission notices for the
   imported `mattpocock/skills` source, Pi subagent example, and bundled
   `pi-web-access` dependency.
2. Release metadata must record the exact imported commits and bundled
   dependency versions.
3. The packed artifact must contain a human-readable third-party notices file.
4. Source review must distinguish upstream content from later Matty-owned
   changes without implying endorsement by the original authors.
5. Release metadata must include the reviewed dependency lifecycle-script
   inventory for the exact lockfile and packed artifact.
6. Matty-owned source must be published under the MIT license in a root
   `LICENSE` file.
7. Matty's own license notice must read
   `Copyright (c) 2026 Yerson Argote`.
8. `THIRD_PARTY_NOTICES` must preserve each third party's license and copyright
   notice separately from Matty's own notice.
9. Package metadata, documentation, and notices must not merge authorship or
   imply endorsement by Matt Pocock, Pi's authors, or `pi-web-access`'s author.

## Compatibility Policy

1. Matty v1 certifies only Pi `0.83.0` on macOS Apple Silicon.
2. The v1 runtime requires Node.js `>=22.19.0`, matching Pi `0.83.0`.
3. Each later admitted Pi version must pass the complete acceptance suite for the
   exact Matty version and target.
4. Covering the three most recent Pi minor lines is a post-MVP goal, not a v1
   requirement or guarantee.
5. A Pi line is supported only after certification.
6. Matty must not declare blanket compatibility with all Pi `0.x.x` versions.
7. Internal Pi-version adapters may vary, but they remain behind one Matty
   activation interface.
8. An uncertified Pi version or operating-system/architecture target produces
   degraded Matty activation while Pi remains usable.
9. The v1 Reference Model Path is GPT-5.6 using ChatGPT/Codex subscription
   authentication.
10. The active model is inherited from Pi and must not be a core activation
    gate.
11. A model outside the Reference Model Path must be reported as unverified in
    status and doctor while Matty remains active.
12. Matty must not select, replace, or rewrite the user's active model,
    provider, authentication, or reasoning settings.
13. A workflow path may be blocked only when its concrete capability preflight
    fails, not merely because the model differs from the Reference Model Path.
14. Phase 0 must record the exact provider and model identifiers Pi exposes for
    the GPT-5.6 Reference Model Path.

## Commands

### `/matty status`

Returns a concise, redacted view containing:

- Matty package version;
- running Pi version;
- certified Pi compatibility set;
- running and certified operating-system/architecture targets;
- active provider and model identifiers;
- Reference Model Path match or unverified state;
- activation state and reason;
- shared catalog state and excluded resources;
- project-skill admission and collisions;
- subagent availability and conditionally blocked workflow paths;
- subagent active, queued, and concurrency-limit state;
- web availability.

Status is observational and performs no repair or configuration writes.
`/matty status --json` returns the same snapshot through Diagnostic Schema
version `1`. Status performs no network request or live capability probe.

### `/matty doctor`

Evaluates the same observable state as status and adds ordered remediation.
Doctor must distinguish:

- unsupported Pi version;
- failed core activation;
- shared skill collision;
- untrusted project inputs;
- unavailable subagent surface;
- missing optional provider configuration;
- provider runtime failure.

Doctor performs no destructive repair automatically.
`/matty doctor --json` returns the same diagnostic snapshot and ordered
remediation through Diagnostic Schema version `1`. Doctor performs no network
request or live capability probe.

Both commands render only Redacted Diagnostics and must not expose raw provider,
subprocess, filesystem, GitHub, or Pi errors. Their JSON modes emit valid JSON
only, without ANSI styling or explanatory prose.

## Implementation Decisions

1. Build Matty as a greenfield project, not as a refactor or compatibility
   layer over Pilly.
2. Use Pi's native package extension seam as the product's external interface.
3. Keep the diagnostic bootstrap separate from transactional workflow
   activation.
4. Place Pi-version differences behind internal adapters selected by capability
   detection and exact certified version.
5. Treat GPT-5.6 with ChatGPT/Codex subscription authentication as a Reference
   Model Path rather than a global activation requirement.
6. Treat the shared skill catalog as package data validated at build and
   activation time.
7. Give shared skill names deterministic ownership independent of discovery
   order.
8. Compose project skills only after Pi reports project trust.
9. Treat the subagent runtime as a core Matty module whose workflow obligations
   are data declared by a Delegation Contract outside imported skill files.
10. Inject Matty Rules through the Pi extension seam rather than external
   context-file mutation.
11. Treat web as an optional adapter with explicit availability and diagnostics.
12. Store Web Contracts outside imported skill files and enforce them at the
    classified skill or branch boundary.
13. Return structured diagnostic results before rendering user-facing command
   output.
14. Keep provider state, project state, and Pi settings outside Matty-owned
   storage.
15. Use Pi's package lifecycle instead of creating a Matty lifecycle.
16. Retain compatibility evidence in CI and release automation, not as local
   user-managed state.
17. Do not carry Pilly commands, storage formats, release concepts, or legacy
    migration code into Matty.
18. Treat Package Contract Data as immutable release contents and provide no
    Matty-owned configuration lookup or persistence layer in v1.

## Testing Decisions

### Primary seam

The highest and primary test seam is a real Pi process loading the packed Matty
package. Acceptance tests must exercise installation artifacts and observable
Pi behavior rather than importing Matty internals.

The primary suite verifies:

- the package is installable by Pi;
- the Matty package declares no installation lifecycle scripts;
- the resolved production dependency lifecycle-script inventory matches the
  reviewed release metadata;
- normal Pi startup loads Matty;
- diagnostic commands are registered;
- a certified Pi version activates the complete workflow;
- an uncertified version leaves Pi usable and Matty degraded;
- shared skills are present and collisions cannot replace them;
- trusted project skills compose additively;
- startup creates no project writes;
- removal or disablement returns Pi to non-Matty behavior.

### Provider adapter seam

The `pi-web-access` integration is tested against deterministic fakes beneath
the packed-package seam and through a real Pi load test. At least one acceptance
scenario authenticates Pi with ChatGPT/Codex subscription credentials, runs a
GPT-5.6 session through the Reference Model Path, performs web search without a
separate OpenAI API key, and verifies cited output. Tests also assert observable
availability, degradation, redaction, and zero Matty ownership of provider
state.

### Repository preparation seam

Repository Preparation tests run in isolated temporary repositories. They
verify missing-precondition guidance, explicit invocation, user consent,
preservation of unrelated files, and absence of startup writes.

### Subagent seam

Delegation acceptance tests run distinct child executions and verify identity,
cardinality, cancellation, aggregation, and returned results. Required
parallelism must demonstrate independent execution rather than infer it from
configuration. Contract validation must cover every imported skill and reject
unknown skills, missing classifications, and unsatisfiable declarations. Tests
must also prove the imported runtime launches separate Pi processes on the
certified version and target, using the same active provider, model,
authentication, and reasoning settings as the parent session. Parent and child
prompt assertions must prove exactly one marked Matty Rules block and the
expected `subagent` tool guidelines. Role tests must prove that non-writing
roles cannot invoke write tools, and Inspection Guard tests for `explorer`,
`designer`, and `reviewer` must cover representative allowed inspection and
blocked mutation commands without claiming complete shell isolation. Explorer
and designer tests must exercise local Git, CodeGraph, and diagnostic
inspection and reject `gh`; reviewer tests must permit representative read-only
`gh` commands. Worker Guard tests must permit working-tree edits,
project-local dependency installation, and project checks while rejecting
representative `gh`, Git-state mutation, global-installation, external-path,
and user-configuration writes without claiming complete isolation. Role
exposure tests must prove that only `researcher` receives the four certified
Web Capability tools. Research file tests must
cover traversal, absolute-path escape, symlink escape, unauthorized overwrite,
parallel workspace isolation, the one-report invariant, clean shutdown cleanup,
and marker-validated stale cleanup in a sandboxed temporary root. Delegation
group tests must inject member failures and assert cancellation, atomic failure,
diagnostic-only partial results, and absence of hidden retries or fallbacks.

### Diagnostic seam

Table-driven tests must attempt to inject secrets, credentials, sensitive URLs,
absolute paths, file contents, prompts, research artifacts, and raw external
errors through every diagnostic producer. Only allowlisted fields and
normalized paths may reach structured or rendered output. Contract tests must
also validate both commands against Diagnostic Schema version `1`, prove that
JSON mode contains no ANSI or surrounding prose, and compare human and JSON
rendering against the same diagnostic snapshot.

### Compatibility matrix seam

Release CI runs the same packed-package acceptance suite against every exact Pi
version and target proposed for certification. Compatibility metadata is
generated only from passing matrix entries.

The complete suite must exercise the Reference Model Path and record Pi's exact
provider and model identifiers. A separate startup scenario with a
non-reference model must prove that the core activates, the model is reported
as unverified, and only a failed concrete capability preflight can block a
workflow path.

### Test principles

- Test external behavior before internal implementation.
- Test the packed artifact, not only the source checkout.
- Install the artifact in an isolated environment and prove that Matty-owned
  lifecycle code does not execute.
- Use isolated HOME, Pi configuration, provider state, and project roots.
- Never write tests against the operator's real configuration.
- Assert that conventional Matty configuration paths are neither read nor
  created.
- Deny network access during startup, status, and doctor tests and assert that
  no request is attempted.
- Assert that every permitted Matty-initiated request is attributable to a
  User-Directed Network Operation and that no separate reporting request is
  emitted.
- Assert fail-open behavior for Pi and fail-closed activation for Matty.
- Assert no network or filesystem writes where the contract forbids them.
- Prefer one real Pi acceptance harness reused across capabilities.
- Add focused module tests only where they provide faster failure localization
  beneath the acceptance seam.

## Success Criteria

The MVP is successful when all of the following are true:

1. A clean supported environment can run:

   ```sh
   pi install npm:@yargote/matty
   pi
   ```

   and immediately access the complete shared workflow.
2. `/skill:ask-matt`, `/matty status`, and `/matty doctor` are observable in a
   real Pi session.
3. Successful startup displays the Startup Hint, while degraded startup
   displays one actionable warning instead.
4. A project or global skill cannot silently replace a reserved shared skill.
5. A trusted project can add a non-colliding skill without copying Matty skills.
6. Every catalog workflow has a validated Delegation Contract, and at least one
   required parallel workflow demonstrates independent execution. Optional
   fallback is demonstrated only when the catalog contains an authentic
   optional scenario.
7. Web access demonstrates available and degraded states without disabling the
   core workflow. A required Web Contract blocks only its affected path, while
   an optional contract continues only with explicit disclosure.
8. An uncertified Pi version still opens and exposes actionable Matty
   diagnostics without loading a partial workflow.
9. Pi startup produces no project writes and performs no shared-skill update.
10. The packed package passes the compatibility matrix for every version and
    target listed as certified.
11. An unprepared repository receives actionable guidance, and explicit
    `setup-matt-pocock-skills` execution creates only consented, project-owned
    policy files.
12. The published package is an Install-Safe Artifact with no unreviewed
    dependency lifecycle scripts.
13. The source and packed artifact contain Matty's MIT license and the complete,
    separately attributed third-party notices.
14. The public npm release exposes verifiable provenance linking it to the
    canonical public repository and release workflow.
15. The configured trusted publisher has stage-only authority, and the routine
    release procedure requires exact-tarball inspection plus maintainer 2FA.

## Delivery Roadmap

### Phase 0 — Validate Pi extension seams

Goal: prove the external contracts before committing to module shapes.

- Create the greenfield package scaffold.
- Pack and install a minimal Matty extension through Pi.
- Prove the package requires no Matty-owned installation lifecycle scripts and
  establish the dependency lifecycle-script inventory format.
- Validate command registration in normal and degraded startup.
- Validate resource discovery, skill collision behavior, and project trust
  hooks on Pi `0.83.0`.
- Validate the official subagent extension surface.
- Record any Pi-version adapter requirements.

Exit gate: a packed prototype demonstrates the install, startup, diagnostic,
skill-precedence, and subagent seams required by this PRD.

The skill-precedence proof is an Activation Safety Gate: it must demonstrate
that an external colliding definition cannot reach the model under a reserved
name before Matty blocks activation. Discovery order alone is not sufficient.
If Pi `0.83.0` cannot satisfy this gate, Matty v1 must not be published.

### Phase 1 — Diagnostic bootstrap and activation

Goal: establish the safe host relationship.

- Implement package metadata and native Pi loading.
- Implement compatibility manifest consumption.
- Implement transactional core activation.
- Implement degraded activation with one warning.
- Implement structured status and doctor results.

Exit gate: supported and unsupported Pi acceptance scenarios pass without
affecting unrelated Pi behavior.

### Phase 2 — Shared workflow and precedence

Goal: deliver the consistent cross-project workflow.

- Package the initial fixed shared skill catalog.
- Record and validate its exact upstream commit and complete source-tree
  coverage.
- Validate catalog completeness and reserved names.
- Register shared skills deterministically.
- Detect and report collisions across project, global, and package sources.
- Integrate `ask-matt` against the guaranteed catalog.

Exit gate: the real Pi harness proves catalog completeness, precedence, and
non-colliding coexistence.

### Phase 3 — Project composition

Goal: support repository-specific extension and explicit policy preparation.

- Compose trusted `.agents/skills` additively.
- Implement project-skill collision diagnostics.
- Detect missing Repository Preparation and guide the user.
- Validate explicit `setup-matt-pocock-skills` behavior in isolated
  repositories.
- Verify startup remains read-only.

Exit gate: isolated repository scenarios pass for trust, additive skills,
collisions, explicit preparation, preservation, and read-only startup.

### Phase 4 — Subagent delegation

Goal: make delegation a dependable workflow capability.

- Package the shared subagent tool and roles.
- Implement and validate the five least-privilege Matty Roles.
- Implement the bounded research file tool and its two write zones.
- Import and adapt Pi `0.83.0`'s subagent example with recorded provenance.
- Inject deduplicated Matty Rules into parent and child prompts.
- Define and validate the Delegation Contract for every imported skill and
  delegation-dependent scenario.
- Implement required, optional, and none behavior.
- Implement cancellation, bounded concurrency, and aggregation.
- Demonstrate real independent parallel execution.

Exit gate: delegation acceptance scenarios prove required blocking, optional
fallback, distinct child execution, and returned results.

### Phase 5 — Optional web access

Goal: support current-information research without coupling core availability.

- Implement the web adapter and diagnostics.
- Bundle exactly `pi-web-access@0.15.0` and validate its ChatGPT/Codex
  subscription path with every claimed Certified Pi Version, Certified Target,
  and Reference Model Path combination. Replace the candidate before `0.1.0`
  rather than publishing if validation fails.
- Define and validate a Web Contract for every imported skill and
  web-dependent branch.
- Implement required, optional, and none preflight and runtime-failure behavior
  without silent model-knowledge substitution.
- Enforce provider ownership and diagnostic redaction.
- Prove graceful degradation.

Exit gate: web access passes required-blocking, optional-disclosure, none,
available, degraded, unavailable, redaction, and state-preservation scenarios.

### Phase 6 — Compatibility and first release

Goal: publish an evidence-backed package.

- Run the packed-package suite against Pi `0.83.0` on macOS Apple Silicon.
- Exercise and record the GPT-5.6 ChatGPT/Codex Reference Model Path, and prove
  that a non-reference model does not become a core activation failure.
- Generate the exact certified compatibility set.
- Verify install, update, disable, and remove behavior through Pi.
- Verify the final Install-Safe Artifact and reviewed dependency
  lifecycle-script inventory.
- Verify public package metadata, exact repository identity, OIDC publishing,
  absence of routine write tokens, and npm provenance.
- Verify that the trusted publisher has stage-only authority and that the
  routine release workflow cannot invoke direct publication.
- Execute the one-time Bootstrap Publication for `0.1.0`, verify its public
  provenance, configure OIDC, disallow token publishing, and record token
  revocation and secret removal.
- Validate package contents, licenses, and upstream skill provenance.
- Verify Matty's MIT license remains distinct from every retained third-party
  notice and attribution.
- Verify the packed third-party notices and exact imported source references.
- Publish the first Matty package only after all claimed matrix entries pass.

Exit gate: `@yargote/matty` satisfies every MVP success criterion from a clean
environment.

## Out of Scope

- Pilly migration, import, cleanup, command aliases, or state compatibility.
- `matty run` or another interactive launcher.
- A globally installed Matty CLI for normal users.
- Internal release materialization, activation pointers, promotion, rollback,
  or local compatibility certificates.
- Per-skill toggles, user-defined profiles, or project replacement of shared
  skills.
- Runtime downloading or self-updating of skills.
- Automatic project writes during startup.
- Certified support for project-local Matty package installations.
- Matty-owned user or project configuration files and persistent runtime state.
- Matty-owned telemetry, analytics, automatic crash reporting, or update probes.
- A separate `/matty setup` command or automatic project scaffolding.
- Automatic provider credential setup.
- Ownership, backup, migration, or deletion of provider data.
- MCP and Engram integrations in the MVP.
- Blanket support for all Pi `0.x.x` versions.
- Linux, Windows, and non-Apple-Silicon macOS certification in v1.
- A graphical interface.

## Risks and Required Discovery

1. Pi may not currently expose enough control to guarantee reserved skill
   precedence after other packages have loaded. Pi `0.83.0` currently reports
   first-wins collisions without a public priority or reservation API. Phase 0
   must prove fail-closed interception before the wrong content reaches the
   model; otherwise release is blocked.
2. Pi's command registration may not permit the diagnostic bootstrap to survive
   every incompatibility. Matty must define the minimum observable fallback
   supported by each certified version.
3. The official subagent surface may differ across Pi minor lines. The
   compatibility window depends on containing those differences without
   weakening independence guarantees.
4. The web integration may assume ownership of configuration or process
   lifecycle. Its adapter must prove that Matty can remain non-custodial.
5. Project trust and resource-discovery timing may constrain when additive
   project skills can be admitted.

If any of these questions cannot be settled from Pi's documented and
observable extension contract, create a throwaway prototype for that single
question before designing the production module.

## Further Notes

- Matty should optimize for a simple interface with substantial behavior behind
  it: install one Pi package, open Pi, and receive one coherent workflow.
- Reproducibility remains valuable as package construction and CI evidence, not
  as a user-facing lifecycle.
- Compatibility is a measured set of exact versions, not a hopeful range.
- Web access is part of the MVP but not a prerequisite for the shared workflow.
- MCP and Engram may be considered after the MVP.
- The roadmap should next be converted into blocker-aware tracer-bullet tickets
  in the Matty repository.
