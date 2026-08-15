import assert from "node:assert/strict";
import test from "node:test";

import {
  registerMatty,
  type MattyHost,
  type NotificationLevel,
} from "../src/application/register-matty.ts";

function createPiHarness() {
  const handlers = new Map<
    string,
    (event: { reason: string }) => Promise<void>
  >();
  const commands = new Map<
    string,
    {
      description: string;
      handle(args: string): Promise<void>;
    }
  >();
  const notifications: Array<{
    message: string;
    level: NotificationLevel;
  }> = [];
  const host: MattyHost = {
    onSessionStart(handler) {
      handlers.set(
        "session_start",
        async (event) => handler(event, notify),
      );
    },
    registerCommand(name, command) {
      commands.set(name, {
        description: command.description,
        handle: async (args) => command.handle(args, notify),
      });
    },
  };
  function notify(message: string, level: NotificationLevel) {
    notifications.push({ message, level });
  }

  return {
    host,
    handlers,
    commands,
    notifications,
  };
}

test("a compatible host activates Matty Core", async () => {
  const harness = createPiHarness();

  registerMatty(harness.host, {
    packageVersion: "0.2.0",
    piVersion: "0.83.0",
    platform: "darwin",
    arch: "arm64",
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

  assert.deepEqual([...harness.commands.keys()], ["matty"]);

  await harness.handlers.get("session_start")?.({ reason: "startup" });
  await harness.handlers.get("session_start")?.({ reason: "startup" });
  assert.deepEqual(harness.notifications, [
    {
      message: "Matty active · /matty status",
      level: "info",
    },
  ]);

  await harness.commands.get("matty")?.handle("status");
  assert.equal(
    harness.notifications.at(-1)?.message,
    [
      "Matty 0.2.0",
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

  await harness.commands.get("matty")?.handle("status --json");
  const jsonStatus = JSON.parse(harness.notifications.at(-1)?.message ?? "");
  assert.equal(jsonStatus.schemaVersion, 1);
  assert.equal(jsonStatus.command, "status");
  assert.equal(jsonStatus.activation.state, "active");
  assert.equal("catalog" in jsonStatus, false);
});

test("an unsupported host stays diagnosable and replaces the active hint", async () => {
  const harness = createPiHarness();

  registerMatty(harness.host, {
    packageVersion: "0.2.0",
    piVersion: "0.84.0",
    platform: "linux",
    arch: "x64",
    web: {
      state: "unavailable",
      registeredTools: [],
    },
  });

  await harness.handlers.get("session_start")?.({ reason: "startup" });
  await harness.handlers.get("session_start")?.({ reason: "startup" });
  assert.deepEqual(harness.notifications, [
    {
      message:
        "Matty degraded · unsupported Pi version or target · /matty status",
      level: "warning",
    },
  ]);

  await harness.commands.get("matty")?.handle("status --json");
  const jsonStatus = JSON.parse(harness.notifications.at(-1)?.message ?? "");
  assert.equal(jsonStatus.pi.state, "unsupported");
  assert.equal(jsonStatus.target.state, "unsupported");
  assert.deepEqual(jsonStatus.activation, {
    state: "degraded",
    reason: "unsupported-host",
    codes: ["host-uncertified"],
  });
});

test("status and doctor render the same Redacted Diagnostic snapshot", async () => {
  const harness = createPiHarness();
  registerMatty(harness.host, {
    packageVersion: "0.2.0",
    piVersion: "0.83.0",
    platform: "darwin",
    arch: "arm64",
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

  await harness.commands.get("matty")?.handle("status --json");
  const statusText = harness.notifications.at(-1)?.message ?? "";
  await harness.commands.get("matty")?.handle("doctor --json");
  const doctorText = harness.notifications.at(-1)?.message ?? "";

  assert.equal(/\u001B\[[0-?]*[ -/]*[@-~]/.test(statusText), false);
  assert.equal(/\u001B\[[0-?]*[ -/]*[@-~]/.test(doctorText), false);
  const status = JSON.parse(statusText);
  const doctor = JSON.parse(doctorText);
  assert.equal(status.command, "status");
  assert.equal(doctor.command, "doctor");
  delete status.command;
  delete doctor.command;
  assert.deepEqual(doctor, status);

  await harness.commands.get("matty")?.handle("doctor");
  assert.equal(
    harness.notifications.at(-1)?.message,
    "Matty doctor · active\nNo remediation required.",
  );
});
