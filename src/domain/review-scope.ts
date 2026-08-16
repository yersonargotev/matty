import { commitSha, type CommitSha } from "./commit-sha.ts";

export type ReviewAxis = "standards" | "spec" | "security" | "correctness" | "maintainability";

export interface ReviewScopeContract {
  schemaVersion: 1;
  issue: {
    repository: string;
    number: number;
    reference: string;
  };
  requirements: string[];
  outOfScope: Array<{ reference: string; reason: string }>;
  baseSha: CommitSha;
  candidateSha: CommitSha;
  axes: ReviewAxis[];
}

export interface ReviewerFindings {
  schemaVersion: 1;
  candidateSha: CommitSha;
  summary: string;
  findings: Array<{
    axis: ReviewAxis;
    severity: "blocking" | "non-blocking";
    requirement: string;
    evidence: string;
  }>;
}

export const REVIEWER_VALIDATION_REASONS = [
  "invalid-json",
  "invalid-shape",
  "candidate-sha-mismatch",
  "axis-not-allowed",
  "requirement-not-in-scope",
  "excluded-reference",
  "validation-failed",
] as const;

export type ReviewerValidationReason =
  (typeof REVIEWER_VALIDATION_REASONS)[number];

export interface ReviewerValidationDiagnostic {
  schemaVersion: 1;
  reason: ReviewerValidationReason;
}

export type ReviewerFindingsValidation =
  | { ok: true; findings: ReviewerFindings }
  | { ok: false; diagnostic: ReviewerValidationDiagnostic };

export function reviewerValidationDiagnostic(
  reason: ReviewerValidationReason,
): ReviewerValidationDiagnostic {
  return { schemaVersion: 1, reason };
}

export function isReviewerValidationReason(
  value: unknown,
): value is ReviewerValidationReason {
  return typeof value === "string" &&
    REVIEWER_VALIDATION_REASONS.some((reason) => reason === value);
}

/** Rebuilds reviewer validation metadata from its closed versioned allowlist. */
export function safeReviewerValidationDiagnostic(
  value: unknown,
): ReviewerValidationDiagnostic {
  const candidate = typeof value === "object" && value !== null
    ? value as Partial<ReviewerValidationDiagnostic>
    : undefined;
  return reviewerValidationDiagnostic(
    candidate?.schemaVersion === 1 && isReviewerValidationReason(candidate.reason)
      ? candidate.reason
      : "validation-failed",
  );
}

const AXES = new Set<ReviewAxis>([
  "standards", "spec", "security", "correctness", "maintainability",
]);

function nonEmptyStrings(value: unknown): value is string[] {
  return Array.isArray(value) && value.length > 0 &&
    value.every((item) => typeof item === "string" && item.trim().length > 0);
}

function hasOnlyKeys(value: object, keys: readonly string[]): boolean {
  return Object.keys(value).every((key) => keys.includes(key));
}

/** Closes and validates review scope at the inspection-delegation seam. */
export function reviewScope(value: unknown): ReviewScopeContract {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("invalid review scope");
  }
  const input = value as Record<string, unknown>;
  const issue = input.issue as Record<string, unknown> | undefined;
  if (
    !hasOnlyKeys(input, ["schemaVersion", "issue", "requirements", "outOfScope", "baseSha", "candidateSha", "axes"]) ||
    input.schemaVersion !== 1 || !issue ||
    !hasOnlyKeys(issue, ["repository", "number", "reference"]) ||
    typeof issue.repository !== "string" || !issue.repository.trim() ||
    !Number.isSafeInteger(issue.number) || (issue.number as number) < 1 ||
    typeof issue.reference !== "string" || !issue.reference.trim() ||
    !nonEmptyStrings(input.requirements) ||
    !Array.isArray(input.outOfScope) || input.outOfScope.some((entry) => {
      if (typeof entry !== "object" || entry === null || Array.isArray(entry)) return true;
      const item = entry as Record<string, unknown>;
      return !hasOnlyKeys(item, ["reference", "reason"]) ||
        typeof item.reference !== "string" || !item.reference.trim() ||
        typeof item.reason !== "string" || !item.reason.trim();
    }) ||
    !nonEmptyStrings(input.axes) || input.axes.some((axis) => !AXES.has(axis as ReviewAxis))
  ) {
    throw new Error("invalid review scope");
  }
  return {
    schemaVersion: 1,
    issue: {
      repository: issue.repository as string,
      number: issue.number as number,
      reference: issue.reference as string,
    },
    requirements: [...input.requirements as string[]],
    outOfScope: (input.outOfScope as ReviewScopeContract["outOfScope"]).map((x) => ({ ...x })),
    baseSha: commitSha(input.baseSha),
    candidateSha: commitSha(input.candidateSha),
    axes: [...input.axes as ReviewAxis[]],
  };
}

function targetsExcludedReference(
  content: string,
  scope: ReviewScopeContract,
): boolean {
  return scope.outOfScope.some(({ reference }) => {
    if (content.includes(reference)) return true;
    const issueNumber = /(?:^#|\/issues\/)([1-9]\d*)\/?$/.exec(reference)?.[1];
    return issueNumber !== undefined && new RegExp(`(?:^|\\s)#${issueNumber}(?!\\d)`).test(content);
  });
}

export function reviewerFindings(
  value: unknown,
  scope: ReviewScopeContract,
): ReviewerFindingsValidation {
  const fail = (reason: ReviewerValidationReason): ReviewerFindingsValidation => ({
    ok: false,
    diagnostic: reviewerValidationDiagnostic(reason),
  });
  try {
    if (typeof value !== "object" || value === null || Array.isArray(value)) {
      return fail("invalid-shape");
    }
    const input = value as Record<string, unknown>;
    if (
      !hasOnlyKeys(input, ["schemaVersion", "candidateSha", "summary", "findings"]) ||
      input.schemaVersion !== 1 ||
      typeof input.summary !== "string" || !input.summary.trim() ||
      !Array.isArray(input.findings)
    ) {
      return fail("invalid-shape");
    }
    let candidate: CommitSha;
    try {
      candidate = commitSha(input.candidateSha);
    } catch {
      return fail("invalid-shape");
    }
    if (candidate !== scope.candidateSha) return fail("candidate-sha-mismatch");

    const findings: ReviewerFindings["findings"] = [];
    for (const finding of input.findings) {
      if (typeof finding !== "object" || finding === null || Array.isArray(finding)) {
        return fail("invalid-shape");
      }
      const item = finding as Record<string, unknown>;
      if (
        !hasOnlyKeys(item, ["axis", "severity", "requirement", "evidence"]) ||
        !AXES.has(item.axis as ReviewAxis) ||
        (item.severity !== "blocking" && item.severity !== "non-blocking") ||
        typeof item.requirement !== "string" ||
        typeof item.evidence !== "string" || !item.evidence.trim()
      ) {
        return fail("invalid-shape");
      }
      if (!scope.axes.includes(item.axis as ReviewAxis)) return fail("axis-not-allowed");
      if (!scope.requirements.includes(item.requirement)) {
        return fail("requirement-not-in-scope");
      }
      if (
        targetsExcludedReference(item.requirement, scope) ||
        targetsExcludedReference(item.evidence, scope)
      ) {
        return fail("excluded-reference");
      }
      findings.push({
        axis: item.axis as ReviewAxis,
        severity: item.severity,
        requirement: item.requirement,
        evidence: item.evidence,
      });
    }
    return {
      ok: true,
      findings: {
        schemaVersion: 1,
        candidateSha: candidate,
        summary: input.summary,
        findings,
      },
    };
  } catch {
    return fail("validation-failed");
  }
}
