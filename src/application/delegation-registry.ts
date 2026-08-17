import { randomUUID } from "node:crypto";

import type { MattyRole } from "../domain/capability-contract.ts";
import {
  safeReviewerValidationDiagnostic,
  type ReviewerValidationDiagnostic,
} from "../domain/review-scope.ts";
import {
  safeChildExecutionActivityObservation,
  type ChildExecutionActivityObservation,
} from "../domain/child-execution-activity.ts";
import type {
  DelegationDiagnostic,
  DelegationDiagnosticCode,
  DelegationPreflightReason,
} from "./delegation-scheduler.ts";

export const DELEGATION_STATES = [
  "queued", "running", "waiting-for-input", "waiting-for-capability", "cancelling", "blocked", "succeeded", "partial",
  "failed", "cancelled",
] as const;
export type DelegationState = (typeof DELEGATION_STATES)[number];
declare const delegationIdBrand: unique symbol;
/** Opaque session-scoped identity. Its JSON/runtime representation remains a string. */
export type DelegationId = string & { readonly [delegationIdBrand]: "DelegationId" };
declare const delegatedTaskIdBrand: unique symbol;
/** Opaque identity for one Delegated Task. It never aliases PID or runId. */
export type DelegatedTaskId = string & {
  readonly [delegatedTaskIdBrand]: "DelegatedTaskId";
};
export type TerminalDelegationState = Extract<
  DelegationState,
  "blocked" | "succeeded" | "partial" | "failed" | "cancelled"
>;

export interface SafeDelegationTaskDeclaration {
  role?: MattyRole;
}

export interface DelegationDeclaration {
  requirement?: "required" | "optional";
  tasks: readonly SafeDelegationTaskDeclaration[];
  maxActive?: number;
}

export interface RedactedDelegationDiagnostic {
  code: DelegationDiagnosticCode;
  taskIndex?: number;
  role?: MattyRole;
  phase?: "before-spawn" | "running";
  reason?: DelegationPreflightReason;
  validation?: ReviewerValidationDiagnostic;
}

export type DelegatedTaskState = Exclude<DelegationState, "partial">;

export interface DelegatedActivitySnapshot {
  taskIndex: number;
  observation: ChildExecutionActivityObservation;
}

export interface DelegatedTaskSnapshot {
  id: DelegatedTaskId;
  displayId: string;
  index: number;
  /** Immutable lineage for a Continuation; absent on original tasks. */
  sourceTaskId?: DelegatedTaskId;
  sourceDelegationId?: DelegationId;
  role?: MattyRole;
  state: DelegatedTaskState;
  queuePosition?: number;
  queuedAt: number;
  startedAt?: number;
  endedAt?: number;
  pid?: number;
  runId?: string;
  diagnostic?: RedactedDelegationDiagnostic;
  resultSummary?: string;
  activities: readonly ChildExecutionActivityObservation[];
}

export interface DelegationSnapshotEntry {
  id: DelegationId;
  displayId: string;
  requirement?: "required" | "optional";
  roles: readonly MattyRole[];
  taskCount: number;
  state: DelegationState;
  acceptedAt: number;
  startedAt?: number;
  endedAt?: number;
  diagnostics: readonly RedactedDelegationDiagnostic[];
  resultSummary?: string;
  activities: readonly DelegatedActivitySnapshot[];
  tasks: readonly DelegatedTaskSnapshot[];
}

export interface DelegationSnapshot {
  schemaVersion: 1;
  delegations: readonly DelegationSnapshotEntry[];
  concurrency: { activeTasks: number; queuedTasks: number };
}

export type DelegationLifecycleEvent =
  | { type: "queued"; taskIndex: number }
  | { type: "started"; taskIndex: number; pid: number }
  | { type: "identified"; taskIndex: number; pid: number; runId: string }
  | { type: "terminating" | "killing"; taskIndex: number; pid: number; runId: string };

export interface DelegationRegistryOptions {
  now?: () => number;
  idFactory?: () => string;
  taskIdFactory?: () => string;
  terminalLimit?: number;
  activityLimitPerTask?: number;
}

