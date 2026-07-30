// THROWAWAY PROTOTYPE: proves T04 against real packed packages and OAuth.
// Run once with: node prototypes/t04-subscription-web-search.mjs
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
  access,
  chmod,
  cp,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { homedir, tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const PI_PACKAGE = "@earendil-works/pi-coding-agent@0.83.0";
const WEB_PACKAGE = "pi-web-access@0.15.0";
const PARENT_PROVIDER = "openai-codex";
const PARENT_MODEL = "gpt-5.6-sol";
const SEARCH_MODEL = "gpt-5.4";
const CODEX_HOST = "chatgpt.com";
const CODEX_PATH = "/backend-api/codex/responses";
const repositoryRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "..",
);
const sourceAuthPath = join(homedir(), ".pi", "agent", "auth.json");

function digest(value) {
  return createHash("sha256").update(value).digest("hex");
}

async function snapshot(file) {
  const [contents, metadata] = await Promise.all([
    readFile(file),
    stat(file, { bigint: true }),
  ]);
  return {
    digest: digest(contents),
    mode: Number(metadata.mode & 0o777n),
    mtimeNs: metadata.mtimeNs,
    size: metadata.size,
  };
}

function assertSameSnapshot(actual, expected) {
  assert.equal(actual.digest, expected.digest, "operator auth bytes changed");
  assert.equal(actual.mode, expected.mode, "operator auth mode changed");
  assert.equal(actual.mtimeNs, expected.mtimeNs, "operator auth mtime changed");
  assert.equal(actual.size, expected.size, "operator auth size changed");
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
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      child.kill("SIGTERM");
    }, options.timeoutMs ?? 180_000);
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.once("error", (error) => {
      clearTimeout(timer);
      rejectRun(error);
    });
    child.once("close", (code, signal) => {
      clearTimeout(timer);
      resolveRun({ code, signal, stdout, stderr, timedOut });
    });
  });
}

async function runChecked(command, args, options) {
  const result = await run(command, args, options);
  if (result.code !== 0) {
    throw new Error(
      `${command} failed with ${String(result.code)}${
        result.timedOut ? " after timeout" : ""
      }`,
    );
  }
  return result;
}

function parseJsonLines(value) {
  return value
    .split("\n")
    .filter((line) => line.trim().length > 0)
    .map((line) => JSON.parse(line));
}

function sanitizedDiagnostic(value) {
  return value
    .replace(/Bearer\s+\S+/gi, "Bearer [REDACTED]")
    .replace(/\beyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g, "[JWT REDACTED]")
    .replace(/\b(?:sk|sess)-[A-Za-z0-9_-]{20,}\b/g, "[KEY REDACTED]")
    .slice(-4_000);
}

const originalAuth = await snapshot(sourceAuthPath);
const authContents = await readFile(sourceAuthPath);
const parsedAuth = JSON.parse(authContents.toString("utf8"));
assert.equal(
  parsedAuth?.["openai-codex"]?.type,
  "oauth",
  "T04 requires an existing openai-codex OAuth login",
);

const sandbox = await mkdtemp(join(tmpdir(), "matty-t04-prototype-"));
const home = join(sandbox, "home");
const agentDir = join(home, ".pi", "agent");
const project = join(sandbox, "project");
const host = join(sandbox, "host");
const artifacts = join(sandbox, "artifacts");
const npmCache = join(sandbox, "npm-cache");
const temporary = join(sandbox, "tmp");
const copiedAuthPath = join(agentDir, "auth.json");
const observationsPath = join(sandbox, "network-observations.jsonl");
const instrumentationPath = join(sandbox, "observe-codex-fetch.mjs");
const driverPath = join(sandbox, "run-pi-proof.mjs");

for (const directory of [
  agentDir,
  project,
  host,
  artifacts,
  npmCache,
  temporary,
]) {
  await mkdir(directory, { recursive: true });
}

