import type {
  ChildInteraction,
  ChildInteractionResult,
  ChildInputResponse,
  ChildInputResult,
  DelegatedTaskPresentation,
  DelegatedTaskRunner,
} from "./child-pi-runtime.ts";

export type DelegationControlResult =
  | ChildInteractionResult
  | { status: "rejected"; code: "delegated-task-unavailable" | "delegation-closing" };

export interface MattyApplicationControl {
  interact(delegatedTaskId: string, interaction: ChildInteraction): Promise<DelegationControlResult>;
  abortTask(delegatedTaskId: string): { status: "accepted" } | {
    status: "rejected";
    code: "delegated-task-unavailable" | "delegation-closing";
  };
  respondToInput(delegatedTaskId: string, requestId: string, response: ChildInputResponse): Promise<ChildInputResult | { status: "rejected"; code: "delegated-task-unavailable" | "delegation-closing" }>;
  extendInputTimeout(delegatedTaskId: string, requestId: string, extensionMs?: number): ChildInputResult | { status: "rejected"; code: "delegated-task-unavailable" | "delegation-closing" };
  /** Private, task-scoped presentation state. Raw Child Session content is never returned elsewhere. */
  taskPresentation(delegatedTaskId: string): DelegatedTaskPresentation | undefined;
  subscribeTaskPresentation(
    delegatedTaskId: string,
    listener: (presentation: DelegatedTaskPresentation) => void,
  ): () => void;
  /** Holds an active Child Session while its task view remains open. */
  retainTaskSession(delegatedTaskId: string): () => void;
  freeze(delegationId: string): Promise<unknown>;
}

interface DelegationRecord {
  phase: "open" | "closing" | "closed";
  terminal: Promise<unknown>;
  resolveTerminal(value: unknown): void;
  frozen?: unknown;
  hasFrozen?: boolean;
  terminalOrder?: number;
  abortGroup?: () => void;
  taskIds: string[];
}

interface TaskRecord {
  delegationId: string;
  requirement: "required" | "optional";
  runner?: DelegatedTaskRunner;
  detachRunner?: () => void;
  abort?: () => void;
  abortRequested?: boolean;
  presentation?: DelegatedTaskPresentation;
  listeners: Set<(presentation: DelegatedTaskPresentation) => void>;
  holders: Map<symbol, (() => void) | undefined>;
  closeWhenReleased?: boolean;
}

function immutable<T>(value: T, visited = new WeakSet<object>()): T {
  if (typeof value !== "object" || value === null || visited.has(value)) return value;
  visited.add(value);
  for (const child of Object.values(value as Record<string, unknown>)) immutable(child, visited);
  return Object.freeze(value);
}

export class DelegationControl implements MattyApplicationControl {
  readonly #terminalLimit: number;
  readonly #onTaskSessionState?: (taskId: string, state: DelegatedTaskPresentation["sessionState"]) => void;
  readonly #delegations = new Map<string, DelegationRecord>();
  readonly #tasks = new Map<string, TaskRecord>();
  #terminalOrder = 0;

  constructor(options: {
    terminalLimit?: number;
    onTaskSessionState?: (taskId: string, state: DelegatedTaskPresentation["sessionState"]) => void;
  } = {}) {
    this.#terminalLimit = Math.max(0, Math.trunc(options.terminalLimit ?? 50));
    if (options.onTaskSessionState) this.#onTaskSessionState = options.onTaskSessionState;
  }

