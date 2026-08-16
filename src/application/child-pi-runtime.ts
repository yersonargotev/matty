// Process-launch and JSONL behavior adapted from Pi's RPC documentation and subagent example.
// Pi is MIT licensed. Matty owns this adapted runtime and its invariants.
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { chmod, mkdtemp, realpath, rm } from "node:fs/promises";
import { isAbsolute, join } from "node:path";

import {
  classifyChildExecutionActivity,
  type ChildExecutionActivityObservation,
} from "../domain/child-execution-activity.ts";
import type { ChildSessionHandle } from "./child-session-store.ts";

export type PiThinkingLevel = "off" | "minimal" | "low" | "medium" | "high" | "xhigh";

export interface ParentPiExecutionContext {
  provider: string;
  model: string;
  thinking: PiThinkingLevel;
  cwd: string;
}

export interface PiInvocation {
  command: string;
  arguments?: readonly string[];
}

export interface ChildSafePiAuthentication {
  provider: string;
  environment: NodeJS.ProcessEnv;
}

export interface ChildIdentity {
  runId: string;
  pid: number;
}

export interface ChildExit {
  code: number | null;
  signal: NodeJS.Signals | null;
}

export interface ChildTranscript {
  readonly entries: readonly Readonly<Record<string, unknown>>[];
}

export type DelegatedTaskProgress =
  | { type: "started"; child: Pick<ChildIdentity, "pid"> }
  | { type: "identified"; child: ChildIdentity }
  | {
      type: "activity";
      child: ChildIdentity;
      observation: ChildExecutionActivityObservation;
    }
  | {
      type: "live";
      child: ChildIdentity;
      revision: number;
    }
  | { type: "terminating"; child: ChildIdentity; signal: "SIGTERM" }
  | { type: "killing"; child: ChildIdentity; signal: "SIGKILL" };

export interface DelegatedAssistantPresentationPart {
  readonly contentIndex: number;
  readonly type: "text" | "thinking";
  readonly content: string;
}

export interface DelegatedToolPresentation {
  readonly toolCallId: string;
  readonly toolName: string;
  readonly status: "running" | "completed";
  readonly args?: string;
  readonly content?: string;
  readonly isError?: boolean;
}

export interface DelegatedTranscriptPresentationEntry {
  readonly id: string;
  readonly category: "message" | "reasoning" | "tool" | "error";
  readonly label: string;
  readonly content: string;
  readonly expandedByDefault: boolean;
}

export interface ChildExtensionInputRequest {
  readonly id: string;
  readonly method: "select" | "confirm" | "input" | "editor";
  readonly title: string;
  readonly message?: string;
  readonly options?: readonly string[];
  readonly placeholder?: string;
  readonly prefill?: string;
  readonly expiresAt: number;
  /** Only Matty's default timeout may be explicitly extended. */
  readonly extendable: boolean;
}

export interface DelegatedTaskPresentation {
  readonly revision: number;
  readonly sessionState: "working" | "settled" | "waiting-for-input" | "waiting-for-capability";
  readonly pendingInput?: ChildExtensionInputRequest;
  readonly assistant: readonly DelegatedAssistantPresentationPart[];
  readonly tools: readonly DelegatedToolPresentation[];
  readonly entries: readonly DelegatedTranscriptPresentationEntry[];
  readonly usage: {
    readonly inputTokens: number;
    readonly outputTokens: number;
    readonly totalTokens: number;
    readonly cost: number;
  };
}

export type DelegatedTaskOutcome =
  | {
      status: "succeeded";
      child: ChildIdentity;
      output: string;
      exit: ChildExit & { code: 0; signal: null };
    }
  | {
      status: "failed";
      child: ChildIdentity | null;
      failure: {
        kind:
          | "invalid-parent-context"
          | "spawn-failed"
          | "protocol-failed"
          | "child-failed"
          | "child-exited";
        message: string;
      };
      exit?: ChildExit;
    }
  | {
      status: "cancelled";
      child: ChildIdentity | null;
      phase: "before-spawn" | "running";
      exit?: ChildExit;
    };

const transcripts = new WeakMap<object, ChildTranscript>();
const terminalResponses = new WeakMap<object, readonly string[]>();

/** Returns sensitive in-memory data only to an explicit runtime caller. It is never serialized. */
export function childTranscript(outcome: object): ChildTranscript | undefined {
  return transcripts.get(outcome);
}

function withTranscript<T extends DelegatedTaskOutcome>(
  outcome: T,
  entries: readonly Readonly<Record<string, unknown>>[],
  responses: readonly string[],
): T {
  transcripts.set(outcome, {
    entries: Object.freeze(entries.map((entry) => Object.freeze(structuredClone(entry)))),
  });
  terminalResponses.set(outcome, Object.freeze([...responses]));
  return outcome;
}

/** Returns terminal assistant texts only to role validators. */
export function childTerminalResponses(outcome: object): readonly string[] {
  const retained = terminalResponses.get(outcome);
  if (retained) return retained;
  const output = (outcome as { output?: unknown }).output;
  return typeof output === "string" ? [output] : [];
}

/** Transfers private Child Session data when replacing an outcome object. */
export function transferChildTranscript<T extends object>(source: object, target: T): T {
  const transcript = transcripts.get(source);
  if (transcript) transcripts.set(target, transcript);
  const responses = terminalResponses.get(source);
  if (responses) terminalResponses.set(target, responses);
  return target;
}

export type ChildInteraction = {
  type: "steer" | "follow_up";
  message: string;
};

export type ChildInteractionResult =
  | { status: "accepted"; commandId: string }
  | { status: "rejected"; code: "child-session-unavailable" | "child-session-settled" | "command-rejected" };

export type ChildInputResponse =
  | { cancelled: true }
  | { value: string }
  | { confirmed: boolean };

export type ChildInputResult =
  | { status: "accepted" }
  | { status: "rejected"; code: "child-session-unavailable" | "input-request-unavailable" | "command-rejected" };

export interface DelegatedTaskRunner {
  run(
    task: string,
    options?: {
      signal?: AbortSignal;
      onProgress?: (progress: DelegatedTaskProgress) => void;
    },
  ): Promise<DelegatedTaskOutcome>;
  interact?(interaction: ChildInteraction): Promise<ChildInteractionResult>;
  respondToInput?(requestId: string, response: ChildInputResponse): Promise<ChildInputResult>;
  extendInputTimeout?(requestId: string, extensionMs?: number): ChildInputResult;
  close?(): Promise<void>;
  freeze?(): void;
  abort?(): void;
  presentation?(): DelegatedTaskPresentation | undefined;
  subscribePresentation?(
    listener: (presentation: DelegatedTaskPresentation) => void,
  ): () => void;
  /** Retains an interactive session until the returned holder is released. */
  retain?(): () => void;
}

export interface ChildPiRunnerOptions {
  invocation: PiInvocation;
  parent: ParentPiExecutionContext;
  authentication: ChildSafePiAuthentication;
  terminationGraceMs?: number;
  /** Worker-only capability lifecycle hooks. Other roles leave these absent. */
  beforeInteraction?: (
    signal?: AbortSignal,
    context?: { readonly candidateObserved: boolean },
  ) => Promise<boolean>;
  onInteractionRejected?: () => void | Promise<void>;
  onTerminalResponse?: (text: string) => void;
  /** Matty-owned session storage used to resume a settled Child Session. */
  sessionRoot?: string;
  session?: ChildSessionHandle | undefined;
  retainSessionUntilClose?: boolean;
  scheduleExecution?: <T>(execution: () => Promise<T>, signal?: AbortSignal) => Promise<T | undefined>;
}

