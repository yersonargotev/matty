import {
  type ExtensionAPI,
  type ExtensionContext,
  type ToolCallEvent,
} from "@earendil-works/pi-coding-agent";
import * as piCodingAgent from "@earendil-works/pi-coding-agent";
import * as piAiCompat from "@earendil-works/pi-ai/compat";
import * as piTui from "@earendil-works/pi-tui";
import * as typebox from "typebox";
import { execFile } from "node:child_process";
import { readFileSync, realpathSync } from "node:fs";
import { lstat, realpath } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { createJiti } from "jiti";

import {
  createChildPiRunner,
  type PiInvocation,
  type PiThinkingLevel,
} from "../application/child-pi-runtime.ts";
import {
  CHILD_CONTROL_ENVIRONMENT as CONTROL_ENV,
  scrubChildControlEnvironment,
} from "../application/child-control-environment.ts";
import {
  blockedInspectionDelegation,
  runInspectionDelegation,
} from "../application/inspection-role-delegation.ts";
import {
  blockedResearcherDelegation,
  runResearcherDelegation,
} from "../application/researcher-delegation.ts";
import {
  blockedWorkerDelegation,
  runWorkerDelegation,
} from "../application/worker-delegation.ts";
import {
  isDelegationLeafFailureCode,
  runDelegationGroup,
  type DelegationLeafFailureCode,
  type DelegationTaskExecution,
} from "../application/delegation-scheduler.ts";
import {
  acquireRepositoryWriter,
  singleWriterStatePath,
} from "../application/single-writer.ts";
import {
  registerMatty,
  type DiagnosticContext,
  type MattyHost,
} from "../application/register-matty.ts";
import {
  DelegationRegistry,
  type DelegationRegistryOptions,
  type DelegationSnapshotEntry,
} from "../application/delegation-registry.ts";
import { createDelegationObserver } from "../application/delegation-observer.ts";
import {
  DelegationControl,
  type MattyApplicationControl,
} from "../application/delegation-control.ts";
import {
  delegationCard,
  renderDelegationTaskSection,
  renderDelegationHumanSnapshot,
  renderDelegationJson,
} from "../application/delegation-presentation.ts";
import { createPiDelegationManagement } from "./pi-delegation-management.ts";
import {
  INSPECTION_TOOLS,
  DELEGATION_INPUT_GUIDANCE,
  INSPECTION_ROLES,
  MATTY_ROLES,
  RESEARCHER_TOOLS,
  WORKER_TOOLS,
  createResearcherCapabilityContract,
  createWorkerCapabilityContract,
  inspectionCapabilityContract,
  isInspectionRole,
  isMattyRole,
  preflightCapability,
  validateCapabilityContract,
  type InspectionRole,
  type MattyRole,
  type ResearcherCapabilityContract,
  type WorkerCapabilityContract,
} from "../domain/capability-contract.ts";
import type {
  DelegationGroupContract,
  DelegationTaskDeclaration,
} from "../domain/delegation-group.ts";
import type { ReviewScopeContract } from "../domain/review-scope.ts";
import { inspectInspectionCommand } from "../domain/inspection-guard.ts";
import { injectMattyGuidance } from "../domain/matty-guidance.ts";
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
import type { RuntimeFacts } from "../domain/status.ts";
import {
  cleanupResearchWorkspace,
  cleanupStaleResearchWorkspaces,
  createResearchWorkspace,
  writeResearchFile,
  type ResearchWorkspace,
} from "../domain/research-workspace.ts";
import {
  WEB_CAPABILITY_TOOLS,
  createParentWebCapabilityContract,
  deriveWebCapabilityState,
  preflightWebCapability,
  runWebCapabilityOperation,
  validateWebCapabilityContract,
  type WebCapabilityContract,
  type WebCapabilityState,
} from "../domain/web-capability.ts";

export interface PiMattyRegistrationOptions {
  /** Explicit test seam. Production registration derives the launcher version. */
  hostPiVersion?: string;
  invocation?: PiInvocation;
  childEnvironment?: NodeJS.ProcessEnv;
  independentRuntimeAvailable?: boolean;
  registerWebExtension?: (pi: ExtensionAPI) => void;
  webContract?: WebCapabilityContract;
  reviewerGithubPreflight?: () => Promise<{
    available: boolean;
    authenticated: boolean;
  }>;
  diagnosticFailures?: RuntimeFacts["failures"];
  delegationRegistryOptions?: DelegationRegistryOptions;
  hostOutput?: (text: string) => void;
  /** Explicit test seam for the scheduler-to-resource-cleanup boundary. */
  resourceCleanupBarrier?: () => Promise<void>;
}

type WriterRelease = () => void | Promise<void>;

interface PreparedWorkerExecution {
  contract: WorkerCapabilityContract;
  takeWriterLease(): WriterRelease | undefined;
  releaseIfUnused(): Promise<void>;
}

interface PreparedResearcherExecution {
  contract: ResearcherCapabilityContract;
  scope: ResearchWorkspace;
  transferred: boolean;
}

type SingleTaskExecutionParams = DelegationTaskDeclaration & {
  executionScope?: "standalone" | "group";
  delegatedTaskId?: string;
  preparedWorker?: PreparedWorkerExecution;
  preparedResearcher?: PreparedResearcherExecution;
  onChildSettled?: () => void;
};

const execFileAsync = promisify(execFile);
const WEB_ACCESS_MODULE = "pi-web-access/index.ts";

