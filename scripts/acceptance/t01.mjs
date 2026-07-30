import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
  access,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const PI_PACKAGE = "@earendil-works/pi-coding-agent@0.83.0";
const ACTIVE_HINT = "Matty active · /matty status";
const REPOSITORY_ROOT = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const NETWORK_GUARD_PATH = join(
  REPOSITORY_ROOT,
  "scripts",
  "acceptance",
  "network-guard.mjs",
);
const KEEP_SANDBOX = process.env.MATTY_KEEP_ACCEPTANCE_SANDBOX === "1";

class PhaseError extends Error {
  constructor(phase, message, cause) {
    super(`[T01:${phase}] ${message}`, { cause });
    this.name = "PhaseError";
  }
}

async function run(phase, command, args, options = {}) {
  return new Promise((resolveRun, rejectRun) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: options.env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    const timer = setTimeout(() => {
      child.kill("SIGTERM");
      rejectRun(
        new PhaseError(
          phase,
          `timed out after ${options.timeoutMs ?? 120_000}ms: ${command} ${args.join(" ")}`,
        ),
      );
    }, options.timeoutMs ?? 120_000);

    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", (error) => {
      clearTimeout(timer);
      rejectRun(
        new PhaseError(
          phase,
          `could not start ${command}: ${error.message}`,
          error,
        ),
      );
    });
    child.on("close", (code, signal) => {
      clearTimeout(timer);
      if (code === 0) {
        resolveRun({ stdout, stderr });
        return;
      }
      rejectRun(
        new PhaseError(
          phase,
          [
            `${command} exited with code ${code ?? "none"}${signal ? ` (${signal})` : ""}`,
            stdout.trim() ? `stdout:\n${stdout.trim()}` : "",
            stderr.trim() ? `stderr:\n${stderr.trim()}` : "",
          ]
            .filter(Boolean)
            .join("\n"),
        ),
      );
    });
  });
}

async function snapshotTree(root) {
  const entries = [];

  async function visit(relativePath) {
    const absolutePath = join(root, relativePath);
    const children = await readdir(absolutePath, { withFileTypes: true });
    children.sort((left, right) => left.name.localeCompare(right.name));

    for (const child of children) {
      const childRelativePath = join(relativePath, child.name);
      if (child.isDirectory()) {
        entries.push(`directory:${childRelativePath}`);
        await visit(childRelativePath);
        continue;
      }

      const metadata = await stat(join(root, childRelativePath));
      const contents = await readFile(join(root, childRelativePath));
      const digest = createHash("sha256").update(contents).digest("hex");
      entries.push(`file:${childRelativePath}:${metadata.size}:${digest}`);
    }
  }

  await visit("");
  return entries;
}

