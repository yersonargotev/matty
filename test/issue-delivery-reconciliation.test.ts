import assert from "node:assert/strict";
import test from "node:test";

import {
  deliverIssue,
  type IssueDeliveryInspection,
  type IssueDeliveryWorkspace,
} from "../src/application/issue-delivery.ts";
import type { DeliveryIdentity } from "../src/domain/issue-delivery.ts";

const BASE_SHA = "0000000000000000000000000000000000000000";
const CANDIDATE_SHA = "1111111111111111111111111111111111111111";

const identity: DeliveryIdentity = {
  repository: "github.com/yersonargotev/matty",
  tracker: "github",
  issue: 36,
};

function activeWorkspace(candidateSha: string | null): IssueDeliveryWorkspace {
  return {
    async inspect() {
      return {
        status: "active" as const,
        delivery: {
          identity,
          branch: "matty/deliver-36-owned",
          integrationBranch: "main",
          integrationSha: BASE_SHA,
          candidateSha,
        },
      };
    },
    async prepare() {
      throw new Error("resume must not prepare the workspace");
    },
  };
}

function inspection(change: Partial<IssueDeliveryInspection> = {}): IssueDeliveryInspection {
  return {
    issue: { state: "open" },
    pullRequests: [],
    remoteBranches: {
      deliverySha: null,
      integrationSha: BASE_SHA,
    },
    checks: [],
    ...change,
  };
}

test("repeating delivery resumes its durable identity without replaying workspace effects", async () => {
  let preflightReads = 0;
  let inspectionReads = 0;
  const outcome = await deliverIssue(
    { intent: "deliver", issue: "36", cwd: "/repo" },
    async () => {
      preflightReads += 1;
      throw new Error("qualification must not replay");
    },
    activeWorkspace(CANDIDATE_SHA),
    async (request) => {
      inspectionReads += 1;
      assert.equal(request.candidateSha, CANDIDATE_SHA);
      return inspection({
        checks: [
          { status: "completed", conclusion: "success" },
          { status: "in_progress", conclusion: null },
        ],
      });
    },
  );

  assert.equal(preflightReads, 0);
  assert.equal(inspectionReads, 1);
  assert.deepEqual(outcome, {
    schemaVersion: 1,
    status: "active",
    deliveryIdentity: identity,
    gate: "verification",
    candidateSha: CANDIDATE_SHA,
    checks: { state: "pending", total: 2, passed: 1, pending: 1, failed: 0 },
    blockers: ["checks-pending"],
  });
});

test("an owned branch at the integration SHA remains at implementation", async () => {
  const outcome = await deliverIssue(
    { intent: "status", issue: "36", cwd: "/repo" },
    async () => { throw new Error("status must not qualify"); },
    activeWorkspace(null),
    async () => inspection(),
  );

  assert.equal(outcome.status, "active");
  if (outcome.status === "active") {
    assert.equal(outcome.gate, "implementation");
    assert.equal(outcome.candidateSha, null);
    assert.deepEqual(outcome.checks, {
      state: "none", total: 0, passed: 0, pending: 0, failed: 0,
    });
    assert.deepEqual(outcome.blockers, ["implementation-required"]);
  }
});

test("a completed startup_failure Check Run is a failed candidate check", async () => {
  const outcome = await deliverIssue(
    { intent: "status", issue: "36", cwd: "/repo" },
    async () => { throw new Error("unused"); },
    activeWorkspace(CANDIDATE_SHA),
    async () => inspection({
      checks: [{ status: "completed", conclusion: "startup_failure" }],
    }),
  );

  assert.equal(outcome.status, "active");
  if (outcome.status === "active") {
    assert.deepEqual(outcome.checks, {
      state: "failing", total: 1, passed: 0, pending: 0, failed: 1,
    });
    assert.deepEqual(outcome.blockers, ["checks-failing"]);
  }
});

test("candidate checks are aggregated without exposing hostile provider data", async () => {
  const hostile = "https://secret.invalid token=ghp_secret /private/path check-name";
  const outcome = await deliverIssue(
    { intent: "status", issue: "36", cwd: "/repo" },
    async () => { throw new Error("unused"); },
    activeWorkspace(CANDIDATE_SHA),
    async () => inspection({
      issue: { state: "closed" },
      checks: [
        { status: "completed", conclusion: "failure", raw: hostile } as never,
        { status: "queued", conclusion: null, raw: hostile } as never,
      ],
    }),
  );

  assert.equal(outcome.status, "active");
  assert.doesNotMatch(JSON.stringify(outcome), /secret|private|check-name/);
  if (outcome.status === "active") {
    assert.equal(outcome.checks.state, "failing");
    assert.deepEqual(outcome.blockers, ["issue-closed", "checks-failing", "checks-pending"]);
  }
});

