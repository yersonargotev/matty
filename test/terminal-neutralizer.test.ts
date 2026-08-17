import assert from "node:assert/strict";
import test from "node:test";

import { createTerminalNeutralizer, neutralizeTerminalText } from "../src/domain/terminal-neutralizer.ts";

test("terminal neutralizer removes complete and unterminated terminal controls", () => {
  const unsafe = [
    "safe",
    "\u001b]52;c;clipboard\u0007after-osc",
    "\u001bPprivate\u001b\\after-dcs",
    "\u001bXprivate\u001b\\after-sos",
    "\u001b^private\u001b\\after-pm",
    "\u001b_private\u001b\\after-apc",
    "\u001b[31mred\u001b[0m",
    "\u001b7short",
    "\u0000c0\u0085c1",
    "\u009d52;c;c1-osc\u0007after-c1-osc",
    "\u009b31mc1-red\u009b0m",
    "unterminated\u001b]52;c;secret",
  ].join("|");
  const safe = neutralizeTerminalText(unsafe);
  assert.equal(safe, "safe|after-osc|after-dcs|after-sos|after-pm|after-apc|red|short|c0c1|after-c1-osc|c1-red|unterminated");
  assert.doesNotMatch(safe, /clipboard|private|secret|\u001b|[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/u);
});

test("terminal neutralizer carries bounded parser state across ordered chunks", () => {
  const neutralizer = createTerminalNeutralizer();
  const safe = [
    neutralizer.write("before-osc\u001b]"),
    neutralizer.write("52;c;osc-payload\u0007after-osc|before-dcs\u001bP"),
    neutralizer.write("dcs-payload\u001b\\after-dcs|before-csi\u001b["),
    neutralizer.write("31mafter-csi|before-short\u001b"),
    neutralizer.write("7after-short|before-c1-csi\u009b"),
    neutralizer.write("31mafter-c1-csi|unterminated\u001b_"),
    neutralizer.write("apc-payload"),
  ].join("");
  neutralizer.end();

  assert.equal(safe, "before-oscafter-osc|before-dcsafter-dcs|before-csiafter-csi|before-shortafter-short|before-c1-csiafter-c1-csi|unterminated");
  assert.doesNotMatch(safe, /payload|52|31m|\u001b|[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/u);
});
