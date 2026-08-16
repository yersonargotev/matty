import assert from "node:assert/strict";
import test from "node:test";

import {
  DELEGATION_STATES,
  DelegationRegistry,
  type DelegationState,
} from "../src/application/delegation-registry.ts";
import {
  delegationCard,
  renderDelegationConsole,
  renderDelegationHumanSnapshot,
  renderDelegationJson,
} from "../src/application/delegation-presentation.ts";

function uuid(index: number): string {
  return `${index.toString(16).padStart(8, "0")}-0000-4000-8000-000000000000`;
}

test("registry assigns opaque UUIDs and unique display IDs without retaining declarations", () => {
  const ids = [
    "aaaaaaaa-0000-4000-8000-000000000001",
    "aaaaaaaa-0000-4000-8000-000000000002",
    "bbbbbbbb-0000-4000-8000-000000000003",
  ];
  const registry = new DelegationRegistry({ idFactory: () => ids.shift()!, now: () => 10 });
  registry.accept({
    requirement: "required",
    tasks: Array.from({ length: 9 }, () => ({ role: "explorer" as const })),
  });
  registry.finish(registry.snapshot().delegations[0]!.id, "blocked");
  registry.accept({ tasks: [{ role: "worker" }] });

  const serialized = JSON.stringify(registry.snapshot());
  const entries = registry.snapshot().delegations;
  assert.equal(entries.length, 2);
  assert.equal(new Set(entries.map((entry) => entry.id)).size, 2);
  assert.equal(new Set(entries.map((entry) => entry.displayId)).size, 2);
  for (const entry of entries) {
    assert.match(entry.id, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-8[0-9a-f]{3}-[0-9a-f]{12}$/);
    assert.match(entry.displayId, /^D-[0-9a-f]{8}$/);
  }
  assert.doesNotMatch(serialized, /prompt|command|argument|response|transcript|secret task/i);
});

test("registry reserves opaque and display IDs for the whole session", () => {
  const ids = [
    "aaaaaaaa-0000-4000-8000-000000000001",
    "aaaaaaaa-0000-4000-8000-000000000002",
    "bbbbbbbb-0000-4000-8000-000000000003",
    "aaaaaaaa-0000-4000-8000-000000000001",
    "cccccccc-0000-4000-8000-000000000004",
  ];
  const registry = new DelegationRegistry({
    idFactory: () => ids.shift()!,
    terminalLimit: 0,
  });
  const first = registry.accept({ tasks: [{ role: "explorer" }] });
  registry.finish(first.id, "succeeded");
  assert.equal(registry.snapshot().delegations.length, 0);

  const second = registry.accept({ tasks: [{ role: "designer" }] });
  assert.equal(second.id, "bbbbbbbb-0000-4000-8000-000000000003");
  assert.equal(second.displayId, "D-bbbbbbbb");

  registry.reset();
  const afterReset = registry.accept({ tasks: [{ role: "worker" }] });
  assert.equal(afterReset.id, first.id);
  assert.equal(afterReset.displayId, first.displayId);
});

test("registry retains active entries and only the newest fifty terminal entries", () => {
  let next = 1;
  const registry = new DelegationRegistry({ idFactory: () => uuid(next++), terminalLimit: 50 });
  const active = registry.accept({ tasks: [{ role: "explorer" }] });
  registry.record(active.id, { type: "started", taskIndex: 0, pid: 42 });
  for (let index = 0; index < 55; index += 1) {
    const entry = registry.accept({ tasks: [{ role: "designer" }] });
    registry.finish(entry.id, "succeeded");
  }
  const snapshot = registry.snapshot();
  assert.equal(snapshot.delegations.length, 51);
  assert.equal(snapshot.delegations[0]?.id, active.id);
  assert.equal(snapshot.delegations.filter((entry) => entry.state === "succeeded").length, 50);
});

