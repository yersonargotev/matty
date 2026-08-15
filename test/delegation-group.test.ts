import assert from "node:assert/strict";
import test from "node:test";

import {
  validateDelegationGroupContract,
} from "../src/domain/delegation-group.ts";
import {
  runDelegationGroup,
  type DelegationDiagnostic,
  type DelegationTaskExecution,
} from "../src/application/delegation-scheduler.ts";

function requiredGroup(taskCount: number) {
  return {
    schemaVersion: 1 as const,
    id: "delegate-group" as const,
    requirement: "required" as const,
    fallback: "none" as const,
    atomic: true,
    cardinality: { min: 1 as const, max: 8 as const },
    concurrency: { maxActive: 4 as const },
    independence: "required" as const,
    tasks: Array.from({ length: taskCount }, (_, index) => ({
      role: "explorer" as const,
      task: `bounded task ${index + 1}`,
    })),
  };
}

function optionalInspectionGroup(taskCount: number) {
  return {
    ...requiredGroup(taskCount),
    requirement: "optional" as const,
    fallback: "skip" as const,
    atomic: false,
  };
}

test("validates a required atomic delegation group of at most eight tasks", () => {
  const contract = {
    schemaVersion: 1,
    id: "delegate-group",
    requirement: "required",
    fallback: "none",
    atomic: true,
    cardinality: { min: 1, max: 8 },
    concurrency: { maxActive: 4 },
    independence: "required",
    tasks: Array.from({ length: 8 }, (_, index) => ({
      role: index === 0 ? "worker" : "explorer",
      task: `bounded task ${index + 1}`,
    })),
  };

  assert.deepEqual(validateDelegationGroupContract(contract), {
    ok: true,
    contract,
  });
});

test("rejects a ninth task without echoing task content", () => {
  const secret = "never include this prompt in a diagnostic";
  const result = validateDelegationGroupContract({
    schemaVersion: 1,
    id: "delegate-group",
    requirement: "required",
    fallback: "none",
    atomic: true,
    cardinality: { min: 1, max: 8 },
    concurrency: { maxActive: 4 },
    independence: "required",
    tasks: Array.from({ length: 9 }, () => ({
      role: "explorer",
      task: secret,
    })),
  });

  assert.deepEqual(result, {
    ok: false,
    errors: [{ code: "task-limit-exceeded" }],
  });
  assert.doesNotMatch(JSON.stringify(result), new RegExp(secret));
});

test("centrally rejects parallel writers and web on a non-researcher role", () => {
  const result = validateDelegationGroupContract({
    ...requiredGroup(4),
    tasks: [
      { role: "worker", task: "first writer" },
      { role: "worker", task: "second writer" },
      { role: "explorer", task: "inspect", web: "required" },
      { role: "researcher", task: "research without a web declaration" },
    ],
  });

  assert.deepEqual(result, {
    ok: false,
    errors: [
      { code: "web-role-incompatible", taskIndex: 2 },
      { code: "researcher-web-required", taskIndex: 3 },
      { code: "single-writer-required" },
    ],
  });
});

test("reviewer requires one closed Review Scope Contract", () => {
  const reviewScope = {
    schemaVersion: 1,
    issue: { repository: "github.com/acme/repo", number: 36, reference: "#36" },
    requirements: ["Keep review scope closed"],
    outOfScope: [{ reference: "#42", reason: "dependent publication behavior" }],
    baseSha: "0000000000000000000000000000000000000000",
    candidateSha: "1111111111111111111111111111111111111111",
    axes: ["spec"],
  };
  const missing = validateDelegationGroupContract({
    ...requiredGroup(1),
    tasks: [{ role: "reviewer", task: "review" }],
  });
  assert.deepEqual(missing, {
    ok: false,
    errors: [{ code: "invalid-task", taskIndex: 0 }],
  });

  const valid = {
    ...requiredGroup(1),
    tasks: [{ role: "reviewer", task: "review", reviewScope }],
  };
  assert.deepEqual(validateDelegationGroupContract(valid), { ok: true, contract: valid });
  assert.deepEqual(validateDelegationGroupContract({
    ...valid,
    tasks: [{ ...valid.tasks[0], reviewScope: { ...reviewScope, expansion: "forbidden" } }],
  }), {
    ok: false,
    errors: [{ code: "invalid-task", taskIndex: 0 }],
  });
});

