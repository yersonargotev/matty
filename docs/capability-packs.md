# Capability packs and manual transition

Capability packs are opt-in additions managed by **Packy core**. Packy remains
available when the optional pack named `matty` is inactive. Discovery, show,
status, and dry-run are inspection-only.

| Current Pack | Purpose | Claude contract |
| --- | --- | --- |
| `matty` **3.0.0** | Workflow skills and guidance | **complete**: every skill and instruction has a native binding and no Claude exclusion. Projection does not prove runtime usability. |
| `engram` **2.0.0** | Memory guidance and MCP | **degraded**, but activatable: its instruction and exact user MCP binding are native; `lifecycle:engram-memory` has the optional `generic-lifecycle-unsupported` exclusion because generic lifecycle translation is unsupported. Packy does not run `engram setup claude-code`. |

In plain release notation, the current contracts are **matty 3.0.0** and
**engram 2.0.0**. Their Claude Code behavior is described in the table above.

`addy` 1.0.0 remains its exact manifest-v2 Codex/OpenCode contract. Historical
`matty` 2.0.0 and `engram` 1.0.0 activations remain pinned to their recorded
versions and surfaces. Updating can select v3 for already-active surfaces, but
never adds Claude intent. Claude activation is a separate explicit choice.

Remote/third-party sources, marketplaces, signing, version selection,
downgrades, unattended Apply, and background runtime management are excluded.

## Manifest v3 bindings and exclusions

Only manifest v3 can declare Claude. Its required, sorted, non-null `surfaces`
array names the Pack's surfaces. Every runtime resource (`skill`, `instruction`,
`mcp_server`, `lifecycle`, `agent`, or `command`) declares exactly one binding
or exclusion for every top-level surface. Assets inherit their consumers;
notices have empty outcome arrays and do not affect readiness. V1 and v2 remain
exact and never infer Claude support.

Claude translations are explicit:

- skills and commands use personal skills under `~/.claude/skills`, never
  legacy `.claude/commands`;
- instructions contribute deterministic text to the global marked Packy block;
- MCP uses official `claude mcp ... --scope user` operations;
- agents require explicit documented native tool and permission translations;
- lifecycle is supported only through an explicit typed command hook.

Generic lifecycle translation and opaque JSON injection are not supported. An
optional exclusion makes compatibility **degraded**. A mandatory exclusion, or
an excluded dependency of a mandatory resource, makes compatibility **blocked**
and the plan non-applicable. Compatibility is **complete** only when every
required resource has a native binding and no degradation or exclusion.

## Inspect and activate

```sh
packy pack list
packy pack show matty
packy pack show engram
packy pack status
packy pack status matty --surface claude

packy pack activate matty --surface claude --dry-run
packy pack activate matty --surface claude
```

There is no automatic adoption of existing content. Each mutation targets one
surface; activating Codex or OpenCode never activates Claude. Dry-run creates a
fresh Preview without approval or mutation. Apply requires an interactive
terminal and plan-bound typed approval; there is no generic `--yes`.

Preview reports compatibility before readiness, every binding/exclusion, exact
projections, preservation, blockers, expected readiness, and pending evidence.
It separates reversible local, executable/external, and destructive-cleanup
phases. Claude typed hooks require executable/external consent. Removing a
last-contributor hook or user MCP definition requires destructive-cleanup
consent. MCP environment values are always redacted. Any preview blocker
executes zero effects.

## Update, reconcile, recovery, and deactivation

```sh
packy pack update engram --surface claude --dry-run
packy pack update engram --surface claude
packy pack reconcile engram --surface claude --dry-run
packy pack deactivate engram --surface claude --dry-run
packy pack deactivate engram --surface claude
```

Approvals belong to one immutable plan. A stale plan executes no actions;
repeat the originating verb for fresh inspection and consent. After a partial
attempt marked `recovery-required`, also repeat the originating verb. Packy
plans recovery from current evidence rather than replaying history.

Packy updates or removes only an exact unchanged recorded projection. It
preserves unmanaged, ambiguous, drifted, foreign, and higher-precedence content.
Shared resources remain while another contributor is active. Deactivation never
deletes credentials, Engram memory, foreign configuration, or external data. If
Claude is unavailable, local cleanup may proceed while user MCP ownership is
retained for later official removal.

## Apply success is not readiness

Readiness is independent of compatibility and successful Apply:

- **configured**: exact required projections are present;
- **authorized**: configured, on a supported Claude version, with observable
  policy/tool permission;
- **usable**: explicit current evidence says skills, commands, agents, and
  instructions loaded; MCP connected; and hooks fired, as applicable.

Assets inherit their consumer; notices and exclusions do not participate. A
version, definition, projection, policy, or Pack-version change invalidates the
affected evidence. Filesystem correctness never manufactures usability.

```sh
packy pack status engram --surface claude
packy pack status engram --surface claude --require usable
```

The second command emits status and exits nonzero until every required signal
is freshly known true. Login, trust, reload, runtime loading, and external setup
remain human/host boundaries; Packy reports but does not perform them.
