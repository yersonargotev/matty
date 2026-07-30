import { createHash } from "node:crypto";

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
  task?.startsWith("Explorer assignment:\nhold\n")
) {
  setInterval(() => {}, 1_000);
} else {
  const failed = task === "failure";
  if (task === "tool-progress") {
    process.stdout.write(
      `${JSON.stringify({
        type: "tool_execution_end",
        toolCallId: "tool-1",
        toolName: "read",
        isError: false,
      })}\n`,
    );
    process.stdout.write(
      `${JSON.stringify({
        type: "tool_execution_end",
        toolCallId: "tool-2",
        toolName: "bash",
        isError: false,
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
