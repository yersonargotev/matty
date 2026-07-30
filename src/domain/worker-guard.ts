import { lstat, realpath } from "node:fs/promises";
import { basename, dirname, isAbsolute, relative, resolve } from "node:path";

export type WorkerMutationClass =
  | "github"
  | "git"
  | "global-installation"
  | "external-path"
  | "user-configuration"
  | "shell";

export type WorkerDecision =
  | { allowed: true }
  | {
      allowed: false;
      mutationClass: WorkerMutationClass;
      reason: string;
    };

export interface WorkerGuardScope {
  workingTree: string;
  temporaryPaths: readonly string[];
  userConfigurationPaths: readonly string[];
}

const GIT_MUTATIONS = new Set([
  "add",
  "am",
  "apply",
  "bisect",
  "branch",
  "checkout",
  "cherry-pick",
  "clean",
  "commit",
  "config",
  "fetch",
  "gc",
  "init",
  "merge",
  "mv",
  "notes",
  "pull",
  "push",
  "rebase",
  "remote",
  "reset",
  "restore",
  "revert",
  "rm",
  "stash",
  "submodule",
  "switch",
  "tag",
  "worktree",
]);

const SIMPLE_WRITE_COMMANDS = new Set([
  "touch",
  "mkdir",
  "rmdir",
  "rm",
  "truncate",
  "tee",
  "cp",
  "mv",
  "ln",
  "install",
]);

function blocked(
  mutationClass: WorkerMutationClass,
  detail: string,
): WorkerDecision {
  return {
    allowed: false,
    mutationClass,
    reason: `Worker Guard blocked recognized ${detail}`,
  };
}

function containsPath(root: string, candidate: string): boolean {
  const remainder = relative(root, candidate);
  return remainder === "" ||
    (!remainder.startsWith("..") && !isAbsolute(remainder));
}

function containsGitInternals(workingTree: string, candidate: string): boolean {
  const remainder = relative(workingTree, candidate);
  return remainder === ".git" ||
    remainder.startsWith(`.git/`) ||
    remainder.includes("/.git/");
}

async function canonicalCandidate(path: string): Promise<string> {
  let existing = path;
  const missing: string[] = [];
  while (true) {
    try {
      await lstat(existing);
      break;
    } catch {
      const parent = dirname(existing);
      if (parent === existing) {
        return path;
      }
      missing.unshift(basename(existing));
      existing = parent;
    }
  }
  return resolve(await realpath(existing), ...missing);
}

export async function inspectWorkerPath(
  scope: WorkerGuardScope,
  requestedPath: string,
  cwd: string = scope.workingTree,
): Promise<WorkerDecision> {
  const absolute = resolve(cwd, requestedPath);
  const candidate = await canonicalCandidate(absolute);

  if (
    scope.userConfigurationPaths.some((path) =>
      containsPath(path, absolute) || containsPath(path, candidate)
    )
  ) {
    return blocked(
      "user-configuration",
      "real user-configuration write",
    );
  }
  if (
    containsGitInternals(scope.workingTree, absolute) ||
    containsGitInternals(scope.workingTree, candidate)
  ) {
    return blocked("git", "Git index or reference mutation");
  }
  const allowedRoots = [scope.workingTree, ...scope.temporaryPaths];
  if (
    !allowedRoots.some((path) => containsPath(path, candidate))
  ) {
    return blocked("external-path", "path outside the validated write roots");
  }
  return { allowed: true };
}

