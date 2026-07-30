# Matty Core v0.1 Product Requirements

## Proposal

Matty Core is a globally installed Pi package that makes delegated engineering
work dependable. Version `0.1` owns a process-based Subagent Runtime, five
least-privilege roles and their guards, Matty Rules, a bounded Web Capability,
local status and doctor diagnostics, and the release controls needed to ship a
safe public artifact.

The MVP deliberately does not distribute or orchestrate a shared skill
library. It supplies the runtime primitives on which workflows can operate
without taking ownership of those workflows, their repository setup, or their
per-workflow capability contracts.

Pi remains the host, interactive entry point, provider and model authority,
authentication owner, project-trust authority, and package loader. Matty adds
only the Core capabilities described here.

## Problem

Pi can launch extensions and tools, but a user who wants repeatable
multi-agent engineering work still needs:

- a maintained child-process runtime with bounded concurrency and cancellation;
- roles whose permissions match common exploration, review, design, research,
  and implementation tasks;
- consistent runtime rules in parent and child contexts;
- a supported route to cited web research;
- actionable, share-safe diagnostics when a capability is unavailable;
- an exact statement of the host combination that has actually been tested;
- and a package and publication process that do not introduce hidden install or
  background behavior.

Without one owned layer, each workflow must reconstruct these foundations and
users cannot distinguish tested behavior from host-version assumptions.

## Goals

1. Ship a Matty-owned Subagent Runtime based on separate Pi child processes.
2. Provide `explorer`, `reviewer`, `designer`, `researcher`, and `worker` roles
   with least-privilege tool surfaces and explicit best-effort guards.
3. Validate delegated and web-enabled Core operations through Capability
   Contracts.
4. Enforce Single Writer and Bounded Concurrency invariants.
5. Inject versioned Matty Rules into parent and child prompts without writing
   global or project instruction files.
6. Bundle and certify a focused Web Capability for the parent and researcher.
7. Keep core activation observable through `/matty status` and
   `/matty doctor`, including versioned JSON output.
8. Certify one exact Pi version and one exact platform target using the packed
   artifact and real processes.
9. Emit Zero Telemetry and perform no background network requests.
10. Publish a prebuilt Install-Safe Artifact.
11. Publish publicly with provenance and use OIDC trusted publishing for
    routine releases.

## Non-goals

The Matty Core `0.1` MVP does not include:

- a Shared Skill Catalog or any Matty-owned workflow library;
- `ask-matt` or another workflow router;
- upstream skill import, reconciliation, provenance, or name-collision policy;
- Repository Preparation or project-policy setup;
- capability, delegation, or web contracts classified per skill;
- per-skill toggles, profiles, discovery, precedence, or activation;
- a standalone Matty executable;
- project-local installation certification;
- a security sandbox or protection against a malicious child process;
- Matty-owned provider credentials, model selection, or web configuration;
- Matty-owned telemetry, analytics, crash reporting, or update checks;
- automatic release approval or routine token-based npm publication;
- certification of Pi versions or platforms beyond the exact tested target.

External workflows may use Matty Core, but their content, setup, trust, and
capability requirements remain outside this MVP.

## User stories

1. As a developer, I want to delegate independent tasks to real child Pi
   processes so that each task has isolated context and a structured outcome.
2. As a developer, I want bounded concurrency and cancellation so that
   delegation does not create unbounded or orphaned work.
3. As a developer, I want to choose a named least-privilege role so that a
   child's permissions match its responsibility.
4. As a repository owner, I want at most one delegated writer so that parallel
   agents do not race over the same working tree.
5. As a researcher, I want cited web tools through my existing Pi
   authentication path so that I do not need a second Matty credential.
6. As a developer, I want Matty Rules applied consistently to parent and child
   contexts without Matty editing my instruction files.
7. As a developer, I want status to explain the active runtime, host
   certification, roles, web availability, and degraded capabilities.
8. As a developer, I want doctor to give ordered remediation without exposing
   secrets, prompts, file contents, or sensitive paths.
9. As an operator, I want startup and diagnostics to remain local so that Matty
   never probes providers or reports usage in the background.
10. As a maintainer, I want the shipped tarball to be prebuilt and audited for
    lifecycle scripts so that installation does not execute Matty-owned code.
