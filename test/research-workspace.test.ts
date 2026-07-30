import assert from "node:assert/strict";
import {
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rm,
  symlink,
  utimes,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  RESEARCH_WORKSPACE_MARKER,
  cleanupResearchWorkspace,
  cleanupStaleResearchWorkspaces,
  createResearchWorkspace,
  writeResearchFile,
} from "../src/domain/research-workspace.ts";

test("creates unique validated workspaces and writes only approved artifacts", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-research-test-"));
  try {
    const project = join(root, "project");
    const temporaryRoot = join(root, "isolated");
    await mkdir(project);
    const report = join(project, "docs", "research", "result.md");

    const first = await createResearchWorkspace({
      temporaryRoot,
      projectRoot: project,
      report,
    });
    const second = await createResearchWorkspace({
      temporaryRoot,
      projectRoot: project,
      report: join(project, "docs", "research", "second.md"),
    });

    assert.notEqual(first.workspace, second.workspace);
    assert.equal(
      first.report,
      join(await realpath(project), "docs", "research", "result.md"),
    );
    assert.equal(await realpath(first.workspace), first.workspace);

    const notes = await writeResearchFile(first, {
      destination: "workspace",
      path: "evidence/source.md",
      content: "useful evidence",
    });
    assert.equal(
      await readFile(notes.path, "utf8"),
      "useful evidence",
    );
    const result = await writeResearchFile(first, {
      destination: "report",
      content: "# Research report\n",
    });
    assert.equal(result.path, first.report);
    assert.equal(await readFile(first.report, "utf8"), "# Research report\n");
    await assert.rejects(
      writeResearchFile(first, {
        destination: "report",
        content: "overwrite",
      }),
      /overwrite is not authorized/,
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("rejects absolute, traversal, and symlink workspace escapes", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-research-escape-"));
  try {
    const project = join(root, "project");
    const external = join(root, "external");
    await Promise.all([mkdir(project), mkdir(external)]);
    const scope = await createResearchWorkspace({
      temporaryRoot: join(root, "isolated"),
      projectRoot: project,
      report: join(project, "report.md"),
    });
    await symlink(external, join(scope.workspace, "linked"));

    for (const path of [
      join(external, "absolute.md"),
      "../traversal.md",
      "linked/escape.md",
    ]) {
      await assert.rejects(
        writeResearchFile(scope, {
          destination: "workspace",
          path,
          content: "blocked",
        }),
        /research workspace path/,
      );
    }
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("rejects an existing Research Report before starting a run", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-research-existing-"));
  try {
    const project = join(root, "project");
    const report = join(project, "report.md");
    await mkdir(project);
    await writeFile(report, "existing");

    await assert.rejects(
      createResearchWorkspace({
        temporaryRoot: join(root, "isolated"),
        projectRoot: project,
        report,
      }),
      /overwrite is not authorized/,
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("clean shutdown removes a workspace but never its report", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-research-clean-"));
  try {
    const project = join(root, "project");
    await mkdir(project);
    const scope = await createResearchWorkspace({
      temporaryRoot: join(root, "isolated"),
      projectRoot: project,
      report: join(project, "report.md"),
    });
    await writeResearchFile(scope, {
      destination: "report",
      content: "durable",
    });

    await cleanupResearchWorkspace(scope);

    await assert.rejects(lstat(scope.workspace));
    assert.equal(await readFile(scope.report, "utf8"), "durable");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("startup cleanup removes only old validated marker-bearing workspaces", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-research-stale-"));
  try {
    const project = join(root, "project");
    const temporaryRoot = join(root, "isolated");
    await mkdir(project);
    const old = await createResearchWorkspace({
      temporaryRoot,
      projectRoot: project,
      report: join(project, "old.md"),
    });
    const recent = await createResearchWorkspace({
      temporaryRoot,
      projectRoot: project,
      report: join(project, "recent.md"),
    });
    const unmarked = join(temporaryRoot, "unmarked");
    const forged = join(temporaryRoot, "forged");
    await mkdir(unmarked);
    await mkdir(forged);
    await writeFile(
      join(forged, RESEARCH_WORKSPACE_MARKER),
      JSON.stringify({
        schemaVersion: 1,
        runId: "forged",
        workspace: forged,
      }),
    );
    const oldTime = new Date("2026-07-28T00:00:00.000Z");
    await utimes(join(old.workspace, RESEARCH_WORKSPACE_MARKER), oldTime, oldTime);
    await utimes(
      join(forged, RESEARCH_WORKSPACE_MARKER),
      oldTime,
      oldTime,
    );

    const removed = await cleanupStaleResearchWorkspaces({
      temporaryRoot,
      now: new Date("2026-07-30T00:00:01.000Z"),
    });

    assert.deepEqual(removed, [old.workspace]);
    await assert.rejects(lstat(old.workspace));
    assert.ok(await lstat(recent.workspace));
    assert.ok(await lstat(unmarked));
    assert.ok(await lstat(forged));
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