test("optional fallback is limited to non-writing inspection groups", () => {
  const result = validateDelegationGroupContract({
    ...optionalInspectionGroup(1),
    tasks: [{ role: "worker", task: "do not silently skip writes" }],
  });

  assert.deepEqual(result, {
    ok: false,
    errors: [{ code: "optional-role-incompatible", taskIndex: 0 }],
  });
});

test("rejects ambiguous group and task fields", () => {
  const result = validateDelegationGroupContract({
    ...requiredGroup(1),
    undeclaredPolicy: "inline",
    tasks: [{
      role: "explorer",
      task: "inspect",
      tools: ["write"],
    }],
  });

  assert.deepEqual(result, {
    ok: false,
    errors: [
      { code: "invalid-contract" },
      { code: "invalid-task", taskIndex: 0 },
    ],
  });
});

test("rejects two researchers targeting the same durable report", () => {
  const result = validateDelegationGroupContract({
    ...requiredGroup(2),
    tasks: [
      {
        role: "researcher",
        task: "first research task",
        web: "required",
        report: "/project/docs/research/shared.md",
      },
      {
        role: "researcher",
        task: "second research task",
        web: "optional",
        report: "/project/docs/research/shared.md",
      },
    ],
  });

  assert.deepEqual(result, {
    ok: false,
    errors: [{ code: "research-report-conflict" }],
  });
});

test("runs at most four children and reports excess accepted work as queued", async () => {
  let active = 0;
  let maximumActive = 0;
  const diagnostics: DelegationDiagnostic[] = [];

  const result = await runDelegationGroup(requiredGroup(8), {
    async preflight() {
      return { ok: true };
    },
    async run(_task, taskIndex) {
      active += 1;
      maximumActive = Math.max(maximumActive, active);
      await new Promise<void>((resolve) => setImmediate(resolve));
      active -= 1;
      return { status: "succeeded", value: taskIndex };
    },
  }, {
    onDiagnostic(diagnostic) {
      diagnostics.push(diagnostic);
    },
  });

  assert.equal(maximumActive, 4);
  assert.deepEqual(
    diagnostics.filter((diagnostic) => diagnostic.code === "queued"),
    [4, 5, 6, 7].map((taskIndex) => ({
      kind: "delegation",
      code: "queued",
      taskIndex,
      role: "explorer",
    })),
  );
  assert.equal(result.status, "succeeded");
  assert.deepEqual(
    result.tasks.map((task) => task.status),
    Array.from({ length: 8 }, () => "succeeded"),
  );
});

test("preflights the entire required group before starting any child", async () => {
  let runCount = 0;

  const result = await runDelegationGroup(requiredGroup(3), {
    async preflight(_task, taskIndex) {
      return taskIndex === 1
        ? { ok: false, reason: "capability-unavailable" }
        : { ok: true };
    },
    async run() {
      runCount += 1;
      return { status: "succeeded", value: "must not run" };
    },
  });

  assert.equal(runCount, 0);
  assert.equal(result.status, "blocked");
  assert.deepEqual(
    result.diagnostics.map((diagnostic) => ({
      code: diagnostic.code,
      taskIndex: diagnostic.taskIndex,
      phase: diagnostic.phase,
      reason: diagnostic.reason,
    })),
    [
      {
        code: "cancelled",
        taskIndex: 0,
        phase: "before-spawn",
        reason: undefined,
      },
      {
        code: "preflight-failed",
        taskIndex: 1,
        phase: undefined,
        reason: "capability-unavailable",
      },
      {
        code: "cancelled",
        taskIndex: 2,
        phase: "before-spawn",
        reason: undefined,
      },
    ],
  );
});

