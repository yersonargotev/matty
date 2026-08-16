import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
  access,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const PI_VERSION = "0.84.2";
const CERTIFIED_TARGET = "darwin/arm64";
const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const keepSandbox = process.env.MATTY_KEEP_ACCEPTANCE_SANDBOX === "1";

function certifiedPreflight() {
  const actual = `${process.platform}/${process.arch}`;
  if (actual !== CERTIFIED_TARGET) {
    throw new Error(
      `[T10:certified-target] packed Delegation TUI acceptance requires ${CERTIFIED_TARGET}; received ${actual}`,
    );
  }
  if (process.execPath === undefined) {
    throw new Error("[T10:certified-target] Node executable is unavailable");
  }
}

async function run(command, args, { cwd, env, timeoutMs = 180_000 }) {
  return await new Promise((resolveRun, rejectRun) => {
    const child = spawn(command, args, { cwd, env, stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    const timer = setTimeout(() => child.kill("SIGTERM"), timeoutMs);
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.once("error", (error) => {
      clearTimeout(timer);
      rejectRun(error);
    });
    child.once("close", (code, signal) => {
      clearTimeout(timer);
      if (code === 0) resolveRun({ stdout, stderr });
      else rejectRun(new Error([
        `${command} exited with ${code ?? "no code"}${signal ? ` (${signal})` : ""}`,
        stdout,
        stderr,
      ].join("\n")));
    });
  });
}

function tclLiteral(value) {
  return `{${value.replaceAll("\\", "/").replaceAll("}", "\\}")}}`;
}

certifiedPreflight();
await access("/usr/bin/expect").catch(() => {
  throw new Error("[T10:certified-target] /usr/bin/expect is required for the certified PTY acceptance");
});

const sandbox = await mkdtemp(join(tmpdir(), "matty-t10-delegation-tui-"));
const home = join(sandbox, "home");
const agentDir = join(home, ".pi", "agent");
const project = join(sandbox, "project");
const host = join(sandbox, "host");
const artifacts = join(sandbox, "artifacts");
const npmCache = join(sandbox, "npm-cache");
const extension = join(sandbox, "t10-extension.mjs");
const expectScript = join(sandbox, "t10.expect");
const transcript = join(sandbox, "t10-transcript.log");
for (const directory of [agentDir, project, host, artifacts, npmCache]) {
  await mkdir(directory, { recursive: true });
}
await writeFile(join(project, "README.md"), "# T10 isolated project\n");

const isolatedEnv = {
  PATH: process.env.PATH ?? "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
  HOME: home,
  XDG_CONFIG_HOME: join(home, ".config"),
  PI_CODING_AGENT_DIR: agentDir,
  PI_OFFLINE: "1",
  NO_UPDATE_NOTIFIER: "1",
  npm_config_cache: npmCache,
  npm_config_userconfig: join(home, ".npmrc"),
  TERM: "xterm-256color",
};

try {
  const providedArtifact = process.env.MATTY_PACKED_ARTIFACT
    ? resolve(process.env.MATTY_PACKED_ARTIFACT)
    : undefined;
  if (!providedArtifact) {
    await run("npm", ["run", "build"], { cwd: repositoryRoot, env: isolatedEnv });
  }
  const packed = await run(
    "npm",
    providedArtifact
      ? ["pack", providedArtifact, "--ignore-scripts", "--dry-run", "--json"]
      : ["pack", repositoryRoot, "--ignore-scripts", "--json", "--pack-destination", artifacts],
    { cwd: project, env: isolatedEnv },
  );
  const [metadata] = JSON.parse(packed.stdout);
  const artifact = providedArtifact ?? join(artifacts, metadata.filename);
  await access(artifact);
  await run("npm", [
    "install", "--prefix", host, "--ignore-scripts",
    `@earendil-works/pi-coding-agent@${PI_VERSION}`, artifact,
  ], { cwd: project, env: isolatedEnv });

  const pi = join(host, "node_modules/.bin/pi");
  const version = await run(pi, ["--version"], { cwd: project, env: isolatedEnv });
  assert.equal(
    version.stdout.trim(),
    PI_VERSION,
    `[T10:certified-target] expected Pi ${PI_VERSION}, received ${version.stdout.trim()}`,
  );
  const mattyExtension = pathToFileURL(join(
    host,
    "node_modules/@yargote/matty/dist/adapters/pi-extension.js",
  )).href;
  const piAi = pathToFileURL(join(
    host,
    "node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-ai/dist/index.js",
  )).href;

  await writeFile(join(agentDir, "auth.json"), JSON.stringify({
    "t10-acceptance": { type: "api_key", key: "isolated-fixture-key" },
  }), { mode: 0o600 });
  await writeFile(extension, `
import { registerPiMatty } from ${JSON.stringify(mattyExtension)};
import { createAssistantMessageEventStream } from ${JSON.stringify(piAi)};

const childMode = Boolean(process.env.MATTY_CHILD_ROLE);
const childLifetimeHandle = childMode ? setInterval(() => {}, 60_000) : undefined;
void childLifetimeHandle;
function assistant(model, content, stopReason = "stop") {
  return {
    role: "assistant", content, api: model.api, provider: model.provider,
    model: model.id,
    usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
    stopReason, timestamp: Date.now(),
  };
}
export default function t10(pi) {
  registerPiMatty(pi, process.env, {
    invocation: {
      command: process.env.MATTY_T10_PI,
      arguments: ["--no-extensions", "-e", process.env.MATTY_T10_EXTENSION],
    },
    childEnvironment: {
      MATTY_T10_PI: process.env.MATTY_T10_PI,
      MATTY_T10_EXTENSION: process.env.MATTY_T10_EXTENSION,
    },
    independentRuntimeAvailable: true,
  });
  pi.registerProvider("t10-acceptance", {
    name: "T10 deterministic fixture", baseUrl: "http://127.0.0.1/unused",
    api: "openai-completions",
    models: [{ id: "observable", name: "T10 observable", reasoning: false,
      input: ["text"], cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      contextWindow: 8192, maxTokens: 128 }],
    streamSimple(model, context) {
      const stream = createAssistantMessageEventStream();
      if (childMode) {
        const partial = assistant(model, [{ type: "text", text: "" }]);
        queueMicrotask(() => {
          stream.push({ type: "start", partial });
          stream.push({ type: "text_start", contentIndex: 0, partial });
          stream.push({
            type: "text_delta",
            contentIndex: 0,
            delta: "\\u001b]0;owned\\u0007\\u001b[999mlive transcript needle\\u001b[0m",
            partial: assistant(model, [{
              type: "text",
              text: "\\u001b]0;owned\\u0007\\u001b[999mlive transcript needle\\u001b[0m",
            }]),
          });
        });
        return stream;
      }
      const results = context.messages.filter((message) => message.role === "toolResult");
      queueMicrotask(() => stream.end(assistant(model,
        results.length === 0
          ? [{ type: "toolCall", id: "five-task-delegation", name: "subagent", arguments: {
              requirement: "required",
              tasks: Array.from({ length: 5 }, (_, index) => ({
                role: "explorer", task: "deterministic hold " + (index + 1),
              })),
            } }]
          : [{ type: "text", text: "T10 delegation cancellation complete" }],
        results.length === 0 ? "toolUse" : "stop",
      )));
      return stream;
    },
  });
}
`);

  if (process.env.MATTY_MANUAL_TUI === "1") {
    const evidencePath = resolve(
      process.env.MATTY_MANUAL_EVIDENCE ??
        join(repositoryRoot, "docs/acceptance/delegation-tui-manual.md"),
    );
    const digest = createHash("sha256").update(await readFile(artifact)).digest("hex");
    await mkdir(dirname(evidencePath), { recursive: true });
    await writeFile(evidencePath, [
      "# Manual Delegation TUI validation",
      "",
      `- Artifact: \`${metadata.filename}\``,
      `- SHA-256: \`${digest}\``,
      `- Pi: \`${PI_VERSION}\``,
      `- Target: \`${CERTIFIED_TARGET}\``,
      `- Date: \`${new Date().toISOString()}\``,
      "",
      "## Subjective observations (complete after exiting Pi)",
      "",
      "- [ ] No duplicated, stale, or corrupted console frames during live rerenders.",
      "- [ ] Cancellation controls and confirmation were visually unambiguous.",
      "- [ ] Cursor and editor focus were correct after closing with `q` and with `Esc`.",
      "- [ ] `/matty status` accepted input immediately after each close.",
      "",
      "Operator/result notes:",
      "",
    ].join("\n"));
    process.stdout.write([
      "Manual steps:",
      "1. Enter: start deterministic delegation",
      "2. After four tasks are active and one queued, wait at least 1.5 seconds and verify that running state remains stable before entering: /matty delegations",
      "3. Inspect live frames; press c, reject with Esc, press c again, confirm with y.",
      "4. Inspect cancelling/cancelled rerenders; close with q.",
      "5. Enter /matty status; repeat console close with Esc if desired; exit with Ctrl+C.",
      `6. Complete the checklist at ${evidencePath}`,
    ].join("\n") + "\n");
    await new Promise((resolveRun, rejectRun) => {
      const child = spawn(pi, [
        "--no-session", "--no-extensions", "-e", extension,
        "--provider", "t10-acceptance", "--model", "observable",
        "--api-key", "isolated-fixture-key", "--offline",
      ], {
        cwd: project,
        env: { ...isolatedEnv, MATTY_T10_PI: pi, MATTY_T10_EXTENSION: extension },
        stdio: "inherit",
      });
      child.once("error", rejectRun);
      child.once("close", (code) => code === 0
        ? resolveRun()
        : rejectRun(new Error(`manual Pi exited with ${code}`)));
    });
    process.stdout.write(`Manual evidence template recorded at ${evidencePath}\n`);
    process.exitCode = 0;
  } else await writeFile(expectScript, `
set timeout 45
log_user 1
log_file -noappend ${tclLiteral(transcript)}
proc phase {name pattern} {
  global expect_out
  expect {
    -re $pattern { puts "T10_PHASE:$name" }
    timeout { puts stderr "T10_TIMEOUT:$name"; exit 70 }
    eof { puts stderr "T10_EOF:$name"; exit 71 }
  }
}
proc drain {} {
  expect -timeout 0 {
    -re {.+} { exp_continue }
    timeout {}
  }
}
spawn -noecho ${tclLiteral(pi)} --no-session --no-extensions -e ${tclLiteral(extension)} --provider t10-acceptance --model observable --api-key isolated-fixture-key --offline
stty rows 40 columns 160
phase startup {Matty active}
send -- "start deterministic delegation\\r"
phase live-counts {Matty fleet[^\\r\\n]*Active tasks: 4[^\\r\\n]*Queued tasks: 1}
after 1600
drain
stty rows 41 columns 160
phase stable-before-open {T-[0-9a-f]{8}[^\\r\\n]*State: queued[^\\r\\n]*queue 1}
send -- "/matty delegations\\r"
phase console-open {Delegation Console.*Delegations}
phase navigation-hint {Enter Delegated Tasks.*Esc/q close}
send -- "\\r"
phase task-list {Delegations.*Delegated Tasks}
phase task-state {T-[0-9a-f]{8}.*State: (running|queued).*Role: explorer}
set taskLine $expect_out(0,string)
regexp {(T-[0-9a-f]{8})} $taskLine _ taskId
send -- "\\r"
phase child-session {Delegations.*Delegated Tasks.*Child Session}
phase session-key-hints {/ search.*f filter.*Esc/q close}
phase accessible-labels {Task state:.*Role:}
phase process-labels {PID:.*Run ID:}
phase usage-labels {Usage:.*Cost:.*Context consumption:}
phase live-transcript {live transcript needle}
send -- "f"
phase filter-message {Filter: message}
send -- "/absent\\r"
phase search-no-match {No transcript entries match}
send -- "/needle\\r"
phase search-match {Search: needle}
phase search-result {live transcript needle}
send -- "f"
phase filter-reasoning {Filter: reasoning}
phase filtered-out {No transcript entries match}
send -- "\\033"
after 200
send -- "/matty task $taskId\\r"
phase exact-task {Delegated Task ID:}
send -- "q"
after 200
send -- "/matty status\\r"
phase focus-restored-q {Matty 0\\.2\\.0}
send -- "/matty delegations\\r"
phase cancellation-console {Delegation Console.*Delegations}
send -- "c"
phase confirmation-first {Confirm cancellation[^\\r\\n]*4 active[^\\r\\n]*1 queued}
send -- "\\033"
after 150
send -- "c"
phase confirmation-after-reject {Confirm cancellation[^\\r\\n]*4 active[^\\r\\n]*1 queued}
send -- "y"
phase cancelling {D-[0-9a-f]{8} cancelling}
phase cancellation-requested {Cancellation requested for D-[0-9a-f]{8}}
phase cancelled {D-[0-9a-f]{8} cancelled}
send -- "q"
after 200
send -- "/matty status\\r"
phase focus-restored {Matty 0\\.2\\.0}
send -- "\\004"
expect eof
`);

  if (process.env.MATTY_MANUAL_TUI !== "1") {
    await run("/usr/bin/expect", [expectScript], {
      cwd: project,
      env: {
        ...isolatedEnv,
        MATTY_T10_PI: pi,
        MATTY_T10_EXTENSION: extension,
      },
      timeoutMs: 90_000,
    });
  }
  if (process.env.MATTY_MANUAL_TUI !== "1") {
    const ptyOutput = await readFile(transcript, "utf8");
    assert.doesNotMatch(
      ptyOutput,
      /owned|\u001b\[999m|\\u001b\[999m/,
      "[T10:clean-terminal-output] child terminal-control payload reached the PTY",
    );
    process.stdout.write([
      "T10 packed Delegation TUI acceptance passed",
      `artifact: ${metadata.filename}`,
      `Pi: ${PI_VERSION}`,
      `target: ${CERTIFIED_TARGET}`,
      "live task-level fleet state: 4 active / 1 queued",
      "Delegations → Delegated Tasks → Child Session navigation: observed",
      "search/filter/key hints/accessibility labels: observed",
      "exact task command and Esc/q focus restoration: observed",
      "cancellation reject/confirm/live rerender: observed",
    ].join("\n") + "\n");
  }
} catch (error) {
  const diagnostic = await readFile(transcript, "utf8").catch(() => "(no PTY transcript captured)");
  throw new Error(
    `[T10:packed-delegation-tui] ${error instanceof Error ? error.message : String(error)}\nPTY transcript:\n${diagnostic.slice(-20_000)}\nsandbox: ${sandbox}`,
    { cause: error },
  );
} finally {
  if (keepSandbox) process.stderr.write(`T10 sandbox kept at ${sandbox}\n`);
  else await rm(sandbox, { recursive: true, force: true });
}
