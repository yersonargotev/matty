import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import test from "node:test";

import {
  createGitIssueDeliveryWorkspace,
  type GitDeliveryCommandRunner,
} from "../src/adapters/git-issue-delivery-workspace.ts";
import { runInspectionDelegation } from "../src/application/inspection-role-delegation.ts";
import {
  deliverIssue,
  type IssueDeliveryPreflight,
} from "../src/application/issue-delivery.ts";
import { deliveryIdentityKey } from "../src/application/issue-delivery-workspace.ts";
import { INSPECTION_TOOLS } from "../src/domain/capability-contract.ts";
import {
  ISSUE_DELIVERY_WORKFLOW,
  type DeliveryIdentity,
} from "../src/domain/issue-delivery.ts";

const execFileAsync = promisify(execFile);
const identity: DeliveryIdentity = {
  repository: "github.com/yersonargotev/matty",
  tracker: "github",
  issue: 35,
};
const key = "4533aa2af6ba5a0fdc6550948150a95638ef5a61c0a4241ca3e5b26c995d6727";
const activeRef = "refs/matty/issue-delivery/active";
const ownerRef = `refs/matty/issue-delivery/owners/${key}`;

function gitFailure(message: string, exitCode: number): Error & { exitCode: number } {
  return Object.assign(new Error(message), { exitCode });
}

function fakeGit(options: {
  fail?: (args: string[]) => Error | undefined;
  detached?: boolean;
  remote?: string;
  ancestor?: boolean;
} = {}) {
  const refs = new Map<string, string>();
  const objects = new Map<string, string>();
  const calls: Array<{ args: string[]; input?: string }> = [];
  let currentBranch = "main";
  let objectNumber = 0;

  const run: GitDeliveryCommandRunner = async (args, _cwd, input) => {
    calls.push({ args, ...(input === undefined ? {} : { input }) });
    const failure = options.fail?.(args);
    if (failure) throw failure;
    if (args[0] === "rev-parse" && args[1] === "--show-toplevel") return "/repo";
    if (args[0] === "remote" && args[1] === "get-url") {
      return options.remote ?? "git@github.com:yersonargotev/matty.git";
    }
    if (args[0] === "merge-base" && args[1] === "--is-ancestor") {
      if (options.ancestor === false) throw gitFailure("not an ancestor", 1);
      return "";
    }
    if (args[0] === "rev-parse" && args[1] === "HEAD") return "base-sha";
    if (args[0] === "rev-parse" && args[1] === "refs/remotes/origin/main") return "base-sha";
    if (args[0] === "rev-parse" && args[1] === "--verify") {
      const value = refs.get(args.at(-1)!);
      if (!value) throw gitFailure("missing ref", 1);
      return value;
    }
    if (args[0] === "symbolic-ref" && args.at(-1) === "HEAD") {
      if (options.detached) throw gitFailure("detached HEAD", 1);
      return currentBranch;
    }
    if (args[0] === "symbolic-ref") return "origin/main";
    if (args[0] === "status") return "";
    if (args[0] === "worktree" && args[1] === "list") {
      return `worktree /repo\nHEAD base-sha\nbranch refs/heads/${currentBranch}\n`;
    }
    if (args[0] === "show-ref") {
      const ref = args.at(-1)!;
      const value = refs.get(ref);
      if (!value) throw gitFailure("missing branch", 1);
      return `${value} ${ref}`;
    }
    if (args[0] === "hash-object") {
      assert.equal(typeof input, "string");
      const object = objectNumber++ === 0 ? "ownership-blob" : `ownership-blob-${objectNumber}`;
      objects.set(object, input!);
      return object;
    }
    if (args[0] === "cat-file") {
      const value = objects.get(args[2]!);
      if (!value) throw new Error("missing object");
      return value;
    }
    if (args[0] === "update-ref" && args[1] === "--stdin") {
      assert.equal(typeof input, "string");
      const staged = new Map(refs);
      for (const line of input!.trimEnd().split("\n")) {
        const [operation, ref, value] = line.split(" ");
        if (operation === "create") {
          if (staged.has(ref!)) throw new Error("compare-and-swap failed");
          staged.set(ref!, value!);
        } else if (!["start", "prepare", "commit"].includes(operation!)) {
          throw new Error(`unexpected transaction command: ${line}`);
        }
      }
      refs.clear();
      for (const [ref, value] of staged) refs.set(ref, value);
      return "";
    }
    if (args[0] === "switch") {
      currentBranch = args[2]!;
      refs.set(`refs/heads/${currentBranch}`, args[3]!);
      return "";
    }
    if (args[0] === "worktree" && args[1] === "add") return "";
    throw new Error(`unexpected git command: ${args.join(" ")}`);
  };

  return { refs, objects, calls, run, currentBranch: () => currentBranch };
}

