import assert from "node:assert/strict";
import test from "node:test";

import {
  childTranscript,
  type DelegatedTaskRunner,
} from "../src/application/child-pi-runtime.ts";
import {
  runWorkerDelegation,
  type WorkerDelegationExecution,
} from "../src/application/worker-delegation.ts";
import {
  WORKER_TOOLS,
  createWorkerCapabilityContract,
} from "../src/domain/capability-contract.ts";
import { workerCompletionReport } from "../src/domain/worker-completion.ts";
import { createRoleSeamChildRunner } from "./support/child-pi-runner.ts";

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
          output: JSON.stringify({
            schemaVersion: 1,
            summary: "implementation complete",
            changedPaths: ["src/example.ts"],
            checks: [{ command: "project check", status: "passed" }],
            evidenceRole: "supporting-only-parent-verification-required",
            reportedFullGate: { status: "not-run" },
          }),
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

test("preserves the private transcript when an invalid worker report becomes a failure", async () => {
  const source = await createRoleSeamChildRunner().run("success");
  assert.equal(source.status, "succeeded");

  const terminal = await runWorkerDelegation(
    "Implement",
    execution({ async run() { return source; } }),
  );

  assert.equal(terminal.outcome.status, "failed");
  assert.deepEqual(
    childTranscript(terminal.outcome)?.entries.map((entry) => entry.type),
    ["message_end", "agent_settled"],
  );
  assert.doesNotMatch(JSON.stringify(terminal.outcome), /transcript/);
});

test("worker completion is closed and cannot claim an authoritative gate", () => {
  assert.throws(() => workerCompletionReport({
    schemaVersion: 1,
    summary: "done",
    changedPaths: [],
    checks: [],
    evidenceRole: "supporting-only-parent-verification-required",
    reportedFullGate: { status: "passed", command: "npm run check" },
    authoritativeGate: { status: "passed" },
  }));
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
