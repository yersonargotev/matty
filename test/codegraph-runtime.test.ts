import assert from "node:assert/strict";
import { homedir } from "node:os";
import { dirname } from "node:path";
import test from "node:test";

import {
  createCodeGraphProjectInitializer,
  unsafeCodeGraphRootReason,
  type CodeGraphRuntimeDependencies,
} from "../src/application/codegraph-runtime.ts";

function runtime(overrides: Partial<CodeGraphRuntimeDependencies> = {}) {
  let nearestRoot: string | null = null;
  let initializeCalls = 0;
  let closeCalls = 0;
  let lockCalls = 0;
  const dependencies: CodeGraphRuntimeDependencies = {
    async canonicalize(path) {
      return `/canonical${path}`;
    },
    findNearestRoot() {
      return nearestRoot;
    },
    unsafeRootReason() {
      return null;
    },
    async initialize() {
      initializeCalls += 1;
      return { close() { closeCalls += 1; } };
    },
    async withProjectLock(_path, operation) {
      lockCalls += 1;
      return operation();
    },
    ...overrides,
  };
  return {
    dependencies,
    setNearestRoot(value: string | null) {
      nearestRoot = value;
    },
    counts() {
      return { initializeCalls, closeCalls, lockCalls };
    },
  };
}

test("CodeGraph initialization skips an indexed ancestor", async () => {
  const fake = runtime();
  fake.setNearestRoot("/workspace");
  const initialize = createCodeGraphProjectInitializer(fake.dependencies);

  assert.deepEqual(await initialize("/workspace/packages/app"), {
    status: "already-initialized",
    root: "/workspace",
  });
  assert.deepEqual(fake.counts(), {
    initializeCalls: 0,
    closeCalls: 0,
    lockCalls: 0,
  });
});

test("CodeGraph initialization indexes once, closes, and single-flights", async () => {
  let release: (() => void) | undefined;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  let calls = 0;
  const fake = runtime({
    async initialize() {
      calls += 1;
      await gate;
      return { close() {} };
    },
  });
  const initialize = createCodeGraphProjectInitializer(fake.dependencies);

  const first = initialize("/repo");
  const second = initialize("/repo");
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(calls, 1);
  release?.();
  assert.deepEqual(await Promise.all([first, second]), [
    { status: "initialized", root: "/canonical/repo" },
    { status: "initialized", root: "/canonical/repo" },
  ]);
});

test("CodeGraph initialization rechecks after acquiring the process lock", async () => {
  const fake = runtime({
    async withProjectLock(_path, operation) {
      fake.setNearestRoot("/canonical/repo");
      return operation();
    },
  });
  const initialize = createCodeGraphProjectInitializer(fake.dependencies);

  assert.deepEqual(await initialize("/repo"), {
    status: "already-initialized",
    root: "/canonical/repo",
  });
  assert.equal(fake.counts().initializeCalls, 0);
});

test("CodeGraph initialization closes a successfully opened graph", async () => {
  const fake = runtime();
  const initialize = createCodeGraphProjectInitializer(fake.dependencies);

  assert.deepEqual(await initialize("/repo"), {
    status: "initialized",
    root: "/canonical/repo",
  });
  assert.deepEqual(fake.counts(), {
    initializeCalls: 1,
    closeCalls: 1,
    lockCalls: 1,
  });
});

test("CodeGraph safety rejects home and its parent", () => {
  assert.equal(unsafeCodeGraphRootReason(homedir()), "your home directory");
  assert.equal(
    unsafeCodeGraphRootReason(dirname(homedir())),
    "a parent of your home directory",
  );
  if (process.platform === "darwin" || process.platform === "win32") {
    assert.equal(
      unsafeCodeGraphRootReason(homedir().toUpperCase()),
      "your home directory",
    );
  }
});

test("CodeGraph initialization refuses unsafe roots", async () => {
  const fake = runtime({
    unsafeRootReason() {
      return "the user home directory";
    },
  });
  const initialize = createCodeGraphProjectInitializer(fake.dependencies);

  await assert.rejects(
    initialize("/home/user"),
    /looks like the user home directory/,
  );
  assert.equal(fake.counts().initializeCalls, 0);
});
