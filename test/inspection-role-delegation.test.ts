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
        output: JSON.stringify({
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
      summary: "role findings",
      evidence: [{ observation: "verified" }],
    });
  }
});

test("reviewer capability preflight completes before runner construction", async () => {
  const blocked = await runInspectionDelegation("reviewer", "Review", {
    availability: {
      availableTools: INSPECTION_TOOLS,
      independentRuntime: true,
      inspectionGuard: true,
      github: { available: true, authenticated: false },
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
