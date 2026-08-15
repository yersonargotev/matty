export interface WorkerCompletionReport {
  schemaVersion: 1;
  summary: string;
  changedPaths: string[];
  checks: Array<{
    command: string;
    status: "passed" | "failed" | "not-run";
  }>;
  evidenceRole: "supporting-only-parent-verification-required";
  reportedFullGate: {
    status: "passed" | "failed" | "not-run";
    command?: string;
  };
}

export function workerCompletionReport(value: unknown): WorkerCompletionReport {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("invalid worker completion report");
  }
  const report = value as Record<string, unknown>;
  const gate = report.reportedFullGate as Record<string, unknown> | undefined;
  const statuses = new Set(["passed", "failed", "not-run"]);
  const hasOnlyKeys = (candidate: object, keys: string[]) =>
    Object.keys(candidate).every((key) => keys.includes(key));
  if (
    !hasOnlyKeys(report, ["schemaVersion", "summary", "changedPaths", "checks", "evidenceRole", "reportedFullGate"]) ||
    report.schemaVersion !== 1 ||
    report.evidenceRole !== "supporting-only-parent-verification-required" ||
    typeof report.summary !== "string" || !report.summary.trim() ||
    !Array.isArray(report.changedPaths) ||
    report.changedPaths.some((path) => typeof path !== "string" || !path.trim()) ||
    !Array.isArray(report.checks) || report.checks.some((check) => {
      if (typeof check !== "object" || check === null || Array.isArray(check)) return true;
      const item = check as Record<string, unknown>;
      return !hasOnlyKeys(item, ["command", "status"]) ||
        typeof item.command !== "string" || !item.command.trim() || !statuses.has(item.status as string);
    }) || !gate || !hasOnlyKeys(gate, ["status", "command"]) || !statuses.has(gate.status as string) ||
    !(gate.command === undefined || (typeof gate.command === "string" && gate.command.trim()))
  ) {
    throw new Error("invalid worker completion report");
  }
  return {
    schemaVersion: 1,
    summary: report.summary as string,
    changedPaths: [...report.changedPaths as string[]],
    checks: (report.checks as WorkerCompletionReport["checks"]).map((check) => ({ ...check })),
    evidenceRole: "supporting-only-parent-verification-required",
    reportedFullGate: { ...gate } as WorkerCompletionReport["reportedFullGate"],
  };
}
