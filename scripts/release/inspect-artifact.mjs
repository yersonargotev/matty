import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
  access,
  mkdir,
  mkdtemp,
  readFile,
  rm,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  loadThemeFromPath,
} from "../../node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/theme/theme.js";

const REPOSITORY_ROOT = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const INSTALL_LIFECYCLE_SCRIPTS = [
  "preinstall",
  "install",
  "postinstall",
  "prepublish",
  "prepare",
  "dependencies",
];
const EXPECTED_DIST_FILES = [
  "dist/adapters/pi-delegation-management.d.ts",
  "dist/adapters/pi-delegation-management.js",
  "dist/adapters/pi-extension.d.ts",
  "dist/adapters/pi-extension.js",
  "dist/application/child-control-environment.d.ts",
  "dist/application/child-control-environment.js",
  "dist/application/child-pi-runtime.d.ts",
  "dist/application/child-pi-runtime.js",
  "dist/application/delegation-observer.d.ts",
  "dist/application/delegation-observer.js",
  "dist/application/delegation-presentation.d.ts",
  "dist/application/delegation-presentation.js",
  "dist/application/delegation-registry.d.ts",
  "dist/application/delegation-registry.js",
  "dist/application/delegation-scheduler.d.ts",
  "dist/application/delegation-scheduler.js",
  "dist/application/explorer-delegation.d.ts",
  "dist/application/explorer-delegation.js",
  "dist/application/inspection-role-delegation.d.ts",
  "dist/application/inspection-role-delegation.js",
  "dist/application/register-matty.d.ts",
  "dist/application/register-matty.js",
  "dist/application/researcher-delegation.d.ts",
  "dist/application/researcher-delegation.js",
  "dist/application/single-writer.d.ts",
  "dist/application/single-writer.js",
  "dist/application/worker-delegation.d.ts",
  "dist/application/worker-delegation.js",
  "dist/domain/capability-contract.d.ts",
  "dist/domain/capability-contract.js",
  "dist/domain/child-execution-activity.d.ts",
  "dist/domain/child-execution-activity.js",
  "dist/domain/commit-sha.d.ts",
  "dist/domain/commit-sha.js",
  "dist/domain/delegation-group.d.ts",
  "dist/domain/delegation-group.js",
  "dist/domain/inspection-guard.d.ts",
  "dist/domain/inspection-guard.js",
  "dist/domain/matty-guidance.d.ts",
  "dist/domain/matty-guidance.js",
  "dist/domain/matty-rules.d.ts",
  "dist/domain/matty-rules.js",
  "dist/domain/package-contract.d.ts",
  "dist/domain/package-contract.js",
  "dist/domain/research-paths.d.ts",
  "dist/domain/research-paths.js",
  "dist/domain/research-workspace.d.ts",
  "dist/domain/research-workspace.js",
  "dist/domain/review-scope.d.ts",
  "dist/domain/review-scope.js",
  "dist/domain/status.d.ts",
  "dist/domain/status.js",
  "dist/domain/web-capability.d.ts",
  "dist/domain/web-capability.js",
  "dist/domain/worker-completion.d.ts",
  "dist/domain/worker-completion.js",
  "dist/domain/worker-guard.d.ts",
  "dist/domain/worker-guard.js",
];
const EXPECTED_PACKED_FILES = [
  "LICENSE",
  "README.md",
  "PRODUCTION_DEPENDENCY_LIFECYCLES.json",
  "THIRD_PARTY_NOTICES.md",
  "THIRD_PARTY_PROVENANCE.json",
  ...EXPECTED_DIST_FILES,
  "package.json",
  "themes/matty-catppuccin-mocha.json",
].sort();

async function readJson(path) {
  return JSON.parse(await readFile(path, "utf8"));
}

async function run(command, args, options) {
  return new Promise((resolveRun, rejectRun) => {
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
    child.on("error", rejectRun);
    child.on("close", (code) => {
      if (code === 0) {
        resolveRun({ stdout, stderr });
        return;
      }
      rejectRun(
        new Error(
          `${command} ${args.join(" ")} failed (${code})\n${stdout}${stderr}`,
        ),
      );
    });
  });
}

function packageNameFromPath(packagePath) {
  const segments = packagePath.split("node_modules/").at(-1)?.split("/") ?? [];
  return segments[0]?.startsWith("@")
    ? `${segments[0]}/${segments[1]}`
    : segments[0];
}

function lifecycleIdentity(entry) {
  return {
    packagePath: entry.packagePath,
    name: entry.name,
    version: entry.version,
    resolved: entry.resolved,
    integrity: entry.integrity,
    license: entry.license,
    scripts: entry.scripts,
  };
}

