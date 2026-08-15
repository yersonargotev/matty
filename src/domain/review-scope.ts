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
): ReviewerFindings {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("invalid reviewer findings");
  }
  const input = value as Record<string, unknown>;
  const candidate = commitSha(input.candidateSha);
  if (
    !hasOnlyKeys(input, ["schemaVersion", "candidateSha", "summary", "findings"]) ||
    input.schemaVersion !== 1 || candidate !== scope.candidateSha ||
    typeof input.summary !== "string" || !input.summary.trim() ||
    !Array.isArray(input.findings) || input.findings.some((finding) => {
      if (typeof finding !== "object" || finding === null || Array.isArray(finding)) return true;
      const item = finding as Record<string, unknown>;
      return !hasOnlyKeys(item, ["axis", "severity", "requirement", "evidence"]) ||
        !AXES.has(item.axis as ReviewAxis) || !scope.axes.includes(item.axis as ReviewAxis) ||
        (item.severity !== "blocking" && item.severity !== "non-blocking") ||
        typeof item.requirement !== "string" || !scope.requirements.includes(item.requirement) ||
        typeof item.evidence !== "string" || !item.evidence.trim() ||
        targetsExcludedReference(item.requirement as string, scope) ||
        targetsExcludedReference(item.evidence as string, scope);
    })
  ) {
    throw new Error("invalid reviewer findings");
  }
  return {
    schemaVersion: 1,
    candidateSha: candidate,
    summary: input.summary as string,
    findings: (input.findings as ReviewerFindings["findings"]).map((x) => ({ ...x })),
  };
}
