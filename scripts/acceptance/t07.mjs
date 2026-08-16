import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import {
  access,
  chmod,
  mkdir,
  mkdtemp,
  readFile,
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
      if (child.exitCode !== null) {
        if (child.exitCode === 0) return;
        throw new Error(`Pi RPC exited with ${child.exitCode}\n${stderr}`);
      }
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
const bin = join(sandbox, "bin");
const temporary = join(sandbox, "temporary");
const external = join(sandbox, "external");
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
  bin,
  temporary,
  external,
]) {
  await mkdir(directory, { recursive: true });
}
await writeFile(join(project, "README.md"), "# acceptance\n");
await writeFile(
  join(project, "package.json"),
  JSON.stringify({ name: "matty-worker-acceptance", private: true }),
);
const gh = join(bin, "gh");
await writeFile(
  gh,
  `#!/bin/sh
case "$1 $2" in
  "--version ") echo "gh version controlled" ;;
  "auth status") echo "authenticated" ;;
  "issue view") echo '{"number":9,"title":"inspection role acceptance"}' ;;
  *) exit 91 ;;
esac
`,
);
await chmod(gh, 0o755);

const isolatedEnv = {
  PATH: `${bin}:${
    process.env.PATH ??
    "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
  }`,
  HOME: home,
  XDG_CONFIG_HOME: join(home, ".config"),
  PI_CODING_AGENT_DIR: agentDir,
  PI_OFFLINE: "1",
  NO_UPDATE_NOTIFIER: "1",
  npm_config_cache: npmCache,
  npm_config_userconfig: join(home, ".npmrc"),
  TMPDIR: temporary,
};

