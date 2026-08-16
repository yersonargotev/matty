// Process-launch and JSONL behavior adapted from Pi's RPC documentation and subagent example.
// Pi is MIT licensed. Matty owns this adapted runtime and its invariants.
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { realpath } from "node:fs/promises";
import { isAbsolute } from "node:path";

import {
  classifyChildExecutionActivity,
  type ChildExecutionActivityObservation,
} from "../domain/child-execution-activity.ts";

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
  | { type: "terminating"; child: ChildIdentity; signal: "SIGTERM" }
  | { type: "killing"; child: ChildIdentity; signal: "SIGKILL" };

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

/** Returns sensitive in-memory data only to an explicit runtime caller. It is never serialized. */
export function childTranscript(outcome: object): ChildTranscript | undefined {
  return transcripts.get(outcome);
}

function withTranscript<T extends DelegatedTaskOutcome>(
  outcome: T,
  entries: readonly Readonly<Record<string, unknown>>[],
): T {
  transcripts.set(outcome, {
    entries: Object.freeze(entries.map((entry) => Object.freeze(structuredClone(entry)))),
  });
  return outcome;
}

/** Transfers the private in-memory transcript when replacing an outcome object. */
export function transferChildTranscript<T extends object>(source: object, target: T): T {
  const transcript = transcripts.get(source);
  if (transcript) transcripts.set(target, transcript);
  return target;
}

export interface DelegatedTaskRunner {
  run(
    task: string,
    options?: {
      signal?: AbortSignal;
      onProgress?: (progress: DelegatedTaskProgress) => void;
    },
  ): Promise<DelegatedTaskOutcome>;
}

export interface ChildPiRunnerOptions {
  invocation: PiInvocation;
  parent: ParentPiExecutionContext;
  authentication: ChildSafePiAuthentication;
  terminationGraceMs?: number;
}

interface PiMessageEnd extends Record<string, unknown> {
  type: "message_end";
  message: {
    role: string;
    content: Array<Record<string, unknown>> | string;
    stopReason?: string;
    errorMessage?: string;
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

function assistantText(message: PiMessageEnd["message"]): string {
  return Array.isArray(message.content)
    ? message.content.flatMap((part) =>
      part.type === "text" && typeof part.text === "string" ? [part.text] : []
    ).join("\n")
    : "";
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
  const parent = { ...options.parent };
  const invocation = {
    command: options.invocation.command,
    arguments: [...(options.invocation.arguments ?? [])],
  };
  const authentication = {
    provider: options.authentication.provider,
    environment: { ...options.authentication.environment },
  };

  return {
    async run(task, runOptions = {}) {
      if (runOptions.signal?.aborted) {
        return { status: "cancelled", child: null, phase: "before-spawn" };
      }
      const parentError = await invalidParentContext(parent, authentication);
      if (parentError) {
        return {
          status: "failed",
          child: null,
          failure: { kind: "invalid-parent-context", message: parentError },
        };
      }

      const runId = randomUUID();
      return await superviseChild({
        command: invocation.command,
        arguments: [
          ...invocation.arguments,
          "--mode", "rpc",
          "--no-session",
          "--session-id", runId,
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
        signal: runOptions.signal,
        onProgress: runOptions.onProgress,
      });
    },
  };
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
  signal: AbortSignal | undefined;
  onProgress: ((progress: DelegatedTaskProgress) => void) | undefined;
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
    const transcriptEntries: Readonly<Record<string, unknown>>[] = [];
    let sequence = 0;
    let promptAccepted = false;
    let agentSettled = false;
    let protocolFailure: string | undefined;
    let cancellationRequested = false;
    let closed = false;
    let resolved = false;
    let terminationTimer: NodeJS.Timeout | undefined;
    let spawned = false;

    const settle = (outcome: DelegatedTaskOutcome): void => {
      if (resolved) return;
      resolved = true;
      if (terminationTimer) clearTimeout(terminationTimer);
      options.signal?.removeEventListener("abort", cancel);
      resolve(withTranscript(outcome, transcriptEntries));
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

      if (event.type === "response") {
        if (
          promptAccepted || event.id !== options.promptId || event.command !== "prompt" ||
          typeof event.success !== "boolean"
        ) {
          terminate("Pi emitted an uncorrelated prompt response");
          return;
        }
        if (!event.success) {
          terminate("Pi rejected the delegated prompt");
          return;
        }
        promptAccepted = true;
        if (identity) {
          emitProgress(options.onProgress, { type: "identified", child: identity });
        }
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
      if (TRANSCRIPT_TYPES.has(event.type)) {
        transcriptEntries.push(event);
      }
      if (isMessageEnd(event) && event.message.role === "assistant") {
        finalMessage = event.message;
      }
      if (activity.recognized && activity.valid && identity) {
        sequence += 1;
        emitProgress(options.onProgress, {
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
        emitProgress(options.onProgress, {
          type: "terminating", child: identity, signal: "SIGTERM",
        });
      }
      child.stdin.destroy();
      child.kill("SIGTERM");
      terminationTimer ??= setTimeout(() => {
        if (closed) return;
        if (identity) {
          emitProgress(options.onProgress, {
            type: "killing", child: identity, signal: "SIGKILL",
          });
        }
        child.kill("SIGKILL");
      }, options.graceMs);
    };

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
    child.stderr.resume();

    child.once("spawn", () => {
      spawned = true;
      if (child.pid === undefined) {
        terminate("Pi spawned without an observable PID");
        return;
      }
      identity = { runId: options.runId, pid: child.pid };
      emitProgress(options.onProgress, { type: "started", child: { pid: child.pid } });
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
