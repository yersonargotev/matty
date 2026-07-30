import assert from "node:assert/strict";
import test from "node:test";

import type { DelegatedTaskRunner } from "../src/application/child-pi-runtime.ts";
import {
  runWorkerDelegation,
  type WorkerDelegationExecution,
} from "../src/application/worker-delegation.ts";
import {
  WORKER_TOOLS,
  createWorkerCapabilityContract,
} from "../src/domain/capability-contract.ts";

function execution(
  runner: DelegatedTaskRunner,
): WorkerDelegationExecution {
  return {
    contract: createWorkerCapabilityContract({
      workingTree: "/trusted/project",
      temporaryPaths: ["/validated/tmp/session"],
    }),
    availability: {
      availableTools: WORKER_TOOLS,
      independentRuntime: true,
      inspectionGuard: false,
      workerGuard: true,
    },
    acquireWriter() {
      return () => {};
    },
    createRunner() {
      return runner;
    },
  };
}

test("runs a worker over the trusted tree through the guarded runtime", async () => {
  let observedTask = "";
  const terminal = await runWorkerDelegation(
    "Implement the requested behavior",
    execution({
      async run(task) {
        observedTask = task;
        return {
          status: "succeeded",
          child: { pid: 42, runId: "run-42" },
          output: "implementation complete",
          exit: { code: 0, signal: null },
        };
      },
    }),
  );

  assert.match(observedTask, /Worker assignment/);
  assert.match(observedTask, /trusted working tree: \/trusted\/project/);
  assert.match(observedTask, /validated temporary paths: \/validated\/tmp\/session/);
  assert.match(observedTask, /parent reviews and integrates all changes/);
  assert.equal(terminal.contract.role, "worker");
  assert.equal(terminal.outcome.status, "succeeded");
});

test("worker preflight rejects parallel writers before constructing a runner", async () => {
  const available = execution({
    async run() {
      throw new Error("must not run");
    },
  });
  const terminal = await runWorkerDelegation("Implement", {
    ...available,
    contract: {
      ...available.contract,
      concurrency: { maxActive: 2 },
    } as never,
    createRunner() {
      throw new Error("must not create a runner");
    },
  });

  assert.deepEqual(terminal.outcome, {
    status: "blocked",
    diagnostic: {
      kind: "capability-preflight",
      contractId: "delegate-worker",
      unmet: ["worker contract requires one writer"],
    },
  });
});
