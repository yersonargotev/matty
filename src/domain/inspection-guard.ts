import type { InspectionRole } from "./capability-contract.ts";

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
const SHELL_CONTROL_FLOW = new Set([
  "!",
  "{",
  "case",
  "for",
  "function",
  "if",
  "select",
  "until",
  "while",
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
const GH_GLOBAL_OPTIONS_WITH_VALUES = new Set([
  "-R",
  "--repo",
  "--hostname",
]);
const GH_READ_ONLY_COMMANDS = new Map<string, ReadonlySet<string>>([
  ["auth", new Set(["status"])],
  ["issue", new Set(["list", "status", "view"])],
  ["pr", new Set(["checks", "diff", "list", "status", "view"])],
  ["repo", new Set(["list", "view"])],
  ["release", new Set(["list", "view"])],
  ["run", new Set(["list", "view"])],
  ["workflow", new Set(["list", "view"])],
  ["gist", new Set(["list", "view"])],
  ["label", new Set(["list"])],
  ["project", new Set(["field-list", "item-list", "list", "view"])],
  ["ruleset", new Set(["check", "list", "view"])],
]);

function shellTokens(segment: string): string[] {
  return (
    segment.match(/(?:[^\s"'\\]+|"(?:\\.|[^"])*"|'[^']*')+/g) ?? []
  ).map((token) => token.replace(/^["']|["']$/g, ""));
}

function commandName(token: string | undefined): string | undefined {
  return token?.split(/[\\/]/).at(-1)?.toLowerCase();
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
    const head = commandName(tokens[0]);
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

function githubInspectionAllowed(tokens: string[]): boolean {
  const arguments_ = tokens.slice(1);
  if (arguments_.length === 1 && arguments_[0] === "--version") {
    return true;
  }
  consumeOptions(arguments_, GH_GLOBAL_OPTIONS_WITH_VALUES);
  const command = arguments_.shift()?.toLowerCase();
  if (!command) {
    return false;
  }
  if (command === "status") {
    return arguments_.length === 0;
  }
  if (command === "search") {
    return true;
  }
  if (command === "api") {
    const methodIndex = arguments_.findIndex((token) =>
      token === "-X" || token === "--method"
    );
    const methodFromPair =
      methodIndex >= 0 ? arguments_[methodIndex + 1]?.toUpperCase() : undefined;
    const methodFromEquals = arguments_
      .find((token) => token.startsWith("--method="))
      ?.slice("--method=".length)
      .toUpperCase();
    const methodFromCompact = arguments_
      .find((token) => token.startsWith("-X") && token.length > 2)
      ?.slice(2)
      .toUpperCase();
    return !(
      (methodFromPair !== undefined && methodFromPair !== "GET") ||
      (methodFromEquals !== undefined && methodFromEquals !== "GET") ||
      (methodFromCompact !== undefined && methodFromCompact !== "GET") ||
      arguments_.some((token) =>
        token === "-f" ||
        (token.startsWith("-f") && token.length > 2) ||
        token === "-F" ||
        (token.startsWith("-F") && token.length > 2) ||
        token === "--field" ||
        token.startsWith("--field=") ||
        token === "--raw-field" ||
        token.startsWith("--raw-field=") ||
        token === "--input" ||
        token.startsWith("--input=")
      )
    );
  }
  return GH_READ_ONLY_COMMANDS.get(command)?.has(
    arguments_[0]?.toLowerCase() ?? "",
  ) ??
    false;
}

function blocked(
  mutationClass: InspectionMutationClass,
  role: InspectionRole,
): InspectionDecision {
  return {
    allowed: false,
    mutationClass,
    reason:
      `Inspection Guard blocked recognized ${mutationClass} ` +
      `mutation; ${role} shell access is inspection-only`,
  };
}

export function inspectInspectionCommand(
  role: InspectionRole,
  command: string,
): InspectionDecision {
  if (/(?:^|[^<])(?:[0-9]*>{1,2})/.test(command)) {
    return blocked("shell", role);
  }
  if (
    command.includes("$(") ||
    command.includes("`") ||
    command.includes("<(") ||
    command.includes(">(")
  ) {
    return blocked("shell", role);
  }
  for (const segment of command.split(/[;&|\r\n]+/)) {
    const tokens = commandTokens(segment);
    const head = commandName(tokens[0]);
    if (!head) {
      continue;
    }
    if (SHELL_CONTROL_FLOW.has(head)) {
      return blocked("shell", role);
    }
    if (head === "gh") {
      if (role !== "reviewer" || !githubInspectionAllowed(tokens)) {
        return blocked("github", role);
      }
      continue;
    }
    if (NETWORK_COMMANDS.has(head)) {
      return blocked("network", role);
    }
    if (
      head === "git" &&
      (
        GIT_MUTATIONS.has(gitSubcommand(tokens) ?? "") ||
        tokens.some((token) =>
          token === "--output" || token.startsWith("--output=")
        )
      )
    ) {
      return blocked("git", role);
    }
    if (FILESYSTEM_MUTATIONS.has(head)) {
      return blocked("filesystem", role);
    }
    if (SHELL_MUTATIONS.has(head)) {
      return blocked("shell", role);
    }
    if (
      ["node", "python", "python3", "ruby", "perl"].includes(head) &&
      tokens.slice(1).some((token) => token === "-e" || token === "-c")
    ) {
      return blocked("shell", role);
    }
    if (
      head === "find" &&
      tokens.some((token) => token === "-delete" || token === "-exec")
    ) {
      return blocked("filesystem", role);
    }
  }
  return { allowed: true };
}

export function inspectExplorerCommand(
  command: string,
): InspectionDecision {
  return inspectInspectionCommand("explorer", command);
}
