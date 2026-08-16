import { isMattyRole } from "../domain/capability-contract.ts";
import type { DelegationDiagnostic, DelegationPreflightReason } from "./delegation-scheduler.ts";
import {
  DelegationRegistry,
  type DelegatedTaskCompletionState,
  type DelegatedTaskTerminalState,
  type DelegationDeclaration,
  type DelegationSnapshotEntry,
  type TerminalDelegationState,
} from "./delegation-registry.ts";
import { delegationCard } from "./delegation-presentation.ts";

export interface DelegationObserverUpdate {
  content: Array<{ type: "text"; text: string }>;
  details: {
    delegation: DelegationSnapshotEntry;
    taskIndex: number;
    code?: "queued";
    type?: "started" | "identified" | "terminating" | "killing";
    progress?: { type: "started" | "identified" | "terminating" | "killing" };
  };
}

export interface DelegationObserver {
  readonly id: string;
  readonly signal: AbortSignal;
  observeProgress(details: unknown): void;
  completeTask(taskIndex: number, status: DelegatedTaskCompletionState): void;
  recordDiagnostic(diagnostic: DelegationDiagnostic): void;
  finish(details: unknown): {
    entry: DelegationSnapshotEntry | undefined;
    safeDetails: unknown;
  };
  fail(): DelegationSnapshotEntry | undefined;
}

interface DelegationObserverOptions {
  registry: DelegationRegistry;
  declaration: unknown;
  signal?: AbortSignal;
  onUpdate?: (update: DelegationObserverUpdate) => void;
}

function record(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null
    ? value as Record<string, unknown>
    : undefined;
}

function safeDeclaration(value: unknown): DelegationDeclaration {
  const candidate = record(value) ?? {};
  const declared = Array.isArray(candidate.tasks) ? candidate.tasks : [candidate];
  return {
    ...(candidate.requirement === "required" || candidate.requirement === "optional"
      ? { requirement: candidate.requirement }
      : {}),
    tasks: declared.map((task) => {
      const role = record(task)?.role;
      return isMattyRole(role) ? { role } : {};
    }),
    maxActive: 4,
  };
}

function terminalState(details: unknown): TerminalDelegationState {
  const direct = record(details);
  const outcome = record(direct?.outcome);
  const status = direct?.status ?? outcome?.status;
  return status === "succeeded" || status === "partial" || status === "failed" ||
      status === "cancelled" || status === "blocked"
    ? status
    : "failed";
}

function taskTerminalStates(details: unknown): Map<number, DelegatedTaskTerminalState> {
  const states = new Map<number, DelegatedTaskTerminalState>();
  const tasks = record(details)?.tasks;
  if (!Array.isArray(tasks)) return states;
  for (const value of tasks) {
    const task = record(value);
    if (!task || typeof task.taskIndex !== "number") continue;
    const state: DelegatedTaskTerminalState | undefined =
      task.status === "succeeded" ? "succeeded"
        : task.status === "failed" ? "failed"
          : task.status === "cancelled" ? "cancelled"
            : task.status === "skipped" ? "blocked"
              : undefined;
    if (state) states.set(task.taskIndex, state);
  }
  return states;
}

function classifyPreflightReason(unmet: unknown): DelegationPreflightReason {
  const values = Array.isArray(unmet)
    ? unmet.filter((value): value is string => typeof value === "string")
    : [];
  const matches = (pattern: RegExp) => values.some((value) => pattern.test(value));
  if (matches(/review-commit-unavailable/i)) return "review-commit-unavailable";
  if (matches(/GitHub CLI|github-unavailable/i)) return "github-unavailable";
  if (matches(/authentication is unavailable|authentication-unavailable/i)) {
    return "authentication-unavailable";
  }
  if (matches(/Subagent Runtime|runtime-unavailable/i)) return "runtime-unavailable";
  if (matches(/Matty Rules conflict|rules-conflict/i)) return "rules-conflict";
  if (matches(/Single Writer|writer-unavailable/i)) return "writer-unavailable";
  if (matches(/web.+unavailable|web-unavailable/i)) return "web-unavailable";
  if (matches(/path scope|artifact destination|Review Scope/i)) {
    return "artifact-destination-invalid";
  }
  if (matches(/tool|guard|tool-surface-incompatible/i)) return "tool-surface-incompatible";
  return "capability-unavailable";
}