function createPiHost(
  pi: ExtensionAPI,
  getDiagnosticContext: () => Pick<
    DiagnosticContext,
    "failures" | "concurrency"
  > = () => ({}),
  management?: {
    registry: DelegationRegistry;
    openConsole(context: ExtensionContext): Promise<void>;
    openTask(context: ExtensionContext, displayId: string): Promise<boolean>;
    output(text: string): void;
  },
): MattyHost {
  function diagnosticContext(
    context: ExtensionContext,
  ): DiagnosticContext {
    const referenceAuthentication =
      context.model?.provider === "openai-codex" &&
      typeof context.modelRegistry?.isUsingOAuth === "function" &&
      context.modelRegistry.isUsingOAuth(context.model)
        ? "chatgpt-codex-subscription" as const
        : undefined;
    return {
      mode: context.mode,
      ...(management
        ? {
          delegationSnapshot: () => {
            const snapshot = management.registry.snapshot();
            return {
              human: renderDelegationHumanSnapshot(snapshot, management.registry.now()),
              json: renderDelegationJson(snapshot),
              jsonEvent: JSON.stringify({
                type: "matty.delegations",
                snapshot,
              }),
            };
          },
          emitOutput: management.output,
          openDelegations: async () => {
            await management.openConsole(context);
          },
          openDelegatedTask: async (displayId: string) =>
            await management.openTask(context, displayId),
        }
        : {}),
      ...(context.model
        ? {
          activeModel: {
            provider: context.model.provider,
            model: context.model.id,
            ...(referenceAuthentication
              ? { authentication: referenceAuthentication }
              : {}),
          },
        }
        : {}),
      ...getDiagnosticContext(),
    };
  }

  return {
    registerCommand(name, command) {
      pi.registerCommand(name, {
        description: command.description,
        handler: async (args, context) => {
          await command.handle(args, (message, level) => {
            context.ui.notify(message, level);
          }, diagnosticContext(context));
        },
      });
    },
    onSessionStart(handler) {
      pi.on("session_start", async (event, context) => {
        await handler(event, (message, level) => {
          context.ui.notify(message, level);
        }, diagnosticContext(context));
      });
    },
  };
}

const PI_PACKAGE_NAME = "@earendil-works/pi-coding-agent";
const UNAVAILABLE_PI_VERSION = "unavailable";

export function detectLauncherPiVersion(
  entrypoint: string | null | undefined = process.argv[1],
): string | undefined {
  if (!entrypoint) return undefined;

  let directory: string;
  try {
    directory = dirname(realpathSync(entrypoint));
  } catch {
    return undefined;
  }

  while (true) {
    const manifestPath = join(directory, "package.json");
    try {
      const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as {
        name?: unknown;
        version?: unknown;
      };
      if (manifest.name === PI_PACKAGE_NAME) {
        return typeof manifest.version === "string" && manifest.version.length > 0
          ? manifest.version
          : undefined;
      }
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") return undefined;
    }

    const parent = dirname(directory);
    if (parent === directory) return undefined;
    directory = parent;
  }
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
  research?: {
    contract: ResearcherCapabilityContract;
    scope: ResearchWorkspace;
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
    [CONTROL_ENV.role]: role,
  };
  if (research) {
    return {
      ...child,
      [CONTROL_ENV.researchContract]: JSON.stringify(research.contract),
      [CONTROL_ENV.researchScope]: JSON.stringify(research.scope),
    };
  }
  if (!worker) {
    return child;
  }
  return {
    ...child,
    [CONTROL_ENV.workerWorkingTree]: worker.contract.workingTree,
    [CONTROL_ENV.workerTemporaryPaths]: JSON.stringify(worker.contract.temporaryPaths),
    [CONTROL_ENV.workerProtectedPaths]: JSON.stringify(worker.protectedPaths),
    ...(worker.userHome ? { [CONTROL_ENV.workerUserHome]: worker.userHome } : {}),
    [CONTROL_ENV.workerUserConfigurationPaths]: JSON.stringify(worker.userConfigurationPaths),
  };
}

function researcherScope(
  environment: NodeJS.ProcessEnv,
): {
  contract: ResearcherCapabilityContract;
  scope: ResearchWorkspace;
} | undefined {
  try {
    const contract = JSON.parse(
      environment[CONTROL_ENV.researchContract] ?? "null",
    ) as unknown;
    const scope = JSON.parse(
      environment[CONTROL_ENV.researchScope] ?? "null",
    ) as unknown;
    const validation = validateCapabilityContract(contract);
    if (
      !validation.ok ||
      validation.contract.role !== "researcher" ||
      typeof scope !== "object" ||
      scope === null ||
      Array.isArray(scope)
    ) {
      return undefined;
    }
    const candidate = scope as Partial<ResearchWorkspace>;
    if (
      typeof candidate.temporaryRoot !== "string" ||
      typeof candidate.projectRoot !== "string" ||
      candidate.temporaryRoot !== validation.contract.workspaceRoot ||
      candidate.projectRoot !== validation.contract.projectRoot ||
      candidate.workspace !== validation.contract.workspace ||
      candidate.report !== validation.contract.report
    ) {
      return undefined;
    }
    return {
      contract: validation.contract,
      scope: candidate as ResearchWorkspace,
    };
  } catch {
    return undefined;
  }
}