export type DelegatedTaskTerminalState = Extract<
  TerminalDelegationState,
  "blocked" | "succeeded" | "failed" | "cancelled"
>;
export type DelegatedTaskCompletionState = Extract<
  DelegatedTaskTerminalState,
  "succeeded" | "failed" | "cancelled"
>;
export type DelegationCancellationResult =
  | "cancelling"
  | "already-cancelling"
  | "already-finished";

interface StoredDelegation extends DelegationSnapshotEntry {
  diagnostics: RedactedDelegationDiagnostic[];
  tasks: DelegatedTaskSnapshot[];
  terminalOrder?: number;
  controller?: AbortController;
}

const terminalStates = new Set<DelegationState>([
  "blocked", "succeeded", "partial", "failed", "cancelled",
]);
const diagnosticCodes = new Set<DelegationDiagnosticCode>([
  "queued", "preflight-failed", "skipped", "task-failed", "cancelled",
  "partial-failure", "child-failed", "protocol-failed", "child-exited",
  "missing-research-report", "invalid-role-output",
]);
const preflightReasons = new Set<DelegationPreflightReason>([
  "authentication-unavailable", "runtime-unavailable", "rules-conflict",
  "github-unavailable", "review-commit-unavailable", "web-unavailable", "writer-unavailable",
  "artifact-destination-invalid", "tool-surface-incompatible",
  "child-session-store-unavailable", "capability-unavailable",
]);

function title(state: DelegationState): string {
  return `${state.slice(0, 1).toUpperCase()}${state.slice(1)}`;
}

function diagnosticSuffix(diagnostic?: RedactedDelegationDiagnostic): string {
  if (!diagnostic) return "";
  const details = [diagnostic.code, diagnostic.reason, diagnostic.phase].filter(Boolean);
  return details.length > 0 ? ` (${details.join(" · ")})` : "";
}

export function isTerminalDelegationState(
  state: DelegationState,
): state is TerminalDelegationState {
  return terminalStates.has(state);
}

function shortCandidate(prefix: "D" | "T", id: string): string {
  const hex = id.replaceAll("-", "").toLowerCase();
  return `${prefix}-${hex.slice(0, 8).padEnd(8, "0")}`;
}

export class DelegationRegistry {
  readonly #now: () => number;
  readonly #idFactory: () => string;
  readonly #taskIdFactory: () => string;
  readonly #terminalLimit: number;
  readonly #activityLimitPerTask: number;
  readonly #entries = new Map<DelegationId, StoredDelegation>();
  readonly #listeners = new Set<() => void>();
  #terminalOrder = 0;

  constructor(options: DelegationRegistryOptions = {}) {
    this.#now = options.now ?? Date.now;
    this.#idFactory = options.idFactory ?? randomUUID;
    this.#taskIdFactory = options.taskIdFactory ?? randomUUID;
    this.#terminalLimit = Math.max(0, Math.trunc(options.terminalLimit ?? 50));
    this.#activityLimitPerTask = Math.max(1, Math.trunc(options.activityLimitPerTask ?? 100));
  }

