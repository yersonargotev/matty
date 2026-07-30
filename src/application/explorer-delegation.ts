import {
  blockedInspectionDelegation,
  runInspectionDelegation,
  type BlockedInspectionOutcome,
  type InspectionDelegationExecution,
  type InspectionDelegationOptions,
  type InspectionDelegationTerminal,
} from "./inspection-role-delegation.ts";
import type { ExplorerCapabilityContract } from "../domain/capability-contract.ts";

export type ExplorerDelegationExecution = InspectionDelegationExecution;
export type ExplorerDelegationOptions = InspectionDelegationOptions;
export type BlockedExplorerOutcome = BlockedInspectionOutcome;
export type ExplorerDelegationTerminal = Omit<
  InspectionDelegationTerminal,
  "contract"
> & {
  contract: ExplorerCapabilityContract;
};

export function blockedExplorerDelegation(
  unmet: string[],
): ExplorerDelegationTerminal {
  return blockedInspectionDelegation(
    "explorer",
    unmet,
  ) as ExplorerDelegationTerminal;
}

export async function runExplorerDelegation(
  task: string,
  execution: ExplorerDelegationExecution,
  options: ExplorerDelegationOptions = {},
): Promise<ExplorerDelegationTerminal> {
  return await runInspectionDelegation(
    "explorer",
    task,
    execution,
    options,
  ) as ExplorerDelegationTerminal;
}
