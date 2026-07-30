import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import {
  access,
  mkdir,
  mkdtemp,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const networkGuard = join(
  repositoryRoot,
  "scripts",
  "acceptance",
  "network-guard.mjs",
);
const keepSandbox =
  process.env.MATTY_KEEP_ACCEPTANCE_SANDBOX === "1";

async function run(command, args, options) {
  return await new Promise((resolveRun, rejectRun) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: options.env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.once("error", rejectRun);
    child.once("close", (code, signal) => {
      if (code === 0) {
        resolveRun({ stdout, stderr });
        return;
      }
      rejectRun(
        new Error(
          [
            `${command} exited with ${String(code)}${signal ? ` (${signal})` : ""}`,
            stdout,
            stderr,
          ].join("\n"),
        ),
      );
    });
  });
}

function startRpc(pi, extension, cwd, env) {
  const child = spawn(
    pi,
    ["--mode", "rpc", "--no-session", "--no-extensions", "-e", extension],
    {
      cwd,
      env,
      stdio: ["pipe", "pipe", "pipe"],
    },
  );
  const events = [];
  let buffer = "";
  let stderr = "";
  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk) => {
    buffer += chunk;
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      if (line.trim()) {
        events.push(JSON.parse(line));
      }
    }
  });
  child.stderr.on("data", (chunk) => {
    stderr += chunk;
  });

  async function waitFor(predicate) {
    for (let attempt = 0; attempt < 800; attempt += 1) {
      const event = events.find(predicate);
      if (event) {
        return event;
      }
      await new Promise((resolveWait) => setTimeout(resolveWait, 25));
    }
    throw new Error(
      `Timed out waiting for Pi RPC\n${stderr}\n${JSON.stringify(events.slice(-30), null, 2)}`,
    );
  }

  return {
    child,
    events,
    send(command) {
      child.stdin.write(`${JSON.stringify(command)}\n`);
    },
    waitFor,
    async close() {
      child.stdin.end();
      await new Promise((resolveClose, rejectClose) => {
        child.once("close", (code) => {
          if (code === 0) {
            resolveClose();
          } else {
            rejectClose(new Error(`Pi RPC exited with ${code}\n${stderr}`));
          }
        });
      });
    },
  };
}

const sandbox = await mkdtemp(join(tmpdir(), "matty-t07-acceptance-"));
const home = join(sandbox, "home");
const agentDir = join(home, ".pi", "agent");
const project = join(sandbox, "project");
const host = join(sandbox, "host");
const artifacts = join(sandbox, "artifacts");
const npmCache = join(sandbox, "npm-cache");
const extension = join(sandbox, "t07-extension.ts");
const guardReady = join(sandbox, "network-guard.ready");
const guardViolation = join(sandbox, "network-guard.violation");
const forbidden = join(project, "forbidden.txt");

for (const directory of [
  agentDir,
  project,
  host,
  artifacts,
  npmCache,
]) {
  await mkdir(directory, { recursive: true });
}
await writeFile(join(project, "README.md"), "# acceptance\n");

const isolatedEnv = {
  PATH:
    process.env.PATH ??
    "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
  HOME: home,
  XDG_CONFIG_HOME: join(home, ".config"),
  PI_CODING_AGENT_DIR: agentDir,
  PI_OFFLINE: "1",
  NO_UPDATE_NOTIFIER: "1",
  npm_config_cache: npmCache,
  npm_config_userconfig: join(home, ".npmrc"),
};

