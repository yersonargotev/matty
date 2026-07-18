# Workspace Notes

## Packy development

- Source control and delivery use GitHub issues, pull requests, and Actions.
- Packy is a Go CLI; the repository completion gate requires `go test ./...`.
- Architecture and symbol discovery use CodeGraph before source inspection;
  runtime behavior is verified with real commands or tests.
- Tests and manual checks that exercise user configuration sandbox `HOME` and
  `XDG_CONFIG_HOME`.
- External project state such as issue labels, pull requests, merges, tags, and
  releases stays with the primary agent.
- `.agents/skills/release-packy` is the existing project-local release skill.

## Canonical loops

- **Issue delivery**: the end-to-end path from a requested GitHub issue through
  validation, implementation, review, pull request, merge, and branch cleanup.
- **Release**: publication of a verified `main` commit through the existing
  `release-packy` gate.
