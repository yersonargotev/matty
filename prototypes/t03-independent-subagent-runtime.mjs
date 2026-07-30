// THROWAWAY PROTOTYPE: validates the real Pi 0.83.0 child-process contract for T03.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdir, mkdtemp, realpath, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const pi = join(root, "node_modules", ".bin", "pi");
const sandbox = await mkdtemp(join(tmpdir(), "matty-t03-prototype-"));
const home = join(sandbox, "home");
const agentDir = join(home, ".pi", "agent");
const project = join(sandbox, "project");
const extension = join(sandbox, "t03-runtime.ts");
const authPath = join(agentDir, "auth.json");
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

await mkdir(agentDir, { recursive: true });
await mkdir(project, { recursive: true });
const canonicalProject = await realpath(project);
await writeFile(
  authPath,
  JSON.stringify({
    "t03-probe": { type: "api_key", key: "prototype-child-safe-auth" },
  }),
  { mode: 0o600 },
);
await writeFile(
  extension,
  `
import { spawn } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import { createAssistantMessageEventStream } from ${JSON.stringify(piAi)};

const childMode = process.env.MATTY_T03_CHILD === "1";
let effectiveThinking = "unknown";

function authDigest() {
  return createHash("sha256")
    .update(readFileSync(process.env.MATTY_T03_AUTH_PATH))
    .digest("hex");
}

function assistant(model, text, stopReason = "stop", errorMessage) {
  return {
    role: "assistant",
    content: [{ type: "text", text }],
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
    stopReason,
    errorMessage,
    timestamp: Date.now(),
  };
}

function finalText(message) {
  return (message?.content ?? [])
    .filter((part) => part.type === "text")
    .map((part) => part.text)
    .join("\\n");
}

function runChild(ctx, scenario) {
  const runId = randomUUID();
  const model = ctx.model;
  if (!model) {
    return Promise.resolve({
      status: "failed",
      child: null,
      failure: { kind: "parent-context-unavailable" },
    });
  }

  const expected = {
    provider: model.provider,
    model: model.id,
    thinking: ctx.thinkingLevel,
    cwd: ctx.cwd,
    authDigest: authDigest(),
  };
  const args = [
    "--mode", "json",
    "-p",
    "--no-session",
    "--session-id", runId,
    "--no-extensions",
    "-e", process.env.MATTY_T03_EXTENSION,
    "--provider", expected.provider,
    "--model", expected.model,
    "--thinking", expected.thinking,
    "T03_CHILD_" + scenario.toUpperCase(),
  ];
  const child = spawn(process.env.MATTY_T03_PI, args, {
    cwd: expected.cwd,
    env: {
      PATH: process.env.PATH,
      HOME: process.env.HOME,
      XDG_CONFIG_HOME: process.env.XDG_CONFIG_HOME,
      PI_CODING_AGENT_DIR: process.env.PI_CODING_AGENT_DIR,
      PI_OFFLINE: "1",
      NO_UPDATE_NOTIFIER: "1",
      MATTY_T03_CHILD: "1",
      MATTY_T03_AUTH_PATH: process.env.MATTY_T03_AUTH_PATH,
      MATTY_T03_EXTENSION: process.env.MATTY_T03_EXTENSION,
      MATTY_T03_PI: process.env.MATTY_T03_PI,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

  return new Promise((resolve) => {
    const childIdentity = { runId, pid: child.pid };
    let stdout = "";
    let stderr = "";
    let headerSeen = false;
    let finalMessage;
    let cancelRequested = false;
    let settled = false;

    ctx.ui.notify("T03_PROGRESS:" + JSON.stringify({
      type: "started",
      child: { pid: child.pid },
    }), "info");

    function settle(result) {
      if (settled) return;
      settled = true;
      resolve(result);
    }

    function consume(line) {
      if (!line.trim()) return;
      const event = JSON.parse(line);
      if (event.type === "session") {
        if (event.id !== runId || event.cwd !== expected.cwd) {
          child.kill("SIGTERM");
          settle({
            status: "failed",
            child: childIdentity,
            failure: { kind: "protocol-failed" },
          });
          return;
        }
        headerSeen = true;
        ctx.ui.notify("T03_PROGRESS:" + JSON.stringify({
          type: "identified",
          child: childIdentity,
        }), "info");
      }
      if (event.type === "message_end" && event.message?.role === "assistant") {
        finalMessage = event.message;
        ctx.ui.notify("T03_PROGRESS:" + JSON.stringify({
          type: "message",
          child: childIdentity,
          stopReason: event.message.stopReason,
        }), "info");
      }
    }

    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
      const lines = stdout.split("\\n");
      stdout = lines.pop() ?? "";
      for (const line of lines) consume(line);
    });
    child.stderr.on("data", (chunk) => {
      stderr = (stderr + chunk).slice(-4096);
    });
    child.on("error", () => {
      settle({
        status: "failed",
        child: null,
        failure: { kind: "spawn-failed" },
      });
    });
    child.on("close", (code, signal) => {
      if (stdout.trim()) consume(stdout);
      const exit = { code, signal };
      if (cancelRequested) {
        settle({ status: "cancelled", child: childIdentity, exit });
      } else if (!headerSeen) {
        settle({
          status: "failed",
          child: childIdentity,
          failure: { kind: "protocol-failed" },
          exit,
        });
      } else if (
        code !== 0 ||
        finalMessage?.stopReason === "error" ||
        finalMessage?.stopReason === "aborted"
      ) {
        settle({
          status: "failed",
          child: childIdentity,
          failure: { kind: "child-failed" },
          exit,
        });
      } else {
        settle({
          status: "succeeded",
          child: childIdentity,
          output: finalText(finalMessage),
          exit,
        });
      }
    });

    if (scenario === "cancel") {
      setTimeout(() => {
        cancelRequested = true;
        ctx.ui.notify("T03_PROGRESS:" + JSON.stringify({
          type: "terminating",
          child: childIdentity,
          signal: "SIGTERM",
        }), "info");
        child.kill("SIGTERM");
        setTimeout(() => {
          if (child.exitCode === null && child.signalCode === null) {
            ctx.ui.notify("T03_PROGRESS:" + JSON.stringify({
              type: "killing",
              child: childIdentity,
              signal: "SIGKILL",
            }), "info");
            child.kill("SIGKILL");
          }
        }, 500);
      }, 250);
    }
  });
}

export default function t03(pi) {
  pi.on("before_agent_start", (_event, ctx) => {
    effectiveThinking = ctx.thinkingLevel;
  });

  pi.registerProvider("t03-probe", {
    name: "T03 observable provider",
    baseUrl: "http://127.0.0.1/unused",
    api: "openai-completions",
    models: [{
      id: "observable",
      name: "T03 observable model",
      reasoning: true,
      input: ["text"],
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      contextWindow: 8192,
      maxTokens: 128,
    }],
    streamSimple(model, context) {
      const stream = createAssistantMessageEventStream();
      if (!childMode) {
        queueMicrotask(() => stream.end(assistant(model, "parent-ready")));
        return stream;
      }
      const prompt = JSON.stringify(context.messages);
      if (prompt.includes("T03_CHILD_CANCEL")) return stream;
      const payload = {
        pid: process.pid,
        ppid: process.ppid,
        provider: model.provider,
        model: model.id,
        thinking: effectiveThinking,
        cwd: process.cwd(),
        authDigest: authDigest(),
        containsParentOnlyContext: prompt.includes("PARENT_ONLY_CONTEXT"),
      };
      queueMicrotask(() => stream.end(
        prompt.includes("T03_CHILD_FAILURE")
          ? assistant(model, JSON.stringify(payload), "error", "prototype failure")
          : assistant(model, JSON.stringify(payload)),
      ));
      return stream;
    },
  });

  if (childMode) return;

  pi.registerCommand("t03-run", {
    description: "Run the T03 child-process prototype",
    handler: async (args, ctx) => {
      const scenario = args.trim();
      const result = await runChild(ctx, scenario);
      ctx.ui.notify("T03_RESULT:" + scenario + ":" + JSON.stringify(result), "info");
    },
  });
  pi.registerCommand("t03-ping", {
    description: "Prove the parent remains usable",
    handler: async (_args, ctx) => {
      ctx.ui.notify("T03_PARENT_OK:" + process.pid, "info");
    },
  });
}
`,
);

