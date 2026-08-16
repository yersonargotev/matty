import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readdir, realpath, rm } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  childTerminalResponses,
  childTranscript,
  createChildPiRunner,
  type DelegatedTaskPresentation,
  type DelegatedTaskProgress,
} from "../src/application/child-pi-runtime.ts";
import { WORKER_TOOLS } from "../src/domain/capability-contract.ts";
import { CHILD_EXECUTION_TOOL_CATEGORIES_V1 } from "../src/domain/child-execution-activity.ts";

const repositoryRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "..",
);
const fixture = resolve(
  repositoryRoot,
  "test/fixtures/child-pi-rpc-fixture.mjs",
);
const canonicalRoot = await realpath(repositoryRoot);
const authMarker = "child-safe-auth-marker";
const authDigest = createHash("sha256").update(authMarker).digest("hex");

function createRunner(
  terminationGraceMs = 1_000,
  authenticationProvider = "controlled-provider",
  lifecycle: {
    beforeInteraction?: () => Promise<boolean>;
    onTerminalResponse?: (text: string) => void;
  } = {},
) {
  return createChildPiRunner({
    invocation: {
      command: process.execPath,
      arguments: [fixture, "--tools", "read,grep"],
    },
    parent: {
      provider: "controlled-provider",
      model: "controlled-model",
      thinking: "high",
      cwd: canonicalRoot,
    },
    authentication: {
      provider: authenticationProvider,
      environment: {
        PATH: process.env.PATH,
        MATTY_TEST_AUTH: authMarker,
      },
    },
    terminationGraceMs,
    ...lifecycle,
  });
}

test("schema-v1 activity categories are an explicit immutable protocol allowlist", () => {
  assert.equal(Object.isFrozen(CHILD_EXECUTION_TOOL_CATEGORIES_V1), true);
  assert.notEqual(CHILD_EXECUTION_TOOL_CATEGORIES_V1, WORKER_TOOLS);
  assert.deepEqual(CHILD_EXECUTION_TOOL_CATEGORIES_V1, [
    "read", "write", "edit", "grep", "find", "ls", "bash",
    "web_search", "source_check", "fetch_content", "get_search_content",
    "research_file", "other",
  ]);
});

test("runs a distinct child with explicit inherited context and ordered progress", async () => {
  const progress: DelegatedTaskProgress[] = [];
  const outcome = await createRunner().run("success", {
    onProgress(event) {
      progress.push(event);
    },
  });

  assert.equal(outcome.status, "succeeded");
  assert.notEqual(outcome.child.pid, process.pid);
  assert.equal(outcome.child.runId.length, 36);
  assert.deepEqual(
    progress.filter((event) => event.type !== "live").map((event) => event.type),
    ["started", "identified", "activity"],
  );

  const observed = JSON.parse(outcome.output);
  assert.equal(observed.pid, outcome.child.pid);
  assert.equal(observed.ppid, process.pid);
  assert.equal(observed.provider, "controlled-provider");
  assert.equal(observed.model, "controlled-model");
  assert.equal(observed.thinking, "high");
  assert.equal(observed.cwd, canonicalRoot);
  assert.equal(observed.authDigest, authDigest);
  assert.deepEqual(observed.modeArguments, ["--mode", "rpc"]);
  assert.equal(observed.promptTerminatedByLf, true);

  const transcript = childTranscript(outcome);
  assert.ok(transcript);
  assert.deepEqual(
    transcript.entries.map((entry) => entry.type),
    ["message_end", "agent_settled"],
  );
  assert.doesNotMatch(JSON.stringify(outcome), /transcript|auth-marker/);
});

test("returns child failure as data and remains reusable", async () => {
  const runner = createRunner();

  const failed = await runner.run("failure");
  assert.equal(failed.status, "failed");
  assert.equal(failed.failure.kind, "child-failed");

  const unrelated = await runner.run("success");
  assert.equal(unrelated.status, "succeeded");
});