let rpc;
try {
  await run("git", ["init", "-q"], { cwd: project, env: isolatedEnv });
  await run("git", ["add", "README.md", "package.json"], {
    cwd: project,
    env: isolatedEnv,
  });
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
  const canonicalProject = await realpath(project);
  const reviewCommit = (await run("git", ["rev-parse", "HEAD"], {
    cwd: project,
    env: isolatedEnv,
  })).stdout.trim();

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
      "@earendil-works/pi-coding-agent@0.84.2",
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

const childRole = process.env.MATTY_CHILD_ROLE;
// Acceptance fixtures capture transport facts before Matty scrubs the real child environment.
const workerTemporaryPaths = JSON.parse(process.env.MATTY_WORKER_TEMPORARY_PATHS ?? "[]");
const workerProtectedPaths = JSON.parse(process.env.MATTY_WORKER_PROTECTED_PATHS ?? "[]");
const childMode = ["explorer", "designer", "reviewer", "researcher", "worker"].includes(childRole);
const requestedRole = process.env.MATTY_T07_ROLE ?? "explorer";
const rulesStartMarker = "<!-- matty:rules -->";
const rulesEndMarker = "<!-- /matty:rules -->";
const guidanceStartMarker = "<!-- matty:guidance -->";
const guidanceEndMarker = "<!-- /matty:guidance -->";
let parentBlocks = {
  rules: { start: 0, end: 0 },
  guidance: { start: 0, end: 0 },
};

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

function toolCall(id, name, arguments_) {
  return { type: "toolCall", id, name, arguments: arguments_ };
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
        requestedRole === "worker"
          ? "read,write,edit,grep,find,ls,bash"
          : requestedRole === "researcher"
          ? "web_search,source_check,fetch_content,get_search_content,research_file"
          : "read,grep,find,ls,bash",
      ],
    },
    childEnvironment: {
      MATTY_T07_PI: process.env.MATTY_T07_PI,
      MATTY_T07_EXTENSION: process.env.MATTY_T07_EXTENSION,
      MATTY_T07_ROLE: requestedRole,
      MATTY_T07_EXTERNAL: process.env.MATTY_T07_EXTERNAL,
      MATTY_NETWORK_GUARD_READY:
        process.env.MATTY_NETWORK_GUARD_READY,
      MATTY_NETWORK_GUARD_VIOLATION:
        process.env.MATTY_NETWORK_GUARD_VIOLATION,
    },
    independentRuntimeAvailable:
      process.env.MATTY_T07_BLOCK_RUNTIME !== "1",
    registerWebExtension(api) {
      for (const name of [
        "web_search",
        "source_check",
        "fetch_content",
        "get_search_content",
      ]) {
        api.registerTool({
          name,
          label: name,
          description: "T15 certified web fixture",
          parameters: { type: "object", properties: {} },
          async execute() {
            return {
              content: [{
                type: "text",
                text: "Current cited result: https://example.invalid/source",
              }],
              details: { provider: "acceptance-fixture" },
            };
          },
        });
      }
    },
  });

  pi.on("before_agent_start", (event) => {
    const observed = {
      rules: {
        start: count(event.systemPrompt, rulesStartMarker),
        end: count(event.systemPrompt, rulesEndMarker),
      },
      guidance: {
        start: count(event.systemPrompt, guidanceStartMarker),
        end: count(event.systemPrompt, guidanceEndMarker),
      },
    };
    if (!childMode) parentBlocks = observed;
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
      ctx.ui.notify("T07_PARENT_BLOCKS:" + JSON.stringify(parentBlocks), "info");
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
            arguments: {
              requirement: "required",
              tasks: [{
                role: requestedRole,
                task: "Inspect Git, shell, and diagnostics; then probe each recognized mutation family.",
                ...(requestedRole === "researcher"
                  ? {
                    web: "required",
                    report: "docs/research/t15-packed-research.md",
                  }
                  : {}),
                ...(requestedRole === "reviewer" ? { reviewScope: {
                  schemaVersion: 1,
                  issue: { repository: "github.com/example/project", number: 9, reference: "#9" },
                  requirements: ["Inspect guarded behavior"],
                  outOfScope: [{ reference: "#42", reason: "dependent publication behavior" }],
                  baseSha: ${JSON.stringify(reviewCommit)},
                  candidateSha: ${JSON.stringify(reviewCommit)},
                  axes: ["standards", "spec"],
                } } : {}),
              }],
            },
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
        if (childRole === "researcher") {
          queueMicrotask(() => stream.end(assistant(model, [
            toolCall("research-web", "web_search", {}),
          ], "toolUse")));
          return stream;
        }
        if (childRole === "worker") {
          queueMicrotask(() => stream.end(assistant(model, [
            toolCall("allowed-write", "write", {
              path: "worker-output.txt",
              content: "before\\n",
            }),
          ], "toolUse")));
          return stream;
        }
        const allowedCommands = [
          ["allowed-git", "git status --short"],
          ["allowed-shell", "pwd && test -z $MATTY_CHILD_ROLE$MATTY_RESEARCH_CONTRACT$MATTY_RESEARCH_SCOPE$MATTY_WORKER_WORKING_TREE$MATTY_WORKER_TEMPORARY_PATHS$MATTY_WORKER_PROTECTED_PATHS$MATTY_WORKER_USER_HOME$MATTY_WORKER_USER_CONFIGURATION_PATHS"],
          ["allowed-diagnostic", "node --version"],
          ...(childRole === "reviewer"
            ? [["allowed-github", "gh issue view 9"]]
            : []),
        ];
        queueMicrotask(() => stream.end(assistant(
          model,
          toolCalls(allowedCommands),
          "toolUse",
        )));
        return stream;
      }
      if (childRole === "worker") {
        if (results.length === 1) {
          queueMicrotask(() => stream.end(assistant(model, [
            toolCall("allowed-edit", "edit", {
              path: "worker-output.txt",
              edits: [{ oldText: "before", newText: "after" }],
            }),
          ], "toolUse")));
          return stream;
        }
        if (results.length === 2) {
          const [temporaryPath] = workerTemporaryPaths;
          queueMicrotask(() => stream.end(assistant(model, [
            toolCall("allowed-temporary", "write", {
              path: temporaryPath + "/worker-temporary.txt",
              content: "temporary\\n",
            }),
          ], "toolUse")));
          return stream;
        }
        if (results.length === 3) {
          queueMicrotask(() => stream.end(assistant(model, toolCalls([
            ["allowed-install", "npm install --offline --ignore-scripts --package-lock=false --no-audit --no-fund"],
            ["allowed-check", "node --test"],
            ["allowed-git", "git status --short"],
            ["allowed-shell", "pwd && test -z $MATTY_CHILD_ROLE$MATTY_RESEARCH_CONTRACT$MATTY_RESEARCH_SCOPE$MATTY_WORKER_WORKING_TREE$MATTY_WORKER_TEMPORARY_PATHS$MATTY_WORKER_PROTECTED_PATHS$MATTY_WORKER_USER_HOME$MATTY_WORKER_USER_CONFIGURATION_PATHS"],
          ]), "toolUse")));
          return stream;
        }
        if (results.length === 7) {
          queueMicrotask(() => stream.end(assistant(model, [
            ...toolCalls([
              ["blocked-github", "gh issue view 10"],
              ["blocked-git", "git add worker-output.txt"],
              ["blocked-git-reference", "git update-ref refs/heads/worker HEAD"],
              ["blocked-global", "npm install --global typescript"],
              [
                "blocked-external",
                "touch " + process.env.MATTY_T07_EXTERNAL + "/forbidden.txt",
              ],
              [
                "blocked-single-writer",
                "rm -rf " + workerProtectedPaths[0],
              ],
            ]),
            toolCall("blocked-user-config", "write", {
              path: process.env.HOME + "/.npmrc",
              content: "changed\\n",
            }),
          ], "toolUse")));
          return stream;
        }
        const byId = new Map(results.map((result) => [result.toolCallId, result]));
        const allowedIds = [
          "allowed-write",
          "allowed-edit",
          "allowed-temporary",
          "allowed-install",
          "allowed-check",
          "allowed-git",
          "allowed-shell",
        ];
        const blockedIds = [
          "blocked-github",
          "blocked-git",
          "blocked-git-reference",
          "blocked-global",
          "blocked-external",
          "blocked-single-writer",
          "blocked-user-config",
        ];
        const payload = {
          rules: {
            start: count(context.systemPrompt, rulesStartMarker),
            end: count(context.systemPrompt, rulesEndMarker),
          },
          guidance: {
            start: count(context.systemPrompt, guidanceStartMarker),
            end: count(context.systemPrompt, guidanceEndMarker),
          },
          allowed: Object.fromEntries(
            allowedIds.map((id) => [id, byId.get(id)?.isError === false]),
          ),
          blocked: Object.fromEntries(
            blockedIds.map((id) => [
              id,
              byId.get(id)?.isError === true &&
                JSON.stringify(byId.get(id)?.content).includes("Worker Guard blocked"),
            ]),
          ),
        };
        queueMicrotask(() => stream.end(assistant(
          model,
          [{
            type: "text",
            text: JSON.stringify({
              schemaVersion: 1,
              summary: JSON.stringify(payload),
              changedPaths: ["worker-output.txt"],
              checks: [{ command: "node --test", status: "passed" }],
              evidenceRole: "supporting-only-parent-verification-required",
              reportedFullGate: { status: "not-run" },
            }),
          }],
        )));
        return stream;
      }
      if (childRole === "researcher") {
        if (results.length === 1) {
          queueMicrotask(() => stream.end(assistant(model, [
            toolCall("research-report", "research_file", {
              destination: "report",
              content:
                "# Packed researcher evidence\\n\\nSource: https://example.invalid/source\\n",
            }),
          ], "toolUse")));
          return stream;
        }
        queueMicrotask(() => stream.end(assistant(model, [{
          type: "text",
          text: JSON.stringify({
            summary: "researcher completed cited packed research",
            evidence: [{
              web: results[0]?.isError === false,
              report: results[1]?.isError === false,
              rules: {
                start: count(context.systemPrompt, rulesStartMarker),
                end: count(context.systemPrompt, rulesEndMarker),
              },
              guidance: {
                start: count(context.systemPrompt, guidanceStartMarker),
                end: count(context.systemPrompt, guidanceEndMarker),
              },
            }],
          }),
        }])));
        return stream;
      }
      const allowedCount = childRole === "reviewer" ? 4 : 3;
      if (results.length === allowedCount) {
        queueMicrotask(() => stream.end(assistant(model, toolCalls([
          ["blocked-filesystem", "touch forbidden.txt"],
          ["blocked-shell", "echo changed > forbidden.txt"],
          ["blocked-git", "git commit --allow-empty -m forbidden"],
          [
            "blocked-github",
            childRole === "reviewer"
              ? "gh issue comment 9 --body forbidden"
              : "gh issue view 9",
          ],
          ["blocked-network", "curl https://example.com"],
        ]), "toolUse")));
        return stream;
      }

      const byId = new Map(results.map((result) => [result.toolCallId, result]));
      const allowedIds = [
        "allowed-git",
        "allowed-shell",
        "allowed-diagnostic",
        ...(childRole === "reviewer" ? ["allowed-github"] : []),
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
          start: count(context.systemPrompt, rulesStartMarker),
          end: count(context.systemPrompt, rulesEndMarker),
        },
        guidance: {
          start: count(context.systemPrompt, guidanceStartMarker),
          end: count(context.systemPrompt, guidanceEndMarker),
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
        [{
          type: "text",
          text: JSON.stringify(childRole === "reviewer" ? {
            schemaVersion: 1,
            candidateSha: ${JSON.stringify(reviewCommit)},
            summary: "review completed",
            findings: [{ axis: "spec", severity: "non-blocking", requirement: "Inspect guarded behavior", evidence: JSON.stringify(payload) }],
          } : {
            summary: childRole + " inspection completed",
            evidence: [payload],
          }),
        }],
      )));
      return stream;
    },
  });
}
`,
  );
  const workerNpm = join(bin, "npm");
  await writeFile(
    workerNpm,
    `#!/bin/sh