  accept(
    declaration: DelegationDeclaration,
    controller?: AbortController,
    lineage?: { sourceTaskId: DelegatedTaskId; sourceDelegationId: DelegationId },
  ): DelegationSnapshotEntry {
    let id = this.#nextId();
    while (this.#entries.has(id)) id = this.#nextId();
    let displayId = shortCandidate("D", id);
    while ([...this.#entries.values()].some((entry) => entry.displayId === displayId)) {
      id = this.#nextId();
      if (this.#entries.has(id)) continue;
      displayId = shortCandidate("D", id);
    }
    const acceptedAt = this.#now();
    const declarations = declaration.tasks.map((task) => ({ ...task }));
    const taskIdentities: Array<{ id: DelegatedTaskId; displayId: string }> = [];
    for (const _declaration of declarations) {
      taskIdentities.push(this.#nextTaskIdentity(id, taskIdentities));
    }
    const roles = declarations.flatMap((task) => task.role ? [task.role] : []);
    const taskCount = declarations.length;
    const maxActive = Math.max(1, Math.trunc(declaration.maxActive ?? 4));
    const entry: StoredDelegation = {
      id,
      displayId,
      ...(declaration.requirement ? { requirement: declaration.requirement } : {}),
      roles,
      taskCount,
      state: "queued",
      acceptedAt,
      diagnostics: [],
      activities: [],
      tasks: declarations.map((declaration, index) => ({
        ...taskIdentities[index]!,
        index,
        ...(index === 0 && lineage ? lineage : {}),
        ...(declaration.role ? { role: declaration.role } : {}),
        state: "queued" as const,
        ...(index >= maxActive ? { queuePosition: index - maxActive + 1 } : {}),
        queuedAt: acceptedAt,
        activities: [],
      })),
      ...(controller ? { controller } : {}),
    };
    this.#entries.set(id, entry);
    this.#changed();
    return this.get(id)!;
  }

  acceptContinuation(
    sourceTaskId: DelegatedTaskId,
    declaration: DelegationDeclaration,
    controller?: AbortController,
  ): DelegationSnapshotEntry | undefined {
    const source = [...this.#entries.values()].find((entry) =>
      entry.tasks.some((task) => task.id === sourceTaskId)
    );
    const sourceTask = source?.tasks.find((task) => task.id === sourceTaskId);
    if (!source || !sourceTask || !isTerminalDelegationState(source.state) ||
        !isTerminalDelegationState(sourceTask.state)) return undefined;
    return this.accept(declaration, controller, {
      sourceTaskId,
      sourceDelegationId: source.id,
    });
  }

  get(id: DelegationId): DelegationSnapshotEntry | undefined {
    const entry = this.#entries.get(id);
    return entry ? this.#copy(entry) : undefined;
  }

  now(): number {
    return this.#now();
  }

  record(id: DelegationId, event: DelegationLifecycleEvent): void {
    const entry = this.#entries.get(id);
    if (!entry || isTerminalDelegationState(entry.state)) return;
    const task = entry.tasks[event.taskIndex];
    if (!task || isTerminalDelegationState(task.state)) return;
    const now = this.#now();
    if (event.type === "queued") {
      task.state = "queued";
    } else if (event.type === "started") {
      task.state = "running";
      delete task.queuePosition;
      this.#normalizeQueue(entry);
      task.startedAt ??= now;
      task.pid = event.pid;
      entry.startedAt ??= now;
    } else if (event.type === "identified") {
      task.state = "running";
      delete task.queuePosition;
      this.#normalizeQueue(entry);
      task.startedAt ??= now;
      task.pid = event.pid;
      task.runId = event.runId;
      entry.startedAt ??= now;
    } else {
      task.state = "cancelling";
      task.startedAt ??= now;
      task.pid = event.pid;
      task.runId = event.runId;
      entry.startedAt ??= now;
    }
    this.#recalculateState(entry);
    this.#changed();
  }

  recordTaskSessionState(
    taskId: string,
    state: "working" | "settled" | "waiting-for-input" | "waiting-for-capability",
  ): void {
    for (const entry of this.#entries.values()) {
      const task = entry.tasks.find((candidate) => candidate.id === taskId);
      if (!task || isTerminalDelegationState(task.state)) continue;
      task.state = state === "working" || state === "settled" ? "running" : state;
      this.#recalculateState(entry);
      this.#changed();
      return;
    }
  }

  recordActivity(id: DelegationId, taskIndex: number, value: unknown): void {
    const entry = this.#entries.get(id);
    if (!entry || isTerminalDelegationState(entry.state)) return;
    const task = entry.tasks[taskIndex];
    if (!task || isTerminalDelegationState(task.state)) return;
    const observation = safeChildExecutionActivityObservation(value);
    if (!observation) return;
    task.activities = [...task.activities, observation].slice(-this.#activityLimitPerTask);
    const counts = new Map<number, number>();
    entry.activities = [...entry.activities, { taskIndex, observation }]
      .reverse()
      .filter((activity) => {
        const count = counts.get(activity.taskIndex) ?? 0;
        counts.set(activity.taskIndex, count + 1);
        return count < this.#activityLimitPerTask;
      })
      .reverse();
    this.#changed();
  }

  finishTask(
    id: DelegationId,
    taskIndex: number,
    state: DelegatedTaskCompletionState,
  ): void {
    const entry = this.#entries.get(id);
    if (!entry || isTerminalDelegationState(entry.state)) return;
    const task = entry.tasks[taskIndex];
    if (!task || isTerminalDelegationState(task.state)) return;
    task.state = state;
    delete task.queuePosition;
    task.endedAt = this.#now();
    task.resultSummary = `${title(state)}${diagnosticSuffix(task.diagnostic)}`;
    this.#normalizeQueue(entry);
    this.#recalculateState(entry);
    this.#changed();
  }

  recordDiagnostic(id: DelegationId, diagnostic: DelegationDiagnostic): void {
    const entry = this.#entries.get(id);
    if (!entry || isTerminalDelegationState(entry.state) ||
      !diagnosticCodes.has(diagnostic.code)) return;
    const validation = diagnostic.code === "invalid-role-output" && diagnostic.validation
      ? safeReviewerValidationDiagnostic(diagnostic.validation)
      : undefined;
    const safe: RedactedDelegationDiagnostic = {
      code: diagnostic.code,
      ...(typeof diagnostic.taskIndex === "number" &&
          Number.isInteger(diagnostic.taskIndex) && diagnostic.taskIndex >= 0
        ? { taskIndex: diagnostic.taskIndex }
        : {}),
      ...(diagnostic.role && entry.roles.includes(diagnostic.role)
        ? { role: diagnostic.role }
        : {}),
      ...(diagnostic.phase === "before-spawn" || diagnostic.phase === "running"
        ? { phase: diagnostic.phase }
        : {}),
      ...(diagnostic.reason && preflightReasons.has(diagnostic.reason)
        ? { reason: diagnostic.reason }
        : {}),
      ...(validation ? { validation } : {}),
    };
    entry.diagnostics.push(safe);
    if (safe.taskIndex !== undefined) {
      const task = entry.tasks[safe.taskIndex];
      if (task && safe.code !== "queued") {
        task.diagnostic = safe;
        if (isTerminalDelegationState(task.state)) {
          task.resultSummary = `${title(task.state)}${diagnosticSuffix(safe)}`;
        }
      }
    }
    this.#changed();
  }

  finish(
    id: DelegationId,
    state: TerminalDelegationState,
    taskStates: ReadonlyMap<number, DelegatedTaskTerminalState> = new Map(),
  ): DelegationSnapshotEntry | undefined {
    const entry = this.#entries.get(id);
    if (!entry || isTerminalDelegationState(entry.state)) return entry && this.#copy(entry);
    const now = this.#now();
    entry.state = state;
    entry.endedAt = now;
    entry.terminalOrder = ++this.#terminalOrder;
    delete entry.controller;
    const delegationDiagnostic = entry.diagnostics.find((diagnostic) =>
      diagnostic.reason !== undefined
    ) ?? entry.diagnostics.find((diagnostic) => diagnostic.taskIndex === undefined);
    entry.resultSummary = state === "blocked" && delegationDiagnostic?.reason
      ? `${title(state)} (${delegationDiagnostic.reason})`
      : `${title(state)}${diagnosticSuffix(delegationDiagnostic)}`;
    for (const task of entry.tasks) {
      if (isTerminalDelegationState(task.state)) continue;
      task.state = taskStates.get(task.index) ??
        (state === "partial" ? "failed" : state);
      delete task.queuePosition;
      task.endedAt = now;
      task.resultSummary = `${title(task.state)}${diagnosticSuffix(task.diagnostic)}`;
    }
    this.#evict();
    this.#changed();
    return this.#copy(entry);
  }

  cancel(id: DelegationId): DelegationCancellationResult {
    const entry = this.#entries.get(id);
    if (!entry || isTerminalDelegationState(entry.state)) return "already-finished";
    if (entry.state === "cancelling") return "already-cancelling";
    entry.state = "cancelling";
    for (const task of entry.tasks) {
      if (task.state === "running" || task.state === "waiting-for-input" || task.state === "waiting-for-capability") task.state = "cancelling";
    }
    this.#changed();
    entry.controller?.abort();
    return "cancelling";
  }

  snapshot(): DelegationSnapshot {
    const entries = [...this.#entries.values()].sort((left, right) => {
      const rank = (entry: StoredDelegation) =>
        entry.state === "running" || entry.state === "cancelling" ? 0
          : entry.state === "queued" ? 1 : 2;
      const difference = rank(left) - rank(right);
      if (difference) return difference;
      if (rank(left) === 2) return (right.terminalOrder ?? 0) - (left.terminalOrder ?? 0);
      return left.acceptedAt - right.acceptedAt || left.id.localeCompare(right.id);
    });
    let activeTasks = 0;
    let queuedTasks = 0;
    for (const entry of entries) {
      for (const task of entry.tasks) {
        if (task.state === "running" || task.state === "waiting-for-input" || task.state === "waiting-for-capability" || task.state === "cancelling") activeTasks += 1;
        if (task.state === "queued") queuedTasks += 1;
      }
    }
    return {
      schemaVersion: 1,
      delegations: entries.map((entry) => this.#copy(entry)),
      concurrency: { activeTasks, queuedTasks },
    };
  }

  restore(sessions: ReadonlyArray<{
    taskId: string;
    delegationId: string;
    sourceTaskId?: string;
    sourceDelegationId?: string;
    taskIndex: number;
    role: MattyRole;
    requirement: "required" | "optional";
    state: "active" | "succeeded" | "failed" | "cancelled" | "interrupted";
    createdAt: number;
    updatedAt: number;
  }>): void {
    const groups = new Map<string, typeof sessions>();
    for (const session of sessions) groups.set(session.delegationId, [...(groups.get(session.delegationId) ?? []), session]);
    for (const [rawDelegationId, group] of groups) {
      const id = rawDelegationId as DelegationId;
      if (this.#entries.has(id)) continue;
      const ordered = [...group].sort((left, right) => left.taskIndex - right.taskIndex);
      const taskStates = ordered.map((item) => item.state === "succeeded" ? "succeeded" as const : item.state === "cancelled" ? "cancelled" as const : "failed" as const);
      const state: TerminalDelegationState = taskStates.every((item) => item === "succeeded") ? "succeeded"
        : taskStates.every((item) => item === "cancelled") ? "cancelled" : "failed";
      const acceptedAt = Math.min(...ordered.map((item) => item.createdAt));
      const endedAt = Math.max(...ordered.map((item) => item.updatedAt));
      const roles = [...new Set(ordered.map((item) => item.role))];
      const entry: StoredDelegation = {
        id,
        displayId: shortCandidate("D", id),
        requirement: ordered[0]!.requirement,
        roles,
        taskCount: ordered.length,
        state,
        acceptedAt,
        startedAt: acceptedAt,
        endedAt,
        diagnostics: [],
        resultSummary: ordered.some((item) => item.state === "interrupted") ? "Failed (interrupted)" : title(state),
        activities: [],
        tasks: ordered.map((item, index) => ({
          id: item.taskId as DelegatedTaskId,
          displayId: shortCandidate("T", item.taskId),
          index,
          ...(item.sourceTaskId && item.sourceDelegationId ? {
            sourceTaskId: item.sourceTaskId as DelegatedTaskId,
            sourceDelegationId: item.sourceDelegationId as DelegationId,
          } : {}),
          role: item.role,
          state: taskStates[index]!,
          queuedAt: item.createdAt,
          startedAt: item.createdAt,
          endedAt: item.updatedAt,
          resultSummary: item.state === "interrupted" ? "Failed (interrupted)" : title(taskStates[index]!),
          activities: [],
        })),
        terminalOrder: ++this.#terminalOrder,
      };
      this.#entries.set(id, entry);
    }
    this.#evict();
    this.#changed();
  }

  subscribe(listener: () => void): () => void {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }

  reset(): void {
    for (const entry of this.#entries.values()) entry.controller?.abort();
    this.#entries.clear();
    this.#terminalOrder = 0;
    this.#changed();
  }

  shutdown(): void {
    this.reset();
    this.#listeners.clear();
  }

  #nextId(): DelegationId {
    return this.#idFactory() as DelegationId;
  }

  #nextTaskIdentity(
    delegationId: DelegationId,
    pending: readonly { id: DelegatedTaskId; displayId: string }[],
  ): { id: DelegatedTaskId; displayId: string } {
    const retainedTasks = [...this.#entries.values()].flatMap((entry) => entry.tasks);
    while (true) {
      const id = this.#taskIdFactory() as DelegatedTaskId;
      const displayId = shortCandidate("T", id);
      const collides = [...retainedTasks, ...pending].some((task) =>
        task.id === id || task.displayId === displayId
      );
      const aliasesDelegation = id === (delegationId as unknown as DelegatedTaskId) ||
        this.#entries.has(id as unknown as DelegationId);
      if (!collides && !aliasesDelegation) return { id, displayId };
    }
  }

  #copy(entry: StoredDelegation): DelegationSnapshotEntry {
    return {
      id: entry.id,
      displayId: entry.displayId,
      ...(entry.requirement ? { requirement: entry.requirement } : {}),
      roles: [...entry.roles],
      taskCount: entry.taskCount,
      state: entry.state,
      acceptedAt: entry.acceptedAt,
      ...(entry.startedAt !== undefined ? { startedAt: entry.startedAt } : {}),
      ...(entry.endedAt !== undefined ? { endedAt: entry.endedAt } : {}),
      diagnostics: entry.diagnostics.map((diagnostic) => ({ ...diagnostic })),
      ...(entry.resultSummary ? { resultSummary: entry.resultSummary } : {}),
      activities: entry.activities.map((activity) => ({
        taskIndex: activity.taskIndex,
        observation: {
          ...activity.observation,
          summary: { ...activity.observation.summary },
        },
      })),
      tasks: entry.tasks.map((task) => ({
        ...task,
        activities: task.activities.map((activity) => ({
          ...activity,
          summary: { ...activity.summary },
        })),
        ...(task.diagnostic ? { diagnostic: { ...task.diagnostic } } : {}),
      })),
    };
  }

  #recalculateState(entry: StoredDelegation): void {
    if (entry.state === "cancelling") return;
    if (entry.tasks.some((task) => task.state === "cancelling")) {
      entry.state = "cancelling";
    } else if (entry.tasks.some((task) => task.state === "running" || task.state === "waiting-for-input" || task.state === "waiting-for-capability")) {
      entry.state = "running";
    } else if (entry.tasks.some((task) => task.state === "queued")) {
      entry.state = "queued";
    }
  }

  #normalizeQueue(entry: StoredDelegation): void {
    const queued = entry.tasks
      .filter((task) => task.state === "queued" && task.queuePosition !== undefined)
      .sort((left, right) =>
        (left.queuePosition ?? 0) - (right.queuePosition ?? 0) || left.index - right.index
      );
    queued.forEach((task, index) => {
      task.queuePosition = index + 1;
    });
  }

  #evict(): void {
    const terminals = [...this.#entries.values()]
      .filter((entry) => isTerminalDelegationState(entry.state))
      .sort((left, right) => (right.terminalOrder ?? 0) - (left.terminalOrder ?? 0));
    for (const entry of terminals.slice(this.#terminalLimit)) this.#entries.delete(entry.id);
  }

  #changed(): void {
    for (const listener of this.#listeners) listener();
  }
}
