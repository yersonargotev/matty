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
