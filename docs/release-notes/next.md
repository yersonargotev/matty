# {{TAG}} — Protected publication rollout

This release verifies Packy's protected publication path after the repository
governance rollout. It does not introduce new Packy CLI behavior or change the
documented install, update, or uninstall workflows.

## Repository controls

- Packy's required checks now protect `main` through the reviewed pull-request
  path, with force pushes and branch deletion denied.
- Release and Homebrew authority remain separated behind their protected
  environments. The tap credential exists only in the `homebrew` environment;
  there is no repository-level fallback credential.
- Version tags are protected from routine movement and deletion, and this is
  the first Packy version published after future-release immutability is
  enabled.

## Publication integrity

- The release is built once from one exact protected-main commit and carries
  deterministic SHA-256 checksums, an SPDX SBOM, and verified provenance.
- GitHub publication completes and is independently read back before the
  separately approved Homebrew update can begin.
- Published release assets and the associated version tag are immutable. Normal
  correction uses a new monotonically increasing version. Release title and
  notes remain editable residual Owner authority; destructive release deletion
  is an Owner-only break-glass action and permanently prevents reuse of the
  deleted version tag.

## Operator impact and limitations

- Claude Code stable **2.1.203 or newer** remains the supported floor, and
  existing installations remain on **state schema v2**.
- **matty 3.0.0** has a complete Claude Code contract.
- **engram 2.0.0** remains **degraded** on Claude Code where generic lifecycle
  translation is unsupported.
- No Packy-managed files or state schemas change in this release.
- Existing installations continue to use `brew upgrade packy`, followed by
  `packy init`, `packy update --dry-run`, and `packy update`.
- Packy remains macOS-first. Darwin Homebrew installs are the supported user
  path; Linux artifacts remain published for future support.
