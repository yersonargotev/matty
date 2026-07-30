import assert from "node:assert/strict";
import test from "node:test";

import {
  runExplorerDelegation,
  type ExplorerDelegationExecution,
} from "../src/application/explorer-delegation.ts";
import type {
  DelegatedTaskProgress,
  DelegatedTaskRunner,
} from "../src/application/child-pi-runtime.ts";
import { EXPLORER_TOOLS } from "../src/domain/capability-contract.ts";

function availableExecution(
  runner: DelegatedTaskRunner,
): ExplorerDelegationExecution {
  return {
    availability: {
      availableTools: EXPLORER_TOOLS,
      independentRuntime: true,
      inspectionGuard: true,
    },
    createRunner() {
      return runner;
    },
  };
}

test("runs one explorer through the delegated task runner with structured output", async () => {
  const progress: DelegatedTaskProgress[] = [];
  const observedTasks: string[] = [];
  const runner: DelegatedTaskRunner = {
    async run(task, options) {
      observedTasks.push(task);
      const event: DelegatedTaskProgress = {
        type: "started",
        child: { pid: 42 },
      };
      options?.onProgress?.(event);
      return {
        status: "succeeded",
        child: { pid: 42, runId: "run-42" },
        output: "explorer findings",
        exit: { code: 0, signal: null },
      };
    },
  };

  const terminal = await runExplorerDelegation(
    "Inspect the runtime",
    availableExecution(runner),
    {
      onProgress(event) {
        progress.push(event);
      },
    },
  );

  assert.match(observedTasks[0] ?? "", /Explorer assignment/);
  assert.match(observedTasks[0] ?? "", /Inspect the runtime/);
  assert.deepEqual(progress.map((event) => event.type), ["started"]);
  assert.equal(terminal.contract.id, "delegate-explorer");
  assert.equal(terminal.contract.role, "explorer");
  assert.equal(terminal.outcome.status, "succeeded");
  if (terminal.outcome.status === "succeeded") {
    assert.equal(terminal.outcome.output, "explorer findings");
  }
});

test("an unmet preflight blocks only that invocation and is diagnosable", async () => {
  let runnerCalls = 0;
  const runner: DelegatedTaskRunner = {
    async run() {
      runnerCalls += 1;
      return {
        status: "succeeded",
        child: { pid: 7, runId: "run-7" },
        output: "ok",
        exit: { code: 0, signal: null },
      };
    },
  };

  const blocked = await runExplorerDelegation("Inspect", {
    availability: {
      availableTools: EXPLORER_TOOLS,
      independentRuntime: false,
      inspectionGuard: true,
    },
    createRunner() {
      throw new Error("must not create a runner");
    },
  });
  assert.deepEqual(blocked.outcome, {
    status: "blocked",
    diagnostic: {
      kind: "capability-preflight",
      contractId: "delegate-explorer",
      unmet: ["independent Subagent Runtime is unavailable"],
    },
  });

  const next = await runExplorerDelegation(
    "Inspect",
    availableExecution(runner),
  );
  assert.equal(next.outcome.status, "succeeded");
  assert.equal(runnerCalls, 1);
});
