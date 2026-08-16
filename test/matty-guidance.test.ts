import assert from "node:assert/strict";
import test from "node:test";

import {
  MATTY_GUIDANCE_END,
  MATTY_GUIDANCE_START,
  injectMattyGuidance,
} from "../src/domain/matty-guidance.ts";

function occurrences(value: string, fragment: string): number {
  return value.split(fragment).length - 1;
}

const EXPECTED_ARGOTE_GUIDANCE = `# Argote guidance

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

test("injects exactly one versioned Matty Guidance block after host instructions", () => {
  const prompt = injectMattyGuidance("Host and project instructions");

  assert.equal(occurrences(prompt, MATTY_GUIDANCE_START), 1);
  assert.equal(occurrences(prompt, MATTY_GUIDANCE_END), 1);
  assert.ok(prompt.indexOf("Host and project instructions") < prompt.indexOf(MATTY_GUIDANCE_START));
  assert.match(prompt, /Matty Guidance v1\n# Argote guidance/);
});

test("preserves the supplied Argote content and precedence language exactly", () => {
  assert.equal(
    injectMattyGuidance(""),
    [
      MATTY_GUIDANCE_START,
      "Matty Guidance v1",
      EXPECTED_ARGOTE_GUIDANCE,
      MATTY_GUIDANCE_END,
    ].join("\n"),
  );
});

test("replaces stale and duplicate guidance, including inline and unmatched markers", () => {
  const existing = [
    `Base ${MATTY_GUIDANCE_START} inline stale ${MATTY_GUIDANCE_END}`,
    MATTY_GUIDANCE_START,
    "duplicate stale",
    MATTY_GUIDANCE_END,
    `orphan ${MATTY_GUIDANCE_START}`,
    `other orphan ${MATTY_GUIDANCE_END}`,
  ].join("\n");

  const prompt = injectMattyGuidance(existing);

  assert.equal(occurrences(prompt, MATTY_GUIDANCE_START), 1);
  assert.equal(occurrences(prompt, MATTY_GUIDANCE_END), 1);
  assert.doesNotMatch(prompt, /inline stale|duplicate stale/);
});

test("injection is deterministic and idempotent", () => {
  const once = injectMattyGuidance("Base");
  assert.equal(injectMattyGuidance(once), once);
});
