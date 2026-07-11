# Context

## Glossary

### Matty
A lightweight AI workflow toolkit inspired by Gentle AI, but intentionally centred on Matt Pocock-style skills, Engram memory, and explicit subagent delegation instead of SDD-first orchestration.

### Matty core
The always-available installer/configurator that manages capability packs and their lifecycle. Matty core is distinct from the optional `matty` capability pack, so deactivating that pack never disables the tool needed to manage it.

### CLI surface
An AI coding CLI that Matty can configure or integrate with. The initial supported CLI surfaces are Codex and OpenCode; Claude Code, Antigravity, and GitHub Copilot CLI are future candidates.

### Skill bundle
The curated set of agent skills Matty installs or exposes for a workflow. The current candidate bundle is based on Matt Pocock's engineering skills rather than Gentle AI's SDD stack.

### Capability pack
A named, composable set of AI workflow capabilities that can remain available while being activated or deactivated as a unit. A capability pack may contribute skills, memory, tools, agents, rules, or other host-supported behavior; it is not a runtime configuration profile.

### Pack resource
One host-independent intent contributed by a capability pack. A CLI-surface adapter may realize one pack resource as multiple host-specific artifacts; host-native schemas, paths, and package formats are projections rather than pack resources.

### Pack requirement
A global prerequisite a capability pack consumes but does not contribute to a CLI surface. External tools such as the Engram executable are requirements; platform-specific acquisition remains Matty core behavior rather than part of the portable pack manifest.

### Lifecycle resource
A pack resource that declares behavior triggered at CLI lifecycle points. It names the portable intent while each CLI-surface adapter owns its event names, handlers, trust model, and rendered artifacts; it is not a universal hook schema.

### Pack activation
The user's explicit consent to a previewed reconciliation of one capability pack on one CLI surface. Activation does not itself grant host trust, authenticate accounts, authorize executable code, or consent to destructive cleanup.

### Pack readiness
The progression from **configured** (Matty-owned projections are reconciled), through **authorized** (required human trust and authentication are complete), to **usable** (the host has loaded the capability under its runtime permissions). An active pack may remain pending human action between these stages.

### Pack desired state
The complete logical outcome Matty computes from the active capability packs on each CLI surface, including required shared resources and readiness, before translating that outcome into host-specific artifacts.

### Pack ownership
Matty's recorded authority over a projected resource or config fragment. Ownership determines whether Matty may update or remove it and is distinct from the host's trust, authentication, and runtime authorization.

### Reconciliation plan
An immutable preview of the exact ordered changes needed to move one approved pack operation from freshly observed state toward pack desired state. Its identity covers the activation intent revision, relied-on observations, actions, and human-consent phases; changed inputs require a new plan and approval.

### Reconciliation attempt
One application of an approved reconciliation plan. An attempt ends verified when its outcome matches desired state, stale when its preconditions changed before any action, or recovery-required when completed and remaining effects must be reconciled from a fresh observation.

### Memory layer
The Engram-backed persistence and recall behaviour Matty provides across supported CLI surfaces.

### Delegation layer
The subagent orchestration behaviour Matty exposes, including read-only exploration and bounded implementation workers where the host CLI supports them.

### Installer/configurator
Matty's v0 product shape: a tool that installs and configures Codex/OpenCode with the right skills, Engram memory hooks, and delegation conventions, rather than an active runtime orchestrator present in every agent session.

### Golden path
Matty v0's primary success path: given an existing repository, configure Codex and OpenCode with Matt Pocock-style skills, Engram memory, and delegation conventions while keeping the initial prompt minimal.

### Global-first configuration
Matty's default configuration model: install skills in `~/.agents/skills`, manage agent/system-prompt configuration in each CLI's global home/config surface, and avoid writing project-local files unless the user explicitly opts into project docs.

### Matty state file
A small global Matty-owned config/state file, expected at `~/.matty/config.json`, used to track installed Matty version, managed skill set, global skill paths, configured CLI surfaces, and doctor/update metadata. It must not become a large prompt store.