test("cancels the owned child and escalates only while it remains open", async () => {
  const controller = new AbortController();
  const progress: DelegatedTaskProgress[] = [];

  const outcome = await createRunner(25).run("ignore-term", {
    signal: controller.signal,
    onProgress(event) {
      progress.push(event);
      if (event.type === "identified") {
        controller.abort();
      }
    },
  });

  assert.equal(outcome.status, "cancelled");
  assert.ok(outcome.exit);
  assert.equal(outcome.exit.signal, "SIGKILL");
  assert.deepEqual(
    progress.map((event) => event.type),
    ["started", "identified", "terminating", "killing"],
  );
});

test("rejects malformed interactions before reserving worker authority", async () => {
  let authorityReservations = 0;
  const controller = new AbortController();
  const runner = createRunner(25, "controlled-provider", {
    async beforeInteraction() {
      authorityReservations += 1;
      return true;
    },
  });
  let identifiedResolve!: () => void;
  const identified = new Promise<void>((resolve) => { identifiedResolve = resolve; });
  const running = runner.run("interactive-backpressure", {
    signal: controller.signal,
    onProgress(progress) { if (progress.type === "identified") identifiedResolve(); },
  });
  await identified;
  assert.deepEqual(
    await runner.interact?.({ type: "steer", message: "x".repeat(64 * 1024 + 1) }),
    { status: "rejected", code: "command-rejected" },
  );
  assert.equal(authorityReservations, 0);
  controller.abort();
  await running;
});

test("bounds interactive command bytes and pending writes under backpressure", async () => {
  const controller = new AbortController();
  const runner = createRunner(25);
  let identifiedResolve!: () => void;
  const identified = new Promise<void>((resolve) => { identifiedResolve = resolve; });
  const running = runner.run("interactive-backpressure", {
    signal: controller.signal,
    onProgress(progress) {
      if (progress.type === "identified") identifiedResolve();
    },
  });
  await identified;
  assert.ok(runner.interact);

  assert.deepEqual(
    await runner.interact({ type: "steer", message: "x".repeat(64 * 1024 + 1) }),
    { status: "rejected", code: "command-rejected" },
  );

  const pending = Array.from({ length: 16 }, () =>
    runner.interact!({ type: "follow_up", message: "x".repeat(64 * 1024) })
  );
  assert.deepEqual(
    await runner.interact({ type: "steer", message: "one command too many" }),
    { status: "rejected", code: "command-rejected" },
  );

  controller.abort();
  assert.equal((await running).status, "cancelled");
  const rejected = await Promise.all(pending);
  assert.ok(rejected.every((result) => result.status === "rejected"));
});

test("respawns a bounded Child Execution on the same session after the original exits", async () => {
  let active = 0;
  let maximumActive = 0;
  let executions = 0;
  let interaction: Promise<unknown> | undefined;
  const runIds: string[] = [];
  const sessionRoot = await mkdtemp(join(resolve(process.env.TMPDIR ?? "/tmp"), "matty-runtime-test-"));
  const runner = createChildPiRunner({
    invocation: { command: process.execPath, arguments: [fixture, "--tools", "read,grep"] },
    parent: { provider: "controlled-provider", model: "controlled-model", thinking: "high", cwd: canonicalRoot },
    authentication: { provider: "controlled-provider", environment: { PATH: process.env.PATH, MATTY_TEST_AUTH: authMarker } },
    sessionRoot,
    retainSessionUntilClose: true,
    async scheduleExecution(execute) {
      active += 1;
      executions += 1;
      maximumActive = Math.max(maximumActive, active);
      try { return await execute(); } finally { active -= 1; }
    },
  });
  const releaseSession = runner.retain?.();
  let interactionResolve!: () => void;
  const interactionStarted = new Promise<void>((resolve) => { interactionResolve = resolve; });
  runner.subscribePresentation?.((presentation) => {
    if (presentation.sessionState === "settled" && !interaction) {
      interaction = runner.interact?.({ type: "follow_up", message: "replace-after-exit" });
      interactionResolve();
    }
  });
  const running = runner.run("settled-respawn-candidate", {
    onProgress(progress) { if (progress.type === "identified") runIds.push(progress.child.runId); },
  });
  await interactionStarted;
  assert.equal((await interaction as { status: string }).status, "accepted");
  releaseSession?.();
  const outcome = await running;
  assert.equal(executions, 2);
  assert.equal(maximumActive, 1);
  assert.equal(new Set(runIds).size, 2);
  assert.deepEqual(childTerminalResponses(outcome).map((response) => JSON.parse(response).summary), [
    "initial candidate", "replacement after respawn",
  ]);
  const presentation = runner.presentation?.();
  assert.deepEqual(presentation?.entries.filter((entry) => entry.label === "Assistant").map((entry) => entry.content), [
    JSON.stringify({ summary: "initial candidate", evidence: ["initial"] }),
    JSON.stringify({ summary: "replacement after respawn", evidence: ["same-session", "same-tools"] }),
  ]);
  assert.equal(new Set(presentation?.entries.map((entry) => entry.id)).size, presentation?.entries.length);
  assert.deepEqual(presentation?.usage, { inputTokens: 2, outputTokens: 2, totalTokens: 4, cost: 0 });
  assert.deepEqual(JSON.parse(childTerminalResponses(outcome).at(-1) ?? "{}").evidence, ["same-session", "same-tools"]);
  await runner.close?.();
  assert.deepEqual(await readdir(sessionRoot), []);
  await rm(sessionRoot, { recursive: true });
});

