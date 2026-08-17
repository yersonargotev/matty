import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readdir, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { ChildSessionStore, ChildSessionStoreError } from "../src/application/child-session-store.ts";

const taskId = "10000000-0000-4000-8000-000000000001";
const delegationId = "20000000-0000-4000-8000-000000000002";

function metadata(id = taskId) {
  return {
    taskId: id,
    delegationId,
    taskIndex: 0,
    role: "explorer" as const,
    requirement: "required" as const,
    declaration: { role: "explorer" as const },
    git: { head: "abc123", workingTree: " M fixture.ts" },
  };
}

test("Child Session Store creates only a closed manifest and Pi session with restrictive modes", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  try {
    const store = new ChildSessionStore({ root: join(sandbox, "child-sessions"), });
    const session = store.session(metadata());
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

test("persistent continuation copies the Pi conversation into a fresh task-scoped session", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  try {
    const store = new ChildSessionStore({ root: join(sandbox, "persistent"), });
    const source = store.session(metadata());
    await source.prepare("/fixture");
    await writeFile(source.sessionFile, [
      JSON.stringify({ type: "session", version: 3, id: taskId, cwd: "/fixture" }),
      JSON.stringify({ type: "message", message: { role: "assistant", content: [{ type: "text", text: "immutable source result" }] } }),
      "",
    ].join("\n"), { mode: 0o600 });
    await source.finish("succeeded");

    const nextTaskId = "30000000-0000-4000-8000-000000000003";
    const nextDelegationId = "40000000-0000-4000-8000-000000000004";
    const continuation = store.continuation(taskId, {
      ...metadata(nextTaskId),
      declaration: { role: "explorer" },
      delegationId: nextDelegationId,
      sourceTaskId: taskId,
      sourceDelegationId: delegationId,
    });
    const sourceBefore = await readFile(source.sessionFile, "utf8");
    await continuation.prepare("/continued");
    const copied = (await readFile(continuation.sessionFile, "utf8")).trim().split("\n").map((line) => JSON.parse(line));
    assert.equal(copied[0].id, nextTaskId);
    assert.equal(copied[0].cwd, "/continued");
    assert.equal(copied[1].message.content[0].text, "immutable source result");
    assert.equal(await readFile(source.sessionFile, "utf8"), sourceBefore);
    const continuedManifest = JSON.parse(await readFile(continuation.manifestFile, "utf8"));
    assert.equal(continuedManifest.schemaVersion, 2);
    assert.deepEqual(continuedManifest.declaration, { role: "explorer" });
    assert.equal(continuedManifest.sourceTaskId, taskId);

    await continuation.finish("succeeded");
    const chainedTaskId = "50000000-0000-4000-8000-000000000005";
    const chained = store.continuation(nextTaskId, {
      ...metadata(chainedTaskId),
      delegationId: "60000000-0000-4000-8000-000000000006",
      declaration: { role: "explorer" },
      sourceTaskId: nextTaskId,
      sourceDelegationId: nextDelegationId,
    });
    await chained.prepare("/chained");
    const chainedManifest = JSON.parse(await readFile(chained.manifestFile, "utf8"));
    assert.deepEqual(chainedManifest.declaration, { role: "explorer" });
    assert.equal(chainedManifest.sourceTaskId, nextTaskId);
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});

test("discovery fails closed on malformed metadata or unexpected store entries", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  try {
    const store = new ChildSessionStore({ root: join(sandbox, "child-sessions"), });
    await mkdir(join(store.root, taskId), { recursive: true });
    await writeFile(join(store.root, taskId, "manifest.json"), JSON.stringify({
      schemaVersion: 1,
      taskId,
      delegationId,
      taskIndex: 0,
      role: "explorer",
      requirement: "required",
      state: "succeeded",
      createdAt: 1,
      updatedAt: 1,
    }), { mode: 0o600 });
    await writeFile(join(store.root, taskId, "session.jsonl"), "", { mode: 0o600 });
    await assert.rejects(store.discover(), (error) => error instanceof ChildSessionStoreError && error.code === "incompatible-metadata");
    await chmod(join(store.root, taskId), 0o700);
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});

test("discovery accepts only schema 2 manifests with strict declarations and Git state", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  try {
    const store = new ChildSessionStore({ root: join(sandbox, "persistent"), });
    const session = store.session(metadata());
    await session.prepare("/fixture");
    await session.finish("succeeded");
    const original = JSON.parse(await readFile(session.manifestFile, "utf8"));
    for (const mutate of [
      (manifest: Record<string, unknown>) => { manifest.schemaVersion = 1; },
      (manifest: Record<string, unknown>) => { (manifest.declaration as Record<string, unknown>).unexpected = true; },
      (manifest: Record<string, unknown>) => { (manifest.declaration as Record<string, unknown>).web = "optional"; },
      (manifest: Record<string, unknown>) => { (manifest.git as Record<string, unknown>).unexpected = true; },
      (manifest: Record<string, unknown>) => { manifest.sourceTaskId = taskId; },
    ]) {
      const malformed = structuredClone(original) as Record<string, unknown>;
      mutate(malformed);
      await writeFile(session.manifestFile, JSON.stringify(malformed), { mode: 0o600 });
      await assert.rejects(store.discover(), (error) => error instanceof ChildSessionStoreError && error.code === "incompatible-metadata");
    }
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
    });
    const session = store.session(metadata(), project);
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
    const store = new ChildSessionStore({ root: join(sandbox, "child-sessions"), now: () => now, maxSessions: 2, maxAgeMs: 7 * 86400_000, maxBytes: 1_000_000 });
    const ids = [1, 2, 3].map((n) => `${n}0000000-0000-4000-8000-00000000000${n}`);
    for (const [index, id] of ids.entries()) {
      const session = store.session(metadata(id));
      await session.prepare("/fixture");
      await writeFile(session.sessionFile, "", { mode: 0o600 });
      if (index < 2) await session.finish("succeeded");
      now += 1_000;
    }
    await store.enforceRetention();
    assert.deepEqual((await readdir(store.root)).sort(), ids.sort());
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});

