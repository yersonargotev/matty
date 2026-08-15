import assert from "node:assert/strict";
import test from "node:test";

import {
  deliverIssue as deliverIssueWithObserver,
  type IssueDeliveryPreflight,
} from "../src/application/issue-delivery.ts";
import {
  createIssueDeliveryWorkspace,
  deliveryIdentityKey,
  type DeliveryOwnershipRecord,
  type IssueDeliveryWorkspacePort,
  type WorkspaceCheckoutFacts,
} from "../src/application/issue-delivery-workspace.ts";
import { initialRepairBudget, ISSUE_DELIVERY_WORKFLOW } from "../src/domain/issue-delivery.ts";
import { commitSha } from "../src/domain/commit-sha.ts";

const BASE_SHA = commitSha("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
const LOCAL_SHA = commitSha("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");

async function deliverIssue(
  request: Parameters<typeof deliverIssueWithObserver>[0],
  qualify: () => Promise<IssueDeliveryPreflight>,
  workspace: Parameters<typeof deliverIssueWithObserver>[2],
) {
  return await deliverIssueWithObserver(request, {
    observeQualification: qualify,
    observeActive: async () => { throw new Error("active observation must not run"); },
  }, workspace);
}

function readyPreflight(issue = 35): IssueDeliveryPreflight {
  return {
    github: { available: true, authenticated: true },
    repository: {
      trusted: true,
      prepared: true,
      tracker: "github",
      canonical: "github.com/yersonargotev/matty",
      readyLabel: "ready-for-agent",
    },
    issueInspection: "available",
    issue: {
      kind: "issue",
      number: issue,
      state: "open",
      labels: ["ready-for-agent"],
      url: `https://github.com/yersonargotev/matty/issues/${issue}`,
    },
    skills: ISSUE_DELIVERY_WORKFLOW.dependencies.map((dependency) => ({
      id: dependency.id,
      identity: dependency.id,
      provenance: dependency.provenance,
      digest: dependency.digest,
    })),
  };
}

class MemoryWorkspacePort implements IssueDeliveryWorkspacePort {
  active?: DeliveryOwnershipRecord;
  readonly preparations: DeliveryOwnershipRecord[] = [];
  readonly preparedKeys = new Set<string>();
  ownership: "absent" | "owned" | "mismatch" = "absent";

  readonly facts: WorkspaceCheckoutFacts;

  constructor(facts: WorkspaceCheckoutFacts) {
    this.facts = facts;
  }

  async inspect(): Promise<WorkspaceCheckoutFacts> {
    return this.facts;
  }

  async inspectActive() {
    return { status: "absent" as const };
  }

  async readActive(): Promise<DeliveryOwnershipRecord | undefined> {
    return this.active;
  }

  async inspectOwnership(): Promise<"absent" | "owned" | "mismatch"> {
    return this.ownership;
  }

  async claim(record: DeliveryOwnershipRecord): Promise<"claimed" | "same" | "different"> {
    if (!this.active) {
      this.active = record;
      this.ownership = "owned";
      return "claimed";
    }
    return this.active.key === record.key ? "same" : "different";
  }

  async prepare(record: DeliveryOwnershipRecord): Promise<void> {
    if (!this.preparedKeys.has(record.key)) {
      this.preparedKeys.add(record.key);
      this.preparations.push(record);
    }
  }
}

const exactBase: WorkspaceCheckoutFacts = {
  root: "/repo",
  ref: "main",
  sha: BASE_SHA,
  clean: true,
  integrationBranch: "main",
  integrationSha: BASE_SHA,
};

async function runWithPort(port: IssueDeliveryWorkspacePort, issue = 35) {
  return await deliverIssue(
    { intent: "deliver", issue: String(issue), cwd: "/repo" },
    async () => readyPreflight(issue),
    createIssueDeliveryWorkspace(port),
  );
}

test("workspace preparation starts only after successful qualification", async () => {
  let workspaceReads = 0;
  const port = new MemoryWorkspacePort(exactBase);
  port.inspect = async () => {
    workspaceReads += 1;
    return exactBase;
  };
  const preflight = readyPreflight();
  preflight.issue!.labels = ["needs-info"];

  const outcome = await deliverIssue(
    { intent: "deliver", issue: "35", cwd: "/repo" },
    async () => preflight,
    createIssueDeliveryWorkspace(port),
  );

  assert.equal(outcome.status, "blocked");
  assert.equal(workspaceReads, 0);
  assert.deepEqual(port.preparations, []);
});

test("a clean checkout exactly at integration base prepares the delivery branch in place", async () => {
  const port = new MemoryWorkspacePort(exactBase);

  const outcome = await runWithPort(port);

  assert.equal(outcome.status, "prepared");
  assert.equal(port.preparations.length, 1);
  if (outcome.status === "prepared") {
    assert.equal(outcome.workspace.branch, "matty/deliver-35-4533aa2a");
    assert.equal(outcome.workspace.isolation, "in-place");
    assert.deepEqual(outcome.workspace.startingCheckout, {
      root: "/repo",
      ref: "main",
      sha: BASE_SHA,
    });
  }
});

test("the same marked Delivery Identity resumes idempotently", async () => {
  const port = new MemoryWorkspacePort(exactBase);

  const first = await runWithPort(port);
  const second = await runWithPort(port);

  assert.equal(first.status, "prepared");
  assert.equal(second.status, "prepared");
  assert.equal(port.preparations.length, 1);
  if (second.status === "prepared") {
    assert.equal(second.workspace.resumed, true);
    assert.deepEqual(second.workspace.startingCheckout, {
      root: "/repo",
      ref: "main",
      sha: BASE_SHA,
    });
  }
});

test("concurrent different Delivery Identities admit one claim with no loser workspace effects", async () => {
  const port = new MemoryWorkspacePort(exactBase);

  const [first, second] = await Promise.all([
    runWithPort(port, 35),
    runWithPort(port, 36),
  ]);

  assert.equal(port.preparations.length, 1);
  assert.deepEqual(
    [first.status, second.status].sort(),
    ["blocked", "prepared"],
  );
  const blocked = first.status === "blocked" ? first : second;
  if (blocked.status === "blocked") {
    assert.deepEqual(blocked.exceptionBrief.evidence, ["delivery-active"]);
  }
});

test("an active record whose key disagrees with its Delivery Identity is blocked without preparation", async () => {
  const port = new MemoryWorkspacePort(exactBase);
  const identity = {
    repository: "github.com/yersonargotev/matty",
    tracker: "github" as const,
    issue: 35,
  };
  port.active = {
    schemaVersion: 1,
    status: "active",
    key: deliveryIdentityKey({ ...identity, issue: 36 }),
    identity,
    branch: "matty/deliver-35-lookalike",
    path: "/repo",
    isolation: "in-place",
    startingCheckout: { root: "/repo", ref: "main", sha: BASE_SHA },
    integration: { branch: "main", sha: BASE_SHA },
    repairBudget: initialRepairBudget(),
  };

  const outcome = await runWithPort(port);

  assert.equal(outcome.status, "blocked");
  assert.deepEqual(port.preparations, []);
  if (outcome.status === "blocked") {
    assert.deepEqual(outcome.exceptionBrief.evidence, [
      "delivery-ownership-mismatch",
    ]);
  }
});

test("a matching branch or worktree name without its ownership record is blocked", async () => {
  const port = new MemoryWorkspacePort(exactBase);
  port.ownership = "mismatch";

  const outcome = await runWithPort(port);

  assert.equal(outcome.status, "blocked");
  assert.deepEqual(port.preparations, []);
  if (outcome.status === "blocked") {
    assert.deepEqual(outcome.exceptionBrief.evidence, [
      "delivery-ownership-mismatch",
    ]);
  }
});

test("dirty, detached, divergent, and unrelated checkouts preserve the checkout and use isolation", async (context) => {
  const cases: Array<[string, Partial<WorkspaceCheckoutFacts>]> = [
    ["dirty", { clean: false }],
    ["detached", { ref: null }],
    ["divergent", { sha: LOCAL_SHA }],
    ["unrelated", { ref: "feature/other" }],
  ];

  for (const [name, change] of cases) {
    await context.test(name, async () => {
      const facts = { ...exactBase, ...change };
      const port = new MemoryWorkspacePort(facts);
      const outcome = await runWithPort(port);

      assert.equal(outcome.status, "prepared");
      assert.equal(port.preparations.length, 1);
      const record = port.preparations[0]!;
      assert.equal(record.isolation, "worktree");
      assert.deepEqual(record.startingCheckout, {
        root: facts.root,
        ref: facts.ref,
        sha: facts.sha,
      });
      assert.deepEqual(facts, { ...exactBase, ...change });
    });
  }
});
