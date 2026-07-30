import {
  VERSION as PI_VERSION,
  type ExtensionAPI,
  type ExtensionContext,
  type ToolCallEvent,
} from "@earendil-works/pi-coding-agent";
import { execFile } from "node:child_process";
import { realpath } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";

import {
  createChildPiRunner,
  type PiInvocation,
  type PiThinkingLevel,
} from "../application/child-pi-runtime.ts";
import {
  blockedInspectionDelegation,
  runInspectionDelegation,
} from "../application/inspection-role-delegation.ts";
import {
  blockedWorkerDelegation,
  runWorkerDelegation,
} from "../application/worker-delegation.ts";
import {
  acquireRepositoryWriter,
  singleWriterStatePath,
} from "../application/single-writer.ts";
import {
  registerMatty,
  type MattyHost,
} from "../application/register-matty.ts";
import {
  INSPECTION_TOOLS,
  DELEGATION_INPUT_GUIDANCE,
  INSPECTION_ROLES,
  MATTY_ROLES,
  WORKER_TOOLS,
  createWorkerCapabilityContract,
  inspectionCapabilityContract,
  isInspectionRole,
  isMattyRole,
  validateCapabilityContract,
  type InspectionRole,
  type MattyRole,
  type WorkerCapabilityContract,
} from "../domain/capability-contract.ts";
import { inspectInspectionCommand } from "../domain/inspection-guard.ts";
import {
  detectMattyRulesConflict,
  injectMattyRules,
} from "../domain/matty-rules.ts";
import {
  inspectWorkerCommand,
  inspectWorkerPath,
  type WorkerGuardScope,
} from "../domain/worker-guard.ts";
import {
  MATTY_PACKAGE_VERSION,
} from "../domain/package-contract.ts";

export interface PiMattyRegistrationOptions {
  invocation?: PiInvocation;
  childEnvironment?: NodeJS.ProcessEnv;
  independentRuntimeAvailable?: boolean;
  reviewerGithubPreflight?: () => Promise<{
    available: boolean;
    authenticated: boolean;
  }>;
}

const execFileAsync = promisify(execFile);

function createPiHost(pi: ExtensionAPI): MattyHost {
  return {
    registerCommand(name, command) {
      pi.registerCommand(name, {
        description: command.description,
        handler: async (args, context) => {
          await command.handle(args, (message, level) => {
            context.ui.notify(message, level);
          });
        },
      });
    },
    onSessionStart(handler) {
      pi.on("session_start", async (event, context) => {
        await handler(event, (message, level) => {
          context.ui.notify(message, level);
        });
      });
    },
  };
}

function currentPiInvocation(): PiInvocation | undefined {
  const entrypoint = process.argv[1];
  if (!entrypoint) {
    return undefined;
  }
  return {
    command: process.execPath,
    arguments: [
      entrypoint,
      "--no-extensions",
      "-e",
      fileURLToPath(import.meta.url),
    ],
  };
}

function invocationWithTools(
  invocation: PiInvocation,
  tools: readonly string[],
): PiInvocation {
  const arguments_: string[] = [];
  for (let index = 0; index < (invocation.arguments ?? []).length; index += 1) {
    const argument = invocation.arguments?.[index];
    if (argument === "--tools") {
      index += 1;
      continue;
    }
    if (argument?.startsWith("--tools=")) {
      continue;
    }
    if (argument !== undefined) {
      arguments_.push(argument);
    }
  }
  return {
    ...invocation,
    arguments: [...arguments_, "--tools", tools.join(",")],
  };
}

function invocationTools(invocation: PiInvocation | undefined): string[] {
  if (!invocation) {
    return [];
  }
  const arguments_ = invocation.arguments ?? [];
  const inline = arguments_.find((argument) =>
    argument.startsWith("--tools=")
  );
  if (inline) {
    return inline.slice("--tools=".length).split(",").filter(Boolean);
  }
  const index = arguments_.lastIndexOf("--tools");
  return index >= 0
    ? (arguments_[index + 1] ?? "").split(",").filter(Boolean)
    : [];
}

function declaresInvocationTools(invocation: PiInvocation): boolean {
  return (invocation.arguments ?? []).some((argument) =>
    argument === "--tools" || argument.startsWith("--tools=")
  );
}