test("atomically caps concurrent respawn interactions across the Child Session lifetime", async () => {
  let executions = 0;
  const sessionRoot = await mkdtemp(join(resolve(process.env.TMPDIR ?? "/tmp"), "matty-runtime-test-"));
  const runner = createChildPiRunner({
    invocation: { command: process.execPath, arguments: [fixture, "--tools", "read,grep"] },
    parent: { provider: "controlled-provider", model: "controlled-model", thinking: "high", cwd: canonicalRoot },
    authentication: { provider: "controlled-provider", environment: { PATH: process.env.PATH, MATTY_TEST_AUTH: authMarker } },
    sessionRoot,
    retainSessionUntilClose: true,
    async scheduleExecution(execute) {
      executions += 1;
      return await execute();
    },
  });
  const release = runner.retain?.();
  let settledResolve!: () => void;
  const settled = new Promise<void>((resolve) => { settledResolve = resolve; });
  runner.subscribePresentation?.((presentation) => {
    if (presentation.sessionState === "settled") settledResolve();
  });
  const running = runner.run("settled-respawn-candidate");
  await settled;
  const interactions = Array.from({ length: 24 }, () =>
    runner.interact!({ type: "follow_up", message: "replace-after-exit" })
  );
  const results = await Promise.all(interactions);
  assert.equal(results.filter((result) => result.status === "accepted").length, 15);
  assert.equal(results.filter((result) => result.status === "rejected").length, 9);
  release?.();
  assert.equal((await running).status, "succeeded");
  assert.equal(executions, 16);
  await runner.close?.();
  await rm(sessionRoot, { recursive: true });
});

test("cancels worker authority when a respawn interaction is rejected by the scheduler", async () => {
  const controller = new AbortController();
  let scheduled = 0;
  let authorityReservations = 0;
  let authorityCancellations = 0;
  const sessionRoot = await mkdtemp(join(resolve(process.env.TMPDIR ?? "/tmp"), "matty-runtime-test-"));
  const runner = createChildPiRunner({
    invocation: { command: process.execPath, arguments: [fixture, "--tools", "read,grep"] },
    parent: { provider: "controlled-provider", model: "controlled-model", thinking: "high", cwd: canonicalRoot },
    authentication: { provider: "controlled-provider", environment: { PATH: process.env.PATH, MATTY_TEST_AUTH: authMarker } },
    sessionRoot,
    retainSessionUntilClose: true,
    async beforeInteraction() {
      authorityReservations += 1;
      return true;
    },
    onInteractionRejected() { authorityCancellations += 1; },
    async scheduleExecution(execute, signal) {
      scheduled += 1;
      if (scheduled === 2) {
        controller.abort();
        return undefined;
      }
      return await execute();
    },
  });
  const release = runner.retain?.();
  let settledResolve!: () => void;
  const settled = new Promise<void>((resolve) => { settledResolve = resolve; });
  runner.subscribePresentation?.((presentation) => {
    if (presentation.sessionState === "settled") settledResolve();
  });
  const running = runner.run("settled-respawn-candidate", { signal: controller.signal });
  await settled;
  assert.deepEqual(await runner.interact?.({ type: "follow_up", message: "replace-after-exit" }), {
    status: "rejected", code: "command-rejected",
  });
  assert.equal(authorityReservations, 1);
  assert.equal(authorityCancellations, 1);
  release?.();
  await running;
  await runner.close?.();
  await rm(sessionRoot, { recursive: true });
});