interface PiMessageEnd extends Record<string, unknown> {
  type: "message_end";
  message: {
    role: string;
    content: Array<Record<string, unknown>> | string;
    stopReason?: string;
    errorMessage?: string;
    usage?: Record<string, unknown>;
    isError?: boolean;
  };
}

function record(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined;
}

function validAssistantContentPart(value: unknown): value is Record<string, unknown> {
  const part = record(value);
  if (!part) return false;
  switch (part.type) {
    case "text":
      return typeof part.text === "string";
    case "thinking":
      return typeof part.thinking === "string";
    case "toolCall":
      return nonemptyString(part.id) && nonemptyString(part.name) &&
        record(part.arguments) !== undefined;
    default:
      return false;
  }
}

function validNonAssistantContentPart(value: unknown): value is Record<string, unknown> {
  const part = record(value);
  return part !== undefined && typeof part.type === "string" &&
    (part.text === undefined || typeof part.text === "string");
}

function validMessage(value: unknown): value is PiMessageEnd["message"] {
  const message = record(value);
  if (!message || typeof message.role !== "string" || message.role.length === 0) return false;
  if (message.role === "assistant") {
    return Array.isArray(message.content) &&
      message.content.every(validAssistantContentPart);
  }
  if (typeof message.content === "string") return true;
  return Array.isArray(message.content) &&
    message.content.every(validNonAssistantContentPart);
}

function isMessageEnd(value: unknown): value is PiMessageEnd {
  const candidate = record(value);
  const message = record(candidate?.message);
  if (candidate?.type !== "message_end" || !validMessage(message)) return false;
  return (message.stopReason === undefined || typeof message.stopReason === "string") &&
    (message.errorMessage === undefined || typeof message.errorMessage === "string");
}

function neutralizeTerminalText(value: string): string {
  return value
    // OSC sequences, including title/clipboard controls, terminate with BEL or ST.
    .replace(/\u001b\][^\u0007\u001b]*(?:\u0007|\u001b\\)/g, "")
    // DCS/SOS/PM/APC strings terminate with ST.
    .replace(/\u001b[PX^_][\s\S]*?\u001b\\/g, "")
    // CSI and short ESC sequences must never reach a terminal renderer.
    .replace(/\u001b\[[0-?]*[ -/]*[@-~]/g, "")
    .replace(/\u001b[@-_]/g, "")
    // Preserve ordinary layout but remove C0/C1 terminal controls.
    .replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g, "");
}

function presentationContent(value: unknown): string {
  if (typeof value === "string") return neutralizeTerminalText(value);
  try {
    return neutralizeTerminalText(JSON.stringify(value));
  } catch {
    return "[unavailable]";
  }
}

function assistantText(message: PiMessageEnd["message"]): string {
  return neutralizeTerminalText(Array.isArray(message.content)
    ? message.content.flatMap((part) =>
      part.type === "text" && typeof part.text === "string" ? [part.text] : []
    ).join("\n")
    : "");
}

async function invalidParentContext(
  parent: ParentPiExecutionContext,
  authentication: ChildSafePiAuthentication,
): Promise<string | undefined> {
  if (!parent.provider.trim()) return "The parent provider is unavailable";
  if (!parent.model.trim()) return "The parent model is unavailable";
  if (!isAbsolute(parent.cwd)) return "The parent working directory must be absolute";
  try {
    if ((await realpath(parent.cwd)) !== parent.cwd) {
      return "The parent working directory must be canonical";
    }
  } catch {
    return "The parent working directory is unavailable";
  }
  if (authentication.provider !== parent.provider) {
    return "The child authentication provider does not match the parent";
  }
  return undefined;
}

function emitProgress(
  callback: ((progress: DelegatedTaskProgress) => void) | undefined,
  progress: DelegatedTaskProgress,
): void {
  try {
    callback?.(progress);
  } catch {
    // Progress observers cannot alter or orphan the child lifecycle.
  }
}

