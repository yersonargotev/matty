import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  createGithubIssueDelivery,
  createGithubIssueDeliveryQualifier,
  type IssueDeliveryCommandReader,
} from "../src/adapters/github-issue-delivery.ts";
import type { GitDeliveryCommandRunner } from "../src/adapters/git-issue-delivery-workspace.ts";
import { deliveryIdentityKey } from "../src/application/issue-delivery-workspace.ts";

async function prepareRepository(root: string) {
  await mkdir(join(root, "docs/agents"), { recursive: true });
  await writeFile(
    join(root, "AGENTS.md"),
    "See docs/agents/issue-tracker.md, docs/agents/triage-labels.md, and docs/agents/domain.md.\n",
  );
  await writeFile(join(root, "CONTEXT.md"), "# Context\n");
  await writeFile(
    join(root, "docs/agents/issue-tracker.md"),
    "Issues and PRDs for this repo live as GitHub issues.\n",
  );
  await writeFile(
    join(root, "docs/agents/triage-labels.md"),
    "| Label in mattpocock/skills | Label in our tracker | Meaning |\n| --- | --- | --- |\n| `ready-for-agent` | `ready-to-build` | Ready |\n",
  );
  await writeFile(join(root, "docs/agents/domain.md"), "# Domain Docs\n");
}

test("production status binds checks to the exact candidate SHA and emits closed facts", async () => {
  const identity = {
    repository: "github.com/yersonargotev/matty",
    tracker: "github" as const,
    issue: 36,
  };
  const key = deliveryIdentityKey(identity);
  const branch = `matty/deliver-36-${key.slice(0, 8)}`;
  const record = {
    schemaVersion: 1 as const,
    status: "active" as const,
    key,
    identity,
    branch,
    path: "/repo",
    isolation: "in-place" as const,
    startingCheckout: { root: "/repo", ref: "main", sha: "0000000000000000000000000000000000000000" },
    integration: { branch: "main", sha: "0000000000000000000000000000000000000000" },
  };
  const gitCalls: string[][] = [];
  const runGit: GitDeliveryCommandRunner = async (args) => {
    gitCalls.push(args);
    if (args[0] === "rev-parse" && args[1] === "--show-toplevel") return "/repo";
    if (args[0] === "rev-parse" && args[1] === "--verify") {
      if (args.at(-1) === "refs/matty/issue-delivery/active") return "active-object";
      if (args.at(-1) === `refs/matty/issue-delivery/owners/${key}`) return "owner-object";
      if (args.at(-1) === `refs/heads/${branch}`) return "1111111111111111111111111111111111111111";
    }
    if (args[0] === "cat-file") return JSON.stringify(record);
    throw new Error(`unexpected git read: ${args.join(" ")}`);
  };
  const calls: Array<{ command: string; args: string[] }> = [];
  const runCommand: IssueDeliveryCommandReader = async (command, args) => {
    calls.push({ command, args });
    if (args[1] === "repos/yersonargotev/matty/issues/36") {
      return JSON.stringify({ number: 36, state: "open", title: "token=secret" });
    }
    if (args[1]?.startsWith("repos/yersonargotev/matty/pulls?")) {
      return JSON.stringify([{ head: { sha: "1111111111111111111111111111111111111111", ref: branch }, html_url: "https://secret.invalid" }]);
    }
    if (args[1] === "repos/yersonargotev/matty/commits/1111111111111111111111111111111111111111/check-runs") {
      return JSON.stringify({ check_runs: [
        { status: "completed", conclusion: "success", name: "hostile secret check" },
        { status: "queued", conclusion: null, output: { text: "/private/token" } },
      ] });
    }
    throw new Error("unexpected command");
  };

  const outcome = await createGithubIssueDelivery({}, runCommand, runGit)(
    { intent: "status", issue: "36", cwd: "/repo" },
  );

  assert.equal(outcome.status, "active");
  assert.doesNotMatch(JSON.stringify(outcome), /hostile|private|secret|https/);
  assert.deepEqual(calls.map(({ args }) => args[1]), [
    "repos/yersonargotev/matty/issues/36",
    `repos/yersonargotev/matty/pulls?state=all&head=${encodeURIComponent(`yersonargotev:${branch}`)}&per_page=100`,
    "repos/yersonargotev/matty/commits/1111111111111111111111111111111111111111/check-runs",
  ]);
  assert.equal(gitCalls.some((args) => ["hash-object", "update-ref", "switch"].includes(args[0]!)), false);
});

test("blocked production qualification performs only Git and GitHub reads", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-delivery-test-"));
  const home = join(root, "home");
  const calls: Array<{ command: string; args: string[] }> = [];
  const runCommand: IssueDeliveryCommandReader = async (command, args) => {
    calls.push({ command, args });
    if (command === "git" && args[0] === "rev-parse") return root;
    if (command === "git" && args[0] === "remote") {
      return "https://github.com/yersonargotev/matty.git";
    }
    if (command === "gh" && args[0] === "--version") return "gh version";
    if (command === "gh" && args[0] === "auth") return "authenticated";
    if (command === "gh" && args[0] === "api") {
      return JSON.stringify({
        number: 34,
        state: "open",
        labels: [{ name: "ready-to-build" }],
        html_url: "https://github.com/yersonargotev/matty/issues/34",
      });
    }
    throw new Error("unexpected command");
  };

  try {
    await prepareRepository(root);
    const before = await readFile(join(root, "AGENTS.md"), "utf8");
    const outcome = await createGithubIssueDeliveryQualifier(
      { HOME: home },
      runCommand,
    )({ intent: "deliver", issue: "34", cwd: root });

    assert.equal(outcome.status, "blocked");
    assert.equal(await readFile(join(root, "AGENTS.md"), "utf8"), before);
    assert.deepEqual(calls, [
      { command: "git", args: ["rev-parse", "--show-toplevel"] },
      { command: "git", args: ["remote", "get-url", "origin"] },
      { command: "gh", args: ["--version"] },
      {
        command: "gh",
        args: ["auth", "status", "--hostname", "github.com"],
      },
      {
        command: "gh",
        args: ["api", "repos/yersonargotev/matty/issues/34"],
      },
    ]);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
