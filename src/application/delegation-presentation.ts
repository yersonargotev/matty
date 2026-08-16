import type {
  DelegatedTaskSnapshot,
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

export function delegatedTaskLifecycleTimeline(
  task: DelegatedTaskSnapshot,
): string[] {
  const lines = [`${new Date(task.queuedAt).toISOString()} · queued`];
  if (task.startedAt !== undefined) {
    lines.push(`${new Date(task.startedAt).toISOString()} · started`);
  }
  if (task.endedAt !== undefined) {
    lines.push(`${new Date(task.endedAt).toISOString()} · ${task.state}`);
  } else if (task.state !== "queued") {
    lines.push(`Current · ${task.state}`);
  }
  return lines;
}

function activityText(
  activity: DelegatedTaskSnapshot["activitySummaries"][number],
): string {
  return activity.kind === "assistant-completed"
    ? "Assistant completed"
    : `Tool ${activity.tool} completed · ${activity.outcome}`;
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
        lines.push("      Lifecycle:");
        lines.push(...delegatedTaskLifecycleTimeline(task).map((event) => `        ${event}`));
        lines.push("      Activity:");
        if (task.activitySummaries.length === 0) {
          lines.push("        No activity reported.");
        } else {
          lines.push(...task.activitySummaries.map((activity, index) =>
            `        ${index + 1}. ${activityText(activity)}`
          ));
        }
        if (task.queuePosition !== undefined) lines.push(`      Queue position: ${task.queuePosition}`);
        const durationStart = task.startedAt ?? task.queuedAt;
        lines.push(`      ${task.endedAt === undefined ? "Elapsed" : "Duration"}: ${formatDuration(durationStart, task.endedAt ?? now)}`);
        if (task.resultSummary) lines.push(`      Result: ${task.resultSummary}`);
      }
    }
  }
  return lines;
}

export function renderDelegationWidget(
  snapshot: DelegationSnapshot,
  now = Date.now(),
  maxLines = 4,
): string[] {
  const useful = snapshot.delegations.filter((entry) =>
    entry.state === "queued" || entry.state === "running" || entry.state === "cancelling"
  );
  if (useful.length === 0 || maxLines <= 0) return [];
  const activeDelegations = useful.filter((entry) =>
    entry.state === "running" || entry.state === "cancelling"
  ).length;
  const plural = (count: number, singular: string) =>
    `${count} ${singular}${count === 1 ? "" : "s"}`;
  const lines = [
    `Matty · ${plural(activeDelegations, "active Delegation")} · ${plural(snapshot.concurrency.queuedTasks, "queued task")}`,
  ];
  const availableCards = Math.max(0, maxLines - 1);
  const visibleCount = useful.length > availableCards
    ? Math.max(0, availableCards - 1)
    : availableCards;
  for (const entry of useful.slice(0, visibleCount)) {
    const stateCounts = new Map<string, number>();
    for (const task of entry.tasks) {
      stateCounts.set(task.state, (stateCounts.get(task.state) ?? 0) + 1);
    }
    const states = [...stateCounts]
      .map(([state, count]) => `${state} ${count}`)
      .join(", ");
    lines.push(
      `${entry.displayId} ${entry.state} · ${roles(entry)} · ${states} · ${duration(entry, now)}`,
    );
  }
  const hidden = useful.slice(visibleCount);
  if (hidden.length > 0 && lines.length < maxLines) {
    const allQueued = hidden.every((entry) => entry.state === "queued");
    lines.push(`+${hidden.length} more${allQueued ? " queued" : ""} Delegation${hidden.length === 1 ? "" : "s"}`);
  }
  return lines.slice(0, maxLines);
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