function escapeSandboxLiteral(value) {
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

function startRpc(piBinary, cwd, env, sandboxRoot, operatorHome) {
  const guardReadyPath = join(sandboxRoot, "network-guard.ready");
  const guardViolationPath = join(
    sandboxRoot,
    "network-guard.violation",
  );
  const protectedReadPaths = [
    ".agents",
    ".aws",
    ".codex",
    ".config",
    ".gnupg",
    ".matty",
    ".npmrc",
    ".pi",
    ".ssh",
  ].map((entry) => join(operatorHome, entry));
  const sandboxProfile = [
    "(version 1)",
    "(allow default)",
    "(deny network*)",
    `(deny file-write* (subpath "${escapeSandboxLiteral(operatorHome)}"))`,
    `(deny file-write* (subpath "${escapeSandboxLiteral(cwd)}"))`,
    `(deny file-write* (subpath "${escapeSandboxLiteral(env.HOME)}"))`,
    ...protectedReadPaths.map(
      (path) =>
        `(deny file-read* (subpath "${escapeSandboxLiteral(path)}"))`,
    ),
  ].join("");
  const child = spawn(
    "/usr/bin/sandbox-exec",
    [
      "-p",
      sandboxProfile,
      piBinary,
      "--mode",
      "rpc",
      "--no-session",
    ],
    {
      cwd,
      env: {
        ...env,
        NODE_OPTIONS: `--import=${NETWORK_GUARD_PATH}`,
        MATTY_NETWORK_GUARD_READY: guardReadyPath,
        MATTY_NETWORK_GUARD_VIOLATION: guardViolationPath,
      },
      stdio: ["pipe", "pipe", "pipe"],
    },
  );
  const events = [];
  const waiters = new Set();
  let stdoutBuffer = "";
  let stderr = "";
  let exitState;

  function settleWaiters() {
    for (const waiter of waiters) {
      const match = events.find(waiter.predicate);
      if (match) {
        clearTimeout(waiter.timer);
        waiters.delete(waiter);
        waiter.resolve(match);
      }
    }
  }

  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk) => {
    stdoutBuffer += chunk;
    const lines = stdoutBuffer.split("\n");
    stdoutBuffer = lines.pop() ?? "";
    for (const line of lines) {
      if (!line.trim()) {
        continue;
      }
      try {
        events.push(JSON.parse(line));
      } catch (error) {
        events.push({
          type: "invalid-json",
          line,
          error: error instanceof Error ? error.message : String(error),
        });
      }
    }
    settleWaiters();
  });
  child.stderr.on("data", (chunk) => {
    stderr += chunk;
  });
  child.on("error", (error) => {
    exitState = { error };
    for (const waiter of waiters) {
      clearTimeout(waiter.timer);
      waiter.reject(
        new PhaseError(
          waiter.phase,
          `Pi RPC failed to start: ${error.message}`,
          error,
        ),
      );
    }
    waiters.clear();
  });
  child.on("close", (code, signal) => {
    exitState = { code, signal };
    for (const waiter of waiters) {
      clearTimeout(waiter.timer);
      waiter.reject(
        new PhaseError(
          waiter.phase,
          [
            "Pi RPC exited before the expected event",
            `exit: ${code ?? "none"}${signal ? ` (${signal})` : ""}`,
            `stderr:\n${stderr.trim() || "(empty)"}`,
          ].join("\n"),
        ),
      );
    }
    waiters.clear();
  });

  function waitFor(phase, predicate, timeoutMs = 30_000) {
    const existing = events.find(predicate);
    if (existing) {
      return Promise.resolve(existing);
    }
    if (exitState) {
      return Promise.reject(
        new PhaseError(
          phase,
          `Pi RPC exited before the expected event\nstderr:\n${stderr.trim() || "(empty)"}`,
        ),
      );
    }

    return new Promise((resolveWait, rejectWait) => {
      const waiter = {
        phase,
        predicate,
        resolve: resolveWait,
        reject: rejectWait,
        timer: setTimeout(() => {
          waiters.delete(waiter);
          rejectWait(
            new PhaseError(
              phase,
              `timed out waiting for Pi RPC event\nstderr:\n${stderr.trim() || "(empty)"}`,
            ),
          );
        }, timeoutMs),
      };
      waiters.add(waiter);
    });
  }

  function send(command) {
    child.stdin.write(`${JSON.stringify(command)}\n`);
  }

  async function close() {
    if (exitState) {
      if (exitState.code === 0) {
        return;
      }
      throw new PhaseError(
        "startup",
        `Pi RPC exited with code ${exitState.code ?? "none"}\nstderr:\n${stderr.trim() || "(empty)"}`,
        exitState.error,
      );
    }
    child.stdin.end();
    await new Promise((resolveClose, rejectClose) => {
      const timer = setTimeout(() => {
        child.kill("SIGTERM");
        rejectClose(
          new PhaseError(
            "startup",
            `Pi RPC did not exit cleanly\nstderr:\n${stderr.trim() || "(empty)"}`,
          ),
        );
      }, 10_000);
      child.once("close", (code) => {
        clearTimeout(timer);
        if (code === 0) {
          resolveClose();
          return;
        }
        rejectClose(
          new PhaseError(
            "startup",
            `Pi RPC exited with code ${code}\nstderr:\n${stderr.trim() || "(empty)"}`,
          ),
        );
      });
    });
  }

  return {
    close,
    events,
    guardReadyPath,
    guardViolationPath,
    send,
    waitFor,
  };
}

