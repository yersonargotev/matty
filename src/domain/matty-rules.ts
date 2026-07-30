import {
  INSPECTION_ROLES,
  INSPECTION_ROLE_INPUT_GUIDANCE,
  type InspectionRole,
} from "./capability-contract.ts";

export const MATTY_RULES_START = "<!-- matty:rules -->";
export const MATTY_RULES_END = "<!-- /matty:rules -->";

export type MattyPromptRole = "parent" | InspectionRole;

function renderMattyRules(role: MattyPromptRole): string {
  const exposedRoles = INSPECTION_ROLES.join(", ").replace(
    /, ([^,]+)$/,
    ", and $1",
  );
  const activeRole =
    role === "parent"
      ? "Active role: parent; delegate bounded work through Capability Contracts."
      : `Active child role: ${role}; inspect only and return findings to the parent.`;

  return [
    MATTY_RULES_START,
    "Matty Rules v1",
    activeRole,
    `- Matty role names are explorer, reviewer, designer, researcher, and worker; the currently exposed path exposes ${exposedRoles}.`,
    `- subagent accepts exactly ${INSPECTION_ROLE_INPUT_GUIDANCE}; this path runs one independent inspection role with read, grep, find, ls, and guarded bash.`,
    "- Reviewer may use read-only gh after availability and authentication preflight; explorer and designer may not use gh.",
    "- This path accepts one task and runs one child; required capability failure is explicit and never falls back to inline parent work.",
    "- The Inspection Guard is a best-effort command policy, not a security sandbox.",
    "- The parent owns commits, pushes, pull requests, reviews, merges, releases, and other external-state mutation.",
    MATTY_RULES_END,
  ].join("\n");
}

function withoutMarkedRules(systemPrompt: string): string {
  return systemPrompt
    .replace(
      /<!-- matty:rules -->[\s\S]*?<!-- \/matty:rules -->/g,
      "",
    )
    .replaceAll(MATTY_RULES_START, "")
    .replaceAll(MATTY_RULES_END, "");
}

export function detectMattyRulesConflict(
  systemPrompt: string,
): string | undefined {
  const projectInstructions = withoutMarkedRules(systemPrompt);
  if (
    /\b(?:ignore|override|relax|disable)\s+(?:the\s+)?Matty Rules\b/i.test(
      projectInstructions,
    )
  ) {
    return "project instructions attempt to disable Matty Rules";
  }
  if (
    /\bexplorers?\s+(?:may|can|must|should)\s+(?:write|edit|modify|mutate)\b/i.test(
      projectInstructions,
    )
  ) {
    return "project instructions grant explorer write authority";
  }
  if (
    /\b(?:designers?|reviewers?)\s+(?:may|can|must|should)\s+(?:write|edit|modify|mutate)\b/i.test(
      projectInstructions,
    )
  ) {
    return "project instructions grant inspection-role mutation authority";
  }
  return undefined;
}

export function injectMattyRules(
  systemPrompt: string,
  role: MattyPromptRole,
): string {
  const withoutStrayMarkers = withoutMarkedRules(systemPrompt).trim();

  return [withoutStrayMarkers, renderMattyRules(role)]
    .filter(Boolean)
    .join("\n\n");
}
