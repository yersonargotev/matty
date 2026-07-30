import assert from "node:assert/strict";
import test from "node:test";

import {
  DESIGNER_CAPABILITY_CONTRACT,
  EXPLORER_CAPABILITY_CONTRACT,
  REVIEWER_CAPABILITY_CONTRACT,
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
