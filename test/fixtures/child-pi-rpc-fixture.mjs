import { createHash } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname } from "node:path";

function argument(name) {
  const index = process.argv.indexOf(name);
  return index === -1 ? undefined : process.argv[index + 1];
}

const provider = argument("--provider");
const model = argument("--model");
const thinking = argument("--thinking");
const authDigest = createHash("sha256")
  .update(process.env.MATTY_TEST_AUTH ?? "")
  .digest("hex");
let input = "";
let interactive = false;
let holdInteractions = false;

function emit(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`);
}

function terminal(text) {
  emit({
    type: "message_end",
    message: {
      role: "assistant",
      content: [{ type: "text", text }],
      stopReason: "stop",
    },
  });
}

function handle(command, promptTerminatedByLf) {
  if (command.type === "prompt") {
    void run(command, promptTerminatedByLf);
    return;
  }
  if (interactive && (command.type === "steer" || command.type === "follow_up")) {
    if (holdInteractions) return;
    emit({ id: command.id, type: "response", command: command.type, success: true });
    emit({ type: "queue_update", steering: [], followUp: [] });
    terminal(command.message.includes("invalid")
      ? "not structured JSON"
      : JSON.stringify({ summary: "replacement candidate", evidence: [command.type] }));
    if (command.message.startsWith("finish")) {
      emit({ type: "agent_settled" });
      process.stdin.unref();
    }
  }
}

process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => {
  input += chunk;
  while (true) {
    const newline = input.indexOf("\n");
    if (newline === -1) return;
    const line = input.slice(0, newline);
    input = input.slice(newline + 1);
    handle(JSON.parse(line), true);
  }
});

async function run(command, promptTerminatedByLf) {
  const task = command.message;
  if (task === "malformed-json" || task?.includes("assignment:\nmalformed-json\n")) {
    process.stdout.write("not-json\n");
    return;
  }
  if (task === "uncorrelated-response") {
    emit({ id: "wrong-request", type: "response", command: "prompt", success: true });
    return;
  }
  if (task === "pre-response-event") {
    emit({ type: "agent_start" });
  }
  if (task === "pre-response-extension-notification") {
    emit({ type: "extension_ui_request", method: "setStatus", params: { text: "starting" } });
  }
  if (task === "ignore-term") {
    process.on("SIGTERM", () => {});
  }
  if (task === "crlf-output") {
    process.stdout.write(`${JSON.stringify({
      id: command.id,
      type: "response",
      command: "prompt",
      success: true,
    })}\r\n`);
  } else {
    emit({ id: command.id, type: "response", command: "prompt", success: true });
  }

  if (task === "oversized-frame" || task?.includes("assignment:\noversized-frame\n")) {
    process.stdout.write(`${JSON.stringify({ type: "unknown", payload: "x".repeat(4 * 1024 * 1024) })}\n`);
    return;
  }
  if (task === "stderr-overflow" || task?.includes("assignment:\nstderr-overflow\n")) {
    process.stderr.write("x".repeat(65 * 1024));
  }
  if (task?.startsWith("Designer assignment:\ninteractive-candidate\n")) {
    interactive = true;
    terminal(JSON.stringify({ summary: "initial candidate", evidence: ["initial"] }));
    return;
  }
  if (task === "interactive-backpressure") {
    interactive = true;
    holdInteractions = true;
    terminal("waiting for bounded interactions");
    process.stdin.pause();
    return;
  }
  if (task === "malformed-message") {
    emit({ type: "message_end", message: { role: "assistant", content: {} } });
    return;
  }
  const malformedAssistantParts = {
    "unknown-assistant-part": { type: "unknown" },
    "assistant-text-without-text": { type: "text" },
    "malformed-thinking-part": { type: "thinking" },
    "malformed-tool-call-part": { type: "toolCall", id: "call-1", name: "read" },
  };
  if (task in malformedAssistantParts) {
    emit({
      type: "message_end",
      message: {
        role: "assistant",
        content: [malformedAssistantParts[task]],
        stopReason: "stop",
      },
    });
    emit({ type: "agent_settled" });
    return;
  }
  if (task === "ignore-term") {
    setInterval(() => {}, 1_000);
    return;
  }
  if (
    task === "hold" || task?.includes("assignment:\nhold\n")
  ) {
    setInterval(() => {}, 1_000);
    return;
  }

  const failed = task === "failure" || task === "aborted-assistant" ||
    task?.startsWith("Explorer assignment:\nfailure\n");
  if (task?.startsWith("Researcher assignment:\n")) {
    const scope = JSON.parse(process.env.MATTY_RESEARCH_SCOPE ?? "null");
    await mkdir(dirname(scope.report), { recursive: true });
    await writeFile(scope.report, "# Fixture Research Report\n", { flag: "wx" });
  }
  if (task === "tool-progress" || task === "sensitive-activity") {
    emit({
      type: "tool_execution_end",
      toolCallId: "secret-tool-call-id",
      toolName: "read",
      args: { path: "/secret/private/path" },
      result: { content: "secret raw tool result" },
      command: "cat /secret/private/path",
      unknownSensitiveField: "secret prompt and transcript",
      isError: false,
    });
    emit({
      type: "tool_execution_end",
      toolCallId: "another-secret-id",
      toolName: task === "sensitive-activity" ? "valid_unknown_tool" : "bash",
      result: "secret response",
      isError: task === "sensitive-activity",
    });
  }
  if (task === "malformed-tool-activity") {
    emit({
      type: "tool_execution_end",
      toolCallId: "secret-id",
      toolName: "read",
      result: "secret result",
      isError: "false",
    });
  }
  if (task === "custom-message-content") {
    emit({
      type: "message_end",
      message: {
        role: "custom",
        customType: "web-search-content-ready",
        content: "search content",
        display: true,
      },
    });
    emit({
      type: "message_end",
      message: { role: "user", content: [{ type: "text", text: "user prompt" }] },
    });
    emit({
      type: "message_end",
      message: {
        role: "toolResult",
        toolCallId: "call-1",
        toolName: "read",
        content: [{ type: "text", text: "tool output" }],
        isError: false,
      },
    });
  }
  let observedOutput;
  if (task === "delta-only-message-update") {
    emit({
      type: "message_update",
      usage: {},
      assistantMessageEvent: { type: "text_delta", contentIndex: 0, delta: "partial" },
    });
  }
  if (task === "presentation-accounting-overflow") {
    for (const toolCallId of ["overflow-1", "overflow-2", "overflow-3"]) {
      emit({
        type: "tool_execution_update",
        toolCallId,
        toolName: "read",
        args: {},
        partialResult: "x".repeat(2 * 1024 * 1024),
      });
    }
    emit({ type: "message_start", message: { role: "assistant", content: [] } });
    emit({
      type: "message_update",
      usage: {},
      assistantMessageEvent: {
        type: "text_delta",
        contentIndex: 0,
        delta: "y".repeat(2 * 1024 * 1024 + 256 * 1024),
      },
    });
    return;
  }
  const liveMarker = task?.includes("interleaved-live-updates-A") ? "A"
    : task?.includes("interleaved-live-updates-B") ? "B"
    : task === "interleaved-live-updates" || task?.startsWith("Designer assignment:\ninterleaved-live-updates\n") ? "base"
    : undefined;
  if (liveMarker) {
    emit({ type: "message_update", usage: {}, assistantMessageEvent: { type: "thinking_delta", contentIndex: 2, delta: `${liveMarker}-thinking` } });
    emit({ type: "message_update", usage: {}, assistantMessageEvent: { type: "text_delta", contentIndex: 1, delta: `${liveMarker}-second` } });
    emit({ type: "message_update", usage: {}, assistantMessageEvent: { type: "text_delta", contentIndex: 0, delta: `${liveMarker}-first-` } });
    emit({ type: "message_update", usage: {}, assistantMessageEvent: { type: "text_delta", contentIndex: 0, delta: "done" } });
    emit({ type: "tool_execution_update", toolCallId: `call-live-${liveMarker}`, toolName: "read", args: {}, partialResult: { content: `${liveMarker}-old` } });
    emit({ type: "tool_execution_update", toolCallId: `call-live-${liveMarker}`, toolName: "read", args: {}, partialResult: { content: `${liveMarker}-new` } });
    emit({
      type: "extension_error",
      extensionPath: "fixture-extension",
      event: "fixture-event",
      error: `${liveMarker}-extension-error`,
    });
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  if (task === "ansi-output") {
    observedOutput = "\u001b]0;owned\u0007\u001b[31msafe\u001b[0m\u0000";
  }
  if (task === "malformed-known-event") {
    emit({ type: "queue_update", steering: "not-an-array", followUp: [] });
  }
  const observed = task?.startsWith("Designer assignment:\nsuccess\n") || liveMarker
    ? { summary: `validated designer result${liveMarker ? ` ${liveMarker}` : ""}`, evidence: ["fixture evidence"] }
    : {
      pid: process.pid,
      ppid: process.ppid,
      provider,
      model,
      thinking,
      cwd: process.cwd(),
      authDigest,
      modeArguments: process.argv.filter((value, index, values) =>
        value === "--mode" || values[index - 1] === "--mode"
      ),
      promptTerminatedByLf,
    };
  emit({
    type: "message_end",
    message: {
      role: "assistant",
      content: task === "valid-assistant-parts" ? [
        { type: "thinking", thinking: "considering fixture output" },
        { type: "toolCall", id: "call-1", name: "read", arguments: { path: "fixture" } },
        { type: "text", text: JSON.stringify(observed) },
      ] : [{
        type: "text",
        text: observedOutput ?? JSON.stringify(observed),
      }],
      provider,
      model,
      ...(task === "absent-assistant-stop-reason" ? {} : {
        stopReason: task === "malformed-assistant-activity" ? "unknown"
          : task === "deferred-assistant" ? "deferred"
          : task === "length-assistant" ? "length"
          : task === "tool-use-settled" ? "toolUse"
          : task === "aborted-assistant" ? "aborted"
          : failed ? "error" : "stop",
      }),
      errorMessage: failed ? "controlled failure" : undefined,
      usage: {
        input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2,
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
      },
    },
  });
  if (task === "ignore-settled-exit") {
    process.on("SIGTERM", () => {});
    setInterval(() => {}, 1_000);
  }
  if (task !== "no-settlement") emit({ type: "agent_settled" });
  if (task === "post-settlement-message") {
    emit({
      type: "message_end",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "late replacement" }],
        stopReason: "stop",
      },
    });
  }
  process.exitCode = failed ? 1 : 0;
  if (task === "no-settlement") process.stdin.unref();
}
