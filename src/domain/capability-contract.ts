import { basename, dirname } from "node:path";

import {
  WEB_CAPABILITY_TOOLS,
  type WebCapabilityState,
} from "./web-capability.ts";
import {
  isPathWithin,
  isResearchRunId,
} from "./research-paths.ts";

export const INSPECTION_TOOLS = [
  "read",
  "grep",
  "find",
  "ls",
  "bash",
] as const;

export const EXPLORER_TOOLS = INSPECTION_TOOLS;

export const WORKER_TOOLS = [
  "read",
  "write",
  "edit",
  "grep",
  "find",
  "ls",
  "bash",
] as const;

export const RESEARCH_FILE_TOOL = "research_file";
export const RESEARCHER_TOOLS = [
  ...WEB_CAPABILITY_TOOLS,
  RESEARCH_FILE_TOOL,
] as const;

export const INSPECTION_ROLES = [
  "explorer",
  "designer",
  "reviewer",
] as const;

export type InspectionRole = (typeof INSPECTION_ROLES)[number];

export const MATTY_ROLES = [
  ...INSPECTION_ROLES,
  "researcher",
  "worker",
] as const;
export type MattyRole = (typeof MATTY_ROLES)[number];

export const DELEGATION_INPUT_GUIDANCE =
  `{"requirement": "required"|"optional", "persistence"?: "persistent"|"ephemeral", "tasks": [{"role": ${
    [...INSPECTION_ROLES, "researcher", "worker"].map((role) =>
      JSON.stringify(role)
    ).join("|")
  }, "task": string, "web"?: "required"|"optional", "report"?: string, "reviewScope"?: {"schemaVersion": 1, "issue": {"repository": string, "number": number, "reference": string}, "requirements": string[], "outOfScope": [{"reference": string, "reason": string}], "baseSha": string, "candidateSha": string, "axes": string[]}}]}`;
export const INSPECTION_ROLE_INPUT_GUIDANCE =
  `{"role": ${
    INSPECTION_ROLES.map((role) => JSON.stringify(role)).join("|")
  }, "task": string}`;

