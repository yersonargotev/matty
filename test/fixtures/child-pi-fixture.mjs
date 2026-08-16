import { createHash } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname } from "node:path";

function argument(name) {
  const index = process.argv.indexOf(name);
  return index === -1 ? undefined : process.argv[index + 1];
}

const task = process.argv.at(-1);
const runId = argument("--session-id");
const provider = argument("--provider");
const model = argument("--model");
const thinking = argument("--thinking");
const authDigest = createHash("sha256")
  .update(process.env.MATTY_TEST_AUTH ?? "")
  .digest("hex");

if (task === "message-before-header") {
  process.stdout.write(`${JSON.stringify({ type: "turn_start" })}\n`);
}

process.stdout.write(
  `${JSON.stringify({
    type: "session",
    version: 3,
    id: runId,
    cwd: process.cwd(),
  })}\n`,
);

if (task === "malformed-message") {
  process.stdout.write(
    `${JSON.stringify({
      type: "message_end",
      message: { role: "assistant", content: {} },
    })}\n`,
  );
} else if (task === "ignore-term") {
  process.on("SIGTERM", () => {});
  setInterval(() => {}, 1_000);
} else if (
  task === "hold" ||
  task?.startsWith("Explorer assignment:\nhold\n") ||
  task?.startsWith("Worker assignment:\nhold\n")
) {
  setInterval(() => {}, 1_000);
} else {
  const failed = task === "failure" ||
    task?.startsWith("Explorer assignment:\nfailure\n");
  if (task?.startsWith("Researcher assignment:\n")) {
    const scope = JSON.parse(process.env.MATTY_RESEARCH_SCOPE ?? "null");
    await mkdir(dirname(scope.report), { recursive: true });
    await writeFile(scope.report, "# Fixture Research Report\n", {
      flag: "wx",
    });
  }
  if (task === "tool-progress" || task === "sensitive-activity") {
    process.stdout.write(
      `${JSON.stringify({
        type: "tool_execution_end",
        toolCallId: "secret-tool-call-id",
        toolName: "read",
        args: { path: "/secret/private/path" },
        result: { content: "secret raw tool result" },
        command: "cat /secret/private/path",
        unknownSensitiveField: "secret prompt and transcript",
        isError: false,
      })}\n`,
    );
    process.stdout.write(
      `${JSON.stringify({
        type: "tool_execution_end",
        toolCallId: "another-secret-id",
        toolName: task === "sensitive-activity" ? "valid_unknown_tool" : "bash",
        result: "secret response",
        isError: task === "sensitive-activity",
      })}\n`,
    );
  }
  if (task === "malformed-tool-activity") {
    process.stdout.write(
      `${JSON.stringify({
        type: "tool_execution_end",
        toolCallId: "secret-id",
        toolName: "read",
        result: "secret result",
        isError: "false",
      })}\n`,
    );
  }
  if (task === "custom-message-content") {
    process.stdout.write(
      `${JSON.stringify({
        type: "message_end",
        message: {
          role: "custom",
          customType: "web-search-content-ready",
          content: "search content",
          display: true,
        },
      })}\n`,
    );
  }
  process.stdout.write(
    `${JSON.stringify({
      type: "message_end",
      message: {
        role: "assistant",
        content: [
          {
            type: "text",
            text: JSON.stringify({
              pid: process.pid,
              ppid: process.ppid,
              provider,
              model,
              thinking,
              cwd: process.cwd(),
              authDigest,
            }),
          },
        ],
        provider,
        model,
        stopReason: failed ? "error" : "stop",
        errorMessage: failed ? "controlled failure" : undefined,
        usage: {
          input: 1,
          output: 1,
          cacheRead: 0,
          cacheWrite: 0,
          totalTokens: 2,
          cost: {
            input: 0,
            output: 0,
            cacheRead: 0,
            cacheWrite: 0,
            total: 0,
          },
        },
      },
    })}\n`,
  );
  process.exitCode = failed ? 1 : 0;
}
