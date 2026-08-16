import type { ExtensionContext } from "@earendil-works/pi-coding-agent";
import * as piTui from "@earendil-works/pi-tui";

import {
  DelegationRegistry,
  type DelegationCancellationResult,
  type DelegationId,
} from "../application/delegation-registry.ts";
import {
  renderDelegationConsole,
  renderDelegationWidget,
} from "../application/delegation-presentation.ts";

const WIDGET_ID = "matty-delegations";

type SessionReason = "new" | "resume" | "reload" | string;

export interface PiDelegationManagement {
  openConsole(context: ExtensionContext): Promise<void>;
  startSession(reason: SessionReason, context: ExtensionContext): void;
  shutdown(): void;
}

export function createPiDelegationManagement(
  registry: DelegationRegistry,
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
      context.ui.setWidget(
        WIDGET_ID,
        lines.length > 0
          ? () => ({
            render(width: number) {
              return lines.map((line) =>
                piTui.truncateToWidth(line, Math.max(1, width))
              );
            },
            invalidate() {},
          })
          : undefined,
      );
    };
    unsubscribeWidget = registry.subscribe(update);
    update();
  };

  return {
    async openConsole(context) {
      await context.ui.custom<void>((tui, _theme, _keybindings, done) => {
        let snapshot = registry.snapshot();
        let selectedId = snapshot.delegations[0]?.id;
        let confirmation: {
          id: DelegationId;
          displayId: string;
          active: number;
          queued: number;
        } | undefined;
        let cancellationStatus: string | undefined;
        const cancellationMessages = {
          "cancelling": (displayId: string) =>
            `Cancellation requested for ${displayId}.`,
          "already-cancelling": (displayId: string) =>
            `Cancellation is already in progress for ${displayId}.`,
          "already-finished": (displayId: string) => `${displayId} is already finished.`,
        } satisfies Record<DelegationCancellationResult, (displayId: string) => string>;
        const reportCancellation = (id: DelegationId, displayId: string) => {
          const result = registry.cancel(id);
          cancellationStatus = cancellationMessages[result](displayId);
          tui.requestRender();
        };
        const expandedIds = new Set<DelegationId>();
        let closed = false;
        const unsubscribe = registry.subscribe(() => {
          snapshot = registry.snapshot();
          if (selectedId && !snapshot.delegations.some((entry) => entry.id === selectedId)) {
            selectedId = snapshot.delegations[0]?.id;
          }
          tui.requestRender();
        });
        const close = () => {
          if (closed) return;
          closed = true;
          unsubscribe();
          activeConsoleClosers.delete(close);
          done();
        };
        activeConsoleClosers.add(close);
        return {
          render(width: number) {
            const lines = renderDelegationConsole(snapshot, {
              ...(selectedId ? { selectedId } : {}),
              expandedIds,
            });
            if (confirmation) {
              lines.push(
                `Confirm cancellation of ${confirmation.displayId}: ${confirmation.active} active, ${confirmation.queued} queued?`,
                "y confirm · n/Esc keep running",
              );
            } else if (cancellationStatus) {
              lines.push(cancellationStatus);
            }
            return lines.map((line) =>
              piTui.truncateToWidth(line, Math.max(1, width))
            );
          },
          handleInput(data: string) {
            if (confirmation) {
              if (piTui.matchesKey(data, "y")) {
                const target = confirmation;
                confirmation = undefined;
                reportCancellation(target.id, target.displayId);
              } else if (
                piTui.matchesKey(data, "n") ||
                piTui.matchesKey(data, piTui.Key.escape)
              ) {
                confirmation = undefined;
                tui.requestRender();
              }
              return;
            }
            if (piTui.matchesKey(data, "q") || piTui.matchesKey(data, piTui.Key.escape)) {
              close();
              return;
            }
            const entries = snapshot.delegations;
            const index = Math.max(0, entries.findIndex((entry) => entry.id === selectedId));
            if (piTui.matchesKey(data, piTui.Key.up) || piTui.matchesKey(data, "k")) {
              selectedId = entries[Math.max(0, index - 1)]?.id ?? selectedId;
              tui.requestRender();
            } else if (piTui.matchesKey(data, piTui.Key.down) || piTui.matchesKey(data, "j")) {
              selectedId = entries[Math.min(entries.length - 1, index + 1)]?.id ?? selectedId;
              tui.requestRender();
            } else if (piTui.matchesKey(data, piTui.Key.enter) && selectedId) {
              if (expandedIds.has(selectedId)) expandedIds.delete(selectedId);
              else expandedIds.add(selectedId);
              tui.requestRender();
            } else if (piTui.matchesKey(data, "c") && selectedId) {
              const target = entries.find((entry) => entry.id === selectedId);
              if (!target) return;
              if (target.state === "queued" || target.state === "running") {
                cancellationStatus = undefined;
                confirmation = {
                  id: target.id,
                  displayId: target.displayId,
                  active: target.tasks.filter((task) => task.state === "running").length,
                  queued: target.tasks.filter((task) => task.state === "queued").length,
                };
                tui.requestRender();
              } else {
                reportCancellation(target.id, target.displayId);
              }
            }
          },
          invalidate() {},
          dispose() {
            closed = true;
            unsubscribe();
            activeConsoleClosers.delete(close);
          },
        };
      }, { overlay: false });
    },

    startSession(reason, context) {
      clearWidget();
      if (reason === "new" || reason === "resume" || reason === "reload") {
        registry.reset();
      }
      bindWidget(context);
    },

    shutdown() {
      clearWidget();
      for (const close of [...activeConsoleClosers]) close();
      registry.shutdown();
    },
  };
}