async function main() {
  const operatorHome = process.env.HOME;
  if (!operatorHome) {
    throw new PhaseError(
      "startup",
      "operator HOME is unavailable; cannot protect real user configuration",
    );
  }
  const sandboxRoot = await mkdtemp(join(tmpdir(), "matty-t01-"));
  const homeRoot = join(sandboxRoot, "home");
  const projectRoot = join(sandboxRoot, "project");
  const piHostRoot = join(sandboxRoot, "pi-host");
  const artifactRoot = join(sandboxRoot, "artifacts");
  const npmCacheRoot = join(sandboxRoot, "npm-cache");
  const temporaryRoot = join(sandboxRoot, "tmp");

  for (const directory of [
    homeRoot,
    projectRoot,
    piHostRoot,
    artifactRoot,
    npmCacheRoot,
    temporaryRoot,
  ]) {
    await mkdir(directory, { recursive: true });
  }

  const isolatedEnv = {
    PATH:
      process.env.PATH ??
      "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
    HOME: homeRoot,
    XDG_CONFIG_HOME: join(homeRoot, ".config"),
    TMPDIR: temporaryRoot,
    LANG: "en_US.UTF-8",
    npm_config_cache: npmCacheRoot,
    npm_config_userconfig: join(homeRoot, ".npmrc"),
    PI_OFFLINE: "1",
    NO_UPDATE_NOTIFIER: "1",
  };

  let succeeded = false;
  try {
    await run("pack", "npm", ["run", "build"], {
      cwd: REPOSITORY_ROOT,
      env: isolatedEnv,
    });
    const packResult = await run(
      "pack",
      "npm",
      [
        "pack",
        REPOSITORY_ROOT,
        "--json",
        "--pack-destination",
        artifactRoot,
      ],
      { cwd: projectRoot, env: isolatedEnv },
    );
    let packMetadata;
    try {
      [packMetadata] = JSON.parse(packResult.stdout);
    } catch (error) {
      throw new PhaseError(
        "pack",
        `npm pack did not return JSON metadata:\n${packResult.stdout.trim()}`,
        error,
      );
    }
    assert.equal(packMetadata.name, "@yargote/matty");
    assert.equal(packMetadata.version, "0.1.0");
    const expectedDistFiles = [
      "dist/adapters/pi-extension.d.ts",
      "dist/adapters/pi-extension.js",
      "dist/application/child-pi-runtime.d.ts",
      "dist/application/child-pi-runtime.js",
      "dist/application/explorer-delegation.d.ts",
      "dist/application/explorer-delegation.js",
      "dist/application/inspection-role-delegation.d.ts",
      "dist/application/inspection-role-delegation.js",
      "dist/application/register-matty.d.ts",
      "dist/application/register-matty.js",
      "dist/application/single-writer.d.ts",
      "dist/application/single-writer.js",
      "dist/application/worker-delegation.d.ts",
      "dist/application/worker-delegation.js",
      "dist/domain/capability-contract.d.ts",
      "dist/domain/capability-contract.js",
      "dist/domain/inspection-guard.d.ts",
      "dist/domain/inspection-guard.js",
      "dist/domain/matty-rules.d.ts",
      "dist/domain/matty-rules.js",
      "dist/domain/package-contract.d.ts",
      "dist/domain/package-contract.js",
      "dist/domain/status.d.ts",
      "dist/domain/status.js",
      "dist/domain/web-capability.d.ts",
      "dist/domain/web-capability.js",
      "dist/domain/worker-guard.d.ts",
      "dist/domain/worker-guard.js",
    ];
    const expectedPackedFiles = [
      "THIRD_PARTY_NOTICES.md",
      ...expectedDistFiles,
      "package.json",
    ].sort();
    assert.deepEqual(
      packMetadata.files.map((file) => file.path).sort(),
      expectedPackedFiles,
      "[T01:pack] packed artifact contents differ from the reviewed runtime",
    );
    assert.equal(
      packMetadata.files.some(
        (file) => file.path === "skills" || file.path.startsWith("skills/"),
      ),
      false,
      "[T01:pack] packed artifact contains skills",
    );
    const artifactPath = join(artifactRoot, packMetadata.filename);
    await access(artifactPath);

    await run(
      "install",
      "npm",
      ["install", "--prefix", piHostRoot, "--ignore-scripts", PI_PACKAGE],
      { cwd: projectRoot, env: isolatedEnv },
    );
    const piBinary = join(piHostRoot, "node_modules", ".bin", "pi");
    const versionResult = await run("install", piBinary, ["--version"], {
      cwd: projectRoot,
      env: isolatedEnv,
    });
    assert.equal(
      versionResult.stdout.trim(),
      "0.83.0",
      `[T01:install] expected Pi 0.83.0, received ${versionResult.stdout.trim()}`,
    );

    const mattySource =
      `npm:@yargote/matty@file:${artifactPath}`;
    await run("install", piBinary, ["install", mattySource], {
      cwd: projectRoot,
      env: isolatedEnv,
    });
    await access(
      join(
        homeRoot,
        ".pi",
        "agent",
        "npm",
        "node_modules",
        "@yargote",
        "matty",
        "dist",
        "adapters",
        "pi-extension.js",
      ),
    );
    const installedMattyRoot = join(
      homeRoot,
      ".pi",
      "agent",
      "npm",
      "node_modules",
    );
    const installedMattyPackage = JSON.parse(
      await readFile(
        join(installedMattyRoot, "@yargote", "matty", "package.json"),
        "utf8",
      ),
    );
    const installedWebPackage = JSON.parse(
      await readFile(
        join(installedMattyRoot, "pi-web-access", "package.json"),
        "utf8",
      ),
    );
    assert.equal(installedMattyPackage.dependencies["pi-web-access"], "0.15.0");
    assert.equal(installedWebPackage.version, "0.15.0");

    await writeFile(
      join(homeRoot, ".pi", "agent", "auth.json"),
      "{}",
      "utf8",
    );
    await writeFile(
      join(projectRoot, "sentinel.txt"),
      "Matty acceptance project sentinel\n",
      "utf8",
    );
    const projectBeforeStartup = await snapshotTree(projectRoot);
    const homeBeforeStartup = await snapshotTree(homeRoot);
    const rpc = startRpc(
      piBinary,
      projectRoot,
      isolatedEnv,
      sandboxRoot,
      operatorHome,
    );

    let rpcFailure;
    try {
      await rpc.waitFor(
        "startup",
        (event) =>
          event.type === "extension_ui_request" &&
          event.method === "notify" &&
          event.message === ACTIVE_HINT,
      );
      rpc.send({ id: "commands", type: "get_commands" });
      const commandsResponse = await rpc.waitFor(
        "command-registration",
        (event) =>
          event.type === "response" &&
          event.id === "commands",
      );
      assert.equal(
        commandsResponse.success,
        true,
        `[T01:command-registration] get_commands failed: ${commandsResponse.error ?? "unknown error"}`,
      );
      assert.ok(
        commandsResponse.data.commands.some(
          (command) =>
            command.name === "matty" && command.source === "extension",
        ),
        "[T01:command-registration] /matty was not registered by the packed extension",
      );
      rpc.send({
        id: "human-status",
        type: "prompt",
        message: "/matty status",
      });
      const humanNotification = await rpc.waitFor(
        "status",
        (event) =>
          event.type === "extension_ui_request" &&
          event.method === "notify" &&
          event.message?.startsWith("Matty 0.1.0\n"),
      );
      assert.match(humanNotification.message, /Pi 0\.83\.0 · certified/);
      assert.match(
        humanNotification.message,
        /Target darwin\/arm64 · certified/,
      );
      assert.match(
        humanNotification.message,
        /Activation active · compatible/,
      );
      assert.match(
        humanNotification.message,
        /Web available · web_search, source_check, fetch_content, get_search_content/,
      );

      rpc.send({
        id: "json-status",
        type: "prompt",
        message: "/matty status --json",
      });
      const jsonNotification = await rpc.waitFor(
        "status",
        (event) => {
          if (
            event.type !== "extension_ui_request" ||
            event.method !== "notify" ||
            !event.message?.startsWith("{")
          ) {
            return false;
          }
          try {
            return JSON.parse(event.message).command === "status";
          } catch {
            return false;
          }
        },
      );
      assert.doesNotMatch(jsonNotification.message, /\u001b\[/);
      const jsonStatus = JSON.parse(jsonNotification.message);
      assert.equal(jsonStatus.schemaVersion, 1);
      assert.deepEqual(jsonStatus.package, {
        name: "@yargote/matty",
        version: "0.1.0",
      });
      assert.deepEqual(jsonStatus.pi, {
        version: "0.83.0",
        certifiedVersions: ["0.83.0"],
        state: "certified",
      });
      assert.equal(jsonStatus.target.platform, "darwin");
      assert.equal(jsonStatus.target.arch, "arm64");
      assert.deepEqual(jsonStatus.activation, {
        state: "active",
        reason: "compatible",
      });
      assert.deepEqual(jsonStatus.web, {
        state: "available",
        tools: [
          "web_search",
          "source_check",
          "fetch_content",
          "get_search_content",
        ],
      });
      assert.equal("catalog" in jsonStatus, false);
    } catch (error) {
      rpcFailure = error;
      throw error;
    } finally {
      try {
        await rpc.close();
      } catch (closeError) {
        if (!rpcFailure) {
          throw closeError;
        }
        process.stderr.write(
          `T01 cleanup also failed: ${
            closeError instanceof Error
              ? closeError.message
              : String(closeError)
          }\n`,
        );
      }
    }

    assert.equal(
      rpc.events.filter(
        (event) =>
          event.type === "extension_ui_request" &&
          event.method === "notify" &&
          event.message === ACTIVE_HINT,
      ).length,
      1,
      "[T01:startup] expected exactly one active hint",
    );
    await access(rpc.guardReadyPath);
    await assert.rejects(
      access(rpc.guardViolationPath),
      "[T01:status] startup or status attempted a network operation",
    );
    const projectAfterStatus = await snapshotTree(projectRoot);
    const homeAfterStatus = await snapshotTree(homeRoot);
    assert.deepEqual(
      projectAfterStatus,
      projectBeforeStartup,
      "[T01:status] Pi startup or /matty status wrote to the project",
    );
    assert.deepEqual(
      homeAfterStatus,
      homeBeforeStartup,
      "[T01:status] Pi startup or /matty status persisted home state",
    );

    for (const forbiddenPath of [
      join(homeRoot, ".matty"),
      join(homeRoot, ".mattyrc"),
      join(projectRoot, ".matty"),
      join(projectRoot, ".mattyrc"),
    ]) {
      await assert.rejects(
        access(forbiddenPath),
        `[T01:status] Matty created forbidden configuration: ${forbiddenPath}`,
      );
    }

    succeeded = true;
    process.stdout.write(
      [
        "T01 packed-package acceptance passed",
        `artifact: ${packMetadata.filename}`,
        "Pi: 0.83.0",
        "target: darwin/arm64",
        "activation: active",
        "Web Capability: available (pi-web-access 0.15.0)",
        "network during startup/status: denied",
        "project writes during startup/status: none",
      ].join("\n") + "\n",
    );
  } catch (error) {
    if (error instanceof PhaseError) {
      throw error;
    }
    throw new PhaseError(
      "assertion",
      error instanceof Error ? error.message : String(error),
      error,
    );
  } finally {
    if (succeeded && !KEEP_SANDBOX) {
      await rm(sandboxRoot, { recursive: true, force: true });
    } else if (!succeeded || KEEP_SANDBOX) {
      process.stderr.write(`T01 sandbox retained at ${sandboxRoot}\n`);
    }
  }
}

await main();