async function reviewCommitsAvailable(
  cwd: string,
  scope: ReviewScopeContract,
): Promise<boolean> {
  try {
    await Promise.all([scope.baseSha, scope.candidateSha].map((sha) =>
      execFileAsync("git", ["cat-file", "-e", `${sha}^{commit}`], {
        cwd,
        timeout: 5_000,
      })
    ));
    return true;
  } catch {
    return false;
  }
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
    const workingTree = environment[CONTROL_ENV.workerWorkingTree];
    const temporaryPaths = JSON.parse(environment[CONTROL_ENV.workerTemporaryPaths] ?? "[]") as unknown;
    const userConfigurationPaths = JSON.parse(environment[CONTROL_ENV.workerUserConfigurationPaths] ?? "[]") as unknown;
    const protectedPaths = JSON.parse(environment[CONTROL_ENV.workerProtectedPaths] ?? "[]") as unknown;
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
    const userHome = environment[CONTROL_ENV.workerUserHome];
    return {
      workingTree: contract.workingTree,
      temporaryPaths: contract.temporaryPaths,
      protectedPaths: protectedPaths as string[],
      ...(userHome ? { userHome } : {}),
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

function isUnknownRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function leafOutcome(details: unknown): {
  status?: unknown;
  failureCode?: DelegationLeafFailureCode;
} {
  if (!isUnknownRecord(details) || !isUnknownRecord(details.outcome)) {
    return {};
  }
  const { status, failure } = details.outcome;
  if (!isUnknownRecord(failure)) {
    return { status };
  }
  return {
    status,
    ...(isDelegationLeafFailureCode(failure.kind)
      ? { failureCode: failure.kind }
      : {}),
  };
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

function blockedCapabilityResult(
  contractId: string,
  unmet: string[],
): {
  content: Array<{ type: "text"; text: string }>;
  details: {
    contract: null;
    outcome: {
      status: "blocked";
      diagnostic: {
        kind: "capability-preflight";
        contractId: string;
        unmet: string[];
      };
    };
  };
  isError: true;
} {
  const details = {
    contract: null,
    outcome: {
      status: "blocked" as const,
      diagnostic: {
        kind: "capability-preflight" as const,
        contractId,
        unmet,
      },
    },
  };
  return {
    content: [{ type: "text", text: JSON.stringify(details) }],
    details,
    isError: true,
  };
}

function defaultResearchReport(task: string): string {
  const slug = task
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 80)
    .replace(/-$/g, "") || "research";
  return `docs/research/${slug}.md`;
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
  if (!isInspectionRole(role)) {
    return undefined;
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
): MattyApplicationControl {
  const delegationRegistry = new DelegationRegistry(options.delegationRegistryOptions);
  const delegationControl = new DelegationControl({
    ...(options.delegationRegistryOptions?.terminalLimit !== undefined
      ? { terminalLimit: options.delegationRegistryOptions.terminalLimit }
      : {}),
  });
  const delegationManagement = createPiDelegationManagement(delegationRegistry, delegationControl);
  const resultCards = new WeakMap<object, DelegationSnapshotEntry>();
  const diagnosticFailures: Array<
    NonNullable<RuntimeFacts["failures"]>[number]
  > = [...(options.diagnosticFailures ?? [])];
  const childRoleValue = environment[CONTROL_ENV.role];
  const childRole = isMattyRole(childRoleValue) ? childRoleValue : undefined;
  const research = childRole === "researcher" ? researcherScope(environment) : undefined;
  const capturedWorkerScope = childRole === "worker" ? workerGuardScope(environment) : undefined;
  // Control contracts are one-hop bootstrap inputs, not ambient child authority.
  // Capture them before any repository-visible tool or process can run.
  scrubChildControlEnvironment(environment);

  const registeredWebTools: string[] = [];
  let webState: WebCapabilityState = "unavailable";
  let webInitializationSucceeded = false;
  const webContractValidation = validateWebCapabilityContract(
    childRole === "researcher" && research
      ? createParentWebCapabilityContract(research.contract.web)
      : options.webContract ?? createParentWebCapabilityContract("required"),
  );
  if (
    (!childRole || (childRole === "researcher" && research)) &&
    options.registerWebExtension &&
    webContractValidation.ok
  ) {
    const certifiedTools = new Set<string>(WEB_CAPABILITY_TOOLS);
    const webContract = webContractValidation.contract;
    const webPolicyResult = (resolution: {
      status: string;
      disclosure?: string;
    }) => ({
      content: [{
        type: "text" as const,
        text: resolution.disclosure ??
          "Required web operation failed. No web research was completed; model knowledge is not web research.",
      }],
      details: resolution,
      isError: resolution.status === "blocked",
    });
    const webApi = new Proxy(pi, {
      get(target, property) {
        if (property === "registerTool") {
          return (tool: {
            name: string;
            execute?: (...args: never[]) => Promise<{ isError?: boolean }>;
          }) => {
            if (!certifiedTools.has(tool.name)) {
              return;
            }
            registeredWebTools.push(tool.name);
            if (
              webContract.requirement !== "none" &&
              registeredWebTools.indexOf(tool.name) ===
                registeredWebTools.lastIndexOf(tool.name)
            ) {
              const upstreamExecute = tool.execute;
              target.registerTool({
                ...tool,
                ...(upstreamExecute
                  ? {
                    async execute(...args: never[]) {
                      const preflight = preflightWebCapability(
                        webContract,
                        webState,
                      );
                      if (preflight.status !== "ready") {
                        return webPolicyResult(preflight);
                      }
                      try {
                        const result = await upstreamExecute(...args);
                        if (result.isError) {
                          return webPolicyResult(
                            runWebCapabilityOperation(
                              webContract,
                              { ok: false },
                            ),
                          );
                        }
                        runWebCapabilityOperation(webContract, {
                          ok: true,
                          source: "web-tool",
                        });
                        return result;
                      } catch {
                        return webPolicyResult(
                          runWebCapabilityOperation(
                            webContract,
                            { ok: false },
                          ),
                        );
                      }
                    },
                  }
                  : {}),
              } as never);
            }
          };
        }
        const value = Reflect.get(target, property, target) as unknown;
        return typeof value === "function" ? value.bind(target) : value;
      },
    });
    try {
      options.registerWebExtension(webApi);
      webInitializationSucceeded = true;
    } catch {
      // Keep Matty diagnostics available. Status reports the local integration
      // as degraded or unavailable without exposing the provider-owned error.
    }
    webState = deriveWebCapabilityState({
      registeredTools: registeredWebTools,
      initializationSucceeded: webInitializationSucceeded,
    });
  }
  if (research) {
    pi.registerTool({
      name: "research_file",
      label: "Research File",
      description:
        "Write a new file in the validated Research Workspace or the one approved Research Report.",
      parameters: {
        type: "object",
        properties: {
          destination: {
            type: "string",
            enum: ["workspace", "report"],
          },
          path: {
            type: "string",
            description:
              "Relative workspace path. Omit for the approved report.",
          },
          content: { type: "string" },
        },
        required: ["destination", "content"],
        additionalProperties: false,
      } as never,
      async execute(
        _toolCallId: string,
        input: {
          destination: "workspace" | "report";
          path?: string;
          content: string;
        },
      ) {
        try {
          const result = input.destination === "workspace" &&
              typeof input.path === "string"
            ? await writeResearchFile(research.scope, {
              destination: "workspace",
              path: input.path,
              content: input.content,
            })
            : input.destination === "report" && input.path === undefined
              ? await writeResearchFile(research.scope, {
                destination: "report",
                content: input.content,
              })
              : undefined;
          if (!result) {
            throw new Error("research file destination is invalid");
          }
          return {
            content: [{
              type: "text" as const,
              text: JSON.stringify(result),
            }],
            details: result,
          };
        } catch (error) {
          const message = error instanceof Error
            ? error.message
            : "research file write failed";
          return {
            content: [{ type: "text" as const, text: message }],
            details: { error: message },
            isError: true,
          };
        }
      },
    } as never);
  }
  let rulesConflict: string | undefined;
  pi.on("before_agent_start", (event) => {
    rulesConflict = detectMattyRulesConflict(event.systemPrompt);
    return {
      systemPrompt: injectMattyRules(
        injectMattyGuidance(event.systemPrompt),
        childRole ?? "parent",
      ),
    };
  });

  if (childRole) {
    if (childRole !== "researcher") {
      const scope = childRole === "worker" ? capturedWorkerScope : undefined;
      pi.on("tool_call", (event) => blockedToolCall(childRole, event, scope));
    }
  } else {
    const activeInvocations = Object.fromEntries(
      INSPECTION_ROLES.map((role) => [role, 0]),
    ) as Record<InspectionRole, number>;
    let activeResearchers = 0;
    const sessionResearchWorkspaces = new Map<string, ResearchWorkspace>();
    const researchTemporaryRoot = resolve(
      environment.TMPDIR ?? tmpdir(),
      "matty",
      "research",
    );
    let researchCleanup: Promise<void> | undefined;
    const prepareResearchRoot = async () => {
      researchCleanup ??= cleanupStaleResearchWorkspaces({
        temporaryRoot: researchTemporaryRoot,
      }).then(() => undefined);
      await researchCleanup;
    };
    pi.on("session_start", (event, context) => {
      delegationControl.reset();
      delegationManagement.startSession(event.reason, context);
    });
    pi.on("session_shutdown", async () => {
      delegationControl.shutdown();
      delegationManagement.shutdown();
      for (const scope of sessionResearchWorkspaces.values()) {
        try {
          await cleanupResearchWorkspace(scope);
          sessionResearchWorkspaces.delete(scope.workspace);
        } catch {
          // A failed validation is safer to leave untouched for startup cleanup.
        }
      }
    });
    const invocation = options.invocation ?? currentPiInvocation();
    const independentRuntimeAvailable =
      options.independentRuntimeAvailable ?? invocation !== undefined;
    const parameters = {
      type: "object",
      properties: {
        requirement: {
          type: "string",
          enum: ["required", "optional"],
          description:
            "Required groups are atomic; optional inspection groups disclose skipped work.",
        },
        tasks: {
          type: "array",
          minItems: 1,
          maxItems: 8,
          items: {
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
              web: {
                type: "string",
                enum: ["required", "optional"],
                description:
                  "Required for researcher; rejected for other roles.",
              },
              report: {
                type: "string",
                minLength: 1,
                description:
                  "Parent-approved Markdown report path for researcher.",
              },
              reviewScope: {
                type: "object",
                description: "Required closed Review Scope Contract for reviewer only.",
                properties: {
                  schemaVersion: { type: "number", const: 1 },
                  issue: {
                    type: "object",
                    properties: {
                      repository: { type: "string", minLength: 1 },
                      number: { type: "number", minimum: 1 },
                      reference: { type: "string", minLength: 1 },
                    },
                    required: ["repository", "number", "reference"],
                    additionalProperties: false,
                  },
                  requirements: {
                    type: "array", minItems: 1,
                    items: { type: "string", minLength: 1 },
                  },
                  outOfScope: {
                    type: "array",
                    items: {
                      type: "object",
                      properties: {
                        reference: { type: "string", minLength: 1 },
                        reason: { type: "string", minLength: 1 },
                      },
                      required: ["reference", "reason"],
                      additionalProperties: false,
                    },
                  },
                  baseSha: { type: "string", pattern: "^(?:[0-9a-f]{40}|[0-9a-f]{64})$" },
                  candidateSha: { type: "string", pattern: "^(?:[0-9a-f]{40}|[0-9a-f]{64})$" },
                  axes: {
                    type: "array", minItems: 1,
                    items: {
                      type: "string",
                      enum: ["standards", "spec", "security", "correctness", "maintainability"],
                    },
                  },
                },
                required: ["schemaVersion", "issue", "requirements", "outOfScope", "baseSha", "candidateSha", "axes"],
                additionalProperties: false,
              },
            },
            required: ["role", "task"],
            allOf: [{
              if: { properties: { role: { const: "reviewer" } }, required: ["role"] },
              then: { required: ["reviewScope"] },
              else: { not: { required: ["reviewScope"] } },
            }],
            additionalProperties: false,
          },
        },
      },
      required: ["requirement", "tasks"],
      additionalProperties: false,
    };

    const trackRunner = <T extends ReturnType<typeof createChildPiRunner>>(
      taskId: string | undefined,
      runner: T,
      onChildSettled?: () => void,
    ): T => {
      if (taskId) delegationControl.attachRunner(taskId, runner);
      if (onChildSettled) {
        const run = runner.run.bind(runner);
        runner.run = async (task, options) => {
          try {
            return await run(task, options);
          } finally {
            onChildSettled();
          }
        };
      }
      return runner;
    };

    const singleTaskTool = {
      name: "subagent",
      label: "Matty Role",
      description:
        "Run one independent explorer, designer, reviewer, researcher, or worker through the Matty Subagent Runtime.",
      promptSnippet:
        "Delegate one bounded task to a named Matty Role.",
      promptGuidelines: [
        `Call subagent with exactly ${DELEGATION_INPUT_GUIDANCE}.`,
        "Researcher also requires web (required or optional) and one approved Markdown report path.",
        "Reviewer requires one closed reviewScope matching the documented exact shape; other roles reject it.",
        "Inspection roles receive read, grep, find, ls, and guarded bash; worker also receives edit and write.",
        "Only reviewer may perform read-only gh inspection after availability and authentication preflight.",
        "Single Writer permits at most one active worker per repository.",
        "Progress and the terminal success, failure, cancellation, or preflight diagnostic are structured.",
        "Inspection Guard and Worker Guard are best-effort policies, not security sandboxes.",
      ],
      parameters: {} as never,
      executionMode: "parallel",
      async execute(
        _toolCallId: string,
        params: SingleTaskExecutionParams,
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
          return blockedCapabilityResult("delegate-invalid", [
            "unsupported Matty Role",
          ]);
        }
        const role = params.role;
        if (isInspectionRole(role) && rulesConflict) {
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
          if (params.preparedWorker) {
            contract = params.preparedWorker.contract;
          } else {
            try {
              contract = await workerContract(ctx.cwd, environment);
            } catch {
              contract = createWorkerCapabilityContract({
                workingTree: resolve(ctx.cwd),
                temporaryPaths: [resolve(environment.TMPDIR ?? tmpdir())],
              });
              unmet.push("worker path scope is unavailable");
            }
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
                if (params.preparedWorker) {
                  return params.preparedWorker.takeWriterLease();
                }
                return await acquireRepositoryWriter(
                  contract.workingTree,
                  writerStateRoot,
                );
              },
              createRunner() {
                return trackRunner(params.delegatedTaskId, createChildPiRunner({
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
                }), params.onChildSettled);
              },
            },
            progressOptions as never,
          );
          return delegationResult(terminal);
        }

        if (role === "researcher") {
          const web = params.web;
          const report = params.report?.trim() ||
            defaultResearchReport(params.task);
          if (web !== "required" && web !== "optional") {
            return blockedCapabilityResult("delegate-researcher", [
              "researcher requires a web requirement",
            ]);
          }
          let scope: ResearchWorkspace;
          if (params.preparedResearcher) {
            scope = params.preparedResearcher.scope;
          } else {
            try {
              await prepareResearchRoot();
              scope = await createResearchWorkspace({
                temporaryRoot: researchTemporaryRoot,
                projectRoot: ctx.cwd,
                report,
              });
            } catch {
              return blockedCapabilityResult("delegate-researcher", [
                "research artifact destinations are invalid",
              ]);
            }
          }
          sessionResearchWorkspaces.set(scope.workspace, scope);
          if (params.preparedResearcher) {
            params.preparedResearcher.transferred = true;
          }
          const contract = params.preparedResearcher?.contract ??
            createResearcherCapabilityContract({
              web,
              workspaceRoot: scope.temporaryRoot,
              projectRoot: scope.projectRoot,
              workspace: scope.workspace,
              report: scope.report,
            });
          if (rulesConflict) {
            unmet.push(`Matty Rules conflict: ${rulesConflict}`);
          }
          if (
            params.executionScope !== "group" &&
            activeResearchers >= contract.concurrency.maxActive
          ) {
            unmet.push(
              `researcher concurrency limit reached: ${activeResearchers} active`,
            );
          }
          if (unmet.length > 0 || !ctx.model || !invocation) {
            return delegationResult(
              blockedResearcherDelegation(contract, unmet),
            );
          }
          const researcherInvocation = !declaresInvocationTools(invocation)
            ? invocationWithTools(invocation, RESEARCHER_TOOLS)
            : invocation;
          activeResearchers += 1;
          try {
            const terminal = await runResearcherDelegation(
              params.task,
              {
                contract,
                availability: {
                  availableTools: invocationTools(researcherInvocation),
                  independentRuntime: independentRuntimeAvailable,
                  inspectionGuard: false,
                  researchFileTool: true,
                  web: webState,
                },
                createRunner() {
                  return trackRunner(params.delegatedTaskId, createChildPiRunner({
                    invocation: researcherInvocation,
                    parent: {
                      provider: ctx.model?.provider ?? "",
                      model: ctx.model?.id ?? "",
                      thinking: thinkingLevel(ctx.thinkingLevel),
                      cwd: scope.projectRoot,
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
                        undefined,
                        { contract, scope },
                      ),
                    },
                  }), params.onChildSettled);
                },
                async reportDelivered() {
                  try {
                    const stats = await lstat(contract.report);
                    return stats.isFile() && !stats.isSymbolicLink();
                  } catch {
                    return false;
                  }
                },
              },
              progressOptions as never,
            );
            return delegationResult(terminal);
          } finally {
            activeResearchers -= 1;
          }
        }

        const github = role === "reviewer"
          ? await (
            options.reviewerGithubPreflight ??
              (() => reviewerGithubPreflight(environment))
          )()
          : { available: false, authenticated: false };
        const contract = inspectionCapabilityContract(role);
        if (
          params.executionScope !== "group" &&
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
              async reviewCommitsAvailable(scope) {
                return await reviewCommitsAvailable(ctx.cwd, scope);
              },
              createRunner() {
                return trackRunner(params.delegatedTaskId, createChildPiRunner({
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
                }), params.onChildSettled);
              },
            },
            {
              ...progressOptions,
              ...(role === "reviewer" ? { reviewScope: params.reviewScope } : {}),
            } as never,
          );
          return delegationResult(terminal);
        } finally {
          activeInvocations[role] -= 1;
        }
      },
    };

    pi.registerTool({
      ...singleTaskTool,
      description:
        "Run one to eight independent Matty Role tasks with at most four active children.",
      promptSnippet:
        "Delegate a required atomic group or an optional inspection group.",
      promptGuidelines: [
        `Call subagent with exactly ${DELEGATION_INPUT_GUIDANCE}.`,
        "A required group is atomic: one failure cancels remaining work and never falls back inline.",
        "An optional group may contain inspection roles only and reports unavailable work as skipped.",
        "At most eight tasks are accepted and at most four children run concurrently; overflow is queued.",
        "Researcher requires web and may receive one approved Markdown report path.",
        "Reviewer requires one closed reviewScope matching the documented exact shape; other roles reject it.",
        "Single Writer permits at most one worker per group and per repository.",
        "Inspection Guard and Worker Guard are best-effort policies, not security sandboxes.",
      ],
      parameters: parameters as never,
      renderShell: "self",
      renderCall(args: unknown) {
        const candidate = isUnknownRecord(args) ? args : {};
        const tasks = Array.isArray(candidate.tasks) ? candidate.tasks : [candidate];
        const safeRoles = tasks.flatMap((task) =>
          isUnknownRecord(task) && isMattyRole(task.role) ? [task.role] : []
        );
        const text = `Delegation · ${[...new Set(safeRoles)].join(",") || "unknown"} · ${tasks.length} task${tasks.length === 1 ? "" : "s"}`;
        return new piTui.Text(text, 0, 0);
      },
      renderResult(result: { details?: unknown }) {
        const details = result.details;
        const entry = isUnknownRecord(details)
          ? resultCards.get(details) ?? (isUnknownRecord(details.delegation)
            ? details.delegation as unknown as DelegationSnapshotEntry
            : undefined)
          : undefined;
        return new piTui.Text(entry ? renderDelegationTaskSection(entry, delegationRegistry.now()).join("\n") : "Delegation · lifecycle unavailable", 0, 0);
      },
      async execute(
        toolCallId: string,
        params:
          | {
              requirement: "required" | "optional";
              tasks: DelegationTaskDeclaration[];
            }
          | DelegationTaskDeclaration,
        signal: AbortSignal | undefined,
        onUpdate:
          | ((update: {
              content: Array<{ type: "text"; text: string }>;
              details: unknown;
            }) => void)
          | undefined,
        ctx: ExtensionContext,
      ) {
        const observer = createDelegationObserver({
          registry: delegationRegistry,
          declaration: params,
          ...(signal ? { signal } : {}),
          ...(onUpdate ? { onUpdate } : {}),
        });
        const declaredTasks = "role" in params ? [params] : params.tasks;
        const requirement = "role" in params ? "required" : params.requirement;
        delegationControl.open(
          observer.id,
          requirement,
          declaredTasks.map((_, taskIndex) => observer.taskId(taskIndex)),
          () => observer.abort(),
        );
        let controlCompleted = false;
        const beginFreeze = (): void => {
          void delegationControl.freeze(observer.id).catch(() => {
            // Session shutdown may discard control state while a child is settling.
          });
        };
        const completeControl = (result: unknown): unknown => {
          const completed = delegationControl.complete(observer.id, result);
          controlCompleted = true;
          return completed;
        };
        const failAndCompleteControl = (): void => {
          beginFreeze();
          if (!controlCompleted) {
            observer.fail();
            completeControl(Object.freeze({ status: "failed" }));
          }
        };
        const finishResult = <T extends { details?: unknown; content?: unknown }>(result: T): T => {
          const finished = observer.finish(result.details);
          if (finished.entry && isUnknownRecord(finished.safeDetails)) {
            resultCards.set(finished.safeDetails, finished.entry);
          }
          const terminal = {
            ...result,
            details: finished.safeDetails,
            ...(Array.isArray(result.content)
              ? {
                content: [{
                  type: "text",
                  text: JSON.stringify(finished.safeDetails),
                }],
              }
              : {}),
          };
          return completeControl(terminal) as T;
        };
        if ("role" in params) {
          try {
            const result = await singleTaskTool.execute(
              toolCallId,
              {
                ...params,
                delegatedTaskId: observer.taskId(0),
                onChildSettled: beginFreeze,
              },
              observer.signal,
              (update) => observer.observeProgress(update.details),
              ctx,
            );
            // Preflight-only executions have no child boundary to trigger this callback.
            beginFreeze();
            return finishResult(result);
          } catch (error) {
            failAndCompleteControl();
            throw error;
          }
        }
        const contract: DelegationGroupContract = {
          schemaVersion: 1,
          id: "delegate-group",
          requirement: params.requirement,
          fallback: params.requirement === "required" ? "none" : "skip",
          atomic: params.requirement === "required",
          cardinality: { min: 1, max: 8 },
          concurrency: { maxActive: 4 },
          independence: "required",
          tasks: params.tasks.map((task) =>
            task.role === "researcher"
              ? {
                ...task,
                report: resolve(
                  ctx.cwd,
                  task.report?.trim() || defaultResearchReport(task.task),
                ),
              }
              : task
          ),
        };
        let authenticationPreflight: Promise<boolean> | undefined;
        const preparedWorkers = new Map<number, PreparedWorkerExecution>();
        const preparedResearchers = new Map<
          number,
          PreparedResearcherExecution
        >();
        const result = await (async () => {
          try {
            try {
              const scheduled = await runDelegationGroup(contract, {
              async preflight(task, taskIndex) {
                if (
                  !ctx.model ||
                  !invocation ||
                  !independentRuntimeAvailable
                ) {
                  return { ok: false, reason: "runtime-unavailable" };
                }
                if (rulesConflict) {
                  return { ok: false, reason: "rules-conflict" };
                }
                authenticationPreflight ??= ctx.modelRegistry
                  .getApiKeyAndHeaders(ctx.model)
                  .then((authentication) => authentication.ok)
                  .catch(() => false);
                if (!(await authenticationPreflight)) {
                  return {
                    ok: false,
                    reason: "authentication-unavailable",
                  };
                }

                if (isInspectionRole(task.role)) {
                  const github = task.role === "reviewer"
                    ? await (
                      options.reviewerGithubPreflight ??
                        (() => reviewerGithubPreflight(environment))
                    )()
                    : { available: false, authenticated: false };
                  if (
                    task.role === "reviewer" &&
                    (!github.available || !github.authenticated)
                  ) {
                    return { ok: false, reason: "github-unavailable" };
                  }
                  if (
                    task.role === "reviewer" &&
                    (!task.reviewScope ||
                      !(await reviewCommitsAvailable(ctx.cwd, task.reviewScope)))
                  ) {
                    return { ok: false, reason: "review-commit-unavailable" };
                  }
                  const roleInvocation = !declaresInvocationTools(invocation)
                    ? invocationWithTools(invocation, INSPECTION_TOOLS)
                    : invocation;
                  const rolePreflight = preflightCapability(
                    inspectionCapabilityContract(task.role),
                    {
                      availableTools: invocationTools(roleInvocation),
                      independentRuntime: independentRuntimeAvailable,
                      inspectionGuard: true,
                      github,
                    },
                  );
                  return rolePreflight.ok
                    ? { ok: true }
                    : { ok: false, reason: "tool-surface-incompatible" };
                }

                if (task.role === "worker") {
                  let workerCapability: WorkerCapabilityContract;
                  try {
                    workerCapability = await workerContract(
                      ctx.cwd,
                      environment,
                    );
                  } catch {
                    return {
                      ok: false,
                      reason: "artifact-destination-invalid",
                    };
                  }
                  const roleInvocation = !declaresInvocationTools(invocation)
                    ? invocationWithTools(invocation, WORKER_TOOLS)
                    : invocation;
                  const rolePreflight = preflightCapability(
                    workerCapability,
                    {
                      availableTools: invocationTools(roleInvocation),
                      independentRuntime: independentRuntimeAvailable,
                      inspectionGuard: false,
                      workerGuard: true,
                    },
                  );
                  if (!rolePreflight.ok) {
                    return {
                      ok: false,
                      reason: "tool-surface-incompatible",
                    };
                  }
                  const writerStateRoot =
                    workerCapability.temporaryPaths.at(-1) ?? tmpdir();
                  let writerLease = await acquireRepositoryWriter(
                    workerCapability.workingTree,
                    writerStateRoot,
                  );
                  if (!writerLease) {
                    return { ok: false, reason: "writer-unavailable" };
                  }
                  preparedWorkers.set(taskIndex, {
                    contract: workerCapability,
                    takeWriterLease() {
                      const lease = writerLease;
                      writerLease = undefined;
                      return lease;
                    },
                    async releaseIfUnused() {
                      const lease = writerLease;
                      writerLease = undefined;
                      await lease?.();
                    },
                  });
                  return { ok: true };
                }

                if (task.web === "required" && webState !== "available") {
                  return { ok: false, reason: "web-unavailable" };
                }
                let scope: ResearchWorkspace;
                try {
                  await prepareResearchRoot();
                  scope = await createResearchWorkspace({
                    temporaryRoot: researchTemporaryRoot,
                    projectRoot: ctx.cwd,
                    report: task.report ??
                      defaultResearchReport(task.task),
                  });
                } catch {
                  return {
                    ok: false,
                    reason: "artifact-destination-invalid",
                  };
                }
                const researcherCapability =
                  createResearcherCapabilityContract({
                    web: task.web ?? "required",
                    workspaceRoot: scope.temporaryRoot,
                    projectRoot: scope.projectRoot,
                    workspace: scope.workspace,
                    report: scope.report,
                  });
                const preparation: PreparedResearcherExecution = {
                  contract: researcherCapability,
                  scope,
                  transferred: false,
                };
                preparedResearchers.set(taskIndex, preparation);
                const roleInvocation = !declaresInvocationTools(invocation)
                  ? invocationWithTools(invocation, RESEARCHER_TOOLS)
                  : invocation;
                const rolePreflight = preflightCapability(
                  researcherCapability,
                  {
                    availableTools: invocationTools(roleInvocation),
                    independentRuntime: independentRuntimeAvailable,
                    inspectionGuard: false,
                    researchFileTool: true,
                    web: webState,
                  },
                );
                return rolePreflight.ok
                  ? { ok: true }
                  : { ok: false, reason: "tool-surface-incompatible" };
              },
              async run(task, taskIndex, taskOptions) {
                const preparedWorker = preparedWorkers.get(taskIndex);
                const preparedResearcher = preparedResearchers.get(taskIndex);
                const leafResult = await singleTaskTool.execute(
                  toolCallId,
                  {
                    ...task,
                    executionScope: "group",
                    delegatedTaskId: observer.taskId(taskIndex),
                    ...(preparedWorker ? { preparedWorker } : {}),
                    ...(preparedResearcher ? { preparedResearcher } : {}),
                  },
                  taskOptions.signal,
                  (update) => observer.observeProgress({
                    taskIndex,
                    progress: update.details,
                  }),
                  ctx,
                ).catch((error: unknown) => {
                  observer.completeTask(taskIndex, "failed");
                  throw error;
                });
                const outcome = leafOutcome(leafResult.details);
                const executionOutcome: DelegationTaskExecution<unknown> =
                  outcome.status === "cancelled"
                    ? { status: "cancelled" }
                    : leafResult.isError
                      ? {
                        status: "failed",
                        ...(outcome.failureCode
                          ? { code: outcome.failureCode }
                          : {}),
                      }
                      : {
                        status: "succeeded",
                        value: leafResult.details,
                      };
                observer.completeTask(taskIndex, executionOutcome.status);
                return executionOutcome;
              },
            }, {
              signal: observer.signal,
              onDiagnostic(diagnostic) {
                observer.recordDiagnostic(diagnostic);
              },
              onTaskAbort(taskIndex, abort) {
                delegationControl.attachAbort(observer.taskId(taskIndex), abort);
              },
            });
              // Close only after every scheduled/accepted group interaction has settled.
              beginFreeze();
              return scheduled;
            } catch (error) {
              beginFreeze();
              throw error;
            }
          } finally {
            await options.resourceCleanupBarrier?.();
            await Promise.all(
              [...preparedWorkers.values()].map(async (preparation) => {
                try {
                  await preparation.releaseIfUnused();
                } catch {
                  // Keep the closed-allowlist group result; stale leases expire.
                }
              }),
            );
            await Promise.all(
              [...preparedResearchers.values()].map(async (preparation) => {
                if (!preparation.transferred) {
                  try {
                    await cleanupResearchWorkspace(preparation.scope);
                  } catch {
                    // Marker-bearing workspaces remain eligible for startup cleanup.
                  }
                }
              }),
            );
          }
        })().catch((error: unknown) => {
          failAndCompleteControl();
          throw error;
        });
        try {
          return finishResult({
            content: [{ type: "text", text: JSON.stringify(result) }],
            details: result,
            isError:
              result.status === "blocked" ||
              result.status === "failed" ||
              result.status === "cancelled",
          });
        } catch (error) {
          failAndCompleteControl();
          throw error;
        }
      },
    } as never);
  }

  registerMatty(createPiHost(pi, () => ({
    failures: [
      ...diagnosticFailures,
      ...(rulesConflict ? [{ source: "rule-injection" as const }] : []),
    ],
    concurrency: {
      activeChildren: delegationRegistry.snapshot().concurrency.activeTasks,
      queuedChildren: delegationRegistry.snapshot().concurrency.queuedTasks,
    },
  }), {
    registry: delegationRegistry,
    openConsole: (context) => delegationManagement.openConsole(context),
    openTask: (context, displayId) => delegationManagement.openTask(context, displayId),
    output: options.hostOutput ?? ((text) => process.stdout.write(text)),
  }), {
    packageVersion: MATTY_PACKAGE_VERSION,
    piVersion: options.hostPiVersion ??
      detectLauncherPiVersion() ??
      UNAVAILABLE_PI_VERSION,
    platform: process.platform,
    arch: process.arch,
    subagentRuntimeAvailable:
      options.independentRuntimeAvailable ??
      (options.invocation ?? currentPiInvocation()) !== undefined,
    web: {
      state: webState,
      registeredTools: registeredWebTools.filter((tool) =>
        WEB_CAPABILITY_TOOLS.some((certified) => certified === tool)
      ),
    },
  });
  return delegationControl;
}

export default async function mattyExtension(pi: ExtensionAPI): Promise<void> {
  let registerWebExtension: ((api: ExtensionAPI) => void) | undefined;
  const diagnosticFailures: Array<
    NonNullable<RuntimeFacts["failures"]>[number]
  > = [];
  const jiti = createJiti(import.meta.url, {
    moduleCache: false,
    tryNative: false,
    virtualModules: {
      "@earendil-works/pi-coding-agent": piCodingAgent,
      "@earendil-works/pi-ai/compat": piAiCompat,
      "@earendil-works/pi-tui": piTui,
      typebox,
    },
  });
  try {
    registerWebExtension = await jiti.import(
      WEB_ACCESS_MODULE,
      { default: true },
    ) as (api: ExtensionAPI) => void;
  } catch {
    // Web availability is observable through local status.
  }
  registerPiMatty(pi, process.env, {
    ...(registerWebExtension ? { registerWebExtension } : {}),
    diagnosticFailures,
  });
}