function readyPreflight(): IssueDeliveryPreflight {
  return {
    github: { available: true, authenticated: true },
    repository: {
      trusted: true,
      prepared: true,
      tracker: "github",
      canonical: identity.repository,
      readyLabel: "ready-for-agent",
    },
    issueInspection: "available",
    issue: {
      kind: "issue",
      number: identity.issue,
      state: "open",
      labels: ["ready-for-agent"],
      url: "https://github.com/yersonargotev/matty/issues/35",
    },
    skills: ISSUE_DELIVERY_WORKFLOW.dependencies.map((dependency) => ({
      id: dependency.id,
      identity: dependency.id,
      provenance: dependency.provenance,
      digest: dependency.digest,
    })),
  };
}

async function deliverWith(run: GitDeliveryCommandRunner) {
  return await deliverIssue(
    { intent: "deliver", issue: "35", cwd: "/repo" },
    async () => readyPreflight(),
    createGitIssueDeliveryWorkspace(run),
  );
}

test("the Git adapter atomically publishes blob-valued owner and active refs before preparation", async () => {
  const git = fakeGit();

  const result = await createGitIssueDeliveryWorkspace(git.run).prepare({
    cwd: "/repo",
    identity,
  });

  assert.equal(result.status, "prepared");
  assert.equal(git.currentBranch(), "matty/deliver-35-4533aa2a");
  assert.equal(git.refs.get(activeRef), "ownership-blob");
  assert.equal(git.refs.get(ownerRef), "ownership-blob");
  const transactionIndex = git.calls.findIndex(({ args }) =>
    args[0] === "update-ref" && args[1] === "--stdin"
  );
  const switchIndex = git.calls.findIndex(({ args }) => args[0] === "switch");
  assert.ok(transactionIndex >= 0 && transactionIndex < switchIndex);
  assert.equal(
    git.calls[transactionIndex]!.input,
    `start\ncreate ${ownerRef} ownership-blob\ncreate ${activeRef} ownership-blob\nprepare\ncommit\n`,
  );
  assert.equal(
    git.calls.filter(({ args }) => args[0] === "update-ref").length,
    1,
    "no provisional commit-valued active ref is published",
  );
});

test("active delivery inspection reads owned markers and candidate without Git effects", async () => {
  const git = fakeGit();
  const workspace = createGitIssueDeliveryWorkspace(git.run);
  const prepared = await workspace.prepare({ cwd: "/repo", identity });
  assert.equal(prepared.status, "prepared");
  const before = git.calls.length;

  const restartedWorkspace = createGitIssueDeliveryWorkspace(git.run);
  const result = await restartedWorkspace.inspect({ cwd: "/repo", issue: 35 });

  assert.deepEqual(result, {
    status: "active",
    delivery: {
      identity,
      branch: "matty/deliver-35-4533aa2a",
      integrationBranch: "main",
      integrationSha: "base-sha",
      candidateSha: null,
    },
  });
  const reads = git.calls.slice(before);
  assert.equal(reads.some(({ args }) =>
    ["hash-object", "update-ref", "switch"].includes(args[0]!) ||
    (args[0] === "worktree" && args[1] === "add")
  ), false);
});

