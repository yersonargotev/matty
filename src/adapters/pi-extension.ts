import {
  VERSION as PI_VERSION,
  type ExtensionAPI,
  type ExtensionContext,
  type ToolCallEvent,
} from "@earendil-works/pi-coding-agent";
import { execFile } from "node:child_process";
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
  registerMatty,
  type MattyHost,
} from "../application/register-matty.ts";
import {
  INSPECTION_TOOLS,
  INSPECTION_ROLE_INPUT_GUIDANCE,
  INSPECTION_ROLES,
  inspectionCapabilityContract,
  isInspectionRole,
  type InspectionRole,
} from "../domain/capability-contract.ts";
import { inspectInspectionCommand } from "../domain/inspection-guard.ts";
import {
  detectMattyRulesConflict,
  injectMattyRules,
} from "../domain/matty-rules.ts";
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
      "--tools",
      INSPECTION_TOOLS.join(","),
    ],
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

function childEnvironment(
  environment: NodeJS.ProcessEnv,
  authenticationEnvironment: Readonly<Record<string, string>> | undefined,
  additions: NodeJS.ProcessEnv | undefined,
  role: InspectionRole,
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
  return {
    ...inherited,
    ...authenticationEnvironment,
    ...additions,
    MATTY_CHILD_ROLE: role,
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

function blockedToolCall(
  role: InspectionRole,
  event: ToolCallEvent,
): { block: true; reason: string } | undefined {
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
  const childRole = isInspectionRole(environment.MATTY_CHILD_ROLE)
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
    pi.on("tool_call", (event) => blockedToolCall(childRole, event));
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
          enum: [...INSPECTION_ROLES],
          description: "The least-privilege inspection role.",
        },
        task: {
          type: "string",
          minLength: 1,
          description: "One bounded codebase-inspection assignment.",
        },
      },
      required: ["role", "task"],
      additionalProperties: false,
    };

    pi.registerTool({
      name: "subagent",
      label: "Inspection role",
      description:
        "Run one independent explorer, designer, or reviewer through the Matty Subagent Runtime.",
      promptSnippet:
        "Delegate one bounded inspection task to a named Matty Role.",
      promptGuidelines: [
        `Call subagent with exactly ${INSPECTION_ROLE_INPUT_GUIDANCE}.`,
        "Each call runs exactly one independent inspection role with read, grep, find, ls, and guarded bash.",
        "Only reviewer may perform read-only gh inspection after availability and authentication preflight.",
        "Progress and the terminal success, failure, cancellation, or preflight diagnostic are structured.",
        "The Inspection Guard is a best-effort command policy, not a security sandbox.",
      ],
      parameters: parameters as never,
      executionMode: "parallel",
      async execute(
        _toolCallId: string,
        params: { role: InspectionRole; task: string },
        signal: AbortSignal | undefined,
        onUpdate:
          | ((update: {
              content: Array<{ type: "text"; text: string }>;
              details: unknown;
            }) => void)
          | undefined,
        ctx: ExtensionContext,
      ) {
        if (!isInspectionRole(params.role)) {
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
        if (rulesConflict) {
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

        activeInvocations[role] += 1;
        try {
          const terminal = await runInspectionDelegation(
            role,
            params.task,
            {
              availability: {
                availableTools: invocationTools(invocation),
                independentRuntime: independentRuntimeAvailable,
                inspectionGuard: true,
                github,
              },
              createRunner() {
                return createChildPiRunner({
                  invocation,
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
            {
              ...(signal ? { signal } : {}),
              onProgress(progress) {
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
            },
          );
          return {
            content: [
              { type: "text" as const, text: JSON.stringify(terminal) },
            ],
            details: terminal,
            isError: terminal.outcome.status !== "succeeded",
          };
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