function shellTokens(segment: string): string[] {
  return (
    segment.match(/(?:[^\s"'\\]+|"(?:\\.|[^"])*"|'[^']*')+/g) ?? []
  ).map((token) => token.replace(/^["']|["']$/g, ""));
}

function commandTokens(segment: string): string[] {
  const tokens = shellTokens(segment.trim());
  while (/^[A-Za-z_][A-Za-z0-9_]*=/.test(tokens[0] ?? "")) {
    tokens.shift();
  }
  while (["command", "env", "sudo", "time", "nohup"].includes(
    basename(tokens[0] ?? "").toLowerCase(),
  )) {
    const wrapper = basename(tokens.shift() ?? "").toLowerCase();
    const optionsWithValues = wrapper === "env"
      ? new Set(["-u", "--unset"])
      : wrapper === "sudo"
      ? new Set([
        "-u",
        "-g",
        "-h",
        "-p",
        "-C",
        "-R",
        "-D",
        "--user",
        "--group",
        "--host",
        "--prompt",
        "--close-from",
        "--chroot",
        "--chdir",
      ])
      : wrapper === "time"
      ? new Set(["-f", "-o", "--format", "--output"])
      : new Set<string>();
    while (tokens[0]?.startsWith("-")) {
      const option = tokens.shift();
      const name = option?.split("=", 1)[0] ?? "";
      if (optionsWithValues.has(name) && !option?.includes("=")) {
        tokens.shift();
      }
    }
    while (/^[A-Za-z_][A-Za-z0-9_]*=/.test(tokens[0] ?? "")) {
      tokens.shift();
    }
  }
  return tokens;
}

function gitSubcommand(tokens: string[]): string | undefined {
  let index = 1;
  while (index < tokens.length) {
    const token = tokens[index];
    if (!token?.startsWith("-")) {
      return token?.toLowerCase();
    }
    index += [
        "-c",
        "-C",
        "--git-dir",
        "--work-tree",
        "--namespace",
        "--config-env",
      ].includes(token)
      ? 2
      : 1;
  }
  return undefined;
}

function isGlobalInstall(tokens: string[]): boolean {
  const head = basename(tokens[0] ?? "").toLowerCase();
  const globalFlag = tokens.some((token) =>
    token === "-g" ||
    token === "--global" ||
    token === "--location=global"
  );
  if (["npm", "pnpm", "bun"].includes(head)) {
    return globalFlag;
  }
  if (head === "yarn") {
    return tokens[1]?.toLowerCase() === "global" || globalFlag;
  }
  return (
    ["cargo", "gem", "pipx"].includes(head) &&
    tokens[1]?.toLowerCase() === "install"
  ) || (
    ["pip", "pip3"].includes(head) &&
    tokens[1]?.toLowerCase() === "install" &&
    !tokens.some((token) =>
      token === "--target" ||
      token.startsWith("--target=") ||
      token === "--prefix" ||
      token.startsWith("--prefix=")
    )
  ) || (
    head === "go" &&
    tokens[1]?.toLowerCase() === "install"
  );
}

function packageManagerWriteTargets(tokens: string[]): string[] {
  const head = basename(tokens[0] ?? "").toLowerCase();
  if (!["npm", "pnpm", "yarn", "bun", "pip", "pip3"].includes(head)) {
    return [];
  }
  const pathOptions = new Set([
    "--prefix",
    "--dir",
    "--cwd",
    "--global-dir",
    "--store-dir",
    "--cache",
    "--target",
  ]);
  const targets: string[] = [];
  for (let index = 1; index < tokens.length; index += 1) {
    const token = tokens[index] ?? "";
    const [name, inlineValue] = token.split("=", 2);
    if (!pathOptions.has(name ?? "")) {
      continue;
    }
    const value = inlineValue ?? tokens[index + 1];
    if (value) {
      targets.push(value);
    }
    if (!inlineValue) {
      index += 1;
    }
  }
  return targets;
}

function writeTargets(tokens: string[]): string[] {
  const head = basename(tokens[0] ?? "").toLowerCase();
  if (SIMPLE_WRITE_COMMANDS.has(head)) {
    return tokens.slice(1).filter((token) => !token.startsWith("-"));
  }
  if (
    head === "sed" &&
    tokens.slice(1).some((token) => token === "-i" || token.startsWith("-i"))
  ) {
    return tokens.slice(1).filter((token) => !token.startsWith("-")).slice(-1);
  }
  return [];
}

function redirectionTargets(segment: string): string[] {
  return Array.from(
    segment.matchAll(/(?:^|\s)(?:[0-9]*>>?|&>)\s*([^\s;&|]+)/g),
    (match) => match[1]?.replace(/^["']|["']$/g, "") ?? "",
  ).filter(Boolean);
}

export async function inspectWorkerCommand(
  scope: WorkerGuardScope,
  command: string,
): Promise<WorkerDecision> {
  if (
    command.includes("$(") ||
    command.includes("`") ||
    command.includes("<(") ||
    command.includes(">(")
  ) {
    return blocked("shell", "indirect shell execution");
  }
  if (/\bGIT_(?:DIR|INDEX_FILE|WORK_TREE|OBJECT_DIRECTORY)=/.test(command)) {
    return blocked("git", "Git index or reference mutation");
  }

  let cwd = scope.workingTree;
  for (const segment of command.split(/[;&|\r\n]+/)) {
    const tokens = commandTokens(segment);
    const head = basename(tokens[0] ?? "").toLowerCase();
    if (!head) {
      continue;
    }
    if (head === "gh") {
      return blocked("github", "GitHub access");
    }
    if (
      ["sh", "bash", "zsh", "fish", "dash"].includes(head) &&
      tokens.slice(1).includes("-c")
    ) {
      return blocked("shell", "indirect shell execution");
    }
    if (head === "git" && GIT_MUTATIONS.has(gitSubcommand(tokens) ?? "")) {
      return blocked("git", "Git index or reference mutation");
    }
    if (
      ["npm", "pnpm", "yarn", "bun", "pip", "pip3"].includes(head) &&
      tokens[1]?.toLowerCase() === "config"
    ) {
      return blocked(
        "user-configuration",
        "real user-configuration write",
      );
    }
    if (isGlobalInstall(tokens)) {
      return blocked("global-installation", "global installation");
    }
    if (head === "cd") {
      const target = tokens[1] ?? scope.workingTree;
      const decision = await inspectWorkerPath(scope, target, cwd);
      if (!decision.allowed) {
        return decision;
      }
      cwd = resolve(cwd, target);
      continue;
    }
    for (const target of [
      ...writeTargets(tokens),
      ...packageManagerWriteTargets(tokens),
      ...redirectionTargets(segment),
    ]) {
      const decision = await inspectWorkerPath(scope, target, cwd);
      if (!decision.allowed) {
        return decision;
      }
    }
  }
  return { allowed: true };
}
