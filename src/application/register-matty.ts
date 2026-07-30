import {
  STARTUP_HINT,
} from "../domain/package-contract.ts";
import {
  createStatusSnapshot,
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

export interface MattyHost {
  registerCommand(
    name: string,
    command: {
      description: string;
      handle(args: string, notify: Notify): Promise<void>;
    },
  ): void;
  onSessionStart(
    handler: (
      event: { reason: string },
      notify: Notify,
    ) => Promise<void>,
  ): void;
}

export function registerMatty(
  host: MattyHost,
  runtimeFacts: RuntimeFacts,
): StatusDiagnostic {
  const snapshot = createStatusSnapshot(runtimeFacts);
  let startupMessageEmitted = false;

  host.registerCommand("matty", {
    description: "Show Matty status",
    handle: async (rawArgs, notify) => {
      const args = rawArgs.trim().replace(/\s+/g, " ");

      if (args === "status") {
        notify(renderStatusHuman(snapshot), "info");
        return;
      }

      if (args === "status --json") {
        notify(renderStatusJson(snapshot), "info");
        return;
      }

      notify("Usage: /matty status [--json]", "warning");
    },
  });

  host.onSessionStart(async (event, notify) => {
    if (event.reason !== "startup" || startupMessageEmitted) {
      return;
    }

    startupMessageEmitted = true;
    if (snapshot.activation.state === "active") {
      notify(STARTUP_HINT, "info");
      return;
    }

    notify(
      snapshot.activation.reason === "activation-safety-gate"
        ? "Matty degraded · Activation Safety Gate blocked · /matty status"
        : "Matty degraded · unsupported Pi version or target · /matty status",
      "warning",
    );
  });

  return snapshot;
}
