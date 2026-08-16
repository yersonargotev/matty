import type { ExtensionContext } from "@earendil-works/pi-coding-agent";
import * as piTui from "@earendil-works/pi-tui";

import type { MattyApplicationControl } from "../application/delegation-control.ts";
import {
  DelegationRegistry,
  type DelegatedTaskId,
  type DelegatedTaskSnapshot,
  type DelegationCancellationResult,
  type DelegationId,
  type DelegationSnapshotEntry,
} from "../application/delegation-registry.ts";
import {
  delegationCard,
  renderDelegationWidget,
} from "../application/delegation-presentation.ts";
import type {
  DelegatedTaskPresentation,
  DelegatedTranscriptPresentationEntry,
} from "../application/child-pi-runtime.ts";

const WIDGET_ID = "matty-delegations";
type SessionReason = "new" | "resume" | "reload" | string;
type ConsoleView = "delegations" | "tasks" | "session";
type TranscriptFilter = "all" | DelegatedTranscriptPresentationEntry["category"];

export interface PiDelegationManagement {
  openConsole(context: ExtensionContext): Promise<void>;
  openTask(context: ExtensionContext, displayId: string): Promise<boolean>;
  startSession(reason: SessionReason, context: ExtensionContext): void;
  shutdown(): void;
}

function taskById(entry: DelegationSnapshotEntry | undefined, id: DelegatedTaskId | undefined) {
  return entry?.tasks.find((task) => task.id === id);
}

