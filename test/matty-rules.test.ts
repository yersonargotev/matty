import assert from "node:assert/strict";
import test from "node:test";

import {
  MATTY_RULES_END,
  MATTY_RULES_START,
  detectMattyRulesConflict,
  injectMattyRules,
} from "../src/domain/matty-rules.ts";

function occurrences(value: string, fragment: string): number {
  return value.split(fragment).length - 1;
}

test("injects exactly one marked Matty Rules block", () => {
  const prompt = injectMattyRules("Base host instructions", "parent");

  assert.equal(occurrences(prompt, MATTY_RULES_START), 1);
  assert.equal(occurrences(prompt, MATTY_RULES_END), 1);
  assert.match(prompt, /Base host instructions/);
  assert.match(prompt, /subagent accepts exactly \{"task": string\}/);
});

test("replaces duplicate and stray markers with one child rules block", () => {
  const existing = [
    "Base",
    MATTY_RULES_START,
    "stale rules",
    MATTY_RULES_END,
    MATTY_RULES_START,
    "duplicate rules",
    MATTY_RULES_END,
    MATTY_RULES_START,
  ].join("\n");

  const prompt = injectMattyRules(existing, "explorer");

  assert.equal(occurrences(prompt, MATTY_RULES_START), 1);
  assert.equal(occurrences(prompt, MATTY_RULES_END), 1);
  assert.doesNotMatch(prompt, /stale rules|duplicate rules/);
  assert.match(prompt, /Active child role: explorer/);
});

test("removes inline and unmatched markers before injecting the block", () => {
  const prompt = injectMattyRules(
    `Base ${MATTY_RULES_START} orphan ${MATTY_RULES_END}`,
    "parent",
  );

  assert.equal(occurrences(prompt, MATTY_RULES_START), 1);
  assert.equal(occurrences(prompt, MATTY_RULES_END), 1);
});

test("detects a direct project instruction conflict outside marked rules", () => {
  assert.equal(
    detectMattyRulesConflict(
      "Project policy: explorers may edit files when convenient.",
    ),
    "project instructions grant explorer write authority",
  );
  assert.equal(
    detectMattyRulesConflict(
      `${MATTY_RULES_START}\nexplorers may edit files\n${MATTY_RULES_END}`,
    ),
    undefined,
  );
});

test("rules describe only the currently exposed explorer path", () => {
  const prompt = injectMattyRules("Base", "parent");

  assert.match(prompt, /currently exposed path selects explorer/);
  assert.doesNotMatch(prompt, /Worker Guard|eight tasks|four children/);
});