const child = spawn(
  pi,
  ["--mode", "rpc", "--no-session", "--no-extensions", "-e", extension],
  {
    cwd: canonicalProject,
    env: {
      PATH: process.env.PATH,
      HOME: home,
      XDG_CONFIG_HOME: join(home, ".config"),
      PI_CODING_AGENT_DIR: agentDir,
      PI_OFFLINE: "1",
      NO_UPDATE_NOTIFIER: "1",
      MATTY_T03_AUTH_PATH: authPath,
      MATTY_T03_EXTENSION: extension,
      MATTY_T03_PI: pi,
    },
    stdio: ["pipe", "pipe", "pipe"],
  },
);

const events = [];
let stdout = "";
let stderr = "";
child.stdout.setEncoding("utf8");
child.stderr.setEncoding("utf8");
child.stdout.on("data", (chunk) => {
  stdout += chunk;
  const lines = stdout.split("\n");
  stdout = lines.pop() ?? "";
  for (const line of lines) {
    if (line.trim()) events.push(JSON.parse(line));
  }
});
child.stderr.on("data", (chunk) => {
  stderr += chunk;
});

async function waitFor(predicate, from = 0) {
  for (let attempt = 0; attempt < 200; attempt += 1) {
    const event = events.slice(from).find(predicate);
    if (event) return event;
    await new Promise((resolveWait) => setTimeout(resolveWait, 50));
  }
  throw new Error(`timed out waiting for Pi RPC event\n${stderr}`);
}

