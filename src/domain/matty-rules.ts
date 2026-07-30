import {
  DELEGATION_INPUT_GUIDANCE,
  MATTY_ROLES,
  type MattyRole,
} from "./capability-contract.ts";

export const MATTY_RULES_START = "<!-- matty:rules -->";
export const MATTY_RULES_END = "<!-- /matty:rules -->";

export type MattyPromptRole = "parent" | MattyRole;

function renderMattyRules(role: MattyPromptRole): string {
  const exposedRoles = MATTY_ROLES.join(", ").replace(
    /, ([^,]+)$/,
    ", and $1",
  );
  const activeRole =
    role === "parent"
      ? "Active role: parent; delegate bounded work through Capability Contracts."
      : role === "worker"
      ? "Active child role: worker; implement within validated paths and return changes to the parent."
      : role === "researcher"
      ? "Active child role: researcher; use certified web tools and write only bounded research artifacts."
      : `Active child role: ${role}; inspect only and return findings to the parent.`;

  return [
    MATTY_RULES_START,
    "Matty Rules v1",
    activeRole,
    `- Matty role names are explorer, reviewer, designer, researcher, and worker; the currently exposed path exposes ${exposedRoles}.`,
    `- subagent accepts exactly ${DELEGATION_INPUT_GUIDANCE}; every task runs through an independent role.`,
    "- Explorer, designer, and reviewer receive read, grep, find, ls, and guarded bash.",
    "- Reviewer may use read-only gh after availability and authentication preflight; explorer and designer may not use gh.",
    "- Researcher receives only the four certified Web Capability tools and research_file; it has no bash, write, or edit authority.",
    "- Worker receives read, write, edit, grep, find, ls, and bash; writes are limited to the trusted working tree and validated temporary paths.",
    "- Single Writer permits at most one active worker per repository; parallel-writer contracts fail preflight.",
    "- One call accepts one to eight tasks and runs at most four children; excess accepted work is queued.",
    "- Required groups are atomic: failure cancels remaining work and never falls back to inline parent work.",
    "- Optional fallback is limited to inspection groups and reports skipped work explicitly.",
    "- Model knowledge is never reported as completed web research; required web failure blocks, and optional web failure is disclosed.",
    "- The Inspection Guard is a best-effort command policy, not a security sandbox.",
    "- The Worker Guard is a best-effort command and path policy, not a security sandbox.",
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
  if (
    /\bworkers?\s+(?:may|can|must|should)\s+(?:commit|push|merge|use\s+gh|write\s+(?:outside|to\s+user))\b/i.test(
      projectInstructions,
    )
  ) {
    return "project instructions grant worker integration authority";
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
