import type {
  DelegatedTaskOutcome,
  DelegatedTaskProgress,
  DelegatedTaskRunner,
} from "./child-pi-runtime.ts";
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
  acquireWriter?(): (() => void) | undefined;
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

export interface WorkerDelegationTerminal {
  contract: WorkerCapabilityContract;
  outcome: DelegatedTaskOutcome | BlockedWorkerOutcome;
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
    "Return a concise implementation summary; the parent reviews and integrates all changes.",
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
  let releaseWriter: (() => void) | undefined;
  if (execution.acquireWriter) {
    releaseWriter = execution.acquireWriter();
    if (!releaseWriter) {
      return blockedWorkerDelegation(execution.contract, [
        "Single Writer already active for this repository",
      ]);
    }
  } else {
    releaseWriter = () => {};
  }
  try {
    const runner = execution.createRunner();
    return {
      contract: preflight.contract,
      outcome: await runner.run(
        workerTask(preflight.contract, task),
        options,
      ),
    };
  } finally {
    releaseWriter();
  }
}
