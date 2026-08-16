import {
  validateDelegationGroupContract,
  type DelegationGroupContract,
  type DelegationGroupContractError,
  type DelegationTaskDeclaration,
} from "../domain/delegation-group.ts";
import type { MattyRole } from "../domain/capability-contract.ts";

export const DELEGATION_LEAF_FAILURE_CODES = [
  "child-failed",
  "protocol-failed",
  "child-exited",
  "missing-research-report",
  "invalid-role-output",
] as const;

export type DelegationLeafFailureCode =
  (typeof DELEGATION_LEAF_FAILURE_CODES)[number];

export function isDelegationLeafFailureCode(
  value: unknown,
): value is DelegationLeafFailureCode {
  return typeof value === "string" &&
    DELEGATION_LEAF_FAILURE_CODES.some((code) => code === value);
}

export type DelegationDiagnosticCode =
  | "queued"
  | "preflight-failed"
  | "skipped"
  | "task-failed"
  | "cancelled"
  | "partial-failure"
  | DelegationLeafFailureCode;

export interface DelegationDiagnostic {
  kind: "delegation";
  code: DelegationDiagnosticCode;
  taskIndex?: number;
  role?: MattyRole;
  phase?: "before-spawn" | "running";
  reason?: DelegationPreflightReason;
}

export type DelegationPreflightReason =
  | "authentication-unavailable"
  | "runtime-unavailable"
  | "rules-conflict"
  | "github-unavailable"
  | "review-commit-unavailable"
  | "web-unavailable"
  | "writer-unavailable"
  | "artifact-destination-invalid"
  | "tool-surface-incompatible"
  | "capability-unavailable";

export type DelegationTaskPreflight =
  | { ok: true }
  | { ok: false; reason: DelegationPreflightReason };

export type DelegationTaskExecution<T> =
  | { status: "succeeded"; value: T }
  | { status: "failed"; code?: DelegationLeafFailureCode }
  | { status: "cancelled" };

export interface DelegationGroupExecution<T> {
  preflight(
    task: DelegationTaskDeclaration,
    taskIndex: number,
  ): Promise<DelegationTaskPreflight>;
  run(
    task: DelegationTaskDeclaration,
    taskIndex: number,
    options: { signal: AbortSignal },
  ): Promise<DelegationTaskExecution<T>>;
}

export interface DelegationGroupOptions {
  signal?: AbortSignal;
  onDiagnostic?: (diagnostic: DelegationDiagnostic) => void;
}

export type DelegationTaskResult<T> =
  | {
      taskIndex: number;
      role: MattyRole;
      status: "succeeded";
      value: T;
    }
  | {
      taskIndex: number;
      role: MattyRole;
      status: "failed" | "cancelled" | "skipped";
      diagnostic: DelegationDiagnostic;
    };

export type DelegationGroupResult<T> =
  | {
      status: "blocked";
      tasks: [];
      validationErrors: readonly DelegationGroupContractError[];
      diagnostics: readonly DelegationDiagnostic[];
    }
  | {
      status: "blocked";
      tasks: readonly DelegationTaskResult<T>[];
      diagnostics: readonly DelegationDiagnostic[];
    }
  | {
      status: "succeeded" | "partial" | "failed" | "cancelled";
      tasks: readonly DelegationTaskResult<T>[];
      diagnostics: readonly DelegationDiagnostic[];
    };

