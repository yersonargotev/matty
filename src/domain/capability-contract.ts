export const INSPECTION_TOOLS = [
  "read",
  "grep",
  "find",
  "ls",
  "bash",
] as const;

export const EXPLORER_TOOLS = INSPECTION_TOOLS;

export const INSPECTION_ROLES = [
  "explorer",
  "designer",
  "reviewer",
] as const;

export type InspectionRole = (typeof INSPECTION_ROLES)[number];

export const INSPECTION_ROLE_INPUT_GUIDANCE =
  `{"role": ${
    INSPECTION_ROLES.map((role) => JSON.stringify(role)).join("|")
  }, "task": string}`;

export interface InspectionCapabilityContract {
  schemaVersion: 1;
  id: `delegate-${InspectionRole}`;
  role: InspectionRole;
  tools: readonly string[];
  writeAuthority: "none";
  mutationPolicy: "inspection-guard";
  web: "absent";
  github: "absent" | "required-readonly";
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

export type ExplorerCapabilityContract = InspectionCapabilityContract & {
  id: "delegate-explorer";
  role: "explorer";
  github: "absent";
};

function inspectionContract(
  role: InspectionRole,
  github: InspectionCapabilityContract["github"],
): InspectionCapabilityContract {
  return {
    schemaVersion: 1,
    id: `delegate-${role}`,
    role,
    tools: [...INSPECTION_TOOLS],
    writeAuthority: "none",
    mutationPolicy: "inspection-guard",
    web: "absent",
    github,
    cardinality: { min: 1, max: 1 },
    concurrency: { maxActive: 1 },
    independence: "required",
    failureBehavior: "fail-invocation",
  };
}

export const EXPLORER_CAPABILITY_CONTRACT =
  inspectionContract("explorer", "absent") as ExplorerCapabilityContract;
export const DESIGNER_CAPABILITY_CONTRACT =
  inspectionContract("designer", "absent");
export const REVIEWER_CAPABILITY_CONTRACT =
  inspectionContract("reviewer", "required-readonly");

export const INSPECTION_CAPABILITY_CONTRACTS = {
  explorer: EXPLORER_CAPABILITY_CONTRACT,
  designer: DESIGNER_CAPABILITY_CONTRACT,
  reviewer: REVIEWER_CAPABILITY_CONTRACT,
} as const satisfies Record<InspectionRole, InspectionCapabilityContract>;

export type CapabilityContractValidation =
  | {
      ok: true;
      contract: InspectionCapabilityContract;
    }
  | {
      ok: false;
      errors: string[];
    };

export interface CapabilityAvailability {
  availableTools: readonly string[];
  independentRuntime: boolean;
  inspectionGuard: boolean;
  github?: {
    available: boolean;
    authenticated: boolean;
  };
}

export interface CapabilityPreflightDiagnostic {
  kind: "capability-preflight";
  contractId: string;
  unmet: string[];
}

export type CapabilityPreflight<T extends InspectionCapabilityContract> =
  | {
      ok: true;
      contract: T;
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

export function isInspectionRole(value: unknown): value is InspectionRole {
  return INSPECTION_ROLES.some((role) => role === value);
}

export function inspectionCapabilityContract(
  role: InspectionRole,
): InspectionCapabilityContract {
  return INSPECTION_CAPABILITY_CONTRACTS[role];
}

export function validateCapabilityContract(
  value: unknown,
): CapabilityContractValidation {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return { ok: false, errors: ["contract must be an object"] };
  }

  const candidate = value as Partial<InspectionCapabilityContract>;
  const errors: string[] = [];
  const expected = isInspectionRole(candidate.role)
    ? inspectionCapabilityContract(candidate.role)
    : undefined;
  if (
    !expected ||
    candidate.schemaVersion !== 1 ||
    candidate.id !== expected.id ||
    candidate.web !== "absent" ||
    candidate.github !== expected.github ||
    candidate.independence !== "required" ||
    candidate.failureBehavior !== "fail-invocation" ||
    candidate.cardinality?.min !== 1 ||
    candidate.cardinality.max !== 1 ||
    candidate.concurrency?.maxActive !== 1
  ) {
    errors.push("contract does not match an inspection role v1 operation");
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
      tools.length !== INSPECTION_TOOLS.length ||
      INSPECTION_TOOLS.some((tool) => !tools.includes(tool))
    ) {
      errors.push("inspection tools must match the package-owned allowlist");
    }
  }

  if (candidate.writeAuthority !== "none") {
    errors.push("inspection role write authority must be none");
  }
  if (candidate.mutationPolicy !== "inspection-guard") {
    errors.push("inspection role mutation policy must be Inspection Guard");
  }

  if (errors.length > 0) {
    return { ok: false, errors };
  }
  return {
    ok: true,
    contract: value as InspectionCapabilityContract,
  };
}

export function preflightCapability<T extends InspectionCapabilityContract>(
  contract: T,
  availability: CapabilityAvailability,
): CapabilityPreflight<T> {
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
  for (const tool of availability.availableTools) {
    if (!contract.tools.includes(tool)) {
      unmet.push(`unapproved tool is available: ${tool}`);
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
  if (contract.github === "required-readonly") {
    if (!availability.github?.available) {
      unmet.push("GitHub CLI is unavailable");
    }
    if (!availability.github?.authenticated) {
      unmet.push("GitHub CLI authentication is unavailable");
    }
  }

  if (unmet.length > 0) {
    return {
      ok: false,
      diagnostic: createCapabilityPreflightDiagnostic(contract.id, unmet),
    };
  }
  return { ok: true, contract };
}
