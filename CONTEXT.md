# Matty

Matty is an opinionated Pi package for dependable day-to-day software
development. Matty Core provides the runtime foundation, while later
Matty-owned workflows provide maintained development experiences; version
`0.1` ships only Core.

## Language

**Matty**:
The product that combines Matty Core with a selected set of opinionated,
Matty-owned development workflows.
_Avoid_: Runtime only, general workflow marketplace

**Matt Skill**:
A skill sourced from Matt Pocock's upstream collection and distributed as a
traceable, reviewed snapshot without competing extension-owned activation.
_Avoid_: Matty-owned skill, floating upstream skill

**Matty Skill Pack**:
The Packy-managed projection that installs one authoritative global copy of the
curated skills for Pi and other supported agent surfaces.
_Avoid_: Extension-bundled catalog, competing skill copy

**Supported Skill Set**:
The versioned subset of the Matty Skill Pack whose identity, provenance,
content, and availability Matty validates as workflow dependencies.
_Avoid_: Certified model behavior, every installed skill, implicit support

**Experimental Skill**:
A skill distributed for evaluation without inheriting the compatibility or
outcome guarantees of the Supported Skill Set.
_Avoid_: Supported workflow dependency, silently supported skill

**Matty Setup**:
A visible, user-directed operation that validates the Matty Skill Pack and a
Prepared Repository and may guide explicitly approved remediation.
_Avoid_: Startup installation, background update

**Matty Workflow**:
A versioned engineering procedure that composes unchanged skills with Matty
control while Matty maintains the resulting experience and outcome quality.
_Avoid_: Agent Skill, modified Matt Skill, unowned third-party recipe

**Workflow Definition**:
The versioned package-owned pairing of Workflow Guidance with enforceable
workflow policy, with no fact assigned authoritatively to both.
_Avoid_: Globally activated skill, duplicated prompt-and-code specification

**Workflow Guidance**:
The non-authoritative reasoning guidance that tells an agent how to clarify,
apply unchanged Matt Skills, and recognize decisions requiring a human.
_Avoid_: Effect authorization, gate state, executable policy

**Workflow Controller**:
The Matty authority that advances a workflow only when its authorization,
evidence, policy, and effect preconditions are satisfied.
_Avoid_: Prompt interpreter, general Agent Skill runtime

**Workflow Dependency**:
A Supported Skill Set member that a named Matty Workflow requires at a specific
gate; other relevant skills remain optional aids selected by the agent.
_Avoid_: Every available skill, implicit dependency

**Supported Workflow**:
A Matty Workflow that has passed its scenario suite and audited real deliveries
on the exact certified capability set.
_Avoid_: Demonstrated workflow, prompt that worked once

**Issue Delivery**:
The Matty Workflow that takes one explicitly authorized ready issue through
implementation, protected integration, issue closure, and owned-resource cleanup.
_Avoid_: Issue-to-PR, PR preparation, autonomous release

**Delivery Authorization**:
The combination of an exact issue, explicit delivery intent, ready triage state,
and trusted repository that permits Issue Delivery to mutate Git and tracker state.
_Avoid_: Implicit task inference, blanket repository autonomy

**Exception Brief**:
A structured blocked outcome that states the active gate, evidence, decision or
capability needed, available options, and Matty's recommendation.
_Avoid_: Generic failure, guessed fallback, free-form steering request

**Delivery Identity**:
The stable combination of canonical repository, tracker, and issue that binds
one Issue Delivery to its verifiable branches, PRs, comments, and artifacts.
_Avoid_: Pi session, branch name, worktree path

**Current Work Adoption**:
The explicit human decision that binds existing commits or uncommitted changes
to a Delivery Identity before Matty treats them as an unverified candidate.
_Avoid_: Automatic scope inference, inherited verification

**Prepared Repository**:
A repository whose consumer-owned agent instructions, tracker policy, domain
documentation policy, and canonical triage mapping were materialized through
the supported setup workflow and remain inspectable by its maintainers.
_Avoid_: Matty-owned repository configuration, inferred repository policy

**Matty User**:
An experienced Pi user who wants dependable daily development workflows across
trusted repositories and accepts explicit control and verification steps.
_Avoid_: General developer, beginner developer

**Dependable Outcome**:
A development result accompanied by inspectable changes, executed verification,
remaining risks, and explicit requests for any required human decision.
_Avoid_: Plausible completion, autonomous completion, fastest completion

**Manual Validation**:
An evidence-producing check of public behavior performed by the agent unless
repository policy, access, or risk requires a human to perform or approve it.
_Avoid_: Mandatory approval for every delivery, unrecorded spot check

**Repair Budget**:
The fixed number of candidate repair cycles permitted at one workflow gate
before Issue Delivery blocks with an Exception Brief.
_Avoid_: Unlimited retries, silent retry loop

**Matty Core**:
The versioned runtime capabilities shipped by Matty: the Subagent Runtime,
Matty Roles and guards, Matty Rules, Web Capability, and diagnostics.
_Avoid_: Workflow catalog, skill distribution