test("publishes waiting-for-capability while an interaction reacquires authority", async () => {
  let releaseCapability!: () => void;
  const capability = new Promise<void>((resolve) => { releaseCapability = resolve; });
  const presentations: DelegatedTaskPresentation[] = [];
  const runner = createRunner(1_000, "controlled-provider", {
    async beforeInteraction() {
      await capability;
      return true;
    },
  });
  runner.subscribePresentation?.((presentation) => presentations.push(presentation));
  let identifiedResolve!: () => void;
  const identified = new Promise<void>((resolve) => { identifiedResolve = resolve; });
  const running = runner.run("interactive-candidate", {
    onProgress(progress) { if (progress.type === "identified") identifiedResolve(); },
  });
  await identified;
  const interaction = runner.interact!({ type: "steer", message: "finish-valid" });
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(presentations.at(-1)?.sessionState, "waiting-for-capability");
  releaseCapability();
  assert.equal((await interaction).status, "accepted");
  assert.equal((await running).status, "succeeded");
});

test("keeps capability contention visible while each queued interaction waits at its gate", async () => {
  const gates: Array<(available: boolean) => void> = [];
  const runner = createRunner(1_000, "controlled-provider", {
    beforeInteraction: async () => await new Promise<boolean>((resolve) => gates.push(resolve)),
  });
  let identifiedResolve!: () => void;
  const identified = new Promise<void>((resolve) => { identifiedResolve = resolve; });
  const running = runner.run("interactive-candidate", {
    onProgress(progress) { if (progress.type === "identified") identifiedResolve(); },
  });
  await identified;
  const first = runner.interact!({ type: "steer", message: "finish-valid" });
  const second = runner.interact!({ type: "follow_up", message: "finish-valid" });
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(gates.length, 2);
  assert.equal(runner.presentation?.()?.sessionState, "waiting-for-capability");
  gates[0]!(true);
  assert.equal((await first).status, "accepted");
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(runner.presentation?.()?.sessionState, "waiting-for-capability");
  gates[1]!(false);
  assert.equal((await second).status, "rejected");
  assert.equal((await running).status, "succeeded");
});

test("protocol-closes malformed method-specific Pi dialog requests", async () => {
  for (const task of [
    "malformed-extension-dialog-title",
    "malformed-extension-dialog-options-empty",
    "malformed-extension-dialog-options-string",
    "malformed-extension-dialog-confirm-message",
  ]) {
    const outcome = await createRunner(25).run(task);
    assert.equal(outcome.status, "failed", task);
    assert.equal(outcome.failure.kind, "protocol-failed", task);
    assert.match(outcome.failure.message, /malformed extension UI request/, task);
  }
});

test("routes extension dialogs into waiting state and responds without parent UI", async () => {
  const runner = createRunner();
  let waitingResolve!: () => void;
  const waiting = new Promise<void>((resolve) => { waitingResolve = resolve; });
  runner.subscribePresentation?.((presentation) => {
    if (presentation.sessionState === "waiting-for-input") waitingResolve();
  });
  const running = runner.run("extension-dialog");
  await waiting;
  const presentation = runner.presentation?.();
  assert.equal(presentation?.sessionState, "waiting-for-input");
  assert.equal(presentation?.pendingInput?.id, "fixture-dialog");
  assert.ok((presentation?.pendingInput?.expiresAt ?? 0) - Date.now() > 299_000);
  assert.equal(presentation?.pendingInput?.extendable, true);
  assert.deepEqual(runner.extendInputTimeout?.("fixture-dialog", 1_000), { status: "accepted" });
  assert.deepEqual(await runner.respondToInput?.("fixture-dialog", { value: "answered" }), { status: "accepted" });
  const outcome = await running;
  assert.equal(outcome.status, "succeeded");
  assert.equal(outcome.output, JSON.stringify({ response: "answered" }));
});

