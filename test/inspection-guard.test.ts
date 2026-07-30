import assert from "node:assert/strict";
import test from "node:test";

import {
  inspectExplorerCommand,
  type InspectionMutationClass,
} from "../src/domain/inspection-guard.ts";

test("allows explorer Git, CodeGraph, shell, and diagnostic inspection", () => {
  for (const command of [
    "git status --short",
    "git log -1 --oneline",
    'codegraph explore "createChildPiRunner"',
    "rg -n TODO src",
    "pwd && node --version",
  ]) {
    assert.deepEqual(inspectExplorerCommand(command), { allowed: true });
  }
});

test("blocks recognized mutation families for an explorer", () => {
  const cases: Array<[string, InspectionMutationClass]> = [
    ["touch changed.txt", "filesystem"],
    ["command touch changed.txt", "filesystem"],
    ["FOO=bar touch changed.txt", "filesystem"],
    ["sudo -u root touch changed.txt", "filesystem"],
    ["env -u FOO touch changed.txt", "filesystem"],
    ["time -o timing.txt touch changed.txt", "filesystem"],
    ["echo changed > changed.txt", "shell"],
    ["git commit -am changed", "git"],
    ["git -C . commit --allow-empty -m changed", "git"],
    ["gh issue view 8", "github"],
    ["command gh issue create --title changed", "github"],
    ["curl https://example.com", "network"],
    ["command curl -X POST https://example.com", "network"],
  ];

  for (const [command, mutationClass] of cases) {
    const result = inspectExplorerCommand(command);
    assert.equal(result.allowed, false, command);
    if (!result.allowed) {
      assert.equal(result.mutationClass, mutationClass, command);
      assert.match(result.reason, /Inspection Guard blocked/);
    }
  }
});

test("recognizes mutation commands after pipes and shell separators", () => {
  for (const command of [
    "pwd; rm -f changed.txt",
    "printf value | tee changed.txt",
    "git status && git checkout main",
    "env FOO=bar sh -c 'mkdir changed'",
  ]) {
    assert.equal(inspectExplorerCommand(command).allowed, false, command);
  }
});