let rpc;
try {
  await run("git", ["init", "-q"], { cwd: project, env: isolatedEnv });
  await run("git", ["add", "README.md"], { cwd: project, env: isolatedEnv });
  await run(
    "git",
    [
      "-c",
      "user.name=Matty Acceptance",
      "-c",
      "user.email=acceptance@example.invalid",
      "commit",
      "-qm",
      "fixture",
    ],
    { cwd: project, env: isolatedEnv },
  );
  await run("codegraph", ["init", "-i"], {
    cwd: project,
    env: isolatedEnv,
  });
  const canonicalProject = await realpath(project);

  await run("npm", ["run", "build"], {
    cwd: repositoryRoot,
    env: isolatedEnv,
  });
  const packed = await run(
    "npm",
    [
      "pack",
      repositoryRoot,
      "--json",
      "--pack-destination",
      artifacts,
    ],
    { cwd: project, env: isolatedEnv },
  );
  const [metadata] = JSON.parse(packed.stdout);
  const artifact = join(artifacts, metadata.filename);
  await access(artifact);
  await run(
    "npm",
    [
      "install",
      "--prefix",
      host,
      "--ignore-scripts",
      "@earendil-works/pi-coding-agent@0.83.0",
      artifact,
    ],
    { cwd: project, env: isolatedEnv },
  );

  const pi = join(host, "node_modules/.bin/pi");
  const mattyExtension = pathToFileURL(
    join(
      host,
      "node_modules/@yargote/matty/dist/adapters/pi-extension.js",
    ),
  ).href;
  const piAi = pathToFileURL(
    join(
      host,
      "node_modules/@earendil-works/pi-coding-agent/node_modules",
      "@earendil-works/pi-ai/dist/index.js",
    ),
  ).href;
  await writeFile(
    join(agentDir, "auth.json"),
    JSON.stringify({
      "t07-acceptance": {
        type: "api_key",
        key: "t07-child-safe-acceptance-auth",
      },
    }),
    { mode: 0o600 },
  );

  await writeFile(
    extension,
    `
import { registerPiMatty } from ${JSON.stringify(mattyExtension)};
import { createAssistantMessageEventStream } from ${JSON.stringify(piAi)};

const childMode = process.env.MATTY_CHILD_ROLE === "explorer";
const startMarker = "<!-- matty:rules -->";
const endMarker = "<!-- /matty:rules -->";
let parentRules = { start: 0, end: 0 };

function count(value, marker) {
  return value.split(marker).length - 1;
}

function assistant(model, content, stopReason = "stop") {
  return {
    role: "assistant",
    content,
    api: model.api,
    provider: model.provider,
    model: model.id,
    usage: {
      input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
    },
    stopReason,
    timestamp: Date.now(),
  };
}

function toolCalls(items) {
  return items.map(([id, command]) => ({
    type: "toolCall",
    id,
    name: "bash",
    arguments: { command },
  }));
}

export default function t07Acceptance(pi) {
  registerPiMatty(pi, process.env, {
    invocation: {
      command: process.env.MATTY_T07_PI,
      arguments: [
        "--no-extensions",
        "-e",
        process.env.MATTY_T07_EXTENSION,
        "--tools",
        "read,grep,find,ls,bash",
      ],
    },
    childEnvironment: {
      MATTY_T07_PI: process.env.MATTY_T07_PI,
      MATTY_T07_EXTENSION: process.env.MATTY_T07_EXTENSION,
      MATTY_NETWORK_GUARD_READY:
        process.env.MATTY_NETWORK_GUARD_READY,
      MATTY_NETWORK_GUARD_VIOLATION:
        process.env.MATTY_NETWORK_GUARD_VIOLATION,
    },
    independentRuntimeAvailable:
      process.env.MATTY_T07_BLOCK_RUNTIME !== "1",
  });

  pi.on("before_agent_start", (event) => {
    const observed = {
      start: count(event.systemPrompt, startMarker),
      end: count(event.systemPrompt, endMarker),
    };
    if (!childMode) parentRules = observed;
  });

  pi.on("tool_execution_update", (event, ctx) => {
    if (event.toolName === "subagent") {
      ctx.ui.notify(
        "T07_PROGRESS:" + JSON.stringify(event.partialResult.details),
        "info",
      );
    }
  });
  pi.on("tool_result", (event, ctx) => {
    if (event.toolName === "subagent") {
      ctx.ui.notify("T07_RESULT:" + JSON.stringify(event.details), "info");
      ctx.ui.notify("T07_PARENT_RULES:" + JSON.stringify(parentRules), "info");
    }
  });

  pi.registerProvider("t07-acceptance", {
    name: "T07 acceptance provider",
    baseUrl: "http://127.0.0.1/unused",
    api: "openai-completions",
    models: [{
      id: "observable",
      name: "T07 observable",
      reasoning: true,
      input: ["text"],
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      contextWindow: 8192,
      maxTokens: 256,
    }],
    streamSimple(model, context) {
      const stream = createAssistantMessageEventStream();
      const results = context.messages.filter(
        (message) => message.role === "toolResult",
      );

      if (!childMode) {
        if (results.length === 0) {
          queueMicrotask(() => stream.end(assistant(model, [{
            type: "toolCall",
            id: "parent-subagent",
            name: "subagent",
            arguments: { task: "Inspect Git, CodeGraph, shell, and diagnostics; then probe each recognized mutation family." },
          }], "toolUse")));
        } else {
          queueMicrotask(() => stream.end(assistant(
            model,
            [{ type: "text", text: "parent-complete" }],
          )));
        }
        return stream;
      }

      if (results.length === 0) {
        queueMicrotask(() => stream.end(assistant(model, toolCalls([
          ["allowed-git", "git status --short"],
          ["allowed-codegraph", "codegraph status"],
          ["allowed-shell", "pwd"],
          ["allowed-diagnostic", "node --version"],
        ]), "toolUse")));
        return stream;
      }
      if (results.length === 4) {
        queueMicrotask(() => stream.end(assistant(model, toolCalls([
          ["blocked-filesystem", "touch forbidden.txt"],
          ["blocked-shell", "echo changed > forbidden.txt"],
          ["blocked-git", "git commit --allow-empty -m forbidden"],
          ["blocked-github", "gh issue view 8"],
          ["blocked-network", "curl https://example.com"],
        ]), "toolUse")));
        return stream;
      }

      const byId = new Map(results.map((result) => [result.toolCallId, result]));
      const allowedIds = [
        "allowed-git",
        "allowed-codegraph",
        "allowed-shell",
        "allowed-diagnostic",
      ];
      const blockedIds = [
        "blocked-filesystem",
        "blocked-shell",
        "blocked-git",
        "blocked-github",
        "blocked-network",
      ];
      const payload = {
        rules: {
          start: count(context.systemPrompt, startMarker),
          end: count(context.systemPrompt, endMarker),
        },
        allowed: Object.fromEntries(
          allowedIds.map((id) => [id, byId.get(id)?.isError === false]),
        ),
        blocked: Object.fromEntries(
          blockedIds.map((id) => [
            id,
            byId.get(id)?.isError === true &&
              JSON.stringify(byId.get(id)?.content).includes("Inspection Guard blocked"),
          ]),
        ),
      };
      queueMicrotask(() => stream.end(assistant(
        model,
        [{ type: "text", text: JSON.stringify(payload) }],
      )));
      return stream;
    },
  });
}
`,
  );

  async function execute(blockRuntime) {
    const activeRpc = startRpc(pi, extension, canonicalProject, {
      ...isolatedEnv,
      NODE_OPTIONS: `--import=${networkGuard}`,
      MATTY_NETWORK_GUARD_READY: guardReady,
      MATTY_NETWORK_GUARD_VIOLATION: guardViolation,
      MATTY_T07_PI: pi,
      MATTY_T07_EXTENSION: extension,
      ...(blockRuntime ? { MATTY_T07_BLOCK_RUNTIME: "1" } : {}),
    });
    rpc = activeRpc;
    activeRpc.send({
      id: "model",
      type: "set_model",
      provider: "t07-acceptance",
      modelId: "observable",
    });
    assert.equal(
      (await activeRpc.waitFor(
        (event) => event.type === "response" && event.id === "model",
      )).success,
      true,
    );
    activeRpc.send({
      id: "delegate",
      type: "prompt",
      message: "Delegate the acceptance explorer.",
    });
    const resultEvent = await activeRpc.waitFor(
      (event) =>
        event.type === "extension_ui_request" &&
        event.message?.startsWith("T07_RESULT:"),
    );
    const rulesEvent = await activeRpc.waitFor(
      (event) =>
        event.type === "extension_ui_request" &&
        event.message?.startsWith("T07_PARENT_RULES:"),
    );
    const progress = activeRpc.events
      .filter(
        (event) =>
          event.type === "extension_ui_request" &&
          event.message?.startsWith("T07_PROGRESS:"),
      )
      .map((event) =>
        JSON.parse(event.message.slice("T07_PROGRESS:".length)),
      );
    await activeRpc.close();
    rpc = undefined;
    return {
      terminal: JSON.parse(resultEvent.message.slice("T07_RESULT:".length)),
      parentRules: JSON.parse(
        rulesEvent.message.slice("T07_PARENT_RULES:".length),
      ),
      progress,
    };
  }

  const success = await execute(false);
  assert.deepEqual(success.parentRules, { start: 1, end: 1 });
  assert.equal(success.terminal.contract.role, "explorer");
  assert.equal(
    success.terminal.outcome.status,
    "succeeded",
    JSON.stringify(success.terminal),
  );
  assert.deepEqual(
    success.progress.map((progress) => progress.type),
    [
      "started",
      "identified",
      "message",
      "tool-result",
      "tool-result",
      "tool-result",
      "tool-result",
      "message",
      "tool-result",
      "tool-result",
      "tool-result",
      "tool-result",
      "tool-result",
      "message",
    ],
  );
  const observed = JSON.parse(success.terminal.outcome.output);
  assert.deepEqual(observed.rules, { start: 1, end: 1 });
  assert.ok(Object.values(observed.allowed).every(Boolean));
  assert.ok(Object.values(observed.blocked).every(Boolean));
  await assert.rejects(access(forbidden));

  const blocked = await execute(true);
  assert.equal(blocked.terminal.outcome.status, "blocked");
  assert.deepEqual(blocked.terminal.outcome.diagnostic, {
    kind: "capability-preflight",
    contractId: "delegate-explorer",
    unmet: ["independent Subagent Runtime is unavailable"],
  });
  assert.deepEqual(blocked.progress, []);

  await access(guardReady);
  await assert.rejects(
    access(guardViolation),
    "T07 acceptance attempted network access",
  );
  process.stdout.write(
    [
      "T07 production explorer delegation acceptance passed",
      `artifact: ${metadata.filename}`,
      "parent/child Matty Rules: exactly one",
      "Capability Contract/preflight: validated and diagnosable",
      "real Subagent Runtime: structured progress and terminal output",
      "Git/CodeGraph/shell/diagnostics: inspected",
      "filesystem/shell/Git/GitHub/network mutations: blocked",
    ].join("\n") + "\n",
  );
} finally {
  if (rpc) {
    await rpc.close();
  }
  if (keepSandbox) {
    process.stderr.write(`T07 sandbox kept at ${sandbox}\n`);
  } else {
    await rm(sandbox, { recursive: true, force: true });
  }
}
