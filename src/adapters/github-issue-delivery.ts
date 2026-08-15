import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, realpath } from "node:fs/promises";
import { homedir } from "node:os";
import { join, relative, resolve } from "node:path";
import { promisify } from "node:util";

import {
  deliverIssue,
  qualifyIssueDelivery,
  type IssueDeliveryInspection,
  type IssueDeliveryInspectionRequest,
  type IssueDeliveryPreflight,
  type IssueDeliveryRequest,
  type ParsedIssueReference,
} from "../application/issue-delivery.ts";
import {
  ISSUE_DELIVERY_WORKFLOW,
  type IssueDeliveryOutcome,
} from "../domain/issue-delivery.ts";
import {
  createGitIssueDeliveryWorkspace,
  type GitDeliveryCommandRunner,
} from "./git-issue-delivery-workspace.ts";

const execFileAsync = promisify(execFile);

export type IssueDeliveryCommandReader = (
  command: string,
  args: string[],
  cwd: string,
) => Promise<string>;

async function runCommandForStdout(
  command: string,
  args: string[],
  cwd: string,
): Promise<string> {
  const result = await execFileAsync(command, args, {
    cwd,
    encoding: "utf8",
    timeout: 15_000,
    maxBuffer: 1024 * 1024,
  });
  return result.stdout.trim();
}

function canonicalGithubRemote(remote: string): string | undefined {
  const match = /^(?:git@github\.com:|https:\/\/github\.com\/)([^/]+\/[^/]+?)(?:\.git)?$/.exec(
    remote.trim(),
  );
  return match ? `github.com/${match[1]!.toLowerCase()}` : undefined;
}

