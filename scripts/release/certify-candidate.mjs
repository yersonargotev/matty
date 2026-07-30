import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
  access,
  chmod,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  writeFile,
} from "node:fs/promises";
import { homedir, tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const REPOSITORY_ROOT = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../..",
);

function parseOutputDirectory(arguments_) {
  const index = arguments_.indexOf("--output-dir");
  const value = index >= 0 ? arguments_[index + 1] : undefined;
  if (!value || arguments_.length !== 2) {
    throw new Error(
      "Usage: node scripts/release/certify-candidate.mjs --output-dir <empty-directory>",
    );
  }
  return resolve(value);
}

async function run(command, args, options) {
  return await new Promise((resolveRun, rejectRun) => {
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
        new Error(`${options.label} timed out after ${options.timeoutMs}ms`),
      );
    }, options.timeoutMs);
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
      process.stdout.write(chunk);
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
      process.stderr.write(chunk);
    });
    child.once("error", (error) => {
      clearTimeout(timer);
      rejectRun(error);
    });
    child.once("close", (code, signal) => {
      clearTimeout(timer);
      if (code === 0) {
        resolveRun({ stdout, stderr });
        return;
      }
      rejectRun(
        new Error(
          `${options.label} failed with ${String(code)}${
            signal ? ` (${signal})` : ""
          }`,
        ),
      );
    });
  });
}

const outputDirectory = parseOutputDirectory(process.argv.slice(2));
assert.equal(
  `${process.platform}/${process.arch}`,
  "darwin/arm64",
  "Matty Core 0.1.0 certification is only valid on macOS Apple Silicon",
);
await mkdir(outputDirectory, { recursive: true });
assert.deepEqual(
  await readdir(outputDirectory),
  [],
  "candidate output directory must be empty",
);

const executionSandbox = await mkdtemp(join(tmpdir(), "matty-candidate-"));
const referenceAuthPath = resolve(
  process.env.MATTY_REFERENCE_AUTH_PATH ??
    join(homedir(), ".pi", "agent", "auth.json"),
);
await access(referenceAuthPath);
const isolatedEnv = {
  ...process.env,
  HOME: join(executionSandbox, "home"),
  XDG_CONFIG_HOME: join(executionSandbox, "home", ".config"),
  TMPDIR: join(executionSandbox, "tmp"),
  npm_config_cache: join(executionSandbox, "npm-cache"),
  npm_config_userconfig: join(executionSandbox, "home", ".npmrc"),
  MATTY_REFERENCE_AUTH_PATH: referenceAuthPath,
};

try {
  for (const directory of [
    isolatedEnv.HOME,
    isolatedEnv.XDG_CONFIG_HOME,
    isolatedEnv.TMPDIR,
    isolatedEnv.npm_config_cache,
  ]) {
    await mkdir(directory, { recursive: true });
  }

  await run("npm", ["run", "typecheck"], {
    cwd: REPOSITORY_ROOT,
    env: isolatedEnv,
    label: "typecheck",
    timeoutMs: 120_000,
  });
  await run("npm", ["test"], {
    cwd: REPOSITORY_ROOT,
    env: isolatedEnv,
    label: "unit tests",
    timeoutMs: 180_000,
  });
  await run("npm", ["run", "build"], {
    cwd: REPOSITORY_ROOT,
    env: isolatedEnv,
    label: "build",
    timeoutMs: 120_000,
  });
  await run("node", ["scripts/release/verify-release-chain.mjs"], {
    cwd: REPOSITORY_ROOT,
    env: isolatedEnv,
    label: "release-chain policy",
    timeoutMs: 30_000,
  });

  const packed = await run(
    "npm",
    [
      "pack",
      REPOSITORY_ROOT,
      "--ignore-scripts",
      "--json",
      "--pack-destination",
      outputDirectory,
    ],
    {
      cwd: executionSandbox,
      env: isolatedEnv,
      label: "candidate pack",
      timeoutMs: 120_000,
    },
  );
  const [metadata] = JSON.parse(packed.stdout);
  assert.equal(metadata.name, "@yargote/matty");
  assert.equal(metadata.version, "0.1.0");
  const artifactPath = join(outputDirectory, metadata.filename);
  await access(artifactPath);
  const artifactDigest = createHash("sha256")
    .update(await readFile(artifactPath))
    .digest("hex");
  await chmod(artifactPath, 0o444);
  const candidateEnv = {
    ...isolatedEnv,
    MATTY_PACKED_ARTIFACT: artifactPath,
  };

  for (const [label, script, timeoutMs] of [
    [
      "exact artifact inspection",
      "scripts/release/inspect-artifact.mjs",
      120_000,
    ],
    ["T01 install/diagnostics/lifecycle", "scripts/acceptance/t01.mjs", 300_000],
    ["T03 runtime", "scripts/acceptance/t03.mjs", 300_000],
    ["T07 roles/guards/contracts", "scripts/acceptance/t07.mjs", 600_000],
    [
      "T04 Reference Model Path",
      "scripts/acceptance/t04-reference-web.mjs",
      300_000,
    ],
  ]) {
    await run("node", [script], {
      cwd: REPOSITORY_ROOT,
      env: candidateEnv,
      label,
      timeoutMs,
    });
  }

  const verifiedArtifactDigest = createHash("sha256")
    .update(await readFile(artifactPath))
    .digest("hex");
  assert.equal(
    verifiedArtifactDigest,
    artifactDigest,
    "the packed artifact changed while the acceptance suite was running",
  );
  await writeFile(
    join(outputDirectory, "SHA256SUMS"),
    `${artifactDigest}  ${metadata.filename}\n`,
    "utf8",
  );
  process.stdout.write(
    [
      "Matty Core 0.1.0 release candidate certified",
      `artifact: ${metadata.filename}`,
      `sha256: ${artifactDigest}`,
      "host: Pi 0.83.0 on darwin/arm64",
      "reference: openai-codex/gpt-5.6-sol via ChatGPT/Codex OAuth",
      "release workflow policy: OIDC stage-only",
      "trusted-publisher activation: pending human bootstrap issue #17",
    ].join("\n") + "\n",
  );
} finally {
  await rm(executionSandbox, { recursive: true, force: true });
}
