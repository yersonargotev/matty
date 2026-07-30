export const WEB_CAPABILITY_TOOLS = [
  "web_search",
  "source_check",
  "fetch_content",
  "get_search_content",
] as const;

export type WebCapabilityRequirement = "required" | "optional" | "none";
export type WebCapabilityState =
  | "available"
  | "degraded"
  | "unavailable";

export interface WebCapabilityContract {
  schemaVersion: 1;
  id: "parent-web";
  role: "parent";
  requirement: WebCapabilityRequirement;
  tools: readonly string[];
  policy: "certified-tools-only";
  failureBehavior: "block" | "disclose" | "proceed-without-web";
}

export type WebCapabilityContractValidation =
  | { ok: true; contract: WebCapabilityContract }
  | { ok: false; errors: string[] };

export const WEB_RESEARCH_DISCLOSURE =
  "No web research was completed. Model knowledge is not web research.";

export function deriveWebCapabilityState(input: {
  registeredTools: readonly string[];
  initializationSucceeded: boolean;
}): WebCapabilityState {
  const registeredTools = [...input.registeredTools];
  if (registeredTools.length === 0) {
    return "unavailable";
  }

  const canonical =
    input.initializationSucceeded &&
    registeredTools.length === WEB_CAPABILITY_TOOLS.length &&
    registeredTools.every(
      (tool, index) => tool === WEB_CAPABILITY_TOOLS[index],
    );

  return canonical ? "available" : "degraded";
}

export function createParentWebCapabilityContract(
  requirement: WebCapabilityRequirement,
): WebCapabilityContract {
  const failureBehavior =
    requirement === "required"
      ? "block"
      : requirement === "optional"
        ? "disclose"
        : "proceed-without-web";

  return {
    schemaVersion: 1,
    id: "parent-web",
    role: "parent",
    requirement,
    tools: requirement === "none" ? [] : [...WEB_CAPABILITY_TOOLS],
    policy: "certified-tools-only",
    failureBehavior,
  };
}

export function validateWebCapabilityContract(
  value: unknown,
): WebCapabilityContractValidation {
  if (typeof value !== "object" || value === null) {
    return { ok: false, errors: ["web capability contract must be an object"] };
  }

  const candidate = value as Record<string, unknown>;
  const errors: string[] = [];
  const tools = candidate.tools;
  const expectedTools = candidate.requirement === "none"
    ? []
    : WEB_CAPABILITY_TOOLS;
  if (
    !Array.isArray(tools) ||
    tools.length !== expectedTools.length ||
    !tools.every((tool, index) => tool === expectedTools[index])
  ) {
    errors.push(
      "web tools must match the package-owned certified tool list",
    );
  }
  if (candidate.policy !== "certified-tools-only") {
    errors.push("web policy must be certified-tools-only");
  }

  const expectedFailureBehavior =
    candidate.requirement === "required"
      ? "block"
      : candidate.requirement === "optional"
        ? "disclose"
        : candidate.requirement === "none"
          ? "proceed-without-web"
          : undefined;
  if (candidate.failureBehavior !== expectedFailureBehavior) {
    if (candidate.requirement === "required") {
      errors.push("required web capability must block on failure");
    } else if (candidate.requirement === "optional") {
      errors.push("optional web capability must disclose failure");
    } else {
      errors.push("no-web capability must proceed without web");
    }
  }

  if (
    candidate.schemaVersion !== 1 ||
    candidate.id !== "parent-web" ||
    candidate.role !== "parent" ||
    expectedFailureBehavior === undefined
  ) {
    errors.push("web capability contract identity is invalid");
  }

  return errors.length > 0
    ? { ok: false, errors }
    : { ok: true, contract: value as WebCapabilityContract };
}

export type WebCapabilityPreflight =
  | { status: "ready" }
  | { status: "proceed-without-web" }
  | { status: "disclosed-continuation"; disclosure: string }
  | {
      status: "blocked";
      diagnostic: {
        kind: "web-capability-preflight";
        requirement: "required";
        state: "degraded" | "unavailable";
        message: string;
      };
    };

export function preflightWebCapability(
  contract: WebCapabilityContract,
  availability: WebCapabilityState,
): WebCapabilityPreflight {
  if (contract.requirement === "none") {
    return { status: "proceed-without-web" };
  }
  if (availability === "available") {
    return { status: "ready" };
  }
  if (contract.requirement === "optional") {
    return {
      status: "disclosed-continuation",
      disclosure: WEB_RESEARCH_DISCLOSURE,
    };
  }
  return {
    status: "blocked",
    diagnostic: {
      kind: "web-capability-preflight",
      requirement: "required",
      state: availability,
      message: `required web capability is ${availability}`,
    },
  };
}

export type WebOperationResult =
  | { ok: true; source: "web-tool" }
  | { ok: false };

export type WebOperationResolution =
  | { status: "completed-research" }
  | { status: "proceed-without-web" }
  | { status: "disclosed-continuation"; disclosure: string }
  | {
      status: "blocked";
      diagnostic: {
        kind: "web-capability-runtime";
        requirement: "required";
        code: "web-operation-failed";
        message: string;
      };
    };

export function runWebCapabilityOperation(
  contract: WebCapabilityContract,
  operation: WebOperationResult,
): WebOperationResolution {
  if (contract.requirement === "none") {
    return { status: "proceed-without-web" };
  }
  if (operation.ok) {
    return { status: "completed-research" };
  }
  if (contract.requirement === "optional") {
    return {
      status: "disclosed-continuation",
      disclosure: WEB_RESEARCH_DISCLOSURE,
    };
  }
  return {
    status: "blocked",
    diagnostic: {
      kind: "web-capability-runtime",
      requirement: "required",
      code: "web-operation-failed",
      message: "required web operation failed",
    },
  };
}