test("validates method-specific Pi dialog response shapes", async () => {
  for (const [task, invalid, valid, expected] of [
    ["extension-dialog-select", { confirmed: true }, { value: "second" }, "second"],
    ["extension-dialog-confirm", { value: "yes" }, { confirmed: false }, false],
    ["extension-dialog-editor", { confirmed: true }, { value: "edited" }, "edited"],
  ] as const) {
    const runner = createRunner();
    let waitingResolve!: () => void;
    const waiting = new Promise<void>((resolve) => { waitingResolve = resolve; });
    runner.subscribePresentation?.((presentation) => { if (presentation.pendingInput) waitingResolve(); });
    const running = runner.run(task);
    await waiting;
    assert.deepEqual(await runner.respondToInput?.("fixture-dialog", invalid), { status: "rejected", code: "command-rejected" });
    assert.deepEqual(await runner.respondToInput?.("fixture-dialog", valid), { status: "accepted" });
    const outcome = await running;
    assert.equal(outcome.status, "succeeded");
    assert.equal(outcome.output, JSON.stringify({ response: expected }));
  }
});

test("respects a valid positive integer extension dialog timeout", async () => {
  const runner = createRunner();
  let waitingResolve!: () => void;
  const waiting = new Promise<void>((resolve) => { waitingResolve = resolve; });
  runner.subscribePresentation?.((presentation) => {
    if (presentation.pendingInput) waitingResolve();
  });
  const startedAt = Date.now();
  const running = runner.run("extension-dialog-declared-timeout");
  await waiting;
  const pending = runner.presentation?.()?.pendingInput;
  assert.equal(pending?.extendable, false);
  assert.ok((pending?.expiresAt ?? 0) - startedAt >= 59_000);
  assert.ok((pending?.expiresAt ?? 0) - startedAt <= 61_000);
  assert.deepEqual(await runner.respondToInput?.("fixture-dialog", { value: "answered" }), { status: "accepted" });
  assert.equal((await running).status, "succeeded");
});

test("protocol-closes unsupported extension dialog timeout declarations", async () => {
  for (const task of [
    "extension-dialog-timeout-zero",
    "extension-dialog-timeout-fractional",
    "extension-dialog-timeout-huge",
    "extension-dialog-timeout-string",
  ]) {
    const outcome = await createRunner(25).run(task);
    assert.equal(outcome.status, "failed", task);
    assert.equal(outcome.failure.kind, "protocol-failed", task);
    assert.match(outcome.failure.message, /invalid extension UI timeout/, task);
  }
});

test("protocol-closes when writing an extension dialog response fails", async () => {
  const runner = createRunner(25);
  let waitingResolve!: () => void;
  const waiting = new Promise<void>((resolve) => { waitingResolve = resolve; });
  runner.subscribePresentation?.((presentation) => {
    if (presentation.pendingInput) waitingResolve();
  });
  const running = runner.run("extension-dialog-write-failure");
  await waiting;
  assert.deepEqual(
    await runner.respondToInput?.("fixture-dialog", { value: "x".repeat(8 * 1024 * 1024) }),
    { status: "rejected", code: "command-rejected" },
  );
  const outcome = await running;
  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "protocol-failed");
  assert.match(outcome.failure.message, /extension UI response/);
});

test("cancels an unanswered extension dialog at its declared timeout and rejects extension", async () => {
  const runner = createRunner();
  let waitingResolve!: () => void;
  const waiting = new Promise<void>((resolve) => { waitingResolve = resolve; });
  runner.subscribePresentation?.((presentation) => {
    if (presentation.pendingInput) waitingResolve();
  });
  const running = runner.run("extension-dialog-timeout");
  await waiting;
  assert.equal(runner.presentation?.()?.pendingInput?.extendable, false);
  assert.deepEqual(runner.extendInputTimeout?.("fixture-dialog"), { status: "rejected", code: "command-rejected" });
  const outcome = await running;
  assert.equal(outcome.status, "succeeded");
  assert.equal(outcome.output, JSON.stringify({ response: "cancelled" }));
});

