import assert from "node:assert/strict";
import test from "node:test";

import {
  createStatusSnapshot,
  renderDoctorHuman,
  renderDoctorJson,
  renderStatusHuman,
  renderStatusJson,
} from "../src/domain/status.ts";

const ACTIVE = {
  state: "active",
  reason: "compatible",
  codes: [],
} as const;
const UNCERTIFIED = {
  state: "degraded",
  reason: "unsupported-host",
  codes: ["host-uncertified"],
} as const;

test("status activates on a certified host", () => {
  const snapshot = createStatusSnapshot({
    packageVersion: "0.1.0",
    piVersion: "0.83.0",
    platform: "darwin",
    arch: "arm64",
    activation: ACTIVE,
    activeModel: {
      provider: "openai-codex",
      model: "gpt-5.6-sol",
      authentication: "chatgpt-codex-subscription",
    },
    subagentRuntimeAvailable: true,
    web: {
      state: "available",
      registeredTools: [
        "web_search",
        "source_check",
        "fetch_content",
        "get_search_content",
      ],
    },
  });

  assert.deepEqual(snapshot, {
    schemaVersion: 1,
    command: "status",
    package: {
      name: "@yargote/matty",
      version: "0.1.0",
    },
    pi: {
      version: "0.83.0",
      certifiedVersions: ["0.83.0"],
      state: "certified",
    },
    target: {
      platform: "darwin",
      arch: "arm64",
      certifiedTargets: ["darwin/arm64"],
      state: "certified",
    },
    referenceModelPath: {
      provider: "openai-codex",
      model: "gpt-5.6-sol",
      authentication: "chatgpt-codex-subscription",
      state: "verified",
    },
    activation: {
      state: "active",
      reason: "compatible",
      codes: [],
    },
    subagentRuntime: {
      state: "available",
      processIsolation: "child-process",
    },
    roles: {
      state: "available",
      available: [
        "explorer",
        "designer",
        "reviewer",
        "researcher",
        "worker",
      ],
    },
    inspectionGuard: {
      state: "best-effort",
      securityBoundary: false,
    },
    workerGuard: {
      state: "best-effort",
      securityBoundary: false,
      singleWriter: true,
    },
    mattyRules: {
      schemaVersion: 1,
      state: "active",
    },
    capabilityContracts: {
      schemaVersion: 1,
      state: "available",
      ids: [
        "delegate-explorer",
        "delegate-designer",
        "delegate-reviewer",
        "delegate-researcher",
        "delegate-worker",
        "delegate-group",
        "parent-web",
      ],
    },
    concurrency: {
      maxTasksPerCall: 8,
      maxActiveChildren: 4,
      singleWriter: true,
      activeChildren: 0,
      queuedChildren: 0,
    },
    web: {
      state: "available",
      tools: [
        "web_search",
        "source_check",
        "fetch_content",
        "get_search_content",
      ],
    },
    diagnostics: [],
  });

  assert.equal(
    renderStatusHuman(snapshot),
    [
      "Matty 0.1.0",
      "Host Pi 0.83.0 · certified",
      "Target darwin/arm64 · certified",
      "Reference Model Path openai-codex/gpt-5.6-sol · verified",
      "Activation active · compatible",
      "Subagent Runtime available · child-process",
      "Roles available · explorer, designer, reviewer, researcher, worker",
      "Inspection Guard best-effort · not a security sandbox",
      "Worker Guard best-effort · Single Writer · not a security sandbox",
      "Matty Rules v1 · active",
      "Capability Contracts v1 · available · delegate-explorer, delegate-designer, delegate-reviewer, delegate-researcher, delegate-worker, delegate-group, parent-web",
      "Concurrency 8 accepted · 4 max active · 0 active · 0 queued · Single Writer",
      "Web Capability available · web_search, source_check, fetch_content, get_search_content",
    ].join("\n"),
  );
  assert.deepEqual(JSON.parse(renderStatusJson(snapshot)), snapshot);
});

test("status degrades on an unsupported host", () => {
  const snapshot = createStatusSnapshot({
    packageVersion: "0.1.0",
    piVersion: "0.84.0",
    platform: "linux",
    arch: "x64",
    activation: UNCERTIFIED,
    web: {
      state: "unavailable",
      registeredTools: [],
    },
  });

  assert.deepEqual(snapshot.activation, {
    state: "degraded",
    reason: "unsupported-host",
    codes: ["host-uncertified"],
  });
  assert.equal(snapshot.pi.state, "unsupported");
  assert.equal(snapshot.target.state, "unsupported");
  assert.deepEqual(snapshot.web, {
    state: "unavailable",
    tools: [],
  });
});