test("an optional inspection group reports unavailable work as skipped", async () => {
  const result = await runDelegationGroup(optionalInspectionGroup(2), {
    async preflight(_task, taskIndex) {
      return taskIndex === 0
        ? { ok: false, reason: "capability-unavailable" }
        : { ok: true };
    },
    async run(_task, taskIndex) {
      return { status: "succeeded", value: taskIndex };
    },
  });

  assert.equal(result.status, "partial");
  assert.deepEqual(
    result.tasks.map((task) => task.status),
    ["skipped", "succeeded"],
  );
  assert.deepEqual(result.diagnostics, [{
    kind: "delegation",
    code: "skipped",
    taskIndex: 0,
    role: "explorer",
    phase: "before-spawn",
    reason: "capability-unavailable",
  }]);
});

test("skipped optional work does not create a false queue diagnostic", async () => {
  const result = await runDelegationGroup(optionalInspectionGroup(5), {
    async preflight(_task, taskIndex) {
      return taskIndex === 0
        ? { ok: false, reason: "capability-unavailable" }
        : { ok: true };
    },
    async run(_task, taskIndex) {
      await new Promise<void>((resolve) => setImmediate(resolve));
      return { status: "succeeded", value: taskIndex };
    },
  });

  assert.equal(
    result.diagnostics.some((diagnostic) => diagnostic.code === "queued"),
    false,
  );
});

test("a required group failure cancels running and queued work", async () => {
  const started: number[] = [];

  const result = await runDelegationGroup(requiredGroup(6), {
    async preflight() {
      return { ok: true };
    },
    async run(_task, taskIndex, { signal }) {
      started.push(taskIndex);
      if (taskIndex === 0) {
        await new Promise<void>((resolve) => setImmediate(resolve));
        return { status: "failed" };
      }
      return await new Promise<DelegationTaskExecution<number>>((resolve) => {
        const timeout = setTimeout(
          () => resolve({ status: "succeeded" as const, value: taskIndex }),
          25,
        );
        signal.addEventListener("abort", () => {
          clearTimeout(timeout);
          resolve({ status: "cancelled" as const });
        }, { once: true });
      });
    },
  });

  assert.deepEqual(started, [0, 1, 2, 3]);
  assert.equal(result.status, "failed");
  assert.deepEqual(
    result.tasks.map((task) => task.status),
    ["failed", "cancelled", "cancelled", "cancelled", "cancelled", "cancelled"],
  );
  assert.ok(
    result.diagnostics.some((diagnostic) =>
      diagnostic.code === "partial-failure" &&
      diagnostic.taskIndex === undefined
    ),
  );
  assert.deepEqual(
    result.tasks.slice(4).map((task) =>
      "diagnostic" in task ? task.diagnostic.phase : undefined
    ),
    ["before-spawn", "before-spawn"],
  );
});

test("scheduler preserves closed-allowlist child failure codes", async () => {
  for (
    const code of [
      "child-failed",
      "protocol-failed",
      "child-exited",
      "missing-research-report",
    ] as const
  ) {
    const result = await runDelegationGroup(requiredGroup(1), {
      async preflight() {
        return { ok: true };
      },
      async run() {
        return { status: "failed", code };
      },
    });

    assert.equal(result.status, "failed");
    assert.equal(result.tasks[0]?.status, "failed");
    assert.equal(
      result.tasks[0] && "diagnostic" in result.tasks[0]
        ? result.tasks[0].diagnostic.code
        : undefined,
      code,
    );
  }
});

test("scheduler redacts unknown task failure codes", async () => {
  const secret = "provider-specific-secret-code";
  const result = await runDelegationGroup(requiredGroup(1), {
    async preflight() {
      return { ok: true };
    },
    async run() {
      return {
        status: "failed",
        code: secret,
      } as unknown as DelegationTaskExecution<never>;
    },
  });

  assert.equal(
    result.tasks[0] && "diagnostic" in result.tasks[0]
      ? result.tasks[0].diagnostic.code
      : undefined,
    "task-failed",
  );
  assert.doesNotMatch(JSON.stringify(result), new RegExp(secret));
});