test("active re-entry rejects a canonical origin that disagrees with the Delivery Identity before effects", async () => {
  const git = fakeGit({ remote: "https://github.com/other/REPOSITORY.git" });
  const workspace = createGitIssueDeliveryWorkspace(git.run);
  assert.equal((await workspace.prepare({ cwd: "/repo", identity })).status, "prepared");
  const before = git.calls.length;

  const result = await workspace.inspect({ cwd: "/repo", issue: 35 });

  assert.equal(result.status, "blocked");
  if (result.status === "blocked") {
    assert.deepEqual(result.exceptionBrief.evidence, ["delivery-ownership-mismatch"]);
  }
  const reads = git.calls.slice(before);
  assert.equal(reads.some(({ args }) =>
    ["hash-object", "update-ref", "switch"].includes(args[0]!) ||
    (args[0] === "worktree" && args[1] === "add")
  ), false);
  assert.equal(JSON.stringify(result).includes("other/REPOSITORY"), false);
});

test("active re-entry rejects an unparseable canonical origin before effects", async () => {
  const git = fakeGit({ remote: "file:///private/hostile/token" });
  const workspace = createGitIssueDeliveryWorkspace(git.run);
  assert.equal((await workspace.prepare({ cwd: "/repo", identity })).status, "prepared");
  const before = git.calls.length;

  const result = await workspace.inspect({ cwd: "/repo", issue: 35 });

  assert.equal(result.status, "blocked");
  if (result.status === "blocked") {
    assert.deepEqual(result.exceptionBrief.evidence, ["delivery-ownership-mismatch"]);
  }
  assert.doesNotMatch(JSON.stringify(result), /private|hostile|token/);
  assert.equal(git.calls.slice(before).some(({ args }) =>
    ["hash-object", "update-ref", "switch"].includes(args[0]!)
  ), false);
});

test("a stale or diverged owned branch blocks before it becomes a candidate", async () => {
  const git = fakeGit({ ancestor: false });
  const workspace = createGitIssueDeliveryWorkspace(git.run);
  assert.equal((await workspace.prepare({ cwd: "/repo", identity })).status, "prepared");
  git.refs.set(`refs/heads/matty/deliver-35-${key.slice(0, 8)}`, "stale-local-sha");
  const before = git.calls.length;

  const result = await workspace.inspect({ cwd: "/repo", issue: 35 });

  assert.equal(result.status, "blocked");
  if (result.status === "blocked") {
    assert.deepEqual(result.exceptionBrief.evidence, ["delivery-candidate-ancestry-unproven"]);
  }
  assert.doesNotMatch(JSON.stringify(result), /stale-local-sha|not an ancestor/);
  assert.equal(git.calls.slice(before).some(({ args }) =>
    ["hash-object", "update-ref", "switch"].includes(args[0]!) ||
    (args[0] === "worktree" && args[1] === "add")
  ), false);
});

test("stale active state with a missing owner marker blocks read-only inspection", async () => {
  const git = fakeGit();
  const workspace = createGitIssueDeliveryWorkspace(git.run);
  assert.equal((await workspace.prepare({ cwd: "/repo", identity })).status, "prepared");
  git.refs.delete(ownerRef);
  const before = git.calls.length;

  const result = await workspace.inspect({ cwd: "/repo", issue: 35 });

  assert.equal(result.status, "blocked");
  if (result.status === "blocked") {
    assert.deepEqual(result.exceptionBrief.evidence, ["delivery-ownership-mismatch"]);
  }
  assert.equal(git.calls.slice(before).some(({ args }) =>
    ["hash-object", "update-ref", "switch"].includes(args[0]!)
  ), false);
});

test("expected missing active, owner, and branch refs allow normal preparation", async () => {
  const git = fakeGit();

  const result = await deliverWith(git.run);

  assert.equal(result.status, "prepared");
  assert.equal(git.calls.some(({ args }) => args[0] === "update-ref"), true);
  assert.equal(git.calls.some(({ args }) => args[0] === "switch"), true);
});

test("a detached symbolic HEAD remains an expected absence and prepares an isolated workspace", async () => {
  const git = fakeGit({ detached: true });

  const result = await deliverWith(git.run);

  assert.equal(result.status, "prepared");
  assert.equal(git.calls.some(({ args }) => args[0] === "update-ref"), true);
  assert.equal(git.calls.some(({ args }) => args[0] === "worktree" && args[1] === "add"), true);
  assert.equal(git.calls.some(({ args }) => args[0] === "switch"), false);
});