test("status reports a locally derived degraded Web Capability", () => {
  const snapshot = createStatusSnapshot({
    packageVersion: "0.1.0",
    piVersion: "0.83.0",
    platform: "darwin",
    arch: "arm64",
    activation: ACTIVE,
    web: {
      state: "degraded",
      registeredTools: ["fetch_content", "get_search_content"],
    },
  });

  assert.deepEqual(snapshot.web, {
    state: "degraded",
    tools: ["fetch_content", "get_search_content"],
  });
  assert.match(renderStatusHuman(snapshot), /Web Capability degraded/);
});

test("a non-reference model is unverified without degrading activation", () => {
  const snapshot = createStatusSnapshot({
    packageVersion: "0.1.0",
    piVersion: "0.83.0",
    platform: "darwin",
    arch: "arm64",
    activation: ACTIVE,
    activeModel: {
      provider: "another-provider",
      model: "another-model",
    },
    subagentRuntimeAvailable: true,
    web: {
      state: "available",
      registeredTools: [
        "web_search",
        "source_check",
        "fetch_content",
        "get_search_content",
      ],
    },
  });

  assert.equal(snapshot.referenceModelPath.state, "unverified");
  assert.deepEqual(snapshot.activation, {
    state: "active",
    reason: "compatible",
    codes: [],
  });
  assert.deepEqual(
    snapshot.diagnostics.map((diagnostic) => diagnostic.code),
    ["reference-model-unverified"],
  );
});

test("diagnostics omit injected secrets, content, URLs, paths, and unknown fields", () => {
  const forbidden = [
    "token-secret",
    "system prompt contents",
    "private research contents",
    "https://user:password@example.invalid/private?token=secret",
    "/Users/private/project/secret.txt",
    "raw provider stderr",
  ];
  const snapshot = createStatusSnapshot({
    packageVersion: "0.1.0",
    piVersion: "0.83.0",
    platform: "darwin",
    arch: "arm64",
    activation: ACTIVE,
    activeModel: {
      provider: "openai-codex",
      model: "gpt-5.6-sol",
      authentication: "chatgpt-codex-subscription",
    },
    subagentRuntimeAvailable: true,
    web: {
      state: "degraded",
      registeredTools: [
        "fetch_content",
        "https://user:password@example.invalid/private?token=secret",
      ],
    },
    failures: forbidden.map((error, index) => ({
      source: index % 2 === 0 ? "runtime-launch" : "dependency",
      error,
      unknown: "private research contents",
    })).concat([
      {
        source: "__proto__",
        error: "raw provider stderr",
        unknown: "private research contents",
      },
    ]),
    unknown: {
      prompt: "system prompt contents",
      path: "/Users/private/project/secret.txt",
    },
  } as never);

  const statusJson = renderStatusJson(snapshot);
  const doctorJson = renderDoctorJson(snapshot);
  const statusHuman = renderStatusHuman(snapshot);
  const doctorHuman = renderDoctorHuman(snapshot);
  for (const injected of forbidden) {
    assert.equal(statusJson.includes(injected), false, injected);
    assert.equal(doctorJson.includes(injected), false, injected);
    assert.equal(statusHuman.includes(injected), false, injected);
    assert.equal(doctorHuman.includes(injected), false, injected);
  }
  assert.deepEqual(snapshot.web.tools, ["fetch_content"]);
  assert.deepEqual(
    snapshot.diagnostics.map((diagnostic) => diagnostic.code),
    [
      "subagent-runtime-unavailable",
      "web-capability-degraded",
      "dependency-unavailable",
    ],
  );
  assert.equal("unknown" in snapshot, false);
});

test("doctor diagnostics use Matty codes in remediation order", () => {
  const snapshot = createStatusSnapshot({
    packageVersion: "0.1.0",
    piVersion: "future",
    platform: "linux",
    arch: "x64",
    activation: UNCERTIFIED,
    subagentRuntimeAvailable: false,
    web: {
      state: "unavailable",
      registeredTools: [],
    },
    failures: [
      { source: "artifact-integrity", error: "raw artifact error" },
      { source: "dependency", error: "raw dependency error" },
      { source: "web-integration", error: "raw web error" },
      { source: "capability-contract", error: "raw contract error" },
      { source: "rule-injection", error: "raw rules error" },
      { source: "role-data", error: "raw role error" },
      { source: "runtime-launch", error: "raw runtime error" },
    ],
  });

  assert.deepEqual(
    snapshot.diagnostics.map((diagnostic) => diagnostic.code),
    [
      "host-uncertified",
      "reference-model-unverified",
      "subagent-runtime-unavailable",
      "role-data-invalid",
      "matty-rules-unavailable",
      "capability-contract-invalid",
      "web-capability-unavailable",
      "dependency-unavailable",
      "artifact-integrity-failed",
    ],
  );
});