test("settled RPC children cannot remain as idle processes", async () => {
  const outcome = await createRunner(25).run("ignore-settled-exit");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "child-failed");
  assert.equal(outcome.exit?.signal, "SIGKILL");
});

test("does not spawn when cancellation is already requested", async () => {
  const controller = new AbortController();
  controller.abort();

  const outcome = await createRunner().run("success", {
    signal: controller.signal,
  });

  assert.deepEqual(outcome, {
    status: "cancelled",
    child: null,
    phase: "before-spawn",
  });
});

test("rejects authentication for a different provider before spawning", async () => {
  const outcome = await createRunner(
    1_000,
    "different-provider",
  ).run("success");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.child, null);
  assert.equal(outcome.failure.kind, "invalid-parent-context");
});

test("closes malformed JSONL as a protocol failure without fallback", async () => {
  const outcome = await createRunner().run("malformed-json");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "protocol-failed");
});

test("fails only the child that exceeds frame or stderr bounds", async () => {
  for (const task of ["oversized-frame", "stderr-overflow"]) {
    const outcome = await createRunner(25).run(task);
    assert.equal(outcome.status, "failed");
    assert.equal(outcome.failure.kind, "protocol-failed");
    assert.match(outcome.failure.message, /frame|stderr/);
  }
  assert.equal((await createRunner().run("success")).status, "succeeded");
});

test("accepts protocol-valid CRLF frames while splitting only on LF", async () => {
  const outcome = await createRunner().run("crlf-output");

  assert.equal(outcome.status, "succeeded");
});

test("rejects known events but ignores extension notifications before the prompt response", async () => {
  const knownEvent = await createRunner().run("pre-response-event");
  assert.equal(knownEvent.status, "failed");
  assert.equal(knownEvent.failure.kind, "protocol-failed");

  const extensionNotification = await createRunner().run("pre-response-extension-notification");
  assert.equal(extensionNotification.status, "succeeded");
});

test("requires the correlated prompt response and settled event", async () => {
  for (const task of ["uncorrelated-response", "no-settlement"]) {
    const outcome = await createRunner().run(task);
    assert.equal(outcome.status, "failed");
    assert.equal(
      outcome.failure.kind,
      task === "uncorrelated-response" ? "protocol-failed" : "child-exited",
    );
  }
});

test("returns malformed assistant events as protocol failure data", async () => {
  for (const task of [
    "malformed-message",
    "malformed-known-event",
    "unknown-assistant-part",
    "assistant-text-without-text",
    "malformed-thinking-part",
    "malformed-tool-call-part",
  ]) {
    const outcome = await createRunner().run(task);

    assert.equal(outcome.status, "failed");
    assert.equal(outcome.failure.kind, "protocol-failed");
  }
});

test("accepts assistant thinking and tool-call parts with their minimum protocol shapes", async () => {
  const outcome = await createRunner().run("valid-assistant-parts");

  assert.equal(outcome.status, "succeeded");
  assert.equal(JSON.parse(outcome.output).provider, "controlled-provider");
});

test("fails closed when a message arrives after agent settlement", async () => {
  const outcome = await createRunner().run("post-settlement-message");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "protocol-failed");
  assert.match(outcome.failure.message, /settled/);
  assert.deepEqual(
    childTranscript(outcome)?.entries.map((entry) => entry.type),
    ["message_end", "agent_settled"],
  );
  assert.doesNotMatch(JSON.stringify(childTranscript(outcome)), /late replacement/);
});

test("fails closed when Pi settles on an intermediate tool-use assistant message", async () => {
  const outcome = await createRunner().run("tool-use-settled");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "protocol-failed");
});

