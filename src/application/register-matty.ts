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

      notify("Usage: /matty <status|doctor> [--json]", "warning");
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
