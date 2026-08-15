import {
  ISSUE_DELIVERY_WORKFLOW,
  type ExceptionBrief,
  type IssueDeliveryEvidenceCode,
  type DeliveryIdentity,
  type DeliveryWorkspace,
  type IssueDeliveryOutcome,
} from "../domain/issue-delivery.ts";

export interface IssueDeliveryRequest {
  intent: "deliver";
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

export interface IssueDeliveryWorkspace {
  prepare(
    request: IssueDeliveryWorkspaceRequest,
  ): Promise<IssueDeliveryWorkspaceResult>;
}

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

export async function deliverIssue(
  request: IssueDeliveryRequest,
  readPreflight: ReadIssueDeliveryPreflight,
  workspace: IssueDeliveryWorkspace,
): Promise<IssueDeliveryOutcome> {
  const qualification = await qualifyIssueDelivery(request, readPreflight);
  if (qualification.status !== "qualified") {
    return qualification;
  }
  let preparation: IssueDeliveryWorkspaceResult;
  try {
    preparation = await workspace.prepare({
      cwd: request.cwd,
      identity: qualification.deliveryIdentity,
    });
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
  return {
    ...qualification,
    status: "prepared",
    workspace: preparation.workspace,
  };
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