test("accepts fixture-valid custom, user, and tool-result messages without replacing the assistant result", async () => {
  const progress: DelegatedTaskProgress[] = [];
  const outcome = await createRunner().run("custom-message-content", {
    onProgress(event) {
      progress.push(event);
    },
  });

  assert.equal(
    outcome.status,
    "succeeded",
    outcome.status === "failed"
      ? outcome.failure.kind
      : `unexpected status: ${outcome.status}`,
  );
  const observed = JSON.parse(outcome.output);
  assert.deepEqual(observed, {
    pid: outcome.child.pid,
    ppid: process.pid,
    provider: "controlled-provider",
    model: "controlled-model",
    thinking: "high",
    cwd: canonicalRoot,
    authDigest,
    modeArguments: ["--mode", "rpc"],
    promptTerminatedByLf: true,
  });
  assert.deepEqual(
    progress.filter((event) => event.type !== "live").map((event) => event.type),
    ["started", "identified", "activity"],
  );
  assert.deepEqual(
    childTranscript(outcome)?.entries.map((entry) => entry.type),
    ["message_end", "message_end", "message_end", "message_end", "agent_settled"],
  );
});

test("reports real Pi tool execution completion as ordered progress", async () => {
  const progress: DelegatedTaskProgress[] = [];
  const outcome = await createRunner().run("tool-progress", {
    onProgress(event) {
      progress.push(event);
    },
  });

  assert.equal(outcome.status, "succeeded");
  assert.deepEqual(
    progress.filter((event) => event.type !== "live").map((event) => event.type),
    ["started", "identified", "activity", "activity", "activity"],
  );
  const observations = progress.flatMap((event) =>
    event.type === "activity" ? [event.observation] : []
  );
  assert.deepEqual(observations.map((observation) => observation.sequence), [1, 2, 3]);
  assert.ok(observations.every((observation) =>
    observation.schemaVersion === 1 && Number.isFinite(observation.observedAt)
  ));
  assert.deepEqual(
    observations.map((observation) => observation.summary),
    [
      { schemaVersion: 1, kind: "tool-completed", tool: "read", outcome: "succeeded" },
      { schemaVersion: 1, kind: "tool-completed", tool: "bash", outcome: "succeeded" },
      { schemaVersion: 1, kind: "assistant-completed", outcome: "succeeded" },
    ],
  );
  assert.deepEqual(
    childTranscript(outcome)?.entries.map((entry) => entry.type),
    ["tool_execution_end", "tool_execution_end", "message_end", "agent_settled"],
  );
  assert.doesNotMatch(JSON.stringify(outcome), /secret-tool-call-id|transcript/);
});

test("redacts sensitive activity fields and categorizes valid unknown tools", async () => {
  const progress: DelegatedTaskProgress[] = [];
  const outcome = await createRunner().run("sensitive-activity", {
    onProgress(event) { progress.push(event); },
  });

  assert.equal(outcome.status, "succeeded");
  assert.deepEqual(
    progress.flatMap((event) => event.type === "activity" ? [event.observation.summary] : []),
    [
      { schemaVersion: 1, kind: "tool-completed", tool: "read", outcome: "succeeded" },
      { schemaVersion: 1, kind: "tool-completed", tool: "other", outcome: "failed" },
      { schemaVersion: 1, kind: "assistant-completed", outcome: "succeeded" },
    ],
  );
  assert.doesNotMatch(
    JSON.stringify(progress),
    /secret|tool-call|private|path|command|prompt|response|result|transcript/i,
  );
});

test("reports error assistant completion as failed activity", async () => {
  const progress: DelegatedTaskProgress[] = [];
  const outcome = await createRunner().run("failure", {
    onProgress(event) { progress.push(event); },
  });

  assert.equal(outcome.status, "failed");
  assert.deepEqual(
    progress.flatMap((event) => event.type === "activity" ? [event.observation.summary] : []),
    [{ schemaVersion: 1, kind: "assistant-completed", outcome: "failed" }],
  );
});

test("ignores delta-only message_update and keeps message_end authoritative", async () => {
  const progress: DelegatedTaskProgress[] = [];
  const outcome = await createRunner().run("delta-only-message-update", {
    onProgress(event) { progress.push(event); },
  });

  assert.equal(outcome.status, "succeeded");
  assert.deepEqual(
    progress.flatMap((event) => event.type === "activity" ? [event.observation.summary] : []),
    [{ schemaVersion: 1, kind: "assistant-completed", outcome: "succeeded" }],
  );
});

