import {
  isCertifiedHost,
  STARTUP_HINT,
} from "../domain/package-contract.ts";
import {
  createDiagnosticSnapshot,
  createStatusDiagnostic,
  renderDoctorHuman,
  renderDoctorJson,
  renderStatusHuman,
  renderStatusJson,
  type RuntimeFacts,
  type StatusDiagnostic,
} from "../domain/status.ts";

export type NotificationLevel = "info" | "warning" | "error";
export type Notify = (
  message: string,
  level: NotificationLevel,
) => void;

export interface DiagnosticContext {
  mode?: "tui" | "rpc" | "json" | "print";
  openDelegations?: () => Promise<void>;
  openDelegatedTask?: (displayId: string) => Promise<boolean>;
  interactWithDelegatedTask?: (
    delegatedTaskId: string,
    interaction: { type: "steer" | "follow_up"; message: string },
  ) => Promise<{ status: string; code?: string; commandId?: string }>;
  delegationSnapshot?: () => { human: string; json: string; jsonEvent: string };
  emitOutput?: (text: string) => void;
  activeModel?: {
    provider: string;
    model: string;
    authentication?: "chatgpt-codex-subscription";
  };
  failures?: RuntimeFacts["failures"];
  concurrency?: RuntimeFacts["concurrency"];
}

export type RuntimeFactInputs = Omit<RuntimeFacts, "activation">;

export interface MattyHost {
  registerCommand(
    name: string,
    command: {
      description: string;
      handle(
        args: string,
        notify: Notify,
        context?: DiagnosticContext,
      ): Promise<void>;
    },
  ): void;
  onSessionStart(
    handler: (
      event: { reason: string },
      notify: Notify,
      context?: DiagnosticContext,
    ) => Promise<void>,
  ): void;
}

