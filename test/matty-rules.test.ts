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
  assert.match(
    prompt,
    /"role": "researcher".*"web": "required"\|"optional".*"report"\?: string/,
  );
  assert.match(
    prompt,
    /Model knowledge is never reported as completed web research/,
  );
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
      "Project policy: reviewers may mutate GitHub state.",
    ),
    "project instructions grant inspection-role mutation authority",
  );
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

test("rules describe all five least-privilege roles", () => {
  const prompt = injectMattyRules("Base", "parent");

  assert.match(
    prompt,
    /exposes explorer, designer, reviewer, researcher, and worker/,
  );
  assert.match(
    prompt,
    /Researcher receives only the four certified Web Capability tools and research_file/,
  );
  assert.match(prompt, /best-effort command policy, not a security sandbox/);
  assert.match(prompt, /Worker Guard is a best-effort command and path policy/);
  assert.match(prompt, /Single Writer permits at most one active worker/);
  assert.doesNotMatch(prompt, /eight tasks|four children/);
});

test("injects the selected designer and reviewer child role", () => {
  assert.match(
    injectMattyRules("Base", "designer"),
    /Active child role: designer/,
  );
  assert.match(
    injectMattyRules("Base", "reviewer"),
    /Active child role: reviewer/,
  );
  assert.match(
    injectMattyRules("Base", "worker"),
    /Active child role: worker; implement within validated paths/,
  );
  assert.match(
    injectMattyRules("Base", "researcher"),
    /Active child role: researcher; use certified web tools and write only bounded research artifacts/,
  );
});
