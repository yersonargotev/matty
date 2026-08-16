import assert from "node:assert/strict";
import test from "node:test";

import { createDelegationObserver } from "../src/application/delegation-observer.ts";
import { DelegationRegistry } from "../src/application/delegation-registry.ts";

const id = "aaaaaaaa-0000-4000-8000-000000000001";

function blocked(unmet: string[]) {
  return {
    contract: null,
    outcome: {
      status: "blocked",
      diagnostic: { kind: "capability-preflight", contractId: "delegate", unmet },
    },
  };
}

test("observer accepts safe task declarations and owns lifecycle/result interpretation", () => {
  let now = 1_000;
  const registry = new DelegationRegistry({ idFactory: () => id, now: () => now });
  const updates: unknown[] = [];
  const observer = createDelegationObserver({
    registry,
    declaration: {
      requirement: "required",
      tasks: [
        { role: "explorer", task: "private first task" },
        { role: "invalid", task: "private second task" },
      ],
    },
    onUpdate(update) { updates.push(update); },
  });

  observer.observeProgress({ taskIndex: 0, progress: { type: "started", child: { pid: 42 } } });
  now = 2_000;
  observer.finish({
    status: "partial",
    tasks: [{ taskIndex: 0, role: "explorer", status: "succeeded", value: "private result" }],
  });

  const entry = registry.get(observer.id)!;
  assert.deepEqual(entry.roles, ["explorer"]);
  assert.equal(entry.taskCount, 2);
  assert.deepEqual(entry.tasks.map((task) => task.role), ["explorer", undefined]);
  assert.equal(entry.state, "partial");
  assert.deepEqual(entry.tasks.map((task) => task.state), ["succeeded", "failed"]);
  assert.match(JSON.stringify(updates), /D-[0-9a-f]{8}/);
  assert.doesNotMatch(
    JSON.stringify({ entry, updates }),
    /private first task|private second task|private result/,
  );
});

test("observer completes each task once and ignores late lifecycle progress", () => {
  let now = 1_000;
  const registry = new DelegationRegistry({ idFactory: () => id, now: () => now });
  const observer = createDelegationObserver({
    registry,
    declaration: {
      requirement: "required",
      tasks: Array.from({ length: 5 }, () => ({ role: "explorer", task: "private" })),
    },
  });
  for (let taskIndex = 0; taskIndex < 4; taskIndex += 1) {
    observer.observeProgress({
      taskIndex,
      progress: { type: "started", child: { pid: 40 + taskIndex } },
    });
  }

  now = 2_000;
  observer.completeTask(0, "failed");
  observer.recordDiagnostic({
    kind: "delegation",
    code: "child-failed",
    taskIndex: 0,
    role: "explorer",
  });
  observer.completeTask(0, "cancelled");
  observer.observeProgress({
    taskIndex: 0,
    progress: { type: "identified", child: { pid: 99, runId: "late" } },
  });
  assert.deepEqual(registry.snapshot().concurrency, { activeTasks: 3, queuedTasks: 1 });
  assert.equal(registry.get(observer.id)?.tasks[0]?.state, "failed");
  assert.equal(registry.get(observer.id)?.tasks[0]?.endedAt, 2_000);
  assert.equal(registry.get(observer.id)?.tasks[0]?.resultSummary, "Failed (child-failed)");

  observer.observeProgress({
    taskIndex: 4,
    progress: { type: "started", child: { pid: 44 } },
  });
  assert.deepEqual(registry.snapshot().concurrency, { activeTasks: 4, queuedTasks: 0 });
});

test("observer stores only closed standalone preflight reasons", () => {
  const cases = [
    ["parent authentication is unavailable: provider secret", "authentication-unavailable"],
    ["independent Subagent Runtime is unavailable", "runtime-unavailable"],
    ["Matty Rules conflict: private policy text", "rules-conflict"],
    ["reviewer requires authenticated GitHub CLI", "github-unavailable"],
    ["unrecognized provider/task detail", "capability-unavailable"],
  ] as const;

  for (const [raw, reason] of cases) {
    const registry = new DelegationRegistry({ idFactory: () => id });
    const observer = createDelegationObserver({
      registry,
      declaration: { role: "reviewer", task: "private task" },
    });
    const finished = observer.finish(blocked([raw]));

    const serialized = JSON.stringify({ snapshot: registry.snapshot(), result: finished.safeDetails });
    assert.equal(registry.get(observer.id)?.diagnostics[0]?.reason, reason);
    assert.equal(registry.get(observer.id)?.resultSummary, `Blocked (${reason})`);
    assert.deepEqual(
      (finished.safeDetails as { outcome: { diagnostic: unknown } }).outcome.diagnostic,
      { kind: "capability-preflight", contractId: "delegate", reason },
    );
    assert.doesNotMatch(serialized, /provider secret|private policy|private task|unrecognized/);
  }
});
