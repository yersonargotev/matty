import assert from "node:assert/strict";
import test from "node:test";

import {
  createStatusSnapshot,
  renderStatusHuman,
  renderStatusJson,
} from "../src/domain/status.ts";

test("status activates on a certified host", () => {
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
    roles: {
      available: ["explorer", "designer", "reviewer", "worker"],
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
  });

  assert.equal(
    renderStatusHuman(snapshot),
    [
      "Matty 0.1.0",
      "Pi 0.83.0 · certified",
      "Target darwin/arm64 · certified",
      "Activation active · compatible",
      "Roles explorer, designer, reviewer, worker",
      "Inspection Guard best-effort · not a security sandbox",
      "Worker Guard best-effort · Single Writer · not a security sandbox",
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
  });

  assert.deepEqual(snapshot.activation, {
    state: "degraded",
    reason: "unsupported-host",
  });
  assert.equal(snapshot.pi.state, "unsupported");
  assert.equal(snapshot.target.state, "unsupported");
});
