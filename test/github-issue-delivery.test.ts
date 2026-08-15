import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  createGithubIssueDelivery,
  type IssueDeliveryCommandReader,
} from "../src/adapters/github-issue-delivery.ts";
import type { GitDeliveryCommandRunner } from "../src/adapters/git-issue-delivery-workspace.ts";
import { deliveryIdentityKey } from "../src/application/issue-delivery-workspace.ts";
import { initialRepairBudget } from "../src/domain/issue-delivery.ts";
import { commitSha } from "../src/domain/commit-sha.ts";

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
    startingCheckout: { root: "/repo", ref: "main", sha: commitSha("0000000000000000000000000000000000000000") },
    integration: { branch: "main", sha: commitSha("0000000000000000000000000000000000000000") },
    repairBudget: initialRepairBudget(),
  };
  const gitCalls: string[][] = [];
  const runGit: GitDeliveryCommandRunner = async (args) => {
    gitCalls.push(args);
    if (args[0] === "rev-parse" && args[1] === "--show-toplevel") return "/repo";
    if (args[0] === "remote" && args[1] === "get-url") {
      return "https://github.com/YersonArgoteV/Matty.git";
    }
    if (args[0] === "merge-base" && args[1] === "--is-ancestor") return "";
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
      return JSON.stringify([[]]);
    }
    if (args[1] === `repos/yersonargotev/matty/git/ref/heads/${encodeURIComponent(branch)}`) {
      return JSON.stringify({ object: { sha: "1111111111111111111111111111111111111111" } });
    }
    if (args[1] === "repos/yersonargotev/matty/git/ref/heads/main") {
      return JSON.stringify({ object: { sha: "0000000000000000000000000000000000000000" }, url: "https://secret.invalid/main" });
    }
    if (args[1] === "repos/yersonargotev/matty/commits/1111111111111111111111111111111111111111/check-runs?per_page=100") {
      return JSON.stringify([
        { total_count: 2, check_runs: [
          { status: "queued", conclusion: null, output: { text: "/private/token" } },
        ] },
        { total_count: 2, check_runs: [
          { status: "completed", conclusion: "startup_failure", name: "hostile secret check" },
        ] },
      ]);
    }
    if (args[1] === "repos/yersonargotev/matty/commits/1111111111111111111111111111111111111111/status?per_page=100") {
      return JSON.stringify([
        { total_count: 2, statuses: [
          { state: "failure", context: "hostile classic secret", description: "/private/classic", target_url: "https://secret.invalid/status" },
        ] },
        { total_count: 2, statuses: [
          { state: "pending", context: "waiting secret" },
        ] },
      ]);
    }
    throw new Error("unexpected command");
  };

  const outcome = await createGithubIssueDelivery({}, runCommand, runGit)(
    { intent: "status", issue: "36", cwd: "/repo" },
  );

  assert.equal(outcome.status, "active");
  assert.doesNotMatch(JSON.stringify(outcome), /hostile|private|secret|https/);
  if (outcome.status === "active") {
    assert.deepEqual(outcome.checks, {
      state: "failing", total: 4, passed: 0, pending: 2, failed: 2,
    });
    assert.deepEqual(outcome.blockers, ["checks-failing", "checks-pending"]);
  }
  assert.deepEqual(calls.map(({ args }) => args[1]), [
    "repos/yersonargotev/matty/issues/36",
    `repos/yersonargotev/matty/pulls?state=all&head=${encodeURIComponent(`yersonargotev:${branch}`)}&per_page=100`,
    `repos/yersonargotev/matty/git/ref/heads/${encodeURIComponent(branch)}`,
    "repos/yersonargotev/matty/git/ref/heads/main",
    "repos/yersonargotev/matty/commits/1111111111111111111111111111111111111111/check-runs?per_page=100",
    "repos/yersonargotev/matty/commits/1111111111111111111111111111111111111111/status?per_page=100",
  ]);
  for (const call of calls.filter(({ args }) => args[0] === "api" && /(?:pulls\?|check-runs|\/status\?)/.test(args[1]!))) {
    assert.deepEqual(call.args.slice(2), ["--paginate", "--slurp"]);
  }
  assert.equal(gitCalls.some((args) => ["hash-object", "update-ref", "switch"].includes(args[0]!)), false);
  assert.equal(gitCalls.some((args) => args[0] === "merge-base" && args[1] === "--is-ancestor"), true);
});

