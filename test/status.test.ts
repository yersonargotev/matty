import assert from "node:assert/strict";
import test from "node:test";

import {
  createStatusSnapshot,
  renderStatusHuman,
  renderStatusJson,
} from "../src/domain/status.ts";

test("status reports an active certified package, Pi, and target from one snapshot", () => {
  const snapshot = createStatusSnapshot({
    packageVersion: "0.1.0",
    piVersion: "0.83.0",
    platform: "darwin",
    arch: "arm64",
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
    activation: {
      state: "active",
      reason: "compatible",
    },
  });

  assert.equal(
    renderStatusHuman(snapshot),
    [
      "Matty 0.1.0",
      "Pi 0.83.0 · certified",
      "Target darwin/arm64 · certified",
      "Activation active · compatible",
    ].join("\n"),
  );
  assert.deepEqual(JSON.parse(renderStatusJson(snapshot)), snapshot);
});
