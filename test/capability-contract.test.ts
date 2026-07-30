import assert from "node:assert/strict";
import test from "node:test";

import {
  EXPLORER_CAPABILITY_CONTRACT,
  preflightCapability,
  validateCapabilityContract,
} from "../src/domain/capability-contract.ts";

test("validates the production explorer Capability Contract", () => {
  const validation = validateCapabilityContract(
    EXPLORER_CAPABILITY_CONTRACT,
  );

  assert.deepEqual(validation, {
    ok: true,
    contract: EXPLORER_CAPABILITY_CONTRACT,
  });
  assert.deepEqual(EXPLORER_CAPABILITY_CONTRACT, {
    schemaVersion: 1,
    id: "delegate-explorer",
    role: "explorer",
    tools: ["read", "grep", "find", "ls", "bash"],
    writeAuthority: "none",
    web: "absent",
    cardinality: { min: 1, max: 1 },
    concurrency: { maxActive: 1 },
    independence: "required",
    failureBehavior: "fail-invocation",
  });
});

test("preflight diagnoses unavailable required explorer capabilities", () => {
  const result = preflightCapability(EXPLORER_CAPABILITY_CONTRACT, {
    availableTools: ["read", "grep", "find", "ls"],
    independentRuntime: false,
    inspectionGuard: true,
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
      "explorer tools must match the package-owned allowlist",
      "explorer write authority must be none",
    ]);
  }
});