export async function runDelegationGroup<T>(
  input: unknown,
  execution: DelegationGroupExecution<T>,
  options: DelegationGroupOptions = {},
): Promise<DelegationGroupResult<T>> {
  const validation = validateDelegationGroupContract(input);
  if (!validation.ok) {
    return {
      status: "blocked",
      tasks: [],
      validationErrors: validation.errors,
      diagnostics: [],
    };
  }

  const contract: DelegationGroupContract = validation.contract;
  const diagnostics: DelegationDiagnostic[] = [];
  const emit = (diagnostic: DelegationDiagnostic) => {
    diagnostics.push(diagnostic);
    options.onDiagnostic?.(diagnostic);
  };
  const cancelBeforeSpawn = (): DelegationGroupResult<T> => {
    const tasks = contract.tasks.map((task, taskIndex) => {
      const diagnostic: DelegationDiagnostic = {
        kind: "delegation",
        code: "cancelled",
        taskIndex,
        role: task.role,
        phase: "before-spawn",
      };
      emit(diagnostic);
      return {
        taskIndex,
        role: task.role,
        status: "cancelled" as const,
        diagnostic,
      };
    });
    return { status: "cancelled", tasks, diagnostics };
  };

  if (options.signal?.aborted) {
    return cancelBeforeSpawn();
  }

  const preflights = await Promise.all(
    contract.tasks.map(async (task, taskIndex) => {
      try {
        return await execution.preflight(task, taskIndex);
      } catch {
        return {
          ok: false,
          reason: "capability-unavailable",
        } as const;
      }
    }),
  );
  const ready = preflights.map((preflight) => preflight.ok);
  if (options.signal?.aborted) {
    return cancelBeforeSpawn();
  }
  const results: Array<DelegationTaskResult<T> | undefined> =
    Array.from({ length: contract.tasks.length });

  if (contract.requirement === "required" && ready.includes(false)) {
    for (const [taskIndex, task] of contract.tasks.entries()) {
      const diagnostic: DelegationDiagnostic = ready[taskIndex]
        ? {
          kind: "delegation",
          code: "cancelled",
          taskIndex,
          role: task.role,
          phase: "before-spawn",
        }
        : {
          kind: "delegation",
          code: "preflight-failed",
          taskIndex,
          role: task.role,
          reason: preflights[taskIndex]?.ok === false
            ? preflights[taskIndex].reason
            : "capability-unavailable",
        };
      emit(diagnostic);
      results[taskIndex] = {
        taskIndex,
        role: task.role,
        status: ready[taskIndex] ? "cancelled" : "failed",
        diagnostic,
      };
    }
    return {
      status: "blocked",
      tasks: results.filter(
        (result): result is DelegationTaskResult<T> => result !== undefined,
      ),
      diagnostics,
    };
  }

  if (contract.requirement === "optional") {
    for (const [taskIndex, task] of contract.tasks.entries()) {
      if (ready[taskIndex]) {
        continue;
      }
      const diagnostic: DelegationDiagnostic = {
        kind: "delegation",
        code: "skipped",
        taskIndex,
        role: task.role,
        phase: "before-spawn",
        reason: preflights[taskIndex]?.ok === false
          ? preflights[taskIndex].reason
          : "capability-unavailable",
      };
      emit(diagnostic);
      results[taskIndex] = {
        taskIndex,
        role: task.role,
        status: "skipped",
        diagnostic,
      };
    }
  }

  const readyTaskIndexes = ready.flatMap((isReady, taskIndex) =>
    isReady ? [taskIndex] : []
  );
  for (
    const taskIndex of readyTaskIndexes.slice(contract.concurrency.maxActive)
  ) {
    const task = contract.tasks[taskIndex];
    if (task) {
      emit({
        kind: "delegation",
        code: "queued",
        taskIndex,
        role: task.role,
      });
    }
  }

  let nextTaskIndex = 0;
  let stopped = false;
  let requiredTaskFailed = false;
  const controllers = new Map<number, AbortController>();
  const abortRunning = () => {
    stopped = true;
    for (const controller of controllers.values()) {
      controller.abort();
    }
  };
  options.signal?.addEventListener("abort", abortRunning, { once: true });
  const executionSlots = Array.from(
    {
      length: Math.min(
        contract.concurrency.maxActive,
        contract.tasks.length,
      ),
    },
    async () => {
      while (!stopped && nextTaskIndex < contract.tasks.length) {
        const taskIndex = nextTaskIndex;
        nextTaskIndex += 1;
        const task = contract.tasks[taskIndex];
        if (!task || !ready[taskIndex]) {
          continue;
        }
        const controller = new AbortController();
        controllers.set(taskIndex, controller);
        try {
          let outcome: DelegationTaskExecution<T>;
          try {
            outcome = await execution.run(task, taskIndex, {
              signal: controller.signal,
            });
          } catch {
            outcome = { status: "failed" };
          }
          if (outcome.status === "succeeded") {
            results[taskIndex] = {
              taskIndex,
              role: task.role,
              status: "succeeded",
              value: outcome.value,
            };
          } else {
            const diagnostic: DelegationDiagnostic = {
              kind: "delegation",
              code: outcome.status === "failed"
                ? isDelegationLeafFailureCode(outcome.code)
                  ? outcome.code
                  : "task-failed"
                : "cancelled",
              taskIndex,
              role: task.role,
              ...(outcome.status === "cancelled"
                ? { phase: "running" as const }
                : {}),
            };
            emit(diagnostic);
            results[taskIndex] = {
              taskIndex,
              role: task.role,
              status: outcome.status,
              diagnostic,
            };
            if (contract.requirement === "required") {
              requiredTaskFailed ||= outcome.status === "failed";
              abortRunning();
            }
          }
        } finally {
          controllers.delete(taskIndex);
        }
      }
    },
  );
  await Promise.all(executionSlots);
  options.signal?.removeEventListener("abort", abortRunning);

  if (stopped) {
    for (const [taskIndex, task] of contract.tasks.entries()) {
      if (results[taskIndex]) {
        continue;
      }
      const diagnostic: DelegationDiagnostic = {
        kind: "delegation",
        code: "cancelled",
        taskIndex,
        role: task.role,
        phase: "before-spawn",
      };
      emit(diagnostic);
      results[taskIndex] = {
        taskIndex,
        role: task.role,
        status: "cancelled",
        diagnostic,
      };
    }
  }

  if (requiredTaskFailed) {
    emit({ kind: "delegation", code: "partial-failure" });
  }

  const completed = results.filter(
    (result): result is DelegationTaskResult<T> => result !== undefined,
  );
  return {
    status: requiredTaskFailed
      ? "failed"
      : completed.every((result) => result.status === "succeeded")
        ? "succeeded"
        : completed.some((result) => result.status === "cancelled") &&
            contract.requirement === "required"
          ? "cancelled"
          : "partial",
    tasks: completed,
    diagnostics,
  };
}