test("an unpublished local candidate does not query GitHub checks or statuses", async () => {
  const candidateSha = commitSha("1111111111111111111111111111111111111111");
  const baseSha = commitSha("0000000000000000000000000000000000000000");
  const identity = {
    repository: "github.com/yersonargotev/matty",
    tracker: "github" as const,
    issue: 36,
  };
  const branch = "matty/deliver-36-owned";
  const workspace = {
    inspect: async () => ({
      status: "active" as const,
      delivery: { identity, branch, integrationBranch: "main", integrationSha: baseSha, candidateSha },
    }),
    prepare: async () => { throw new Error("active delivery must not prepare"); },
  };
  const calls: string[] = [];
  const runCommand: IssueDeliveryCommandReader = async (_command, args) => {
    const endpoint = args[1]!;
    calls.push(endpoint);
    if (endpoint === "repos/yersonargotev/matty/issues/36") {
      return JSON.stringify({ number: 36, state: "open" });
    }
    if (endpoint.startsWith("repos/yersonargotev/matty/pulls?")) return JSON.stringify([[]]);
    if (endpoint === `repos/yersonargotev/matty/git/ref/heads/${encodeURIComponent(branch)}`) {
      throw Object.assign(new Error("missing branch token=secret"), { stderr: "HTTP 404" });
    }
    if (endpoint === "repos/yersonargotev/matty/git/ref/heads/main") {
      return JSON.stringify({ object: { sha: baseSha } });
    }
    throw new Error(`candidate-bound endpoint must not be queried: ${endpoint}`);
  };

  const outcome = await createGithubIssueDelivery({}, runCommand, undefined, workspace)(
    { intent: "status", issue: "36", cwd: "/repo" },
  );

  assert.deepEqual(outcome, {
    schemaVersion: 1,
    status: "active",
    deliveryIdentity: identity,
    gate: "verification",
    candidateSha,
    candidateState: "local-unpublished",
    repairBudget: initialRepairBudget(),
    checks: { state: "none", total: 0, passed: 0, pending: 0, failed: 0 },
    blockers: [],
  });
  assert.equal(calls.some((endpoint) => /check-runs|\/status\?/.test(endpoint)), false);
});

test("a remote delivery candidate mismatch returns drift without querying stale checks", async () => {
  const candidateSha = commitSha("1111111111111111111111111111111111111111");
  const remoteSha = commitSha("2222222222222222222222222222222222222222");
  const baseSha = commitSha("0000000000000000000000000000000000000000");
  const identity = {
    repository: "github.com/yersonargotev/matty",
    tracker: "github" as const,
    issue: 36,
  };
  const branch = "matty/deliver-36-owned";
  const workspace = {
    inspect: async () => ({
      status: "active" as const,
      delivery: { identity, branch, integrationBranch: "main", integrationSha: baseSha, candidateSha },
    }),
    prepare: async () => { throw new Error("active delivery must not prepare"); },
  };
  const calls: string[] = [];
  const runCommand: IssueDeliveryCommandReader = async (_command, args) => {
    const endpoint = args[1]!;
    calls.push(endpoint);
    if (endpoint === "repos/yersonargotev/matty/issues/36") {
      return JSON.stringify({ number: 36, state: "open" });
    }
    if (endpoint.startsWith("repos/yersonargotev/matty/pulls?")) return JSON.stringify([[]]);
    if (endpoint === `repos/yersonargotev/matty/git/ref/heads/${encodeURIComponent(branch)}`) {
      return JSON.stringify({ object: { sha: remoteSha } });
    }
    if (endpoint === "repos/yersonargotev/matty/git/ref/heads/main") {
      return JSON.stringify({ object: { sha: baseSha } });
    }
    throw new Error(`stale candidate endpoint must not be queried: ${endpoint}`);
  };

  const outcome = await createGithubIssueDelivery({}, runCommand, undefined, workspace)(
    { intent: "status", issue: "36", cwd: "/repo" },
  );

  assert.equal(outcome.status, "blocked");
  if (outcome.status === "blocked") {
    assert.deepEqual(outcome.exceptionBrief.evidence, ["delivery-candidate-drift"]);
  }
  assert.equal(calls.some((endpoint) => /check-runs|\/status\?/.test(endpoint)), false);
});

