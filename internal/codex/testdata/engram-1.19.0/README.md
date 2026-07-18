# Engram 1.19.0 Codex contract fixture

The two companion files in this directory were captured byte-for-byte from
`/opt/homebrew/bin/engram setup codex` version 1.19.0 on 2026-07-12 with
`HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, and `CODEX_HOME` isolated under
`/tmp/packy-engram-converge.7GOgu6`.

SHA-256:

- `engram-instructions.md`: `74176fb0847b06fb725ae8992c9a5fa12022ff347ca3ee2ef3e77c6d318d5fb3`
- `engram-compact-prompt.md`: `c779d9584c8ca16331ebb31a753f7fbb5bcb8193b229572a54da189ffaa97fd1`

The integration fixture executes a sandboxed external process that writes
these captured files plus the native MCP, marketplace, and plugin settings.
Update this directory deliberately when Packy adopts a new Engram contract.
