import {
  RESEARCHER_TOOLS,
  WORKER_TOOLS,
} from "./capability-contract.ts";

export const CHILD_EXECUTION_ACTIVITY_SCHEMA_VERSION = 1 as const;

const knownToolNames = new Set<string>([
  ...WORKER_TOOLS,
  ...RESEARCHER_TOOLS,
]);
const safeToolCategories = new Set<string>([...knownToolNames, "other"]);

export type ChildExecutionToolCategory =
  | (typeof WORKER_TOOLS)[number]
  | (typeof RESEARCHER_TOOLS)[number]
  | "other";

/**
 * The only Child Execution activity data permitted to cross into parent-facing
 * progress and session state. Additions require a schema version change.
 */
export type ChildExecutionActivitySummary =
  | {
      schemaVersion: typeof CHILD_EXECUTION_ACTIVITY_SCHEMA_VERSION;
      kind: "assistant-completed";
    }
  | {
      schemaVersion: typeof CHILD_EXECUTION_ACTIVITY_SCHEMA_VERSION;
      kind: "tool-completed";
      tool: ChildExecutionToolCategory;
      outcome: "succeeded" | "failed";
    };

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
  if (candidate.kind === "assistant-completed") {
    return { schemaVersion: 1, kind: "assistant-completed" };
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

/** Classify a raw Pi event while intentionally copying no raw event fields. */
export function classifyChildExecutionActivity(
  value: unknown,
): ChildExecutionActivityClassification {
  const event = record(value);
  if (!event) return { recognized: false };

  if (event.type === "message_end") {
    const message = record(event.message);
    if (message?.role !== "assistant") return { recognized: false };
    return {
      recognized: true,
      valid: true,
      summary: { schemaVersion: 1, kind: "assistant-completed" },
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
