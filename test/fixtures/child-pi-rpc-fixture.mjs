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
let handled = false;

function emit(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`);
}

process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => {
  input += chunk;
  const newline = input.indexOf("\n");
  if (newline === -1 || handled) return;
  handled = true;
  const line = input.slice(0, newline);
  const command = JSON.parse(line);
  void run(command, input[newline] === "\n");
});

async function run(command, promptTerminatedByLf) {
  const task = command.message;
  if (task === "malformed-json") {
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
    process.on("SIGTERM", () => {});
    setInterval(() => {}, 1_000);
    return;
  }
  if (
    task === "hold" || task?.startsWith("Explorer assignment:\nhold\n") ||
    task?.startsWith("Worker assignment:\nhold\n")
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
  if (task === "delta-only-message-update") {
    emit({
      type: "message_update",
      usage: {},
      assistantMessageEvent: { type: "text_delta", contentIndex: 0, delta: "partial" },
    });
  }
  if (task === "malformed-known-event") {
    emit({ type: "queue_update", steering: "not-an-array", followUp: [] });
  }
  const observed = task?.startsWith("Designer assignment:\nsuccess\n")
    ? { summary: "validated designer result", evidence: ["fixture evidence"] }
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
        text: JSON.stringify(observed),
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