function childEnvironment(
  environment: NodeJS.ProcessEnv,
  authenticationEnvironment: Readonly<Record<string, string>> | undefined,
  additions: NodeJS.ProcessEnv | undefined,
  role: MattyRole,
  worker?: {
    contract: WorkerCapabilityContract;
    protectedPaths: readonly string[];
    userHome?: string;
    userConfigurationPaths: readonly string[];
  },
): NodeJS.ProcessEnv {
  const inheritedNames = [
    "PATH",
    "HOME",
    "XDG_CONFIG_HOME",
    "PI_CODING_AGENT_DIR",
    "PI_OFFLINE",
    "NO_UPDATE_NOTIFIER",
    "NODE_OPTIONS",
    "TMPDIR",
  ] as const;
  const inherited = Object.fromEntries(
    inheritedNames.flatMap((name) => {
      const value = environment[name];
      return value === undefined ? [] : [[name, value]];
    }),
  );
  const child = {
    ...inherited,
    ...authenticationEnvironment,
    ...additions,
    MATTY_CHILD_ROLE: role,
  };
  if (!worker) {
    return child;
  }
  return {
    ...child,
    MATTY_WORKER_WORKING_TREE: worker.contract.workingTree,
    MATTY_WORKER_TEMPORARY_PATHS: JSON.stringify(
      worker.contract.temporaryPaths,
    ),
    MATTY_WORKER_PROTECTED_PATHS: JSON.stringify(worker.protectedPaths),
    ...(worker.userHome
      ? { MATTY_WORKER_USER_HOME: worker.userHome }
      : {}),
    MATTY_WORKER_USER_CONFIGURATION_PATHS: JSON.stringify(
      worker.userConfigurationPaths,
    ),
  };
}

async function reviewerGithubPreflight(
  environment: NodeJS.ProcessEnv,
): Promise<{ available: boolean; authenticated: boolean }> {
  try {
    await execFileAsync("gh", ["--version"], {
      env: environment,
      timeout: 5_000,
    });
  } catch {
    return { available: false, authenticated: false };
  }
  try {
    await execFileAsync("gh", ["auth", "status"], {
      env: environment,
      timeout: 5_000,
    });
    return { available: true, authenticated: true };
  } catch {
    return { available: true, authenticated: false };
  }
}

function thinkingLevel(value: string | undefined): PiThinkingLevel {
  switch (value) {
    case "minimal":
    case "low":
    case "medium":
    case "high":
    case "xhigh":
      return value;
    default:
      return "off";
  }
}

function workerGuardScope(
  environment: NodeJS.ProcessEnv,
): WorkerGuardScope | undefined {
  try {
    const workingTree = environment.MATTY_WORKER_WORKING_TREE;
    const temporaryPaths = JSON.parse(
      environment.MATTY_WORKER_TEMPORARY_PATHS ?? "[]",
    ) as unknown;
    const userConfigurationPaths = JSON.parse(
      environment.MATTY_WORKER_USER_CONFIGURATION_PATHS ?? "[]",
    ) as unknown;
    const protectedPaths = JSON.parse(
      environment.MATTY_WORKER_PROTECTED_PATHS ?? "[]",
    ) as unknown;
    if (
      !workingTree ||
      !Array.isArray(temporaryPaths) ||
      temporaryPaths.some((path) => typeof path !== "string") ||
      !Array.isArray(protectedPaths) ||
      protectedPaths.some((path) => typeof path !== "string") ||
      !Array.isArray(userConfigurationPaths) ||
      userConfigurationPaths.some((path) => typeof path !== "string")
    ) {
      return undefined;
    }
    const contract = createWorkerCapabilityContract({
      workingTree,
      temporaryPaths: temporaryPaths as string[],
    });
    if (!validateCapabilityContract(contract).ok) {
      return undefined;
    }
    return {
      workingTree: contract.workingTree,
      temporaryPaths: contract.temporaryPaths,
      protectedPaths: protectedPaths as string[],
      ...(environment.MATTY_WORKER_USER_HOME
        ? { userHome: environment.MATTY_WORKER_USER_HOME }
        : {}),
      userConfigurationPaths: userConfigurationPaths as string[],
    };
  } catch {
    return undefined;
  }
}