11. As a maintainer, I want releases tied to the canonical repository and
    workflow through provenance and OIDC.
12. As a user on an uncertified host, I want an explicit unsupported result
    rather than a broad compatibility promise.

## Functional requirements

### Activation and host boundary

1. Matty must load as a Pi package and must not require a separate executable.
2. Matty `0.1` must support only global installation through Pi.
3. Activation must not write project files, global instruction files, provider
   configuration, or real user configuration.
4. Matty must inherit provider, model, authentication, reasoning, current
   directory, and trust decisions from Pi.
5. Core capabilities must have explicit active, degraded, unavailable, or
   uncertified states.
6. A failure in an optional integration must not make Pi unusable.
7. Diagnostics must remain available whenever Pi can load Matty's diagnostic
   bootstrap.

### Subagent Runtime

1. Matty must own and maintain the runtime adapted from Pi `0.83.0`'s official
   subagent example.
2. Each child must run as a separate Pi process with an explicit canonical
   working directory and inherited session configuration.
3. Each run must have a Matty-generated run identifier and verified child
   process/session identity.
4. The runtime must emit ordered structured progress and one terminal success,
   failure, or cancellation result.
5. Malformed child output, mismatched session identity, unexpected exit, and
   missing terminal output must fail explicitly.
6. Cancellation must target only a still-open child, first with graceful
   termination and then forced termination after a bounded grace period.
7. One delegation call must accept at most eight tasks.
8. At most four child processes may be active for one call; excess accepted
   tasks must be reported as queued.
9. Required grouped work must not be presented as wholly successful when one
   member fails; partial output may appear only as diagnostic context.
10. The runtime must never silently replace requested parallel child execution
    with inline parent execution.

### Capability Contracts

1. Every delegated or web-enabled Matty Core operation must have a versioned
   Capability Contract.
2. A contract must declare its Matty Role, tool surface, write authority, web
   requirement, cardinality, concurrency, independence, and failure behavior.
3. Capability Preflight must reject ambiguous, incompatible, or unavailable
   required behavior before the operation produces effects.
4. Optional fallback is permitted only when the contract declares it and must
   produce an observable disclosure.
5. A failed required group must cancel pending group work and must not be
   represented as a successful inline or sequential substitute.
6. Contracts describe Core operations, not external workflows or skills.

### Roles and guards

1. Matty must provide exactly five roles in `0.1`: `explorer`, `reviewer`,
   `designer`, `researcher`, and `worker`.
2. Role definitions and guard policy must be package-owned, versioned data.
3. `explorer` and `designer` may inspect local code, files, diagnostics, and Git
   history but may not intentionally mutate local or remote state.
4. `reviewer` has the same local inspection boundary and may inspect remote
   GitHub state through inherited `gh` authentication, but may not mutate it.
5. `researcher` may use the certified Web Capability, write temporary research
   artifacts, and persist at most one cited Markdown report to a prevalidated
   repository path.
6. `worker` may edit the trusted working tree, use validated temporary paths,
   install project-local dependencies, and run project checks.
7. The Worker Guard must block recognized GitHub mutation, Git index/reference
   mutation, global installation, external-path writes, and real
   user-configuration writes.
8. Inspection and Worker Guards must be documented as best-effort command
   policies, not security boundaries.
9. Only one `worker` may be active for a repository at a time.
10. The main agent retains ownership of commits, pushes, pull requests, review
    submission, merges, releases, and other external-state mutation.

### Matty Rules

1. Matty Rules must define the Core runtime invariants and role semantics.
2. Rules must be injected into parent and child system prompts through Pi's
   supported extension seam.
3. The injected block must use stable `matty:rules` markers and be deduplicated.
4. Matty must not write or modify external `AGENTS.md` files.
5. Project instructions may add repository policy but must not silently relax a
   Core safety invariant.
6. Tool guidance must document the exact delegation schema, role names,
   concurrency limits, guard limitations, and failure behavior.

### Web Capability

1. Matty must bundle exactly `pi-web-access@0.15.0`.
2. The certified surface is `web_search`, `source_check`, `fetch_content`, and
   `get_search_content`.
3. The four tools must be available only to the parent and `researcher`.
4. Credentials, provider selection, fallback behavior, configuration, and
   stored state remain owned by Pi and `pi-web-access`.
