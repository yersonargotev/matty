import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, realpath } from "node:fs/promises";
import { homedir } from "node:os";
import { join, relative, resolve } from "node:path";
import { promisify } from "node:util";

import {
  qualifyIssueDelivery,
  type IssueDeliveryPreflight,
  type IssueDeliveryRequest,
  type ParsedIssueReference,
} from "../application/issue-delivery.ts";
import {
  ISSUE_DELIVERY_WORKFLOW,
  type IssueDeliveryOutcome,
} from "../domain/issue-delivery.ts";

const execFileAsync = promisify(execFile);

async function runRead(
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

async function inspectRepository(cwd: string): Promise<{
  root: string;
  canonical?: string;
  prepared: boolean;
  trusted: boolean;
}> {
  const root = resolve(await runRead("git", ["rev-parse", "--show-toplevel"], cwd));
  const fromRoot = relative(root, resolve(cwd));
  const inside = fromRoot === "" || (!fromRoot.startsWith("..") && fromRoot !== "..");
  const remote = await runRead("git", ["remote", "get-url", "origin"], root);
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
  const trusted = inside && canonical !== undefined &&
    /issue-tracker\.md/.test(contents.get("AGENTS.md") ?? "") &&
    /triage-labels\.md/.test(contents.get("AGENTS.md") ?? "") &&
    /domain\.md/.test(contents.get("AGENTS.md") ?? "") &&
    /GitHub/.test(contents.get("docs/agents/issue-tracker.md") ?? "") &&
    /ready-for-agent/.test(contents.get("docs/agents/triage-labels.md") ?? "");
  return {
    root,
    ...(canonical ? { canonical } : {}),
    prepared,
    trusted,
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
        contentDigest: digest,
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
): Promise<IssueDeliveryPreflight> {
  const repository = await inspectRepository(cwd);
  let available = true;
  let authenticated = true;
  try {
    await runRead("gh", ["--version"], repository.root);
  } catch {
    available = false;
    authenticated = false;
  }
  if (available) {
    try {
      await runRead("gh", ["auth", "status"], repository.root);
    } catch {
      authenticated = false;
    }
  }

  let issue: IssueDeliveryPreflight["issue"];
  if (authenticated && repository.canonical) {
    const repositoryName = repository.canonical.slice("github.com/".length);
    try {
      const raw = await runRead(
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
    } catch {
      // Missing or inaccessible issues remain absent from the closed facts.
    }
  }

  return {
    github: { available, authenticated },
    repository: {
      trusted: repository.trusted,
      prepared: repository.prepared,
      tracker: repository.canonical ? "github" : "unsupported",
      ...(repository.canonical ? { canonical: repository.canonical } : {}),
    },
    ...(issue ? { issue } : {}),
    skills: await inspectSkills(environment),
  };
}

export function createGithubIssueDeliveryQualifier(
  environment: NodeJS.ProcessEnv,
): (request: IssueDeliveryRequest) => Promise<IssueDeliveryOutcome> {
  return async (request) =>
    qualifyIssueDelivery(
      request,
      (issue, cwd) => readPreflight(issue, cwd, environment),
    );
}
