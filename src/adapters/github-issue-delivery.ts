import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, realpath } from "node:fs/promises";
import { homedir } from "node:os";
import { join, relative, resolve } from "node:path";
import { promisify } from "node:util";

import {
  deliverIssue,
  type IssueDeliveryInspection,
  type IssueDeliveryInspectionRequest,
  type IssueDeliveryPreflight,
  type IssueDeliveryRequest,
  type IssueDeliveryWorkspace,
  type ParsedIssueReference,
} from "../application/issue-delivery.ts";
import {
  candidateCheck,
  ISSUE_DELIVERY_WORKFLOW,
  type IssueDeliveryOutcome,
} from "../domain/issue-delivery.ts";
import {
  canonicalGithubRemote,
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
      issueInspection = githubNotFound(error) ? "not-found" : "failed";
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

function githubNotFound(error: unknown): boolean {
  const stderr = error && typeof error === "object" &&
      "stderr" in error && typeof error.stderr === "string"
    ? error.stderr
    : "";
  return /(?:HTTP\s+404|status\s+404|not found)/i.test(stderr);
}

function parsePaginatedPages(raw: string, kind: string): unknown[] {
  const value: unknown = JSON.parse(raw);
  if (!Array.isArray(value) || value.length === 0) {
    throw new Error(`invalid ${kind} pagination`);
  }
  return value;
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
      "--paginate",
      "--slurp",
    ],
    cwd,
  );
  const pullPages = parsePaginatedPages(pullsRaw, "pull request");
  const pullRequests: IssueDeliveryInspection["pullRequests"] = [];
  for (const page of pullPages) {
    if (!Array.isArray(page)) throw new Error("invalid pull request inspection");
    for (const value of page) {
      if (typeof value !== "object" || value === null || Array.isArray(value)) {
        throw new Error("invalid pull request inspection");
      }
      const pull = value as {
        head?: unknown;
        base?: unknown;
        state?: unknown;
        merged_at?: unknown;
      };
      if (
        typeof pull.head !== "object" || pull.head === null || Array.isArray(pull.head) ||
        typeof pull.base !== "object" || pull.base === null || Array.isArray(pull.base) ||
        (pull.state !== "open" && pull.state !== "closed") ||
        !(pull.merged_at === null || typeof pull.merged_at === "string")
      ) {
        throw new Error("invalid pull request inspection");
      }
      const head = pull.head as { sha?: unknown; ref?: unknown; repo?: unknown };
      const base = pull.base as { ref?: unknown };
      if (
        typeof head.sha !== "string" || head.sha.length === 0 ||
        typeof head.ref !== "string" || !Object.hasOwn(head, "repo") ||
        !(head.repo === null ||
          (typeof head.repo === "object" && !Array.isArray(head.repo) &&
            typeof (head.repo as { full_name?: unknown }).full_name === "string")) ||
        typeof base.ref !== "string"
      ) {
        throw new Error("invalid pull request inspection");
      }
      const headRepository = head.repo === null
        ? null
        : `github.com/${
          (head.repo as { full_name: string }).full_name.toLowerCase()
        }`;
      const compatible =
        headRepository === request.identity.repository &&
        head.ref === request.branch &&
        base.ref === request.integrationBranch &&
        pull.state === "open" &&
        pull.merged_at === null;
      pullRequests.push(compatible
        ? { compatibility: "compatible", headSha: head.sha }
        : { compatibility: "incompatible" });
    }
  }

  const readBranchSha = async (
    branch: string,
    missingIsNormal: boolean,
  ): Promise<string | null> => {
    try {
      const raw = await runCommand(
        "gh",
        ["api", `repos/${repositoryName}/git/ref/heads/${encodeURIComponent(branch)}`],
        cwd,
      );
      const value = JSON.parse(raw) as { object?: unknown };
      if (
        typeof value.object !== "object" || value.object === null ||
        Array.isArray(value.object) ||
        typeof (value.object as { sha?: unknown }).sha !== "string"
      ) {
        throw new Error("invalid branch inspection");
      }
      return (value.object as { sha: string }).sha;
    } catch (error) {
      if (missingIsNormal && githubNotFound(error)) return null;
      throw error;
    }
  };
  const deliverySha = await readBranchSha(request.branch, true);
  const integrationSha = await readBranchSha(request.integrationBranch, false);
  if (integrationSha === null) throw new Error("integration branch is unavailable");

  let checks: IssueDeliveryInspection["checks"] = [];
  if (
    request.candidateSha !== null &&
    deliverySha === request.candidateSha
  ) {
    const [checksRaw, statusesRaw] = await Promise.all([
      runCommand(
        "gh",
        ["api", `repos/${repositoryName}/commits/${request.candidateSha}/check-runs?per_page=100`, "--paginate", "--slurp"],
        cwd,
      ),
      runCommand(
        "gh",
        ["api", `repos/${repositoryName}/commits/${request.candidateSha}/status?per_page=100`, "--paginate", "--slurp"],
        cwd,
      ),
    ]);
    const checkRunPages = parsePaginatedPages(checksRaw, "check run");
    const rawCheckRuns: unknown[] = [];
    let expectedTotal: number | undefined;
    let hasTotalCount: boolean | undefined;
    for (const page of checkRunPages) {
      if (typeof page !== "object" || page === null || Array.isArray(page)) {
        throw new Error("invalid check inspection");
      }
      const { check_runs: pageRuns, total_count: totalCount } = page as {
        check_runs?: unknown;
        total_count?: unknown;
      };
      if (!Array.isArray(pageRuns)) throw new Error("invalid check inspection");
      const pageHasTotalCount = totalCount !== undefined;
      if (hasTotalCount !== undefined && hasTotalCount !== pageHasTotalCount) {
        throw new Error("inconsistent check inspection");
      }
      hasTotalCount = pageHasTotalCount;
      if (pageHasTotalCount) {
        if (!Number.isSafeInteger(totalCount) || (totalCount as number) < 0) {
          throw new Error("invalid check inspection");
        }
        if (expectedTotal !== undefined && expectedTotal !== totalCount) {
          throw new Error("inconsistent check inspection");
        }
        expectedTotal = totalCount as number;
      }
      rawCheckRuns.push(...pageRuns);
    }
    if (expectedTotal !== undefined && rawCheckRuns.length !== expectedTotal) {
      throw new Error("incomplete check inspection");
    }

    const statusPages = parsePaginatedPages(statusesRaw, "status");
    const rawStatuses: unknown[] = [];
    let expectedStatusTotal: number | undefined;
    let hasStatusTotalCount: boolean | undefined;
    for (const page of statusPages) {
      if (typeof page !== "object" || page === null || Array.isArray(page)) {
        throw new Error("invalid status inspection");
      }
      const { statuses: pageStatuses, total_count: totalCount } = page as {
        statuses?: unknown;
        total_count?: unknown;
      };
      if (!Array.isArray(pageStatuses)) throw new Error("invalid status inspection");
      const pageHasTotalCount = totalCount !== undefined;
      if (
        (hasStatusTotalCount !== undefined && hasStatusTotalCount !== pageHasTotalCount) ||
        (pageHasTotalCount && (!Number.isSafeInteger(totalCount) || (totalCount as number) < 0)) ||
        (expectedStatusTotal !== undefined && expectedStatusTotal !== totalCount)
      ) {
        throw new Error("inconsistent status inspection");
      }
      hasStatusTotalCount = pageHasTotalCount;
      if (pageHasTotalCount) expectedStatusTotal = totalCount as number;
      rawStatuses.push(...pageStatuses);
    }
    if (expectedStatusTotal !== undefined && rawStatuses.length !== expectedStatusTotal) {
      throw new Error("incomplete status inspection");
    }
    checks = rawCheckRuns.map((run) => {
      if (typeof run !== "object" || run === null || Array.isArray(run)) {
        throw new Error("invalid check inspection");
      }
      const { status, conclusion } = run as { status?: unknown; conclusion?: unknown };
      return candidateCheck(status, conclusion);
    });
    checks.push(...rawStatuses.map((status) => {
      if (typeof status !== "object" || status === null || Array.isArray(status)) {
        throw new Error("invalid status inspection");
      }
      const state = (status as { state?: unknown }).state;
      if (state === "pending") return candidateCheck("queued", null);
      if (state === "success") return candidateCheck("completed", "success");
      if (state === "failure" || state === "error") {
        return candidateCheck("completed", "failure");
      }
      throw new Error("invalid status inspection");
    }));
  }
  return {
    issue: { state: issue.state as "open" | "closed" },
    pullRequests,
    remoteBranches: { deliverySha, integrationSha },
    checks,
  };
}

export function createGithubIssueDelivery(
  environment: NodeJS.ProcessEnv,
  runCommand: IssueDeliveryCommandReader = runCommandForStdout,
  runGit?: GitDeliveryCommandRunner,
  workspace: IssueDeliveryWorkspace = createGitIssueDeliveryWorkspace(runGit),
): (request: IssueDeliveryRequest) => Promise<IssueDeliveryOutcome> {
  return async (request) =>
    deliverIssue(
      request,
      (issue, cwd) => readPreflight(issue, cwd, environment, runCommand),
      workspace,
      (inspection) => readDeliveryInspection(inspection, request.cwd, runCommand),
    );
}