test("a later pull-request page containing a related incompatible PR is ambiguous", async () => {
  const candidateSha = commitSha("1111111111111111111111111111111111111111");
  const identity = {
    repository: "github.com/yersonargotev/matty",
    tracker: "github" as const,
    issue: 36,
  };
  const branch = "matty/deliver-36-owned";
  const workspace = {
    inspect: async () => ({
      status: "active" as const,
      delivery: {
        identity,
        branch,
        integrationBranch: "main",
        integrationSha: commitSha("0000000000000000000000000000000000000000"),
        candidateSha,
      },
    }),
    prepare: async () => { throw new Error("active delivery must not prepare"); },
  };
  const runCommand: IssueDeliveryCommandReader = async (_command, args) => {
    if (args[1] === "repos/yersonargotev/matty/issues/36") {
      return JSON.stringify({ number: 36, state: "open" });
    }
    if (args[1]?.startsWith("repos/yersonargotev/matty/pulls?")) {
      const pull = {
        head: { sha: candidateSha, ref: branch, repo: { full_name: "yersonargotev/matty" } },
        base: { ref: "main" },
        state: "open",
        merged_at: null,
      };
      return JSON.stringify([
        [pull],
        [{ ...pull, base: { ref: "release" } }],
      ]);
    }
    if (args[1] === `repos/yersonargotev/matty/git/ref/heads/${encodeURIComponent(branch)}`) {
      return JSON.stringify({ object: { sha: candidateSha } });
    }
    if (args[1] === "repos/yersonargotev/matty/git/ref/heads/main") {
      return JSON.stringify({ object: { sha: "0000000000000000000000000000000000000000" } });
    }
    if (args[1]?.includes("/check-runs?")) {
      return JSON.stringify([{ total_count: 0, check_runs: [] }]);
    }
    if (args[1]?.includes("/status?")) {
      return JSON.stringify([{ statuses: [] }]);
    }
    throw new Error(`unexpected command: ${args.join(" ")}`);
  };

  const outcome = await createGithubIssueDelivery({}, runCommand, undefined, workspace)(
    { intent: "status", issue: "36", cwd: "/repo" },
  );

  assert.deepEqual(outcome, {
    schemaVersion: 1,
    status: "blocked",
    exceptionBrief: {
      schemaVersion: 1,
      gate: "verification",
      evidence: ["delivery-pr-ambiguous"],
      need: "Owned delivery pull request facts are ambiguous.",
      options: ["Resolve incompatible or duplicate pull requests related to the owned delivery branch."],
      recommendation: "Preserve the candidate and reconcile the ambiguity before continuing.",
    },
  });
});

test("dependency inspection fails closed on malformed and authorization failures", async (context) => {
  for (const [name, dependencyFailure] of [
    ["malformed", () => "{not-json"],
    ["unauthorized", () => { throw Object.assign(new Error("denied"), { stderr: "HTTP 401" }); }],
  ] as const) {
    await context.test(name, async () => {
      const root = await mkdtemp(join(tmpdir(), "matty-dependency-failure-"));
      try {
        await prepareRepository(root);
        const runCommand: IssueDeliveryCommandReader = async (command, args) => {
          if (command === "git" && args[0] === "rev-parse") return root;
          if (command === "git" && args[0] === "remote") return "https://github.com/yersonargotev/matty.git";
          if (command === "gh" && args[0] === "--version") return "gh version";
          if (command === "gh" && args[0] === "auth") return "authenticated";
          if (args[1] === "repos/yersonargotev/matty/issues/34") {
            return JSON.stringify({
              number: 34, state: "open", labels: [{ name: "ready-to-build" }],
              html_url: "https://github.com/yersonargotev/matty/issues/34",
              title: "Issue", body: "",
            });
          }
          if (args[1]?.includes("/dependencies/blocked_by")) return dependencyFailure();
          throw new Error("unexpected command");
        };
        const outcome = await createGithubIssueDelivery(
          { HOME: join(root, "home") }, runCommand, undefined,
          { inspect: async () => ({ status: "absent" }), prepare: async () => { throw new Error("must not prepare"); } },
        )({ intent: "deliver", issue: "34", cwd: root });
        assert.equal(outcome.status, "blocked");
        if (outcome.status === "blocked") {
          assert.equal(outcome.exceptionBrief.evidence[0], "qualification-inspection-failed");
        }
      } finally {
        await rm(root, { recursive: true, force: true });
      }
    });
  }
});

