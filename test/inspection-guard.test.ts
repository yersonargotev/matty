import assert from "node:assert/strict";
import test from "node:test";

import {
  inspectExplorerCommand,
  inspectInspectionCommand,
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
    "pwd\n/usr/bin/touch changed.txt",
    "printf value | tee changed.txt",
    "git status && git checkout main",
    "env FOO=bar sh -c 'mkdir changed'",
    'echo "$(touch changed.txt)"',
    "cat <(touch changed.txt)",
    "cat <(curl https://example.com)",
    "{ touch changed.txt; }",
    "if true; then touch changed.txt; fi",
    "git diff --output=changed.patch",
  ]) {
    assert.equal(inspectExplorerCommand(command).allowed, false, command);
  }
});

test("allows reviewer read-only gh inspection after role preflight", () => {
  for (const command of [
    "gh auth status",
    "gh issue view 9 --comments",
    "gh pr diff 42",
    "gh api repos/yersonargotev/matty/issues/9",
  ]) {
    assert.deepEqual(inspectInspectionCommand("reviewer", command), {
      allowed: true,
    });
  }
});

test("blocks gh for designer and recognized GitHub mutations for reviewer", () => {
  assert.equal(
    inspectInspectionCommand("designer", "gh issue view 9").allowed,
    false,
  );
  for (const command of [
    "gh issue create --title changed",
    "gh issue comment 9 --body changed",
    "gh pr merge 42",
    "gh api --method POST repos/yersonargotev/matty/issues/9/comments",
    "gh api -X PATCH repos/yersonargotev/matty/issues/9",
    "gh api -XPOST repos/yersonargotev/matty/issues",
    "gh api -Ftitle=changed repos/yersonargotev/matty/issues",
    "gh repo delete yersonargotev/matty",
    "gh auth login",
    "gh auth token",
    "gh extension install owner/extension",
    "gh codespace create",
    "gh cache delete 123",
    "gh repo set-default owner/repo",
    "gh ssh-key add key.pub",
    "gh made-up inspect",
  ]) {
    const result = inspectInspectionCommand("reviewer", command);
    assert.equal(result.allowed, false, command);
    if (!result.allowed) {
      assert.equal(result.mutationClass, "github", command);
    }
  }
});
