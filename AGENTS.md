# Agent guidance

- Read the relevant accepted ADR under `docs/adr/` before changing architecture; keep architectural decisions there rather than duplicating them here.
- Keep Matty domain behavior in its owning package under `internal/`; `internal/cli` should adapt that behavior to commands and state.
- Sandbox `HOME` and `XDG_CONFIG_HOME` for tests or manual checks that resolve or write user paths.
- Run `go test ./...` before committing or reporting success.
