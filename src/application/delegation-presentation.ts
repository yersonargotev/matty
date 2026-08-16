import type {
  DelegationSnapshot,
  DelegationSnapshotEntry,
} from "./delegation-registry.ts";

export interface DelegationPresentationState {
  selectedId?: string;
  expandedIds: ReadonlySet<string>;
}

function formatDuration(start: number, end: number): string {
  const seconds = Math.max(0, Math.floor((end - start) / 1_000));
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m${String(seconds % 60).padStart(2, "0")}s`;
}

function duration(entry: DelegationSnapshotEntry, now: number): string {
  return formatDuration(entry.startedAt ?? entry.acceptedAt, entry.endedAt ?? now);
}

function roles(entry: DelegationSnapshotEntry): string {
  return [...new Set(entry.roles)].join(",") || "unknown";
}

export function delegationCard(
  entry: DelegationSnapshotEntry,
  now = Date.now(),
): string {
  return `${entry.displayId} ${entry.state} · ${roles(entry)} · ${entry.taskCount} task${entry.taskCount === 1 ? "" : "s"} · ${duration(entry, now)}`;
}

export function renderDelegationConsole(
  snapshot: DelegationSnapshot,
  state: DelegationPresentationState,
  now = Date.now(),
): string[] {
  const lines = [
    "Matty Delegations (session only)",
    "↑/↓ select · Enter details · c cancel · Esc/q close",
  ];
  if (snapshot.delegations.length === 0) {
    lines.push("No delegations in this session.");
    return lines;
  }
  let previousGroup = "";
  for (const entry of snapshot.delegations) {
    const group = entry.state === "running" || entry.state === "cancelling"
      ? "Active / Cancelling"
      : entry.state === "queued" ? "Queued" : "Recent";
    if (group !== previousGroup) {
      lines.push(`${group}:`);
      previousGroup = group;
    }
    const selected = state.selectedId === entry.id ? ">" : " ";
    lines.push(`${selected} ${delegationCard(entry, now)}`);
    if (state.expandedIds.has(entry.id)) {
      lines.push(`  Delegation ID: ${entry.id}`);
      lines.push(`  Accepted: ${new Date(entry.acceptedAt).toISOString()}`);
      if (entry.startedAt !== undefined) lines.push(`  Started: ${new Date(entry.startedAt).toISOString()}`);
      if (entry.endedAt !== undefined) lines.push(`  Ended: ${new Date(entry.endedAt).toISOString()}`);
      if (entry.resultSummary) lines.push(`  Result: ${entry.resultSummary}`);
      lines.push("  Delegated Tasks:");
      for (const task of entry.tasks) {
        const identity = [
          task.pid !== undefined ? `PID ${task.pid}` : undefined,
          task.runId ? `runId ${task.runId}` : undefined,
        ].filter(Boolean).join(" · ");
        lines.push(`    ${task.index + 1}. ${task.role ?? "unknown"} · ${task.state}${identity ? ` · ${identity}` : ""}`);
        if (task.queuePosition !== undefined) lines.push(`      Queue position: ${task.queuePosition}`);
        lines.push(`      Queued: ${new Date(task.queuedAt).toISOString()}`);
        if (task.startedAt !== undefined) lines.push(`      Started: ${new Date(task.startedAt).toISOString()}`);
        if (task.endedAt !== undefined) lines.push(`      Ended: ${new Date(task.endedAt).toISOString()}`);
        const durationStart = task.startedAt ?? task.queuedAt;
        lines.push(`      ${task.endedAt === undefined ? "Elapsed" : "Duration"}: ${formatDuration(durationStart, task.endedAt ?? now)}`);
        if (task.resultSummary) lines.push(`      Result: ${task.resultSummary}`);
      }
    }
  }
  return lines;
}

export function renderDelegationHumanSnapshot(
  snapshot: DelegationSnapshot,
  now = Date.now(),
): string {
  if (snapshot.delegations.length === 0) return "Matty delegations: none (session only)";
  return [
    "Matty delegations (session only)",
    ...snapshot.delegations.map((entry) => delegationCard(entry, now)),
  ].join("\n");
}

export function renderDelegationJson(snapshot: DelegationSnapshot): string {
  return JSON.stringify(snapshot);
}
