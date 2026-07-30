import assert from "node:assert/strict";
import test from "node:test";

import {
  DESIGNER_CAPABILITY_CONTRACT,
  EXPLORER_CAPABILITY_CONTRACT,
  RESEARCHER_TOOLS,
  REVIEWER_CAPABILITY_CONTRACT,
  WORKER_TOOLS,
  createResearcherCapabilityContract,
  createWorkerCapabilityContract,
  preflightCapability,
  validateCapabilityContract,
} from "../src/domain/capability-contract.ts";

test("validates the production inspection-role Capability Contracts", () => {
  for (const contract of [
    EXPLORER_CAPABILITY_CONTRACT,
    DESIGNER_CAPABILITY_CONTRACT,
    REVIEWER_CAPABILITY_CONTRACT,
  ]) {
    assert.deepEqual(validateCapabilityContract(contract), {
      ok: true,
      contract,
    });
    assert.deepEqual(contract.tools, ["read", "grep", "find", "ls", "bash"]);
    assert.equal(contract.writeAuthority, "none");
    assert.equal(contract.mutationPolicy, "inspection-guard");
    assert.equal(contract.requirement, "required");
    assert.deepEqual(contract.cardinality, { min: 1, max: 1 });
  }
  assert.equal(DESIGNER_CAPABILITY_CONTRACT.github, "absent");
  assert.equal(REVIEWER_CAPABILITY_CONTRACT.github, "required-readonly");
});

test("preflight diagnoses unavailable required explorer capabilities", () => {
  const result = preflightCapability(EXPLORER_CAPABILITY_CONTRACT, {
    availableTools: ["read", "grep", "find", "ls"],
    independentRuntime: false,
    inspectionGuard: true,
    github: { available: false, authenticated: false },
  });

  assert.deepEqual(result, {
    ok: false,
    diagnostic: {
      kind: "capability-preflight",
      contractId: "delegate-explorer",
      unmet: [
        "required tool is unavailable: bash",
        "independent Subagent Runtime is unavailable",
      ],
    },
  });
});

test("rejects an ambiguous or incompatible contract", () => {
  const result = validateCapabilityContract({
    ...EXPLORER_CAPABILITY_CONTRACT,
    tools: ["read", "read"],
    writeAuthority: "working-tree",
  });

  assert.equal(result.ok, false);
  if (!result.ok) {
    assert.deepEqual(result.errors, [
      "tools must be unique",
      "inspection tools must match the package-owned allowlist",
      "inspection role write authority must be none",
    ]);
  }
});

test("reviewer preflight requires available authenticated gh inspection", () => {
  const result = preflightCapability(REVIEWER_CAPABILITY_CONTRACT, {
    availableTools: REVIEWER_CAPABILITY_CONTRACT.tools,
    independentRuntime: true,
    inspectionGuard: true,
    github: { available: false, authenticated: false },
  });

  assert.deepEqual(result, {
    ok: false,
    diagnostic: {
      kind: "capability-preflight",
      contractId: "delegate-reviewer",
      unmet: [
        "GitHub CLI is unavailable",
        "GitHub CLI authentication is unavailable",
      ],
    },
  });
});

test("preflight rejects tools outside the contract allowlist", () => {
  const result = preflightCapability(DESIGNER_CAPABILITY_CONTRACT, {
    availableTools: [...DESIGNER_CAPABILITY_CONTRACT.tools, "write"],
    independentRuntime: true,
    inspectionGuard: true,
  });

  assert.deepEqual(result, {
    ok: false,
    diagnostic: {
      kind: "capability-preflight",
      contractId: "delegate-designer",
      unmet: ["unapproved tool is available: write"],
    },
  });
});

test("validates a Single Writer Capability Contract with bounded paths", () => {
  const contract = createWorkerCapabilityContract({
    workingTree: "/trusted/project",
    temporaryPaths: ["/validated/tmp/session"],
  });

  assert.deepEqual(validateCapabilityContract(contract), {
    ok: true,
    contract,
  });
  assert.deepEqual(contract.tools, WORKER_TOOLS);
  assert.equal(contract.writeAuthority, "trusted-working-tree");
  assert.equal(contract.mutationPolicy, "worker-guard");
  assert.equal(contract.requirement, "required");
  assert.deepEqual(contract.cardinality, { min: 1, max: 1 });
  assert.deepEqual(contract.concurrency, { maxActive: 1 });
});

