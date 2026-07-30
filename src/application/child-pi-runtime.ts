// Process-launch and JSONL behavior adapted from Pi's subagent example:
// https://github.com/earendil-works/pi-mono/tree/845d6ff1f6643aba440341cce877ce1c43ebbc39/packages/coding-agent/examples/extensions/subagent
// Pi is MIT licensed. Matty owns this adapted runtime and its invariants.
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { realpath } from "node:fs/promises";
import { isAbsolute } from "node:path";

export type PiThinkingLevel =
  | "off"
  | "minimal"
  | "low"
  | "medium"
  | "high"
  | "xhigh";

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

export type DelegatedTaskProgress =
  | {
      type: "started";
      child: Pick<ChildIdentity, "pid">;
    }
  | {
      type: "identified";
      child: ChildIdentity;
    }
  | {
      type: "message" | "tool-result";
      child: ChildIdentity;
      sequence: number;
    }
  | {
      type: "terminating";
      child: ChildIdentity;
      signal: "SIGTERM";
    }
  | {
      type: "killing";
      child: ChildIdentity;
      signal: "SIGKILL";
    };

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

interface PiSessionHeader {
  type: "session";
  id: string;
  cwd: string;
}

interface PiMessageEnd {
  type: "message_end";
  message?: {
    role?: string;
    content?: Array<{ type?: string; text?: string }>;
    stopReason?: string;
    errorMessage?: string;
  };
}

function isSessionHeader(value: unknown): value is PiSessionHeader {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Partial<PiSessionHeader>;
  return (
    candidate.type === "session" &&
    typeof candidate.id === "string" &&
    typeof candidate.cwd === "string"
  );
}

function isMessageEnd(value: unknown): value is PiMessageEnd {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Partial<PiMessageEnd>;
  if (candidate.type !== "message_end") {
    return false;
  }
  if (
    typeof candidate.message !== "object" ||
    candidate.message === null ||
    Array.isArray(candidate.message) ||
    typeof candidate.message.role !== "string" ||
    !Array.isArray(candidate.message.content)
  ) {
    return false;
  }
  return candidate.message.content.every(
    (part) =>
      typeof part === "object" &&
      part !== null &&
      typeof part.type === "string" &&
      (part.text === undefined || typeof part.text === "string"),
  );
}

function assistantText(message: PiMessageEnd["message"]): string {
  const content = Array.isArray(message?.content) ? message.content : [];
  return content
    .filter(
      (part): part is { type: "text"; text: string } =>
        part.type === "text" && typeof part.text === "string",
    )
    .map((part) => part.text)
    .join("\n");
}