function compositionText(data: string): string | undefined {
  const text = data.replace(/^\u001b\[200~/, "").replace(/\u001b\[201~$/, "");
  return text.length > 0 && !/[\u0000-\u0008\u000b-\u001f\u007f]/u.test(text)
    ? text
    : undefined;
}

function taskMetadata(task: DelegatedTaskSnapshot, presentation: DelegatedTaskPresentation | undefined, now: number): string[] {
  const started = task.startedAt ?? task.queuedAt;
  const ended = task.endedAt ?? now;
  return [
    `Task state: ${task.state} · Role: ${task.role ?? "unknown"}`,
    `PID: ${task.pid ?? "unavailable"} · Run ID: ${task.runId ?? "unavailable"}`,
    `Usage: input ${presentation?.usage.inputTokens ?? 0} · output ${presentation?.usage.outputTokens ?? 0} tokens · Cost: $${(presentation?.usage.cost ?? 0).toFixed(4)} · Context consumption: ${presentation?.usage.totalTokens ?? 0} tokens`,
    `Timing: ${Math.max(0, Math.floor((ended - started) / 1_000))}s · Queued at: ${new Date(task.queuedAt).toISOString()}`,
  ];
}

export function createPiDelegationManagement(
  registry: DelegationRegistry,
  control: MattyApplicationControl,
): PiDelegationManagement {
  const activeConsoleClosers = new Set<() => void>();
  let widgetContext: ExtensionContext | undefined;
  let unsubscribeWidget: (() => void) | undefined;

  const clearWidget = () => {
    unsubscribeWidget?.();
    unsubscribeWidget = undefined;
    widgetContext?.ui.setWidget(WIDGET_ID, undefined);
    widgetContext = undefined;
  };

  const bindWidget = (context: ExtensionContext) => {
    if (context.mode !== "tui") return;
    widgetContext = context;
    const update = () => {
      const lines = renderDelegationWidget(registry.snapshot(), registry.now(), 4);
      context.ui.setWidget(WIDGET_ID, lines.length > 0 ? () => ({
        render(width: number) {
          return lines.map((line) => piTui.truncateToWidth(line, Math.max(1, width)));
        },
        invalidate() {},
      }) : undefined);
    };
    unsubscribeWidget = registry.subscribe(update);
    update();
  };

  const open = async (
    context: ExtensionContext,
    initial?: { delegationId: DelegationId; taskId: DelegatedTaskId },
  ): Promise<void> => {
    await context.ui.custom<void>((tui, _theme, _keybindings, done) => {
      let snapshot = registry.snapshot();
      let view: ConsoleView = initial ? "session" : "delegations";
      let selectedDelegationId = initial?.delegationId ?? snapshot.delegations[0]?.id;
      let selectedTaskId = initial?.taskId;
      let presentation = selectedTaskId ? control.taskPresentation(selectedTaskId) : undefined;
      let detachPresentation: (() => void) | undefined;
      let releaseSession: (() => void) | undefined;
      let confirmation: { id: DelegationId; displayId: string; active: number; queued: number } | undefined;
      let taskAbortConfirmation: { id: DelegatedTaskId; requirement: "required" | "optional" } | undefined;
      let composition: { type: "steer" | "follow_up" | "input"; text: string } | undefined;
      let interactionStatus: string | undefined;
      let cancellationStatus: string | undefined;
      let filter: TranscriptFilter = "all";
      let query = "";
      let enteringSearch = false;
      let selectedEntryId: string | undefined;
      let selectedDialogOption = 0;
      const collapsed = new Set<string>();
      const initializedEntries = new Set<string>();
      let scroll = 0;
      let closed = false;

      const currentDelegation = () => snapshot.delegations.find((entry) => entry.id === selectedDelegationId);
      const bindPresentation = () => {
        detachPresentation?.();
        detachPresentation = undefined;
        presentation = selectedTaskId ? control.taskPresentation(selectedTaskId) : undefined;
        if (!selectedTaskId) return;
        detachPresentation = control.subscribeTaskPresentation(selectedTaskId, (next) => {
          presentation = next;
          for (const entry of next.entries) {
            if (initializedEntries.has(entry.id)) continue;
            initializedEntries.add(entry.id);
            if (!entry.expandedByDefault) collapsed.add(entry.id);
          }
          tui.requestRender();
        });
      };
      bindPresentation();
      if (view === "session" && selectedTaskId) releaseSession = control.retainTaskSession(selectedTaskId);

      const cancellationMessages = {
        cancelling: (displayId: string) => `Cancellation requested for ${displayId}.`,
        "already-cancelling": (displayId: string) => `Cancellation is already in progress for ${displayId}.`,
        "already-finished": (displayId: string) => `${displayId} is already finished.`,
      } satisfies Record<DelegationCancellationResult, (displayId: string) => string>;
      const reportCancellation = (id: DelegationId, displayId: string) => {
        cancellationStatus = cancellationMessages[registry.cancel(id)](displayId);
        tui.requestRender();
      };
      const unsubscribe = registry.subscribe(() => {
        snapshot = registry.snapshot();
        if (selectedDelegationId && !snapshot.delegations.some((entry) => entry.id === selectedDelegationId)) {
          selectedDelegationId = snapshot.delegations[0]?.id;
          selectedTaskId = undefined;
          view = "delegations";
          bindPresentation();
        }
        tui.requestRender();
      });
      const close = () => {
        if (closed) return;
        closed = true;
        unsubscribe();
        detachPresentation?.();
        releaseSession?.();
        releaseSession = undefined;
        activeConsoleClosers.delete(close);
        done();
      };
      activeConsoleClosers.add(close);

      const move = <T extends { id: string }>(items: readonly T[], selected: string | undefined, delta: number): string | undefined => {
        if (items.length === 0) return undefined;
        const index = Math.max(0, items.findIndex((item) => item.id === selected));
        return items[Math.max(0, Math.min(items.length - 1, index + delta))]?.id;
      };

      return {
        render(width: number) {
          const lines: string[] = [];
          const delegation = currentDelegation();
          if (view === "delegations") {
            lines.push("Delegation Console · Delegations", "↑/↓ select · Enter Delegated Tasks · c cancel · Esc/q close");
            if (snapshot.delegations.length === 0) lines.push("No delegations in this session.");
            for (const entry of snapshot.delegations) {
              lines.push(`${entry.id === selectedDelegationId ? ">" : " "} ${delegationCard(entry, registry.now())}`);
            }
          } else if (view === "tasks") {
            lines.push("Delegation Console · Delegations → Delegated Tasks", "↑/↓ select · Enter Child Session · Esc/q close");
            if (delegation) {
              lines.push(`Delegation ID: ${delegation.id}`);
              for (const task of delegation.tasks) {
                lines.push(`${task.id === selectedTaskId ? ">" : " "} ${task.displayId} · State: ${task.state} · Role: ${task.role ?? "unknown"}`);
              }
            }
          } else {
            const task = taskById(delegation, selectedTaskId);
            lines.push("Delegation Console · Delegations → Delegated Tasks → Child Session");
            lines.push("↑/↓ scroll/select · Enter collapse · / search · f filter · s Steer · u Follow up · a abort · Esc/q close");
            if (task) {
              lines.push(`Delegated Task ID: ${task.id}`, ...taskMetadata(task, presentation, registry.now()));
              lines.push(`Child Session: ${presentation?.sessionState ?? (task.state === "succeeded" ? "settled" : "working")}`);
              if (presentation?.pendingInput) {
                const request = presentation.pendingInput;
                lines.push(
                  `Waiting for ${request.method} · ${request.title}`,
                  `${request.message ?? request.placeholder ?? "Response required"}`,
                );
                if (request.method === "select") {
                  for (const [index, option] of (request.options ?? []).entries()) {
                    lines.push(`${index === selectedDialogOption ? ">" : " "} ${option}`);
                  }
                  lines.push(`↑/↓ select · Enter respond · x cancel request${request.extendable ? " · e extend 5 minutes" : ""}`);
                } else if (request.method === "confirm") {
                  lines.push(`y yes · n no · x cancel request${request.extendable ? " · e extend 5 minutes" : ""}`);
                } else {
                  lines.push(`r compose ${request.method} response · x cancel request${request.extendable ? " · e extend 5 minutes" : ""}`);
                }
              }
              lines.push(`Filter: ${filter} · Search: ${enteringSearch ? `${query}█` : query || "none"}`);
              const entries = (presentation?.entries ?? []).filter((entry) =>
                (filter === "all" || entry.category === filter) &&
                (!query || `${entry.label}\n${entry.content}`.toLowerCase().includes(query.toLowerCase()))
              );
              if (!selectedEntryId || !entries.some((entry) => entry.id === selectedEntryId)) selectedEntryId = entries[0]?.id;
              const entryLines: string[] = [];
              for (const entry of entries) {
                const isCollapsed = collapsed.has(entry.id);
                entryLines.push(`${entry.id === selectedEntryId ? ">" : " "} ${isCollapsed ? "▶" : "▼"} ${entry.label} [${entry.category}]`);
                if (!isCollapsed) entryLines.push(...entry.content.split(/\r?\n/).map((line) => `    ${line}`));
              }
              if (entryLines.length === 0) entryLines.push("No transcript entries match.");
              lines.push(...entryLines.slice(scroll, scroll + 24));
            } else lines.push("Delegated Task is unavailable.");
          }
          if (composition) lines.push(
            `${composition.type === "steer" ? "Steer" : composition.type === "follow_up" ? "Follow up" : "Input response"}: ${composition.text}█`,
            "Enter send · Esc cancel composition",
          );
          else if (taskAbortConfirmation) lines.push(
            taskAbortConfirmation.requirement === "required"
              ? "Abort this required task? Required sibling atomicity cancels every open sibling."
              : "Abort this optional task? Only this task is cancelled; siblings continue.",
            "y confirm · n/Esc keep running",
          );
          else if (confirmation) lines.push(
            `Confirm cancellation of ${confirmation.displayId}: ${confirmation.active} active, ${confirmation.queued} queued?`,
            "y confirm · n/Esc keep running",
          );
          else if (interactionStatus) lines.push(interactionStatus);
          else if (cancellationStatus) lines.push(cancellationStatus);
          return lines.map((line) => piTui.truncateToWidth(line, Math.max(1, width)));
        },
        handleInput(data: string) {
          if (composition) {
            if (piTui.matchesKey(data, piTui.Key.escape)) {
              composition = undefined;
            } else if (piTui.matchesKey(data, piTui.Key.enter)) {
              const submitted = composition;
              composition = undefined;
              if (selectedTaskId) {
                if (submitted.type === "input" && presentation?.pendingInput) {
                  void control.respondToInput(selectedTaskId, presentation.pendingInput.id, { value: submitted.text }).then((result) => {
                    interactionStatus = result.status === "accepted" ? "Input response accepted." : `Input response rejected: ${result.code}.`;
                    tui.requestRender();
                  });
                } else if (submitted.type !== "input") {
                  void control.interact(selectedTaskId, { type: submitted.type, message: submitted.text }).then((result) => {
                    interactionStatus = result.status === "accepted"
                      ? `${submitted.type === "steer" ? "Steer" : "Follow up"} accepted.`
                      : `Interaction rejected: ${result.code}.`;
                    tui.requestRender();
                  });
                }
              }
            } else if (data === "\u007f") composition.text = composition.text.slice(0, -1);
            else {
              const text = compositionText(data);
              if (text !== undefined) composition.text += text;
            }
            tui.requestRender();
            return;
          }
          if (enteringSearch) {
            if (piTui.matchesKey(data, piTui.Key.escape)) {
              enteringSearch = false;
            } else if (piTui.matchesKey(data, piTui.Key.enter)) enteringSearch = false;
            else if (data === "\u007f") query = query.slice(0, -1);
            else if (/^[ -~]$/.test(data)) query += data;
            scroll = 0;
            tui.requestRender();
            return;
          }
          if (taskAbortConfirmation) {
            if (piTui.matchesKey(data, "y")) {
              const target = taskAbortConfirmation;
              taskAbortConfirmation = undefined;
              const result = control.abortTask(target.id);
              interactionStatus = result.status === "accepted" ? "Task abort requested." : `Task abort rejected: ${result.code}.`;
              tui.requestRender();
            } else if (piTui.matchesKey(data, "n") || piTui.matchesKey(data, piTui.Key.escape)) {
              taskAbortConfirmation = undefined;
              tui.requestRender();
            }
            return;
          }
          if (confirmation) {
            if (piTui.matchesKey(data, "y")) {
              const target = confirmation;
              confirmation = undefined;
              reportCancellation(target.id, target.displayId);
            } else if (piTui.matchesKey(data, "n") || piTui.matchesKey(data, piTui.Key.escape)) {
              confirmation = undefined;
              tui.requestRender();
            }
            return;
          }
          if (piTui.matchesKey(data, "q") || piTui.matchesKey(data, piTui.Key.escape)) {
            close();
            return;
          }
          const direction = piTui.matchesKey(data, piTui.Key.up) || piTui.matchesKey(data, "k") ? -1
            : piTui.matchesKey(data, piTui.Key.down) || piTui.matchesKey(data, "j") ? 1 : 0;
          if (view === "delegations") {
            if (direction) selectedDelegationId = move(snapshot.delegations, selectedDelegationId, direction) as DelegationId | undefined;
            else if (piTui.matchesKey(data, piTui.Key.enter)) {
              selectedTaskId = currentDelegation()?.tasks[0]?.id;
              view = "tasks";
            } else if (piTui.matchesKey(data, "c") && selectedDelegationId) {
              const target = currentDelegation();
              if (!target) return;
              if (target.state === "queued" || target.state === "running") confirmation = {
                id: target.id,
                displayId: target.displayId,
                active: target.tasks.filter((task) => task.state === "running" || task.state === "waiting-for-input" || task.state === "waiting-for-capability").length,
                queued: target.tasks.filter((task) => task.state === "queued").length,
              };
              else reportCancellation(target.id, target.displayId);
            }
          } else if (view === "tasks") {
            const tasks = currentDelegation()?.tasks ?? [];
            if (direction) selectedTaskId = move(tasks, selectedTaskId, direction) as DelegatedTaskId | undefined;
            else if (piTui.matchesKey(data, piTui.Key.enter) && selectedTaskId) {
              view = "session";
              releaseSession?.();
              releaseSession = control.retainTaskSession(selectedTaskId);
              bindPresentation();
            }
          } else {
            const pendingRequest = presentation?.pendingInput;
            if (piTui.matchesKey(data, "e") && selectedTaskId && pendingRequest?.extendable) {
              const result = control.extendInputTimeout(selectedTaskId, pendingRequest.id);
              interactionStatus = result.status === "accepted" ? "Dialog timeout extended by 5 minutes." : `Timeout extension rejected: ${result.code}.`;
              tui.requestRender();
              return;
            }
            if (pendingRequest?.method === "select") {
              const options = pendingRequest.options ?? [];
              if (direction && options.length > 0) {
                selectedDialogOption = Math.max(0, Math.min(options.length - 1, selectedDialogOption + direction));
              } else if (piTui.matchesKey(data, piTui.Key.enter) && selectedTaskId && options[selectedDialogOption] !== undefined) {
                void control.respondToInput(selectedTaskId, pendingRequest.id, { value: options[selectedDialogOption]! }).then(() => tui.requestRender());
              } else if (piTui.matchesKey(data, "x") && selectedTaskId) {
                void control.respondToInput(selectedTaskId, pendingRequest.id, { cancelled: true }).then(() => tui.requestRender());
              }
              tui.requestRender();
              return;
            }
            if (pendingRequest?.method === "confirm" && selectedTaskId && (piTui.matchesKey(data, "y") || piTui.matchesKey(data, "n"))) {
              void control.respondToInput(selectedTaskId, pendingRequest.id, { confirmed: piTui.matchesKey(data, "y") }).then(() => tui.requestRender());
              return;
            }
            const entries = (presentation?.entries ?? []).filter((entry) =>
              (filter === "all" || entry.category === filter) &&
              (!query || `${entry.label}\n${entry.content}`.toLowerCase().includes(query.toLowerCase()))
            );
            if (direction) {
              selectedEntryId = move(entries, selectedEntryId, direction);
              const transcriptLineCount = entries.reduce(
                (count, entry) => count + 1 +
                  (collapsed.has(entry.id) ? 0 : entry.content.split(/\r?\n/).length),
                0,
              );
              scroll = Math.max(0, Math.min(scroll + direction, Math.max(0, transcriptLineCount - 24)));
            } else if (piTui.matchesKey(data, piTui.Key.enter) && selectedEntryId) {
              if (collapsed.has(selectedEntryId)) collapsed.delete(selectedEntryId); else collapsed.add(selectedEntryId);
              const transcriptLineCount = entries.reduce(
                (count, entry) => count + 1 +
                  (collapsed.has(entry.id) ? 0 : entry.content.split(/\r?\n/).length),
                0,
              );
              scroll = Math.min(scroll, Math.max(0, transcriptLineCount - 24));
            } else if (piTui.matchesKey(data, "/")) {
              enteringSearch = true;
              query = "";
            } else if (piTui.matchesKey(data, "f")) {
              const filters: TranscriptFilter[] = ["all", "message", "reasoning", "tool", "error"];
              filter = filters[(filters.indexOf(filter) + 1) % filters.length]!;
              selectedEntryId = undefined;
              scroll = 0;
            } else if (piTui.matchesKey(data, "s")) {
              composition = { type: "steer", text: "" };
            } else if (piTui.matchesKey(data, "u")) {
              composition = { type: "follow_up", text: "" };
            } else if (piTui.matchesKey(data, "a") && selectedTaskId) {
              taskAbortConfirmation = {
                id: selectedTaskId,
                requirement: currentDelegation()?.requirement ?? "required",
              };
            } else if (piTui.matchesKey(data, "r") && presentation?.pendingInput &&
              (presentation.pendingInput.method === "input" || presentation.pendingInput.method === "editor")) {
              composition = { type: "input", text: presentation.pendingInput.prefill ?? "" };
            } else if (piTui.matchesKey(data, "x") && selectedTaskId && presentation?.pendingInput) {
              void control.respondToInput(selectedTaskId, presentation.pendingInput.id, { cancelled: true }).then(() => tui.requestRender());
            }
          }
          tui.requestRender();
        },
        invalidate() {},
        dispose() {
          closed = true;
          unsubscribe();
          detachPresentation?.();
          releaseSession?.();
          releaseSession = undefined;
          activeConsoleClosers.delete(close);
        },
      };
    }, { overlay: false });
  };

  return {
    async openConsole(context) { await open(context); },
    async openTask(context, displayId) {
      for (const delegation of registry.snapshot().delegations) {
        const task = delegation.tasks.find((candidate) => candidate.displayId.toUpperCase() === displayId.toUpperCase());
        if (task) {
          await open(context, { delegationId: delegation.id, taskId: task.id });
          return true;
        }
      }
      return false;
    },
    startSession(reason, context) {
      clearWidget();
      if (reason === "new" || reason === "resume" || reason === "reload") registry.reset();
      bindWidget(context);
    },
    shutdown() {
      clearWidget();
      for (const close of [...activeConsoleClosers]) close();
      registry.shutdown();
    },
  };
}