export function createChildPiRunner(options: ChildPiRunnerOptions): DelegatedTaskRunner {
  const graceMs = options.terminationGraceMs ?? 5_000;
  let activeControl: ChildProcessControl | undefined;
  let presentation: DelegatedTaskPresentation | undefined;
  let sessionDirectory: string | undefined;
  let sessionId: string | undefined;
  let sessionTerminalState: "succeeded" | "failed" | "cancelled" | undefined;
  let sessionFinished = false;
  let runSignal: AbortSignal | undefined;
  let runProgress: ((progress: DelegatedTaskProgress) => void) | undefined;
  let acceptingInteractions = true;
  const interactionLifetime = new AbortController();
  let executionChain: Promise<void> = Promise.resolve();
  let executionCount = 0;
  let holderCount = 0;
  const holderWaiters = new Set<() => void>();
  const outcomes: DelegatedTaskOutcome[] = [];
  const presentationListeners = new Set<(value: DelegatedTaskPresentation) => void>();
  let capabilityWaiters = 0;
  let presentationNotificationPending = false;
  const publishPresentation = (value: DelegatedTaskPresentation): void => {
    presentation = capabilityWaiters > 0 && value.sessionState !== "waiting-for-input"
      ? Object.freeze({ ...value, sessionState: "waiting-for-capability" as const })
      : value;
    if (presentationNotificationPending) return;
    presentationNotificationPending = true;
    queueMicrotask(() => {
      presentationNotificationPending = false;
      if (!presentation) return;
      for (const listener of presentationListeners) {
        try { listener(presentation); } catch { /* Observers cannot block protocol ingestion. */ }
      }
    });
  };
  const parent = { ...options.parent };
  const invocation = { command: options.invocation.command, arguments: [...(options.invocation.arguments ?? [])] };
  const authentication = { provider: options.authentication.provider, environment: { ...options.authentication.environment } };
  const schedule = options.scheduleExecution ?? (async <T>(execution: () => Promise<T>) => await execution());

  const execute = async (
    task: string,
    accepted?: (accepted: boolean) => void,
    reservedExecutionNumber?: number,
  ): Promise<DelegatedTaskOutcome> => {
    const runId = randomUUID();
    const executionNumber = reservedExecutionNumber ?? ++executionCount;
    const priorEntries = presentation?.entries ?? [];
    const priorUsage = presentation?.usage ?? { inputTokens: 0, outputTokens: 0, totalTokens: 0, cost: 0 };
    let promptAccepted = false;
    const publishExecutionPresentation = (value: DelegatedTaskPresentation): void => {
      const qualify = (id: string) => `execution:${executionNumber}:${id}`;
      publishPresentation(Object.freeze({
        ...value,
        assistant: Object.freeze(value.assistant.map((part) => Object.freeze({ ...part }))),
        tools: Object.freeze(value.tools.map((tool) => Object.freeze({ ...tool, toolCallId: qualify(tool.toolCallId) }))),
        entries: Object.freeze([
          ...priorEntries,
          ...value.entries.map((entry) => Object.freeze({ ...entry, id: qualify(entry.id) })),
        ]),
        usage: Object.freeze({
          inputTokens: priorUsage.inputTokens + value.usage.inputTokens,
          outputTokens: priorUsage.outputTokens + value.usage.outputTokens,
          totalTokens: priorUsage.totalTokens + value.usage.totalTokens,
          cost: priorUsage.cost + value.usage.cost,
        }),
      }));
    };
    const retainedTranscriptBytes = outcomes.reduce((total, outcome) =>
      total + (transcripts.get(outcome)?.entries ?? []).reduce(
        (bytes, entry) => bytes + Buffer.byteLength(JSON.stringify(entry)), 0,
      ), 0);
    const executeChild = async () => await superviseChild({
      command: invocation.command,
      arguments: [
        ...invocation.arguments,
        "--mode", "rpc",
        ...(options.session
          ? ["--session", options.session.sessionFile]
          : sessionDirectory && sessionId
            ? ["--session-dir", sessionDirectory, "--session-id", sessionId]
            : ["--no-session", "--session-id", runId]),
        "--provider", parent.provider,
        "--model", parent.model,
        "--thinking", parent.thinking,
      ],
      cwd: parent.cwd,
      environment: authentication.environment,
      task,
      runId,
      promptId: randomUUID(),
      graceMs,
      transcriptBudgetBytes: Math.max(0, MAX_TRANSCRIPT_BYTES - retainedTranscriptBytes),
      signal: runSignal,
      onProgress: runProgress,
      onControl(control) { activeControl = control; },
      onPresentation: publishExecutionPresentation,
      ...(accepted ? { onPromptAccepted() { promptAccepted = true; accepted(true); } } : {}),
      ...(options.beforeInteraction ? { beforeInteraction: (candidateObserved) => options.beforeInteraction!(interactionLifetime.signal, { candidateObserved }) } : {}),
      ...(options.onInteractionRejected ? { onInteractionRejected: options.onInteractionRejected } : {}),
      ...(options.onTerminalResponse ? { onTerminalResponse: options.onTerminalResponse } : {}),
    });
    const outcome = await schedule(executeChild, runSignal) ?? {
      status: "cancelled" as const, child: null, phase: "before-spawn" as const,
    };
    outcomes.push(outcome);
    activeControl = undefined;
    if (accepted && !promptAccepted) accepted(false);
    return outcome;
  };

  const combinedOutcome = (fallback: DelegatedTaskOutcome): DelegatedTaskOutcome => {
    const latest = outcomes.at(-1) ?? fallback;
    const entries = outcomes.flatMap((outcome) => transcripts.get(outcome)?.entries ?? []);
    const responses = outcomes.flatMap((outcome) => terminalResponses.get(outcome) ?? []);
    return withTranscript(latest, entries, responses);
  };
  const cleanup = async (): Promise<void> => {
    acceptingInteractions = false;
    await executionChain;
    const directory = sessionDirectory;
    sessionDirectory = undefined;
    const storedOutcome = outcomes.at(-1);
    const terminalState = sessionTerminalState ?? (storedOutcome
      ? storedOutcome.status === "succeeded"
        ? "succeeded"
        : storedOutcome.status === "cancelled" ? "cancelled" : "failed"
      : undefined);
    if (options.session && !sessionFinished && terminalState) {
      sessionFinished = true;
      await options.session.finish(terminalState);
    }
    if (options.session) await options.session.close();
    else if (directory) await rm(directory, { recursive: true, force: true });
  };

  return {
    async run(task, runOptions = {}) {
      if (runOptions.signal?.aborted) return { status: "cancelled", child: null, phase: "before-spawn" };
      const parentError = await invalidParentContext(parent, authentication);
      if (parentError) return { status: "failed", child: null, failure: { kind: "invalid-parent-context", message: parentError } };
      runSignal = runOptions.signal;
      runProgress = runOptions.onProgress;
      acceptingInteractions = true;
      outcomes.length = 0;
      executionCount = 0;
      presentation = undefined;
      sessionTerminalState = undefined;
      sessionFinished = false;
      sessionId = options.session?.sessionId ?? randomUUID();
      if (options.session) {
        await options.session.prepare(parent.cwd);
        sessionDirectory = options.session.directory;
      } else if (options.sessionRoot) {
        sessionDirectory = await mkdtemp(join(options.sessionRoot, "matty-child-session-"));
        await chmod(sessionDirectory, 0o700);
      }
      const initialExecution = execute(task);
      executionChain = initialExecution.then(() => {});
      const initial = await initialExecution;
      if (holderCount > 0) {
        await new Promise<void>((resolve) => holderWaiters.add(resolve));
      }
      acceptingInteractions = false;
      await executionChain;
      const result = combinedOutcome(initial);
      sessionTerminalState = result.status === "succeeded"
        ? "succeeded"
        : result.status === "cancelled" ? "cancelled" : "failed";
      if (!options.retainSessionUntilClose) await cleanup();
      return result;
    },
    async interact(interaction) {
      if (activeControl && presentation?.sessionState !== "settled") {
        const result = await activeControl.interact(interaction);
        if (result.status === "accepted" || result.code !== "child-session-settled") return result;
      }
      if (!acceptingInteractions || !sessionId || !sessionDirectory) {
        return { status: "rejected", code: "child-session-unavailable" };
      }
      if (
        Buffer.byteLength(interaction.message) > MAX_INTERACTION_MESSAGE_BYTES ||
        executionCount >= MAX_RESPAWN_EXECUTIONS
      ) {
        return { status: "rejected", code: "command-rejected" };
      }
      // Reserve admission before yielding so over-limit calls never acquire authority.
      const executionNumber = ++executionCount;
      if (options.beforeInteraction) {
        capabilityWaiters += 1;
        if (presentation) publishPresentation(Object.freeze({
          ...presentation,
          revision: presentation.revision + 1,
          sessionState: "waiting-for-capability" as const,
        }));
        let available = false;
        try {
          available = await options.beforeInteraction(interactionLifetime.signal, { candidateObserved: true });
        } catch {
          available = false;
        } finally {
          capabilityWaiters = Math.max(0, capabilityWaiters - 1);
        }
        if (presentation) publishPresentation(Object.freeze({
          ...presentation,
          revision: presentation.revision + 1,
          sessionState: capabilityWaiters > 0 ? "waiting-for-capability" as const : "settled" as const,
        }));
        if (!available) return { status: "rejected", code: "command-rejected" };
      }
      const commandId = randomUUID();
      let acceptedResolve!: (accepted: boolean) => void;
      const accepted = new Promise<boolean>((resolve) => { acceptedResolve = resolve; });
      executionChain = executionChain.then(async () => {
        await execute(interaction.message, acceptedResolve, executionNumber);
      });
      const wasAccepted = await accepted;
      if (!wasAccepted) await options.onInteractionRejected?.();
      return wasAccepted
        ? { status: "accepted", commandId }
        : { status: "rejected", code: "command-rejected" };
    },
    async respondToInput(requestId, response) {
      return activeControl ? await activeControl.respondToInput(requestId, response) : { status: "rejected", code: "child-session-unavailable" };
    },
    extendInputTimeout(requestId, extensionMs) {
      return activeControl ? activeControl.extendInputTimeout(requestId, extensionMs) : { status: "rejected", code: "child-session-unavailable" };
    },
    async close() { interactionLifetime.abort(); await cleanup(); },
    freeze() { acceptingInteractions = false; interactionLifetime.abort(); },
    abort() { acceptingInteractions = false; interactionLifetime.abort(); activeControl?.abort(); },
    presentation() { return presentation; },
    subscribePresentation(listener) {
      presentationListeners.add(listener);
      if (presentation) listener(presentation);
      return () => presentationListeners.delete(listener);
    },
    retain() {
      holderCount += 1;
      let released = false;
      return () => {
        if (released) return;
        released = true;
        holderCount = Math.max(0, holderCount - 1);
        if (holderCount === 0) {
          for (const resolve of holderWaiters) resolve();
          holderWaiters.clear();
        }
      };
    },
  };
}