const isolatedEnv = {
  PATH:
    process.env.PATH ??
    "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
  HOME: home,
  XDG_CONFIG_HOME: join(home, ".config"),
  PI_CODING_AGENT_DIR: agentDir,
  PI_OFFLINE: "1",
  PI_TELEMETRY: "0",
  NO_UPDATE_NOTIFIER: "1",
  TMPDIR: temporary,
  npm_config_cache: npmCache,
  npm_config_userconfig: join(home, ".npmrc"),
};

let verdict;
try {
  await cp(sourceAuthPath, copiedAuthPath);
  await chmod(copiedAuthPath, 0o600);
  await writeFile(
    join(agentDir, "settings.json"),
    `${JSON.stringify({ transport: "sse" }, null, 2)}\n`,
  );
  await writeFile(
    instrumentationPath,
    `
import { appendFileSync, writeFileSync } from "node:fs";

const originalFetch = globalThis.fetch;
const target = process.env.MATTY_T04_OBSERVATIONS;
if (target) writeFileSync(target, "");

function headerValue(headers, name) {
  return new Headers(headers).get(name);
}

globalThis.fetch = async function observedFetch(input, init) {
  const url = new URL(
    typeof input === "string" || input instanceof URL ? input : input.url,
  );
  let observation;
  if (
    url.hostname === ${JSON.stringify(CODEX_HOST)} ||
    url.hostname === "api.openai.com"
  ) {
    const headers = init?.headers ?? (
      typeof input === "object" && "headers" in input ? input.headers : {}
    );
    let body = {};
    if (typeof init?.body === "string") {
      try {
        body = JSON.parse(init.body);
      } catch {
        body = {};
      }
    }
    const nativeSearch = Array.isArray(body.tools) &&
      body.tools.some((tool) => tool?.type === "web_search");
    observation = {
      host: url.hostname,
      path: url.pathname,
      model: typeof body.model === "string" ? body.model : null,
      requestKind: nativeSearch ? "native-web-search" : "parent-model",
      hasAuthorization: Boolean(headerValue(headers, "authorization")),
      hasAccountId: Boolean(headerValue(headers, "chatgpt-account-id")),
      originator: headerValue(headers, "originator"),
      hasNativeWebSearchTool: nativeSearch,
      store: body.store ?? null,
      stream: body.stream ?? null,
    };
  }

  try {
    const response = await originalFetch.call(this, input, init);
    if (observation && target) {
      appendFileSync(
        target,
        JSON.stringify({
          ...observation,
          responseStatus: response.status,
          responseOk: response.ok,
        }) + "\\n",
      );
    }
    return response;
  } catch (error) {
    if (observation && target) {
      appendFileSync(
        target,
        JSON.stringify({
          ...observation,
          networkError: error instanceof Error ? error.name : "unknown",
        }) + "\\n",
      );
    }
    throw error;
  }
};
`,
  );

  await runChecked("npm", ["run", "build"], {
    cwd: repositoryRoot,
    env: isolatedEnv,
  });
  const packed = await runChecked(
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
  const mattyArtifact = join(artifacts, metadata.filename);
  await access(mattyArtifact);

  await runChecked(
    "npm",
    [
      "install",
      "--prefix",
      host,
      "--ignore-scripts",
      "--no-audit",
      "--no-fund",
      "--no-save",
      PI_PACKAGE,
      WEB_PACKAGE,
      mattyArtifact,
    ],
    { cwd: project, env: isolatedEnv },
  );

  const piPackage = JSON.parse(
    await readFile(
      join(
        host,
        "node_modules",
        "@earendil-works",
        "pi-coding-agent",
        "package.json",
      ),
      "utf8",
    ),
  );
  const webPackage = JSON.parse(
    await readFile(
      join(host, "node_modules", "pi-web-access", "package.json"),
      "utf8",
    ),
  );
  const mattyPackage = JSON.parse(
    await readFile(
      join(host, "node_modules", "@yargote", "matty", "package.json"),
      "utf8",
    ),
  );
  assert.equal(piPackage.version, "0.83.0");
  assert.equal(webPackage.version, "0.15.0");
  assert.equal(mattyPackage.version, "0.1.0");

  const mattyExtension = join(
    host,
    "node_modules",
    "@yargote",
    "matty",
    "dist",
    "adapters",
    "pi-extension.js",
  );
  const webExtension = join(
    host,
    "node_modules",
    "pi-web-access",
    "index.ts",
  );
  const piSdk = join(
    host,
    "node_modules",
    "@earendil-works",
    "pi-coding-agent",
    "dist",
    "index.js",
  );
  await writeFile(
    driverPath,
    `
import assert from "node:assert/strict";
import {
  createAgentSession,
  DefaultResourceLoader,
  ModelRuntime,
  SessionManager,
  SettingsManager,
} from ${JSON.stringify(pathToFileURL(piSdk).href)};

const cwd = ${JSON.stringify(project)};
const agentDir = ${JSON.stringify(agentDir)};
const settingsManager = SettingsManager.inMemory(
  { transport: "sse" },
  { projectTrusted: true },
);
const resourceLoader = new DefaultResourceLoader({
  cwd,
  agentDir,
  settingsManager,
  additionalExtensionPaths: [
    ${JSON.stringify(mattyExtension)},
    ${JSON.stringify(webExtension)},
  ],
  noSkills: true,
  noPromptTemplates: true,
  noThemes: true,
  noContextFiles: true,
});
await resourceLoader.reload({ resolveProjectTrust: async () => true });
assert.deepEqual(resourceLoader.getExtensions().errors, []);

const modelRuntime = await ModelRuntime.create({
  authPath: ${JSON.stringify(copiedAuthPath)},
  modelsPath: null,
  allowModelNetwork: false,
});
const model = modelRuntime.getModel(
  ${JSON.stringify(PARENT_PROVIDER)},
  ${JSON.stringify(PARENT_MODEL)},
);
assert.ok(model);

const { session } = await createAgentSession({
  cwd,
  agentDir,
  modelRuntime,
  model,
  thinkingLevel: "low",
  resourceLoader,
  settingsManager,
  sessionManager: SessionManager.inMemory(cwd),
});

try {
  await session.bindExtensions({
    mode: "print",
    onError: ({ error }) => {
      throw error;
    },
  });
  session.setActiveToolsByName([]);
  let parent;
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    await session.prompt("Reply only with T04_PARENT_OK.");
    parent = session.messages
      .filter((message) => message.role === "assistant")
      .at(-1);
    if (parent?.stopReason !== "error") break;
    if (
      !parent.errorMessage?.toLowerCase().includes("overloaded") ||
      attempt === 3
    ) {
      break;
    }
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  assert.ok(parent);
  assert.equal(parent.provider, ${JSON.stringify(PARENT_PROVIDER)});
  assert.equal(parent.model, ${JSON.stringify(PARENT_MODEL)});
  assert.notEqual(
    parent.stopReason,
    "error",
    parent.errorMessage ?? "parent model returned an error",
  );
  assert.notEqual(parent.stopReason, "aborted");

  const allTools = session.getAllTools();
  const certifiedTools = [
    "web_search",
    "source_check",
    "fetch_content",
    "get_search_content",
  ];
  for (const name of certifiedTools) {
    assert.ok(
      allTools.some((tool) => tool.name === name),
      \`missing \${name}; loaded: \${allTools.map((tool) => tool.name).join(",")}\`,
    );
  }
  const webSearch = session.getToolDefinition("web_search");
  assert.ok(webSearch);
  const result = await webSearch.execute(
    "t04-deterministic-search",
    {
      query:
        "What is the latest stable Node.js release listed on nodejs.org today?",
      numResults: 5,
      domainFilter: ["nodejs.org"],
      provider: "openai",
      workflow: "none",
    },
    undefined,
    undefined,
    session.extensionRunner.createContext(),
  );
  const toolText = result.content
    .filter((part) => part?.type === "text")
    .map((part) => part.text)
    .join("\\n");
  console.log(JSON.stringify({
    parent: {
      provider: parent.provider,
      model: parent.model,
      stopReason: parent.stopReason,
    },
    toolNames: certifiedTools,
    toolText,
  }));
} finally {
  session.dispose();
}
`,
  );
  const liveEnv = {
    ...isolatedEnv,
    NODE_OPTIONS: `--import=${pathToFileURL(instrumentationPath).href}`,
    MATTY_T04_OBSERVATIONS: observationsPath,
  };
  assert.equal(liveEnv.OPENAI_API_KEY, undefined);

  const live = await run(
    process.execPath,
    [driverPath],
    {
      cwd: project,
      env: liveEnv,
      timeoutMs: 180_000,
    },
  );
  assert.equal(live.timedOut, false, "Pi live proof timed out");
  assert.equal(
    live.code,
    0,
    `Pi live proof failed: ${sanitizedDiagnostic(live.stderr)}`,
  );

  const [driverResult] = parseJsonLines(live.stdout);
  assert.equal(driverResult.parent.provider, PARENT_PROVIDER);
  assert.equal(driverResult.parent.model, PARENT_MODEL);
  assert.deepEqual(driverResult.toolNames, [
    "web_search",
    "source_check",
    "fetch_content",
    "get_search_content",
  ]);
  const toolText = driverResult.toolText;
  assert.match(toolText, /https:\/\/[^\s)]+/, "tool result lacks a citation URL");
  assert.match(toolText, /Sources?:/i, "tool result lacks a Sources section");
  const observations = parseJsonLines(
    await readFile(observationsPath, "utf8"),
  );
  const searchObservation = observations.find(
    (entry) => entry.requestKind === "native-web-search",
  );
  assert.ok(
    searchObservation,
    `native web-search transport was not observed: ${JSON.stringify(
      observations,
    )}`,
  );
  assert.equal(searchObservation.host, CODEX_HOST);
  assert.equal(searchObservation.path, CODEX_PATH);
  assert.equal(searchObservation.model, SEARCH_MODEL);
  assert.equal(searchObservation.hasAuthorization, true);
  assert.equal(searchObservation.hasAccountId, true);
  assert.equal(searchObservation.originator, "pi");
  assert.equal(searchObservation.hasNativeWebSearchTool, true);
  assert.equal(searchObservation.store, false);
  assert.equal(searchObservation.stream, true);
  assert.equal(searchObservation.responseOk, true);

  await assert.rejects(access(join(agentDir, "web-search.json")));
  await assert.rejects(access(join(home, ".matty")));
  await assert.rejects(access(join(project, ".matty")));
  assertSameSnapshot(await snapshot(sourceAuthPath), originalAuth);

  verdict = {
    result: "PASS",
    packed: {
      matty: `${mattyPackage.name}@${mattyPackage.version}`,
      pi: `${piPackage.name}@${piPackage.version}`,
      web: `${webPackage.name}@${webPackage.version}`,
    },
    parent: `${PARENT_PROVIDER}/${PARENT_MODEL}`,
    searchFallback: `${PARENT_PROVIDER}/${SEARCH_MODEL}`,
    transport: `https://${CODEX_HOST}${CODEX_PATH}`,
    citationObserved: true,
    separateOpenAiApiKey: false,
    operatorAuthUnchanged: true,
    browserCookieAccess: false,
    providerConfigWritten: false,
  };
} finally {
  assertSameSnapshot(await snapshot(sourceAuthPath), originalAuth);
  await rm(sandbox, { recursive: true, force: true });
}

console.log(JSON.stringify(verdict, null, 2));
