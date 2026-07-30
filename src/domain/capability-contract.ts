export const EXPLORER_TOOLS = [
  "read",
  "grep",
  "find",
  "ls",
  "bash",
] as const;

export interface ExplorerCapabilityContract {
  schemaVersion: 1;
  id: "delegate-explorer";
  role: "explorer";
  tools: readonly string[];
  writeAuthority: "none";
  web: "absent";
  cardinality: {
    min: 1;
    max: 1;
  };
  concurrency: {
    maxActive: 1;
  };
  independence: "required";
  failureBehavior: "fail-invocation";
}

export const EXPLORER_CAPABILITY_CONTRACT: ExplorerCapabilityContract = {
  schemaVersion: 1,
  id: "delegate-explorer",
  role: "explorer",
  tools: [...EXPLORER_TOOLS],
  writeAuthority: "none",
  web: "absent",
  cardinality: { min: 1, max: 1 },
  concurrency: { maxActive: 1 },
  independence: "required",
  failureBehavior: "fail-invocation",
};

export type CapabilityContractValidation =
  | {
      ok: true;
      contract: ExplorerCapabilityContract;
    }
  | {
      ok: false;
      errors: string[];
    };

export interface CapabilityAvailability {
  availableTools: readonly string[];
  independentRuntime: boolean;
  inspectionGuard: boolean;
}

export interface CapabilityPreflightDiagnostic {
  kind: "capability-preflight";
  contractId: string;
  unmet: string[];
}

export type CapabilityPreflight =
  | {
      ok: true;
      contract: ExplorerCapabilityContract;
    }
  | {
      ok: false;
      diagnostic: CapabilityPreflightDiagnostic;
    };

export function createCapabilityPreflightDiagnostic(
  contractId: string,
  unmet: string[],
): CapabilityPreflightDiagnostic {
  return {
    kind: "capability-preflight",
    contractId,
    unmet,
  };
}

export function validateCapabilityContract(
  value: unknown,
): CapabilityContractValidation {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return { ok: false, errors: ["contract must be an object"] };
  }

  const candidate = value as Partial<ExplorerCapabilityContract>;
  const errors: string[] = [];
  if (
    candidate.schemaVersion !== 1 ||
    candidate.id !== "delegate-explorer" ||
    candidate.role !== "explorer" ||
    candidate.web !== "absent" ||
    candidate.independence !== "required" ||
    candidate.failureBehavior !== "fail-invocation" ||
    candidate.cardinality?.min !== 1 ||
    candidate.cardinality.max !== 1 ||
    candidate.concurrency?.maxActive !== 1
  ) {
    errors.push("contract does not match the explorer v1 operation");
  }

  if (!Array.isArray(candidate.tools)) {
    errors.push("tools must be an array");
  } else {
    const tools = candidate.tools as unknown[];
    if (
      tools.some((tool) => typeof tool !== "string") ||
      tools.length !== new Set(tools).size
    ) {
      errors.push("tools must be unique");
    }
    if (
      tools.length !== EXPLORER_TOOLS.length ||
      EXPLORER_TOOLS.some((tool) => !tools.includes(tool))
    ) {
      errors.push("explorer tools must match the package-owned allowlist");
    }
  }

  if (candidate.writeAuthority !== "none") {
    errors.push("explorer write authority must be none");
  }

  if (errors.length > 0) {
    return { ok: false, errors };
  }
  return {
    ok: true,
    contract: value as ExplorerCapabilityContract,
  };
}

export function preflightCapability(
  contract: ExplorerCapabilityContract,
  availability: CapabilityAvailability,
): CapabilityPreflight {
  const validation = validateCapabilityContract(contract);
  const unmet: string[] = [];
  if (!validation.ok) {
    unmet.push(...validation.errors);
  }
  for (const tool of contract.tools) {
    if (!availability.availableTools.includes(tool)) {
      unmet.push(`required tool is unavailable: ${tool}`);
    }
  }
  if (
    contract.independence === "required" &&
    !availability.independentRuntime
  ) {
    unmet.push("independent Subagent Runtime is unavailable");
  }
  if (!availability.inspectionGuard) {
    unmet.push("Inspection Guard is unavailable");
  }

  if (unmet.length > 0) {
    return {
      ok: false,
      diagnostic: createCapabilityPreflightDiagnostic(contract.id, unmet),
    };
  }
  return { ok: true, contract };
}
