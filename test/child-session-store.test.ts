import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { ChildSessionStore, ChildSessionStoreError } from "../src/application/child-session-store.ts";

const taskId = "10000000-0000-4000-8000-000000000001";
const delegationId = "20000000-0000-4000-8000-000000000002";

function metadata(id = taskId) {
  return { taskId: id, delegationId, taskIndex: 0, role: "explorer" as const, requirement: "required" as const };
}

test("Child Session Store creates only a closed manifest and Pi session with restrictive modes", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  try {
    const store = new ChildSessionStore({ root: join(sandbox, "child-sessions"), ephemeralRoot: join(sandbox, "ephemeral") });
    const session = store.session(metadata(), "persistent");
    await session.prepare("/fixture");
    await writeFile(session.sessionFile, '{"type":"session","version":3}\n', { mode: 0o600 });
    await session.finish("succeeded");

    assert.deepEqual(await readdir(session.directory), ["manifest.json", "session.jsonl"]);
    assert.equal((await stat(store.root)).mode & 0o777, 0o700);
    assert.equal((await stat(session.directory)).mode & 0o777, 0o700);
    assert.equal((await stat(session.manifestFile)).mode & 0o777, 0o600);
    assert.equal((await stat(session.sessionFile)).mode & 0o777, 0o600);
    assert.doesNotMatch(await readFile(session.manifestFile, "utf8"), /project|transcript|prompt|path/i);
    await session.close();
    assert.deepEqual((await store.discover()).map((item) => item.manifest.state), ["succeeded"]);
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});

test("ephemeral Child Session state is removed at close", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  try {
    const store = new ChildSessionStore({ root: join(sandbox, "persistent"), ephemeralRoot: join(sandbox, "temporary") });
    const session = store.session(metadata(), "ephemeral");
    await session.prepare("/fixture");
    await writeFile(session.sessionFile, "", { mode: 0o600 });
    await session.close();
    await assert.rejects(stat(session.directory));
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});

test("discovery fails closed on malformed metadata or unexpected store entries", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  try {
    const store = new ChildSessionStore({ root: join(sandbox, "child-sessions"), ephemeralRoot: join(sandbox, "temporary") });
    await mkdir(join(store.root, taskId), { recursive: true });
    await writeFile(join(store.root, taskId, "manifest.json"), JSON.stringify({ schemaVersion: 99 }), { mode: 0o600 });
    await writeFile(join(store.root, taskId, "session.jsonl"), "", { mode: 0o600 });
    await assert.rejects(store.discover(), (error) => error instanceof ChildSessionStoreError && error.code === "incompatible-metadata");
    await chmod(join(store.root, taskId), 0o700);
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});

test("Child Session Store rejects project-overlapping storage without deleting worker files", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  const project = join(sandbox, "project");
  try {
    await mkdir(project);
    const sentinel = join(project, "worker-output.txt");
    await writeFile(sentinel, "keep me");
    const store = new ChildSessionStore({
      root: join(project, ".pi", "agent", "matty", "child-sessions"),
      ephemeralRoot: join(sandbox, "temporary"),
    });
    const session = store.session(metadata(), "persistent", project);
    await assert.rejects(
      session.prepare(project),
      (error) => error instanceof ChildSessionStoreError && error.code === "malformed-store",
    );
    assert.equal(await readFile(sentinel, "utf8"), "keep me");
    await assert.rejects(stat(join(project, ".pi")));
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});

test("retention evicts terminal sessions oldest-first but never active sessions", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  let now = Date.parse("2026-02-01T00:00:00Z");
  try {
    const store = new ChildSessionStore({ root: join(sandbox, "child-sessions"), ephemeralRoot: join(sandbox, "temporary"), now: () => now, maxSessions: 2, maxAgeMs: 7 * 86400_000, maxBytes: 1_000_000 });
    const ids = [1, 2, 3].map((n) => `${n}0000000-0000-4000-8000-00000000000${n}`);
    for (const [index, id] of ids.entries()) {
      const session = store.session(metadata(id), "persistent");
      await session.prepare("/fixture");
      await writeFile(session.sessionFile, "", { mode: 0o600 });
      if (index < 2) await session.finish("succeeded");
      now += 1_000;
    }
    await store.enforceRetention();
    assert.deepEqual((await readdir(store.root)).sort(), ids.slice(1).sort());
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});

test("retention enforces age and global byte bounds independently", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  let now = Date.parse("2026-02-01T00:00:00Z");
  const ids = [4, 5, 6].map((n) => `${n}0000000-0000-4000-8000-00000000000${n}`);
  try {
    const root = join(sandbox, "child-sessions");
    const store = new ChildSessionStore({ root, ephemeralRoot: join(sandbox, "temporary"), now: () => now, maxSessions: 100, maxAgeMs: 7 * 86400_000, maxBytes: 700 });
    for (const id of ids.slice(0, 2)) {
      const session = store.session(metadata(id), "persistent");
      await session.prepare("/fixture");
      await writeFile(session.sessionFile, "x".repeat(400), { mode: 0o600 });
      await session.finish("succeeded");
      now += 1_000;
    }
    now += 8 * 86400_000;
    const recent = store.session(metadata(ids[2]), "persistent");
    await recent.prepare("/fixture");
    await recent.finish("succeeded");
    await store.enforceRetention();
    assert.deepEqual(await readdir(root), [ids[2]]);

    const byteRoot = join(sandbox, "byte-child-sessions");
    const byteStore = new ChildSessionStore({
      root: byteRoot,
      ephemeralRoot: join(sandbox, "byte-temporary"),
      now: () => now,
      maxSessions: 100,
      maxAgeMs: Number.MAX_SAFE_INTEGER,
      maxBytes: 700,
    });
    const byteIds = [7, 8].map((n) => `${n}0000000-0000-4000-8000-00000000000${n}`);
    for (const id of byteIds) {
      const session = byteStore.session(metadata(id), "persistent");
      await session.prepare("/fixture");
      await writeFile(session.sessionFile, "x".repeat(400), { mode: 0o600 });
      await session.finish("succeeded");
      now += 1_000;
    }
    await byteStore.enforceRetention();
    assert.deepEqual(await readdir(byteRoot), [byteIds[1]]);
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});
