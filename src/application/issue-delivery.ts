import {
  candidateCheck,
  ISSUE_DELIVERY_WORKFLOW,
  type CandidateCheck,
  type ExceptionBrief,
  type IssueDeliveryEvidenceCode,
  type DeliveryBlockerCode,
  type DeliveryIdentity,
  type DeliveryWorkspace,
  type IssueDeliveryOutcome,
} from "../domain/issue-delivery.ts";

export interface IssueDeliveryRequest {
  intent: "deliver" | "status";
  issue: string;
  cwd: string;
}

export interface ParsedIssueReference {
  number: number;
  repository?: string;
}

export interface IssueDeliveryPreflight {
  github: {
    available: boolean;
    authenticated: boolean;
  };
  repository: {
    trusted: boolean;
    prepared: boolean;
    tracker: "github" | "unsupported";
    canonical?: string;
    readyLabel?: string;
  };
  issueInspection?: "available" | "not-found" | "failed";
  issue?: {
    kind: "issue" | "pull-request";
    number: number;
    state: "open" | "closed";
    labels: string[];
    url: string;
  };
  skills: Array<{
    id: string;
    identity: string;
    provenance: string;
    digest: string;
  }>;
}

export type ReadIssueDeliveryPreflight = (
  issue: ParsedIssueReference,
  cwd: string,
) => Promise<IssueDeliveryPreflight>;

export interface IssueDeliveryWorkspaceRequest {
  cwd: string;
  identity: DeliveryIdentity;
}

export type IssueDeliveryWorkspaceResult =
  | { status: "prepared"; workspace: DeliveryWorkspace }
  | { status: "blocked"; exceptionBrief: ExceptionBrief };

export interface ActiveDeliveryInspection {
  identity: DeliveryIdentity;
  branch: string;
  integrationBranch: string;
  integrationSha: string;
  candidateSha: string | null;
}

export type ExistingIssueDeliveryResult =
  | { status: "absent" }
  | { status: "active"; delivery: ActiveDeliveryInspection }
  | { status: "blocked"; exceptionBrief: ExceptionBrief };

export interface IssueDeliveryWorkspace {
  inspect(
    request: { cwd: string; issue: number },
  ): Promise<ExistingIssueDeliveryResult>;
  prepare(
    request: IssueDeliveryWorkspaceRequest,
  ): Promise<IssueDeliveryWorkspaceResult>;
}

export interface IssueDeliveryInspection {
  issue: { state: "open" | "closed" };
  pullRequests: Array<{ headSha: string }>;
  remoteBranches: {
    deliverySha: string | null;
    integrationSha: string;
  };
  checks: CandidateCheck[];
}

export interface IssueDeliveryInspectionRequest extends ActiveDeliveryInspection {}

export type ReadIssueDeliveryInspection = (
  request: IssueDeliveryInspectionRequest,
) => Promise<IssueDeliveryInspection>;

