import assert from "node:assert/strict";
import test from "node:test";

import {
  consumeRepairCycle,
  initialRepairBudget,
  recordExceptionalRepairAuthorization,
  renderIssueDeliveryOutcome,
  repairBudget,
  type IssueDeliveryOutcome,
} from "../src/domain/issue-delivery.ts";
import { commitSha } from "../src/domain/commit-sha.ts";

test("active Issue Delivery renders semantic candidate, checks, blockers, and Repair Budget safely", () => {
  const sha = commitSha("1111111111111111111111111111111111111111");
  const outcome: IssueDeliveryOutcome = {
    schemaVersion: 1,
    status: "active",
    deliveryIdentity: { repository: "github.com/acme/repo", tracker: "github", issue: 42 },
    gate: "verification",
    candidateSha: sha,
    candidateState: "local-unpublished",
    repairBudget: initialRepairBudget(),
    checks: { state: "pending", total: 2, passed: 1, pending: 1, failed: 0 },
    blockers: ["checks-pending"],
  };
  const rendered = renderIssueDeliveryOutcome(outcome);
  assert.match(rendered, /Gate: verification/);
  assert.match(rendered, /Candidate: local-unpublished/);
  assert.match(rendered, /Checks: pending/);
  assert.match(rendered, /Blockers: checks-pending/);
  assert.match(rendered, /Repair Budget: 0\/3/);
  assert.doesNotMatch(rendered, new RegExp(sha));
});

test("blocked Issue Delivery renders the complete Exception Brief without JSON", () => {
  const rendered = renderIssueDeliveryOutcome({
    schemaVersion: 1,
    status: "blocked",
    exceptionBrief: {
      schemaVersion: 1,
      gate: "verification",
      evidence: ["checks-failing" as never],
      need: "A passing authoritative gate.",
      options: ["Repair accepted findings."],
      recommendation: "Inspect and rerun.",
    },
  });
  assert.match(rendered, /Need: A passing authoritative gate/);
  assert.match(rendered, /Options: Repair accepted findings/);
  assert.doesNotMatch(rendered, /^\{/);
});

test("Repair Budget consumes adjudicated cycles and rejects exhaustion", () => {
  const first = consumeRepairCycle(initialRepairBudget(), {
    reason: "accepted review findings",
    findings: ["nominal SHA", "closed reviewer scope"],
  });
  assert.deepEqual(first, {
    schemaVersion: 1,
    limit: 3,
    used: 1,
    remaining: 2,
    cycles: [{
      cycle: 1,
      reason: "accepted review findings",
      findings: ["nominal SHA", "closed reviewer scope"],
    }],
  });
  const exhausted = consumeRepairCycle(consumeRepairCycle(first, {
    reason: "new adjudicated finding",
    findings: ["second finding"],
  }), {
    reason: "final adjudicated finding",
    findings: ["third finding"],
  });
  assert.equal(exhausted.remaining, 0);
  assert.throws(() => consumeRepairCycle(exhausted, {
    reason: "over budget",
    findings: ["fourth finding"],
  }), /exhausted/);
});

test("Repair Budget records and safely renders complete exceptional authorization", () => {
  const consumed = consumeRepairCycle(initialRepairBudget(), {
    reason: "accepted\nfindings",
    findings: ["repair\tone", "repair two"],
  });
  const budget = recordExceptionalRepairAuthorization(consumed, {
    decision: "authorized",
    rationale: "urgent\nrepair",
    actor: "release\tmanager",
    evidence: "incident\u001b[31m",
    scope: "one candidate",
  });
  const rendered = renderIssueDeliveryOutcome({
    schemaVersion: 1,
    status: "active",
    deliveryIdentity: { repository: "github.com/acme/repo", tracker: "github", issue: 42 },
    gate: "implementation",
    candidateSha: null,
    candidateState: "none",
    repairBudget: budget,
    checks: { state: "none", total: 0, passed: 0, pending: 0, failed: 0 },
    blockers: ["implementation-required"],
  });
  assert.match(rendered, /Repair cycle 1: accepted\\u000afindings/);
  assert.match(rendered, /Finding: repair\\u0009one/);
  assert.match(rendered, /Finding: repair two/);
  assert.match(rendered, /decision: authorized/);
  assert.match(rendered, /rationale: urgent\\u000arepair/);
  assert.match(rendered, /actor: release\\u0009manager/);
  assert.match(rendered, /evidence: incident\\u001b\[31m/);
  assert.match(rendered, /scope: one candidate/);
});

test("Repair Budget rejects inconsistent or invented cycle records", () => {
  assert.throws(() => repairBudget({ schemaVersion: 1, limit: 2, used: 0, remaining: 2, cycles: [] }));
  assert.throws(() => repairBudget({ schemaVersion: 1, limit: 3, used: 1, remaining: 2, cycles: [] }));
  assert.throws(() => repairBudget({ schemaVersion: 1, limit: 3, used: 1, remaining: 2, cycles: [{ cycle: 1, reason: "", findings: [] }] }));
});
