export const CHILD_EXECUTION_ACTIVITY_SCHEMA_VERSION = 1 as const;
export const CHILD_EXECUTION_ACTIVITY_OBSERVATION_SCHEMA_VERSION = 1 as const;

/**
 * Schema-v1 categories are a protocol allowlist. They must not inherit changes
 * from mutable role tool surfaces without an explicit activity schema change.
 */
export const CHILD_EXECUTION_TOOL_CATEGORIES_V1 = Object.freeze([
  "read",
  "write",
  "edit",
  "grep",
  "find",
  "ls",
  "bash",
  "web_search",
  "source_check",
  "fetch_content",
  "get_search_content",
  "research_file",
  "other",
] as const);

const safeToolCategories = new Set<string>(CHILD_EXECUTION_TOOL_CATEGORIES_V1);
const knownToolNames = new Set<string>(
  CHILD_EXECUTION_TOOL_CATEGORIES_V1.filter((tool) => tool !== "other"),
);
const assistantStopReasons = new Set([
  "pending", "stop", "length", "toolUse", "error", "aborted", "deferred",
]);

export type ChildExecutionToolCategory =
  (typeof CHILD_EXECUTION_TOOL_CATEGORIES_V1)[number];

/** The only event-derived activity detail allowed into parent state. */
export type ChildExecutionActivitySummary =
  | {
      schemaVersion: typeof CHILD_EXECUTION_ACTIVITY_SCHEMA_VERSION;
      kind: "assistant-completed";
      outcome: "succeeded" | "failed";
    }
  | {
      schemaVersion: typeof CHILD_EXECUTION_ACTIVITY_SCHEMA_VERSION;
      kind: "tool-completed";
      tool: ChildExecutionToolCategory;
      outcome: "succeeded" | "failed";
    };

/** Closed, versioned ordering envelope attached to one task by the registry. */
export interface ChildExecutionActivityObservation {
  schemaVersion: typeof CHILD_EXECUTION_ACTIVITY_OBSERVATION_SCHEMA_VERSION;
  sequence: number;
  observedAt: number;
  summary: ChildExecutionActivitySummary;
}

export type ChildExecutionActivityClassification =
  | { recognized: false }
  | { recognized: true; valid: false }
  | {
      recognized: true;
      valid: true;
      summary: ChildExecutionActivitySummary;
    };

function record(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined;
}

function validToolName(value: unknown): value is string {
  return typeof value === "string" &&
    /^[A-Za-z][A-Za-z0-9_-]{0,127}$/.test(value);
}

export function safeChildExecutionActivitySummary(
  value: unknown,
): ChildExecutionActivitySummary | undefined {
  const candidate = record(value);
  if (candidate?.schemaVersion !== 1) return undefined;
  if (
    candidate.kind === "assistant-completed" &&
    (candidate.outcome === "succeeded" || candidate.outcome === "failed")
  ) {
    return { schemaVersion: 1, kind: "assistant-completed", outcome: candidate.outcome };
  }
  if (
    candidate.kind === "tool-completed" &&
    typeof candidate.tool === "string" &&
    safeToolCategories.has(candidate.tool) &&
    (candidate.outcome === "succeeded" || candidate.outcome === "failed")
  ) {
    return {
      schemaVersion: 1,
      kind: "tool-completed",
      tool: candidate.tool as ChildExecutionToolCategory,
      outcome: candidate.outcome,
    };
  }
  return undefined;
}

export function safeChildExecutionActivityObservation(
  value: unknown,
): ChildExecutionActivityObservation | undefined {
  const candidate = record(value);
  const summary = safeChildExecutionActivitySummary(candidate?.summary);
  if (
    candidate?.schemaVersion !== 1 ||
    !Number.isSafeInteger(candidate.sequence) ||
    (candidate.sequence as number) < 1 ||
    typeof candidate.observedAt !== "number" ||
    !Number.isSafeInteger(candidate.observedAt) ||
    candidate.observedAt < 0 ||
    candidate.observedAt > 8_640_000_000_000_000 ||
    !summary
  ) return undefined;
  return {
    schemaVersion: 1,
    sequence: candidate.sequence as number,
    observedAt: candidate.observedAt,
    summary,
  };
}

/** Classify a raw Pi event while intentionally copying no raw event fields. */
export function classifyChildExecutionActivity(
  value: unknown,
): ChildExecutionActivityClassification {
  const event = record(value);
  if (!event) return { recognized: false };

  if (event.type === "message_end") {
    const message = record(event.message);
    if (message?.role !== "assistant") return { recognized: false };
    if (
      typeof message.stopReason !== "string" ||
      !assistantStopReasons.has(message.stopReason)
    ) return { recognized: true, valid: false };
    const outcome = message.stopReason === "error" || message.stopReason === "aborted"
      ? "failed" as const
      : "succeeded" as const;
    return {
      recognized: true,
      valid: true,
      summary: { schemaVersion: 1, kind: "assistant-completed", outcome },
    };
  }

  if (event.type !== "tool_execution_end") return { recognized: false };
  if (
    typeof event.toolCallId !== "string" || event.toolCallId.length === 0 ||
    !validToolName(event.toolName) ||
    typeof event.isError !== "boolean" ||
    !("result" in event)
  ) {
    return { recognized: true, valid: false };
  }

  return {
    recognized: true,
    valid: true,
    summary: {
      schemaVersion: 1,
      kind: "tool-completed",
      tool: knownToolNames.has(event.toolName)
        ? event.toolName as ChildExecutionToolCategory
        : "other",
      outcome: event.isError ? "failed" : "succeeded",
    },
  };
}