**Supported Installation**:
A Matty package installed globally through Pi. Project-local installation is
not certified for Matty `0.1`.
_Avoid_: Global Matty CLI, supported project-local installation

**Subagent Runtime**:
The Matty-owned capability for bounded delegated execution with structured
progress and terminal outcomes.
_Avoid_: In-process agent simulation, stable Pi subagent API

**Matty Role**:
One of five least-privilege child profiles: `explorer`, `reviewer`, `designer`,
`researcher`, or `worker`.
_Avoid_: General-purpose agent, unrestricted subagent

**Inspection Guard**:
A best-effort command policy for `explorer`, `designer`, and `reviewer` that
permits inspection while blocking recognized mutation. It is not a security
sandbox.
_Avoid_: Read-only shell, role sandbox

**Worker Guard**:
A best-effort command and path policy that confines a `worker` to the trusted
working tree and validated temporary paths while blocking external-state,
Git-history, global-installation, and user-configuration mutation. It is not a
security sandbox.
_Avoid_: Worker sandbox, unrestricted worker

**Single Writer**:
The invariant that permits at most one active `worker` per repository while
non-writing roles may run concurrently.
_Avoid_: Parallel workers, shared-worktree writers

**Bounded Concurrency**:
The limit of eight tasks accepted by one delegation call and four child Pi
processes active at once.
_Avoid_: Unlimited delegation, eight-way parallelism

**Capability Contract**:
Versioned Matty-owned policy for one Core operation, covering its role, tools,
write authority, web requirement, cardinality, concurrency, and failure behavior.
_Avoid_: Skill contract, workflow hint

**Capability Preflight**:
The evaluation of a Capability Contract before its operation produces effects.
An unmet required capability blocks only that operation.
_Avoid_: Startup capability gate, global Core disablement

**Matty Rules**:
Versioned runtime and workflow invariants applied consistently to parent and
child agents without becoming project-owned policy.
_Avoid_: Matty configuration file, project rules patch

**Web Capability**:
The supported web research surface available to the parent and `researcher`,
with credentials and configuration remaining externally owned.
_Avoid_: Matty web engine, general Matty MCP surface

**Certified Pi Version**:
The exact Pi version that has passed Matty's complete packed-artifact acceptance
suite. Matty `0.1` initially certifies Pi `0.83.0`.
_Avoid_: Compatible Pi range, presumed-compatible version

**Certified Target**:
The exact operating-system and architecture combination that has passed the
same acceptance suite. Matty `0.1` initially certifies macOS on Apple Silicon.
_Avoid_: Supported platform family, presumed-compatible platform

**Reference Model Path**:
The provider, model, and authentication combination exercised by the complete
acceptance suite. It is evidence for that path, not a model allowlist.
_Avoid_: Required Matty model, certified-only model

**Exact Host Certification**:
The release rule that only one named Pi version and Certified Target are
supported until another exact combination passes the complete suite.
_Avoid_: Best-effort compatibility, semver compatibility claim

**Redacted Diagnostic**:
A status, doctor, or error result built from a closed allowlist of safe fields.
All other fields are omitted.
_Avoid_: Sanitized raw error, best-effort redaction

**Diagnostic Schema**:
The versioned machine-readable representation shared by
`/matty status --json` and `/matty doctor --json`, beginning at schema version
`1`.
_Avoid_: Parsed human output, unversioned JSON

**Degraded State**:
A condition in which Matty remains diagnosable but one or more Core
capabilities are unavailable or the host is uncertified.
_Avoid_: Silent fallback, partially certified host

**Install-Safe Artifact**:
A prebuilt npm artifact with no Matty-owned install lifecycle scripts and a
reviewed inventory of lifecycle scripts in its production dependency tree.
_Avoid_: Install-time build, implicit postinstall

**Zero Telemetry**:
The rule that Matty emits no analytics, usage metrics, crash reports, update
probes, or background network requests.
_Avoid_: Anonymous telemetry, silent update check

**User-Directed Network Operation**:
A network request attributable to a visible user or workflow action, including
Pi model calls, delegated execution, web research, reviewer GitHub inspection,
or project-local dependency installation.
_Avoid_: Background Matty request

**Public Distribution**:
The public repository `https://github.com/yersonargotev/matty` and public npm
package `@yargote/matty`, published with provenance.
_Avoid_: Private registry, unverifiable artifact

**Staged Release**:
A post-bootstrap release submitted by the canonical GitHub Actions workflow
through npm trusted publishing, inspected as an exact staged artifact, and made
public only after explicit maintainer approval.
_Avoid_: Direct CI publication, automatic approval

**Bootstrap Publication**:
The one-time publication needed to create the npm package before configuring
its trusted publisher, using a minimal short-lived token that is immediately
revoked.
_Avoid_: Routine token publication, placeholder release

**OIDC Trusted Publishing**:
The tokenless identity relationship between the canonical GitHub Actions
workflow and npm used for routine releases with provenance.
_Avoid_: Long-lived npm token, local maintainer publication

**Matty Source License**:
The MIT license applied to Matty-owned source, separate from retained
third-party licenses and notices.
_Avoid_: Upstream endorsement, combined copyright
