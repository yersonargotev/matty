import assert from "node:assert/strict";
import test from "node:test";

import { DelegationControl } from "../src/application/delegation-control.ts";
import type {
  DelegatedTaskPresentation,
  DelegatedTaskRunner,
} from "../src/application/child-pi-runtime.ts";

function runner(presentation: DelegatedTaskPresentation, detached: () => void): DelegatedTaskRunner {
  return {
    async run() {
      return {
        status: "cancelled",
        child: null,
        phase: "before-spawn",
      };
    },
    presentation() {
      return presentation;
    },
    subscribePresentation() {
      return detached;
    },
  };
}

test("control retains active plus newest terminal limit and detaches completion closures", async () => {
  const control = new DelegationControl({ terminalLimit: 2 });
  let detached = 0;
  let activeAborts = 0;
  const presentation = Object.freeze({
    revision: 1,
    assistant: Object.freeze([]),
    tools: Object.freeze([]),
  });

  control.open("active", "required", ["active-task"], () => { activeAborts += 1; });
  for (let index = 0; index < 3; index += 1) {
    const delegationId = `terminal-${index}`;
    const taskId = `task-${index}`;
    control.open(delegationId, "optional", [taskId], () => {});
    control.attachRunner(taskId, runner(presentation, () => { detached += 1; }));
    const source = { index, nested: { mutable: true } };
    const completed = control.complete(delegationId, source) as typeof source;
    assert.equal(Object.isFrozen(completed), true);
    assert.equal(Object.isFrozen(completed.nested), true);
    assert.equal(control.complete(delegationId, { replacement: true }), completed);
  }

  assert.equal(detached, 3);
  assert.equal(control.taskPresentation("task-0"), undefined);
  assert.equal(control.taskPresentation("task-1"), presentation);
  assert.equal(control.taskPresentation("task-2"), presentation);
  assert.deepEqual(control.abortTask("active-task"), { status: "accepted" });
  assert.equal(activeAborts, 1);
  await assert.rejects(control.freeze("terminal-0"), /unavailable/);
  assert.equal((await control.freeze("terminal-2") as { index: number }).index, 2);
});

test("control reset resolves waiters and drops task-scoped presentation subscriptions", async () => {
  const control = new DelegationControl();
  let detached = 0;
  let aborted = 0;
  control.open("delegation", "required", ["task"], () => { aborted += 1; });
  control.attachRunner("task", runner(Object.freeze({
    revision: 1,
    assistant: Object.freeze([]),
    tools: Object.freeze([]),
  }), () => { detached += 1; }));
  const terminal = control.freeze("delegation");

  control.reset();

  assert.deepEqual(await terminal, { status: "cancelled" });
  assert.equal(aborted, 1);
  assert.equal(detached, 1);
  assert.equal(control.taskPresentation("task"), undefined);
  assert.deepEqual(await control.interact("task", { type: "steer", message: "late" }), {
    status: "rejected",
    code: "delegated-task-unavailable",
  });
});