test("validates a researcher Capability Contract with two bounded write zones", () => {
  const contract = createResearcherCapabilityContract({
    web: "required",
    workspaceRoot: "/validated/tmp/matty/research",
    projectRoot: "/trusted/project",
    workspace:
      "/validated/tmp/matty/research/123e4567-e89b-42d3-a456-426614174000",
    report: "/trusted/project/docs/research/result.md",
  });

  assert.deepEqual(validateCapabilityContract(contract), {
    ok: true,
    contract,
  });
  assert.deepEqual(contract.tools, RESEARCHER_TOOLS);
  assert.equal(contract.writeAuthority, "research-artifacts");
  assert.equal(contract.requirement, "required");
  assert.deepEqual(contract.writeLimits, {
    workspaceFiles: "multiple",
    researchReports: 1,
    overwrite: "forbidden",
  });
});

test("rejects escaped researcher paths and altered write limits", () => {
  const contract = createResearcherCapabilityContract({
    web: "optional",
    workspaceRoot: "/validated/tmp/matty/research",
    projectRoot: "/trusted/project",
    workspace:
      "/validated/tmp/matty/research/123e4567-e89b-42d3-a456-426614174000",
    report: "/trusted/project/docs/research/result.md",
  });
  const result = validateCapabilityContract({
    ...contract,
    workspace: "/validated/tmp/matty/research/../escape",
    report: "/trusted/project/docs/research/result.txt",
    writeLimits: {
      ...contract.writeLimits,
      researchReports: 2,
      overwrite: "allowed",
    },
  });

  assert.equal(result.ok, false);
  if (!result.ok) {
    assert.deepEqual(result.errors, [
      "researcher contract requires exactly one bounded report",
      "research workspace must be an absolute normalized path",
      "research report must be an absolute normalized Markdown path",
    ]);
  }
});

test("rejects researcher artifacts outside their declared authority", () => {
  const contract = createResearcherCapabilityContract({
    web: "required",
    workspaceRoot: "/validated/tmp/matty/research",
    projectRoot: "/trusted/project",
    workspace:
      "/different/tmp/matty/research/123e4567-e89b-42d3-a456-426614174000",
    report: "/different/project/report.md",
  });
  const result = validateCapabilityContract(contract);

  assert.equal(result.ok, false);
  if (!result.ok) {
    assert.deepEqual(result.errors, [
      "research artifact paths exceed their declared authority",
    ]);
  }
});

test("researcher preflight requires web and the bounded file tool", () => {
  const contract = createResearcherCapabilityContract({
    web: "required",
    workspaceRoot: "/validated/tmp/matty/research",
    projectRoot: "/trusted/project",
    workspace:
      "/validated/tmp/matty/research/123e4567-e89b-42d3-a456-426614174000",
    report: "/trusted/project/docs/research/result.md",
  });

  assert.deepEqual(
    preflightCapability(contract, {
      availableTools: RESEARCHER_TOOLS,
      independentRuntime: true,
      inspectionGuard: false,
      researchFileTool: false,
      web: "degraded",
    }),
    {
      ok: false,
      diagnostic: {
        kind: "capability-preflight",
        contractId: "delegate-researcher",
        unmet: [
          "Research File tool is unavailable",
          "required web capability is degraded",
        ],
      },
    },
  );
});

test("rejects parallel-writer and escaped-path contracts before execution", () => {
  const contract = createWorkerCapabilityContract({
    workingTree: "/trusted/project",
    temporaryPaths: ["/validated/tmp/session"],
  });
  const result = validateCapabilityContract({
    ...contract,
    workingTree: "/trusted/project/../escape",
    temporaryPaths: ["/validated/tmp/session", "relative/tmp"],
    concurrency: { maxActive: 2 },
  });

  assert.equal(result.ok, false);
  if (!result.ok) {
    assert.deepEqual(result.errors, [
      "worker contract requires one writer",
      "working tree must be an absolute normalized path",
      "temporary paths must be unique absolute normalized paths",
    ]);
  }
});

test("worker preflight rejects an unavailable guard and unapproved tools", () => {
  const contract = createWorkerCapabilityContract({
    workingTree: "/trusted/project",
    temporaryPaths: ["/validated/tmp/session"],
  });

  assert.deepEqual(
    preflightCapability(contract, {
      availableTools: [...WORKER_TOOLS, "web_search"],
      independentRuntime: true,
      inspectionGuard: false,
      workerGuard: false,
    }),
    {
      ok: false,
      diagnostic: {
        kind: "capability-preflight",
        contractId: "delegate-worker",
        unmet: [
          "unapproved tool is available: web_search",
          "Worker Guard is unavailable",
        ],
      },
    },
  );
});
