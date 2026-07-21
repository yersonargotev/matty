# {{TAG}} — First-class Claude Code support

This is the first Packy release with Claude Code as a supported global surface
alongside Codex and OpenCode.

## Operator changes

- Claude Code is a user-managed prerequisite. Install stable Claude Code
  **2.1.203 or newer** before applying Packy; Packy does not install, upgrade,
  authenticate, or run a model.
- Classic state migrates from v1 to **state schema v2** only after a verified
  Apply. Existing Codex/OpenCode ownership is preserved while Claude becomes a
  desired surface.
- Existing unmanaged, ambiguous, or drifted Claude files, instructions, hooks,
  skills, agents, and user MCP definitions are preserved for explicit recovery.

## Capability packs

- **matty 3.0.0** has a complete Claude Code contract.
- **engram 2.0.0** is activatable but **degraded** on Claude Code: instructions
  and the exact user MCP projection are native, while generic Engram lifecycle
  translation remains an optional `generic-lifecycle-unsupported` exclusion.

## Limitations

- Packy does not use Claude plugins, project/local Claude configuration, managed
  policy, credentials, login, authentication, REPL, print/model mode, or model
  calls.
- Packy does not claim Claude usability from a successful Apply. Check fresh
  readiness and complete any host-owned trust, reload, or runtime steps.
- Packy remains macOS-first. Artifact names, supported platform artifacts,
  checksums, Homebrew dependencies, and the formula's `packy --version` test are
  unchanged.
