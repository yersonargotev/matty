import { spawn } from "node:child_process";

import {
  createIssueDeliveryWorkspace,
  deliveryIdentityKey,
  type DeliveryOwnershipRecord,
  type IssueDeliveryWorkspacePort,
  type WorkspaceCheckoutFacts,
} from "../application/issue-delivery-workspace.ts";
import type { IssueDeliveryWorkspace } from "../application/issue-delivery.ts";

const ACTIVE_REF = "refs/matty/issue-delivery/active";
const OWNER_REF_PREFIX = "refs/matty/issue-delivery/owners";

export type GitDeliveryCommandRunner = (
  args: string[],
  cwd: string,
  input?: string,
) => Promise<string>;

export class GitDeliveryCommandError extends Error {
  readonly exitCode: number | null;
  readonly args: readonly string[];

  constructor(exitCode: number | null, args: readonly string[]) {
    super(
      exitCode === null
        ? "git command terminated without an exit status"
        : `git command failed with exit status ${exitCode}`,
    );
    this.name = "GitDeliveryCommandError";
    this.exitCode = exitCode;
    this.args = args;
  }
}

async function runGit(
  args: string[],
  cwd: string,
  input?: string,
): Promise<string> {
  return await new Promise((resolve, reject) => {
    const child = spawn("git", args, {
      cwd,
      stdio: ["pipe", "pipe", "pipe"],
    });
    let stdout = "";
    child.stdout.setEncoding("utf8");
    child.stdout.on("data", (chunk) => stdout += chunk);
    child.stderr.resume();
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) {
        resolve(stdout.trim());
      } else {
        reject(new GitDeliveryCommandError(code, args));
      }
    });
    child.stdin.end(input);
  });
}

function ownerRef(key: string): string {
  return `${OWNER_REF_PREFIX}/${key}`;
}

function isRecord(value: unknown): value is DeliveryOwnershipRecord {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }
  const record = value as Partial<DeliveryOwnershipRecord>;
  return record.schemaVersion === 1 && record.status === "active" &&
    typeof record.key === "string" && typeof record.branch === "string" &&
    typeof record.path === "string" &&
    (record.isolation === "in-place" || record.isolation === "worktree") &&
    typeof record.identity === "object" && record.identity !== null &&
    typeof record.identity.repository === "string" &&
    record.identity.tracker === "github" &&
    typeof record.identity.issue === "number" &&
    typeof record.startingCheckout === "object" &&
    record.startingCheckout !== null &&
    typeof record.startingCheckout.root === "string" &&
    (typeof record.startingCheckout.ref === "string" ||
      record.startingCheckout.ref === null) &&
    typeof record.startingCheckout.sha === "string" &&
    typeof record.integration === "object" && record.integration !== null &&
    typeof record.integration.branch === "string" &&
    typeof record.integration.sha === "string" &&
    record.key === deliveryIdentityKey(record.identity as DeliveryOwnershipRecord["identity"]);
}

function sameOwnership(
  actual: DeliveryOwnershipRecord,
  expected: DeliveryOwnershipRecord,
): boolean {
  return actual.key === expected.key &&
    actual.identity.repository === expected.identity.repository &&
    actual.identity.tracker === expected.identity.tracker &&
    actual.identity.issue === expected.identity.issue &&
    actual.branch === expected.branch && actual.path === expected.path &&
    actual.isolation === expected.isolation &&
    actual.startingCheckout.root === expected.startingCheckout.root &&
    actual.startingCheckout.ref === expected.startingCheckout.ref &&
    actual.startingCheckout.sha === expected.startingCheckout.sha &&
    actual.integration.branch === expected.integration.branch &&
    actual.integration.sha === expected.integration.sha;
}

function isExpectedAbsence(error: unknown): boolean {
  return typeof error === "object" && error !== null &&
    "exitCode" in error && error.exitCode === 1;
}

async function absentOnExitOne(
  operation: () => Promise<string>,
): Promise<string | undefined> {
  try {
    return await operation();
  } catch (error) {
    if (isExpectedAbsence(error)) return undefined;
    throw error;
  }
}

function worktrees(raw: string): Array<{ path: string; branch?: string }> {
  return raw.split(/\n\n+/).flatMap((entry) => {
    const path = /^worktree (.+)$/m.exec(entry)?.[1];
    if (!path) return [];
    const branch = /^branch refs\/heads\/(.+)$/m.exec(entry)?.[1];
    return [{ path, ...(branch ? { branch } : {}) }];
  });
}

const preparationLocks = new Map<string, Promise<void>>();

async function serialized(key: string, operation: () => Promise<void>) {
  const prior = preparationLocks.get(key) ?? Promise.resolve();
  const current = prior.catch(() => undefined).then(operation);
  preparationLocks.set(key, current);
  try {
    await current;
  } finally {
    if (preparationLocks.get(key) === current) {
      preparationLocks.delete(key);
    }
  }
}