function userConfigurationPaths(
  environment: NodeJS.ProcessEnv,
): string[] {
  const home = environment.HOME;
  return Array.from(
    new Set(
      [
        environment.XDG_CONFIG_HOME ?? (home ? resolve(home, ".config") : undefined),
        environment.PI_CODING_AGENT_DIR ??
          (home ? resolve(home, ".pi", "agent") : undefined),
        environment.npm_config_userconfig ??
          (home ? resolve(home, ".npmrc") : undefined),
        home ? resolve(home, ".gitconfig") : undefined,
        home ? resolve(home, ".ssh") : undefined,
      ].flatMap((path) => path ? [resolve(path)] : []),
    ),
  );
}

async function workerContract(
  cwd: string,
  environment: NodeJS.ProcessEnv,
): Promise<WorkerCapabilityContract> {
  const temporaryPaths = await Promise.all(
    [environment.TMPDIR ?? tmpdir(), tmpdir()].map(async (path) =>
      await realpath(path)
    ),
  );
  return createWorkerCapabilityContract({
    workingTree: await realpath(cwd),
    temporaryPaths: [...new Set(temporaryPaths)],
  });
}

function delegationResult<T extends { outcome: { status: string } }>(
  terminal: T,
): {
  content: Array<{ type: "text"; text: string }>;
  details: T;
  isError: boolean;
} {
  return {
    content: [{ type: "text", text: JSON.stringify(terminal) }],
    details: terminal,
    isError: terminal.outcome.status !== "succeeded",
  };
}

async function blockedToolCall(
  role: MattyRole,
  event: ToolCallEvent,
  workerScope?: WorkerGuardScope,
): Promise<{ block: true; reason: string } | undefined> {
  if (role === "worker") {
    if (!workerScope) {
      return {
        block: true,
        reason: "Worker Guard blocked malformed validated path scope",
      };
    }
    if (event.toolName === "edit" || event.toolName === "write") {
      const path = (event.input as { path?: unknown }).path;
      if (typeof path !== "string") {
        return {
          block: true,
          reason: "Worker Guard blocked malformed write path",
        };
      }
      const decision = await inspectWorkerPath(workerScope, path);
      return decision.allowed
        ? undefined
        : { block: true, reason: decision.reason };
    }
    if (event.toolName !== "bash") {
      return undefined;
    }
    const command = (event.input as { command?: unknown }).command;
    if (typeof command !== "string") {
      return {
        block: true,
        reason: "Worker Guard blocked malformed bash input",
      };
    }
    const decision = await inspectWorkerCommand(workerScope, command);
    return decision.allowed
      ? undefined
      : { block: true, reason: decision.reason };
  }
  if (event.toolName === "edit" || event.toolName === "write") {
    return {
      block: true,
      reason:
        `Inspection Guard blocked recognized filesystem mutation; ${role} tools are inspection-only`,
    };
  }
  if (event.toolName !== "bash") {
    return undefined;
  }
  const command = (event.input as { command?: unknown }).command;
  if (typeof command !== "string") {
    return {
      block: true,
      reason:
        `Inspection Guard blocked malformed ${role} bash input`,
    };
  }
  const decision = inspectInspectionCommand(role, command);
  return decision.allowed
    ? undefined
    : { block: true, reason: decision.reason };
}

