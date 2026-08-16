import {
  childTerminalResponses,
  transferChildTranscript,
  type DelegatedTaskOutcome,
  type DelegatedTaskProgress,
  type DelegatedTaskRunner,
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
  createRunner(lifecycle?: {
    beforeInteraction(
      signal?: AbortSignal,
      context?: { readonly candidateObserved: boolean },
    ): Promise<boolean>;
    onInteractionRejected(): void | Promise<void>;
    onTerminalResponse(text: string): void;
  }): DelegatedTaskRunner;
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
  let releaseWriter = await execution.acquireWriter();
  if (!releaseWriter) {
    return blockedWorkerDelegation(execution.contract, [
      "Single Writer already active for this repository",
    ]);
  }
  let releasePending: Promise<void> = Promise.resolve();
  let candidateSequence = 0;
  const candidateWaiters = new Set<() => void>();
  let interactionTail: Promise<void> = Promise.resolve();
  let activeInteraction: { complete(): void } | undefined;
  const releaseCandidateWriter = (): void => {
    const release = releaseWriter;
    releaseWriter = undefined;
    if (release) releasePending = releasePending.then(async () => { await release(); });
  };
  const completeActiveInteraction = (): void => {
    const active = activeInteraction;
    activeInteraction = undefined;
    active?.complete();
  };
  try {
    const runner = execution.createRunner({
      async beforeInteraction(signal = options.signal, context) {
        const predecessor = interactionTail;
        let complete!: () => void;
        let completed = false;
        interactionTail = new Promise<void>((resolve) => {
          complete = () => {
            if (completed) return;
            completed = true;
            resolve();
          };
        });
        const targetCandidate = candidateSequence + (context?.candidateObserved === false ? 1 : 0);
        await predecessor;
        if (signal?.aborted) {
          complete();
          return false;
        }
        if (candidateSequence < targetCandidate) {
          await new Promise<void>((resolveCandidate) => {
            const finish = () => {
              candidateWaiters.delete(finish);
              signal?.removeEventListener("abort", finish);
              resolveCandidate();
            };
            candidateWaiters.add(finish);
            signal?.addEventListener("abort", finish, { once: true });
          });
        }
        if (signal?.aborted) {
          complete();
          return false;
        }
        await releasePending;
        while (!releaseWriter && !signal?.aborted) {
          releaseWriter = await execution.acquireWriter();
          if (!releaseWriter) {
            await new Promise<void>((resolveWait) => {
              const finish = () => {
                clearTimeout(timer);
                signal?.removeEventListener("abort", finish);
                resolveWait();
              };
              const timer = setTimeout(finish, 10);
              signal?.addEventListener("abort", finish, { once: true });
            });
          }
        }
        if (!releaseWriter) {
          complete();
          return false;
        }
        activeInteraction = { complete };
        return true;
      },
      async onInteractionRejected() {
        releaseCandidateWriter();
        completeActiveInteraction();
        await releasePending;
      },
      onTerminalResponse(text) {
        try {
          workerCompletionReport(JSON.parse(text));
          candidateSequence += 1;
          releaseCandidateWriter();
          for (const resolveCandidate of [...candidateWaiters]) resolveCandidate();
          completeActiveInteraction();
        } catch {
          // Invalid responses remain transcript-only and do not alter authority.
        }
      },
    });
    const outcome = await runner.run(workerTask(preflight.contract, task), options);
    if (outcome.status !== "succeeded") {
      return { contract: preflight.contract, outcome };
    }
    let candidate: WorkerCompletionReport | undefined;
    let invalidAfterCandidate = false;
    for (const response of childTerminalResponses(outcome)) {
      try {
        candidate = workerCompletionReport(JSON.parse(response));
        invalidAfterCandidate = false;
      } catch {
        if (candidate) invalidAfterCandidate = true;
      }
    }
    if (candidate) {
      return {
        contract: preflight.contract,
        outcome: transferChildTranscript(outcome, {
          ...outcome,
          output: candidate,
          ...(invalidAfterCandidate
            ? { diagnostic: { kind: "candidate", code: "invalid-role-output" } }
            : {}),
        }),
      };
    }
    return {
      contract: preflight.contract,
      outcome: transferChildTranscript(outcome, {
        status: "failed",
        child: outcome.child,
        failure: {
          kind: "protocol-failed",
          message: "worker completed without a valid structured completion report",
        },
        exit: outcome.exit,
      }),
    };
  } finally {
    for (const resolveCandidate of [...candidateWaiters]) resolveCandidate();
    completeActiveInteraction();
    await interactionTail;
    await releasePending;
    await releaseWriter?.();
    releaseWriter = undefined;
  }
}
