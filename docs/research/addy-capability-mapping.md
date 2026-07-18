# Addy capability mapping: Packy, Codex, and OpenCode

## Research question

For every capability type in Addy `0.6.4`, what is its portable Pack intent,
what projection exists on Codex and OpenCode today, and does the remaining gap
require adapter work, Pack-schema evolution, declared degradation, or
exclusion?

This mapping is a planning decision, not an implementation or activation plan.
The exact upstream inventory and safety boundary live in the
[pinned inventory](./addy-upstream-inventory.md).

## Source basis

- Addy release `0.6.4`, commit
  [`98967c45a42b88d6b8fb3a88b7ff6273920763d6`](https://github.com/addyosmani/agent-skills/tree/98967c45a42b88d6b8fb3a88b7ff6273920763d6).
- Packy commit
  [`e066d1c3863f6da0398891d251ea785b6e05f9ec`](https://github.com/yersonargotev/packy/tree/e066d1c3863f6da0398891d251ea785b6e05f9ec).
- Current Codex product manual fetched on 2026-07-18: [skills](https://learn.chatgpt.com/docs/build-skills),
  [custom subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents),
  [lifecycle hooks](https://learn.chatgpt.com/docs/hooks),
  [plugin structure](https://learn.chatgpt.com/docs/build-plugins#plugin-structure),
  and [built-in slash commands](https://learn.chatgpt.com/docs/developer-commands?surface=cli).
- Current OpenCode documentation fetched on 2026-07-18: [skills](https://opencode.ai/docs/skills),
  [agents](https://opencode.ai/docs/agents),
  [commands](https://opencode.ai/docs/commands), and
  [plugins/hooks](https://opencode.ai/docs/plugins).

All upstream content remained inert. No upstream hook, script, installer, test,
or CI workflow was run.

## What Packy can express and project now

Pack schema v1 accepts only `skill`, `instruction`, `mcp_server`, and
`lifecycle` resources. `skill` and `instruction` each have one relative
`source`; `mcp_server` has a command and arguments; `lifecycle` has only an ID
([decoder](https://github.com/yersonargotev/packy/blob/e066d1c3863f6da0398891d251ea785b6e05f9ec/internal/capabilitypack/catalog.go#L314-L345)).

The complete host adapters do not expose that whole vocabulary generically:

- Codex projects skills as links, instructions as managed blocks in its prompt
  file, and MCP servers as managed config blocks
  ([adapter](https://github.com/yersonargotev/packy/blob/e066d1c3863f6da0398891d251ea785b6e05f9ec/internal/codex/surface.go#L60-L151)).
- OpenCode projects skills as links, instructions as separate files plus config
  references, and MCP servers as managed JSONC entries
  ([adapter](https://github.com/yersonargotev/packy/blob/e066d1c3863f6da0398891d251ea785b6e05f9ec/internal/opencode/surface.go#L54-L161)).
- Neither generic adapter dispatches `agent`, `command`, `hook`, shared asset,
  notice, or validation resource kinds. The accepted adapter boundary requires
  each host module to own host syntax and projection translation while
  capability-pack retains lifecycle and safety policy
  ([ADR 0005](../adr/0005-capability-pack-surface-adapter.md)).

Therefore, a host feature is **not** Packy-supported merely because Codex or
OpenCode can represent it natively.

## Capability-by-capability mapping

| Upstream capability | Portable Pack intent | Codex projection | OpenCode projection | Required disposition |
|---|---|---|---|---|
| 24 skill directories | `skill` | Native skill discovery; Packy already links skill trees | Native skill discovery; Packy already links skill trees | **Current schema + current adapters**, subject to dependency closure and trust metadata below |
| Skill-local files and `idea-refine.sh` | Files owned by the containing `skill`, not independent activation resources | Preserved by the linked skill tree; execution remains agent/tool-policy controlled | Same | **Adapter/source validation work** only; preserve modes and declare required tools, never execute during sync/activation |
| Seven shared references | Dependency assets of the skills that reference them | No Packy projection; a linked skill's single source tree does not own sibling `references/` | Same | **Pack-schema evolution** for dependency-closed assets or source sets; deterministic vendoring/rewrite is an alternative adapter transformation but must be explicit and validated |
| Four personas | `agent` with prompt, description, mode, and tool/permission intent | Codex now supports custom subagents, but Packy has no resource or projection | OpenCode supports Markdown/JSON agents, but Packy has no resource or projection | **Pack-schema evolution + both adapters**; host policy translation must be explicit rather than copying Claude frontmatter blindly |
| Eight logical commands | `command` with stable invocation, prompt template, orchestration target, and arguments | Codex exposes built-in slash commands, not arbitrary packaged slash-command files; a reusable workflow can be a skill | OpenCode has native global/project Markdown commands | **Pack-schema evolution + both adapters**; OpenCode can preserve `/name`, while Codex needs **declared degradation** to a same-named skill invocation such as `$build` |
| Claude-registered `SessionStart` hook | `hook` with event, matcher, handler, trust, tools, and effect metadata | Codex supports trust-gated `SessionStart` command hooks, but Addy's Codex manifest intentionally declares no hook and Packy cannot express/project one | OpenCode plugins expose lifecycle events, but Claude's hook protocol is not directly portable and Packy cannot project it | **Schema evolution + host adapters** would be required; do not activate it implicitly. The observable-contract decision must choose explicit opt-in or exclusion |
| Three unregistered opt-in hooks | Optional `hook` resources with filesystem/network/process effect declarations | Native hook machinery exists, but event/matcher/payload/exit semantics need translation and trust | Plugin events exist, but the shell hooks consume Claude-specific payloads and control semantics | **Schema evolution + adapter work + declared opt-in degradation**; exclude from default activation until each translation has safety and behavior evidence |
| Root `AGENTS.md` and host setup guides | Source-maintainer documentation, not consumer capability intent | Upstream explicitly says not to copy root `AGENTS.md` into consumer configuration | Same | **Exclude from runtime projection**; retain only as source evidence/documentation where useful |
| Host plugin/marketplace manifests and `.opencode/skills` symlink | Upstream distribution metadata and host projections, not canonical portable resources | Packy must derive its own projection from portable intent | Same | **Exclude from Pack runtime resources**; retain as mapping/provenance evidence |
| Eval JSON, validators, tests, CI, and test fixtures | Source-maintenance and validation evidence | No user-facing runtime projection | No user-facing runtime projection | **Exclude from activation**, but admit selected inert assets to the later validation matrix |
| MIT license and attribution | Pack/source notice and redistribution metadata | No current Pack notice projection | No current Pack notice projection | **Pack metadata/schema evolution** or another canonical bundle notice mechanism; redistribution must retain the notice |

## Semantic decisions established by the mapping

### 1. Skills are the only direct Addy runtime fit today

All 24 skill directories fit Packy's existing `skill` intent and both current
adapters. The skill directory is already a tree, so skill-local examples,
frameworks, and the helper script can remain part of that resource without a
new runtime kind. This does not authorize execution: acquired bytes remain
inert, and later activation/readiness policy must expose scripts and tool
requirements truthfully.

The shared `references/` directory is different. Addy treats it as common
progressive-disclosure input, but Packy's `skill.source` names only one tree.
Silently dropping those references would produce broken or semantically
incomplete skills. The portable model therefore needs either dependency assets
attached to a skill/source set or an explicit synchronization transform that
materializes and verifies a dependency-closed tree. The former is the cleaner
schema direction; the latter is not a no-op copy and requires compatibility
classification.

### 2. Personas are portable agents, not instructions

Flattening a persona into Packy's global `instruction` kind would erase its
separate invocation identity, role boundary, report contract, and tool policy.
Both target hosts now have a native agent concept, so the semantic gap is in
Packy, not in either host. Add `agent` intent and host translations; do not
encode one host's frontmatter as the portable contract.

### 3. Commands need one intentional asymmetric projection

OpenCode can preserve Addy's `/build`, `/plan`, `/ship`, and other command
names using native custom commands. Codex's documented slash command namespace
is product-controlled; Codex's reusable workflow surface is a skill. The Codex
projection must therefore be declared as degraded, with the logical name and
behavior preserved but invocation changed from `/name` to `$name` (or explicit
skill selection). It must not pretend `/name` was installed.

Command host files are projections, not separate capabilities. Packy should
model eight logical commands and render each target, rather than package all 24
Claude/Gemini/Antigravity files as independent resources.

### 4. Hooks require a real safety-bearing contract

Packy's existing `lifecycle` kind is not a generic hook model: it carries no
event, matcher, handler, payload protocol, trust requirement, tool dependency,
or effect declaration, and the generic adapter switches do not project it.
Reusing that kind for Addy hooks would create false support.

Codex and OpenCode both have extensibility points, but Addy's shell handlers
were written for Claude event payloads and exit behavior. A portable `hook`
contract must make protocol translation and effects observable and preserve
each host's trust/permission step. Until then, no hook may be silently installed
or simulated. The registered router hook is also not required to make the 24
skills discoverable on either target host, so excluding or separately opting
into it can remain coherent.

### 5. Source-only material stays outside activation

Maintainer guidance, host manifests, evals, validators, tests, CI, and upstream
projection symlinks help establish provenance and validation, but are not
consumer runtime capabilities. Their exclusion from activation does not narrow
the user-facing Addy system. License/notice bytes are the exception: they must
remain in the redistributed bundle even though they are not activated.

## Gap summary

### Adapter work without a new portable kind

- Validate and preserve complete skill trees, file modes, and inert helper
  assets.
- Resolve or materialize skill dependency closure if the schema chooses an
  explicit source transformation instead of dependency assets.
- Observe host loading/readiness rather than equating filesystem presence with
  usability.

### Pack-schema evolution

- `agent` resources.
- `command` resources with per-surface invocation/projection declarations.
- Safety-bearing `hook` resources.
- Dependency assets/source sets for shared references.
- Canonical license/notice metadata or bundle notice ownership.

This evolution affects both the runtime manifest contract and Pack Source
bindings/locks: synchronization currently binds resources by `kind`,
`resource_id`, and one `upstream_path`, so new kinds and multi-path closure
cannot be added only inside a surface adapter
([source configuration](https://github.com/yersonargotev/packy/blob/e066d1c3863f6da0398891d251ea785b6e05f9ec/bundle/sources.json)).
Any published Pack Source schema change must follow the immutable-suite rules
in [ADR 0011](../adr/0011-publish-versioned-pack-source-schema-suite.md).

### Declared degradation

- Codex command invocation: `/name` becomes a same-named skill invocation;
  behavior and identity remain visible, but the invocation difference is
  explicit.
- Hooks, if included later, are opt-in and may be unavailable on a surface
  until an evidence-backed protocol adapter exists. Unavailable hooks must be
  reported, not omitted.

### Exclusion

- Root maintainer `AGENTS.md` and host setup documentation from consumer config.
- Upstream host manifests, marketplace descriptors, and projection symlinks as
  runtime resources.
- Evals, validators, tests, CI, and fixtures from activation (while retaining
  selected items as inert validation evidence).
- Any hook for which the observable contract, trust step, or protocol
  translation remains unresolved.

## Answer

Packy's current portable vocabulary is **not sufficient** for the complete Addy
capability system. It can safely represent and project the 24 skills on both
surfaces today, but even those require dependency-closure handling for shared
references. Complete, truthful support requires new portable intents for
agents, commands, hooks, dependency assets, and redistribution notices, plus
corresponding complete-adapter projections.

OpenCode can preserve skills, personas, and slash commands natively. Codex can
preserve skills and personas natively, but Addy's logical commands must degrade
to skills because arbitrary custom slash commands are not a documented Codex
extension surface. Hooks are technically representable by both hosts only
after host-specific protocol and trust translation; none should activate by
default. Maintainer and validation artifacts remain source evidence rather than
consumer runtime resources.

The next observable-contract decision can now choose exactly which evolved
intents enter the first `addy` contract and whether hooks are explicit opt-in or
excluded. It must not revisit whether the current schema alone is adequate: it
is not.