case " $* " in
  *" --global "*|*" -g "*) exit 92 ;;
esac
mkdir -p node_modules/matty-worker-fixture
printf 'installed\\n' > node_modules/matty-worker-fixture/installed.txt
`,
  );
  await chmod(workerNpm, 0o755);

  async function execute(role, blockRuntime) {
    const activeRpc = startRpc(pi, extension, canonicalProject, {
      ...isolatedEnv,
      NODE_OPTIONS: `--import=${networkGuard}`,
      MATTY_NETWORK_GUARD_READY: guardReady,
      MATTY_NETWORK_GUARD_VIOLATION: guardViolation,
      MATTY_T07_PI: pi,
      MATTY_T07_EXTENSION: extension,
      MATTY_T07_ROLE: role,
      MATTY_T07_EXTERNAL: external,
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
        event.message?.startsWith("T07_PARENT_BLOCKS:"),
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
      parentBlocks: JSON.parse(
        rulesEvent.message.slice("T07_PARENT_BLOCKS:".length),
      ),
      progress,
    };
  }

  const success = await execute("explorer", false);
  assert.deepEqual(success.parentBlocks, {
    rules: { start: 1, end: 1 },
    guidance: { start: 1, end: 1 },
  });
  assert.equal(success.terminal.status, "succeeded");
  const successLeaf = success.terminal.tasks[0].value;
  assert.equal(successLeaf.contract.role, "explorer");
  assert.equal(
    successLeaf.outcome.status,
    "succeeded",
    JSON.stringify(success.terminal),
  );
  assert.deepEqual(
    success.progress.slice(0, 2).map((progress) => progress.progress.type),
    ["started", "identified"],
  );
  const activities = success.progress.slice(2).map((progress) => progress.progress);
  assert.ok(activities.length > 0, JSON.stringify(success.progress));
  assert.ok(activities.every((progress) => progress.type === "activity"));
  assert.ok(activities.every((progress) =>
    progress.observation?.schemaVersion === 1 &&
    Number.isSafeInteger(progress.observation.sequence) &&
    Number.isSafeInteger(progress.observation.observedAt) &&
    progress.observation.summary?.schemaVersion === 1 &&
    (progress.observation.summary.kind === "assistant-completed" ||
      (progress.observation.summary.kind === "tool-completed" &&
        ["read", "grep", "find", "ls", "bash", "other"].includes(progress.observation.summary.tool) &&
        ["succeeded", "failed"].includes(progress.observation.summary.outcome)))
  ), JSON.stringify(activities));
  assert.deepEqual(
    success.progress.at(-1)?.delegation.tasks[0].activities,
    activities.map((progress) => progress.observation),
  );
  assert.ok(
    success.progress.every((progress) =>
      /^D-[0-9a-f]{8}$/.test(progress.delegation.displayId) &&
      progress.delegation.roles[0] === "explorer" &&
      progress.delegation.taskCount === 1
    ),
    JSON.stringify(success.progress),
  );
  assert.doesNotMatch(
    JSON.stringify(success.progress),
    /tool-result|allowed-git|blocked-filesystem|inspection completed|toolCallId|args|command|partialResult|raw result|prompt|response|transcript/,
  );
  const observed = successLeaf.outcome.output.evidence[0];
  assert.deepEqual(observed.rules, { start: 1, end: 1 });
  assert.deepEqual(observed.guidance, { start: 1, end: 1 });
  assert.ok(Object.values(observed.allowed).every(Boolean), JSON.stringify(observed.allowed));
  assert.ok(Object.values(observed.blocked).every(Boolean));
  await assert.rejects(access(forbidden));

  for (const role of ["designer", "reviewer"]) {
    const roleResult = await execute(role, false);
    assert.equal(roleResult.terminal.status, "succeeded");
    const roleLeaf = roleResult.terminal.tasks[0].value;
    assert.equal(roleLeaf.contract.role, role);
    assert.equal(
      roleLeaf.outcome.status,
      "succeeded",
      JSON.stringify(roleResult.terminal),
    );
    const roleObserved = role === "reviewer"
      ? JSON.parse(roleLeaf.outcome.output.findings[0].evidence)
      : roleLeaf.outcome.output.evidence[0];
    assert.deepEqual(roleResult.parentBlocks, {
      rules: { start: 1, end: 1 },
      guidance: { start: 1, end: 1 },
    });
    assert.deepEqual(roleObserved.rules, { start: 1, end: 1 });
    assert.deepEqual(roleObserved.guidance, { start: 1, end: 1 });
    assert.ok(Object.values(roleObserved.allowed).every(Boolean));
    assert.ok(Object.values(roleObserved.blocked).every(Boolean));
    await assert.rejects(access(forbidden));
  }

  const worker = await execute("worker", false);
  assert.equal(worker.terminal.status, "succeeded");
  const workerLeaf = worker.terminal.tasks[0].value;
  assert.equal(workerLeaf.contract.role, "worker");
  assert.equal(
    workerLeaf.outcome.status,
    "succeeded",
    JSON.stringify(worker.terminal),
  );
  const workerOutput = workerLeaf.outcome.output;
  const workerObserved = JSON.parse(workerOutput.summary);
  assert.deepEqual(worker.parentBlocks, {
    rules: { start: 1, end: 1 },
    guidance: { start: 1, end: 1 },
  });
  assert.deepEqual(workerObserved.rules, { start: 1, end: 1 });
  assert.deepEqual(workerObserved.guidance, { start: 1, end: 1 });
  assert.ok(Object.values(workerObserved.allowed).every(Boolean));
  assert.ok(Object.values(workerObserved.blocked).every(Boolean));
  assert.equal(await readFile(join(project, "worker-output.txt"), "utf8"), "after\n");
  assert.equal(
    await readFile(join(temporary, "worker-temporary.txt"), "utf8"),
    "temporary\n",
  );
  assert.equal(
    await readFile(
      join(project, "node_modules", "matty-worker-fixture", "installed.txt"),
      "utf8",
    ),
    "installed\n",
  );
  await assert.rejects(access(join(external, "forbidden.txt")));
  await assert.rejects(access(join(home, ".npmrc")));

  const researcher = await execute("researcher", false);
  assert.equal(researcher.terminal.status, "succeeded");
  const researcherLeaf = researcher.terminal.tasks[0].value;
  assert.equal(researcherLeaf.contract.role, "researcher");
  assert.equal(
    researcherLeaf.outcome.status,
    "succeeded",
    JSON.stringify(researcher.terminal),
  );
  const researcherOutput = JSON.parse(researcherLeaf.outcome.output);
  assert.deepEqual(researcher.parentBlocks, {
    rules: { start: 1, end: 1 },
    guidance: { start: 1, end: 1 },
  });
  assert.deepEqual(researcherOutput.evidence, [{
    web: true,
    report: true,
    rules: { start: 1, end: 1 },
    guidance: { start: 1, end: 1 },
  }]);
  assert.match(
    await readFile(
      join(project, "docs", "research", "t15-packed-research.md"),
      "utf8",
    ),
    /https:\/\/example\.invalid\/source/,
  );

  const blocked = await execute("explorer", true);
  assert.equal(blocked.terminal.status, "blocked");
  assert.deepEqual(blocked.terminal.diagnostics, [{
    kind: "delegation",
    code: "preflight-failed",
    taskIndex: 0,
    role: "explorer",
    reason: "runtime-unavailable",
  }]);
  assert.deepEqual(
    blocked.progress.map((progress) => progress.code),
    [undefined],
  );
  assert.deepEqual(blocked.progress[0]?.delegation.diagnostics, [{
    code: "preflight-failed",
    taskIndex: 0,
    role: "explorer",
    reason: "runtime-unavailable",
  }]);

  await access(guardReady);
  await assert.rejects(
    access(guardViolation),
    "T07 acceptance attempted network access",
  );
  process.stdout.write(
    [
      "T07/T08/T09/T12 production role delegation acceptance passed",
      `artifact: ${metadata.filename}`,
      "parent/child Matty Guidance and Rules: exactly one each",
      "Capability Contract/preflight: validated and diagnosable",
      "real Subagent Runtime: structured progress and terminal output",
      "delegation groups: validated, atomic, bounded, and redacted",
      "Git/shell/diagnostics: inspected",
      "designer: gh blocked",
      "reviewer: gh availability/auth/read inspection passed; mutation blocked",
      "filesystem/shell/Git/GitHub/network mutations: blocked",
      "worker: project writes, validated temporary writes, local install, and checks passed",
      "Single Writer/Worker Guard: GitHub, Git, global install, external path, and user configuration blocked",
      "researcher: required web contract and one cited bounded report passed",
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
