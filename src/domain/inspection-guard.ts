export type InspectionMutationClass =
  | "filesystem"
  | "shell"
  | "git"
  | "github"
  | "network";

export type InspectionDecision =
  | { allowed: true }
  | {
      allowed: false;
      mutationClass: InspectionMutationClass;
      reason: string;
    };

const FILESYSTEM_MUTATIONS = new Set([
  "rm",
  "mv",
  "cp",
  "mkdir",
  "rmdir",
  "touch",
  "chmod",
  "chown",
  "truncate",
  "dd",
  "install",
  "ln",
  "tee",
]);
const GIT_MUTATIONS = new Set([
  "add",
  "am",
  "apply",
  "bisect",
  "branch",
  "checkout",
  "cherry-pick",
  "clean",
  "clone",
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
const NETWORK_COMMANDS = new Set([
  "curl",
  "wget",
  "http",
  "https",
  "ssh",
  "scp",
  "sftp",
  "nc",
  "ncat",
  "telnet",
  "ftp",
  "rsync",
  "npm",
  "pnpm",
  "yarn",
  "bun",
  "pip",
  "pipx",
]);
const SHELL_MUTATIONS = new Set([
  "sh",
  "bash",
  "zsh",
  "fish",
  "dash",
  "eval",
  "exec",
  "xargs",
  "kill",
  "killall",
  "pkill",
  "launchctl",
]);
const WRAPPERS = new Set(["command", "builtin", "nohup", "time"]);
const ENV_OPTIONS_WITH_VALUES = new Set(["-u", "--unset"]);
const SUDO_OPTIONS_WITH_VALUES = new Set([
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
  "--role",
  "--type",
  "--other-user",
]);
const TIME_OPTIONS_WITH_VALUES = new Set(["-f", "-o", "--format", "--output"]);

function shellTokens(segment: string): string[] {
  return (
    segment.match(/(?:[^\s"'\\]+|"(?:\\.|[^"])*"|'[^']*')+/g) ?? []
  ).map((token) => token.replace(/^["']|["']$/g, ""));
}

function consumeOptions(
  tokens: string[],
  optionsWithValues: ReadonlySet<string>,
): void {
  while (tokens[0]?.startsWith("-")) {
    const option = tokens.shift();
    if (option === "--") {
      return;
    }
    const name = option?.split("=", 1)[0];
    if (
      name &&
      optionsWithValues.has(name) &&
      !option?.includes("=")
    ) {
      tokens.shift();
    }
  }
}

function commandTokens(segment: string): string[] {
  const tokens = shellTokens(segment.trim().replace(/^\(+/, ""));
  while (/^[A-Za-z_][A-Za-z0-9_]*=/.test(tokens[0] ?? "")) {
    tokens.shift();
  }
  while (tokens.length > 0) {
    const head = tokens[0]?.toLowerCase();
    if (/^[A-Za-z_][A-Za-z0-9_]*=/.test(tokens[0] ?? "")) {
      tokens.shift();
      continue;
    }
    if (head === "env") {
      tokens.shift();
      consumeOptions(tokens, ENV_OPTIONS_WITH_VALUES);
      continue;
    }
    if (head === "sudo") {
      tokens.shift();
      consumeOptions(tokens, SUDO_OPTIONS_WITH_VALUES);
      continue;
    }
    if (head === "time") {
      tokens.shift();
      consumeOptions(tokens, TIME_OPTIONS_WITH_VALUES);
      continue;
    }
    if (head && WRAPPERS.has(head)) {
      tokens.shift();
      consumeOptions(tokens, new Set());
      continue;
    }
    break;
  }
  return tokens;
}

function gitSubcommand(tokens: string[]): string | undefined {
  let index = 1;
  while (index < tokens.length) {
    const token = tokens[index]?.toLowerCase();
    if (!token?.startsWith("-")) {
      return token;
    }
    if (
      token === "-c" ||
      token === "-C" ||
      token === "--git-dir" ||
      token === "--work-tree" ||
      token === "--namespace" ||
      token === "--config-env"
    ) {
      index += 2;
    } else {
      index += 1;
    }
  }
  return undefined;
}

function blocked(
  mutationClass: InspectionMutationClass,
): InspectionDecision {
  return {
    allowed: false,
    mutationClass,
    reason:
      `Inspection Guard blocked recognized ${mutationClass} ` +
      "mutation; explorer shell access is inspection-only",
  };
}

export function inspectExplorerCommand(
  command: string,
): InspectionDecision {
  if (/(?:^|[^<])(?:[0-9]*>{1,2})/.test(command)) {
    return blocked("shell");
  }
  for (const segment of command.split(/[;&|]+/)) {
    const tokens = commandTokens(segment);
    const head = tokens[0]?.toLowerCase();
    if (!head) {
      continue;
    }
    if (head === "gh") {
      return blocked("github");
    }
    if (NETWORK_COMMANDS.has(head)) {
      return blocked("network");
    }
    if (
      head === "git" &&
      GIT_MUTATIONS.has(gitSubcommand(tokens) ?? "")
    ) {
      return blocked("git");
    }
    if (FILESYSTEM_MUTATIONS.has(head)) {
      return blocked("filesystem");
    }
    if (SHELL_MUTATIONS.has(head)) {
      return blocked("shell");
    }
    if (
      ["node", "python", "python3", "ruby", "perl"].includes(head) &&
      tokens.slice(1).some((token) => token === "-e" || token === "-c")
    ) {
      return blocked("shell");
    }
    if (
      head === "find" &&
      tokens.some((token) => token === "-delete" || token === "-exec")
    ) {
      return blocked("filesystem");
    }
  }
  return { allowed: true };
}