export interface InspectionCapabilityContract {
  schemaVersion: 1;
  id: `delegate-${InspectionRole}`;
  requirement: "required";
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

export interface WorkerCapabilityContract {
  schemaVersion: 1;
  id: "delegate-worker";
  requirement: "required";
  role: "worker";
  tools: readonly string[];
  writeAuthority: "trusted-working-tree";
  mutationPolicy: "worker-guard";
  web: "absent";
  github: "absent";
  workingTree: string;
  temporaryPaths: readonly string[];
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

export interface ResearcherCapabilityContract {
  schemaVersion: 1;
  id: "delegate-researcher";
  requirement: "required";
  role: "researcher";
  tools: readonly string[];
  writeAuthority: "research-artifacts";
  mutationPolicy: "bounded-research-files";
  web: "required" | "optional";
  github: "absent";
  workspaceRoot: string;
  projectRoot: string;
  workspace: string;
  report: string;
  writeLimits: {
    workspaceFiles: "multiple";
    researchReports: 1;
    overwrite: "forbidden";
  };
  cardinality: {
    min: 1;
    max: 1;
  };
  concurrency: {
    maxActive: 1;
  };
  independence: "required";
  failureBehavior: "fail-invocation" | "disclose-web-failure";
}

export type CapabilityContract =
  | InspectionCapabilityContract
  | ResearcherCapabilityContract
  | WorkerCapabilityContract;

export function createResearcherCapabilityContract(
  scope: Pick<
    ResearcherCapabilityContract,
    "web" | "workspaceRoot" | "projectRoot" | "workspace" | "report"
  >,
): ResearcherCapabilityContract {
  return {
    schemaVersion: 1,
    id: "delegate-researcher",
    requirement: "required",
    role: "researcher",
    tools: [...RESEARCHER_TOOLS],
    writeAuthority: "research-artifacts",
    mutationPolicy: "bounded-research-files",
    web: scope.web,
    github: "absent",
    workspaceRoot: scope.workspaceRoot,
    projectRoot: scope.projectRoot,
    workspace: scope.workspace,
    report: scope.report,
    writeLimits: {
      workspaceFiles: "multiple",
      researchReports: 1,
      overwrite: "forbidden",
    },
    cardinality: { min: 1, max: 1 },
    concurrency: { maxActive: 1 },
    independence: "required",
    failureBehavior: scope.web === "required"
      ? "fail-invocation"
      : "disclose-web-failure",
  };
}

export function createWorkerCapabilityContract(
  scope: Pick<WorkerCapabilityContract, "workingTree" | "temporaryPaths">,
): WorkerCapabilityContract {
  return {
    schemaVersion: 1,
    id: "delegate-worker",
    requirement: "required",
    role: "worker",
    tools: [...WORKER_TOOLS],
    writeAuthority: "trusted-working-tree",
    mutationPolicy: "worker-guard",
    web: "absent",
    github: "absent",
    workingTree: scope.workingTree,
    temporaryPaths: [...scope.temporaryPaths],
    cardinality: { min: 1, max: 1 },
    concurrency: { maxActive: 1 },
    independence: "required",
    failureBehavior: "fail-invocation",
  };
}

function inspectionContract(
  role: InspectionRole,
  github: InspectionCapabilityContract["github"],
): InspectionCapabilityContract {
  return {
    schemaVersion: 1,
    id: `delegate-${role}`,
    requirement: "required",
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
      contract: CapabilityContract;
    }
  | {
      ok: false;
      errors: string[];
    };

export interface CapabilityAvailability {
  availableTools: readonly string[];
  independentRuntime: boolean;
  inspectionGuard: boolean;
  workerGuard?: boolean;
  researchFileTool?: boolean;
  web?: WebCapabilityState;
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

export type CapabilityPreflight<T extends CapabilityContract> =
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

export function isMattyRole(value: unknown): value is MattyRole {
  return MATTY_ROLES.some((role) => role === value);
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

  const candidate = value as Partial<CapabilityContract>;
  if (candidate.role === "worker") {
    return validateWorkerCapabilityContract(
      value as Partial<WorkerCapabilityContract>,
    );
  }
  if (candidate.role === "researcher") {
    return validateResearcherCapabilityContract(
      value as Partial<ResearcherCapabilityContract>,
    );
  }
  const errors: string[] = [];
  const expected = isInspectionRole(candidate.role)
    ? inspectionCapabilityContract(candidate.role)
    : undefined;
  if (
    !expected ||
    candidate.schemaVersion !== 1 ||
    candidate.id !== expected.id ||
    candidate.requirement !== "required" ||
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

function isAbsoluteNormalizedPath(value: unknown): value is string {
  if (typeof value !== "string" || !value.startsWith("/")) {
    return false;
  }
  const segments = value.split("/");
  return !segments.includes(".") &&
    !segments.includes("..") &&
    !value.includes("//") &&
    (value === "/" || !value.endsWith("/"));
}

function validateWorkerCapabilityContract(
  candidate: Partial<WorkerCapabilityContract>,
): CapabilityContractValidation {
  const errors: string[] = [];
  if (
    candidate.schemaVersion !== 1 ||
    candidate.id !== "delegate-worker" ||
    candidate.requirement !== "required" ||
    candidate.role !== "worker" ||
    candidate.web !== "absent" ||
    candidate.github !== "absent" ||
    candidate.independence !== "required" ||
    candidate.failureBehavior !== "fail-invocation"
  ) {
    errors.push("contract does not match the worker v1 operation");
  }
  if (
    candidate.cardinality?.min !== 1 ||
    candidate.cardinality.max !== 1 ||
    candidate.concurrency?.maxActive !== 1
  ) {
    errors.push("worker contract requires one writer");
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
      tools.length !== WORKER_TOOLS.length ||
      WORKER_TOOLS.some((tool) => !tools.includes(tool))
    ) {
      errors.push("worker tools must match the package-owned allowlist");
    }
  }
  if (candidate.writeAuthority !== "trusted-working-tree") {
    errors.push("worker write authority must be the trusted working tree");
  }
  if (candidate.mutationPolicy !== "worker-guard") {
    errors.push("worker mutation policy must be Worker Guard");
  }
  if (!isAbsoluteNormalizedPath(candidate.workingTree)) {
    errors.push("working tree must be an absolute normalized path");
  }
  if (
    !Array.isArray(candidate.temporaryPaths) ||
    candidate.temporaryPaths.length === 0 ||
    candidate.temporaryPaths.some((path) => !isAbsoluteNormalizedPath(path)) ||
    candidate.temporaryPaths.length !== new Set(candidate.temporaryPaths).size
  ) {
    errors.push("temporary paths must be unique absolute normalized paths");
  }
  if (errors.length > 0) {
    return { ok: false, errors };
  }
  return {
    ok: true,
    contract: candidate as WorkerCapabilityContract,
  };
}

function validateResearcherCapabilityContract(
  candidate: Partial<ResearcherCapabilityContract>,
): CapabilityContractValidation {
  const errors: string[] = [];
  const expectedFailureBehavior = candidate.web === "required"
    ? "fail-invocation"
    : candidate.web === "optional"
      ? "disclose-web-failure"
      : undefined;
  if (
    candidate.schemaVersion !== 1 ||
    candidate.id !== "delegate-researcher" ||
    candidate.requirement !== "required" ||
    candidate.role !== "researcher" ||
    candidate.github !== "absent" ||
    candidate.independence !== "required" ||
    candidate.failureBehavior !== expectedFailureBehavior
  ) {
    errors.push("contract does not match the researcher v1 operation");
  }
  if (
    candidate.cardinality?.min !== 1 ||
    candidate.cardinality.max !== 1 ||
    candidate.concurrency?.maxActive !== 1 ||
    candidate.writeLimits?.workspaceFiles !== "multiple" ||
    candidate.writeLimits.researchReports !== 1 ||
    candidate.writeLimits.overwrite !== "forbidden"
  ) {
    errors.push("researcher contract requires exactly one bounded report");
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
      tools.length !== RESEARCHER_TOOLS.length ||
      RESEARCHER_TOOLS.some((tool) => !tools.includes(tool))
    ) {
      errors.push("researcher tools must match the package-owned allowlist");
    }
  }
  if (candidate.writeAuthority !== "research-artifacts") {
    errors.push("researcher write authority must be bounded artifacts");
  }
  if (candidate.mutationPolicy !== "bounded-research-files") {
    errors.push("researcher mutation policy must be bounded research files");
  }
  const workspaceRoot = candidate.workspaceRoot;
  const projectRoot = candidate.projectRoot;
  const workspace = candidate.workspace;
  const report = candidate.report;
  const workspaceRootValid = isAbsoluteNormalizedPath(workspaceRoot);
  const projectRootValid = isAbsoluteNormalizedPath(projectRoot);
  const workspaceValid = isAbsoluteNormalizedPath(workspace);
  const reportPathValid = isAbsoluteNormalizedPath(report);
  if (!workspaceRootValid || !projectRootValid) {
    errors.push("research authority roots must be absolute normalized paths");
  }
  if (!workspaceValid) {
    errors.push("research workspace must be an absolute normalized path");
  }
  if (
    !reportPathValid ||
    (reportPathValid && !report.endsWith(".md"))
  ) {
    errors.push("research report must be an absolute normalized Markdown path");
  }
  if (
    workspaceRootValid &&
    projectRootValid &&
    workspaceValid &&
    reportPathValid &&
    (
      !workspaceRoot.endsWith("/matty/research") ||
      dirname(workspace) !== workspaceRoot ||
      !isResearchRunId(basename(workspace)) ||
      !isPathWithin(projectRoot, report) ||
      report === projectRoot
    )
  ) {
    errors.push("research artifact paths exceed their declared authority");
  }
  if (errors.length > 0) {
    return { ok: false, errors };
  }
  return {
    ok: true,
    contract: candidate as ResearcherCapabilityContract,
  };
}

export function preflightCapability<T extends CapabilityContract>(
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
  if (contract.role === "worker") {
    if (!availability.workerGuard) {
      unmet.push("Worker Guard is unavailable");
    }
  } else if (contract.role === "researcher") {
    if (!availability.researchFileTool) {
      unmet.push("Research File tool is unavailable");
    }
    if (
      contract.web === "required" &&
      availability.web !== "available"
    ) {
      unmet.push(
        `required web capability is ${availability.web ?? "unavailable"}`,
      );
    }
  } else if (!availability.inspectionGuard) {
    unmet.push("Inspection Guard is unavailable");
  }
  if (
    contract.role !== "worker" &&
    contract.github === "required-readonly"
  ) {
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