test("ambiguous PRs and remote candidate drift return fixed Exception Briefs", async (context) => {
  const otherSha = "2222222222222222222222222222222222222222";
  const cases: Array<[string, IssueDeliveryInspection, string]> = [
    ["ambiguous", inspection({ pullRequests: [
      { compatibility: "compatible", headSha: CANDIDATE_SHA },
      { compatibility: "compatible", headSha: CANDIDATE_SHA },
    ] }), "delivery-pr-ambiguous"],
    ["PR drift", inspection({ pullRequests: [
      { compatibility: "compatible", headSha: otherSha },
    ] }), "delivery-candidate-drift"],
    ["remote branch drift", inspection({ remoteBranches: { deliverySha: otherSha, integrationSha: BASE_SHA } }), "delivery-candidate-drift"],
  ];
  for (const [name, facts, code] of cases) {
    await context.test(name, async () => {
      const outcome = await deliverIssue(
        { intent: "status", issue: "36", cwd: "/repo" },
        async () => { throw new Error("unused"); },
        activeWorkspace(CANDIDATE_SHA),
        async () => facts,
      );
      assert.equal(outcome.status, "blocked");
      if (outcome.status === "blocked") {
        assert.deepEqual(outcome.exceptionBrief.evidence, [code]);
      }
    });
  }
});

test("an advanced remote integration branch is visible without changing the qualified base", async () => {
  const advancedSha = "3333333333333333333333333333333333333333";
  let inspectedBase: string | undefined;
  const outcome = await deliverIssue(
    { intent: "status", issue: "36", cwd: "/repo" },
    async () => { throw new Error("unused"); },
    activeWorkspace(CANDIDATE_SHA),
    async (request) => {
      inspectedBase = request.integrationSha;
      return inspection({
        remoteBranches: { deliverySha: CANDIDATE_SHA, integrationSha: advancedSha },
      });
    },
  );

  assert.equal(inspectedBase, BASE_SHA);
  assert.equal(outcome.status, "active");
  if (outcome.status === "active") {
    assert.deepEqual(outcome.blockers, ["integration-advanced"]);
  }
});

test("malformed required inspection returns a fixed redacted Exception Brief", async () => {
  const outcome = await deliverIssue(
    { intent: "status", issue: "36", cwd: "/repo" },
    async () => { throw new Error("unused"); },
    activeWorkspace("not-a-commit /private/token"),
    async () => inspection(),
  );
  assert.equal(outcome.status, "blocked");
  assert.doesNotMatch(JSON.stringify(outcome), /not-a-commit|private|token/);
  if (outcome.status === "blocked") {
    assert.deepEqual(outcome.exceptionBrief.evidence, ["delivery-inspection-unavailable"]);
  }
});

test("malformed candidate checks block without exposing provider fields", async () => {
  const outcome = await deliverIssue(
    { intent: "status", issue: "36", cwd: "/repo" },
    async () => { throw new Error("unused"); },
    activeWorkspace(CANDIDATE_SHA),
    async () => inspection({
      checks: [{
        status: "completed",
        conclusion: "ghp_secret /private/provider",
      } as never],
    }),
  );

  assert.equal(outcome.status, "blocked");
  assert.doesNotMatch(JSON.stringify(outcome), /secret|private|provider/);
  if (outcome.status === "blocked") {
    assert.deepEqual(outcome.exceptionBrief.evidence, ["delivery-inspection-unavailable"]);
  }
});

test("unavailable reconciliation is redacted and cannot produce effects", async () => {
  const outcome = await deliverIssue(
    { intent: "status", issue: "36", cwd: "/repo" },
    async () => { throw new Error("unused"); },
    activeWorkspace(CANDIDATE_SHA),
    async () => { throw new Error("ghp_secret provider output"); },
  );
  assert.equal(outcome.status, "blocked");
  assert.doesNotMatch(JSON.stringify(outcome), /ghp_secret|provider output/);
  if (outcome.status === "blocked") {
    assert.deepEqual(outcome.exceptionBrief.evidence, ["delivery-inspection-unavailable"]);
  }
});