5. Matty must not enable browser-cookie access or write provider configuration.
6. Provider selection must remain observable at a safe diagnostic seam.
7. Required, optional, and absent web access must follow the calling
   Capability Contract during preflight and runtime failure.
8. If web access is unavailable or fails, the calling parent or researcher must
   receive an explicit failure or declared optional disclosure; Matty must not
   present model knowledge as web research.
9. Capabilities outside the four certified tools remain provider-owned and
   outside the Matty support contract.

### Status, doctor, and redaction

1. `/matty status` and `/matty doctor` must work without network access.
2. Human and `--json` output must derive from the same diagnostic snapshot.
3. JSON output must use Diagnostic Schema version `1`, contain a top-level
   command discriminator, and emit valid JSON without ANSI or surrounding text.
4. Diagnostics must use a closed allowlist. Unknown fields must be omitted.
5. Allowed information may include package and Pi versions, target and
   certification state, Core capability states, role names, concurrency state,
   coarse web provider identity, normalized paths, error codes, and remediation.
6. Tokens, cookies, headers, environment values, provider configuration,
   prompts, queries, research or file content, raw external stderr, sensitive
   URLs, and revealing absolute paths must never be emitted.
7. `/matty status` must summarize activation, exact-host certification,
   Subagent Runtime, roles/guards, Matty Rules, web availability, and active or
   queued child work.
8. `/matty doctor` must inspect the same state and add ordered remediation for
   unsupported host, runtime launch, role data, rule injection, web integration,
   dependency, and artifact-integrity failures.

### Exact host certification

1. Matty `0.1` must certify exactly Pi `0.83.0` on macOS Apple Silicon.
2. The Reference Model Path is `openai-codex/gpt-5.6-sol` with ChatGPT/Codex
   subscription authentication.
3. A different active model is unverified and must be reported, but does not by
   itself disable Core activation.
4. A different Pi version or target is uncertified until that exact combination
   passes the complete acceptance suite.
5. Certification must exercise the packed npm artifact, real Pi parent and
   child processes, all roles and guards, diagnostics, rule injection, and the
   pinned web integration.
6. Compatibility claims must come from the acceptance suite, not semver ranges
   or source inspection alone.

### Network and privacy

1. Matty must emit Zero Telemetry.
2. Startup, status, and doctor must make no network requests or live provider
   probes.
3. Network requests may occur only through a visible User-Directed Network
   Operation.
4. Matty must not duplicate provider reporting or add a parallel analytics
   channel.
5. Any future telemetry requires a new explicit product and architecture
   decision covering consent, disclosure, ownership, retention, and opt-out.

### Packaging and publication

1. `@yargote/matty` must ship precompiled JavaScript and required package data.
2. The package must declare no Matty-owned install lifecycle script.
3. Release CI must inspect the exact production dependency tree for lifecycle
   scripts; new or changed unreviewed scripts block publication.
4. Package contents, licenses, dependency inventory, repository identity, and
   artifact digest must be verified before publication.
5. The public package must identify the case-exact canonical public repository
   and publish with npm provenance.
6. After the one-time bootstrap, routine releases must use the canonical
   GitHub-hosted workflow and npm OIDC trusted publishing without a long-lived
   write token.
7. Routine CI may submit a staged artifact but must not make it public
   automatically.
8. A maintainer must inspect the exact staged artifact and approve publication
   with 2FA.
9. The bootstrap token must be minimally scoped, short-lived, protected from
   logs, revoked immediately after trusted-publisher configuration, and never
   reused.

## Quality requirements

1. Core domain behavior must have focused deterministic tests.
2. Process lifecycle tests must cover success, failure, malformed output,
   cancellation, timeout, and cleanup.
3. Role tests must prove allow and deny cases for every guard and Single Writer.
4. Capability Contract tests must cover required, optional, unavailable,
   incompatible, grouped, and cancellation behavior.
5. Diagnostic tests must prove schema validity and secret/path redaction.
6. Network-denied tests must prove startup, status, and doctor remain local.
7. Packed-artifact tests must install into isolated homes and repositories and
   must never write the operator's real configuration.
8. Acceptance tests must distinguish certified, unverified-model, degraded, and
   uncertified-host states.
9. Release checks must operate on the exact tarball submitted for publication.

## Roadmap

