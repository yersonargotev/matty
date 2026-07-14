# Domain Docs

Matty uses a single-context domain layout. These rules tell engineering skills how to consume its domain documentation before exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repository root for the project's domain vocabulary.
- **`docs/adr/`** for accepted decisions that touch the area about to be changed.

If either location does not exist, proceed silently. Do not flag its absence or suggest creating it upfront. The `/domain-modeling` skill creates domain documentation lazily when terms or decisions are resolved.

## File structure

```text
/
├── CONTEXT.md
└── docs/adr/
```

## Use the glossary's vocabulary

When output names a domain concept—in an issue title, refactor proposal, hypothesis, or test name—use the term defined in `CONTEXT.md`. Do not drift to synonyms the glossary explicitly avoids.

If a needed concept is absent, reconsider whether the term belongs to the project or note the gap for `/domain-modeling`.

## Flag ADR conflicts

If output contradicts an accepted ADR, surface the conflict explicitly rather than silently overriding the decision.