async function inspectRepository(
  cwd: string,
  runCommand: IssueDeliveryCommandReader,
): Promise<{
  root: string;
  canonical?: string;
  prepared: boolean;
  trusted: boolean;
  readyLabel?: string;
}> {
  const root = resolve(await runCommand("git", ["rev-parse", "--show-toplevel"], cwd));
  const fromRoot = relative(root, resolve(cwd));
  const inside = fromRoot === "" || (!fromRoot.startsWith("..") && fromRoot !== "..");
  const remote = await runCommand("git", ["remote", "get-url", "origin"], root);
  const canonical = canonicalGithubRemote(remote);
  const required = [
    "AGENTS.md",
    "docs/agents/issue-tracker.md",
    "docs/agents/triage-labels.md",
    "docs/agents/domain.md",
  ];
  let prepared = true;
  const contents = new Map<string, string>();
  for (const path of required) {
    try {
      contents.set(path, await readFile(join(root, path), "utf8"));
    } catch {
      prepared = false;
    }
  }
  try {
    await readFile(join(root, "CONTEXT.md"), "utf8");
  } catch {
    try {
      await readFile(join(root, "CONTEXT-MAP.md"), "utf8");
    } catch {
      prepared = false;
    }
  }
  const readyLabel = /^\|\s*`ready-for-agent`\s*\|\s*`([^`]+)`\s*\|/m.exec(
    contents.get("docs/agents/triage-labels.md") ?? "",
  )?.[1];
  const trusted = inside && canonical !== undefined &&
    /issue-tracker\.md/.test(contents.get("AGENTS.md") ?? "") &&
    /triage-labels\.md/.test(contents.get("AGENTS.md") ?? "") &&
    /domain\.md/.test(contents.get("AGENTS.md") ?? "") &&
    /issues and PRDs.*GitHub/i.test(
      contents.get("docs/agents/issue-tracker.md") ?? "",
    ) && readyLabel !== undefined;
  return {
    root,
    ...(canonical ? { canonical } : {}),
    prepared,
    trusted,
    ...(readyLabel ? { readyLabel } : {}),
  };
}

async function inspectSkills(environment: NodeJS.ProcessEnv) {
  const home = environment.HOME ?? homedir();
  const skills: IssueDeliveryPreflight["skills"] = [];
  for (const dependency of ISSUE_DELIVERY_WORKFLOW.dependencies) {
    const path = join(home, ".agents", "skills", dependency.id, "SKILL.md");
    try {
      const [content, actualPath] = await Promise.all([
        readFile(path, "utf8"),
        realpath(path),
      ]);
      const digest = createHash("sha256").update(content).digest("hex");
      const identity = /^name:\s*([^\s]+)\s*$/m.exec(content)?.[1] ?? "";
      const expectedSuffix = `/packy/bundle/skills/engineering/${dependency.id}/SKILL.md`;
      skills.push({
        id: dependency.id,
        identity,
        provenance: actualPath.endsWith(expectedSuffix)
          ? dependency.provenance
          : "unsupported",
        digest,
      });
    } catch {
      // Absence is represented by omitting the dependency from the closed facts.
    }
  }
  return skills;
}

async function readPreflight(
  issueReference: ParsedIssueReference,
  cwd: string,
  environment: NodeJS.ProcessEnv,
  runCommand: IssueDeliveryCommandReader,
): Promise<IssueDeliveryPreflight> {
  const repository = await inspectRepository(cwd, runCommand);
  let available = true;
  let authenticated = true;
  try {
    await runCommand("gh", ["--version"], repository.root);
  } catch {
    available = false;
    authenticated = false;
  }
  if (available) {
    try {
      await runCommand(
        "gh",
        ["auth", "status", "--hostname", "github.com"],
        repository.root,
      );
    } catch {
      authenticated = false;
    }
  }

  let issue: IssueDeliveryPreflight["issue"];
  let issueInspection: IssueDeliveryPreflight["issueInspection"];
  if (authenticated && repository.canonical) {
    const repositoryName = repository.canonical.slice("github.com/".length);
    try {
      const raw = await runCommand(
        "gh",
        ["api", `repos/${repositoryName}/issues/${issueReference.number}`],
        repository.root,
      );
      const value = JSON.parse(raw) as {
        number?: unknown;
        state?: unknown;
        labels?: Array<{ name?: unknown }>;
        html_url?: unknown;
        pull_request?: unknown;
      };
      if (
        typeof value.number === "number" &&
        (value.state === "open" || value.state === "closed") &&
        typeof value.html_url === "string"
      ) {
        issueInspection = "available";
        issue = {
          kind: value.pull_request ? "pull-request" : "issue",
          number: value.number,
          state: value.state,
          labels: (value.labels ?? []).flatMap((label) =>
            typeof label.name === "string" ? [label.name] : []
          ),
          url: value.html_url,
        };
      }
    } catch (error) {
      const stderr = error && typeof error === "object" &&
          "stderr" in error && typeof error.stderr === "string"
        ? error.stderr
        : "";
      issueInspection = /(?:HTTP\s+404|status\s+404|not found)/i.test(stderr)
        ? "not-found"
        : "failed";
    }
  }

  return {
    github: { available, authenticated },
    repository: {
      trusted: repository.trusted,
      prepared: repository.prepared,
      tracker: repository.canonical ? "github" : "unsupported",
      ...(repository.canonical ? { canonical: repository.canonical } : {}),
      ...(repository.readyLabel ? { readyLabel: repository.readyLabel } : {}),
    },
    ...(issueInspection ? { issueInspection } : {}),
    ...(issue ? { issue } : {}),
    skills: await inspectSkills(environment),
  };
}

async function readDeliveryInspection(
  request: IssueDeliveryInspectionRequest,
  cwd: string,
  runCommand: IssueDeliveryCommandReader,
): Promise<IssueDeliveryInspection> {
  const repositoryName = request.identity.repository.slice("github.com/".length);
  const owner = repositoryName.split("/")[0];
  if (!owner || !repositoryName.includes("/")) {
    throw new Error("invalid canonical repository identity");
  }
  const issueRaw = await runCommand(
    "gh",
    ["api", `repos/${repositoryName}/issues/${request.identity.issue}`],
    cwd,
  );
  const issue = JSON.parse(issueRaw) as Record<string, unknown>;
  if (
    issue.number !== request.identity.issue ||
    (issue.state !== "open" && issue.state !== "closed") ||
    issue.pull_request !== undefined
  ) {
    throw new Error("invalid issue inspection");
  }

  const pullsRaw = await runCommand(
    "gh",
    [
      "api",
      `repos/${repositoryName}/pulls?state=all&head=${encodeURIComponent(`${owner}:${request.branch}`)}&per_page=100`,
    ],
    cwd,
  );
  const pullsValue: unknown = JSON.parse(pullsRaw);
  if (!Array.isArray(pullsValue)) throw new Error("invalid pull request inspection");
  const pullRequests = pullsValue.map((value) => {
    if (typeof value !== "object" || value === null || Array.isArray(value)) {
      throw new Error("invalid pull request inspection");
    }
    const head = (value as { head?: unknown }).head;
    if (typeof head !== "object" || head === null || Array.isArray(head)) {
      throw new Error("invalid pull request inspection");
    }
    const { sha, ref } = head as { sha?: unknown; ref?: unknown };
    if (typeof sha !== "string" || sha.length === 0 || ref !== request.branch) {
      throw new Error("invalid pull request inspection");
    }
    return { headSha: sha };
  });

  let checks: IssueDeliveryInspection["checks"] = [];
  if (request.candidateSha !== null) {
    const checksRaw = await runCommand(
      "gh",
      ["api", `repos/${repositoryName}/commits/${request.candidateSha}/check-runs`],
      cwd,
    );
    const value = JSON.parse(checksRaw) as { check_runs?: unknown };
    if (!Array.isArray(value.check_runs)) throw new Error("invalid check inspection");
    const statuses = new Set(["queued", "in_progress", "completed"]);
    const conclusions = new Set([
      "success", "neutral", "skipped", "failure", "cancelled", "timed_out",
      "action_required", "stale",
    ]);
    checks = value.check_runs.map((run) => {
      if (typeof run !== "object" || run === null || Array.isArray(run)) {
        throw new Error("invalid check inspection");
      }
      const { status, conclusion } = run as { status?: unknown; conclusion?: unknown };
      if (
        typeof status !== "string" || !statuses.has(status) ||
        !(conclusion === null ||
          (typeof conclusion === "string" && conclusions.has(conclusion))) ||
        (status === "completed" && conclusion === null) ||
        (status !== "completed" && conclusion !== null)
      ) {
        throw new Error("invalid check inspection");
      }
      return {
        status: status as IssueDeliveryInspection["checks"][number]["status"],
        conclusion: conclusion as IssueDeliveryInspection["checks"][number]["conclusion"],
      };
    });
  }
  return {
    issue: { state: issue.state as "open" | "closed" },
    pullRequests,
    checks,
  };
}

export function createGithubIssueDelivery(
  environment: NodeJS.ProcessEnv,
  runCommand: IssueDeliveryCommandReader = runCommandForStdout,
  runGit?: GitDeliveryCommandRunner,
): (request: IssueDeliveryRequest) => Promise<IssueDeliveryOutcome> {
  const workspace = createGitIssueDeliveryWorkspace(runGit);
  return async (request) =>
    deliverIssue(
      request,
      (issue, cwd) => readPreflight(issue, cwd, environment, runCommand),
      workspace,
      (inspection) => readDeliveryInspection(inspection, request.cwd, runCommand),
    );
}

export function createGithubIssueDeliveryQualifier(
  environment: NodeJS.ProcessEnv,
  runCommand: IssueDeliveryCommandReader = runCommandForStdout,
): (request: IssueDeliveryRequest) => Promise<IssueDeliveryOutcome> {
  return async (request) =>
    qualifyIssueDelivery(
      request,
      (issue, cwd) => readPreflight(issue, cwd, environment, runCommand),
    );
}