test("retention enforces age and global byte bounds independently", async () => {
  const sandbox = await mkdtemp(join(tmpdir(), "matty-child-store-"));
  let now = Date.parse("2026-02-01T00:00:00Z");
  const ids = [4, 5, 6].map((n) => `${n}0000000-0000-4000-8000-00000000000${n}`);
  try {
    const root = join(sandbox, "child-sessions");
    const store = new ChildSessionStore({ root, now: () => now, maxSessions: 100, maxAgeMs: 7 * 86400_000, maxBytes: 700 });
    for (const id of ids.slice(0, 2)) {
      const session = store.session(metadata(id));
      await session.prepare("/fixture");
      await writeFile(session.sessionFile, "x".repeat(400), { mode: 0o600 });
      await session.finish("succeeded");
      now += 1_000;
    }
    now += 8 * 86400_000;
    const recent = store.session(metadata(ids[2]));
    await recent.prepare("/fixture");
    await recent.finish("succeeded");
    await store.enforceRetention();
    assert.deepEqual(await readdir(root), [ids[2]]);

    const byteRoot = join(sandbox, "byte-child-sessions");
    const byteStore = new ChildSessionStore({
      root: byteRoot,
      now: () => now,
      maxSessions: 100,
      maxAgeMs: Number.MAX_SAFE_INTEGER,
      maxBytes: 1_200,
    });
    const byteIds = [7, 8].map((n) => `${n}0000000-0000-4000-8000-00000000000${n}`);
    for (const id of byteIds) {
      const session = byteStore.session(metadata(id));
      await session.prepare("/fixture");
      await writeFile(session.sessionFile, "x".repeat(400), { mode: 0o600 });
      await session.finish("succeeded");
      now += 1_000;
    }
    await byteStore.enforceRetention();
    assert.deepEqual(await readdir(byteRoot), [byteIds[1]]);
  } finally { await rm(sandbox, { recursive: true, force: true }); }
});