test("non-missing Git inspection failures block without ref or workspace effects", async (context) => {
  const cases: Array<[string, (args: string[]) => boolean]> = [
    ["active ref", (args) => args[0] === "rev-parse" && args.at(-1) === activeRef],
    ["owner ref", (args) => args[0] === "rev-parse" && args.at(-1) === ownerRef],
    ["branch", (args) => args[0] === "show-ref"],
    ["symbolic ref", (args) => args[0] === "symbolic-ref" && args.at(-1) === "HEAD"],
  ];

  for (const [name, matches] of cases) {
    await context.test(name, async () => {
      const git = fakeGit({
        fail: (args) => matches(args) ? gitFailure("inspection denied", 128) : undefined,
      });

      const result = await deliverWith(git.run);

      assert.equal(result.status, "blocked");
      if (result.status === "blocked") {
        assert.deepEqual(result.exceptionBrief.evidence, [
          name === "active ref"
            ? "delivery-inspection-unavailable"
            : "workspace-preparation-failed",
        ]);
        assert.equal(JSON.stringify(result).includes("inspection denied"), false);
      }
      assert.equal(git.calls.some(({ args }) => args[0] === "update-ref"), false);
      assert.equal(git.calls.some(({ args }) => args[0] === "switch"), false);
      assert.equal(
        git.calls.some(({ args }) => args[0] === "worktree" && args[1] === "add"),
        false,
      );
    });
  }
});

test("a final owner inspection failure occurs before any Git mutation", async () => {
  let ownerInspections = 0;
  const git = fakeGit({
    fail: (args) => {
      if (args[0] !== "rev-parse" || args.at(-1) !== ownerRef) return undefined;
      ownerInspections += 1;
      return ownerInspections === 2
        ? gitFailure("final owner inspection denied", 128)
        : undefined;
    },
  });

  const result = await deliverWith(git.run);

  assert.equal(result.status, "blocked");
  assert.equal(ownerInspections, 2);
  for (const command of ["hash-object", "update-ref", "switch"] as const) {
    assert.equal(
      git.calls.some(({ args }) => args[0] === command),
      false,
      `${command} must not run`,
    );
  }
  assert.equal(
    git.calls.some(({ args }) => args[0] === "worktree" && args[1] === "add"),
    false,
    "worktree add must not run",
  );
});

test("the Git adapter rejects mismatched starting and integration authority without preparation", async (context) => {
  for (const field of ["starting checkout", "integration"] as const) {
    await context.test(field, async () => {
      const git = fakeGit();
      const active = {
        schemaVersion: 1,
        status: "active",
        key,
        identity,
        branch: "matty/deliver-35-4533aa2a",
        path: "/repo",
        isolation: "in-place",
        startingCheckout: { root: "/repo", ref: "main", sha: "base-sha" },
        integration: { branch: "main", sha: "base-sha" },
      };
      const mismatched = structuredClone(active);
      if (field === "starting checkout") mismatched.startingCheckout.sha = "other-start";
      else mismatched.integration.sha = "other-integration";
      git.objects.set("active-record", JSON.stringify(active));
      git.objects.set("mismatched-owner", JSON.stringify(mismatched));
      git.refs.set(activeRef, "active-record");
      git.refs.set(ownerRef, "mismatched-owner");

      const result = await createGitIssueDeliveryWorkspace(git.run).prepare({
        cwd: "/repo",
        identity,
      });

      assert.equal(result.status, "blocked");
      if (result.status === "blocked") {
        assert.deepEqual(result.exceptionBrief.evidence, ["delivery-ownership-mismatch"]);
      }
      assert.equal(git.calls.some(({ args }) => args[0] === "switch"), false);
      assert.equal(git.calls.some(({ args }) => args[0] === "update-ref"), false);
    });
  }
});

test("concurrent differing claims atomically admit one winner", async () => {
  const git = fakeGit();
  const workspace = createGitIssueDeliveryWorkspace(git.run);

  const outcomes = await Promise.all([
    workspace.prepare({ cwd: "/repo", identity }),
    workspace.prepare({ cwd: "/repo", identity: { ...identity, issue: 36 } }),
  ]);

  assert.deepEqual(outcomes.map(({ status }) => status).sort(), ["blocked", "prepared"]);
  assert.equal(git.calls.filter(({ args }) => args[0] === "switch").length, 1);
  const activeObject = git.refs.get(activeRef)!;
  const active = JSON.parse(git.objects.get(activeObject)!) as { key: string };
  assert.ok([
    deliveryIdentityKey(identity),
    deliveryIdentityKey({ ...identity, issue: 36 }),
  ].includes(active.key));
});

