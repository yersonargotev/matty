import {
  VERSION as PI_VERSION,
  type ExtensionAPI,
  type ExtensionContext,
  type ToolCallEvent,
} from "@earendil-works/pi-coding-agent";
import { fileURLToPath } from "node:url";

import {
  createChildPiRunner,
  type PiInvocation,
  type PiThinkingLevel,
} from "../application/child-pi-runtime.ts";
import {
  blockedExplorerDelegation,
  runExplorerDelegation,
} from "../application/explorer-delegation.ts";
import {
  registerMatty,
  type MattyHost,
} from "../application/register-matty.ts";
import { EXPLORER_TOOLS } from "../domain/capability-contract.ts";
import { inspectExplorerCommand } from "../domain/inspection-guard.ts";
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
}

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
      EXPLORER_TOOLS.join(","),
    ],
  };
}

function childEnvironment(
  environment: NodeJS.ProcessEnv,
  authenticationEnvironment: Readonly<Record<string, string>> | undefined,
  additions: NodeJS.ProcessEnv | undefined,
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
    MATTY_CHILD_ROLE: "explorer",
  };
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
  event: ToolCallEvent,
): { block: true; reason: string } | undefined {
  if (event.toolName === "edit" || event.toolName === "write") {
    return {
      block: true,
      reason:
        "Inspection Guard blocked recognized filesystem mutation; explorer tools are inspection-only",
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
        "Inspection Guard blocked malformed explorer bash input",
    };
  }
  const decision = inspectExplorerCommand(command);
  return decision.allowed
    ? undefined
    : { block: true, reason: decision.reason };
}

export function registerPiMatty(
  pi: ExtensionAPI,
  environment: NodeJS.ProcessEnv = process.env,
  options: PiMattyRegistrationOptions = {},
): void {
  const childRole =
    environment.MATTY_CHILD_ROLE === "explorer" ? "explorer" : undefined;
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
    pi.on("tool_call", (event) => blockedToolCall(event));
  } else {
    const invocation = options.invocation ?? currentPiInvocation();
    const independentRuntimeAvailable =
      options.independentRuntimeAvailable ?? invocation !== undefined;
    const parameters = {
      type: "object",
      properties: {
        task: {
          type: "string",
          minLength: 1,
          description: "One bounded codebase-inspection assignment.",
        },
      },
      required: ["task"],
      additionalProperties: false,
    };

    pi.registerTool({
      name: "subagent",
      label: "Explorer",
      description:
        "Run one independent, inspection-only explorer through the Matty Subagent Runtime.",
      promptSnippet:
        "Delegate one bounded codebase inspection to an independent explorer.",
      promptGuidelines: [
        'Call subagent with exactly {"task": string}.',
        "This v1 path runs exactly one independent explorer with read, grep, find, ls, and guarded bash.",
        "Progress and the terminal success, failure, cancellation, or preflight diagnostic are structured.",
        "The Inspection Guard is best-effort and blocks recognized local and remote mutation.",
      ],
      parameters: parameters as never,
      executionMode: "parallel",
      async execute(
        _toolCallId: string,
        params: { task: string },
        signal: AbortSignal | undefined,
        onUpdate:
          | ((update: {
              content: Array<{ type: "text"; text: string }>;
              details: unknown;
            }) => void)
          | undefined,
        ctx: ExtensionContext,
      ) {
        if (rulesConflict) {
          const terminal = blockedExplorerDelegation([
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
        if (unmet.length > 0 || !ctx.model || !invocation) {
          const terminal = blockedExplorerDelegation(unmet);
          return {
            content: [{ type: "text" as const, text: JSON.stringify(terminal) }],
            details: terminal,
            isError: true,
          };
        }

        const terminal = await runExplorerDelegation(
          params.task,
          {
            availability: {
              availableTools: EXPLORER_TOOLS,
              independentRuntime: independentRuntimeAvailable,
              inspectionGuard: true,
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