async function inspectLifecycleInventory(lockfile, inventory) {
  assert.equal(inventory.schemaVersion, 1);
  assert.equal(inventory.lockfileVersion, lockfile.lockfileVersion);

  const actual = [];
  for (const [packagePath, metadata] of Object.entries(lockfile.packages)) {
    if (!packagePath || metadata.dev || !metadata.hasInstallScript) {
      continue;
    }
    const manifestPath = join(REPOSITORY_ROOT, packagePath, "package.json");
    await access(manifestPath);
    const manifest = await readJson(manifestPath);
    const scripts = Object.fromEntries(
      INSTALL_LIFECYCLE_SCRIPTS
        .filter((name) => typeof manifest.scripts?.[name] === "string")
        .map((name) => [name, manifest.scripts[name]]),
    );
    actual.push({
      packagePath,
      name: packageNameFromPath(packagePath),
      version: metadata.version,
      resolved: metadata.resolved,
      integrity: metadata.integrity,
      license: metadata.license,
      scripts,
    });
  }
  actual.sort((left, right) =>
    left.packagePath.localeCompare(right.packagePath)
  );

  for (const entry of inventory.entries) {
    assert.equal(typeof entry.effects, "string");
    assert.ok(entry.effects.length > 0);
    assert.equal(typeof entry.justification, "string");
    assert.ok(entry.justification.length > 0);
  }
  const reviewed = inventory.entries
    .map(lifecycleIdentity)
    .sort((left, right) => left.packagePath.localeCompare(right.packagePath));
  assert.deepEqual(
    actual,
    reviewed,
    "production dependency lifecycle scripts differ from the reviewed inventory",
  );
}

function inspectPackageMetadata(manifest) {
  assert.equal(manifest.name, "@yargote/matty");
  assert.equal(manifest.license, "MIT");
  assert.deepEqual(manifest.repository, {
    type: "git",
    url: "git+https://github.com/yersonargotev/matty.git",
  });
  assert.equal(
    manifest.homepage,
    "https://github.com/yersonargotev/matty#readme",
  );
  assert.deepEqual(manifest.bugs, {
    url: "https://github.com/yersonargotev/matty/issues",
  });
  assert.deepEqual(manifest.publishConfig, {
    access: "public",
    provenance: true,
  });
  assert.deepEqual(manifest.files, [
    "dist",
    "themes",
    "PRODUCTION_DEPENDENCY_LIFECYCLES.json",
    "THIRD_PARTY_NOTICES.md",
    "THIRD_PARTY_PROVENANCE.json",
  ]);
  assert.deepEqual(manifest.pi, {
    extensions: ["./dist/adapters/pi-extension.js"],
    themes: ["./themes/matty-catppuccin-mocha.json"],
  });
  assert.deepEqual(manifest.peerDependencies, {
    "@earendil-works/pi-coding-agent": "0.84.2",
  });
  for (const name of INSTALL_LIFECYCLE_SCRIPTS) {
    assert.equal(
      Object.hasOwn(manifest.scripts ?? {}, name),
      false,
      `Matty declares forbidden installation lifecycle script ${name}`,
    );
  }
}

function inspectThirdPartyProvenance(provenance, lockfile) {
  assert.equal(provenance.schemaVersion, 1);
  assert.deepEqual(provenance.adaptedSources, [
    {
      component: "Pi coding agent subagent example",
      repository: "https://github.com/earendil-works/pi-mono",
      commit: "845d6ff1f6643aba440341cce877ce1c43ebbc39",
      path: "packages/coding-agent/examples/extensions/subagent",
      license: "MIT",
      notice: "THIRD_PARTY_NOTICES.md",
    },
    {
      component: "Catppuccin Mocha palette",
      repository: "https://github.com/catppuccin/palette",
      commit: "07d02aa110ef9eb7e7427afca5c73ba9cf7f8ebd",
      path: "palette.json",
      license: "MIT",
      notice: "THIRD_PARTY_NOTICES.md",
    },
  ]);

  const paths = new Map([
    ["@earendil-works/pi-coding-agent", "node_modules/@earendil-works/pi-coding-agent"],
    ["jiti", "node_modules/jiti"],
    ["pi-web-access", "node_modules/pi-web-access"],
  ]);
  for (const runtimePackage of provenance.runtimePackages) {
    const packagePath = paths.get(runtimePackage.name);
    assert.ok(packagePath, `unexpected provenance package ${runtimePackage.name}`);
    const locked = lockfile.packages[packagePath];
    assert.ok(locked, `missing lock entry ${packagePath}`);
    assert.deepEqual(
      {
        version: runtimePackage.version,
        resolved: runtimePackage.resolved,
        integrity: runtimePackage.integrity,
        license: runtimePackage.license,
      },
      {
        version: locked.version,
        resolved: locked.resolved,
        integrity: locked.integrity,
        license: locked.license,
      },
      `provenance differs from ${packagePath}`,
    );
  }
  assert.deepEqual(
    provenance.runtimePackages.map((entry) => entry.name).sort(),
    [...paths.keys()].sort(),
  );
}