export function registerMatty(
  host: MattyHost,
  runtimeFacts: RuntimeFactInputs,
): StatusDiagnostic {
  const activation = createActivation(runtimeFacts);
  let snapshot = createDiagnosticSnapshot({
    ...runtimeFacts,
    activation,
  });
  let snapshotKey = diagnosticKey(
    runtimeFacts.activeModel,
    runtimeFacts.failures,
  );
  let startupMessageEmitted = false;

  function createActivation(
    facts: RuntimeFactInputs,
  ): RuntimeFacts["activation"] {
    const compatible = isCertifiedHost(
      facts.piVersion,
      facts.platform,
      facts.arch,
    );
    return compatible
      ? {
        state: "active",
        reason: "compatible",
        codes: [],
      }
      : {
        state: "degraded",
        reason: "unsupported-host",
        codes: ["host-uncertified"],
      };
  }

  function diagnosticKey(
    activeModel: RuntimeFacts["activeModel"],
    failures: RuntimeFacts["failures"],
    concurrency: RuntimeFacts["concurrency"] = runtimeFacts.concurrency,
  ): string {
    return JSON.stringify([
      activeModel?.provider ?? null,
      activeModel?.model ?? null,
      activeModel?.authentication ?? null,
      (failures ?? []).map((failure) => failure.source),
      concurrency?.activeChildren ?? 0,
      concurrency?.queuedChildren ?? 0,
    ]);
  }

  function currentSnapshot(context?: DiagnosticContext) {
    const activeModel = context && Object.hasOwn(context, "activeModel")
      ? context.activeModel
      : runtimeFacts.activeModel;
    const failures = context && Object.hasOwn(context, "failures")
      ? context.failures
      : runtimeFacts.failures;
    const concurrency = context && Object.hasOwn(context, "concurrency")
      ? context.concurrency
      : runtimeFacts.concurrency;
    const nextSnapshotKey = diagnosticKey(activeModel, failures, concurrency);
    if (nextSnapshotKey !== snapshotKey) {
      snapshotKey = nextSnapshotKey;
      const {
        activeModel: _initialActiveModel,
        failures: _initialFailures,
        concurrency: _initialConcurrency,
        ...stableFacts
      } = runtimeFacts;
      snapshot = createDiagnosticSnapshot({
        ...stableFacts,
        activation,
        ...(activeModel ? { activeModel } : {}),
        ...(failures ? { failures } : {}),
        ...(concurrency ? { concurrency } : {}),
      });
    }
    return snapshot;
  }

  host.registerCommand("matty", {
    description: "Show Matty status or doctor diagnostics",
    handle: async (rawArgs, notify, context) => {
      const args = rawArgs.trim().replace(/\s+/g, " ");
      const diagnostic = currentSnapshot(context);

      if (args === "status") {
        notify(renderStatusHuman(diagnostic), "info");
        return;
      }

      if (args === "status --json") {
        notify(renderStatusJson(diagnostic), "info");
        return;
      }

      if (args === "doctor") {
        notify(renderDoctorHuman(diagnostic), "info");
        return;
      }

      if (args === "doctor --json") {
        notify(renderDoctorJson(diagnostic), "info");
        return;
      }

      const interactionMatch = /^(steer|follow-up) ([0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}) (.+)$/is.exec(rawArgs.trim());
      if (interactionMatch) {
        if (!context?.interactWithDelegatedTask) {
          notify("Delegated Task interaction is unavailable", "warning");
          return;
        }
        const result = await context.interactWithDelegatedTask(
          interactionMatch[2]!.toLowerCase(),
          {
            type: interactionMatch[1]!.toLowerCase() === "steer" ? "steer" : "follow_up",
            message: interactionMatch[3]!,
          },
        );
        notify(
          result.status === "accepted"
            ? `${interactionMatch[1] === "steer" ? "Steer" : "Follow up"} accepted for ${interactionMatch[2]}.`
            : `Interaction rejected: ${result.code ?? "unavailable"}`,
          result.status === "accepted" ? "info" : "warning",
        );
        return;
      }

      const taskMatch = /^task (T-[0-9a-f]{8})$/i.exec(args);
      if (taskMatch) {
        if (context?.mode !== "tui" || !context.openDelegatedTask) {
          notify("Delegated Task browsing requires TUI mode", "warning");
          return;
        }
        const displayId = taskMatch[1]!.toUpperCase();
        if (!await context.openDelegatedTask(displayId)) {
          notify(`Delegated Task ${displayId} was not found in this session.`, "warning");
        }
        return;
      }

      if (args === "delegations" || args === "delegations --json") {
        const delegation = context?.delegationSnapshot?.();
        if (!delegation) {
          notify("Delegation Registry is unavailable", "warning");
          return;
        }
        if (context?.mode === "tui" && args === "delegations" && context.openDelegations) {
          await context.openDelegations();
          return;
        }
        if (context?.mode === "json") {
          context.emitOutput?.(`${delegation.jsonEvent}\n`);
        } else if (context?.mode === "print") {
          context.emitOutput?.(`${delegation.human}\n`);
        } else {
          notify(args.endsWith("--json") ? delegation.json : delegation.human, "info");
        }
        return;
      }

      notify("Usage: /matty <status|doctor|delegations> [--json] | /matty task T-<id> | /matty <steer|follow-up> <exact-task-id> <message>", "warning");
    },
  });

  host.onSessionStart(async (event, notify, context) => {
    if (event.reason !== "startup" || startupMessageEmitted) {
      return;
    }

    startupMessageEmitted = true;
    const diagnostic = currentSnapshot(context);
    if (diagnostic.activation.state === "active") {
      notify(STARTUP_HINT, "info");
      return;
    }

    const reason = diagnostic.activation.codes.includes("host-uncertified")
      ? "unsupported Pi version or target · /matty status"
      : "/matty doctor";
    notify(`Matty degraded · ${reason}`, "warning");
  });

  return createStatusDiagnostic(snapshot);
}
