import type {
  DelegatedTaskOutcome,
  DelegatedTaskProgress,
  DelegatedTaskRunner,
} from "./child-pi-runtime.ts";
import { workerCompletionReport, type WorkerCompletionReport } from "../domain/worker-completion.ts";
import {
  createCapabilityPreflightDiagnostic,
  preflightCapability,
  type CapabilityAvailability,
  type CapabilityPreflightDiagnostic,
  type WorkerCapabilityContract,
} from "../domain/capability-contract.ts";

export interface WorkerDelegationExecution {
  contract: WorkerCapabilityContract;
  availability: CapabilityAvailability;
  acquireWriter():
    | (() => void | Promise<void>)
    | undefined
    | Promise<(() => void | Promise<void>) | undefined>;
  createRunner(): DelegatedTaskRunner;
}

export interface WorkerDelegationOptions {
  signal?: AbortSignal;
  onProgress?: (progress: DelegatedTaskProgress) => void;
}

export interface BlockedWorkerOutcome {
  status: "blocked";
  diagnostic: CapabilityPreflightDiagnostic;
}

type SuccessfulWorkerOutcome =
  Extract<DelegatedTaskOutcome, { status: "succeeded" }> extends infer Outcome
    ? Omit<Outcome, "output"> & { output: WorkerCompletionReport }
    : never;

type WorkerDelegationOutcome = Exclude<DelegatedTaskOutcome, { status: "succeeded" }> |
  SuccessfulWorkerOutcome | BlockedWorkerOutcome;

export interface WorkerDelegationTerminal {
  contract: WorkerCapabilityContract;
  outcome: WorkerDelegationOutcome;
}

export function blockedWorkerDelegation(
  contract: WorkerCapabilityContract,
  unmet: string[],
): WorkerDelegationTerminal {
  return {
    contract,
    outcome: {
      status: "blocked",
      diagnostic: createCapabilityPreflightDiagnostic(contract.id, unmet),
    },
  };
}

function workerTask(
  contract: WorkerCapabilityContract,
  task: string,
): string {
  return [
    "Worker assignment:",
    task.trim(),
    "",
    `Write only within the trusted working tree: ${contract.workingTree}`,
    `Write only within validated temporary paths: ${contract.temporaryPaths.join(", ")}`,
    "You may read, edit, create, install project-local dependencies, and run project checks.",
    "Do not use gh, mutate the Git index or references, install globally, write outside validated paths, or write real user configuration.",
    "The Worker Guard is a best-effort command and path policy, not a security sandbox.",
    'Return JSON exactly {"schemaVersion":1,"summary":string,"changedPaths":string[],"checks":[{"command":string,"status":"passed"|"failed"|"not-run"}],"evidenceRole":"supporting-only-parent-verification-required","reportedFullGate":{"status":"passed"|"failed"|"not-run","command"?:string}}.',
    "Checks are supporting evidence, not verification. The parent reviews and integrates all changes only after inspecting the diff and independently running this repository's authoritative full gate.",
  ].join("\n");
}

export async function runWorkerDelegation(
  task: string,
  execution: WorkerDelegationExecution,
  options: WorkerDelegationOptions = {},
): Promise<WorkerDelegationTerminal> {
  const preflight = preflightCapability(
    execution.contract,
    execution.availability,
  );
  if (!preflight.ok) {
    return blockedWorkerDelegation(
      execution.contract,
      preflight.diagnostic.unmet,
    );
  }
  const releaseWriter = await execution.acquireWriter();
  if (!releaseWriter) {
    return blockedWorkerDelegation(execution.contract, [
      "Single Writer already active for this repository",
    ]);
  }
  try {
    const runner = execution.createRunner();
    const outcome = await runner.run(workerTask(preflight.contract, task), options);
    if (outcome.status !== "succeeded") {
      return { contract: preflight.contract, outcome };
    }
    try {
      return {
        contract: preflight.contract,
        outcome: { ...outcome, output: workerCompletionReport(JSON.parse(outcome.output)) },
      };
    } catch {
      return {
        contract: preflight.contract,
        outcome: {
          status: "failed",
          child: outcome.child,
          failure: {
            kind: "protocol-failed",
            message: "worker completed without a valid structured completion report",
          },
          exit: outcome.exit,
        },
      };
    }
  } finally {
    await releaseWriter();
  }
}