test("scheduler exceptions become redacted task diagnostics", async () => {
  const secret = "provider stderr and prompt content";
  const result = await runDelegationGroup(requiredGroup(1), {
    async preflight() {
      return { ok: true };
    },
    async run() {
      throw new Error(secret);
    },
  });

  assert.equal(result.status, "failed");
  assert.deepEqual(result.diagnostics, [
    {
      kind: "delegation",
      code: "task-failed",
      taskIndex: 0,
      role: "explorer",
    },
    { kind: "delegation", code: "partial-failure" },
  ]);
  assert.doesNotMatch(JSON.stringify(result), new RegExp(secret));
});

test("a cancelled required task cancels running and queued work", async () => {
  const started: number[] = [];

  const result = await runDelegationGroup(requiredGroup(5), {
    async preflight() {
      return { ok: true };
    },
    async run(_task, taskIndex, { signal }) {
      started.push(taskIndex);
      if (taskIndex === 0) {
        await new Promise<void>((resolve) => setImmediate(resolve));
        return { status: "cancelled" };
      }
      return await new Promise<DelegationTaskExecution<number>>((resolve) => {
        const timeout = setTimeout(
          () => resolve({ status: "succeeded", value: taskIndex }),
          25,
        );
        signal.addEventListener("abort", () => {
          clearTimeout(timeout);
          resolve({ status: "cancelled" });
        }, { once: true });
      });
    },
  });

  assert.deepEqual(started, [0, 1, 2, 3]);
  assert.equal(result.status, "cancelled");
  assert.ok(result.tasks.every((task) => task.status === "cancelled"));
});

test("preflight exceptions block a required group without leaking details", async () => {
  const secret = "authentication failure details";
  const result = await runDelegationGroup(requiredGroup(1), {
    async preflight() {
      throw new Error(secret);
    },
    async run() {
      return { status: "succeeded", value: "must not run" };
    },
  });

  assert.equal(result.status, "blocked");
  assert.deepEqual(result.diagnostics, [{
    kind: "delegation",
    code: "preflight-failed",
    taskIndex: 0,
    role: "explorer",
    reason: "capability-unavailable",
  }]);
  assert.doesNotMatch(JSON.stringify(result), new RegExp(secret));
});

test("an already-cancelled group never starts preflight or child work", async () => {
  const controller = new AbortController();
  controller.abort();
  let preflightCount = 0;
  let runCount = 0;

  const result = await runDelegationGroup(requiredGroup(2), {
    async preflight() {
      preflightCount += 1;
      return { ok: true };
    },
    async run() {
      runCount += 1;
      return { status: "succeeded", value: "must not run" };
    },
  }, { signal: controller.signal });

  assert.equal(preflightCount, 0);
  assert.equal(runCount, 0);
  assert.equal(result.status, "cancelled");
  assert.deepEqual(
    result.tasks.map((task) => task.status),
    ["cancelled", "cancelled"],
  );
  assert.ok(result.diagnostics.every((diagnostic) =>
    diagnostic.code === "cancelled" &&
    diagnostic.phase === "before-spawn"
  ));
});

test("cancellation during preflight prevents every child from spawning", async () => {
  const controller = new AbortController();
  let runCount = 0;

  const result = await runDelegationGroup(requiredGroup(2), {
    async preflight(_task, taskIndex) {
      if (taskIndex === 0) {
        controller.abort();
      }
      return { ok: true };
    },
    async run() {
      runCount += 1;
      return { status: "succeeded", value: "must not run" };
    },
  }, { signal: controller.signal });

  assert.equal(runCount, 0);
  assert.equal(result.status, "cancelled");
  assert.ok(result.tasks.every((task) => task.status === "cancelled"));
});
