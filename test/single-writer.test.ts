import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdir, mkdtemp, realpath, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { acquireRepositoryWriter } from "../src/application/single-writer.ts";

test("Single Writer coordinates the same repository across parent processes", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-single-writer-"));
  try {
    const project = join(root, "project");
    const state = join(root, "state");
    await Promise.all([
      mkdir(project, { recursive: true }),
      mkdir(state, { recursive: true }),
    ]);
    const workingTree = await realpath(project);
    const stateRoot = await realpath(state);
    const child = spawn(
      process.execPath,
      [
        "test/fixtures/single-writer-fixture.mjs",
        workingTree,
        stateRoot,
      ],
      {
        cwd: process.cwd(),
        stdio: ["pipe", "pipe", "pipe"],
      },
    );
    await new Promise<void>((resolveReady, rejectReady) => {
      child.once("error", rejectReady);
      child.stdout.setEncoding("utf8");
      child.stdout.once("data", (chunk) => {
        if (String(chunk).includes("acquired")) {
          resolveReady();
        } else {
          rejectReady(new Error(`unexpected child output: ${String(chunk)}`));
        }
      });
    });

    assert.equal(
      await acquireRepositoryWriter(workingTree, stateRoot),
      undefined,
    );
    child.stdin.end();
    await new Promise<void>((resolveExit, rejectExit) => {
      child.once("error", rejectExit);
      child.once("close", (code) => {
        code === 0
          ? resolveExit()
          : rejectExit(new Error(`lock fixture exited with ${code}`));
      });
    });

    const release = await acquireRepositoryWriter(workingTree, stateRoot);
    assert.ok(release);
    await release();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
