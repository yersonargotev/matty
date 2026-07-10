Type: grilling
Status: resolved
Blocked by: 01

## Question

What is the smallest host-independent capability-pack manifest and resource vocabulary that fully represents the real `matty` and `engram` packs while keeping `web` and `mobile` credible validation scenarios rather than speculative schema features?

## Answer

The portable contract is a strict JSON file named `pack.json`. Its smallest top-level envelope is:

```json
{
  "schema_version": 1,
  "id": "matty",
  "version": "1.0.0",
  "provides": ["workflow:matty"],
  "requires": {
    "capabilities": [],
    "tools": []
  },
  "conflicts": [],
  "resources": []
}
```

`id` and `version` identify the desired pack revision; `schema_version` versions this contract. `provides`, `requires.capabilities`, and `conflicts` contain namespaced capability identifiers. `requires.tools` contains global executable prerequisites that Matty core resolves independently of any one CLI surface. External tools are requirements rather than activatable resources, so the Engram executable does not churn when one surface is enabled or disabled.

The first schema supports exactly four resource kinds:

| Kind | Required fields | Portable meaning |
| --- | --- | --- |
| `skill` | `kind`, `id`, `source` | One explicitly enumerated agent skill. |
| `instruction` | `kind`, `id`, `source` | One independently owned body of always-active behavioral guidance. |
| `mcp_server` | `kind`, `id`, `command`, `args` | One named local MCP server invocation. |
| `lifecycle` | `kind`, `id` | One event-driven behavioral intent implemented independently by every supporting CLI-surface adapter. |

Resources form one discriminated list. Their identity is `(kind, id)`, and every resource implicitly provides capability `<kind>:<id>`; top-level `provides` therefore contains only additional semantic capabilities. There is no generic `config` map, resource-to-resource dependency graph, host target, destination path, enablement flag, or universal hook schema. A surface adapter must reject a resource kind or lifecycle intent it cannot implement rather than silently omit it.

Pack IDs, resource IDs, and capability segments use lowercase kebab-case. Pack versions use SemVer. Strict decoding rejects unknown fields, unknown resource kinds, duplicate `(kind, id)` identities, invalid identifiers, and invalid versions. Each `source` is relative to the logical pack root; validation rejects absolute paths, `..` traversal, and symlink resolution outside that root.

### Real pack proof

The `matty` pack provides `workflow:matty`. It requires no tool or capability, declares no conflict, owns one `instruction` named `matty-guidance`, and enumerates each of its 23 skills individually:

```text
ask-matt, code-review, codebase-design, diagnosing-bugs, domain-modeling,
grill-with-docs, implement, improve-codebase-architecture, prototype,
research, resolving-merge-conflicts, setup-matt-pocock-skills, tdd,
to-spec, to-tickets, triage, wayfinder, loop-me, grill-me, grilling,
handoff, teach, writing-great-skills
```

The instruction groups Matty workflow, operational rules, and delegation guidance because they share one ownership and activation lifecycle. Adapters may render it as multiple Codex marker blocks or one OpenCode instruction file. Every skill has its own `source`; directory globs are forbidden. Skill-to-skill relationships such as `wayfinder` consuming `grilling` do not create an internal dependency graph because the pack activates them together. If a future pack separates such a consumer, it can require the derived capability `skill:grilling`.

The `engram` pack provides `memory:persistent`, requires tool `engram`, requires no capability, and declares no conflict. Its complete logical resources are:

```json
[
  {
    "kind": "instruction",
    "id": "engram-memory",
    "source": "instructions/engram-memory.md"
  },
  {
    "kind": "mcp_server",
    "id": "engram",
    "command": "engram",
    "args": ["mcp", "--tools=agent"]
  },
  {
    "kind": "lifecycle",
    "id": "engram-memory"
  }
]
```

`engram-memory.md` is a small Matty-owned portable instruction, not a copy of the Engram source tree or a reference into an installed plugin. The adapters continue delegating external integration to the required Engram tool and avoid duplicating guidance. Codex plugin files, hooks, scripts, trust state, and MCP schema and the OpenCode plugin callbacks, config, and system-prompt injection are adapter projections—not common manifest fields.

`matty` and `engram` remain independently activatable. Matty's “use Engram when available” guidance is optional interoperability, not a hard dependency.

`web` and `mobile` are now entirely deferred at the user's request. They do not validate this schema and reserve no fields or resource kinds; a future request will define them from their concrete skills, MCPs, and other evidence.
