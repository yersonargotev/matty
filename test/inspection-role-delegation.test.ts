import assert from "node:assert/strict";
import test from "node:test";

import {
  runInspectionDelegation,
  type InspectionDelegationExecution,
} from "../src/application/inspection-role-delegation.ts";
import type {
  DelegatedTaskRunner,
} from "../src/application/child-pi-runtime.ts";
import {
  INSPECTION_TOOLS,
} from "../src/domain/capability-contract.ts";
import { commitSha } from "../src/domain/commit-sha.ts";

function availableExecution(
  runner: DelegatedTaskRunner,
): InspectionDelegationExecution {
  return {
    availability: {
      availableTools: INSPECTION_TOOLS,
      independentRuntime: true,
      inspectionGuard: true,
      github: { available: true, authenticated: true },
    },
    async reviewCommitsAvailable() {
      return true;
    },
    createRunner() {
      return runner;
    },
  };
}

test("runs designer and reviewer as independent roles with structured output", async () => {
  const observedTasks: string[] = [];
  const runner: DelegatedTaskRunner = {
    async run(task) {
      observedTasks.push(task);
      return {
        status: "succeeded",
        child: { pid: 42, runId: "run-42" },
        output: JSON.stringify(task.startsWith("Reviewer") ? {
          schemaVersion: 1,
          candidateSha: "1111111111111111111111111111111111111111",
          summary: "role findings",
          findings: [{ axis: "spec", severity: "blocking", requirement: "Issue 9", evidence: "verified" }],
        } : {
          summary: "role findings",
          evidence: [{ observation: "verified" }],
        }),
        exit: { code: 0, signal: null },
      };
    },
  };

  const designer = await runInspectionDelegation(
    "designer",
    "Propose an interface",
    availableExecution(runner),
  );
  const reviewer = await runInspectionDelegation(
    "reviewer",
    "Review issue 9",
    availableExecution(runner),
    { reviewScope: {
      schemaVersion: 1,
      issue: { repository: "github.com/acme/repo", number: 9, reference: "#9" },
      requirements: ["Issue 9"],
      outOfScope: [{ reference: "#42", reason: "dependent publication behavior" }],
      baseSha: commitSha("0000000000000000000000000000000000000000"),
      candidateSha: commitSha("1111111111111111111111111111111111111111"),
      axes: ["spec"],
    } },
  );

  assert.match(observedTasks[0] ?? "", /Designer assignment/);
  assert.match(observedTasks[0] ?? "", /Propose an interface/);
  assert.match(observedTasks[1] ?? "", /Reviewer assignment/);
  assert.match(observedTasks[1] ?? "", /read-only GitHub inspection/);
  assert.equal(designer.contract.role, "designer");
  assert.equal(reviewer.contract.role, "reviewer");
  assert.equal(designer.outcome.status, "succeeded");
  assert.equal(reviewer.outcome.status, "succeeded");
  if (reviewer.outcome.status === "succeeded") {
    assert.deepEqual(reviewer.outcome.output, {
      schemaVersion: 1,
      candidateSha: "1111111111111111111111111111111111111111",
      summary: "role findings",
      findings: [{ axis: "spec", severity: "blocking", requirement: "Issue 9", evidence: "verified" }],
    });
  }
});

test("reviewer rejects findings outside exact requirements or targeting excluded #42", async (context) => {
  const scope = {
    schemaVersion: 1 as const,
    issue: { repository: "github.com/acme/repo", number: 9, reference: "#9" },
    requirements: ["Implement the bounded contract"],
    outOfScope: [{ reference: "https://github.com/acme/repo/issues/42", reason: "dependent publication behavior" }],
    baseSha: commitSha("0000000000000000000000000000000000000000"),
    candidateSha: commitSha("1111111111111111111111111111111111111111"),
    axes: ["spec" as const],
  };
  for (const [name, requirement, evidence] of [
    ["unmatched requirement", "Prompt compliance", "local evidence"],
    ["excluded dependency", "Implement the bounded contract", "This instead requires #42 publication"],
  ] as const) {
    await context.test(name, async () => {
      const terminal = await runInspectionDelegation(
        "reviewer",
        "Review",
        availableExecution({
          async run() {
            return {
              status: "succeeded",
              child: { pid: 42, runId: "run-42" },
              output: JSON.stringify({
                schemaVersion: 1,
                candidateSha: scope.candidateSha,
                summary: "review",
                findings: [{ axis: "spec", severity: "blocking", requirement, evidence }],
              }),
              exit: { code: 0, signal: null },
            };
          },
        }),
        { reviewScope: scope },
      );
      assert.equal(terminal.outcome.status, "failed");
      if (terminal.outcome.status === "failed") {
        assert.equal(terminal.outcome.failure.kind, "invalid-role-output");
      }
    });
  }
});

test("reviewer rejects an unresolved review commit before runner construction", async () => {
  let runnerConstructed = false;
  const terminal = await runInspectionDelegation(
    "reviewer",
    "Review",
    {
      ...availableExecution({ async run() { throw new Error("must not run"); } }),
      async reviewCommitsAvailable() {
        return false;
      },
      createRunner() {
        runnerConstructed = true;
        throw new Error("must not create a runner");
      },
    },
    { reviewScope: {
      schemaVersion: 1,
      issue: { repository: "github.com/acme/repo", number: 9, reference: "#9" },
      requirements: ["Issue 9"],
      outOfScope: [],
      baseSha: commitSha("0000000000000000000000000000000000000000"),
      candidateSha: commitSha("1111111111111111111111111111111111111111"),
      axes: ["spec"],
    } },
  );

  assert.equal(runnerConstructed, false);
  assert.deepEqual(terminal.outcome, {
    status: "blocked",
    diagnostic: {
      kind: "capability-preflight",
      contractId: "delegate-reviewer",
      unmet: ["review-commit-unavailable"],
    },
  });
});

test("reviewer capability preflight completes before runner construction", async () => {
  const blocked = await runInspectionDelegation("reviewer", "Review", {
    availability: {
      availableTools: INSPECTION_TOOLS,
      independentRuntime: true,
      inspectionGuard: true,
      github: { available: true, authenticated: false },
    },
    async reviewCommitsAvailable() {
      throw new Error("capability preflight must happen first");
    },
    createRunner() {
      throw new Error("must not create a runner");
    },
  });

  assert.deepEqual(blocked.outcome, {
    status: "blocked",
    diagnostic: {
      kind: "capability-preflight",
      contractId: "delegate-reviewer",
      unmet: ["GitHub CLI authentication is unavailable"],
    },
  });
});

test("rejects an unstructured successful role response", async () => {
  const terminal = await runInspectionDelegation(
    "designer",
    "Design",
    availableExecution({
      async run() {
        return {
          status: "succeeded",
          child: { pid: 42, runId: "run-42" },
          output: "plain text",
          exit: { code: 0, signal: null },
        };
      },
    }),
  );

  assert.deepEqual(terminal.outcome, {
    status: "failed",
    child: { pid: 42, runId: "run-42" },
    failure: {
      kind: "invalid-role-output",
      message: "inspection role output must be structured JSON findings",
    },
    exit: { code: 0, signal: null },
  });
});
