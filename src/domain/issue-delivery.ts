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
  | `workflow-dependency-missing:${string}`
  | `workflow-dependency-identity-mismatch:${string}`
  | `workflow-dependency-provenance-mismatch:${string}`
  | `workflow-dependency-content-digest-mismatch:${string}`;

export interface ExceptionBrief {
  schemaVersion: 1;
  gate: "delivery-authorization" | "capability-preflight";
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
  deliveryIdentity: {
    repository: string;
    tracker: "github";
    issue: number;
  };
  evidence: IssueDeliveryEvidenceCode[];
}

export interface BlockedIssueDelivery {
  schemaVersion: 1;
  status: "blocked";
  exceptionBrief: ExceptionBrief;
}

export type IssueDeliveryOutcome =
  | QualifiedIssueDelivery
  | BlockedIssueDelivery;