function parseIssueReference(value: string): ParsedIssueReference | undefined {
  const short = /^(?:#)?([1-9]\d*)$/.exec(value);
  if (short) {
    return { number: Number(short[1]) };
  }
  const url = /^https:\/\/github\.com\/([^/]+\/[^/]+)\/issues\/([1-9]\d*)\/?$/.exec(
    value,
  );
  if (!url) {
    return undefined;
  }
  return {
    number: Number(url[2]),
    repository: `github.com/${url[1]!.toLowerCase()}`,
  };
}

function blocked(exceptionBrief: ExceptionBrief): IssueDeliveryOutcome {
  return { schemaVersion: 1, status: "blocked", exceptionBrief };
}

function invalidIssueReference(): IssueDeliveryOutcome {
  return blocked({
    schemaVersion: 1,
    gate: "delivery-authorization",
    evidence: ["issue-reference-invalid"],
    need: "One exact GitHub issue number or canonical issue URL is required.",
    options: ["Run /matty deliver <issue-number> with exactly one issue."],
    recommendation:
      "Choose the intended issue, then run /matty deliver <issue-number>.",
  });
}

function reconciliationBlocked(
  gate: "implementation" | "verification",
  evidence: "delivery-inspection-unavailable" | "delivery-pr-ambiguous" |
    "delivery-candidate-drift" | "delivery-not-active" |
    "delivery-ownership-mismatch",
): IssueDeliveryOutcome {
  const messages = {
    "delivery-inspection-unavailable": {
      need: "Required delivery facts could not be inspected safely.",
      option: "Restore read access to the owned Git and GitHub delivery facts.",
      recommendation: "Do not replay delivery effects while required facts are unavailable.",
    },
    "delivery-pr-ambiguous": {
      need: "More than one pull request matches the owned delivery branch.",
      option: "Resolve the duplicate owned-branch pull requests.",
      recommendation: "Preserve the candidate and reconcile the ambiguity before continuing.",
    },
    "delivery-candidate-drift": {
      need: "The pull request head disagrees with the owned local candidate.",
      option: "Reconcile the owned branch with its remote pull request head.",
      recommendation: "Do not infer which candidate is authoritative.",
    },
    "delivery-not-active": {
      need: "No active owned Issue Delivery matches this issue.",
      option: "Run /matty deliver for a ready issue before requesting delivery status.",
      recommendation: "Inspect only an explicitly owned Delivery Identity.",
    },
    "delivery-ownership-mismatch": {
      need: "The requested issue disagrees with the active Delivery Identity.",
      option: "Request status for the issue named by the active ownership marker.",
      recommendation: "Do not infer ownership from branch names.",
    },
  } as const;
  const message = messages[evidence];
  return blocked({
    schemaVersion: 1,
    gate,
    evidence: [evidence],
    need: message.need,
    options: [message.option],
    recommendation: message.recommendation,
  });
}

function isCommitSha(value: unknown): value is string {
  return typeof value === "string" && /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(value);
}

function activeReport(
  delivery: ActiveDeliveryInspection,
  facts: IssueDeliveryInspection,
): IssueDeliveryOutcome {
  if (
    typeof delivery.integrationBranch !== "string" ||
    delivery.integrationBranch.length === 0 ||
    !isCommitSha(delivery.integrationSha) ||
    (delivery.candidateSha !== null && !isCommitSha(delivery.candidateSha)) ||
    (facts.issue.state !== "open" && facts.issue.state !== "closed") ||
    !Array.isArray(facts.pullRequests) ||
    facts.pullRequests.some((pullRequest) => !isCommitSha(pullRequest.headSha)) ||
    typeof facts.remoteBranches !== "object" || facts.remoteBranches === null ||
    !(facts.remoteBranches.deliverySha === null ||
      isCommitSha(facts.remoteBranches.deliverySha)) ||
    !isCommitSha(facts.remoteBranches.integrationSha) ||
    !Array.isArray(facts.checks)
  ) {
    throw new Error("malformed delivery inspection");
  }
  for (const check of facts.checks) {
    candidateCheck(check.status, check.conclusion);
  }
  const gate = delivery.candidateSha === null ? "implementation" : "verification";
  if (facts.pullRequests.length > 1) {
    return reconciliationBlocked(gate, "delivery-pr-ambiguous");
  }
  const ownedCandidate = delivery.candidateSha ?? delivery.integrationSha;
  if (
    (facts.pullRequests.length === 1 &&
      facts.pullRequests[0]!.headSha !== ownedCandidate) ||
    (facts.remoteBranches.deliverySha !== null &&
      facts.remoteBranches.deliverySha !== ownedCandidate)
  ) {
    return reconciliationBlocked(gate, "delivery-candidate-drift");
  }

  const passed = facts.checks.filter((check) =>
    check.status === "completed" &&
    (check.conclusion === "success" || check.conclusion === "neutral" ||
      check.conclusion === "skipped")
  ).length;
  const pending = facts.checks.filter((check) => check.status !== "completed").length;
  const failed = facts.checks.length - passed - pending;
  const blockers: DeliveryBlockerCode[] = [];
  if (gate === "implementation") blockers.push("implementation-required");
  if (facts.issue.state === "closed") blockers.push("issue-closed");
  if (facts.remoteBranches.integrationSha !== delivery.integrationSha) {
    blockers.push("integration-advanced");
  }
  if (failed > 0) blockers.push("checks-failing");
  if (pending > 0) blockers.push("checks-pending");
  const state = failed > 0 ? "failing" : pending > 0 ? "pending" :
    facts.checks.length === 0 ? "none" : "passing";
  return {
    schemaVersion: 1,
    status: "active",
    deliveryIdentity: delivery.identity,
    gate,
    candidateSha: delivery.candidateSha,
    checks: { state, total: facts.checks.length, passed, pending, failed },
    blockers,
  };
}

export async function deliverIssue(
  request: IssueDeliveryRequest,
  readPreflight: ReadIssueDeliveryPreflight,
  workspace: IssueDeliveryWorkspace,
  readInspection?: ReadIssueDeliveryInspection,
): Promise<IssueDeliveryOutcome> {
  const issueReference = parseIssueReference(request.issue);
  if (!issueReference) return invalidIssueReference();

  {
    let existing: ExistingIssueDeliveryResult;
    try {
      existing = await workspace.inspect({ cwd: request.cwd, issue: issueReference.number });
    } catch {
      return reconciliationBlocked("implementation", "delivery-inspection-unavailable");
    }
    if (existing.status === "blocked") {
      return blocked(existing.exceptionBrief);
    }
    if (existing.status === "active") {
      const gate = existing.delivery.candidateSha === null ? "implementation" : "verification";
      if (
        existing.delivery.identity.issue !== issueReference.number ||
        (issueReference.repository !== undefined &&
          existing.delivery.identity.repository !== issueReference.repository)
      ) {
        return reconciliationBlocked(gate, "delivery-ownership-mismatch");
      }
      if (!readInspection) {
        return reconciliationBlocked(gate, "delivery-inspection-unavailable");
      }
      try {
        return activeReport(existing.delivery, await readInspection(existing.delivery));
      } catch {
        return reconciliationBlocked(gate, "delivery-inspection-unavailable");
      }
    }
    if (request.intent === "status") {
      return reconciliationBlocked("implementation", "delivery-not-active");
    }
  }

  const qualification = await qualifyIssueDelivery(request, readPreflight);
  if (qualification.status !== "qualified") return qualification;
  let preparation: IssueDeliveryWorkspaceResult;
  try {
    preparation = await workspace.prepare({ cwd: request.cwd, identity: qualification.deliveryIdentity });
  } catch {
    return blocked({
      schemaVersion: 1,
      gate: "workspace-preparation",
      evidence: ["workspace-preparation-failed"],
      need: "The qualified delivery workspace could not be prepared safely.",
      options: [`Inspect the Git checkout, then repeat /matty deliver ${qualification.deliveryIdentity.issue}.`],
      recommendation: "Resolve the workspace preparation failure without changing unrelated work.",
    });
  }
  if (preparation.status === "blocked") {
    return { schemaVersion: 1, status: "blocked", exceptionBrief: preparation.exceptionBrief };
  }
  return { ...qualification, status: "prepared", workspace: preparation.workspace };
}

export async function qualifyIssueDelivery(
  request: IssueDeliveryRequest,
  readPreflight: ReadIssueDeliveryPreflight,
): Promise<IssueDeliveryOutcome> {
  const issueReference = parseIssueReference(request.issue);
  if (request.intent !== "deliver" || !issueReference) {
    return invalidIssueReference();
  }

  let preflight: IssueDeliveryPreflight;
  try {
    preflight = await readPreflight(issueReference, request.cwd);
  } catch {
    return blocked({
      schemaVersion: 1,
      gate: "capability-preflight",
      evidence: ["qualification-inspection-failed"],
      need:
        "Issue Delivery qualification facts could not be inspected safely; no delivery effects were produced.",
      options: [
        `Run /matty deliver ${issueReference.number} again after checking local Git and GitHub availability.`,
      ],
      recommendation:
        "Resolve the local inspection failure before authorizing Issue Delivery.",
    });
  }

  const evidence: IssueDeliveryEvidenceCode[] = [];
  const options: string[] = [];
  const command = `/matty deliver ${issueReference.number}`;
  const add = (code: IssueDeliveryEvidenceCode, remediation: string) => {
    evidence.push(code);
    options.push(remediation);
  };

  if (!preflight.github.available) {
    add(
      "github-capability-missing",
      `Install the gh CLI, then repeat ${command}.`,
    );
  } else if (!preflight.github.authenticated) {
    add(
      "github-authentication-missing",
      `Run gh auth login, then repeat ${command}.`,
    );
  }
  if (!preflight.repository.prepared) {
    add(
      "prepared-repository-missing",
      `Run /skill:setup-matt-pocock-skills in this repository, review its changes, then repeat ${command}.`,
    );
  } else if (!preflight.repository.trusted) {
    add(
      "prepared-repository-untrusted",
      `Review and restore the repository-owned preparation policy, then repeat ${command}.`,
    );
  }
  if (preflight.repository.tracker !== "github") {
    add(
      "tracker-unsupported",
      "Use Issue Delivery only in a Prepared Repository configured for GitHub.",
    );
  }
  if (preflight.issueInspection === "failed") {
    add(
      "github-capability-missing",
      `Verify GitHub connectivity and issue access, then repeat ${command}.`,
    );
  } else if (
    preflight.github.available &&
    preflight.github.authenticated &&
    preflight.repository.tracker === "github" &&
    (!preflight.issue || preflight.issue.kind !== "issue" ||
      preflight.issue.number !== issueReference.number)
  ) {
    add("issue-not-found", `Confirm issue #${issueReference.number} exists in the current repository, then repeat ${command}.`);
  } else if (preflight.issue) {
    if (preflight.issue.state !== "open") {
      add("issue-not-open", `Reopen issue #${issueReference.number} or choose an open ready issue.`);
    }
    const readyLabel = preflight.repository.readyLabel;
    if (!readyLabel || !preflight.issue.labels.includes(readyLabel)) {
      add(
        "issue-not-ready",
        readyLabel
          ? `Apply the repository's ${readyLabel} triage label to issue #${issueReference.number}, then repeat ${command}.`
          : `Restore the repository's canonical ready-for-agent triage mapping, then repeat ${command}.`,
      );
    }
  }
  if (
    issueReference.repository &&
    preflight.repository.canonical !== issueReference.repository
  ) {
    add(
      "issue-not-found",
      `Run ${command} from the repository named by the issue URL, or use an issue from the current repository.`,
    );
  }

  for (const dependency of ISSUE_DELIVERY_WORKFLOW.dependencies) {
    const actual = preflight.skills.find((skill) => skill.id === dependency.id);
    if (!actual) {
      add(
        `workflow-dependency-missing:${dependency.id}`,
        `Install the Packy-provisioned certified ${dependency.id} skill, then repeat ${command}.`,
      );
    } else if (actual.identity !== dependency.id) {
      add(
        `workflow-dependency-identity-mismatch:${dependency.id}`,
        `Restore the Packy-provisioned certified ${dependency.id} skill, then repeat ${command}.`,
      );
    } else if (actual.provenance !== dependency.provenance) {
      add(
        `workflow-dependency-provenance-mismatch:${dependency.id}`,
        `Restore the Packy-provisioned certified ${dependency.id} skill, then repeat ${command}.`,
      );
    } else if (actual.digest !== dependency.digest) {
      add(
        `workflow-dependency-content-digest-mismatch:${dependency.id}`,
        `Restore the Packy-provisioned certified ${dependency.id} skill, then repeat ${command}.`,
      );
    }
  }

  if (evidence.length > 0) {
    return blocked({
      schemaVersion: 1,
      gate: evidence.some((code) =>
          code.startsWith("workflow-dependency-") ||
          code.startsWith("github-")
        )
        ? "capability-preflight"
        : "delivery-authorization",
      evidence,
      need:
        "Issue Delivery only is blocked until every listed authorization and capability requirement is satisfied.",
      options,
      recommendation: options[0] ?? `Repeat ${command} after remediation.`,
    });
  }

  const canonical = preflight.repository.canonical;
  if (!canonical) {
    return blocked({
      schemaVersion: 1,
      gate: "delivery-authorization",
      evidence: ["prepared-repository-untrusted"],
      need: "The Prepared Repository must have one canonical GitHub identity.",
      options: [`Restore one canonical GitHub remote, then repeat ${command}.`],
      recommendation: "Restore the repository identity before delivery.",
    });
  }

  return {
    schemaVersion: 1,
    status: "qualified",
    workflow: {
      id: ISSUE_DELIVERY_WORKFLOW.id,
      definitionVersion: ISSUE_DELIVERY_WORKFLOW.definitionVersion,
      guidanceVersion: ISSUE_DELIVERY_WORKFLOW.guidanceVersion,
    },
    deliveryIdentity: {
      repository: canonical,
      tracker: "github",
      issue: issueReference.number,
    },
    evidence: [
      "delivery-intent-explicit",
      "github-authenticated",
      "prepared-repository-trusted",
      "issue-ready",
      "workflow-dependencies-certified",
    ],
  };
}
