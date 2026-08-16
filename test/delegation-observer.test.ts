import assert from "node:assert/strict";
import { realpath } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { createChildPiRunner } from "../src/application/child-pi-runtime.ts";
import { createDelegationObserver } from "../src/application/delegation-observer.ts";
import { DelegationRegistry } from "../src/application/delegation-registry.ts";
import {
  renderDelegationConsole,
  renderDelegationHumanSnapshot,
  renderDelegationJson,
  renderDelegationWidget,
} from "../src/application/delegation-presentation.ts";

const id = "aaaaaaaa-0000-4000-8000-000000000001";
const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const fixture = resolve(repositoryRoot, "test/fixtures/child-pi-rpc-fixture.mjs");
const canonicalRoot = await realpath(repositoryRoot);

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

test("observer exposes only ordered redacted activity across updates and presentations", () => {
  const secret = "SENSITIVE-command-path-prompt-response-result-transcript-tool-id";
  const registry = new DelegationRegistry({ idFactory: () => id, now: () => 1_000 });
  const updates: unknown[] = [];
  const observer = createDelegationObserver({
    registry,
    declaration: {
      tasks: [
        { role: "explorer", task: secret },
        { role: "worker", task: secret },
      ],
    },
    onUpdate(update) { updates.push(update); },
  });

  observer.observeProgress({
    taskIndex: 1,
    progress: {
      type: "activity",
      child: { pid: 42, runId: secret },
      observation: {
        schemaVersion: 1,
        sequence: 1,
        observedAt: 1_500,
        summary: {
          schemaVersion: 1,
          kind: "tool-completed",
          tool: "read",
          outcome: "succeeded",
          args: secret,
          command: secret,
          path: secret,
          rawResult: secret,
          unknown: secret,
        },
      },
    },
  });
  observer.observeProgress({
    taskIndex: 0,
    progress: {
      type: "activity",
      observation: {
        schemaVersion: 1,
        sequence: 1,
        observedAt: 1_400,
        summary: {
          schemaVersion: 1,
          kind: "assistant-completed",
          outcome: "succeeded",
          response: secret,
        },
      },
    },
  });

  const snapshot = registry.snapshot();
  const consoleOutput = renderDelegationConsole(snapshot, {
    selectedId: observer.id,
    expandedIds: new Set([observer.id]),
  }, 2_000).join("\n");
  const surfaces = JSON.stringify({
    updates,
    snapshot,
    json: renderDelegationJson(snapshot),
    diagnostic: renderDelegationHumanSnapshot(snapshot, 2_000),
    widget: renderDelegationWidget(snapshot, 2_000),
    consoleOutput,
  });

  assert.deepEqual(snapshot.delegations[0]?.tasks.map((task) => task.activities), [
    [{ schemaVersion: 1, sequence: 1, observedAt: 1_400, summary: {
      schemaVersion: 1, kind: "assistant-completed", outcome: "succeeded",
    } }],
    [{ schemaVersion: 1, sequence: 1, observedAt: 1_500, summary: {
      schemaVersion: 1, kind: "tool-completed", tool: "read", outcome: "succeeded",
    } }],
  ]);
  assert.match(consoleOutput, /Timeline:[\s\S]*Lifecycle ·[\s\S]*Activity ·/);
  assert.match(consoleOutput, /Assistant completed · succeeded/);
  assert.match(consoleOutput, /Tool read completed · succeeded/);
  assert.doesNotMatch(surfaces, /SENSITIVE|args|command|path|prompt|response|rawResult|transcript|tool-id/);
});

test("sensitive child fixture is sanitized through runtime, observer, registry, and every renderer", async () => {
  const updates: unknown[] = [];
  const registry = new DelegationRegistry({ idFactory: () => id });
  const observer = createDelegationObserver({
    registry,
    declaration: { tasks: [{ role: "worker", task: "fixture task sentinel" }] },
    onUpdate(update) { updates.push(update); },
  });
  const runner = createChildPiRunner({
    invocation: { command: process.execPath, arguments: [fixture] },
    parent: {
      provider: "controlled-provider",
      model: "controlled-model",
      thinking: "high",
      cwd: canonicalRoot,
    },
    authentication: {
      provider: "controlled-provider",
      environment: { PATH: process.env.PATH, MATTY_TEST_AUTH: "fixture-auth-sentinel" },
    },
  });

  const outcome = await runner.run("sensitive-activity", {
    signal: observer.signal,
    onProgress(progress) { observer.observeProgress({ taskIndex: 0, progress }); },
  });
  assert.equal(outcome.status, "succeeded");
  const sensitiveDiagnostic = {
    kind: "delegation" as const,
    code: "child-failed" as const,
    taskIndex: 0,
    rawResult: "secret raw diagnostic result",
  };
  observer.recordDiagnostic(sensitiveDiagnostic);
  const snapshot = registry.snapshot();
  assert.deepEqual(snapshot.delegations[0]?.diagnostics, [
    { code: "child-failed", taskIndex: 0 },
  ]);
  const state = { selectedId: observer.id, expandedIds: new Set([observer.id]) };
  const surfaces = [
    JSON.stringify(updates),
    JSON.stringify(snapshot),
    renderDelegationJson(snapshot),
    renderDelegationHumanSnapshot(snapshot),
    renderDelegationConsole(snapshot, state).join("\n"),
    renderDelegationWidget(snapshot).join("\n"),
  ];

  for (const surface of surfaces) {
    assert.doesNotMatch(surface, /secret-tool-call-id|another-secret-id|secret raw tool result|secret raw diagnostic result|secret prompt and transcript|fixture-auth-sentinel/i);
    assert.doesNotMatch(surface, /"(?:args|command|path|rawResult|response|result|transcript|toolCallId|unknownSensitiveField)"\s*:/i);
  }
  assert.deepEqual(
    snapshot.delegations[0]?.tasks[0]?.activities.map((activity) => activity.sequence),
    [1, 2, 3],
  );
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