function standalonePreflightDiagnostic(
  details: unknown,
  role: unknown,
): DelegationDiagnostic | undefined {
  const outcome = record(record(details)?.outcome);
  const diagnostic = record(outcome?.diagnostic);
  if (outcome?.status !== "blocked" || diagnostic?.kind !== "capability-preflight") {
    return undefined;
  }
  return {
    kind: "delegation",
    code: "preflight-failed",
    taskIndex: 0,
    ...(isMattyRole(role) ? { role } : {}),
    reason: classifyPreflightReason(diagnostic.unmet),
  };
}

function safeTerminalDetails(
  details: unknown,
  diagnostic: DelegationDiagnostic | undefined,
): unknown {
  if (!diagnostic?.reason) return details;
  const source = record(details);
  const outcome = record(source?.outcome);
  const rawDiagnostic = record(outcome?.diagnostic);
  if (!source || !outcome || !rawDiagnostic) return details;
  return {
    ...source,
    outcome: {
      ...outcome,
      diagnostic: {
        kind: "capability-preflight",
        ...(typeof rawDiagnostic.contractId === "string"
          ? { contractId: rawDiagnostic.contractId }
          : {}),
        reason: diagnostic.reason,
      },
    },
  };
}

export function createDelegationObserver(
  options: DelegationObserverOptions,
): DelegationObserver {
  const controller = new AbortController();
  if (options.signal?.aborted) controller.abort();
  else options.signal?.addEventListener("abort", () => controller.abort(), { once: true });
  const declaration = safeDeclaration(options.declaration);
  const accepted = options.registry.accept(declaration, controller);
  const declaredRole = declaration.tasks[0]?.role;

  const emit = (
    taskIndex: number,
    lifecycle?: "started" | "identified" | "terminating" | "killing",
    queued = false,
    nested = false,
  ) => {
    const entry = options.registry.get(accepted.id);
    if (!entry) return;
    options.onUpdate?.({
      content: [{ type: "text", text: delegationCard(entry, options.registry.now()) }],
      details: {
        delegation: entry,
        taskIndex,
        ...(queued ? { code: "queued" as const } : {}),
        ...(lifecycle ? { type: lifecycle } : {}),
        ...(lifecycle && nested ? { progress: { type: lifecycle } } : {}),
      },
    });
  };

  return {
    id: accepted.id,
    signal: controller.signal,
    observeProgress(value) {
      const details = record(value) ?? {};
      const taskIndex = typeof details.taskIndex === "number" ? details.taskIndex : 0;
      const nested = record(details.progress);
      const progress = nested ?? details;
      if (details.code === "queued") {
        options.registry.record(accepted.id, { type: "queued", taskIndex });
        emit(taskIndex, undefined, true);
        return;
      }
      const child = record(progress.child);
      const type = progress.type;
      if (type === "started" && typeof child?.pid === "number") {
        options.registry.record(accepted.id, { type, taskIndex, pid: child.pid });
        emit(taskIndex, type, false, nested !== undefined);
      } else if (
        (type === "identified" || type === "terminating" || type === "killing") &&
        typeof child?.pid === "number" && typeof child.runId === "string"
      ) {
        options.registry.record(accepted.id, {
          type,
          taskIndex,
          pid: child.pid,
          runId: child.runId,
        });
        emit(taskIndex, type, false, nested !== undefined);
      }
    },
    completeTask(taskIndex, status) {
      options.registry.finishTask(accepted.id, taskIndex, status);
    },
    recordDiagnostic(diagnostic) {
      options.registry.recordDiagnostic(accepted.id, diagnostic);
      const taskIndex = typeof diagnostic.taskIndex === "number" ? diagnostic.taskIndex : 0;
      emit(taskIndex, undefined, diagnostic.code === "queued");
    },
    finish(details) {
      const diagnostic = standalonePreflightDiagnostic(details, declaredRole);
      if (diagnostic) options.registry.recordDiagnostic(accepted.id, diagnostic);
      return {
        entry: options.registry.finish(
          accepted.id,
          terminalState(details),
          taskTerminalStates(details),
        ),
        safeDetails: safeTerminalDetails(details, diagnostic),
      };
    },
    fail() {
      return options.registry.finish(accepted.id, "failed");
    },
  };
}
