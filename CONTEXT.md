# Context

## Glossary

### Packy
A lightweight AI workflow toolkit inspired by Gentle AI, but intentionally centred on Matt Pocock-style skills, Engram memory, and explicit subagent delegation instead of SDD-first orchestration.

### Packy core
The always-available installer/configurator that manages capability packs and their lifecycle. Packy core is distinct from the optional `matty` capability pack, so deactivating that pack never disables the tool needed to manage it.

### Packy core lifecycle
The install, update, and uninstall behavior that reconciles Packy-managed global workflow artifacts. It excludes Installed Source initialization, setup health diagnosis, and capability-pack lifecycle operations.

### CLI surface
An AI coding CLI that Packy can configure or integrate with. The initial supported CLI surfaces are Codex and OpenCode; Claude Code, Antigravity, and GitHub Copilot CLI are future candidates.

### Skill bundle
The curated set of agent skills Packy installs or exposes for a workflow. The current candidate bundle is based on Matt Pocock's engineering skills rather than Gentle AI's SDD stack.

### Skill source
The single resolved source from which Packy reads its skill bundle. Its origin may be an explicit operator override, the current Packy repository checkout, or the Installed Source, but every consumer uses the same selection.

### Installed Source
The user-owned Packy checkout initialized for package-installed operation. It is one candidate for Skill Source selection and remains distinct from the selected Skill Source itself.

### Capability pack
A named, composable set of AI workflow capabilities that can remain available while being activated or deactivated as a unit. A capability pack may contribute skills, memory, tools, agents, rules, or other host-supported behavior; it is not a runtime configuration profile.

### Selectable pack catalog
The Packy-owned set of current capability-pack versions advertised by `packy pack list` and available for fresh activation or update. Bundled or historical pack artifacts are not selectable merely because their bytes exist in the distribution.

### Pack resource
One host-independent intent contributed by a capability pack. A CLI-surface adapter may realize one pack resource as multiple host-specific artifacts; host-native schemas, paths, and package formats are projections rather than pack resources.

### Pack requirement
A global prerequisite a capability pack consumes but does not contribute to a CLI surface. External tools such as the Engram executable are requirements; platform-specific acquisition remains Packy core behavior rather than part of the portable pack manifest.

### Lifecycle resource
A pack resource that declares behavior triggered at CLI lifecycle points. It names the portable intent while each CLI-surface adapter owns its event names, handlers, trust model, and rendered artifacts; it is not a universal hook schema.

### Pack activation
The user's explicit consent to a previewed reconciliation of one capability pack on one CLI surface. Activation does not itself grant host trust, authenticate accounts, authorize executable code, or consent to destructive cleanup.

### Pack readiness
The progression from **configured** (Packy-owned projections are reconciled), through **authorized** (required human trust and authentication are complete), to **usable** (the host has loaded the capability under its runtime permissions). An active pack may remain pending human action between these stages.

### Pack desired state
The complete logical outcome Packy computes from the active capability packs on each CLI surface, including required shared resources and readiness, before translating that outcome into host-specific artifacts.

### Pack ownership
Packy's recorded authority over a projected resource or config fragment. Ownership determines whether Packy may update or remove it and is distinct from the host's trust, authentication, and runtime authorization.

### Pack observable contract
The complete user-visible behavior of a capability pack, including its skill content, declared resources, requirements, capabilities, and activation or update experience. A pack version describes this contract rather than its upstream source version or textual diff size.

### Pack compatibility
Whether a newer pack observable contract preserves the workflows and expectations of an active older version without an incompatible migration or newly mandatory user action.

### Decision-ready synchronization proposal
A source update whose exact identity, provenance, content changes, pack compatibility evidence, migrations, and validations are complete and unchanged, leaving human acceptance as the only remaining decision.

### Reconciliation plan
An immutable preview of the exact ordered changes needed to move one approved pack operation from freshly observed state toward pack desired state. Its identity covers the activation intent revision, relied-on observations, actions, and human-consent phases; changed inputs require a new plan and approval.

### Reconciliation attempt
One application of an approved reconciliation plan. An attempt ends verified when its outcome matches desired state, stale when its preconditions changed before any action, or recovery-required when completed and remaining effects must be reconciled from a fresh observation.

### Memory layer
The Engram-backed persistence and recall behaviour Packy provides across supported CLI surfaces.

### Delegation layer
The subagent orchestration behaviour Packy exposes, including read-only exploration and bounded implementation workers where the host CLI supports them.

### Governance expected-state contract
The versioned, reviewable description of Packy's accepted repository and
publication controls and the promotion or publication boundaries they protect.

### Governance observation
One read-only, sanitized projection of the effective controls bound to a
repository, protected ref, commit, workflow definition, and UTC collection
time.

### Governance drift
A confirmed, unclassifiable, missing, failed, or stale disagreement between the
governance expected-state contract and a governance observation.

### Installer/configurator
Packy's v0 product shape: a tool that installs and configures Codex/OpenCode with the right skills, Engram memory hooks, and delegation conventions, rather than an active runtime orchestrator present in every agent session.

### Golden path
Packy v0's primary success path: given an existing repository, configure Codex and OpenCode with Matt Pocock-style skills, Engram memory, and delegation conventions while keeping the initial prompt minimal.

### Global-first configuration
Packy's default configuration model: install skills in `~/.agents/skills`, manage agent/system-prompt configuration in each CLI's global home/config surface, and avoid writing project-local files unless the user explicitly opts into project docs.

### Packy state file
A small global Packy-owned config/state file, expected at `~/.packy/config.json`, used to track installed Packy version, managed skill set, global skill paths, configured CLI surfaces, and doctor/update metadata. It must not become a large prompt store.

### Packy Home
The single workstation root reserved for Packy-owned state. Domains may own separate files beneath it without sharing ownership of those files.

### Workstation snapshot
The immutable, normalized view of ambient workstation facts used by one Packy command invocation. It is created only when an operation needs workstation access, and every participant in that operation observes the same snapshot.
