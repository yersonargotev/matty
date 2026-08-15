# Matty

Matty Core is a small, opinionated runtime layer for Pi. Version `0.1` provides
bounded delegation, web research, runtime rules, diagnostics, and a
releaseable package without owning higher-level engineering processes.

## Language

**Matty User**:
An experienced Pi user who wants dependable delegated work across trusted
repositories.
_Avoid_: General developer, beginner developer

**Matty Core**:
The versioned runtime capabilities shipped by Matty: the Subagent Runtime,
Matty Roles and guards, Matty Rules, Web Capability, and diagnostics.
_Avoid_: Process catalog, skill distribution

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
_Avoid_: Skill contract, task hint

**Capability Preflight**:
The evaluation of a Capability Contract before its operation produces effects.
An unmet required capability blocks only that operation.
_Avoid_: Startup capability gate, global Core disablement

**Matty Rules**:
Versioned Core runtime invariants applied consistently to parent and child
agents without becoming project-owned policy.
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
A network request attributable to a visible user or delegated action, including
Pi model calls, delegated execution, web research, reviewer GitHub inspection,
or project-local dependency installation.
_Avoid_: Background Matty request

**Public Distribution**:
The public repository `https://github.com/yersonargotev/matty` and public npm
package `@yargote/matty`, published with provenance.
_Avoid_: Private registry, unverifiable artifact

**Staged Release**:
A post-bootstrap release submitted by the canonical GitHub Actions automation
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
release automation and npm used for routine releases with provenance.
_Avoid_: Long-lived npm token, local maintainer publication

**Matty Source License**:
The MIT license applied to Matty-owned source, separate from retained
third-party licenses and notices.
_Avoid_: Upstream endorsement, combined copyright