interface ChildProcessControl {
  interact(interaction: ChildInteraction): Promise<ChildInteractionResult>;
  respondToInput(requestId: string, response: ChildInputResponse): Promise<ChildInputResult>;
  extendInputTimeout(requestId: string, extensionMs?: number): ChildInputResult;
  abort(): void;
}

interface SupervisionOptions {
  command: string;
  arguments: string[];
  cwd: string;
  environment: NodeJS.ProcessEnv;
  task: string;
  runId: string;
  promptId: string;
  graceMs: number;
  transcriptBudgetBytes: number;
  signal: AbortSignal | undefined;
  onProgress: ((progress: DelegatedTaskProgress) => void) | undefined;
  onControl(control: ChildProcessControl): void;
  onPresentation(presentation: DelegatedTaskPresentation): void;
  onPromptAccepted?: () => void;
  beforeInteraction?: (candidateObserved: boolean) => Promise<boolean>;
  onInteractionRejected?: () => void | Promise<void>;
  onTerminalResponse?: (text: string) => void;
}

const TRANSCRIPT_TYPES = new Set([
  "agent_start", "agent_end", "agent_settled", "turn_start", "turn_end",
  "message_start", "message_update", "message_end", "tool_execution_start",
  "tool_execution_update", "tool_execution_end", "queue_update", "compaction_start",
  "compaction_end", "auto_retry_start", "auto_retry_end",
  "summarization_retry_scheduled", "summarization_retry_attempt_start",
  "summarization_retry_finished", "extension_error",
]);
const terminalAssistantStopReasons = new Set([
  "stop", "length", "deferred", "error", "aborted",
]);
const compactionReasons = new Set(["manual", "threshold", "overflow"]);

function stringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

function nonemptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function validMessageUpdate(value: unknown): boolean {
  const update = record(value);
  if (!update || typeof update.type !== "string") return false;
  switch (update.type) {
    case "text_start":
    case "thinking_start":
      return Number.isSafeInteger(update.contentIndex);
    case "text_delta":
    case "thinking_delta":
    case "toolcall_delta":
      return Number.isSafeInteger(update.contentIndex) && typeof update.delta === "string";
    case "text_end":
      return Number.isSafeInteger(update.contentIndex) && typeof update.content === "string";
    case "thinking_end":
      return Number.isSafeInteger(update.contentIndex) && typeof update.content === "string";
    case "toolcall_start":
      return Number.isSafeInteger(update.contentIndex);
    case "toolcall_end":
      return Number.isSafeInteger(update.contentIndex) && record(update.toolCall) !== undefined;
    default:
      return false;
  }
}

function validKnownTranscriptEvent(event: Record<string, unknown>): boolean {
  switch (event.type) {
    case "agent_start":
    case "agent_settled":
    case "turn_start":
    case "summarization_retry_finished":
      return true;
    case "agent_end":
      return Array.isArray(event.messages) && event.messages.every(validMessage) &&
        typeof event.willRetry === "boolean";
    case "turn_end":
      return validMessage(event.message) && Array.isArray(event.toolResults) &&
        event.toolResults.every(validMessage);
    case "message_start":
      return validMessage(event.message);
    case "message_update":
      return record(event.usage) !== undefined && validMessageUpdate(event.assistantMessageEvent);
    case "message_end":
      return isMessageEnd(event);
    case "tool_execution_start":
      return nonemptyString(event.toolCallId) && nonemptyString(event.toolName) &&
        record(event.args) !== undefined;
    case "tool_execution_update":
      return nonemptyString(event.toolCallId) && nonemptyString(event.toolName) &&
        record(event.args) !== undefined && "partialResult" in event;
    case "tool_execution_end":
      return nonemptyString(event.toolCallId) && nonemptyString(event.toolName) &&
        "result" in event && typeof event.isError === "boolean";
    case "queue_update":
      return stringArray(event.steering) && stringArray(event.followUp);
    case "compaction_start":
      return typeof event.reason === "string" && compactionReasons.has(event.reason);
    case "compaction_end":
      return typeof event.reason === "string" && compactionReasons.has(event.reason) &&
        (event.result === undefined || event.result === null || record(event.result) !== undefined) &&
        typeof event.aborted === "boolean" && typeof event.willRetry === "boolean" &&
        (event.errorMessage === undefined || typeof event.errorMessage === "string");
    case "auto_retry_start":
    case "summarization_retry_scheduled":
      return Number.isSafeInteger(event.attempt) && Number.isSafeInteger(event.maxAttempts) &&
        Number.isSafeInteger(event.delayMs) && typeof event.errorMessage === "string";
    case "auto_retry_end":
      return typeof event.success === "boolean" && Number.isSafeInteger(event.attempt) &&
        (event.finalError === undefined || typeof event.finalError === "string");
    case "summarization_retry_attempt_start":
      return event.source === "branchSummary" ||
        (event.source === "compaction" && typeof event.reason === "string" &&
          compactionReasons.has(event.reason));
    case "extension_error":
      return typeof event.extensionPath === "string" && typeof event.event === "string" &&
        typeof event.error === "string";
    default:
      return false;
  }
}

const MAX_FRAME_BYTES = 4 * 1024 * 1024;
const MAX_BUFFER_BYTES = 8 * 1024 * 1024;
const MAX_TRANSCRIPT_BYTES = 8 * 1024 * 1024;
const MAX_STDERR_BYTES = 64 * 1024;
const MAX_INTERACTION_MESSAGE_BYTES = 64 * 1024;
const MAX_PENDING_INTERACTION_COMMANDS = 16;
const MAX_RESPAWN_EXECUTIONS = 16;
const DEFAULT_EXTENSION_INPUT_TIMEOUT_MS = 5 * 60 * 1_000;
const MAX_EXTENSION_INPUT_TIMEOUT_MS = 60 * 60 * 1_000;
const MAX_NODE_TIMER_DELAY_MS = 2_147_483_647;
const extensionDialogMethods = new Set(["select", "confirm", "input", "editor"]);

