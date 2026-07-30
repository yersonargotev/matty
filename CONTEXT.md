# Matty

Matty packages an opinionated engineering workflow for Pi users who want a
consistent development process across trusted repositories.

## Language

**Matty User**:
An experienced Pi user who adopts the Matty engineering workflow across trusted
repositories and configures optional providers when needed. Prior familiarity
with the workflow is helpful but not required.
_Avoid_: General developer, beginner developer

**Supported Installation**:
A Matty package installed globally through Pi and therefore available across
the user's repositories. A project-local Pi package installation may work
under Pi's trust model but is not certified or supported by Matty v1.
_Avoid_: Global Matty CLI, supported project-local installation

**Install-Safe Artifact**:
A prebuilt Matty npm artifact with no package-owned install lifecycle scripts
and a reviewed inventory of lifecycle scripts in its dependency tree.
Unreviewed dependency scripts block publication.
_Avoid_: Install-time build, implicit postinstall

**Public Distribution**:
The public source repository
`https://github.com/yersonargotev/matty` and public scoped npm package
`@yargote/matty`, published from cloud CI with verifiable npm provenance.
Normal publication uses stage-only OIDC trusted publishing rather than a
long-lived token or direct CI publication.
_Avoid_: Private Matty registry, token-based routine publishing

**Staged Release**:
A post-bootstrap release submitted by the canonical OIDC workflow through
`npm stage publish`, inspected as the exact staged tarball, and made public only
after explicit maintainer approval with 2FA.
_Avoid_: Direct CI publish, automatic release approval

**Bootstrap Publication**:
The one-time creation of `@yargote/matty@0.1.0` from the canonical GitHub
Actions workflow using a minimally scoped, shortest-lived granular npm token
and explicit provenance. The token is then revoked and token publishing
disabled after OIDC trusted publishing is configured.
_Avoid_: Placeholder package version, routine token publication

**Package Contract Data**:
Immutable Matty Rules, compatibility manifests, role definitions, and workflow
contracts shipped inside a particular Matty package version. They are product
contents, not user configuration or persistent runtime state.
_Avoid_: Matty config, generated user manifest

**User-Directed Network Operation**:
A network request attributable to a visible user or workflow action, such as a
Pi model call, delegated execution, Web Capability use, reviewer GitHub
inspection, project-local dependency installation, or Pi-owned package
lifecycle action. Matty emits no unrelated telemetry or probes.
_Avoid_: Background Matty request, silent update check

**Matty Engineering Workflow**:
The opinionated, Matty-curated development process shipped as a fixed part of
each Matty release. It may draw from Matt Pocock's public work but is neither an
official implementation nor a promise of continuing exact fidelity to it.
_Avoid_: Matt Pocock workflow, official Matt Pocock workflow

**Matty Source License**:
The MIT license applied to Matty-owned source. It is distinct from the retained
licenses and copyright notices for imported or bundled third-party material.
Its notice is `Copyright (c) 2026 Yerson Argote`.
_Avoid_: Combined copyright notice, upstream endorsement

**Shared Skill Catalog**:
The complete, inseparable set of Matty-owned skills included in a particular
Matty release. Projects may extend it with non-colliding skills but users cannot
replace, enable, or disable its individual members.
_Avoid_: Skill profile, user skill selection, optional shared skills

**Upstream Skill Set**:
The complete contents of `skills/engineering` and `skills/productivity` from
the pinned `mattpocock/skills` commit selected for a deliberate Skill Import.
_Avoid_: Hand-picked Matt skills, latest Matt skills

**Skill Import**:
A maintainer-initiated event that copies the Upstream Skill Set into Matty.
Imported files become Matty-owned source and may diverge until another deliberate
import or update is chosen.
_Avoid_: Upstream sync, automatic update

**Upstream Reconciliation**:
A maintainer-initiated, skill-by-skill review of new upstream changes against
Matty's current source. Matty changes are preserved unless the maintainer
deliberately replaces them.
_Avoid_: Upstream overwrite, automatic merge

**Repository Preparation**:
The explicit, once-per-repository configuration performed through
`setup-matt-pocock-skills` before using engineering flows that depend on project
policy. It is user-invoked and never runs during Pi startup.
_Avoid_: Matty setup, automatic bootstrap

**Delegation Contract**:
Matty-owned metadata, separate from imported skill files, that states a skill
or scenario's delegation requirement, cardinality, parallelism, independence,
role, and unavailable-capability behavior.
_Avoid_: Delegation hint, embedded skill patch

**Delegation Preflight**:
The runtime evaluation of the applicable Delegation Contract before a skill or
delegation-dependent branch produces effects. It blocks only the affected path
when required capabilities are unavailable.
_Avoid_: Startup delegation check, global workflow disablement

**Subagent Runtime**:
The Matty-owned, Pi-process-based delegation implementation imported initially
from Pi `0.83.0`'s official subagent example and adapted to enforce Delegation
Contracts and Matty role policy.
_Avoid_: Pi subagent API, in-process agent simulation

**Matty Role**:
One of five least-privilege subagent profiles: `explorer`, `reviewer`,
`designer`, `researcher`, or `worker`. Ambiguous upstream role names are mapped
to a concrete Matty Role by the applicable Delegation Contract. Only
`researcher` receives the certified Web Capability tools.
_Avoid_: General-purpose agent, unrestricted subagent

