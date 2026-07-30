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
    packageVersion: "0.1.0",
    piVersion: "0.83.0",
    platform: "darwin",
    arch: "arm64",
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
      "Matty 0.1.0",
      "Pi 0.83.0 · certified",
      "Target darwin/arm64 · certified",
      "Activation active · compatible",
      "Roles explorer, designer, reviewer",
      "Inspection Guard best-effort · not a security sandbox",
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
    packageVersion: "0.1.0",
    piVersion: "0.84.0",
    platform: "linux",
    arch: "x64",
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
  });
});
