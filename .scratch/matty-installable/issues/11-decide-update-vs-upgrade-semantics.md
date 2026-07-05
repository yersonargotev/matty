# 11 — Decide update vs upgrade semantics for package-installed Matty

Type: grilling
Status: open
Blocked by: 01, 03

## Question

After Matty is installed through Homebrew and initialized with a versioned source, what should `matty update` mean, and is a separate `matty upgrade` needed?

Current `matty update` refreshes Engram via Homebrew and reapplies Matty-managed skills/prompts. Package installation introduces two more moving parts: the Matty binary and the initialized source/bundle.

## Acceptance criteria

- Defines whether `matty update` updates only managed workflow artifacts or also the initialized source/bundle.
- Defines whether binary upgrades are delegated to `brew upgrade matty` or wrapped by a `matty upgrade` command.
- Explains how dry-run behaves without mutating the initialized source.
- Identifies any follow-up implementation issues created by the decision.
