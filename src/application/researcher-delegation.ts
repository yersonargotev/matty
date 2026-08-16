import {
  transferChildTranscript,
  type DelegatedTaskOutcome,
  type DelegatedTaskProgress,
  type DelegatedTaskRunner,
} from "./child-pi-runtime.ts";
import {
  createCapabilityPreflightDiagnostic,
  preflightCapability,
  type CapabilityAvailability,
  type CapabilityPreflightDiagnostic,
  type ResearcherCapabilityContract,
} from "../domain/capability-contract.ts";

export interface ResearcherDelegationExecution {
  contract: ResearcherCapabilityContract;
  availability: CapabilityAvailability;
  createRunner(): DelegatedTaskRunner;
  reportDelivered(): Promise<boolean>;
}

export interface ResearcherDelegationOptions {
  signal?: AbortSignal;
  onProgress?: (progress: DelegatedTaskProgress) => void;
}

export interface ResearcherDelegationTerminal {
  contract: ResearcherCapabilityContract;
  artifacts: {
    workspace: string;
    report: string;
  };
  outcome:
    | DelegatedTaskOutcome
    | {
        status: "failed";
        child: NonNullable<DelegatedTaskOutcome["child"]>;
        failure: {
          kind: "missing-research-report";
          message: string;
        };
        exit: {
          code: 0;
          signal: null;
        };
      }
    | {
        status: "blocked";
        diagnostic: CapabilityPreflightDiagnostic;
      };
}

function artifacts(contract: ResearcherCapabilityContract): {
  workspace: string;
  report: string;
} {
  return {
    workspace: contract.workspace,
    report: contract.report,
  };
}

export function blockedResearcherDelegation(
  contract: ResearcherCapabilityContract,
  unmet: string[],
): ResearcherDelegationTerminal {
  return {
    contract,
    artifacts: artifacts(contract),
    outcome: {
      status: "blocked",
      diagnostic: createCapabilityPreflightDiagnostic(contract.id, unmet),
    },
  };
}

function researcherTask(
  contract: ResearcherCapabilityContract,
  task: string,
): string {
  return [
    "Researcher assignment:",
    task.trim(),
    "",
    `Complete ${contract.web} web research using only the certified Web Capability tools.`,
    "Preserve useful working evidence with research_file in the validated workspace.",
    `Validated workspace: ${contract.workspace}`,
    `Approved Markdown report: ${contract.report}`,
    "Write exactly one durable Research Report with research_file destination report.",
    "Do not use shell, general write/edit tools, GitHub, or any other write destination.",
    "Return concise findings to the parent; artifact locations are returned separately.",
  ].join("\n");
}

export async function runResearcherDelegation(
  task: string,
  execution: ResearcherDelegationExecution,
  options: ResearcherDelegationOptions = {},
): Promise<ResearcherDelegationTerminal> {
  const preflight = preflightCapability(
    execution.contract,
    execution.availability,
  );
  if (!preflight.ok) {
    return blockedResearcherDelegation(
      execution.contract,
      preflight.diagnostic.unmet,
    );
  }
  const runner = execution.createRunner();
  const outcome = await runner.run(
    researcherTask(preflight.contract, task),
    options,
  );
  if (
    outcome.status === "succeeded" &&
    !(await execution.reportDelivered())
  ) {
    return {
      contract: preflight.contract,
      artifacts: artifacts(preflight.contract),
      outcome: transferChildTranscript(outcome, {
        status: "failed",
        child: outcome.child,
        failure: {
          kind: "missing-research-report",
          message: "researcher did not create the approved Research Report",
        },
        exit: outcome.exit,
      }),
    };
  }
  return {
    contract: preflight.contract,
    artifacts: artifacts(preflight.contract),
    outcome,
  };
}
