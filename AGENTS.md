# Agent guidance

- Read the relevant accepted ADR under `docs/adr/` before changing architecture; keep architectural decisions there rather than duplicating them here.
- Keep Matty domain behavior in its owning package under `internal/`; `internal/cli` should adapt that behavior to commands and state.
- Sandbox `HOME` and `XDG_CONFIG_HOME` for tests or manual checks that resolve or write user paths.
- Run `./scripts/validate-matty.sh` as the repository validation authority
  before committing or reporting success. Keep `go test ./...` green while the
  repository has no vendored upstream Go content.

## Agent skills

### Issue tracker

Issues are tracked in GitHub through the `gh` CLI; external pull requests are not a triage surface. See `docs/agents/issue-tracker.md`.

### Triage labels

Canonical triage roles map to the repository's existing status vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

Matty uses a single-context domain layout. See `docs/agents/domain.md`.
