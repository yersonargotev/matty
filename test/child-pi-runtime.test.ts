import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { realpath } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  childTranscript,
  createChildPiRunner,
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
) {
  return createChildPiRunner({
    invocation: {
      command: process.execPath,
      arguments: [fixture],
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
    progress.map((event) => event.type),
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
    progress.map((event) => event.type),
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
    progress.map((event) => event.type),
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