### Phase 0 — Core contract and host proof

- Confirm the Pi `0.83.0` extension seams used for activation, prompt injection,
  diagnostics, process launch, and tool registration.
- Define Core states and Diagnostic Schema version `1`.
- Prove the packed artifact can load in an isolated global Pi installation.

Exit gate: the packed package loads on the exact target, reports certification,
and leaves Pi usable when a Core capability is degraded.

### Phase 1 — Delegation foundation

- Complete the owned child-process lifecycle.
- Add structured progress, identity validation, cancellation, limits, queuing,
  and grouped failure semantics.
- Implement Capability Contracts and Capability Preflight for delegated
  operations.
- Implement the five roles, guards, and Single Writer enforcement.
- Inject and verify Matty Rules in parent and child contexts.

Exit gate: real-process tests demonstrate every role, allow/deny policy,
concurrency bound, cancellation path, and no orphaned child.

### Phase 2 — Web and diagnostics

- Integrate the exact pinned web dependency.
- Expose only the certified tools to the parent and researcher.
- Enforce required, optional, and absent web policy through Capability
  Contracts.
- Implement status, doctor, JSON output, redaction, and remediation.
- Prove startup and diagnostics make no network request.

Exit gate: web research returns current cited results on the Reference Model
Path, and all unavailable/degraded cases remain explicit and safely diagnosable.

### Phase 3 — Artifact and release

- Build and inspect the precompiled package.
- Audit dependency lifecycle scripts and licenses.
- Run the complete exact-host suite against the final tarball.
- Perform bootstrap publication if required, then configure stage-only OIDC
  trusted publishing and remove token publication.

Exit gate: the public artifact has provenance, matches the accepted digest, is
install-safe, and routine publication has no long-lived npm write token.

### Later, outside the MVP

Possible workflow distribution, repository preparation, or richer capability
contracts require separate product evidence and ADRs. They are not implied by
Matty Core `0.1`.

## Success criteria

Matty Core `0.1` succeeds when:

1. The packed artifact activates on Pi `0.83.0` on macOS Apple Silicon.
2. A user can run all five roles through real child Pi processes and receive
   structured, correctly terminated results.
3. Limits of eight accepted tasks, four active children, and one repository
   writer are enforced and observable.
4. Capability Preflight enforces required and optional behavior without silent
   inline or model-knowledge substitution.
5. Guards block their documented mutation classes in acceptance tests without
   being described as a sandbox.
6. Matty Rules appear exactly once in parent and child contexts and no external
   instruction file is written.
7. The parent and researcher can produce a current cited web result through the
   pinned integration; other roles cannot access those tools.
8. Status and doctor accurately report active, degraded, unverified-model, and
   uncertified-host states in human and schema-valid JSON forms.
9. Redaction tests prove forbidden data cannot enter diagnostics.
10. Startup, status, and doctor pass with network access denied.
11. Installation from the final tarball runs no Matty-owned lifecycle script
    and does not mutate real user or project configuration.
12. The public package carries provenance from the canonical workflow, and
    routine publishing uses OIDC without a long-lived write token.

## Risks

1. **Pi extension seams change.** The owned runtime and exact-version
   certification limit the claim, but upgrading Pi requires the complete suite.
2. **Best-effort guards are bypassable.** Clear language, least-privilege tool
   sets, isolated tests, and main-agent ownership of external mutations reduce
   accidents but do not create a hostile-code boundary.
3. **Child processes leak or outlive cancellation.** Identity checks, ordered
   shutdown, timeouts, and real-process cleanup tests are release-critical.
4. **Provider behavior changes behind the pinned web integration.** Observable
   provider selection and explicit failure prevent silent claims of research,
   but cannot freeze an external service.
5. **Diagnostics expose sensitive data.** A closed allowlist and adversarial
   tests are required; raw provider or process objects must never be rendered.
6. **Dependency lifecycle behavior changes.** Exact artifact inspection and a
   blocking production-tree inventory are required for every release.
7. **Bootstrap credentials persist.** The initial publication must be audited,
   and OIDC configuration, token revocation, and secret removal are part of the
   same release record.
8. **Core scope grows back into workflow ownership.** Catalogs, workflow
   routers, repository setup, and per-workflow contracts require a new product
   decision and cannot enter `0.1` as incidental implementation.
