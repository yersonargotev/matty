import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { realpath } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  createChildPiRunner,
  type DelegatedTaskProgress,
} from "../src/application/child-pi-runtime.ts";

const repositoryRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "..",
);
const fixture = resolve(
  repositoryRoot,
  "test/fixtures/child-pi-fixture.mjs",
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
    ["started", "identified", "message"],
  );

  const observed = JSON.parse(outcome.output);
  assert.equal(observed.pid, outcome.child.pid);
  assert.equal(observed.ppid, process.pid);
  assert.equal(observed.provider, "controlled-provider");
  assert.equal(observed.model, "controlled-model");
  assert.equal(observed.thinking, "high");
  assert.equal(observed.cwd, canonicalRoot);
  assert.equal(observed.authDigest, authDigest);
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

test("requires the Pi session header to be the first JSONL record", async () => {
  const outcome = await createRunner().run("message-before-header");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "protocol-failed");
});

test("returns malformed assistant events as protocol failure data", async () => {
  const outcome = await createRunner().run("malformed-message");

  assert.equal(outcome.status, "failed");
  assert.equal(outcome.failure.kind, "protocol-failed");
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
    ["started", "identified", "tool-result", "tool-result", "message"],
  );
});
