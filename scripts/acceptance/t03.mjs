import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
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
  "scripts/acceptance/network-guard.mjs",
);

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
    for (let attempt = 0; attempt < 400; attempt += 1) {
      const event = events.find(predicate);
      if (event) {
        return event;
      }
      await new Promise((resolveWait) => setTimeout(resolveWait, 25));
    }
    throw new Error(
      `Timed out waiting for Pi RPC\n${stderr}\n${JSON.stringify(events.slice(-20), null, 2)}`,
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

const sandbox = await mkdtemp(join(tmpdir(), "matty-t03-acceptance-"));
const home = join(sandbox, "home");
const agentDir = join(home, ".pi", "agent");
const project = join(sandbox, "project");
const host = join(sandbox, "host");
const artifacts = join(sandbox, "artifacts");
const npmCache = join(sandbox, "npm-cache");
const extension = join(sandbox, "t03-extension.ts");
const guardReady = join(sandbox, "network-guard.ready");
const guardViolation = join(sandbox, "network-guard.violation");
const authPath = join(agentDir, "auth.json");
const authKey = "t03-child-safe-acceptance-auth";
const authContents = JSON.stringify({
  "t03-acceptance": { type: "api_key", key: authKey },
});
const authDigest = createHash("sha256")
  .update(authContents)
  .digest("hex");
const credentialDigest = createHash("sha256")
  .update(authKey)
  .digest("hex");

for (const directory of [
  agentDir,
  project,
  host,
  artifacts,
  npmCache,
]) {
  await mkdir(directory, { recursive: true });
}
const canonicalProject = await realpath(project);

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
  const providedArtifact = process.env.MATTY_PACKED_ARTIFACT
    ? resolve(process.env.MATTY_PACKED_ARTIFACT)
    : undefined;
  if (!providedArtifact) {
    await run("npm", ["run", "build"], {
      cwd: repositoryRoot,
      env: isolatedEnv,
    });
  }
  const packed = await run(
    "npm",
    providedArtifact
      ? ["pack", providedArtifact, "--ignore-scripts", "--dry-run", "--json"]
      : [
        "pack",
        repositoryRoot,
        "--ignore-scripts",
        "--json",
        "--pack-destination",
        artifacts,
      ],
    { cwd: project, env: isolatedEnv },
  );
  const [metadata] = JSON.parse(packed.stdout);
  const artifact = providedArtifact ?? join(artifacts, metadata.filename);
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
  const runtime = pathToFileURL(
    join(
      host,
      "node_modules/@yargote/matty/dist/application/child-pi-runtime.js",
    ),
  ).href;
  const piAi = pathToFileURL(
    join(
      host,
      "node_modules/@earendil-works/pi-coding-agent/node_modules",
      "@earendil-works/pi-ai/dist/index.js",
    ),
  ).href;
  await writeFile(authPath, authContents, { mode: 0o600 });

  await writeFile(
    extension,
    `
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { createChildPiRunner } from ${JSON.stringify(runtime)};
import { createAssistantMessageEventStream } from ${JSON.stringify(piAi)};

const childMode = process.env.MATTY_T03_CHILD === "1";
let thinking = "unknown";
let parentCredentialDigest = "unobserved";

function digestAuth() {
  return createHash("sha256")
    .update(readFileSync(process.env.MATTY_T03_AUTH_PATH))
    .digest("hex");
}

function digestCredential(value) {
  return createHash("sha256").update(value ?? "").digest("hex");
}

function assistant(model, text, stopReason = "stop", errorMessage) {
  return {
    role: "assistant",
    content: [{ type: "text", text }],
    api: model.api,
    provider: model.provider,
    model: model.id,
    usage: {
      input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
    },
    stopReason,
    errorMessage,
    timestamp: Date.now(),
  };
}

export default function t03Acceptance(pi) {
  pi.on("before_agent_start", (_event, ctx) => {
    thinking = ctx.thinkingLevel;
  });

  pi.registerProvider("t03-acceptance", {
    name: "T03 acceptance provider",
    baseUrl: "http://127.0.0.1/unused",
    api: "openai-completions",
    models: [{
      id: "observable",
      name: "T03 observable",
      reasoning: true,
      input: ["text"],
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      contextWindow: 8192,
      maxTokens: 128,
    }],
    streamSimple(model, context, options) {
      const stream = createAssistantMessageEventStream();
      if (!childMode) {
        parentCredentialDigest = digestCredential(options?.apiKey);
        queueMicrotask(() => stream.end(assistant(model, "parent-ready")));
        return stream;
      }
      const serialized = JSON.stringify(context.messages);
      if (serialized.includes("T03_CANCEL")) {
        return stream;
      }
      const payload = JSON.stringify({
        pid: process.pid,
        ppid: process.ppid,
        provider: model.provider,
        model: model.id,
        thinking,
        cwd: process.cwd(),
        authDigest: digestAuth(),
        credentialDigest: digestCredential(options?.apiKey),
        containsParentContext: serialized.includes("PARENT_ONLY_CONTEXT"),
      });
      queueMicrotask(() => stream.end(
        serialized.includes("T03_FAILURE")
          ? assistant(model, payload, "error", "controlled delegated failure")
          : assistant(model, payload),
      ));
      return stream;
    },
  });

  if (childMode) return;

  pi.registerCommand("t03-accept", {
    description: "Exercise the packed Matty child runtime",
    handler: async (args, ctx) => {
      const scenario = args.trim();
      const runner = createChildPiRunner({
        invocation: {
          command: process.env.MATTY_T03_PI,
          arguments: [
            "--no-extensions",
            "-e",
            process.env.MATTY_T03_EXTENSION,
          ],
        },
        parent: {
          provider: ctx.model.provider,
          model: ctx.model.id,
          thinking: ctx.thinkingLevel,
          cwd: ctx.cwd,
        },
        authentication: {
          provider: ctx.model.provider,
          environment: {
            PATH: process.env.PATH,
            HOME: process.env.HOME,
            XDG_CONFIG_HOME: process.env.XDG_CONFIG_HOME,
            PI_CODING_AGENT_DIR: process.env.PI_CODING_AGENT_DIR,
            PI_OFFLINE: "1",
            NO_UPDATE_NOTIFIER: "1",
            MATTY_T03_CHILD: "1",
            MATTY_T03_PI: process.env.MATTY_T03_PI,
            MATTY_T03_EXTENSION: process.env.MATTY_T03_EXTENSION,
            MATTY_T03_AUTH_PATH: process.env.MATTY_T03_AUTH_PATH,
          },
        },
        terminationGraceMs: 100,
      });
      ctx.ui.notify(
        "T03_PARENT_AUTH:" + parentCredentialDigest,
        "info",
      );
      const controller = new AbortController();
      const timer = scenario === "cancel"
        ? setTimeout(() => controller.abort(), 5_000)
        : undefined;
      const outcome = await runner.run("T03_" + scenario.toUpperCase(), {
        signal: controller.signal,
        onProgress(progress) {
          ctx.ui.notify("T03_PROGRESS:" + JSON.stringify(progress), "info");
          if (scenario === "cancel" && progress.type === "identified") {
            controller.abort();
          }
        },
      });
      if (timer) clearTimeout(timer);
      ctx.ui.notify(
        "T03_RESULT:" + scenario + ":" + JSON.stringify(outcome),
        "info",
      );
    },
  });
  pi.registerCommand("t03-parent-ping", {
    description: "Prove parent liveness",
    handler: async (_args, ctx) => {
      ctx.ui.notify("T03_PARENT_OK:" + process.pid, "info");
    },
  });
}
`,
  );

  rpc = startRpc(pi, extension, canonicalProject, {
    ...isolatedEnv,
    NODE_OPTIONS: `--import=${networkGuard}`,
    MATTY_NETWORK_GUARD_READY: guardReady,
    MATTY_NETWORK_GUARD_VIOLATION: guardViolation,
    MATTY_T03_PI: pi,
    MATTY_T03_EXTENSION: extension,
    MATTY_T03_AUTH_PATH: authPath,
  });

  rpc.send({
    id: "model",
    type: "set_model",
    provider: "t03-acceptance",
    modelId: "observable",
  });
  assert.equal(
    (await rpc.waitFor(
      (event) => event.type === "response" && event.id === "model",
    )).success,
    true,
  );
  rpc.send({ id: "thinking", type: "set_thinking_level", level: "high" });
  assert.equal(
    (await rpc.waitFor(
      (event) => event.type === "response" && event.id === "thinking",
    )).success,
    true,
  );
  rpc.send({
    id: "parent-context",
    type: "prompt",
    message: "PARENT_ONLY_CONTEXT",
  });
  await rpc.waitFor((event) => event.type === "agent_end");

  async function scenario(name) {
    const start = rpc.events.length;
    rpc.send({
      id: `scenario-${name}`,
      type: "prompt",
      message: `/t03-accept ${name}`,
    });
    const resultEvent = await rpc.waitFor(
      (event) =>
        rpc.events.indexOf(event) >= start &&
        event.type === "extension_ui_request" &&
        event.message?.startsWith(`T03_RESULT:${name}:`),
    );
    return {
      outcome: JSON.parse(
        resultEvent.message.slice(`T03_RESULT:${name}:`.length),
      ),
      progress: rpc.events
        .slice(start)
        .filter(
          (event) =>
            event.type === "extension_ui_request" &&
            event.message?.startsWith("T03_PROGRESS:"),
        )
        .map((event) =>
          JSON.parse(event.message.slice("T03_PROGRESS:".length)),
        ),
      parentCredentialDigest: rpc.events
        .slice(start)
        .find(
          (event) =>
            event.type === "extension_ui_request" &&
            event.message?.startsWith("T03_PARENT_AUTH:"),
        )
        ?.message.slice("T03_PARENT_AUTH:".length),
    };
  }

  const success = await scenario("success");
  assert.equal(success.outcome.status, "succeeded");
  assert.deepEqual(
    success.progress.map((progress) => progress.type),
    ["started", "identified", "activity"],
  );
  assert.deepEqual(success.progress[2]?.observation.summary, {
    schemaVersion: 1,
    kind: "assistant-completed",
    outcome: "succeeded",
  });
  assert.equal(success.progress[2]?.observation.schemaVersion, 1);
  assert.equal(success.progress[2]?.observation.sequence, 1);
  assert.ok(Number.isSafeInteger(success.progress[2]?.observation.observedAt));
  const observed = JSON.parse(success.outcome.output);
  assert.equal(observed.pid, success.outcome.child.pid);
  assert.equal(observed.ppid, rpc.child.pid);
  assert.equal(observed.provider, "t03-acceptance");
  assert.equal(observed.model, "observable");
  assert.equal(observed.thinking, "high");
  assert.equal(observed.cwd, canonicalProject);
  assert.equal(observed.authDigest, authDigest);
  assert.equal(success.parentCredentialDigest, credentialDigest);
  assert.equal(observed.credentialDigest, success.parentCredentialDigest);
  assert.equal(observed.containsParentContext, false);

  const failure = await scenario("failure");
  assert.equal(failure.outcome.status, "failed");
  assert.equal(failure.outcome.failure.kind, "child-failed");

  const cancellation = await scenario("cancel");
  assert.equal(cancellation.outcome.status, "cancelled");
  assert.ok(
    cancellation.progress.some(
      (progress) => progress.type === "terminating",
    ),
  );

  rpc.send({
    id: "parent-ping",
    type: "prompt",
    message: "/t03-parent-ping",
  });
  await rpc.waitFor(
    (event) =>
      event.type === "extension_ui_request" &&
      event.message === `T03_PARENT_OK:${rpc.child.pid}`,
  );

  await access(guardReady);
  await assert.rejects(
    access(guardViolation),
    "T03 acceptance attempted network access",
  );
  process.stdout.write(
    [
      "T03 packed-runtime acceptance passed",
      `artifact: ${metadata.filename}`,
      "Pi source: 0.83.0 @ 845d6ff1f6643aba440341cce877ce1c43ebbc39",
      "independent PID/context: proven",
      "provider/model/auth/reasoning: inherited",
      "success/failure/cancellation/progress: structured",
      "parent after delegated failure: usable",
    ].join("\n") + "\n",
  );
} finally {
  if (rpc) {
    await rpc.close();
  }
  await rm(sandbox, { recursive: true, force: true });
}
