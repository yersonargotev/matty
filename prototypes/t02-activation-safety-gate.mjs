// THROWAWAY PROTOTYPE: validates the Pi 0.83.0 input-ordering contract for T02.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import {
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const pi = join(root, "node_modules", ".bin", "pi");
const sandbox = await mkdtemp(join(tmpdir(), "matty-t02-prototype-"));
const home = join(sandbox, "home");
const project = join(sandbox, "project");
const skill = join(home, ".pi", "agent", "skills", "ask-matt");
const extension = join(sandbox, "gate.ts");
const providerCapture = join(sandbox, "provider-context.json");
const marker = "EXTERNAL_COLLISION_BODY_MUST_NOT_REACH_MODEL";
const piAi = pathToFileURL(
  join(
    root,
    "node_modules",
    "@earendil-works",
    "pi-coding-agent",
    "node_modules",
    "@earendil-works",
    "pi-ai",
    "dist",
    "index.js",
  ),
).href;

await mkdir(skill, { recursive: true });
await mkdir(project, { recursive: true });
await writeFile(
  join(skill, "SKILL.md"),
  `---\nname: ask-matt\ndescription: ${marker}\n---\n${marker}\n`,
);
await writeFile(
  extension,
  `
import { writeFileSync } from "node:fs";
import { createAssistantMessageEventStream } from ${JSON.stringify(piAi)};

export default function gate(pi) {
  const reserved = "skill:ask-matt";
  const unsafe = () => pi.getCommands().some(
    (command) => command.name === reserved && command.source === "skill"
  );

  pi.on("session_start", (_event, ctx) => {
    if (unsafe()) ctx.ui.notify("PROTOTYPE_DEGRADED_EXTERNAL_RESERVED_SKILL", "warning");
  });

  pi.on("input", (_event, ctx) => {
    if (!unsafe()) return { action: "continue" };
    ctx.ui.notify("PROTOTYPE_BLOCKED_BEFORE_MODEL", "error");
    return { action: "handled" };
  });

  pi.on("before_agent_start", () => {
    throw new Error("prototype failure: model-bound flow was reached");
  });

  pi.on("before_provider_request", (event) => {
    if (JSON.stringify(event.payload).includes(${JSON.stringify(marker)})) {
      throw new Error("prototype failure: external collision reached provider payload");
    }
    throw new Error("prototype failure: provider boundary was reached");
  });

  pi.registerCommand("pi-trigger", {
    description: "Bypass the input event with a supported Pi API",
    handler: async () => {
      pi.sendMessage({
        customType: "t02-trigger",
        content: "T02_EXTENSION_TRIGGER",
        display: false,
      }, { triggerTurn: true });
    },
  });

  pi.registerProvider("t02-direct", {
    name: "T02 direct stream provider",
    baseUrl: "http://127.0.0.1/unused",
    api: "openai-completions",
    apiKey: "acceptance-only",
    models: [{
      id: "direct",
      name: "T02 direct stream",
      reasoning: false,
      input: ["text"],
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      contextWindow: 8192,
      maxTokens: 128,
    }],
    streamSimple(model, context) {
      writeFileSync(${JSON.stringify(providerCapture)}, JSON.stringify(context), "utf8");
      const stream = createAssistantMessageEventStream();
      queueMicrotask(() => stream.end({
        role: "assistant",
        content: [{ type: "text", text: "ok" }],
        api: model.api,
        provider: model.provider,
        model: model.id,
        usage: {
          input: 0,
          output: 0,
          cacheRead: 0,
          cacheWrite: 0,
          totalTokens: 0,
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
        },
        stopReason: "stop",
        timestamp: Date.now(),
      }));
      return stream;
    },
  });
}
`,
);

const child = spawn(
  pi,
  ["--mode", "rpc", "--no-session", "--no-extensions", "-e", extension],
  {
    cwd: project,
    env: {
      PATH: process.env.PATH,
      HOME: home,
      XDG_CONFIG_HOME: join(home, ".config"),
      PI_OFFLINE: "1",
      NO_UPDATE_NOTIFIER: "1",
    },
    stdio: ["pipe", "pipe", "pipe"],
  },
);

const events = [];
let stdout = "";
let stderr = "";
child.stdout.setEncoding("utf8");
child.stderr.setEncoding("utf8");
child.stderr.on("data", (chunk) => {
  stderr += chunk;
});
child.stdout.on("data", (chunk) => {
  stdout += chunk;
  const lines = stdout.split("\n");
  stdout = lines.pop() ?? "";
  for (const line of lines) {
    if (line.trim()) events.push(JSON.parse(line));
  }
});

async function waitFor(predicate) {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    const event = events.find(predicate);
    if (event) return event;
    await new Promise((resolveWait) => setTimeout(resolveWait, 50));
  }
  throw new Error(`timed out waiting for Pi RPC event\n${stderr}`);
}

try {
  await waitFor(
    (event) =>
      event.type === "extension_ui_request" &&
      event.message === "PROTOTYPE_DEGRADED_EXTERNAL_RESERVED_SKILL",
  );
  child.stdin.write(
    `${JSON.stringify({ id: "commands", type: "get_commands" })}\n`,
  );
  const commands = await waitFor(
    (event) => event.type === "response" && event.id === "commands",
  );
  assert.ok(
    commands.data.commands.some(
      (command) =>
        command.name === "skill:ask-matt" && command.source === "skill",
    ),
  );

  child.stdin.write(
    `${JSON.stringify({
      id: "unsafe-prompt",
      type: "prompt",
      message: "/skill:ask-matt run",
    })}\n`,
  );
  await waitFor(
    (event) =>
      event.type === "extension_ui_request" &&
      event.message === "PROTOTYPE_BLOCKED_BEFORE_MODEL",
  );
  await new Promise((resolveWait) => setTimeout(resolveWait, 250));
  assert.equal(
    events.some(
      (event) =>
        event.type === "agent_start" ||
        event.type === "agent_end" ||
        event.type === "message_start",
    ),
    false,
  );

  child.stdin.write(
    `${JSON.stringify({
      id: "model",
      type: "set_model",
      provider: "t02-direct",
      modelId: "direct",
    })}\n`,
  );
  const model = await waitFor(
    (event) => event.type === "response" && event.id === "model",
  );
  assert.equal(model.success, true);
  child.stdin.write(
    `${JSON.stringify({
      id: "bypass",
      type: "prompt",
      message: "/pi-trigger",
    })}\n`,
  );
  await waitFor((event) => event.type === "agent_end");
  const capturedContext = await readFile(providerCapture, "utf8");
  assert.match(capturedContext, new RegExp(marker));
  assert.match(capturedContext, /<name>ask-matt<\/name>/);

  process.stdout.write(
    [
      "RELEASE BLOCKED: direct input is intercepted, but",
      "sendMessage(triggerTurn) plus custom streamSimple bypasses input,",
      "before_agent_start, and before_provider_request with the external skill in context.",
      "",
    ].join(" "),
  );
} finally {
  child.stdin.end();
  await new Promise((resolveClose) => child.once("close", resolveClose));
  await rm(sandbox, { recursive: true, force: true });
}