  open(
    delegationId: string,
    requirement: "required" | "optional",
    tasks: readonly string[],
    abortGroup: () => void,
  ): void {
    let resolveTerminal!: (value: unknown) => void;
    const terminal = new Promise<unknown>((resolve) => { resolveTerminal = resolve; });
    this.#delegations.set(delegationId, {
      phase: "open",
      terminal,
      resolveTerminal,
      abortGroup,
      taskIds: [...tasks],
    });
    for (const taskId of tasks) {
      this.#tasks.set(taskId, {
        delegationId,
        requirement,
        listeners: new Set(),
        holders: new Map(),
      });
    }
  }

  restore(sessions: ReadonlyArray<{ taskId: string; delegationId: string; requirement: "required" | "optional"; presentation: DelegatedTaskPresentation }>): void {
    const groups = new Map<string, typeof sessions>();
    for (const session of sessions) groups.set(session.delegationId, [...(groups.get(session.delegationId) ?? []), session]);
    for (const [delegationId, group] of groups) {
      const terminal = Promise.resolve(Object.freeze({ status: "restored" as const }));
      this.#delegations.set(delegationId, {
        phase: "closed",
        terminal,
        resolveTerminal() {},
        frozen: Object.freeze({ status: "restored" as const }),
        hasFrozen: true,
        terminalOrder: ++this.#terminalOrder,
        taskIds: group.map((item) => item.taskId),
      });
      for (const item of group) this.#tasks.set(item.taskId, {
        delegationId,
        requirement: item.requirement,
        presentation: item.presentation,
        listeners: new Set(),
        holders: new Map(),
      });
    }
    this.#evict();
  }

  attachRunner(taskId: string, runner: DelegatedTaskRunner): void {
    const task = this.#tasks.get(taskId);
    if (!task) return;
    task.detachRunner?.();
    for (const release of task.holders.values()) release?.();
    task.runner = runner;
    for (const holder of task.holders.keys()) task.holders.set(holder, runner.retain?.());
    const update = (presentation: DelegatedTaskPresentation) => {
      task.presentation = presentation;
      this.#onTaskSessionState?.(taskId, presentation.sessionState);
      for (const listener of task.listeners) {
        try {
          listener(presentation);
        } catch {
          // Presentation observers cannot alter Child Session ingestion.
        }
      }
    };
    const initial = runner.presentation?.();
    if (initial) update(initial);
    const detachRunner = runner.subscribePresentation?.(update);
    if (detachRunner) task.detachRunner = detachRunner;
    else delete task.detachRunner;
  }

  attachAbort(taskId: string, abort: () => void): void {
    const task = this.#tasks.get(taskId);
    if (!task) return;
    task.abort = abort;
    if (task.abortRequested) abort();
  }

  async interact(
    taskId: string,
    interaction: ChildInteraction,
  ): Promise<DelegationControlResult> {
    const task = this.#tasks.get(taskId);
    if (!task) return { status: "rejected", code: "delegated-task-unavailable" };
    const delegation = this.#delegations.get(task.delegationId);
    if (!delegation || delegation.phase !== "open") {
      return { status: "rejected", code: "delegation-closing" };
    }
    if (!task.runner?.interact) {
      return { status: "rejected", code: "child-session-unavailable" };
    }
    // Once accepted here, this interaction drains even if the delegation starts closing.
    return await task.runner.interact(interaction);
  }

  async respondToInput(taskId: string, requestId: string, response: ChildInputResponse) {
    const task = this.#tasks.get(taskId);
    if (!task) return { status: "rejected" as const, code: "delegated-task-unavailable" as const };
    const delegation = this.#delegations.get(task.delegationId);
    if (!delegation || delegation.phase !== "open") {
      return { status: "rejected" as const, code: "delegation-closing" as const };
    }
    return task.runner?.respondToInput
      ? await task.runner.respondToInput(requestId, response)
      : { status: "rejected" as const, code: "child-session-unavailable" as const };
  }

  extendInputTimeout(taskId: string, requestId: string, extensionMs?: number) {
    const task = this.#tasks.get(taskId);
    if (!task) return { status: "rejected" as const, code: "delegated-task-unavailable" as const };
    const delegation = this.#delegations.get(task.delegationId);
    if (!delegation || delegation.phase !== "open") {
      return { status: "rejected" as const, code: "delegation-closing" as const };
    }
    return task.runner?.extendInputTimeout
      ? task.runner.extendInputTimeout(requestId, extensionMs)
      : { status: "rejected" as const, code: "child-session-unavailable" as const };
  }

  abortTask(taskId: string): ReturnType<MattyApplicationControl["abortTask"]> {
    const task = this.#tasks.get(taskId);
    if (!task) return { status: "rejected", code: "delegated-task-unavailable" };
    const delegation = this.#delegations.get(task.delegationId);
    if (!delegation || delegation.phase !== "open") {
      return { status: "rejected", code: "delegation-closing" };
    }
    if (task.requirement === "required") delegation.abortGroup?.();
    else if (task.abort) task.abort();
    else task.abortRequested = true;
    return { status: "accepted" };
  }

  taskPresentation(taskId: string): DelegatedTaskPresentation | undefined {
    return this.#tasks.get(taskId)?.presentation;
  }

  subscribeTaskPresentation(
    taskId: string,
    listener: (presentation: DelegatedTaskPresentation) => void,
  ): () => void {
    const task = this.#tasks.get(taskId);
    if (!task) return () => {};
    task.listeners.add(listener);
    if (task.presentation) listener(task.presentation);
    return () => task.listeners.delete(listener);
  }

  retainTaskSession(taskId: string): () => void {
    const task = this.#tasks.get(taskId);
    const delegation = task ? this.#delegations.get(task.delegationId) : undefined;
    if (!task || !delegation || delegation.phase !== "open") return () => {};
    const holder = Symbol("task-session-holder");
    task.holders.set(holder, task.runner?.retain?.());
    return () => {
      if (!task.holders.has(holder)) return;
      task.holders.get(holder)?.();
      task.holders.delete(holder);
      if (task.holders.size === 0 && task.closeWhenReleased) this.#closeTaskRunner(task);
    };
  }

  freeze(delegationId: string): Promise<unknown> {
    const delegation = this.#delegations.get(delegationId);
    if (!delegation) return Promise.reject(new Error("Delegation is unavailable"));
    if (delegation.phase === "open") {
      delegation.phase = "closing";
      for (const taskId of delegation.taskIds) this.#tasks.get(taskId)?.runner?.freeze?.();
    }
    return delegation.terminal;
  }

  complete(delegationId: string, result: unknown): unknown {
    const delegation = this.#delegations.get(delegationId);
    if (!delegation) return immutable(result);
    if (delegation.hasFrozen) return delegation.frozen;
    delegation.phase = "closing";
    delegation.frozen = immutable(result);
    delegation.hasFrozen = true;
    delegation.phase = "closed";
    delegation.terminalOrder = ++this.#terminalOrder;
    delete delegation.abortGroup;
    for (const taskId of delegation.taskIds) {
      const task = this.#tasks.get(taskId);
      if (!task) continue;
      const finalPresentation = task.runner?.presentation?.();
      if (finalPresentation) task.presentation = finalPresentation;
      if (task.holders.size > 0) task.closeWhenReleased = true;
      else this.#closeTaskRunner(task);
      delete task.abort;
      delete task.abortRequested;
    }
    delegation.resolveTerminal(delegation.frozen);
    this.#evict();
    return delegation.frozen;
  }

  reset(): void {
    const cancelled = immutable({ status: "cancelled" as const });
    for (const delegation of this.#delegations.values()) {
      delegation.abortGroup?.();
      if (delegation.phase !== "closed") delegation.resolveTerminal(cancelled);
    }
    for (const task of this.#tasks.values()) {
      task.detachRunner?.();
      for (const release of task.holders.values()) release?.();
      task.holders.clear();
      void task.runner?.close?.().catch(() => {
        // Ephemeral Child Session cleanup is best effort during host shutdown.
      });
      task.listeners.clear();
    }
    this.#delegations.clear();
    this.#tasks.clear();
    this.#terminalOrder = 0;
  }

  async shutdown(): Promise<void> {
    const cancelled = immutable({ status: "cancelled" as const });
    const cleanup: Promise<void>[] = [];
    for (const delegation of this.#delegations.values()) {
      delegation.abortGroup?.();
      if (delegation.phase !== "closed") delegation.resolveTerminal(cancelled);
    }
    for (const task of this.#tasks.values()) {
      task.detachRunner?.();
      for (const release of task.holders.values()) release?.();
      task.holders.clear();
      if (task.runner?.close) cleanup.push(task.runner.close());
      task.listeners.clear();
    }
    await Promise.allSettled(cleanup);
    this.#delegations.clear();
    this.#tasks.clear();
    this.#terminalOrder = 0;
  }

  #closeTaskRunner(task: TaskRecord): void {
    task.detachRunner?.();
    void task.runner?.close?.().catch(() => {
      // Ephemeral Child Session cleanup is best effort after lifecycle completion.
    });
    delete task.detachRunner;
    delete task.runner;
    delete task.closeWhenReleased;
  }

  #evict(): void {
    const terminals = [...this.#delegations.entries()]
      .filter(([, delegation]) => delegation.phase === "closed")
      .sort((left, right) => (right[1].terminalOrder ?? 0) - (left[1].terminalOrder ?? 0));
    for (const [delegationId, delegation] of terminals.slice(this.#terminalLimit)) {
      this.#delegations.delete(delegationId);
      for (const taskId of delegation.taskIds) {
        this.#tasks.get(taskId)?.listeners.clear();
        this.#tasks.delete(taskId);
      }
    }
  }
}