async function superviseChild(options: SupervisionOptions): Promise<DelegatedTaskOutcome> {
  let child;
  try {
    child = spawn(options.command, options.arguments, {
      cwd: options.cwd,
      env: options.environment,
      shell: false,
      stdio: ["pipe", "pipe", "pipe"],
    });
  } catch (error) {
    return {
      status: "failed",
      child: null,
      failure: {
        kind: "spawn-failed",
        message: error instanceof Error ? error.message : String(error),
      },
    };
  }

  return await new Promise<DelegatedTaskOutcome>((resolve) => {
    let identity: ChildIdentity | null = null;
    let stdout = "";
    let finalMessage: PiMessageEnd["message"] | undefined;
    const terminalAssistantTexts: string[] = [];
    const transcriptEntries: Readonly<Record<string, unknown>>[] = [];
    const partialTranscriptIndexes = new Map<string, number>();
    const assistantParts = new Map<number, {
      type: "text" | "thinking";
      content: string;
    }>();
    const presentedTools = new Map<string, {
      toolCallId: string;
      toolName: string;
      status: "running" | "completed";
      args?: string;
      content?: string;
      isError?: boolean;
    }>();
    const presentedEntries = new Map<string, DelegatedTranscriptPresentationEntry>();
    let turnSequence = 0;
    let usage = { inputTokens: 0, outputTokens: 0, totalTokens: 0, cost: 0 };
    const pendingCommands = new Map<string, {
      command: ChildInteraction["type"];
      frame: string;
      resolve(result: ChildInteractionResult): void;
    }>();
    const interactionWriteQueue: string[] = [];
    let interactionWriteBackpressured = false;
    let transcriptBytes = 0;
    let liveAssistantBytes = 0;
    let liveRevision = 0;
    let liveUpdatePending = false;
    let waitingForCapability = 0;
    let pendingInput: {
      request: ChildExtensionInputRequest;
      timer: NodeJS.Timeout;
    } | undefined;
    let extensionResponseWritePending = false;
    let stderrBytes = 0;
    let sequence = 0;
    let promptAccepted = false;
    let agentSettled = false;
    let protocolFailure: string | undefined;
    let cancellationRequested = false;
    let closed = false;
    let resolved = false;
    let terminationTimer: NodeJS.Timeout | undefined;
    let spawned = false;
    const progressQueue: DelegatedTaskProgress[] = [];
    let progressDispatchPending = false;

    const scheduleProgressDispatch = (): void => {
      if (progressDispatchPending || !options.onProgress) return;
      progressDispatchPending = true;
      setImmediate(() => {
        progressDispatchPending = false;
        const batch = progressQueue.splice(0);
        for (const update of batch) emitProgress(options.onProgress, update);
        if (progressQueue.length > 0) scheduleProgressDispatch();
      });
    };
    const dispatchProgress = (progress: DelegatedTaskProgress): void => {
      if (!options.onProgress) return;
      if (progress.type === "live" && progressQueue.at(-1)?.type === "live") {
        progressQueue[progressQueue.length - 1] = progress;
      } else {
        progressQueue.push(progress);
      }
      scheduleProgressDispatch();
    };

    const resumeInteractionWrites = (): void => {
      interactionWriteBackpressured = false;
      pumpInteractionWrites();
    };

    const rejectPendingCommand = (commandId: string): void => {
      const pending = pendingCommands.get(commandId);
      if (!pending) return;
      pendingCommands.delete(commandId);
      pending.resolve({ status: "rejected", code: "command-rejected" });
    };

    const pumpInteractionWrites = (): void => {
      if (interactionWriteBackpressured || closed || child.stdin.destroyed) return;
      while (interactionWriteQueue.length > 0) {
        const commandId = interactionWriteQueue.shift()!;
        const pending = pendingCommands.get(commandId);
        if (!pending) continue;
        let writable: boolean;
        try {
          writable = child.stdin.write(pending.frame, (error) => {
            if (error) rejectPendingCommand(commandId);
          });
        } catch {
          rejectPendingCommand(commandId);
          continue;
        }
        if (!writable) {
          interactionWriteBackpressured = true;
          child.stdin.once("drain", resumeInteractionWrites);
          return;
        }
      }
    };

    const settle = (outcome: DelegatedTaskOutcome): void => {
      if (resolved) return;
      resolved = true;
      if (terminationTimer) clearTimeout(terminationTimer);
      options.signal?.removeEventListener("abort", cancel);
      child.stdin.off("drain", resumeInteractionWrites);
      interactionWriteQueue.length = 0;
      for (const pending of pendingCommands.values()) {
        pending.resolve({ status: "rejected", code: "child-session-settled" });
      }
      pendingCommands.clear();
      if (pendingInput) clearTimeout(pendingInput.timer);
      pendingInput = undefined;
      resolve(withTranscript(outcome, transcriptEntries, terminalAssistantTexts));
    };

    const terminate = (message: string): void => {
      protocolFailure ??= message;
      if (closed) return;
      child.stdin.destroy();
      child.kill("SIGTERM");
      terminationTimer ??= setTimeout(() => {
        if (!closed) child.kill("SIGKILL");
      }, options.graceMs);
    };

    const emitLive = (): void => {
      if (liveUpdatePending || !identity) return;
      liveUpdatePending = true;
      queueMicrotask(() => {
        liveUpdatePending = false;
        if (!identity || resolved) return;
        liveRevision += 1;
        const assistant = [...assistantParts.entries()]
          .sort(([left], [right]) => left - right)
          .map(([contentIndex, part]) => Object.freeze({ contentIndex, ...part }));
        const tools = [...presentedTools.values()].map((tool) => Object.freeze({ ...tool }));
        const entries = [...presentedEntries.values()].map((entry) => Object.freeze({ ...entry }));
        options.onPresentation(Object.freeze({
          revision: liveRevision,
          sessionState: pendingInput ? "waiting-for-input" : waitingForCapability > 0 ? "waiting-for-capability" : agentSettled ? "settled" : "working",
          ...(pendingInput ? { pendingInput: pendingInput.request } : {}),
          assistant: Object.freeze(assistant),
          tools: Object.freeze(tools),
          entries: Object.freeze(entries),
          usage: Object.freeze({ ...usage }),
        }));
        dispatchProgress({ type: "live", child: identity, revision: liveRevision });
      });
    };

    const retainTranscript = (event: Readonly<Record<string, unknown>>, key?: string): boolean => {
      const bytes = Buffer.byteLength(JSON.stringify(event));
      const existing = key === undefined ? undefined : partialTranscriptIndexes.get(key);
      if (existing === undefined) {
        if (transcriptBytes + bytes > options.transcriptBudgetBytes) return false;
        transcriptEntries.push(event);
        transcriptBytes += bytes;
        if (key !== undefined) partialTranscriptIndexes.set(key, transcriptEntries.length - 1);
      } else {
        const previous = transcriptEntries[existing];
        const previousBytes = previous ? Buffer.byteLength(JSON.stringify(previous)) : 0;
        if (transcriptBytes - previousBytes + bytes > options.transcriptBudgetBytes) return false;
        transcriptEntries[existing] = event;
        transcriptBytes += bytes - previousBytes;
      }
      return true;
    };

    const consumeLine = (framedLine: string): void => {
      if (protocolFailure) return;
      const line = framedLine.endsWith("\r") ? framedLine.slice(0, -1) : framedLine;
      if (!line || Buffer.byteLength(line) > MAX_FRAME_BYTES) {
        terminate("Pi emitted an invalid JSONL frame");
        return;
      }
      let parsed: unknown;
      try {
        parsed = JSON.parse(line);
      } catch {
        terminate("Pi emitted invalid JSONL");
        return;
      }
      const event = record(parsed);
      if (!event || typeof event.type !== "string") {
        terminate("Pi emitted a malformed RPC frame");
        return;
      }

      if (event.type === "extension_ui_request") {
        if (!nonemptyString(event.method) || !extensionDialogMethods.has(event.method)) return;
        const validDialogShape = nonemptyString(event.id) && nonemptyString(event.title) &&
          (event.method !== "select" || (Array.isArray(event.options) && event.options.length > 0 && event.options.every(nonemptyString))) &&
          (event.method !== "confirm" || nonemptyString(event.message));
        if (!validDialogShape) {
          terminate("Pi emitted a malformed extension UI request");
          return;
        }
        if (!spawned || agentSettled || pendingInput) {
          terminate("Pi emitted an invalid extension UI dialog request");
          return;
        }
        const hasDeclaredTimeout = event.timeout !== undefined;
        if (hasDeclaredTimeout &&
            (typeof event.timeout !== "number" || !Number.isInteger(event.timeout) ||
              event.timeout <= 0 || event.timeout > MAX_NODE_TIMER_DELAY_MS)) {
          terminate("Pi emitted an invalid extension UI timeout");
          return;
        }
        const timeout = hasDeclaredTimeout ? event.timeout as number : DEFAULT_EXTENSION_INPUT_TIMEOUT_MS;
        const request = Object.freeze({
          id: event.id as string,
          method: event.method as ChildExtensionInputRequest["method"],
          title: neutralizeTerminalText(typeof event.title === "string" ? event.title : event.method),
          ...(typeof event.message === "string" ? { message: neutralizeTerminalText(event.message) } : {}),
          ...(stringArray(event.options) ? { options: Object.freeze(event.options.map(neutralizeTerminalText)) } : {}),
          ...(typeof event.placeholder === "string" ? { placeholder: neutralizeTerminalText(event.placeholder) } : {}),
          ...(typeof event.prefill === "string" ? { prefill: neutralizeTerminalText(event.prefill) } : {}),
          expiresAt: Date.now() + timeout,
          extendable: !hasDeclaredTimeout,
        });
        const expire = () => {
          if (pendingInput?.request.id !== request.id || closed || child.stdin.destroyed) return;
          child.stdin.write(`${JSON.stringify({ type: "extension_ui_response", id: request.id, cancelled: true })}\n`);
          pendingInput = undefined;
          emitLive();
        };
        pendingInput = { request, timer: setTimeout(expire, timeout) };
        emitLive();
        return;
      }

      if (event.type === "response") {
        if (typeof event.id !== "string" || typeof event.command !== "string" ||
            typeof event.success !== "boolean") {
          terminate("Pi emitted a malformed command response");
          return;
        }
        if (event.id === options.promptId) {
          if (promptAccepted || event.command !== "prompt") {
            terminate("Pi emitted an uncorrelated prompt response");
            return;
          }
          if (!event.success) {
            terminate("Pi rejected the delegated prompt");
            return;
          }
          promptAccepted = true;
          options.onPromptAccepted?.();
          if (identity) dispatchProgress({ type: "identified", child: identity });
          return;
        }
        const pending = pendingCommands.get(event.id);
        if (!pending || event.command !== pending.command) {
          terminate("Pi emitted an uncorrelated interaction response");
          return;
        }
        pendingCommands.delete(event.id);
        pending.resolve(event.success
          ? { status: "accepted", commandId: event.id }
          : { status: "rejected", code: "command-rejected" });
        return;
      }

      if (TRANSCRIPT_TYPES.has(event.type)) {
        if (!promptAccepted) {
          terminate("Pi emitted an event before accepting the delegated prompt");
          return;
        }
        if (agentSettled) {
          terminate("Pi emitted a transcript event after agent settled");
          return;
        }
        if (!validKnownTranscriptEvent(event)) {
          terminate(`Pi emitted a malformed ${event.type} event`);
          return;
        }
      }
      const activity = classifyChildExecutionActivity(event);
      if (activity.recognized && !activity.valid) {
        terminate("Pi emitted a malformed child activity event");
        return;
      }
      if (event.type === "turn_start") {
        turnSequence += 1;
        assistantParts.clear();
        presentedTools.clear();
        liveAssistantBytes = 0;
        partialTranscriptIndexes.clear();
      } else if (event.type === "message_start" && record(event.message)?.role === "assistant") {
        assistantParts.clear();
        liveAssistantBytes = [...presentedTools.values()].reduce(
          (bytes, tool) => bytes + Buffer.byteLength(tool.content ?? ""),
          0,
        );
      }
      if (event.type === "message_update") {
        const update = record(event.assistantMessageEvent)!;
        const index = update.contentIndex as number;
        if (
          update.type === "text_delta" || update.type === "thinking_delta" ||
          update.type === "text_end" || update.type === "thinking_end"
        ) {
          const type = update.type === "text_delta" || update.type === "text_end"
            ? "text" as const
            : "thinking" as const;
          const previous = assistantParts.get(index);
          const previousContent = previous?.type === type ? previous.content : "";
          const next = neutralizeTerminalText(
            update.type === "text_delta" || update.type === "thinking_delta"
              ? `${previousContent}${update.delta as string}`
              : update.content as string,
          );
          liveAssistantBytes += Buffer.byteLength(next) -
            Buffer.byteLength(previous?.content ?? "");
          if (liveAssistantBytes > MAX_BUFFER_BYTES) {
            terminate("Pi exceeded the live assistant buffer limit");
            return;
          }
          assistantParts.set(index, { type, content: next });
          presentedEntries.set(`assistant:${turnSequence}:${index}`, Object.freeze({
            id: `assistant:${turnSequence}:${index}`,
            category: type === "thinking" ? "reasoning" : "message",
            label: type === "thinking" ? "Reasoning" : "Assistant",
            content: next,
            expandedByDefault: type === "text",
          }));
          const usageRecord = record(event.usage);
          if (usageRecord) {
            const costRecord = record(usageRecord.cost);
            usage = {
              inputTokens: typeof usageRecord.input === "number"
                ? usageRecord.input
                : usage.inputTokens,
              outputTokens: typeof usageRecord.output === "number"
                ? usageRecord.output
                : usage.outputTokens,
              totalTokens: typeof usageRecord.totalTokens === "number"
                ? usageRecord.totalTokens
                : usage.totalTokens,
              cost: typeof costRecord?.total === "number" ? costRecord.total : usage.cost,
            };
          }
          emitLive();
        }
        if (!retainTranscript(event, `message:${index}`)) {
          terminate("Pi exceeded the in-memory transcript limit");
          return;
        }
      } else if (
        event.type === "tool_execution_start" ||
        event.type === "tool_execution_update" ||
        event.type === "tool_execution_end"
      ) {
        const toolCallId = event.toolCallId as string;
        const previous = presentedTools.get(toolCallId);
        const content = event.type === "tool_execution_update"
          ? presentationContent(event.partialResult)
          : event.type === "tool_execution_end"
            ? presentationContent(event.result)
            : undefined;
        const previousBytes = Buffer.byteLength(previous?.content ?? "");
        const nextBytes = Buffer.byteLength(content ?? "");
        liveAssistantBytes += nextBytes - previousBytes;
        if (liveAssistantBytes > MAX_BUFFER_BYTES) {
          terminate("Pi exceeded the live presentation buffer limit");
          return;
        }
        const args = event.args !== undefined
          ? presentationContent(event.args)
          : previous?.args;
        const toolName = neutralizeTerminalText(event.toolName as string);
        presentedTools.set(toolCallId, {
          toolCallId,
          toolName,
          status: event.type === "tool_execution_end" ? "completed" : "running",
          ...(args !== undefined ? { args } : {}),
          ...(content !== undefined ? { content } : {}),
          ...(event.type === "tool_execution_end" ? { isError: event.isError as boolean } : {}),
        });
        presentedEntries.set(`tool:${toolCallId}`, Object.freeze({
          id: `tool:${toolCallId}`,
          category: event.type === "tool_execution_end" && event.isError ? "error" : "tool",
          label: `${event.type === "tool_execution_end" && event.isError ? "Tool error" : "Tool"} · ${toolName}`,
          content: [args ? `Arguments: ${args}` : undefined, content ? `Result: ${content}` : undefined]
            .filter(Boolean).join("\n") || "Running",
          expandedByDefault: false,
        }));
        emitLive();
        if (!retainTranscript(event, `tool:${event.toolCallId as string}`)) {
          terminate("Pi exceeded the in-memory transcript limit");
          return;
        }
      } else if (TRANSCRIPT_TYPES.has(event.type) && !retainTranscript(event)) {
        terminate("Pi exceeded the in-memory transcript limit");
        return;
      }
      const eventError = event.type === "extension_error"
        ? event.error
        : event.type === "auto_retry_end" && !event.success
          ? event.finalError
          : event.type === "compaction_end"
            ? event.errorMessage
            : event.type === "auto_retry_start" || event.type === "summarization_retry_scheduled"
              ? event.errorMessage
              : undefined;
      if (typeof eventError === "string" && eventError.length > 0) {
        const id = `error:${turnSequence}:${presentedEntries.size}`;
        presentedEntries.set(id, Object.freeze({
          id,
          category: "error",
          label: "Error",
          content: neutralizeTerminalText(eventError),
          expandedByDefault: false,
        }));
        emitLive();
      }
      if (isMessageEnd(event)) {
        const messageUsage = record(event.message.usage);
        const messageCost = record(messageUsage?.cost);
        if (messageUsage) {
          usage = {
            inputTokens: typeof messageUsage.input === "number"
              ? messageUsage.input
              : usage.inputTokens,
            outputTokens: typeof messageUsage.output === "number"
              ? messageUsage.output
              : usage.outputTokens,
            totalTokens: typeof messageUsage.totalTokens === "number"
              ? messageUsage.totalTokens
              : usage.totalTokens,
            cost: typeof messageCost?.total === "number" ? messageCost.total : usage.cost,
          };
        }
        if (event.message.role !== "assistant") {
          const content = presentationContent(event.message.content);
          presentedEntries.set(`message:${turnSequence}:${presentedEntries.size}`, Object.freeze({
            id: `message:${turnSequence}:${presentedEntries.size}`,
            category: event.message.role === "toolResult" && event.message.isError ? "error" : "message",
            label: event.message.role === "user"
              ? "User"
              : event.message.role === "toolResult"
                ? "Tool result"
                : neutralizeTerminalText(event.message.role),
            content,
            expandedByDefault: event.message.role === "user",
          }));
        }
      }
      if (isMessageEnd(event) && event.message.role === "assistant") {
        finalMessage = event.message;
        if (terminalAssistantStopReasons.has(event.message.stopReason ?? "")) {
          const text = assistantText(event.message);
          terminalAssistantTexts.push(text);
          try { options.onTerminalResponse?.(text); } catch { /* Candidate observers cannot break RPC ingestion. */ }
        }
        assistantParts.clear();
        if (Array.isArray(event.message.content)) {
          event.message.content.forEach((part, contentIndex) => {
            if (part.type === "text" && typeof part.text === "string") {
              assistantParts.set(contentIndex, {
                type: "text",
                content: neutralizeTerminalText(part.text),
              });
            } else if (part.type === "thinking" && typeof part.thinking === "string") {
              assistantParts.set(contentIndex, {
                type: "thinking",
                content: neutralizeTerminalText(part.thinking),
              });
            }
          });
        }
        for (const [contentIndex, part] of assistantParts) {
          presentedEntries.set(`assistant:${turnSequence}:${contentIndex}`, Object.freeze({
            id: `assistant:${turnSequence}:${contentIndex}`,
            category: part.type === "thinking" ? "reasoning" : "message",
            label: part.type === "thinking" ? "Reasoning" : "Assistant",
            content: part.content,
            expandedByDefault: part.type === "text",
          }));
        }
        if (event.message.errorMessage) {
          presentedEntries.set(`error:${turnSequence}`, Object.freeze({
            id: `error:${turnSequence}`,
            category: "error",
            label: "Error",
            content: neutralizeTerminalText(event.message.errorMessage),
            expandedByDefault: false,
          }));
        }
        liveAssistantBytes = [...assistantParts.values(), ...presentedTools.values()]
          .reduce((bytes, part) => bytes + Buffer.byteLength(part.content ?? ""), 0);
        if (liveAssistantBytes > MAX_BUFFER_BYTES) {
          terminate("Pi exceeded the live presentation buffer limit");
          return;
        }
        emitLive();
      }
      if (activity.recognized && activity.valid && identity) {
        sequence += 1;
        dispatchProgress({
          type: "activity",
          child: identity,
          observation: {
            schemaVersion: 1,
            sequence,
            observedAt: Date.now(),
            summary: activity.summary,
          },
        });
      }
      if (event.type === "agent_settled") {
        if (
          agentSettled || !finalMessage ||
          !terminalAssistantStopReasons.has(finalMessage.stopReason ?? "")
        ) {
          terminate("Pi settled without a terminal correlated assistant result");
          return;
        }
        agentSettled = true;
        emitLive();
        child.stdin.end();
        terminationTimer ??= setTimeout(() => {
          if (closed) return;
          child.kill("SIGTERM");
          terminationTimer = setTimeout(() => {
            if (!closed) child.kill("SIGKILL");
          }, options.graceMs);
        }, options.graceMs);
      }
    };

    const cancel = (): void => {
      if (cancellationRequested || closed) return;
      cancellationRequested = true;
      if (identity) {
        dispatchProgress({
          type: "terminating", child: identity, signal: "SIGTERM",
        });
      }
      child.stdin.destroy();
      child.kill("SIGTERM");
      terminationTimer ??= setTimeout(() => {
        if (closed) return;
        if (identity) {
          dispatchProgress({
            type: "killing", child: identity, signal: "SIGKILL",
          });
        }
        child.kill("SIGKILL");
      }, options.graceMs);
    };

    options.onControl({
      async interact(interaction) {
        if (agentSettled || closed) {
          return { status: "rejected", code: "child-session-settled" };
        }
        if (!spawned || !promptAccepted || cancellationRequested) {
          return { status: "rejected", code: "child-session-unavailable" };
        }
        if (
          Buffer.byteLength(interaction.message) > MAX_INTERACTION_MESSAGE_BYTES ||
          pendingCommands.size >= MAX_PENDING_INTERACTION_COMMANDS ||
          child.stdin.destroyed || !child.stdin.writable
        ) {
          return { status: "rejected", code: "command-rejected" };
        }
        if (options.beforeInteraction) {
          waitingForCapability += 1;
          emitLive();
          let available = false;
          try { available = await options.beforeInteraction(terminalAssistantTexts.length > 0); } catch { available = false; }
          waitingForCapability = Math.max(0, waitingForCapability - 1);
          emitLive();
          if (!available) return { status: "rejected", code: "command-rejected" };
          if (
            agentSettled || closed || cancellationRequested ||
            pendingCommands.size >= MAX_PENDING_INTERACTION_COMMANDS ||
            child.stdin.destroyed || !child.stdin.writable
          ) {
            await options.onInteractionRejected?.();
            return agentSettled || closed
              ? { status: "rejected", code: "child-session-settled" }
              : { status: "rejected", code: "command-rejected" };
          }
        }
        const commandId = randomUUID();
        const result = await new Promise<ChildInteractionResult>((resolveCommand) => {
          pendingCommands.set(commandId, {
            command: interaction.type,
            frame: `${JSON.stringify({
              id: commandId,
              type: interaction.type,
              message: interaction.message,
            })}\n`,
            resolve: resolveCommand,
          });
          interactionWriteQueue.push(commandId);
          pumpInteractionWrites();
        });
        if (result.status === "rejected") await options.onInteractionRejected?.();
        return result;
      },
      async respondToInput(requestId, response) {
        if (!pendingInput || pendingInput.request.id !== requestId) {
          return { status: "rejected", code: "input-request-unavailable" };
        }
        const method = pendingInput.request.method;
        const validResponse = ("cancelled" in response && response.cancelled === true) ||
          ((method === "select" || method === "input" || method === "editor") && "value" in response && typeof response.value === "string") ||
          (method === "confirm" && "confirmed" in response && typeof response.confirmed === "boolean");
        if (!validResponse) return { status: "rejected", code: "command-rejected" };
        if (closed || child.stdin.destroyed || !child.stdin.writable) {
          return { status: "rejected", code: "command-rejected" };
        }
        clearTimeout(pendingInput.timer);
        pendingInput = undefined;
        extensionResponseWritePending = true;
        try {
          await new Promise<void>((resolveWrite, rejectWrite) => {
            child.stdin.write(`${JSON.stringify({ type: "extension_ui_response", id: requestId, ...response })}\n`, (error) =>
              error ? rejectWrite(error) : resolveWrite()
            );
          });
          emitLive();
          return { status: "accepted" };
        } catch {
          terminate("Pi extension UI response write failed");
          return { status: "rejected", code: "command-rejected" };
        } finally {
          extensionResponseWritePending = false;
        }
      },
      extendInputTimeout(requestId, extensionMs = DEFAULT_EXTENSION_INPUT_TIMEOUT_MS) {
        if (!pendingInput || pendingInput.request.id !== requestId) {
          return { status: "rejected", code: "input-request-unavailable" };
        }
        if (!pendingInput.request.extendable) {
          return { status: "rejected", code: "command-rejected" };
        }
        const extension = Math.max(1, Math.min(extensionMs, MAX_EXTENSION_INPUT_TIMEOUT_MS));
        clearTimeout(pendingInput.timer);
        const request = Object.freeze({ ...pendingInput.request, expiresAt: Date.now() + extension });
        const timer = setTimeout(() => {
          if (pendingInput?.request.id !== requestId || closed || child.stdin.destroyed) return;
          child.stdin.write(`${JSON.stringify({ type: "extension_ui_response", id: requestId, cancelled: true })}\n`);
          pendingInput = undefined;
          emitLive();
        }, extension);
        pendingInput = { request, timer };
        emitLive();
        return { status: "accepted" };
      },
      abort: cancel,
    });

    child.stdout.setEncoding("utf8");
    child.stdout.on("data", (chunk: string) => {
      stdout += chunk;
      if (Buffer.byteLength(stdout) > MAX_BUFFER_BYTES) {
        terminate("Pi exceeded the JSONL buffer limit");
        return;
      }
      while (true) {
        const newline = stdout.indexOf("\n");
        if (newline === -1) break;
        const line = stdout.slice(0, newline);
        stdout = stdout.slice(newline + 1);
        consumeLine(line);
      }
    });
    child.stderr.on("data", (chunk: Buffer | string) => {
      stderrBytes += Buffer.byteLength(chunk);
      if (stderrBytes > MAX_STDERR_BYTES) {
        terminate("Pi exceeded the stderr limit");
      }
    });
    child.stdin.on("error", () => {
      child.stdin.off("drain", resumeInteractionWrites);
      interactionWriteBackpressured = false;
      interactionWriteQueue.length = 0;
      for (const commandId of [...pendingCommands.keys()]) rejectPendingCommand(commandId);
      if (!closed && !cancellationRequested && !agentSettled) {
        terminate(extensionResponseWritePending
          ? "Pi extension UI response write failed"
          : "Pi child RPC input failed");
      }
    });

    child.once("spawn", () => {
      spawned = true;
      if (child.pid === undefined) {
        terminate("Pi spawned without an observable PID");
        return;
      }
      identity = { runId: options.runId, pid: child.pid };
      dispatchProgress({ type: "started", child: { pid: child.pid } });
      if (options.signal?.aborted) {
        cancel();
        return;
      }
      const command = JSON.stringify({
        id: options.promptId,
        type: "prompt",
        message: options.task,
      });
      child.stdin.write(`${command}\n`, (error) => {
        if (error && !closed && !cancellationRequested) {
          terminate("Pi rejected the RPC prompt stream");
        }
      });
    });

    child.on("error", (error) => {
      if (resolved) return;
      if (!spawned) {
        settle({
          status: "failed",
          child: null,
          failure: { kind: "spawn-failed", message: error.message },
        });
      } else {
        terminate("Pi child process control failed");
      }
    });

    child.once("close", (code, signal) => {
      closed = true;
      if (stdout.length > 0 && !protocolFailure) {
        protocolFailure = "Pi closed with an unterminated JSONL frame";
      }
      const exit = { code, signal };
      if (cancellationRequested) {
        settle({ status: "cancelled", child: identity, phase: "running", exit });
      } else if (protocolFailure) {
        settle({
          status: "failed",
          child: identity,
          failure: { kind: "protocol-failed", message: protocolFailure },
          exit,
        });
      } else if (!identity || !promptAccepted || !agentSettled || !finalMessage) {
        settle({
          status: "failed",
          child: identity,
          failure: {
            kind: "child-exited",
            message: "Pi child exited before producing a settled assistant result",
          },
          exit,
        });
      } else if (
        code !== 0 || signal !== null || finalMessage.stopReason === "error" ||
        finalMessage.stopReason === "aborted"
      ) {
        settle({
          status: "failed",
          child: identity,
          failure: {
            kind: "child-failed",
            message: finalMessage.errorMessage ?? `Pi child exited with code ${String(code)}`,
          },
          exit,
        });
      } else {
        settle({
          status: "succeeded",
          child: identity,
          output: assistantText(finalMessage),
          exit: { code: 0, signal: null },
        });
      }
    });

    options.signal?.addEventListener("abort", cancel, { once: true });
  });
}
