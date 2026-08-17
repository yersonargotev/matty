import { reviewScope, type ReviewScopeContract } from "./review-scope.ts";
import {
  isInspectionRole,
  isMattyRole,
} from "./capability-contract.ts";

export type DelegationTaskDeclaration =
  | { role: "explorer"; task: string }
  | { role: "designer"; task: string }
  | { role: "worker"; task: string }
  | {
    role: "reviewer";
    task: string;
    reviewScope: ReviewScopeContract;
  }
  | {
    role: "researcher";
    task: string;
    web: "required" | "optional";
    report?: string;
  };

export type DelegationTaskContinuationDeclaration =
  | { role: "explorer" | "designer" | "worker" }
  | { role: "reviewer"; reviewScope: ReviewScopeContract }
  | {
    role: "researcher";
    web: "required" | "optional";
    report?: string;
  };

export interface DelegationGroupContract {
  schemaVersion: 1;
  id: "delegate-group";
  requirement: "required" | "optional";
  fallback: "none" | "skip";
  atomic: boolean;
  cardinality: {
    min: 1;
    max: 8;
  };
  concurrency: {
    maxActive: 4;
  };
  independence: "required";
  persistence: "persistent" | "ephemeral";
  tasks: readonly DelegationTaskDeclaration[];
}

export type DelegationGroupContractErrorCode =
  | "invalid-contract"
  | "invalid-group-policy"
  | "task-list-required"
  | "task-limit-exceeded"
  | "invalid-task"
  | "unsupported-role"
  | "empty-task"
  | "researcher-web-required"
  | "web-role-incompatible"
  | "report-role-incompatible"
  | "single-writer-required"
  | "research-report-conflict"
  | "optional-role-incompatible";

export interface DelegationGroupContractError {
  code: DelegationGroupContractErrorCode;
  taskIndex?: number;
}

export type DelegationGroupContractValidation =
  | { ok: true; contract: DelegationGroupContract }
  | { ok: false; errors: readonly DelegationGroupContractError[] };

function hasOnlyKeys(
  value: object,
  allowed: readonly string[],
): boolean {
  return Object.keys(value).every((key) => allowed.includes(key));
}

export function validateDelegationGroupContract(
  value: unknown,
): DelegationGroupContractValidation {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return { ok: false, errors: [{ code: "invalid-contract" }] };
  }

  const candidate = value as Partial<DelegationGroupContract>;
  const errors: DelegationGroupContractError[] = [];
  if (
    !hasOnlyKeys(candidate, [
      "schemaVersion",
      "id",
      "requirement",
      "fallback",
      "atomic",
      "cardinality",
      "concurrency",
      "independence",
      "persistence",
      "tasks",
    ])
  ) {
    errors.push({ code: "invalid-contract" });
  }
  if (
    candidate.schemaVersion !== 1 ||
    candidate.id !== "delegate-group" ||
    candidate.cardinality?.min !== 1 ||
    candidate.cardinality.max !== 8 ||
    candidate.concurrency?.maxActive !== 4 ||
    candidate.independence !== "required" ||
    (candidate.persistence !== undefined &&
      candidate.persistence !== "persistent" &&
      candidate.persistence !== "ephemeral") ||
    (
      candidate.requirement !== "required" &&
      candidate.requirement !== "optional"
    ) ||
    (
      candidate.requirement === "required" &&
      (candidate.fallback !== "none" || candidate.atomic !== true)
    ) ||
    (
      candidate.requirement === "optional" &&
      (candidate.fallback !== "skip" || candidate.atomic !== false)
    )
  ) {
    errors.push({ code: "invalid-group-policy" });
  }

  if (!Array.isArray(candidate.tasks) || candidate.tasks.length === 0) {
    errors.push({ code: "task-list-required" });
  } else if (candidate.tasks.length > 8) {
    errors.push({ code: "task-limit-exceeded" });
  }

  let workers = 0;
  const researchReports = new Set<string>();
  let researchReportConflict = false;
  if (Array.isArray(candidate.tasks)) {
    for (const [taskIndex, task] of candidate.tasks.entries()) {
      if (typeof task !== "object" || task === null || Array.isArray(task)) {
        errors.push({ code: "invalid-task", taskIndex });
        continue;
      }
      const declaration = task as Record<string, unknown>;
      if (!hasOnlyKeys(declaration, ["role", "task", "web", "report", "reviewScope"])) {
        errors.push({ code: "invalid-task", taskIndex });
      }
      if (!isMattyRole(declaration.role)) {
        errors.push({ code: "unsupported-role", taskIndex });
        continue;
      }
      if (declaration.role === "worker") {
        workers += 1;
      }
      if (
        typeof declaration.task !== "string" ||
        declaration.task.trim().length === 0
      ) {
        errors.push({ code: "empty-task", taskIndex });
      }
      if (declaration.role === "researcher") {
        if (
          declaration.web !== "required" &&
          declaration.web !== "optional"
        ) {
          errors.push({ code: "researcher-web-required", taskIndex });
        }
        if (
          declaration.report !== undefined &&
          (
            typeof declaration.report !== "string" ||
            declaration.report.trim().length === 0
          )
        ) {
          errors.push({ code: "invalid-task", taskIndex });
        } else if (typeof declaration.report === "string") {
          if (researchReports.has(declaration.report)) {
            researchReportConflict = true;
          }
          researchReports.add(declaration.report);
        }
      } else {
        if (declaration.web !== undefined) {
          errors.push({ code: "web-role-incompatible", taskIndex });
        }
        if (declaration.report !== undefined) {
          errors.push({ code: "report-role-incompatible", taskIndex });
        }
      }
      if (declaration.role === "reviewer") {
        try { reviewScope(declaration.reviewScope); } catch {
          errors.push({ code: "invalid-task", taskIndex });
        }
      } else if (declaration.reviewScope !== undefined) {
        errors.push({ code: "invalid-task", taskIndex });
      }
      if (
        candidate.requirement === "optional" &&
        !isInspectionRole(declaration.role)
      ) {
        errors.push({ code: "optional-role-incompatible", taskIndex });
      }
    }
  }
  if (workers > 1) {
    errors.push({ code: "single-writer-required" });
  }
  if (researchReportConflict) {
    errors.push({ code: "research-report-conflict" });
  }

  if (errors.length > 0) {
    return { ok: false, errors };
  }
  return {
    ok: true,
    contract: {
      ...(value as Omit<DelegationGroupContract, "persistence">),
      persistence: candidate.persistence ?? "persistent",
    },
  };
}
