import assert from "node:assert/strict";
import test from "node:test";

import { commitSha } from "../src/domain/commit-sha.ts";
import {
  reviewerFindings,
  type ReviewScopeContract,
  type ReviewerValidationReason,
} from "../src/domain/review-scope.ts";

const scope: ReviewScopeContract = {
  schemaVersion: 1,
  issue: { repository: "github.com/acme/repo", number: 84, reference: "#84" },
  requirements: ["Bind findings verbatim"],
  outOfScope: [{ reference: "#42", reason: "Excluded dependency" }],
  baseSha: commitSha("0000000000000000000000000000000000000000"),
  candidateSha: commitSha("1111111111111111111111111111111111111111"),
  axes: ["spec"],
};

function response(overrides: Record<string, unknown> = {}): unknown {
  return {
    schemaVersion: 1,
    candidateSha: scope.candidateSha,
    summary: "Review complete",
    findings: [{
      axis: "spec",
      severity: "blocking",
      requirement: "Bind findings verbatim",
      evidence: "Observed in the candidate",
    }],
    ...overrides,
  };
}

const failures: Array<[string, unknown, ReviewerValidationReason]> = [
  ["invalid shape", response({ extra: "private metadata" }), "invalid-shape"],
  ["candidate mismatch", response({ candidateSha: "2222222222222222222222222222222222222222" }), "candidate-sha-mismatch"],
  ["axis outside scope", response({ findings: [{ axis: "security", severity: "blocking", requirement: "Bind findings verbatim", evidence: "Observed" }] }), "axis-not-allowed"],
  ["requirement outside scope", response({ findings: [{ axis: "spec", severity: "blocking", requirement: "A paraphrase", evidence: "Observed" }] }), "requirement-not-in-scope"],
  ["excluded reference", response({ findings: [{ axis: "spec", severity: "blocking", requirement: "Bind findings verbatim", evidence: "Also inspect #42" }] }), "excluded-reference"],
];

for (const [name, value, reason] of failures) {
  test(`reviewer validation reports ${name} with a closed diagnostic`, () => {
    assert.deepEqual(reviewerFindings(value, scope), {
      ok: false,
      diagnostic: { schemaVersion: 1, reason },
    });
  });
}

test("reviewer rejects a specific criterion when the supplied scope has only broad requirements", () => {
  const broadScope = {
    ...scope,
    requirements: [
      "All issue #77 acceptance criteria are fully implemented and evidenced",
      "No policy weakening or parent-model controls",
    ],
  };
  assert.deepEqual(reviewerFindings(response({
    findings: [{
      axis: "spec",
      severity: "blocking",
      requirement: "Distinct TUI/headless Steer and Follow up",
      evidence: "Observed in the candidate",
    }],
  }), broadScope), {
    ok: false,
    diagnostic: { schemaVersion: 1, reason: "requirement-not-in-scope" },
  });
});

test("reviewer validation fails closed when validation itself throws", () => {
  const hostile = new Proxy({}, {
    ownKeys() {
      throw new Error("private validator exception");
    },
  });
  assert.deepEqual(reviewerFindings(hostile, scope), {
    ok: false,
    diagnostic: { schemaVersion: 1, reason: "validation-failed" },
  });
});

test("reviewer validation returns typed findings on success", () => {
  assert.deepEqual(reviewerFindings(response(), scope), {
    ok: true,
    findings: response(),
  });
});
