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
const PARENT_PROVIDER = "openai-codex";
const PARENT_MODEL = "gpt-5.6-sol";
const SEARCH_MODEL = "gpt-5.4";
const CODEX_HOST = "chatgpt.com";
const CODEX_PATH = "/backend-api/codex/responses";
const REPOSITORY_ROOT = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const ARTIFACT = process.env.MATTY_PACKED_ARTIFACT
  ? resolve(process.env.MATTY_PACKED_ARTIFACT)
  : undefined;
const SOURCE_AUTH_PATH = resolve(
  process.env.MATTY_REFERENCE_AUTH_PATH ??
    join(homedir(), ".pi", "agent", "auth.json"),
);

assert.equal(
  `${process.platform}/${process.arch}`,
  "darwin/arm64",
  "Reference Model Path certification is only valid on macOS Apple Silicon",
);
assert.ok(
  ARTIFACT,
  "MATTY_PACKED_ARTIFACT is required for Reference Model Path certification",
);
await access(ARTIFACT);

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
  assert.equal(actual.digest, expected.digest, "source auth bytes changed");
  assert.equal(actual.mode, expected.mode, "source auth mode changed");
  assert.equal(actual.mtimeNs, expected.mtimeNs, "source auth mtime changed");
  assert.equal(actual.size, expected.size, "source auth size changed");
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
      }\n${sanitize(result.stderr)}`,
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

function sanitize(value) {
  return value
    .replace(/Bearer\s+\S+/gi, "Bearer [REDACTED]")
    .replace(
      /\beyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g,
      "[JWT REDACTED]",
    )
    .replace(/\b(?:sk|sess)-[A-Za-z0-9_-]{20,}\b/g, "[KEY REDACTED]")
    .slice(-4_000);
}

const originalAuth = await snapshot(SOURCE_AUTH_PATH);
const authContents = await readFile(SOURCE_AUTH_PATH);
const parsedAuth = JSON.parse(authContents.toString("utf8"));
assert.equal(
  parsedAuth?.["openai-codex"]?.type,
  "oauth",
  "Reference Model Path requires openai-codex OAuth",
);

const sandbox = await mkdtemp(join(tmpdir(), "matty-t04-reference-"));
const home = join(sandbox, "home");
const agentDir = join(home, ".pi", "agent");
const project = join(sandbox, "project");
const host = join(sandbox, "host");
const npmCache = join(sandbox, "npm-cache");
const temporary = join(sandbox, "tmp");
const copiedAuthPath = join(agentDir, "auth.json");
const observationsPath = join(sandbox, "network-observations.jsonl");
const instrumentationPath = join(sandbox, "observe-codex-fetch.mjs");
const driverPath = join(sandbox, "run-reference-proof.mjs");

for (const directory of [
  agentDir,
  project,
  host,
  npmCache,
  temporary,
]) {
  await mkdir(directory, { recursive: true });
}

const isolatedEnv = {
  ...process.env,
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
delete isolatedEnv.OPENAI_API_KEY;

let verdict;
try {
  await cp(SOURCE_AUTH_PATH, copiedAuthPath);
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
const target = process.env.MATTY_REFERENCE_OBSERVATIONS;
if (target) writeFileSync(target, "");

globalThis.fetch = async function observedFetch(input, init) {
  const url = new URL(
    typeof input === "string" || input instanceof URL ? input : input.url,
  );
  let observation;
  if (url.hostname === ${JSON.stringify(CODEX_HOST)}) {
    const headers = new Headers(
      init?.headers ??
        (typeof input === "object" && "headers" in input ? input.headers : {}),
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
      hasAuthorization: Boolean(headers.get("authorization")),
      hasAccountId: Boolean(headers.get("chatgpt-account-id")),
      originator: headers.get("originator"),
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
      ARTIFACT,
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
  additionalExtensionPaths: [${JSON.stringify(mattyExtension)}],
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
  session.setActiveToolsByName(["web_search"]);
  await session.prompt(
    [
      "Use web_search exactly once with provider=openai, workflow=none,",
      "numResults=5, and domainFilter=nodejs.org to find the latest",
      "stable Node.js release listed on nodejs.org today.",
      "Then answer concisely with a Sources section containing citation URLs.",
    ].join(" "),
  );
  const assistantMessages = session.messages.filter(
    (message) => message.role === "assistant",
  );
  const toolCalls = assistantMessages.flatMap((message) =>
    Array.isArray(message.content)
      ? message.content.filter(
          (part) => part?.type === "toolCall" && part.name === "web_search",
        )
      : [],
  );
  assert.equal(
    toolCalls.length,
    1,
    "Reference Model Path must invoke web_search exactly once",
  );
  const parent = session.messages
    .filter((message) => message.role === "assistant")
    .at(-1);
  assert.ok(parent);
  assert.equal(parent.provider, ${JSON.stringify(PARENT_PROVIDER)});
  assert.equal(parent.model, ${JSON.stringify(PARENT_MODEL)});
  assert.notEqual(parent.stopReason, "error", parent.errorMessage);
  assert.notEqual(parent.stopReason, "aborted");
  const parentText = parent.content
    .filter((part) => part?.type === "text")
    .map((part) => part.text)
    .join("\\n");
  const webToolResults = session.messages.filter(
    (message) =>
      message.role === "toolResult" && message.toolName === "web_search",
  );
  assert.equal(webToolResults.length, 1);
  const toolResultText = webToolResults[0].content
    .filter((part) => part?.type === "text")
    .map((part) => part.text)
    .join("\\n");
  const extractUrls = (text) =>
    [...new Set(
      text
        .replaceAll("\\n", " ")
        .split(" ")
        .filter((token) => token.startsWith("http"))
        .map((token) => token.replace(/[),.;]+$/g, "")),
    )];
  const toolCitationUrls = extractUrls(toolResultText);
  const parentCitationUrls = extractUrls(parentText);
  assert.ok(toolCitationUrls.length > 0, "web_search result lacks source URLs");
  assert.ok(
    parentCitationUrls.some((url) => toolCitationUrls.includes(url)),
    "Reference Model citations must include a URL returned by web_search",
  );
  assert.ok(
    parentText.includes("https://"),
    "Reference Model response must contain a citation URL: " +
      JSON.stringify(parent.content).slice(0, 2_000),
  );
  assert.ok(
    parentText.toLowerCase().includes("source"),
    "Reference Model response must identify its sources",
  );

  const certifiedTools = [
    "web_search",
    "source_check",
    "fetch_content",
    "get_search_content",
  ];
  const allTools = session.getAllTools();
  assert.deepEqual(
    certifiedTools.filter((name) => allTools.some((tool) => tool.name === name)),
    certifiedTools,
  );
  console.log(JSON.stringify({
    parent: {
      provider: parent.provider,
      model: parent.model,
      stopReason: parent.stopReason,
    },
    toolNames: certifiedTools,
    toolCallCount: toolCalls.length,
    toolCitationUrls,
    parentText,
  }));
} finally {
  session.dispose();
}
`,
  );

  const live = await run(process.execPath, [driverPath], {
    cwd: project,
    env: {
      ...isolatedEnv,
      NODE_OPTIONS: `--import=${pathToFileURL(instrumentationPath).href}`,
      MATTY_REFERENCE_OBSERVATIONS: observationsPath,
    },
    timeoutMs: 180_000,
  });
  assert.equal(live.timedOut, false, "Reference Model Path timed out");
  assert.equal(
    live.code,
    0,
    `Reference Model Path failed: ${sanitize(live.stderr)}`,
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
  assert.match(
    driverResult.parentText,
    /https:\/\/[^\s)]+/,
    "Reference Model response lacks a citation URL",
  );
  assert.ok(
    driverResult.parentText.toLowerCase().includes("source"),
    "Reference Model response lacks a Sources section",
  );
  assert.equal(driverResult.toolCallCount, 1);
  const citationUrls = [
    ...new Set(
      [...driverResult.parentText.matchAll(/https:\/\/[^\s)]+/g)].map(
        ([url]) => url.replace(/[.,;]+$/, ""),
      ),
    ),
  ];
  assert.ok(
    citationUrls.some((url) => driverResult.toolCitationUrls.includes(url)),
    "final citations are not linked to the web_search evidence",
  );
  const observations = parseJsonLines(
    await readFile(observationsPath, "utf8"),
  );
  const searchObservation = observations.find(
    (entry) => entry.requestKind === "native-web-search",
  );
  assert.ok(searchObservation, "native subscription web search was not observed");
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
  assertSameSnapshot(await snapshot(SOURCE_AUTH_PATH), originalAuth);

  verdict = {
    result: "PASS",
    artifact: {
      name: `${mattyPackage.name}@${mattyPackage.version}`,
      sha256: digest(await readFile(ARTIFACT)),
    },
    host: {
      pi: `${piPackage.name}@${piPackage.version}`,
      target: `${process.platform}/${process.arch}`,
    },
    referenceModelPath: `${PARENT_PROVIDER}/${PARENT_MODEL}`,
    searchInternalModel: `${PARENT_PROVIDER}/${SEARCH_MODEL}`,
    web: `${webPackage.name}@${webPackage.version}`,
    transport: `https://${CODEX_HOST}${CODEX_PATH}`,
    citationUrls,
    separateOpenAiApiKey: false,
    sourceAuthUnchanged: true,
    browserCookieAccess: false,
    providerConfigWritten: false,
  };
} finally {
  assertSameSnapshot(await snapshot(SOURCE_AUTH_PATH), originalAuth);
  await rm(sandbox, { recursive: true, force: true });
}

process.stdout.write(
  `T04 Reference Model Path acceptance passed\n${JSON.stringify(verdict)}\n`,
);
