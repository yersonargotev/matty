Type: research
Status: resolved

## Question

What can the current official Codex and OpenCode extension/configuration surfaces represent, install, enable, disable, update, and remove for skills, plugins, hooks, MCPs, agents, rules, and prompts, and which gaps require Matty-owned adapters?

## Answer

[Host capability surfaces: Codex and OpenCode](../host-capability-surfaces.md) records the verified matrix and lifecycle gaps.

The hosts share a practical user-skill projection at `~/.agents/skills` and a common logical MCP model, but their plugin, hook, agent, rule, prompt, enablement, trust, update, and removal contracts are not interchangeable. Capability packs therefore need a Matty-owned resource vocabulary with per-surface adapters, explicit ownership/reconciliation state, and restart/trust/auth reporting; neither host plugin format can serve as the universal pack manifest.
