import type {
  DelegatedTaskOutcome,
  DelegatedTaskProgress,
  DelegatedTaskRunner,
} from "./child-pi-runtime.ts";
import {
  EXPLORER_CAPABILITY_CONTRACT,
  createCapabilityPreflightDiagnostic,
  preflightCapability,
  type CapabilityAvailability,
  type CapabilityPreflightDiagnostic,
  type ExplorerCapabilityContract,
} from "../domain/capability-contract.ts";

export interface ExplorerDelegationExecution {
  availability: CapabilityAvailability;
  createRunner(): DelegatedTaskRunner;
}

export interface ExplorerDelegationOptions {
  signal?: AbortSignal;
  onProgress?: (progress: DelegatedTaskProgress) => void;
}

export interface BlockedExplorerOutcome {
  status: "blocked";
  diagnostic: CapabilityPreflightDiagnostic;
}

export interface ExplorerDelegationTerminal {
  contract: ExplorerCapabilityContract;
  outcome: DelegatedTaskOutcome | BlockedExplorerOutcome;
}

export function blockedExplorerDelegation(
  unmet: string[],
): ExplorerDelegationTerminal {
  return {
    contract: EXPLORER_CAPABILITY_CONTRACT,
    outcome: {
      status: "blocked",
      diagnostic: createCapabilityPreflightDiagnostic(
        EXPLORER_CAPABILITY_CONTRACT.id,
        unmet,
      ),
    },
  };
}

function explorerTask(task: string): string {
  return [
    "Explorer assignment:",
    task.trim(),
    "",
    "Inspect only. Use read, grep, find, ls, or guarded bash for local Git, CodeGraph, shell, and diagnostics. Do not mutate local or remote state. Return concise evidence to the parent.",
  ].join("\n");
}

export async function runExplorerDelegation(
  task: string,
  execution: ExplorerDelegationExecution,
  options: ExplorerDelegationOptions = {},
): Promise<ExplorerDelegationTerminal> {
  const preflight = preflightCapability(
    EXPLORER_CAPABILITY_CONTRACT,
    execution.availability,
  );
  if (!preflight.ok) {
    return blockedExplorerDelegation(preflight.diagnostic.unmet);
  }

  const runner = execution.createRunner();
  const outcome = await runner.run(explorerTask(task), options);
  return {
    contract: preflight.contract,
    outcome,
  };
}