test("registry represents every state, cancellation, task outcomes, counts, and first terminal wins", () => {
  let now = 1_000;
  let next = 1;
  const registry = new DelegationRegistry({ now: () => now, idFactory: () => uuid(next++) });
  const queued = registry.accept({
    tasks: [{ role: "explorer" }, { role: "worker" }],
  });
  assert.equal(registry.snapshot().concurrency.queuedTasks, 2);
  registry.record(queued.id, { type: "started", taskIndex: 0, pid: 12 });
  registry.record(queued.id, { type: "identified", taskIndex: 0, pid: 12, runId: "run-12" });
  assert.equal(registry.get(queued.id)?.state, "running");
  registry.record(queued.id, { type: "terminating", taskIndex: 0, pid: 12, runId: "run-12" });
  assert.equal(registry.get(queued.id)?.state, "cancelling");
  registry.record(queued.id, { type: "started", taskIndex: 1, pid: 13 });
  assert.equal(registry.get(queued.id)?.state, "cancelling");
  assert.deepEqual(registry.snapshot().concurrency, { activeTasks: 2, queuedTasks: 0 });
  now = 2_000;
  registry.finish(queued.id, "partial", new Map([[0, "cancelled"], [1, "succeeded"]]));
  registry.finish(queued.id, "failed");
  assert.equal(registry.get(queued.id)?.state, "partial");
  assert.deepEqual(registry.get(queued.id)?.tasks.map((task) => task.state), ["cancelled", "succeeded"]);

  const observed = new Set<DelegationState>(["queued", "running", "cancelling", "partial"]);
  for (const state of ["blocked", "succeeded", "failed", "cancelled"] as const) {
    const entry = registry.accept({ tasks: [{ role: "designer" }] });
    registry.finish(entry.id, state);
    observed.add(registry.get(entry.id)!.state);
  }
  assert.deepEqual([...observed].sort(), [...DELEGATION_STATES].sort());
});

test("registry cancellation is one transition, aborts owned work, and first terminal wins", () => {
  let aborts = 0;
  const controller = new AbortController();
  controller.signal.addEventListener("abort", () => { aborts += 1; });
  const registry = new DelegationRegistry({ idFactory: () => uuid(1), now: () => 1_000 });
  const entry = registry.accept({
    requirement: "required",
    tasks: Array.from({ length: 5 }, () => ({ role: "explorer" as const })),
    maxActive: 4,
  }, controller);
  for (let taskIndex = 0; taskIndex < 4; taskIndex += 1) {
    registry.record(entry.id, { type: "started", taskIndex, pid: 40 + taskIndex });
  }

  assert.equal(registry.cancel(entry.id), "cancelling");
  assert.equal(registry.get(entry.id)?.state, "cancelling");
  assert.equal(aborts, 1);
  assert.equal(registry.cancel(entry.id), "already-cancelling");
  assert.equal(aborts, 1);

  registry.finish(entry.id, "succeeded");
  registry.finish(entry.id, "cancelled");
  assert.equal(registry.get(entry.id)?.state, "succeeded");
  assert.equal(registry.cancel(entry.id), "already-finished");
  assert.equal(registry.cancel("missing"), "already-finished");
});

test("registry releases an active slot when a task completes before queue promotion", () => {
  let now = 1_000;
  const registry = new DelegationRegistry({ now: () => now, idFactory: () => uuid(1) });
  const entry = registry.accept({
    tasks: Array.from({ length: 5 }, () => ({ role: "explorer" as const })),
    maxActive: 4,
  });
  for (let taskIndex = 0; taskIndex < 4; taskIndex += 1) {
    registry.record(entry.id, { type: "started", taskIndex, pid: 40 + taskIndex });
  }

  now = 2_000;
  registry.finishTask(entry.id, 0, "succeeded");
  assert.deepEqual(registry.snapshot().concurrency, { activeTasks: 3, queuedTasks: 1 });
  assert.equal(registry.get(entry.id)?.state, "running");
  assert.deepEqual(registry.get(entry.id)?.tasks[0], {
    index: 0,
    role: "explorer",
    state: "succeeded",
    queuedAt: 1_000,
    startedAt: 1_000,
    endedAt: 2_000,
    pid: 40,
    resultSummary: "Succeeded",
  });

  registry.record(entry.id, { type: "started", taskIndex: 4, pid: 44 });
  assert.deepEqual(registry.snapshot().concurrency, { activeTasks: 4, queuedTasks: 0 });
});

test("registry keeps the first task terminal state and late progress cannot reopen it", () => {
  let now = 1_000;
  const registry = new DelegationRegistry({ now: () => now, idFactory: () => uuid(1) });
  const entry = registry.accept({
    tasks: [{ role: "explorer" }, { role: "designer" }],
    maxActive: 1,
  });
  registry.record(entry.id, { type: "started", taskIndex: 0, pid: 40 });
  registry.recordDiagnostic(entry.id, {
    kind: "delegation",
    code: "task-failed",
    taskIndex: 0,
    role: "explorer",
  });

  now = 2_000;
  registry.finishTask(entry.id, 0, "failed");
  now = 3_000;
  registry.finishTask(entry.id, 0, "cancelled");
  registry.record(entry.id, { type: "identified", taskIndex: 0, pid: 99, runId: "late" });

  const task = registry.get(entry.id)?.tasks[0];
  assert.equal(task?.state, "failed");
  assert.equal(task?.endedAt, 2_000);
  assert.equal(task?.pid, 40);
  assert.equal(task?.runId, undefined);
  assert.equal(task?.resultSummary, "Failed (task-failed)");
  assert.equal(registry.get(entry.id)?.state, "queued");

  registry.finishTask(entry.id, 1, "cancelled");
  registry.finishTask(entry.id, 1, "failed");
  assert.equal(registry.get(entry.id)?.tasks[1]?.state, "cancelled");
  assert.equal(registry.get(entry.id)?.state, "queued");
  registry.finish(entry.id, "cancelled", new Map([[0, "cancelled"], [1, "failed"]]));
  assert.deepEqual(registry.get(entry.id)?.tasks.map((candidate) => candidate.state), [
    "failed",
    "cancelled",
  ]);
});