**Inspection Guard**:
A best-effort, role-aware command policy that allows `explorer`, `designer`,
and `reviewer` roles to use `bash` for inspection while blocking recognized
mutating command families. Only `reviewer` may inspect remote GitHub state
through `gh`. It reduces accidents but is not a security sandbox.
_Avoid_: Explorer sandbox, designer sandbox, reviewer sandbox, read-only shell

**Worker Guard**:
A best-effort command and path policy that confines a `worker` to working-tree
and validated temporary writes while blocking `gh`, Git index or reference
mutation, global installation, and user-configuration writes. It is not a
security sandbox.
_Avoid_: Worker sandbox, unrestricted worker

**Single Writer**:
The v1 delegation invariant that permits at most one active `worker` for a
repository while allowing non-writing Matty Roles to run concurrently.
_Avoid_: Parallel workers, shared-worktree writers

**Bounded Concurrency**:
The v1 Subagent Runtime limit of eight tasks accepted per call and four child
Pi processes active at once. Queued tasks are not described as simultaneously
parallel.
_Avoid_: Unlimited delegation, eight-way parallelism

**Delegation Group**:
The atomic set of child executions required together by one Delegation
Contract. If any required member fails, the group produces no successful
workflow result.
_Avoid_: Partial parallel result, degraded required delegation

**Redacted Diagnostic**:
A status, doctor, or error result built from an explicit allowlist of safe
fields. Any field not allowed by the contract is omitted by default.
_Avoid_: Sanitized raw error, best-effort redaction

**Diagnostic Schema**:
The versioned machine-readable representation shared by `/matty status --json`
and `/matty doctor --json`. It is rendered from the same Redacted Diagnostic
snapshot as human output and begins at schema version `1`.
_Avoid_: Parsed human output, unversioned diagnostic JSON

**Startup Hint**:
The single informational line shown after successful Matty activation:
`Matty active · /skill:ask-matt · /matty status`.
_Avoid_: Welcome wizard, startup warning

**Research Workspace**:
A per-run directory under the operating system's temporary root where a
`researcher` may retain notes and source artifacts while investigating.
_Avoid_: Project research directory, durable research archive

**Research Report**:
The single cited Markdown file a `researcher` persists to a prevalidated
repository path, using the existing project convention or `docs/research` when
none exists.
_Avoid_: Research scratch, raw source dump

**Matty Rules**:
Versioned compatibility and workflow rules bundled by Matty and injected into
each parent and child system prompt between `matty:rules` markers. They adapt
host-specific terminology and govern Matty workflow invariants without
modifying imported skills or external `AGENTS.md` files.
_Avoid_: Matty AGENTS.md, project rules patch

**Web Capability**:
The `pi-web-access` integration bundled by Matty so workflows can search the
web and inspect sources. Its provider credentials, configuration, and stored
state remain owned by Pi and `pi-web-access`; its internal Exa MCP fallback is
not a general Matty MCP capability. Matty v1 certifies only `web_search`,
`source_check`, `fetch_content`, and `get_search_content`. These tools are
available to the parent agent and the `researcher` role only.
_Avoid_: Matty web engine, built-in Pi web

**Web Contract**:
Matty-owned metadata, separate from imported skill files, that classifies a
skill or branch's Web Capability requirement as `required`, `optional`, or
`none`, including its unavailable-capability behavior.
_Avoid_: Web hint, implicit research requirement

**Web Preflight**:
The runtime evaluation of the applicable Web Contract before a classified
skill or branch produces effects. An unmet `required` contract blocks only that
path; an unmet `optional` contract permits execution with explicit disclosure.
_Avoid_: Startup web requirement, silent model-knowledge fallback

**Certified Pi Version**:
An exact Pi version that has passed Matty's complete packed-package acceptance
suite for a Certified Target. Matty v1 initially certifies Pi `0.83.0`.
_Avoid_: Compatible Pi range, presumed-compatible Pi version

**Certified Target**:
An exact operating-system and architecture combination that has passed Matty's
complete packed-package acceptance suite. Matty v1 initially certifies macOS on
Apple Silicon.
_Avoid_: Supported platform family, presumed-compatible platform

**Reference Model Path**:
The provider, model, and authentication combination exercised by Matty's full
acceptance suite. Matty v1 uses GPT-5.6 through ChatGPT/Codex subscription
authentication. It is evidence for that path, not a global model allowlist.
_Avoid_: Required Matty model, certified-only model

**Shared Skill Collision**:
A conflict in which a project, global, or third-party skill claims a name
reserved by the Shared Skill Catalog.
_Avoid_: Skill override, shadowed Matty skill

**Incomplete Shared Skill Catalog**:
A Matty package load from which Pi has excluded one or more expected shared
skills. It is never a valid active workflow configuration.
_Avoid_: Customized catalog, partial Matty profile

**Activation Safety Gate**:
The Phase 0 release condition proving that no external definition can reach the
model under a Matty-reserved skill name. Failure to prove it blocks release.
_Avoid_: Best-effort precedence, assumed load order
