import assert from "node:assert/strict";
import test from "node:test";

import type { DelegatedTaskRunner } from "../src/application/child-pi-runtime.ts";
import {
  runResearcherDelegation,
  type ResearcherDelegationExecution,
} from "../src/application/researcher-delegation.ts";
import {
  RESEARCHER_TOOLS,
  createResearcherCapabilityContract,
} from "../src/domain/capability-contract.ts";

function execution(
  runner: DelegatedTaskRunner,
): ResearcherDelegationExecution {
  return {
    contract: createResearcherCapabilityContract({
      web: "required",
      workspaceRoot: "/validated/tmp/matty/research",
      projectRoot: "/trusted/project",
      workspace:
        "/validated/tmp/matty/research/123e4567-e89b-42d3-a456-426614174000",
      report: "/trusted/project/docs/research/result.md",
    }),
    availability: {
      availableTools: RESEARCHER_TOOLS,
      independentRuntime: true,
      inspectionGuard: false,
      researchFileTool: true,
      web: "available",
    },
    createRunner() {
      return runner;
    },
    async reportDelivered() {
      return true;
    },
  };
}

test("runs a researcher with exact artifact destinations and normalizes results", async () => {
  let observedTask = "";
  const terminal = await runResearcherDelegation(
    "Research the primary sources",
    execution({
      async run(task) {
        observedTask = task;
        return {
          status: "succeeded",
          child: { pid: 42, runId: "run-42" },
          output: "research complete",
          exit: { code: 0, signal: null },
        };
      },
    }),
  );

  assert.match(observedTask, /Researcher assignment/);
  assert.match(observedTask, /required web research/);
  assert.match(
    observedTask,
    /workspace: \/validated\/tmp\/matty\/research\/123e4567-e89b-42d3-a456-426614174000/,
  );
  assert.match(
    observedTask,
    /report: \/trusted\/project\/docs\/research\/result\.md/,
  );
  assert.deepEqual(terminal.artifacts, {
    workspace:
      "/validated/tmp/matty/research/123e4567-e89b-42d3-a456-426614174000",
    report: "/trusted/project/docs/research/result.md",
  });
  assert.equal(terminal.outcome.status, "succeeded");
});

test("does not report success when the approved Research Report is missing", async () => {
  const available = execution({
    async run() {
      return {
        status: "succeeded",
        child: { pid: 42, runId: "run-42" },
        output: "claimed success",
        exit: { code: 0, signal: null },
      };
    },
  });
  const terminal = await runResearcherDelegation("Research", {
    ...available,
    async reportDelivered() {
      return false;
    },
  });

  assert.deepEqual(terminal.outcome, {
    status: "failed",
    child: { pid: 42, runId: "run-42" },
    failure: {
      kind: "missing-research-report",
      message: "researcher did not create the approved Research Report",
    },
    exit: { code: 0, signal: null },
  });
});

test("required researcher web preflight blocks before constructing a runner", async () => {
  const available = execution({
    async run() {
      throw new Error("must not run");
    },
  });
  const terminal = await runResearcherDelegation("Research", {
    ...available,
    availability: {
      ...available.availability,
      web: "unavailable",
    },
    createRunner() {
      throw new Error("must not create a runner");
    },
  });

  assert.deepEqual(terminal.outcome, {
    status: "blocked",
    diagnostic: {
      kind: "capability-preflight",
      contractId: "delegate-researcher",
      unmet: ["required web capability is unavailable"],
    },
  });
  assert.deepEqual(terminal.artifacts, {
    workspace:
      "/validated/tmp/matty/research/123e4567-e89b-42d3-a456-426614174000",
    report: "/trusted/project/docs/research/result.md",
  });
});
