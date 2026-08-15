import { abbreviatedCommitSha, type CommitSha } from "./commit-sha.ts";

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
  | "issue-blocked-by-dependency"
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

export interface IssueScopeSnapshot {
  schemaVersion: 1;
  reference: string;
  title: string;
  body: string;
  requirements: string[];
  dependencies: Array<{
    reference: string;
    title: string;
    state: "open" | "closed";
  }>;
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
  scope: IssueScopeSnapshot;
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
    sha: CommitSha;
  };
}

export interface PreparedIssueDelivery {
  schemaVersion: 1;
  status: "prepared";
  workflow: QualifiedIssueDelivery["workflow"];
  deliveryIdentity: DeliveryIdentity;
  scope: IssueScopeSnapshot;
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
  status: "queued" | "in_progress" | "waiting" | "requested" | "pending" |
    "completed";
  conclusion: "success" | "neutral" | "skipped" | "failure" |
    "cancelled" | "timed_out" | "action_required" | "stale" |
    "startup_failure" | null;
}

/** Constructs one closed candidate check while discarding provider-owned fields. */
export function candidateCheck(
  status: unknown,
  conclusion: unknown,
): CandidateCheck {
  const statuses = new Set([
    "queued", "in_progress", "waiting", "requested", "pending", "completed",
  ]);
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

export interface RepairCycleReason {
  cycle: number;
  reason: string;
  findings: string[];
}

export interface ExceptionalRepairAuthorization {
  decision: "authorized" | "denied";
  rationale: string;
  actor: string;
  evidence: string;
  scope: string;
}

export interface RepairBudget {
  schemaVersion: 1;
  limit: 3;
  used: number;
  remaining: number;
  cycles: RepairCycleReason[];
  exceptionalAuthorization?: ExceptionalRepairAuthorization;
}

export function repairBudget(value: unknown): RepairBudget {
  if (typeof value !== "object" || value === null || Array.isArray(value)) throw new Error("invalid repair budget");
  const item = value as Partial<RepairBudget>;
  const auth = item.exceptionalAuthorization;
  const hasOnlyKeys = (candidate: object, keys: string[]) =>
    Object.keys(candidate).every((key) => keys.includes(key));
  if (!hasOnlyKeys(item, ["schemaVersion", "limit", "used", "remaining", "cycles", "exceptionalAuthorization"]) ||
    item.schemaVersion !== 1 || item.limit !== 3 ||
    !Number.isSafeInteger(item.used) || (item.used ?? -1) < 0 ||
    item.remaining !== 3 - (item.used ?? 0) || (item.remaining ?? -1) < 0 ||
    !Array.isArray(item.cycles) || item.cycles.length !== item.used ||
    item.cycles.some((cycle, index) =>
      typeof cycle !== "object" || cycle === null || Array.isArray(cycle) ||
      !hasOnlyKeys(cycle, ["cycle", "reason", "findings"]) ||
      cycle.cycle !== index + 1 || typeof cycle.reason !== "string" || !cycle.reason.trim() ||
      !Array.isArray(cycle.findings) || cycle.findings.length === 0 ||
      cycle.findings.some((finding) => typeof finding !== "string" || !finding.trim())
    ) ||
    (auth !== undefined && (
      typeof auth !== "object" || auth === null || Array.isArray(auth) ||
      !hasOnlyKeys(auth, ["decision", "rationale", "actor", "evidence", "scope"]) ||
      (auth.decision !== "authorized" && auth.decision !== "denied") ||
      typeof auth.rationale !== "string" || !auth.rationale.trim() ||
      typeof auth.actor !== "string" || !auth.actor.trim() ||
      typeof auth.evidence !== "string" || !auth.evidence.trim() ||
      typeof auth.scope !== "string" || !auth.scope.trim()
    ))) {
    throw new Error("invalid repair budget");
  }
  return {
    schemaVersion: 1,
    limit: 3,
    used: item.used,
    remaining: item.remaining,
    cycles: item.cycles.map((cycle) => ({ ...cycle, findings: [...cycle.findings] })),
    ...(auth ? { exceptionalAuthorization: { ...auth } } : {}),
  } as RepairBudget;
}

export function initialRepairBudget(): RepairBudget {
  return repairBudget({ schemaVersion: 1, limit: 3, used: 0, remaining: 3, cycles: [] });
}

export function consumeRepairCycle(
  budget: RepairBudget,
  adjudication: Pick<RepairCycleReason, "reason" | "findings">,
): RepairBudget {
  const current = repairBudget(budget);
  if (current.remaining === 0) throw new Error("repair budget exhausted");
  return repairBudget({
    ...current,
    used: current.used + 1,
    remaining: current.remaining - 1,
    cycles: [...current.cycles, {
      cycle: current.used + 1,
      reason: adjudication.reason,
      findings: [...adjudication.findings],
    }],
  });
}

export function recordExceptionalRepairAuthorization(
  budget: RepairBudget,
  authorization: ExceptionalRepairAuthorization,
): RepairBudget {
  return repairBudget({
    ...repairBudget(budget),
    exceptionalAuthorization: { ...authorization },
  });
}

export interface ActiveIssueDelivery {
  schemaVersion: 1;
  status: "active";
  deliveryIdentity: DeliveryIdentity;
  gate: DeliveryGate;
  candidateSha: CommitSha | null;
  candidateState: "none" | "local-unpublished" | "published";
  repairBudget: RepairBudget;
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

function safePresentation(value: string): string {
  return value.replace(/[\u0000-\u001f\u007f]/g, (character) =>
    `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`
  );
}

export function renderIssueDeliveryOutcome(outcome: IssueDeliveryOutcome): string {
  if (outcome.status === "blocked") {
    const brief = outcome.exceptionBrief;
    return [
      `Issue Delivery blocked at ${brief.gate}`,
      `Need: ${brief.need}`,
      `Evidence: ${brief.evidence.join(", ")}`,
      `Options: ${brief.options.join(" | ")}`,
      `Recommendation: ${brief.recommendation}`,
    ].join("\n");
  }
  const identity = `${outcome.deliveryIdentity.repository}#${outcome.deliveryIdentity.issue}`;
  if (outcome.status === "qualified") return `Issue Delivery qualified: ${identity}\nScope: ${outcome.scope.title}`;
  if (outcome.status === "prepared") {
    return `Issue Delivery prepared: ${identity}\nWorkspace: ${outcome.workspace.isolation} · ${outcome.workspace.branch}`;
  }
  const candidate = outcome.candidateSha === null
    ? "none"
    : `${outcome.candidateState} (${abbreviatedCommitSha(outcome.candidateSha)})`;
  return [
    `Issue Delivery active: ${identity}`,
    `Gate: ${outcome.gate}`,
    `Candidate: ${candidate}`,
    `Checks: ${outcome.checks.state} (${outcome.checks.passed} passed, ${outcome.checks.pending} pending, ${outcome.checks.failed} failed)`,
    `Blockers: ${outcome.blockers.length ? outcome.blockers.join(", ") : "none"}`,
    `Repair Budget: ${outcome.repairBudget.used}/${outcome.repairBudget.limit} used · ${outcome.repairBudget.remaining} remaining`,
    ...outcome.repairBudget.cycles.flatMap((cycle) => [
      `Repair cycle ${cycle.cycle}: ${safePresentation(cycle.reason)}`,
      ...cycle.findings.map((finding) => `  Finding: ${safePresentation(finding)}`),
    ]),
    ...(outcome.repairBudget.exceptionalAuthorization
      ? [
        `Exceptional authorization decision: ${outcome.repairBudget.exceptionalAuthorization.decision}`,
        `Exceptional authorization rationale: ${safePresentation(outcome.repairBudget.exceptionalAuthorization.rationale)}`,
        `Exceptional authorization actor: ${safePresentation(outcome.repairBudget.exceptionalAuthorization.actor)}`,
        `Exceptional authorization evidence: ${safePresentation(outcome.repairBudget.exceptionalAuthorization.evidence)}`,
        `Exceptional authorization scope: ${safePresentation(outcome.repairBudget.exceptionalAuthorization.scope)}`,
      ]
      : []),
  ].join("\n");
}