test("coalesces private live state and emits only safe revision markers", async () => {
  const progress: DelegatedTaskProgress[] = [];
  const presentations: DelegatedTaskPresentation[] = [];
  const runner = createRunner();
  runner.subscribePresentation?.((presentation) => presentations.push(presentation));
  const outcome = await runner.run("interleaved-live-updates", {
    onProgress(event) { progress.push(event); },
  });

  assert.equal(outcome.status, "succeeded");
  const live = progress.filter((event) => event.type === "live");
  assert.ok(live.length > 0);
  assert.deepEqual(live.at(-1), {
    type: "live",
    child: outcome.child,
    revision: live.length,
  });
  assert.doesNotMatch(JSON.stringify(live), /first|second|read|old|new|call-live/);
  const partial = presentations.find((presentation) =>
    presentation.assistant.length === 3 &&
    presentation.tools[0]?.content === JSON.stringify({ content: "base-new" })
  );
  assert.ok(partial);
  assert.deepEqual(partial.assistant.map((part) => [part.contentIndex, part.type]), [
    [0, "text"], [1, "text"], [2, "thinking"],
  ]);
  assert.deepEqual(partial.tools, [{
    toolCallId: "execution:1:call-live-base",
    toolName: "read",
    status: "running",
    args: "{}",
    content: JSON.stringify({ content: "base-new" }),
  }]);
  assert.doesNotMatch(JSON.stringify(partial), /base-old/);
  assert.deepEqual(runner.presentation?.()?.assistant.map((part) => part.contentIndex), [0]);
});

test("presents bounded transcript entries and labeled usage without exposing terminal controls", async () => {
  const runner = createRunner();
  const outcome = await runner.run("interleaved-live-updates");

  assert.equal(outcome.status, "succeeded");
  const presentation = runner.presentation?.();
  assert.ok(presentation);
  assert.ok(presentation.entries.some((entry) =>
    entry.category === "message" && entry.label === "Assistant" && entry.expandedByDefault
  ));
  assert.ok(presentation.entries.some((entry) =>
    entry.category === "reasoning" && !entry.expandedByDefault
  ));
  assert.ok(presentation.entries.some((entry) =>
    entry.category === "tool" && entry.label.includes("read") && !entry.expandedByDefault
  ));
  assert.ok(presentation.entries.some((entry) =>
    entry.category === "error" && entry.content === "base-extension-error" &&
    !entry.expandedByDefault
  ));
  assert.deepEqual(presentation.usage, {
    inputTokens: 1,
    outputTokens: 1,
    totalTokens: 2,
    cost: 0,
  });
  assert.doesNotMatch(JSON.stringify(presentation), /\u001b|\u0000/);
  assert.equal(Object.isFrozen(presentation.entries), true);
});

test("retains tool bytes in the presentation limit across assistant messages", async () => {
  const outcome = await createRunner(25).run("presentation-accounting-overflow");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "protocol-failed");
  assert.match(outcome.failure.message, /live (?:assistant|presentation) buffer limit/);
});

test("neutralizes ANSI and terminal controls in the settled assistant output", async () => {
  const outcome = await createRunner().run("ansi-output");

  assert.equal(outcome.status, "succeeded");
  assert.equal(outcome.output, "safe");
});

test("accepts Pi length and deferred assistant completion", async () => {
  for (const task of ["length-assistant", "deferred-assistant"]) {
    const progress: DelegatedTaskProgress[] = [];
    const outcome = await createRunner().run(task, {
      onProgress(event) { progress.push(event); },
    });

    assert.equal(outcome.status, "succeeded");
    assert.deepEqual(
      progress.flatMap((event) => event.type === "activity" ? [event.observation.summary] : []),
      [{ schemaVersion: 1, kind: "assistant-completed", outcome: "succeeded" }],
    );
  }
});

test("keeps aborted terminal assistant completion as a child failure", async () => {
  const outcome = await createRunner().run("aborted-assistant");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "child-failed");
});

test("rejects absent or unknown assistant stop reasons as a protocol failure", async () => {
  for (const task of [
    "malformed-tool-activity",
    "malformed-assistant-activity",
    "absent-assistant-stop-reason",
  ]) {
    const outcome = await createRunner().run(task);

    assert.equal(outcome.status, "failed");
    assert.equal(outcome.failure.kind, "protocol-failed");
  }
});