export function registerPiMatty(
  pi: ExtensionAPI,
  environment: NodeJS.ProcessEnv = process.env,
  options: PiMattyRegistrationOptions = {},
): void {
  const childRole = isMattyRole(environment.MATTY_CHILD_ROLE)
    ? environment.MATTY_CHILD_ROLE
    : undefined;
  let rulesConflict: string | undefined;
  pi.on("before_agent_start", (event) => {
    rulesConflict = detectMattyRulesConflict(event.systemPrompt);
    return {
      systemPrompt: injectMattyRules(
        event.systemPrompt,
        childRole ?? "parent",
      ),
    };
  });

  if (childRole) {
    const scope = childRole === "worker"
      ? workerGuardScope(environment)
      : undefined;
    pi.on("tool_call", (event) => blockedToolCall(childRole, event, scope));
  } else {
    const activeInvocations = Object.fromEntries(
      INSPECTION_ROLES.map((role) => [role, 0]),
    ) as Record<InspectionRole, number>;
    const invocation = options.invocation ?? currentPiInvocation();
    const independentRuntimeAvailable =
      options.independentRuntimeAvailable ?? invocation !== undefined;
    const parameters = {
      type: "object",
      properties: {
        role: {
          type: "string",
          enum: [...MATTY_ROLES],
          description: "The least-privilege Matty Role.",
        },
        task: {
          type: "string",
          minLength: 1,
          description: "One bounded delegated assignment.",
        },
      },
      required: ["role", "task"],
      additionalProperties: false,
    };

    pi.registerTool({
      name: "subagent",
      label: "Matty Role",
      description:
        "Run one independent explorer, designer, reviewer, or worker through the Matty Subagent Runtime.",
      promptSnippet:
        "Delegate one bounded inspection task to a named Matty Role.",
      promptGuidelines: [
        `Call subagent with exactly ${DELEGATION_INPUT_GUIDANCE}.`,
        "Inspection roles receive read, grep, find, ls, and guarded bash; worker also receives edit and write.",
        "Only reviewer may perform read-only gh inspection after availability and authentication preflight.",
        "Single Writer permits at most one active worker per repository.",
        "Progress and the terminal success, failure, cancellation, or preflight diagnostic are structured.",
        "Inspection Guard and Worker Guard are best-effort policies, not security sandboxes.",
      ],
      parameters: parameters as never,
      executionMode: "parallel",
      async execute(
        _toolCallId: string,
        params: { role: MattyRole; task: string },
        signal: AbortSignal | undefined,
        onUpdate:
          | ((update: {
              content: Array<{ type: "text"; text: string }>;
              details: unknown;
            }) => void)
          | undefined,
        ctx: ExtensionContext,
      ) {
        if (!isMattyRole(params.role)) {
          const terminal = {
            contract: null,
            outcome: {
              status: "blocked" as const,
              diagnostic: {
                kind: "capability-preflight" as const,
                contractId: "delegate-invalid",
                unmet: ["unsupported inspection role"],
              },
            },
          };
          return {
            content: [{ type: "text" as const, text: JSON.stringify(terminal) }],
            details: terminal,
            isError: true,
          };
        }
        const role = params.role;
        if (role !== "worker" && rulesConflict) {
          const terminal = blockedInspectionDelegation(role, [
            `Matty Rules conflict: ${rulesConflict}`,
          ]);
          return {
            content: [
              { type: "text" as const, text: JSON.stringify(terminal) },
            ],
            details: terminal,
            isError: true,
          };
        }
        const unmet: string[] = [];
        if (!ctx.model) {
          unmet.push("parent model is unavailable");
        }
        if (!invocation || !independentRuntimeAvailable) {
          unmet.push("independent Subagent Runtime is unavailable");
        }
        const authentication = ctx.model
          ? await ctx.modelRegistry.getApiKeyAndHeaders(ctx.model)
          : undefined;
        if (authentication && !authentication.ok) {
          unmet.push(
            `parent authentication is unavailable: ${authentication.error}`,
          );
        }
        const progressOptions = {
          ...(signal ? { signal } : {}),
          onProgress(progress: Parameters<
            NonNullable<typeof onUpdate>
          >[0]["details"] & { type: string }) {
            onUpdate?.({
              content: [
                {
                  type: "text",
                  text: JSON.stringify({ type: "progress", progress }),
                },
              ],
              details: progress,
            });
          },
        };

        if (role === "worker") {
          let contract: WorkerCapabilityContract;
          try {
            contract = await workerContract(ctx.cwd, environment);
          } catch {
            contract = createWorkerCapabilityContract({
              workingTree: resolve(ctx.cwd),
              temporaryPaths: [resolve(environment.TMPDIR ?? tmpdir())],
            });
            unmet.push("worker path scope is unavailable");
          }
          if (rulesConflict) {
            unmet.push(`Matty Rules conflict: ${rulesConflict}`);
          }
          if (unmet.length > 0 || !ctx.model || !invocation) {
            const terminal = blockedWorkerDelegation(contract, unmet);
            return {
              content: [
                { type: "text" as const, text: JSON.stringify(terminal) },
              ],
              details: terminal,
              isError: true,
            };
          }
          const workerInvocation = !declaresInvocationTools(invocation)
            ? invocationWithTools(invocation, WORKER_TOOLS)
            : invocation;
          const configurationPaths = userConfigurationPaths(environment);
          const writerStateRoot = contract.temporaryPaths.at(-1) ?? tmpdir();
          const protectedPaths = [singleWriterStatePath(writerStateRoot)];
          const terminal = await runWorkerDelegation(
            params.task,
            {
              contract,
              availability: {
                availableTools: invocationTools(workerInvocation),
                independentRuntime: independentRuntimeAvailable,
                inspectionGuard: false,
                workerGuard: true,
              },
              async acquireWriter() {
                return await acquireRepositoryWriter(
                  contract.workingTree,
                  writerStateRoot,
                );
              },
              createRunner() {
                return createChildPiRunner({
                  invocation: workerInvocation,
                  parent: {
                    provider: ctx.model?.provider ?? "",
                    model: ctx.model?.id ?? "",
                    thinking: thinkingLevel(ctx.thinkingLevel),
                    cwd: contract.workingTree,
                  },
                  authentication: {
                    provider: ctx.model?.provider ?? "",
                    environment: childEnvironment(
                      environment,
                      authentication?.ok
                        ? authentication.env
                        : undefined,
                      options.childEnvironment,
                      role,
                      {
                        contract,
                        protectedPaths,
                        ...(environment.HOME
                          ? { userHome: resolve(environment.HOME) }
                          : {}),
                        userConfigurationPaths: configurationPaths,
                      },
                    ),
                  },
                });
              },
            },
            progressOptions as never,
          );
          return delegationResult(terminal);
        }

        const github = role === "reviewer"
          ? await (
            options.reviewerGithubPreflight ??
              (() => reviewerGithubPreflight(environment))
          )()
          : { available: false, authenticated: false };
        const contract = inspectionCapabilityContract(role);
        if (
          activeInvocations[role] >= contract.concurrency.maxActive
        ) {
          unmet.push(
            `${role} concurrency limit reached: ${activeInvocations[role]} active`,
          );
        }
        if (unmet.length > 0 || !ctx.model || !invocation) {
          const terminal = blockedInspectionDelegation(role, unmet);
          return {
            content: [{ type: "text" as const, text: JSON.stringify(terminal) }],
            details: terminal,
            isError: true,
          };
        }

        const inspectionInvocation = !declaresInvocationTools(invocation)
          ? invocationWithTools(invocation, INSPECTION_TOOLS)
          : invocation;
        activeInvocations[role] += 1;
        try {
          const terminal = await runInspectionDelegation(
            role,
            params.task,
            {
              availability: {
                availableTools: invocationTools(inspectionInvocation),
                independentRuntime: independentRuntimeAvailable,
                inspectionGuard: true,
                github,
              },
              createRunner() {
                return createChildPiRunner({
                  invocation: inspectionInvocation,
                  parent: {
                    provider: ctx.model?.provider ?? "",
                    model: ctx.model?.id ?? "",
                    thinking: thinkingLevel(ctx.thinkingLevel),
                    cwd: ctx.cwd,
                  },
                  authentication: {
                    provider: ctx.model?.provider ?? "",
                    environment: childEnvironment(
                      environment,
                      authentication?.ok
                        ? authentication.env
                        : undefined,
                      options.childEnvironment,
                      role,
                    ),
                  },
                });
              },
            },
            progressOptions as never,
          );
          return delegationResult(terminal);
        } finally {
          activeInvocations[role] -= 1;
        }
      },
    } as never);
  }

  registerMatty(createPiHost(pi), {
    packageVersion: MATTY_PACKAGE_VERSION,
    piVersion: PI_VERSION,
    platform: process.platform,
    arch: process.arch,
  });
}

export default function mattyExtension(pi: ExtensionAPI): void {
  registerPiMatty(pi);
}