async function git(cwd: string, ...args: string[]): Promise<string> {
  const { stdout } = await execFileAsync("git", args, { cwd, encoding: "utf8" });
  return stdout.trim();
}

async function temporaryRepository(): Promise<{ container: string; repository: string; integrationSha: string }> {
  const container = await mkdtemp(join(tmpdir(), "matty-delivery-git-"));
  const repository = join(container, "repository");
  await mkdir(repository);
  await git(repository, "init", "-b", "main");
  await git(repository, "remote", "add", "origin", "https://github.com/YersonArgoteV/Matty.git");
  await git(repository, "config", "user.name", "Matty Test");
  await git(repository, "config", "user.email", "matty@example.invalid");
  await writeFile(join(repository, "tracked.txt"), "initial\n");
  await git(repository, "add", "tracked.txt");
  await git(repository, "commit", "-m", "initial");
  await writeFile(join(repository, "tracked.txt"), "base\n");
  await git(repository, "add", "tracked.txt");
  await git(repository, "commit", "-m", "base");
  const integrationSha = await git(repository, "rev-parse", "HEAD");
  await git(repository, "update-ref", "refs/remotes/origin/main", integrationSha);
  await git(
    repository,
    "symbolic-ref",
    "refs/remotes/origin/HEAD",
    "refs/remotes/origin/main",
  );
  return { container, repository, integrationSha };
}

test("a dirty real checkout is preserved while a linked worktree starts at integration", async () => {
  const fixture = await temporaryRepository();
  try {
    await writeFile(join(fixture.repository, "tracked.txt"), "dirty tracked\n");
    await writeFile(join(fixture.repository, "untracked.txt"), "dirty untracked\n");
    const before = {
      status: await git(fixture.repository, "status", "--porcelain=v1", "--untracked-files=normal"),
      head: await git(fixture.repository, "rev-parse", "HEAD"),
      branch: await git(fixture.repository, "symbolic-ref", "--short", "HEAD"),
    };

    const result = await createGitIssueDeliveryWorkspace().prepare({
      cwd: fixture.repository,
      identity,
    });

    assert.equal(result.status, "prepared");
    if (result.status === "prepared") {
      assert.equal(result.workspace.isolation, "worktree");
      assert.notEqual(result.workspace.path, fixture.repository);
      assert.equal(await git(result.workspace.path, "rev-parse", "HEAD"), fixture.integrationSha);
      assert.equal(
        await git(result.workspace.path, "symbolic-ref", "--short", "HEAD"),
        result.workspace.branch,
      );
    }
    assert.deepEqual({
      status: await git(fixture.repository, "status", "--porcelain=v1", "--untracked-files=normal"),
      head: await git(fixture.repository, "rev-parse", "HEAD"),
      branch: await git(fixture.repository, "symbolic-ref", "--short", "HEAD"),
    }, before);
    assert.equal(await readFile(join(fixture.repository, "tracked.txt"), "utf8"), "dirty tracked\n");
    assert.equal(await readFile(join(fixture.repository, "untracked.txt"), "utf8"), "dirty untracked\n");
  } finally {
    await rm(fixture.container, { recursive: true, force: true });
  }
});

test("a lookalike unowned branch blocks in a real repository", async () => {
  const fixture = await temporaryRepository();
  try {
    await git(fixture.repository, "branch", `matty/deliver-35-${key.slice(0, 8)}`, fixture.integrationSha);

    const result = await createGitIssueDeliveryWorkspace().prepare({
      cwd: fixture.repository,
      identity,
    });

    assert.equal(result.status, "blocked");
    if (result.status === "blocked") {
      assert.deepEqual(result.exceptionBrief.evidence, ["delivery-ownership-mismatch"]);
    }
    await assert.rejects(git(fixture.repository, "rev-parse", "--verify", activeRef));
    assert.equal(await git(fixture.repository, "symbolic-ref", "--short", "HEAD"), "main");
  } finally {
    await rm(fixture.container, { recursive: true, force: true });
  }
});

