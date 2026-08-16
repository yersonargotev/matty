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
import {
  reviewScope,
  reviewerFindings,
  type ReviewScopeContract,
  type ReviewerFindings,
} from "../domain/review-scope.ts";

export interface InspectionDelegationExecution {
  availability: CapabilityAvailability;
  reviewCommitsAvailable?(scope: ReviewScopeContract): Promise<boolean>;
  createRunner(): DelegatedTaskRunner;
}

export interface InspectionDelegationOptions {
  signal?: AbortSignal;
  onProgress?: (progress: DelegatedTaskProgress) => void;
  reviewScope?: ReviewScopeContract;
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
    ? Omit<Outcome, "output"> & { output: InspectionRoleFindings | ReviewerFindings }
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

function inspectionTask(
  role: InspectionRole,
  task: string,
  scope?: ReviewScopeContract,
): string {
  const roleGuidance = role === "reviewer"
    ? "You may use guarded bash for read-only GitHub inspection after preflight."
    : "Do not use GitHub CLI.";
  return [
    `${role[0]?.toUpperCase()}${role.slice(1)} assignment:`,
    task.trim(),
    "",
    "Inspect only. Use read, grep, find, ls, or guarded bash for local Git, shell, and diagnostics.",
    roleGuidance,
    ...(role === "reviewer" && scope
      ? [
        "Review Scope Contract (exact JSON; do not expand it):",
        JSON.stringify(scope),
        "Bind every finding to candidateSha. Treat listed dependent/out-of-scope issues as excluded.",
      ]
      : []),
    role === "explorer"
      ? "Do not mutate local or remote state. Return concise evidence to the parent."
      : role === "reviewer"
      ? 'Do not mutate local or remote state. Return JSON exactly {"schemaVersion":1,"candidateSha":string,"summary":string,"findings":[{"axis":string,"severity":"blocking"|"non-blocking","requirement":string,"evidence":string}]}. The parent adjudicates scope and contradictions.'
      : 'Do not mutate local or remote state. Return JSON exactly {"summary": string, "evidence": array}.',
  ].join("\n");
}

function structuredOutcome(
  outcome: DelegatedTaskOutcome,
  required: boolean,
  scope?: ReviewScopeContract,
): InspectionDelegationOutcome {
  if (outcome.status !== "succeeded") {
    return outcome;
  }
  try {
    const output = JSON.parse(outcome.output) as unknown;
    if (scope) {
      return { ...outcome, output: reviewerFindings(output, scope) };
    }
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
  let scope: ReviewScopeContract | undefined;
  if (role === "reviewer") {
    try {
      scope = reviewScope(options.reviewScope);
    } catch {
      return blockedInspectionDelegation(role, ["valid Review Scope Contract is required"]);
    }
    try {
      if (!execution.reviewCommitsAvailable ||
          !(await execution.reviewCommitsAvailable(scope))) {
        return blockedInspectionDelegation(role, ["review-commit-unavailable"]);
      }
    } catch {
      return blockedInspectionDelegation(role, ["review-commit-unavailable"]);
    }
  }

  const runner = execution.createRunner();
  const outcome = structuredOutcome(
    await runner.run(inspectionTask(role, task, scope), options),
    role !== "explorer",
    scope,
  );
  return {
    contract: preflight.contract,
    outcome,
  };
}