function send(command) {
  child.stdin.write(`${JSON.stringify(command)}\n`);
}

async function runScenario(scenario) {
  const from = events.length;
  send({
    id: `run-${scenario}`,
    type: "prompt",
    message: `/t03-run ${scenario}`,
  });
  const event = await waitFor(
    (candidate) =>
      candidate.type === "extension_ui_request" &&
      candidate.message.startsWith(`T03_RESULT:${scenario}:`),
    from,
  );
  return {
    result: JSON.parse(event.message.slice(`T03_RESULT:${scenario}:`.length)),
    progress: events
      .slice(from)
      .filter(
        (candidate) =>
          candidate.type === "extension_ui_request" &&
          candidate.message.startsWith("T03_PROGRESS:"),
      )
      .map((candidate) =>
        JSON.parse(candidate.message.slice("T03_PROGRESS:".length)),
      ),
  };
}

try {
  send({
    id: "model",
    type: "set_model",
    provider: "t03-probe",
    modelId: "observable",
  });
  assert.equal(
    (await waitFor(
      (event) => event.type === "response" && event.id === "model",
    )).success,
    true,
  );
  send({ id: "thinking", type: "set_thinking_level", level: "high" });
  assert.equal(
    (await waitFor(
      (event) => event.type === "response" && event.id === "thinking",
    )).success,
    true,
  );

  send({
    id: "parent-context",
    type: "prompt",
    message: "PARENT_ONLY_CONTEXT",
  });
  await waitFor((event) => event.type === "agent_end");

  const success = await runScenario("success");
  assert.equal(success.result.status, "succeeded");
  assert.notEqual(success.result.child.pid, child.pid);
  assert.equal(success.result.child.runId.length, 36);
  assert.deepEqual(
    success.progress.slice(0, 2).map((event) => event.type),
    ["started", "identified"],
  );
  const observed = JSON.parse(success.result.output);
  assert.equal(observed.pid, success.result.child.pid);
  assert.equal(observed.ppid, child.pid);
  assert.equal(observed.provider, "t03-probe");
  assert.equal(observed.model, "observable");
  assert.equal(observed.thinking, "high");
  assert.equal(observed.cwd, canonicalProject);
  assert.equal(observed.containsParentOnlyContext, false);

  const failure = await runScenario("failure");
  assert.equal(failure.result.status, "failed");
  assert.equal(failure.result.failure.kind, "child-failed");

  const cancellation = await runScenario("cancel");
  assert.equal(cancellation.result.status, "cancelled");
  assert.ok(
    cancellation.progress.some((event) => event.type === "terminating"),
  );

  const pingFrom = events.length;
  send({ id: "ping", type: "prompt", message: "/t03-ping" });
  const ping = await waitFor(
    (event) =>
      event.type === "extension_ui_request" &&
      event.message === `T03_PARENT_OK:${child.pid}`,
    pingFrom,
  );
  assert.ok(ping);

  process.stdout.write(
    [
      "PROTOTYPE PASSED:",
      "real child PID and Pi run ID observed;",
      "provider/model/auth/reasoning/cwd inherited;",
      "parent context isolated;",
      "structured success/failure/cancellation and progress returned;",
      "parent remained usable after delegated failure.",
      "",
    ].join(" "),
  );
} finally {
  child.stdin.end();
  await new Promise((resolveClose) => child.once("close", resolveClose));
  await rm(sandbox, { recursive: true, force: true });
}