async function main() {
  const lockfile = await readJson(join(REPOSITORY_ROOT, "package-lock.json"));
  const sandbox = await mkdtemp(join(tmpdir(), "matty-artifact-"));
  const artifactRoot = join(sandbox, "artifacts");
  const extractedRoot = join(sandbox, "extracted");
  const isolatedEnv = {
    ...process.env,
    HOME: join(sandbox, "home"),
    XDG_CONFIG_HOME: join(sandbox, "home", ".config"),
    TMPDIR: join(sandbox, "tmp"),
    npm_config_cache: join(sandbox, "npm-cache"),
    npm_config_userconfig: join(sandbox, "home", ".npmrc"),
  };
  try {
    await mkdir(artifactRoot, { recursive: true });
    await mkdir(extractedRoot, { recursive: true });
    const providedArtifact = process.env.MATTY_PACKED_ARTIFACT
      ? resolve(process.env.MATTY_PACKED_ARTIFACT)
      : undefined;
    const packed = await run(
      "npm",
      providedArtifact
        ? ["pack", providedArtifact, "--ignore-scripts", "--dry-run", "--json"]
        : [
          "pack",
          REPOSITORY_ROOT,
          "--ignore-scripts",
          "--json",
          "--pack-destination",
          artifactRoot,
        ],
      { cwd: sandbox, env: isolatedEnv },
    );
    const [metadata] = JSON.parse(packed.stdout);
    const artifactPath = providedArtifact ??
      join(artifactRoot, metadata.filename);
    await access(artifactPath);
    await run(
      "tar",
      ["-xzf", artifactPath, "-C", extractedRoot],
      { cwd: sandbox, env: isolatedEnv },
    );
    const packageRoot = join(extractedRoot, "package");
    const manifest = await readJson(join(packageRoot, "package.json"));
    const inventory = await readJson(
      join(packageRoot, "PRODUCTION_DEPENDENCY_LIFECYCLES.json"),
    );
    const provenance = await readJson(
      join(packageRoot, "THIRD_PARTY_PROVENANCE.json"),
    );
    const license = await readFile(join(packageRoot, "LICENSE"), "utf8");
    const notices = await readFile(
      join(packageRoot, "THIRD_PARTY_NOTICES.md"),
      "utf8",
    );

    inspectPackageMetadata(manifest);
    await inspectLifecycleInventory(lockfile, inventory);
    inspectThirdPartyProvenance(provenance, lockfile);
    assert.match(
      license,
      /^MIT License\n\nCopyright \(c\) 2026 Yerson Argote\n/,
    );
    assert.match(notices, /^# Third-party notices\n/);
    assert.match(notices, /Copyright \(c\) 2021 Catppuccin/);
    const themePath = join(
      packageRoot,
      "themes",
      "matty-catppuccin-mocha.json",
    );
    const themeJson = await readJson(themePath);
    assert.equal(themeJson.name, "matty-catppuccin-mocha");
    assert.deepEqual(Object.keys(themeJson.export).sort(), [
      "cardBg",
      "infoBg",
      "pageBg",
    ]);
    assert.doesNotThrow(() => loadThemeFromPath(themePath, "truecolor"));
    assert.doesNotThrow(() => loadThemeFromPath(themePath, "256color"));
    assert.equal(metadata.name, manifest.name);
    assert.equal(metadata.version, manifest.version);
    assert.deepEqual(
      metadata.files.map((file) => file.path).sort(),
      EXPECTED_PACKED_FILES,
      "packed artifact contents differ from the reviewed allowlist",
    );
    assert.equal(
      metadata.files.some(
        (file) =>
          file.path === "skills" ||
          file.path.startsWith("skills/") ||
          (file.path.endsWith(".ts") && !file.path.endsWith(".d.ts")) ||
          file.path.startsWith("scripts/"),
      ),
      false,
      "packed artifact contains skills, source, or install-time generators",
    );
    const artifact = await readFile(artifactPath);
    const digest = createHash("sha256").update(artifact).digest("hex");
    process.stdout.write(
      [
        "Install-Safe Artifact inspection passed",
        `artifact: ${metadata.filename}`,
        `sha256: ${digest}`,
        `files: ${metadata.entryCount}`,
        `reviewed production lifecycle entries: ${inventory.entries.length}`,
      ].join("\n") + "\n",
    );
  } finally {
    await rm(sandbox, { recursive: true, force: true });
  }
}

await main();
