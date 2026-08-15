import assert from "node:assert/strict";
import test from "node:test";

import {
  deliverIssue,
  type IssueDeliveryPreflight,
  type IssueDeliveryWorkspace,
} from "../src/application/issue-delivery.ts";
import {
  ISSUE_DELIVERY_WORKFLOW,
} from "../src/domain/issue-delivery.ts";
import { commitSha } from "../src/domain/commit-sha.ts";

const qualificationWorkspace: IssueDeliveryWorkspace = {
  inspect: async () => ({ status: "absent" }),
  prepare: async () => ({
    status: "prepared",
    workspace: {
      root: "/repo",
      path: "/repo",
      branch: "matty/deliver-34-owned",
      isolation: "in-place",
      resumed: false,
      startingCheckout: {
        root: "/repo",
        ref: "main",
        sha: commitSha("0000000000000000000000000000000000000000"),
      },
    },
  }),
};

function qualificationObserver(read: () => Promise<IssueDeliveryPreflight>) {
  return {
    observeQualification: read,
    observeActive: async () => { throw new Error("active observation must not run"); },
  };
}

function readyPreflight(): IssueDeliveryPreflight {
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
      number: 34,
      state: "open",
      labels: ["ready-for-agent"],
      url: "https://github.com/yersonargotev/matty/issues/34",
    },
    skills: ISSUE_DELIVERY_WORKFLOW.dependencies.map((dependency) => ({
      id: dependency.id,
      identity: dependency.id,
      provenance: dependency.provenance,
      digest: dependency.digest,
    })),
  };
}

test("Issue Delivery prepares one exact ready GitHub issue through its controller", async () => {
  let reads = 0;
  const outcome = await deliverIssue(
    { intent: "deliver", issue: "#34", cwd: "/repo" },
    qualificationObserver(async () => {
      reads += 1;
      return readyPreflight();
    }),
    qualificationWorkspace,
  );

  assert.equal(reads, 1);
  assert.deepEqual(outcome, {
    schemaVersion: 1,
    status: "prepared",
    workflow: {
      id: "issue-delivery",
      definitionVersion: 1,
      guidanceVersion: 1,
    },
    deliveryIdentity: {
      repository: "github.com/yersonargotev/matty",
      tracker: "github",
      issue: 34,
    },
    scope: {
      schemaVersion: 1,
      reference: "https://github.com/yersonargotev/matty/issues/34",
      title: "Issue #34",
      body: "",
      requirements: [],
      dependencies: [],
    },
    evidence: [
      "delivery-intent-explicit",
      "github-authenticated",
      "prepared-repository-trusted",
      "issue-ready",
      "workflow-dependencies-certified",
    ],
    workspace: {
      root: "/repo",
      path: "/repo",
      branch: "matty/deliver-34-owned",
      isolation: "in-place",
      resumed: false,
      startingCheckout: {
        root: "/repo",
        ref: "main",
        sha: commitSha("0000000000000000000000000000000000000000"),
      },
    },
  });
});

test("Issue scope extracts only explicit Markdown task-list acceptance items", async () => {
  const preflight = readyPreflight();
  preflight.issue!.body = [
    "## Acceptance criteria",
    "This prose is evidence, not a requirement.",
    "- [ ] First explicit requirement",
    "- [x] Completed explicit requirement",
    "- [X] Uppercase completed requirement",
    "* [ ] wrong bullet marker",
  ].join("\n");
  const outcome = await deliverIssue(
    { intent: "deliver", issue: "34", cwd: "/repo" },
    qualificationObserver(async () => preflight),
    qualificationWorkspace,
  );
  assert.equal(outcome.status, "prepared");
  if (outcome.status === "prepared") {
    assert.equal(outcome.scope.body, preflight.issue!.body);
    assert.deepEqual(outcome.scope.requirements, [
      "First explicit requirement",
      "Completed explicit requirement",
      "Uppercase completed requirement",
    ]);
  }
});

test("Issue Delivery rejects ambiguous input before capability reads", async () => {
  let reads = 0;
  const outcome = await deliverIssue(
    { intent: "deliver", issue: "34 35", cwd: "/repo" },
    qualificationObserver(async () => {
      reads += 1;
      return readyPreflight();
    }),
    qualificationWorkspace,
  );

  assert.equal(reads, 0);
  assert.equal(outcome.status, "blocked");
  if (outcome.status === "blocked") {
    assert.deepEqual(outcome.exceptionBrief, {
      schemaVersion: 1,
      gate: "delivery-authorization",
      evidence: ["issue-reference-invalid"],
      need: "One exact GitHub issue number or canonical issue URL is required.",
      options: ["Run /matty deliver <issue-number> with exactly one issue."],
      recommendation:
        "Choose the intended issue, then run /matty deliver <issue-number>.",
    });
  }
});

test("failed qualification reports exact remediation and cannot produce delivery effects", async () => {
  const preflight = readyPreflight();
  preflight.github.authenticated = false;
  preflight.repository.prepared = false;
  preflight.issue!.labels = ["needs-info"];
  preflight.skills[0] = {
    ...preflight.skills[0]!,
    digest: "modified",
  };
  const effects: string[] = [];

  const outcome = await deliverIssue(
    { intent: "deliver", issue: "34", cwd: "/repo" },
    qualificationObserver(async () => preflight),
    qualificationWorkspace,
  );

  assert.deepEqual(effects, []);
  assert.equal(outcome.status, "blocked");
  if (outcome.status === "blocked") {
    assert.deepEqual(outcome.exceptionBrief.evidence, [
      "github-authentication-missing",
      "prepared-repository-missing",
      "issue-not-ready",
      "workflow-dependency-content-digest-mismatch:implement",
    ]);
    assert.match(outcome.exceptionBrief.need, /Issue Delivery only/);
    assert.deepEqual(outcome.exceptionBrief.options, [
      "Run gh auth login, then repeat /matty deliver 34.",
      "Run /skill:setup-matt-pocock-skills in this repository, review its changes, then repeat /matty deliver 34.",
      "Apply the repository's ready-for-agent triage label to issue #34, then repeat /matty deliver 34.",
      "Restore the Packy-provisioned certified implement skill, then repeat /matty deliver 34.",
    ]);
  }
});

test("a ready label cannot bypass an open native dependency such as #42", async () => {
  const preflight = readyPreflight();
  preflight.issue!.dependencies = [{
    reference: "https://github.com/yersonargotev/matty/issues/42",
    title: "Dependent publication behavior",
    state: "open",
  }];
  const outcome = await deliverIssue(
    { intent: "deliver", issue: "34", cwd: "/repo" },
    qualificationObserver(async () => preflight),
    qualificationWorkspace,
  );
  assert.equal(outcome.status, "blocked");
  if (outcome.status === "blocked") {
    assert.ok(outcome.exceptionBrief.evidence.includes("issue-blocked-by-dependency"));
  }
});

test("unexpected inspection failures become a closed Exception Brief", async () => {
  const outcome = await deliverIssue(
    { intent: "deliver", issue: "34", cwd: "/repo" },
    qualificationObserver(async () => {
      throw new Error("secret provider output");
    }),
    qualificationWorkspace,
  );

  assert.equal(outcome.status, "blocked");
  const rendered = JSON.stringify(outcome);
  assert.doesNotMatch(rendered, /secret provider output/);
  assert.match(rendered, /qualification-inspection-failed/);
});
