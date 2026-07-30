import assert from "node:assert/strict";
import { mkdir, mkdtemp, realpath, rm, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  inspectWorkerCommand,
  inspectWorkerPath,
  type WorkerGuardScope,
  type WorkerMutationClass,
} from "../src/domain/worker-guard.ts";

async function withScope(
  run: (scope: WorkerGuardScope, paths: {
    root: string;
    project: string;
    temporary: string;
    home: string;
    external: string;
  }) => Promise<void>,
): Promise<void> {
  const root = await mkdtemp(join(tmpdir(), "matty-worker-guard-"));
  try {
    const project = join(root, "project");
    const temporary = join(root, "temporary");
    const home = join(root, "home");
    const external = join(root, "external");
    await Promise.all(
      [project, temporary, home, external, join(project, ".git")].map(
        (path) => mkdir(path, { recursive: true }),
      ),
    );
    await run(
      {
        workingTree: await realpath(project),
        temporaryPaths: [await realpath(temporary)],
        userConfigurationPaths: [
          await realpath(home),
          join(home, ".config"),
        ],
      },
      { root, project, temporary, home, external },
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

test("allows worker writes in the trusted tree and validated temporary paths", async () => {
  await withScope(async (scope, paths) => {
    for (const path of [
      "src/new-file.ts",
      join(paths.project, "package.json"),
      join(paths.temporary, "compiler-output.txt"),
    ]) {
      assert.deepEqual(await inspectWorkerPath(scope, path), { allowed: true });
    }

    for (const command of [
      "npm install",
      "npm run typecheck",
      "node --test test/worker.test.ts",
      "touch src/generated.ts",
      `printf value > ${join(paths.temporary, "result.txt")}`,
      "git status --short",
      "git diff --check",
    ]) {
      assert.deepEqual(await inspectWorkerCommand(scope, command), {
        allowed: true,
      }, command);
    }
  });
});

test("blocks external, user-configuration, Git, GitHub, and global-install writes", async () => {
  await withScope(async (scope, paths) => {
    const cases: Array<[string, WorkerMutationClass]> = [
      ["gh issue view 10", "github"],
      ["git add src/file.ts", "git"],
      ["git commit -m changed", "git"],
      ["git checkout main", "git"],
      ["npm install --global typescript", "global-installation"],
      ["pnpm add -g typescript", "global-installation"],
      ["cargo install ripgrep", "global-installation"],
      ["npm config set registry https://example.invalid", "user-configuration"],
      [`npm install --prefix ${paths.external}`, "external-path"],
      ["command gh issue view 10", "github"],
      ["sh -c 'git add src/file.ts'", "shell"],
      [`touch ${join(paths.external, "escape.txt")}`, "external-path"],
      [`printf changed > ${join(paths.home, ".npmrc")}`, "user-configuration"],
      ["touch ../escape.txt", "external-path"],
    ];

    for (const [command, mutationClass] of cases) {
      const decision = await inspectWorkerCommand(scope, command);
      assert.equal(decision.allowed, false, command);
      if (!decision.allowed) {
        assert.equal(decision.mutationClass, mutationClass, command);
        assert.match(decision.reason, /Worker Guard blocked/);
      }
    }
  });
});

test("blocks direct path escape, Git internals, and symlink escape", async () => {
  await withScope(async (scope, paths) => {
    const link = join(paths.project, "linked-outside");
    await symlink(paths.external, link);

    const cases: Array<[string, WorkerMutationClass]> = [
      [join(paths.external, "file.txt"), "external-path"],
      [join(paths.home, ".config", "settings.json"), "user-configuration"],
      [join(paths.project, ".git", "index"), "git"],
      [join(link, "escaped.txt"), "external-path"],
    ];
    for (const [path, mutationClass] of cases) {
      const decision = await inspectWorkerPath(scope, path);
      assert.equal(decision.allowed, false, path);
      if (!decision.allowed) {
        assert.equal(decision.mutationClass, mutationClass, path);
      }
    }
  });
});