test("confirmed missing native dependency endpoint uses exact Blocked by fallback", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-dependency-fallback-"));
  const calls: string[] = [];
  try {
    await prepareRepository(root);
    const runCommand: IssueDeliveryCommandReader = async (command, args) => {
      if (command === "git" && args[0] === "rev-parse") return root;
      if (command === "git" && args[0] === "remote") return "https://github.com/yersonargotev/matty.git";
      if (command === "gh" && args[0] === "--version") return "gh version";
      if (command === "gh" && args[0] === "auth") return "authenticated";
      const endpoint = args[1]!;
      calls.push(endpoint);
      if (endpoint === "repos/yersonargotev/matty/issues/34") {
        return JSON.stringify({
          number: 34, state: "open", labels: [{ name: "ready-to-build" }],
          html_url: "https://github.com/yersonargotev/matty/issues/34",
          title: "Issue", body: "Blocked by: #42\n\nSupporting prose #99",
        });
      }
      if (endpoint.includes("/dependencies/blocked_by")) {
        throw Object.assign(new Error("endpoint unavailable"), { stderr: "HTTP 404" });
      }
      if (endpoint === "repos/yersonargotev/matty/issues/42") {
        return JSON.stringify({
          number: 42, state: "open", title: "Publication dependency",
          html_url: "https://github.com/yersonargotev/matty/issues/42",
        });
      }
      throw new Error("unexpected command");
    };
    const outcome = await createGithubIssueDelivery(
      { HOME: join(root, "home") }, runCommand, undefined,
      { inspect: async () => ({ status: "absent" }), prepare: async () => { throw new Error("must not prepare"); } },
    )({ intent: "deliver", issue: "34", cwd: root });
    assert.equal(outcome.status, "blocked");
    if (outcome.status === "blocked") {
      assert.ok(outcome.exceptionBrief.evidence.includes("issue-blocked-by-dependency"));
    }
    assert.ok(calls.includes("repos/yersonargotev/matty/issues/42"));
    assert.equal(calls.includes("repos/yersonargotev/matty/issues/99"), false);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("native dependency inspection reads every page before qualification", async () => {
  const root = await mkdtemp(join(tmpdir(), "matty-dependency-pagination-"));
  try {
    await prepareRepository(root);
    const runCommand: IssueDeliveryCommandReader = async (command, args) => {
      if (command === "git" && args[0] === "rev-parse") return root;
      if (command === "git" && args[0] === "remote") return "https://github.com/yersonargotev/matty.git";
      if (command === "gh" && args[0] === "--version") return "gh version";
      if (command === "gh" && args[0] === "auth") return "authenticated";
      const endpoint = args[1]!;
      if (endpoint === "repos/yersonargotev/matty/issues/34") {
        return JSON.stringify({
          number: 34, state: "open", labels: [{ name: "ready-to-build" }],
          html_url: "https://github.com/yersonargotev/matty/issues/34",
          title: "Issue", body: "",
        });
      }
      if (endpoint.includes("/dependencies/blocked_by")) {
        assert.deepEqual(args.slice(2), ["--paginate", "--slurp"]);
        return JSON.stringify([
          [{ number: 41, state: "closed", title: "Closed dependency", html_url: "https://github.com/yersonargotev/matty/issues/41" }],
          [{ number: 42, state: "open", title: "Open dependency", html_url: "https://github.com/yersonargotev/matty/issues/42" }],
        ]);
      }
      throw new Error("unexpected command");
    };
    const outcome = await createGithubIssueDelivery(
      { HOME: join(root, "home") }, runCommand, undefined,
      { inspect: async () => ({ status: "absent" }), prepare: async () => { throw new Error("must not prepare"); } },
    )({ intent: "deliver", issue: "34", cwd: root });
    assert.equal(outcome.status, "blocked");
    if (outcome.status === "blocked") {
      assert.ok(outcome.exceptionBrief.evidence.includes("issue-blocked-by-dependency"));
    }
  } finally {
    await rm(root, { recursive: true, force: true });
  }
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
    const workspace = {
      inspect: async () => ({ status: "absent" as const }),
      prepare: async () => { throw new Error("blocked qualification must not prepare"); },
    };
    const outcome = await createGithubIssueDelivery(
      { HOME: home },
      runCommand,
      undefined,
      workspace,
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
