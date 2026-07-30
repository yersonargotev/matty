import type {
  DelegatedTaskOutcome,
  DelegatedTaskProgress,
  DelegatedTaskRunner,
} from "./child-pi-runtime.ts";
import {
  createCapabilityPreflightDiagnostic,
  inspectionCapabilityContract,
  preflightCapability,
  type CapabilityAvailability,
  type CapabilityPreflightDiagnostic,
  type InspectionCapabilityContract,
  type InspectionRole,
} from "../domain/capability-contract.ts";

export interface InspectionDelegationExecution {
  availability: CapabilityAvailability;
  createRunner(): DelegatedTaskRunner;
}

export interface InspectionDelegationOptions {
  signal?: AbortSignal;
  onProgress?: (progress: DelegatedTaskProgress) => void;
}

export interface BlockedInspectionOutcome {
  status: "blocked";
  diagnostic: CapabilityPreflightDiagnostic;
}

export interface InspectionRoleFindings {
  summary: string;
  evidence: unknown[];
}

type SuccessfulInspectionOutcome =
  Extract<DelegatedTaskOutcome, { status: "succeeded" }> extends infer Outcome
    ? Omit<Outcome, "output"> & { output: InspectionRoleFindings }
    : never;

interface InvalidInspectionOutput {
  status: "failed";
  child: Extract<
    DelegatedTaskOutcome,
    { status: "succeeded" }
  >["child"];
  failure: {
    kind: "invalid-role-output";
    message: string;
  };
  exit: Extract<
    DelegatedTaskOutcome,
    { status: "succeeded" }
  >["exit"];
}

export type InspectionDelegationOutcome =
  | DelegatedTaskOutcome
  | SuccessfulInspectionOutcome
  | InvalidInspectionOutput;

export interface InspectionDelegationTerminal {
  contract: InspectionCapabilityContract;
  outcome: InspectionDelegationOutcome | BlockedInspectionOutcome;
}

export function blockedInspectionDelegation(
  role: InspectionRole,
  unmet: string[],
): InspectionDelegationTerminal {
  const contract = inspectionCapabilityContract(role);
  return {
    contract,
    outcome: {
      status: "blocked",
      diagnostic: createCapabilityPreflightDiagnostic(contract.id, unmet),
    },
  };
}

function inspectionTask(role: InspectionRole, task: string): string {
  const roleGuidance = role === "reviewer"
    ? "You may use guarded bash for read-only GitHub inspection after preflight."
    : "Do not use GitHub CLI.";
  return [
    `${role[0]?.toUpperCase()}${role.slice(1)} assignment:`,
    task.trim(),
    "",
    "Inspect only. Use read, grep, find, ls, or guarded bash for local Git, CodeGraph, shell, and diagnostics.",
    roleGuidance,
    role === "explorer"
      ? "Do not mutate local or remote state. Return concise evidence to the parent."
      : 'Do not mutate local or remote state. Return JSON exactly {"summary": string, "evidence": array}.',
  ].join("\n");
}

function structuredOutcome(
  outcome: DelegatedTaskOutcome,
  required: boolean,
): InspectionDelegationOutcome {
  if (outcome.status !== "succeeded") {
    return outcome;
  }
  try {
    const output = JSON.parse(outcome.output) as unknown;
    if (
      typeof output === "object" &&
      output !== null &&
      !Array.isArray(output) &&
      typeof (output as Partial<InspectionRoleFindings>).summary === "string" &&
      Array.isArray((output as Partial<InspectionRoleFindings>).evidence)
    ) {
      return { ...outcome, output: output as InspectionRoleFindings };
    }
  } catch {
    // Converted to a structured role failure below.
  }
  if (!required) {
    return outcome;
  }
  return {
    status: "failed",
    child: outcome.child,
    failure: {
      kind: "invalid-role-output",
      message: "inspection role output must be structured JSON findings",
    },
    exit: outcome.exit,
  };
}

export async function runInspectionDelegation(
  role: InspectionRole,
  task: string,
  execution: InspectionDelegationExecution,
  options: InspectionDelegationOptions = {},
): Promise<InspectionDelegationTerminal> {
  const contract = inspectionCapabilityContract(role);
  const preflight = preflightCapability(contract, execution.availability);
  if (!preflight.ok) {
    return blockedInspectionDelegation(role, preflight.diagnostic.unmet);
  }

  const runner = execution.createRunner();
  const outcome = structuredOutcome(
    await runner.run(inspectionTask(role, task), options),
    role !== "explorer",
  );
  return {
    contract: preflight.contract,
    outcome,
  };
}