async function invalidParentContext(
  parent: ParentPiExecutionContext,
  authentication: ChildSafePiAuthentication,
): Promise<string | undefined> {
  if (!parent.provider.trim()) {
    return "The parent provider is unavailable";
  }
  if (!parent.model.trim()) {
    return "The parent model is unavailable";
  }
  if (!isAbsolute(parent.cwd)) {
    return "The parent working directory must be absolute";
  }
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

export function createChildPiRunner(
  runnerOptions: ChildPiRunnerOptions,
): DelegatedTaskRunner {
  const graceMs = runnerOptions.terminationGraceMs ?? 5_000;
  const parent = { ...runnerOptions.parent };
  const invocation = {
    command: runnerOptions.invocation.command,
    arguments: [...(runnerOptions.invocation.arguments ?? [])],
  };
  const authentication = {
    provider: runnerOptions.authentication.provider,
    environment: { ...runnerOptions.authentication.environment },
  };

  return {
    async run(task, options = {}) {
      if (options.signal?.aborted) {
        return {
          status: "cancelled",
          child: null,
          phase: "before-spawn",
        };
      }

      const parentError = await invalidParentContext(parent, authentication);
      if (parentError) {
        return {
          status: "failed",
          child: null,
          failure: {
            kind: "invalid-parent-context",
            message: parentError,
          },
        };
      }

      const runId = randomUUID();
      const invocationArguments = [
        ...invocation.arguments,
        "--mode",
        "json",
        "-p",
        "--no-session",
        "--session-id",
        runId,
        "--provider",
        parent.provider,
        "--model",
        parent.model,
        "--thinking",
        parent.thinking,
        task,
      ];

      return await superviseChild({
        command: invocation.command,
        arguments: invocationArguments,
        cwd: parent.cwd,
        environment: authentication.environment,
        expectedRunId: runId,
        graceMs,
        signal: options.signal,
        onProgress: options.onProgress,
      });
    },
  };
}

interface SupervisionOptions {
  command: string;
  arguments: string[];
  cwd: string;
  environment: NodeJS.ProcessEnv;
  expectedRunId: string;
  graceMs: number;
  signal: AbortSignal | undefined;
  onProgress:
    | ((progress: DelegatedTaskProgress) => void)
    | undefined;
}

async function superviseChild(
  options: SupervisionOptions,
): Promise<DelegatedTaskOutcome> {
  let child;
  try {
    child = spawn(options.command, options.arguments, {
      cwd: options.cwd,
      env: options.environment,
      shell: false,
      stdio: ["ignore", "pipe", "pipe"],
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
    let finalMessage: PiMessageEnd["message"];
    let sequence = 0;
    let protocolFailure: string | undefined;
    let cancellationRequested = false;
    let closed = false;
    let settled = false;
    let terminationTimer: NodeJS.Timeout | undefined;
    let spawned = false;
    let processControlFailureHandled = false;

    const settle = (outcome: DelegatedTaskOutcome): void => {
      if (settled) {
        return;
      }
      settled = true;
      if (terminationTimer) {
        clearTimeout(terminationTimer);
      }
      options.signal?.removeEventListener("abort", cancel);
      resolve(outcome);
    };

    const terminateForProtocolFailure = (message: string): void => {
      protocolFailure ??= message;
      if (!closed) {
        child.kill("SIGTERM");
        terminationTimer ??= setTimeout(() => {
          if (!closed) {
            child.kill("SIGKILL");
          }
        }, options.graceMs);
      }
    };

    const consumeLine = (line: string): void => {
      if (!line.trim() || protocolFailure) {
        return;
      }

      let event: unknown;
      try {
        event = JSON.parse(line);
      } catch {
        terminateForProtocolFailure("Pi emitted invalid JSONL");
        return;
      }

      if (isSessionHeader(event)) {
        if (
          identity ||
          event.id !== options.expectedRunId ||
          event.cwd !== options.cwd ||
          child.pid === undefined
        ) {
          terminateForProtocolFailure(
            "Pi emitted a mismatched session header",
          );
          return;
        }
        identity = {
          runId: event.id,
          pid: child.pid,
        };
        emitProgress(options.onProgress, {
          type: "identified",
          child: identity,
        });
        return;
      }

      if (!identity) {
        terminateForProtocolFailure(
          "Pi emitted an event before its session header",
        );
        return;
      }

      if (
        typeof event === "object" &&
        event !== null &&
        (event as { type?: unknown }).type === "message_end" &&
        !isMessageEnd(event)
      ) {
        terminateForProtocolFailure(
          "Pi emitted a malformed message_end event",
        );
        return;
      }

      if (isMessageEnd(event) && event.message?.role === "assistant") {
        finalMessage = event.message;
        sequence += 1;
        emitProgress(options.onProgress, {
          type: "message",
          child: identity,
          sequence,
        });
        return;
      }

      if (
        typeof event === "object" &&
        event !== null &&
        (event as { type?: string }).type === "tool_result_end" &&
        identity
      ) {
        sequence += 1;
        emitProgress(options.onProgress, {
          type: "tool-result",
          child: identity,
          sequence,
        });
      }
    };

    const cancel = (): void => {
      if (cancellationRequested || closed) {
        return;
      }
      cancellationRequested = true;
      if (identity) {
        emitProgress(options.onProgress, {
          type: "terminating",
          child: identity,
          signal: "SIGTERM",
        });
      }
      child.kill("SIGTERM");
      terminationTimer ??= setTimeout(() => {
        if (closed) {
          return;
        }
        if (identity) {
          emitProgress(options.onProgress, {
            type: "killing",
            child: identity,
            signal: "SIGKILL",
          });
        }
        child.kill("SIGKILL");
      }, options.graceMs);
    };

    child.stdout.setEncoding("utf8");
    child.stdout.on("data", (chunk: string) => {
      stdout += chunk;
      const lines = stdout.split("\n");
      stdout = lines.pop() ?? "";
      for (const line of lines) {
        consumeLine(line);
      }
    });
    child.stderr.resume();

    child.once("spawn", () => {
      spawned = true;
      if (child.pid === undefined) {
        terminateForProtocolFailure("Pi spawned without an observable PID");
        return;
      }
      emitProgress(options.onProgress, {
        type: "started",
        child: { pid: child.pid },
      });
      if (options.signal?.aborted) {
        cancel();
      }
    });

    child.on("error", (error) => {
      if (settled) {
        return;
      }
      if (!spawned) {
        settle({
          status: "failed",
          child: null,
          failure: {
            kind: "spawn-failed",
            message: error.message,
          },
        });
        return;
      }

      if (processControlFailureHandled) {
        return;
      }
      processControlFailureHandled = true;
      terminateForProtocolFailure(
        `Pi child process control failed: ${error.message}`,
      );
    });

    child.once("close", (code, signal) => {
      closed = true;
      if (stdout.trim()) {
        consumeLine(stdout);
      }
      const exit = { code, signal };

      if (cancellationRequested) {
        settle({
          status: "cancelled",
          child: identity,
          phase: "running",
          exit,
        });
        return;
      }

      if (protocolFailure) {
        settle({
          status: "failed",
          child: identity,
          failure: {
            kind: "protocol-failed",
            message: protocolFailure,
          },
          exit,
        });
        return;
      }

      if (!identity) {
        settle({
          status: "failed",
          child: null,
          failure: {
            kind: "protocol-failed",
            message: "Pi exited before confirming its session identity",
          },
          exit,
        });
        return;
      }

      if (
        code !== 0 ||
        signal !== null ||
        finalMessage?.stopReason === "error" ||
        finalMessage?.stopReason === "aborted"
      ) {
        settle({
          status: "failed",
          child: identity,
          failure: {
            kind: "child-failed",
            message:
              finalMessage?.errorMessage ??
              `Pi child exited with code ${String(code)}`,
          },
          exit,
        });
        return;
      }

      if (!finalMessage) {
        settle({
          status: "failed",
          child: identity,
          failure: {
            kind: "child-exited",
            message: "Pi child completed without an assistant result",
          },
          exit,
        });
        return;
      }

      settle({
        status: "succeeded",
        child: identity,
        output: assistantText(finalMessage),
        exit: {
          code: 0,
          signal: null,
        },
      });
    });

    options.signal?.addEventListener("abort", cancel, { once: true });
  });
}