test("real Git blocks behind and diverged owned branches without inspection effects", async (context) => {
  for (const state of ["behind", "diverged"] as const) {
    await context.test(state, async () => {
      const fixture = await temporaryRepository();
      try {
        const workspace = createGitIssueDeliveryWorkspace();
        const prepared = await workspace.prepare({ cwd: fixture.repository, identity });
        assert.equal(prepared.status, "prepared");
        if (prepared.status !== "prepared") return;

        await git(fixture.repository, "switch", "main");
        if (state === "behind") {
          await git(fixture.repository, "branch", "older-history", `${fixture.integrationSha}^`);
          await git(fixture.repository, "switch", "older-history");
        } else {
          await git(fixture.repository, "switch", "--orphan", "diverged-history");
          await writeFile(join(fixture.repository, "tracked.txt"), "diverged\n");
          await git(fixture.repository, "add", "tracked.txt");
          await git(fixture.repository, "commit", "-m", "diverged root");
        }
        const staleSha = await git(fixture.repository, "rev-parse", "HEAD");
        await git(fixture.repository, "switch", "main");
        await git(fixture.repository, "branch", "-f", prepared.workspace.branch, staleSha);
        const before = {
          refs: await git(fixture.repository, "for-each-ref", "--format=%(refname) %(objectname)"),
          head: await git(fixture.repository, "rev-parse", "HEAD"),
          status: await git(fixture.repository, "status", "--porcelain=v1"),
        };

        const result = await workspace.inspect({ cwd: fixture.repository, issue: identity.issue });

        assert.equal(result.status, "blocked");
        if (result.status === "blocked") {
          assert.deepEqual(result.exceptionBrief.evidence, ["delivery-candidate-ancestry-unproven"]);
        }
        assert.deepEqual({
          refs: await git(fixture.repository, "for-each-ref", "--format=%(refname) %(objectname)"),
          head: await git(fixture.repository, "rev-parse", "HEAD"),
          status: await git(fixture.repository, "status", "--porcelain=v1"),
        }, before);
      } finally {
        await rm(fixture.container, { recursive: true, force: true });
      }
    });
  }
});

test("two concurrent real claims produce one prepared delivery and one blocked delivery", async () => {
  const fixture = await temporaryRepository();
  try {
    const workspace = createGitIssueDeliveryWorkspace();
    const outcomes = await Promise.all([
      workspace.prepare({ cwd: fixture.repository, identity }),
      workspace.prepare({ cwd: fixture.repository, identity: { ...identity, issue: 36 } }),
    ]);

    assert.deepEqual(outcomes.map(({ status }) => status).sort(), ["blocked", "prepared"]);
    assert.equal((await git(fixture.repository, "for-each-ref", "--format=%(refname)", "refs/heads/matty/")).split("\n").filter(Boolean).length, 1);
  } finally {
    await rm(fixture.container, { recursive: true, force: true });
  }
});

test("an active delivery does not block Core read-only inspection", async () => {
  const fixture = await temporaryRepository();
  try {
    const result = await createGitIssueDeliveryWorkspace().prepare({
      cwd: fixture.repository,
      identity,
    });
    assert.equal(result.status, "prepared");

    const inspection = await runInspectionDelegation(
      "explorer",
      "Inspect repository status",
      {
        availability: {
          availableTools: INSPECTION_TOOLS,
          independentRuntime: true,
          inspectionGuard: true,
        },
        createRunner: () => ({
          async run() {
            return {
              status: "succeeded",
              child: { pid: 42, runId: "inspection-during-delivery" },
              output: await git(fixture.repository, "status", "--short"),
              exit: { code: 0, signal: null },
            };
          },
        }),
      },
    );

    assert.equal(inspection.outcome.status, "succeeded");
    assert.equal(await git(fixture.repository, "rev-parse", "--verify", activeRef) !== "", true);
  } finally {
    await rm(fixture.container, { recursive: true, force: true });
  }
});