export function createGitIssueDeliveryWorkspace(
  run: GitDeliveryCommandRunner = runGit,
): IssueDeliveryWorkspace {
  const readRecord = async (
    ref: string,
    root: string,
  ): Promise<DeliveryOwnershipRecord | undefined> => {
    const object = await absentOnExitOne(() =>
      run(["rev-parse", "--verify", "--quiet", ref], root)
    );
    if (!object) return undefined;
    const raw = await run(["cat-file", "blob", object], root);
    const value: unknown = JSON.parse(raw);
    if (!isRecord(value)) throw new Error("invalid delivery ownership record");
    return value;
  };

  const port: IssueDeliveryWorkspacePort = {
    async inspectActive(cwd) {
      const root = await run(["rev-parse", "--show-toplevel"], cwd);
      const active = await readRecord(ACTIVE_REF, root);
      if (!active) return { status: "absent" };
      const owner = await readRecord(ownerRef(active.key), root);
      if (!owner || !sameOwnership(owner, active)) {
        return {
          status: "blocked",
          exceptionBrief: {
            schemaVersion: 1,
            gate: "implementation",
            evidence: ["delivery-ownership-mismatch"],
            need: "Active and owner delivery markers disagree.",
            options: ["Restore one matching pair of durable delivery ownership markers."],
            recommendation: "Do not infer ownership from branch or worktree names.",
          },
        };
      }
      const candidateSha = await run(
        ["rev-parse", "--verify", `refs/heads/${active.branch}`],
        root,
      );
      if (!candidateSha) throw new Error("owned delivery branch is unavailable");
      return {
        status: "active",
        delivery: {
          identity: active.identity,
          branch: active.branch,
          integrationSha: active.integration.sha,
          candidateSha: candidateSha === active.integration.sha ? null : candidateSha,
        },
      };
    },

    async inspect(cwd): Promise<WorkspaceCheckoutFacts> {
      const root = await run(["rev-parse", "--show-toplevel"], cwd);
      const [ref, sha, status, remoteHead] = await Promise.all([
        absentOnExitOne(() =>
          run(["symbolic-ref", "--quiet", "--short", "HEAD"], root)
        ),
        run(["rev-parse", "HEAD"], root),
        run(["status", "--porcelain=v1", "--untracked-files=normal"], root),
        run(["symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"], root),
      ]);
      if (!remoteHead.startsWith("origin/")) {
        throw new Error("canonical integration branch is unavailable");
      }
      const integrationBranch = remoteHead.slice("origin/".length);
      const integrationSha = await run(
        ["rev-parse", `refs/remotes/origin/${integrationBranch}`],
        root,
      );
      return {
        root,
        ref: ref ?? null,
        sha,
        clean: status === "",
        integrationBranch,
        integrationSha,
      };
    },

    async readActive(root) {
      return await readRecord(ACTIVE_REF, root);
    },

    async inspectOwnership(record) {
      const actual = await readRecord(ownerRef(record.key), record.startingCheckout.root);
      if (actual) return sameOwnership(actual, record) ? "owned" : "mismatch";

      const root = record.startingCheckout.root;
      const branch = await absentOnExitOne(async () => {
        await run(
          ["show-ref", "--verify", "--quiet", `refs/heads/${record.branch}`],
          root,
        );
        return "present";
      });
      const listed = worktrees(await run(["worktree", "list", "--porcelain"], root));
      return branch ||
          (record.isolation === "worktree" &&
            listed.some((entry) => entry.path === record.path))
        ? "mismatch"
        : "absent";
    },

    async claim(record) {
      const root = record.startingCheckout.root;
      const existing = await readRecord(ACTIVE_REF, root);
      if (existing) return sameOwnership(existing, record) ? "same" : "different";

      const ownership = await readRecord(ownerRef(record.key), root);
      if (ownership && !sameOwnership(ownership, record)) {
        throw new Error("delivery ownership record mismatch");
      }
      const serializedRecord = JSON.stringify(record);
      const object = await run(["hash-object", "-w", "--stdin"], root, serializedRecord);

      const transaction = [
        "start",
        ...(ownership ? [] : [`create ${ownerRef(record.key)} ${object}`]),
        `create ${ACTIVE_REF} ${object}`,
        "prepare",
        "commit",
        "",
      ].join("\n");
      try {
        await run(["update-ref", "--stdin"], root, transaction);
        return "claimed";
      } catch {
        const winner = await readRecord(ACTIVE_REF, root);
        return winner && sameOwnership(winner, record) ? "same" : "different";
      }
    },

    async prepare(record) {
      await serialized(`${record.startingCheckout.root}\n${record.key}`, async () => {
        const ownership = await readRecord(
          ownerRef(record.key),
          record.startingCheckout.root,
        );
        if (!ownership || !sameOwnership(ownership, record)) {
          throw new Error("delivery resource is not owned");
        }
        const root = record.startingCheckout.root;
        const branchExists = await absentOnExitOne(async () => {
          await run(
            ["show-ref", "--verify", "--quiet", `refs/heads/${record.branch}`],
            root,
          );
          return "present";
        });
        if (record.isolation === "in-place") {
          const current = await absentOnExitOne(() =>
            run(["symbolic-ref", "--quiet", "--short", "HEAD"], root)
          );
          if (branchExists) {
            if (current !== record.branch) {
              throw new Error("owned in-place delivery branch is not checked out");
            }
            return;
          }
          const [sha, status] = await Promise.all([
            run(["rev-parse", "HEAD"], root),
            run(["status", "--porcelain=v1", "--untracked-files=normal"], root),
          ]);
          if (
            current !== record.integration.branch ||
            sha !== record.integration.sha || status !== ""
          ) {
            throw new Error("operator checkout changed after qualification");
          }
          await run(["switch", "-c", record.branch, record.integration.sha], root);
          return;
        }

        const listed = worktrees(await run(["worktree", "list", "--porcelain"], root));
        const existing = listed.find((entry) => entry.path === record.path);
        if (existing) {
          if (existing.branch !== record.branch) {
            throw new Error("owned worktree branch mismatch");
          }
          return;
        }
        await run(
          branchExists
            ? ["worktree", "add", record.path, record.branch]
            : ["worktree", "add", "-b", record.branch, record.path, record.integration.sha],
          root,
        );
      });
    },
  };

  return createIssueDeliveryWorkspace(port);
}
