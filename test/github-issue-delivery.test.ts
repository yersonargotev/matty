import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  createGithubIssueDeliveryQualifier,
  type IssueDeliveryCommandReader,
} from "../src/adapters/github-issue-delivery.ts";

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
