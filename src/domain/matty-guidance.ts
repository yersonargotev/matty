export const MATTY_GUIDANCE_START = "<!-- matty:guidance -->";
export const MATTY_GUIDANCE_END = "<!-- /matty:guidance -->";

const ARGOTE_GUIDANCE = `# Argote guidance

## Engineering principles

Apply these defaults unless a more specific user or project instruction governs the work.

- Choose the simplest implementation that fully satisfies the known requirements.
- Build in small, end-to-end increments and keep the product working after each meaningful change.
- Give each component cohesive ownership. Add a boundary, layer, or module only when it provides a concrete separation benefit.
- Before implementing common functionality, inspect the existing dependencies, documentation, and types. Prefer an established, well-maintained library when it reduces total complexity or materially improves reliability.
- Design for every known requirement without planning a later replacement. When requirements remain uncertain, choose a simple, reversible decision.
- Remove obsolete and dead paths as part of the requested change. Keep compatibility layers only when compatibility is explicit.
- Follow the project's explicit compatibility policy. Otherwise, preserve public behavior, persisted data, and external contracts unless the task authorizes breaking them.

## Neutral Spanish

Use neutral, international Spanish for user-facing conversation. Use natural \`tú\` and avoid marked regionalisms.

Write code, identifiers, comments, documentation, plans, ADRs, and commit messages in English. Preserve the language of a source or interface. Preserve technical terms, code, commands, and product names when that improves precision or naturalness.

An explicitly requested output language overrides this default. This guidance governs observable messages and artifacts; do not claim or control hidden reasoning. More-specific user or project instructions override these defaults, and higher-priority authority and safety instructions prevail.`;

function renderMattyGuidance(): string {
  return [
    MATTY_GUIDANCE_START,
    "Matty Guidance v1",
    ARGOTE_GUIDANCE,
    MATTY_GUIDANCE_END,
  ].join("\n");
}

export function withoutMattyGuidance(systemPrompt: string): string {
  return systemPrompt
    .replace(
      /<!-- matty:guidance -->[\s\S]*?<!-- \/matty:guidance -->/g,
      "",
    )
    .replaceAll(MATTY_GUIDANCE_START, "")
    .replaceAll(MATTY_GUIDANCE_END, "");
}

export function injectMattyGuidance(systemPrompt: string): string {
  const hostAndProjectInstructions = withoutMattyGuidance(systemPrompt).trim();

  return [hostAndProjectInstructions, renderMattyGuidance()]
    .filter(Boolean)
    .join("\n\n");
}
