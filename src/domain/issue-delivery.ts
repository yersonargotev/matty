export const ISSUE_DELIVERY_WORKFLOW = {
  id: "issue-delivery",
  definitionVersion: 1,
  guidanceVersion: 1,
  dependencies: [
    {
      id: "implement",
      provenance: "packy/engineering/implement",
      digest: "6d3fd9e83b8f36e5213854779db49b256a457a7ebb4a503e53fa7dcff696adc3",
    },
    {
      id: "tdd",
      provenance: "packy/engineering/tdd",
      digest: "5e6b9c16b547113e90afbb946489d1c1384be5c2128f0159bd0bee57251ecf08",
    },
    {
      id: "code-review",
      provenance: "packy/engineering/code-review",
      digest: "7b2611d766ed7b9f375e73c821c7727535a6c036cf66870882770cd5a8188f70",
    },
  ],
} as const;

export type IssueDeliveryEvidenceCode =
  | "delivery-intent-explicit"
  | "github-authenticated"
  | "prepared-repository-trusted"
  | "issue-ready"
  | "workflow-dependencies-certified"
  | "issue-reference-invalid"
  | "github-capability-missing"
  | "github-authentication-missing"
  | "prepared-repository-missing"
  | "prepared-repository-untrusted"
  | "tracker-unsupported"
  | "issue-not-found"
  | "issue-not-open"
  | "issue-not-ready"
  | "qualification-inspection-failed"
  | "delivery-active"
  | "delivery-ownership-mismatch"
  | "workspace-preparation-failed"
  | "delivery-not-active"
  | "delivery-inspection-unavailable"
  | "delivery-pr-ambiguous"
  | "delivery-candidate-drift"
  | "delivery-candidate-ancestry-unproven"
  | `workflow-dependency-missing:${string}`
  | `workflow-dependency-identity-mismatch:${string}`
  | `workflow-dependency-provenance-mismatch:${string}`
  | `workflow-dependency-content-digest-mismatch:${string}`;

export interface ExceptionBrief {
  schemaVersion: 1;
  gate:
    | "delivery-authorization"
    | "capability-preflight"
    | "workspace-preparation"
    | "implementation"
    | "verification";
  evidence: IssueDeliveryEvidenceCode[];
  need: string;
  options: string[];
  recommendation: string;
}

export interface QualifiedIssueDelivery {
  schemaVersion: 1;
  status: "qualified";
  workflow: {
    id: "issue-delivery";
    definitionVersion: 1;
    guidanceVersion: 1;
  };
  deliveryIdentity: DeliveryIdentity;
  evidence: IssueDeliveryEvidenceCode[];
}

export interface DeliveryIdentity {
  repository: string;
  tracker: "github";
  issue: number;
}

export interface DeliveryWorkspace {
  root: string;
  path: string;
  branch: string;
  isolation: "in-place" | "worktree";
  resumed: boolean;
  startingCheckout: {
    root: string;
    ref: string | null;
    sha: string;
  };
}

export interface PreparedIssueDelivery {
  schemaVersion: 1;
  status: "prepared";
  workflow: QualifiedIssueDelivery["workflow"];
  deliveryIdentity: DeliveryIdentity;
  evidence: IssueDeliveryEvidenceCode[];
  workspace: DeliveryWorkspace;
}

export type DeliveryGate = "implementation" | "verification";

export type DeliveryBlockerCode =
  | "implementation-required"
  | "issue-closed"
  | "integration-advanced"
  | "checks-pending"
  | "checks-failing";

export interface CandidateCheck {
  status: "queued" | "in_progress" | "completed";
  conclusion: "success" | "neutral" | "skipped" | "failure" |
    "cancelled" | "timed_out" | "action_required" | "stale" |
    "startup_failure" | null;
}

/** Constructs one closed candidate check while discarding provider-owned fields. */
export function candidateCheck(
  status: unknown,
  conclusion: unknown,
): CandidateCheck {
  const statuses = new Set(["queued", "in_progress", "completed"]);
  const conclusions = new Set([
    "success", "neutral", "skipped", "failure", "cancelled", "timed_out",
    "action_required", "stale", "startup_failure",
  ]);
  if (
    typeof status !== "string" || !statuses.has(status) ||
    !(conclusion === null ||
      (typeof conclusion === "string" && conclusions.has(conclusion))) ||
    (status === "completed") === (conclusion === null)
  ) {
    throw new Error("invalid candidate check");
  }
  return {
    status: status as CandidateCheck["status"],
    conclusion: conclusion as CandidateCheck["conclusion"],
  };
}

export interface ActiveIssueDelivery {
  schemaVersion: 1;
  status: "active";
  deliveryIdentity: DeliveryIdentity;
  gate: DeliveryGate;
  candidateSha: string | null;
  checks: {
    state: "none" | "passing" | "pending" | "failing";
    total: number;
    passed: number;
    pending: number;
    failed: number;
  };
  blockers: DeliveryBlockerCode[];
}

export interface BlockedIssueDelivery {
  schemaVersion: 1;
  status: "blocked";
  exceptionBrief: ExceptionBrief;
}

export type IssueDeliveryOutcome =
  | QualifiedIssueDelivery
  | PreparedIssueDelivery
  | ActiveIssueDelivery
  | BlockedIssueDelivery;
