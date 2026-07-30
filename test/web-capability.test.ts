import assert from "node:assert/strict";
import test from "node:test";

import {
  WEB_CAPABILITY_TOOLS,
  createParentWebCapabilityContract,
  deriveWebCapabilityState,
  preflightWebCapability,
  runWebCapabilityOperation,
  validateWebCapabilityContract,
} from "../src/domain/web-capability.ts";

test("certifies exactly the canonical web tools", () => {
  assert.deepEqual(WEB_CAPABILITY_TOOLS, [
    "web_search",
    "source_check",
    "fetch_content",
    "get_search_content",
  ]);
});

test("derives available, degraded, and unavailable local web states", () => {
  assert.deepEqual(
    deriveWebCapabilityState({
      registeredTools: WEB_CAPABILITY_TOOLS,
      initializationSucceeded: true,
    }),
    "available",
  );

  for (const registeredTools of [
    ["web_search"],
    [...WEB_CAPABILITY_TOOLS, "web_search"],
  ]) {
    assert.equal(
      deriveWebCapabilityState({
        registeredTools,
        initializationSucceeded: true,
      }),
      "degraded",
    );
  }
  assert.equal(
    deriveWebCapabilityState({
      registeredTools: [...WEB_CAPABILITY_TOOLS, "provider_owned_tool"],
      initializationSucceeded: true,
    }),
    "available",
  );

  assert.equal(
    deriveWebCapabilityState({
      registeredTools: ["web_search"],
      initializationSucceeded: false,
    }),
    "degraded",
  );
  assert.equal(
    deriveWebCapabilityState({
      registeredTools: [],
      initializationSucceeded: false,
    }),
    "unavailable",
  );
});

test("owns and validates parent Web Capability Contracts", () => {
  for (const requirement of ["required", "optional", "none"] as const) {
    const contract = createParentWebCapabilityContract(requirement);
    assert.deepEqual(validateWebCapabilityContract(contract), {
      ok: true,
      contract,
    });
    assert.equal(contract.role, "parent");
    assert.deepEqual(
      contract.tools,
      requirement === "none" ? [] : WEB_CAPABILITY_TOOLS,
    );
  }

  const invalid = {
    ...createParentWebCapabilityContract("required"),
    tools: ["web_search", "web_search"],
    policy: "provider-defined",
    failureBehavior: "continue",
  };
  assert.deepEqual(validateWebCapabilityContract(invalid), {
    ok: false,
    errors: [
      "web tools must match the package-owned certified tool list",
      "web policy must be certified-tools-only",
      "required web capability must block on failure",
    ],
  });
});

test("preflight blocks required web research and discloses optional continuation", () => {
  const degraded = deriveWebCapabilityState({
    registeredTools: ["web_search"],
    initializationSucceeded: true,
  });
  const unavailable = deriveWebCapabilityState({
    registeredTools: [],
    initializationSucceeded: true,
  });

  for (const state of [degraded, unavailable]) {
    assert.deepEqual(
      preflightWebCapability(createParentWebCapabilityContract("required"), state),
      {
        status: "blocked",
        diagnostic: {
          kind: "web-capability-preflight",
          requirement: "required",
          state,
          message: `required web capability is ${state}`,
        },
      },
    );

    assert.deepEqual(
      preflightWebCapability(createParentWebCapabilityContract("optional"), state),
      {
        status: "disclosed-continuation",
        disclosure:
          "No web research was completed. Model knowledge is not web research.",
      },
    );
  }
});

test("no-web contracts proceed without tools and available contracts permit operations", () => {
  const unavailable = deriveWebCapabilityState({
    registeredTools: [],
    initializationSucceeded: false,
  });
  const available = deriveWebCapabilityState({
    registeredTools: WEB_CAPABILITY_TOOLS,
    initializationSucceeded: true,
  });

  assert.deepEqual(
    preflightWebCapability(createParentWebCapabilityContract("none"), unavailable),
    { status: "proceed-without-web" },
  );
  assert.deepEqual(
    preflightWebCapability(createParentWebCapabilityContract("required"), available),
    { status: "ready" },
  );
});

test("runtime failure blocks required research, discloses optional failure, and success alone completes research", () => {
  assert.deepEqual(
    runWebCapabilityOperation(createParentWebCapabilityContract("required"), {
      ok: false,
    }),
    {
      status: "blocked",
      diagnostic: {
        kind: "web-capability-runtime",
        requirement: "required",
        code: "web-operation-failed",
        message: "required web operation failed",
      },
    },
  );
  assert.deepEqual(
    runWebCapabilityOperation(createParentWebCapabilityContract("optional"), {
      ok: false,
    }),
    {
      status: "disclosed-continuation",
      disclosure:
        "No web research was completed. Model knowledge is not web research.",
    },
  );
  assert.deepEqual(
    runWebCapabilityOperation(createParentWebCapabilityContract("optional"), {
      ok: true,
      source: "web-tool",
    }),
    { status: "completed-research" },
  );
  assert.deepEqual(
    runWebCapabilityOperation(createParentWebCapabilityContract("none"), {
      ok: true,
      source: "web-tool",
    }),
    { status: "proceed-without-web" },
  );
});