test("registry tracks bounded queue positions and clears them as tasks are promoted", () => {
  let now = 1_000;
  const registry = new DelegationRegistry({ now: () => now, idFactory: () => uuid(1) });
  const entry = registry.accept({
    tasks: ["explorer", "explorer", "explorer", "explorer", "designer", "reviewer"]
      .map((role) => ({ role: role as "explorer" | "designer" | "reviewer" })),
    maxActive: 4,
  });

  assert.deepEqual(
    registry.get(entry.id)?.tasks.map((task) => task.queuePosition),
    [undefined, undefined, undefined, undefined, 1, 2],
  );
  now = 2_000;
  registry.record(entry.id, { type: "started", taskIndex: 4, pid: 44 });
  assert.deepEqual(
    registry.get(entry.id)?.tasks.map((task) => task.queuePosition),
    [undefined, undefined, undefined, undefined, undefined, 1],
  );
});

test("expanded task details show safe lifecycle timing, duration, diagnostics, and results", () => {
  let now = Date.parse("2026-02-01T12:00:00.000Z");
  const secret = "raw unmet provider secret and delegated task text";
  const registry = new DelegationRegistry({ now: () => now, idFactory: () => uuid(1) });
  const entry = registry.accept({ tasks: [{ role: "explorer" }], maxActive: 4 });
  now += 1_000;
  registry.record(entry.id, { type: "started", taskIndex: 0, pid: 42 });
  now += 2_500;
  registry.recordDiagnostic(entry.id, {
    kind: "delegation",
    code: "task-failed",
    taskIndex: 0,
    role: "explorer",
    phase: "running",
    reason: secret,
    arbitraryError: secret,
  } as never);
  registry.finish(entry.id, "failed", new Map([[0, "failed"]]));

  const snapshot = registry.snapshot();
  const lines = renderDelegationConsole(snapshot, {
    selectedId: entry.id,
    expandedIds: new Set([entry.id]),
  }, now).join("\n");
  assert.match(lines, /Queued: 2026-02-01T12:00:00\.000Z/);
  assert.match(lines, /Started: 2026-02-01T12:00:01\.000Z/);
  assert.match(lines, /Ended: 2026-02-01T12:00:03\.500Z/);
  assert.match(lines, /Duration: 2s/);
  assert.match(lines, /Result: Failed \(task-failed · running\)/);
  assert.doesNotMatch(JSON.stringify(snapshot), new RegExp(secret));
});

test("presentation groups safely and exposes PID and runId only in expanded details", () => {
  let next = 1;
  const registry = new DelegationRegistry({ idFactory: () => uuid(next++), now: () => 1_000 });
  const active = registry.accept({ tasks: [{ role: "worker" }] });
  registry.record(active.id, { type: "identified", taskIndex: 0, pid: 321, runId: "run-safe" });
  const cancelling = registry.accept({ tasks: [{ role: "explorer" }] });
  registry.record(cancelling.id, { type: "terminating", taskIndex: 0, pid: 22, runId: "run-cancel" });
  registry.accept({ tasks: [{ role: "designer" }] });
  const recent = registry.accept({ tasks: [{ role: "reviewer" }] });
  registry.finish(recent.id, "blocked");
  const snapshot = registry.snapshot();

  const compact = delegationCard(snapshot.delegations[0]!, 4_000);
  assert.match(compact, /D-[0-9a-f]{8} (running|cancelling) · (worker|explorer) · 1 task · 3s/);
  assert.doesNotMatch(compact, /321|run-safe/);
  const lines = renderDelegationConsole(snapshot, { selectedId: active.id, expandedIds: new Set([active.id]) }, 4_000);
  assert.ok(lines.indexOf("Active / Cancelling:") < lines.indexOf("Queued:"));
  assert.ok(lines.indexOf("Queued:") < lines.indexOf("Recent:"));
  assert.match(lines.join("\n"), /PID 321 · runId run-safe/);
  assert.doesNotMatch(lines.join("\n"), /task text|prompt|args|command|response|transcript/i);
  assert.equal(JSON.parse(renderDelegationJson(snapshot)).schemaVersion, 1);
  assert.match(renderDelegationHumanSnapshot(snapshot, 4_000), /Matty delegations \(session only\)/);
});
