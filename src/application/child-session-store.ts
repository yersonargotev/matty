import { randomUUID } from "node:crypto";
import { chmod, lstat, mkdir, readFile, readdir, realpath, rename, rm, stat, writeFile } from "node:fs/promises";
import { basename, dirname, join, relative, resolve, sep } from "node:path";

import type { MattyRole } from "../domain/capability-contract.ts";
import type { DelegatedTaskPresentation, DelegatedTranscriptPresentationEntry } from "./child-pi-runtime.ts";

export type DelegationPersistence = "persistent" | "ephemeral";
export type StoredChildSessionState = "active" | "succeeded" | "failed" | "cancelled" | "interrupted";

export interface ChildSessionManifest {
  schemaVersion: 1;
  taskId: string;
  delegationId: string;
  taskIndex: number;
  role: MattyRole;
  requirement: "required" | "optional";
  state: StoredChildSessionState;
  createdAt: number;
  updatedAt: number;
}

export interface ChildSessionMetadata {
  taskId: string;
  delegationId: string;
  taskIndex: number;
  role: MattyRole;
  requirement: "required" | "optional";
}

export class ChildSessionStoreError extends Error {
  readonly code: "invalid-task-id" | "incompatible-metadata" | "malformed-store";

  constructor(code: "invalid-task-id" | "incompatible-metadata" | "malformed-store") {
    super(`Child Session Store ${code}`);
    this.name = "ChildSessionStoreError";
    this.code = code;
  }
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const roles = new Set<MattyRole>(["explorer", "designer", "reviewer", "researcher", "worker"]);
const states = new Set<StoredChildSessionState>(["active", "succeeded", "failed", "cancelled", "interrupted"]);
const MANIFEST = "manifest.json";
const SESSION = "session.jsonl";
const DEFAULT_MAX_AGE = 7 * 24 * 60 * 60 * 1_000;
const DEFAULT_MAX_SESSIONS = 100;
const DEFAULT_MAX_BYTES = 1024 ** 3;

function directDirectory(root: string, taskId: string): string {
  if (!UUID.test(taskId)) throw new ChildSessionStoreError("invalid-task-id");
  const directory = join(root, taskId);
  if (resolve(directory) !== join(resolve(root), taskId)) throw new ChildSessionStoreError("invalid-task-id");
  return directory;
}

function validManifest(value: unknown, taskId: string): value is ChildSessionManifest {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const item = value as Record<string, unknown>;
  return Object.keys(item).length === 9 &&
    Object.keys(item).every((key) => ["schemaVersion", "taskId", "delegationId", "taskIndex", "role", "requirement", "state", "createdAt", "updatedAt"].includes(key)) &&
    item.schemaVersion === 1 && item.taskId === taskId && UUID.test(String(item.delegationId)) &&
    Number.isSafeInteger(item.taskIndex) && (item.taskIndex as number) >= 0 && roles.has(item.role as MattyRole) &&
    (item.requirement === "required" || item.requirement === "optional") && states.has(item.state as StoredChildSessionState) &&
    typeof item.createdAt === "number" && Number.isFinite(item.createdAt) && typeof item.updatedAt === "number" && Number.isFinite(item.updatedAt);
}

async function writeManifest(path: string, manifest: ChildSessionManifest): Promise<void> {
  const temporary = `${path}.${randomUUID()}.tmp`;
  await writeFile(temporary, `${JSON.stringify(manifest)}\n`, { mode: 0o600, flag: "wx" });
  await chmod(temporary, 0o600);
  await rename(temporary, path);
}

function overlaps(left: string, right: string): boolean {
  const fromLeft = relative(left, right);
  const fromRight = relative(right, left);
  const within = (value: string) => value === "" || (!value.startsWith(`..${sep}`) && value !== "..");
  return within(fromLeft) || within(fromRight);
}

async function futureRealpath(path: string): Promise<string> {
  let existing = resolve(path);
  const missing: string[] = [];
  while (true) {
    try {
      await lstat(existing);
      return join(await realpath(existing), ...missing);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
      const parent = dirname(existing);
      if (parent === existing) throw new ChildSessionStoreError("malformed-store");
      missing.unshift(basename(existing));
      existing = parent;
    }
  }
}

async function prepareBase(base: string, protectedRoot?: string): Promise<void> {
  const canonicalTarget = await futureRealpath(base);
  if (protectedRoot && overlaps(canonicalTarget, await realpath(protectedRoot))) {
    throw new ChildSessionStoreError("malformed-store");
  }
  await mkdir(base, { recursive: true, mode: 0o700 });
  const baseStat = await lstat(base);
  if (!baseStat.isDirectory() || baseStat.isSymbolicLink() || await realpath(base) !== canonicalTarget) {
    throw new ChildSessionStoreError("malformed-store");
  }
  await chmod(base, 0o700);
}

export interface ChildSessionHandle {
  readonly directory: string;
  readonly manifestFile: string;
  readonly sessionFile: string;
  readonly sessionId: string;
  prepare(cwd: string): Promise<void>;
  finish(state: Exclude<StoredChildSessionState, "active" | "interrupted">): Promise<void>;
  close(): Promise<void>;
}

const MAX_PRESENTATION_BYTES = 8 * 1024 * 1024;

function neutralize(value: string): string {
  return value
    .replace(/\u001b\[[0-?]*[ -/]*[@-~]/gu, "␛")
    .replace(/[\u0000-\u0008\u000b-\u001a\u001c-\u001f\u007f]/gu, "�");
}

function textContent(value: unknown): string {
  if (typeof value === "string") return neutralize(value);
  if (!Array.isArray(value)) return "";
  return value.flatMap((part) => {
    if (typeof part !== "object" || part === null) return [];
    const item = part as Record<string, unknown>;
    if (item.type === "text" && typeof item.text === "string") return [neutralize(item.text)];
    if (item.type === "image" && typeof item.mimeType === "string") return [`[image ${neutralize(item.mimeType)}]`];
    return [];
  }).join("\n");
}

function printable(value: unknown): string {
  try { return neutralize(JSON.stringify(value)); } catch { return "[unavailable]"; }
}

/** Loads persisted transcript content only for the private Child Session presentation seam. */
export async function loadChildSessionPresentation(sessionFile: string): Promise<DelegatedTaskPresentation> {
  if ((await stat(sessionFile)).size > MAX_PRESENTATION_BYTES) {
    throw new ChildSessionStoreError("malformed-store");
  }
  const source = await readFile(sessionFile, "utf8");
  const lines = source.trim().split(/\r?\n/).filter(Boolean);
  let header = false;
  const entries: DelegatedTranscriptPresentationEntry[] = [];
  const add = (
    id: string,
    category: DelegatedTranscriptPresentationEntry["category"],
    label: string,
    content: string,
    expandedByDefault: boolean,
  ) => entries.push(Object.freeze({ id, category, label, content, expandedByDefault }));
  let inputTokens = 0, outputTokens = 0, totalTokens = 0, cost = 0;
  for (const [index, line] of lines.entries()) {
    let parsed: unknown;
    try { parsed = JSON.parse(line); } catch { throw new ChildSessionStoreError("malformed-store"); }
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) throw new ChildSessionStoreError("malformed-store");
    const item = parsed as Record<string, unknown>;
    if (index === 0) {
      if (item.type !== "session" || item.version !== 3) throw new ChildSessionStoreError("malformed-store");
      header = true;
      continue;
    }
    if (item.type !== "message" || typeof item.message !== "object" || item.message === null) continue;
    const message = item.message as Record<string, unknown>;
    const role = typeof message.role === "string" ? message.role : "message";
    if (role === "assistant" && Array.isArray(message.content)) {
      for (const [partIndex, value] of message.content.entries()) {
        if (typeof value !== "object" || value === null) continue;
        const part = value as Record<string, unknown>;
        if (part.type === "text" && typeof part.text === "string") {
          add(`persisted:${index}:${partIndex}`, "message", "Assistant", neutralize(part.text), true);
        } else if (part.type === "thinking" && typeof part.thinking === "string") {
          add(`persisted:${index}:${partIndex}`, "reasoning", "Reasoning", neutralize(part.thinking), false);
        } else if (part.type === "toolCall" && typeof part.name === "string") {
          add(`persisted:${index}:${partIndex}`, "tool", `Tool · ${neutralize(part.name)}`, `Arguments: ${printable(part.arguments)}`, false);
        }
      }
      if (typeof message.errorMessage === "string" && message.errorMessage.length > 0) {
        add(`persisted:${index}:error`, "error", "Error", neutralize(message.errorMessage), false);
      }
    } else if (role === "toolResult") {
      const toolName = typeof message.toolName === "string" ? ` · ${neutralize(message.toolName)}` : "";
      const result = textContent(message.content);
      const details = message.details === undefined ? "" : `\nDetails: ${printable(message.details)}`;
      add(`persisted:${index}`, message.isError ? "error" : "tool", `Tool result${toolName}`, `${result}${details}`.trim() || "No textual result", false);
    } else if (role === "bashExecution") {
      add(`persisted:${index}`, message.exitCode === 0 ? "tool" : "error", "Bash execution", [
        typeof message.command === "string" ? `Command: ${neutralize(message.command)}` : undefined,
        typeof message.output === "string" ? `Output: ${neutralize(message.output)}` : undefined,
      ].filter(Boolean).join("\n") || "No textual result", false);
    } else {
      const content = textContent(message.content) ||
        (typeof message.summary === "string" ? neutralize(message.summary) : "");
      if (content) add(`persisted:${index}`, "message", role === "user" ? "User" : neutralize(role), content, role === "user");
    }
    if (typeof message.usage === "object" && message.usage !== null) {
      const usage = message.usage as Record<string, unknown>;
      inputTokens += typeof usage.input === "number" ? usage.input : 0;
      outputTokens += typeof usage.output === "number" ? usage.output : 0;
      totalTokens += typeof usage.totalTokens === "number" ? usage.totalTokens : 0;
      if (typeof usage.cost === "object" && usage.cost !== null) {
        const total = (usage.cost as Record<string, unknown>).total;
        cost += typeof total === "number" ? total : 0;
      }
    }
  }
  if (!header) throw new ChildSessionStoreError("malformed-store");
  return Object.freeze({ revision: 1, sessionState: "settled", assistant: Object.freeze([]), tools: Object.freeze([]), entries: Object.freeze(entries), usage: Object.freeze({ inputTokens, outputTokens, totalTokens, cost }) });
}

export class ChildSessionStore {
  readonly root: string;
  readonly #ephemeralRoot: string;
  readonly #now: () => number;
  readonly #maxAgeMs: number;
  readonly #maxSessions: number;
  readonly #maxBytes: number;
  #operation: Promise<void> = Promise.resolve();

  constructor(options: { root: string; ephemeralRoot: string; now?: () => number; maxAgeMs?: number; maxSessions?: number; maxBytes?: number }) {
    this.root = resolve(options.root);
    this.#ephemeralRoot = resolve(options.ephemeralRoot);
    this.#now = options.now ?? Date.now;
    this.#maxAgeMs = options.maxAgeMs ?? DEFAULT_MAX_AGE;
    this.#maxSessions = options.maxSessions ?? DEFAULT_MAX_SESSIONS;
    this.#maxBytes = options.maxBytes ?? DEFAULT_MAX_BYTES;
  }

  session(metadata: ChildSessionMetadata, persistence: DelegationPersistence, protectedRoot?: string): ChildSessionHandle {
    const base = persistence === "persistent" ? this.root : this.#ephemeralRoot;
    const directory = directDirectory(base, metadata.taskId);
    const manifestFile = join(directory, MANIFEST);
    const sessionFile = join(directory, SESSION);
    let manifest: ChildSessionManifest | undefined;
    return {
      directory, manifestFile, sessionFile, sessionId: metadata.taskId,
      prepare: async (cwd) => await this.#exclusive(async () => {
        await prepareBase(base, protectedRoot);
        await mkdir(directory, { mode: 0o700 });
        await chmod(directory, 0o700);
        const timestamp = this.#now();
        manifest = { schemaVersion: 1, ...metadata, state: "active", createdAt: timestamp, updatedAt: timestamp };
        await writeManifest(manifestFile, manifest);
        await writeFile(sessionFile, `${JSON.stringify({
          type: "session", version: 3, id: metadata.taskId,
          timestamp: new Date(this.#now()).toISOString(), cwd,
        })}\n`, { mode: 0o600, flag: "wx" });
        await chmod(sessionFile, 0o600);
      }),
      finish: async (state) => await this.#exclusive(async () => {
        if (!manifest) throw new ChildSessionStoreError("malformed-store");
        await chmod(sessionFile, 0o600);
        manifest = { ...manifest, state, updatedAt: this.#now() };
        await writeManifest(manifestFile, manifest);
        if (persistence === "persistent") await this.#enforceRetention(protectedRoot);
      }),
      close: async () => await this.#exclusive(async () => {
        if (persistence === "ephemeral") await rm(directory, { recursive: true, force: true });
      }),
    };
  }

  async discover(options: { interruptActive?: boolean; protectedRoot?: string } = { interruptActive: true }): Promise<Array<{ manifest: ChildSessionManifest; directory: string; sessionFile: string; bytes: number }>> {
    return await this.#exclusive(async () => await this.#discover(options));
  }

  async #discover(options: { interruptActive?: boolean; protectedRoot?: string }): Promise<Array<{ manifest: ChildSessionManifest; directory: string; sessionFile: string; bytes: number }>> {
    await prepareBase(this.root, options.protectedRoot);
    const names = await readdir(this.root);
    const discovered: Array<{ manifest: ChildSessionManifest; directory: string; sessionFile: string; bytes: number }> = [];
    for (const name of names.sort()) {
      if (!UUID.test(name)) throw new ChildSessionStoreError("malformed-store");
      const directory = directDirectory(this.root, name);
      const directoryStat = await lstat(directory);
      if (!directoryStat.isDirectory() || directoryStat.isSymbolicLink()) throw new ChildSessionStoreError("malformed-store");
      const children = (await readdir(directory)).sort();
      if (children.length !== 2 || children[0] !== MANIFEST || children[1] !== SESSION) throw new ChildSessionStoreError("malformed-store");
      for (const child of children) {
        const childStat = await lstat(join(directory, child));
        if (!childStat.isFile() || childStat.isSymbolicLink()) throw new ChildSessionStoreError("malformed-store");
      }
      let parsed: unknown;
      try { parsed = JSON.parse(await readFile(join(directory, MANIFEST), "utf8")); }
      catch { throw new ChildSessionStoreError("incompatible-metadata"); }
      if (!validManifest(parsed, name)) throw new ChildSessionStoreError("incompatible-metadata");
      const manifest = parsed;
      if (manifest.state === "active" && options.interruptActive !== false) {
        const interrupted = { ...manifest, state: "interrupted" as const, updatedAt: this.#now() };
        await writeManifest(join(directory, MANIFEST), interrupted);
        discovered.push({ manifest: interrupted, directory, sessionFile: join(directory, SESSION), bytes: (await stat(join(directory, MANIFEST))).size + (await stat(join(directory, SESSION))).size });
      } else {
        discovered.push({ manifest, directory, sessionFile: join(directory, SESSION), bytes: (await stat(join(directory, MANIFEST))).size + (await stat(join(directory, SESSION))).size });
      }
    }
    return discovered;
  }

  async enforceRetention(protectedRoot?: string): Promise<void> {
    await this.#exclusive(async () => await this.#enforceRetention(protectedRoot));
  }

  async #enforceRetention(protectedRoot?: string): Promise<void> {
    const sessions = await this.#discover({ interruptActive: false, ...(protectedRoot ? { protectedRoot } : {}) });
    const terminal = sessions.filter((item) => item.manifest.state !== "active")
      .sort((left, right) => left.manifest.updatedAt - right.manifest.updatedAt || left.manifest.taskId.localeCompare(right.manifest.taskId));
    const remove = new Set<string>();
    for (const item of terminal) if (this.#now() - item.manifest.updatedAt > this.#maxAgeMs) remove.add(item.manifest.taskId);
    let count = sessions.length - remove.size;
    let bytes = sessions.filter((item) => !remove.has(item.manifest.taskId)).reduce((sum, item) => sum + item.bytes, 0);
    for (const item of terminal) {
      if (remove.has(item.manifest.taskId)) continue;
      if (count <= this.#maxSessions && bytes <= this.#maxBytes) break;
      remove.add(item.manifest.taskId); count -= 1; bytes -= item.bytes;
    }
    for (const item of terminal) if (remove.has(item.manifest.taskId)) await rm(directDirectory(this.root, item.manifest.taskId), { recursive: true });
  }

  async #exclusive<T>(operation: () => Promise<T>): Promise<T> {
    const result = this.#operation.then(operation, operation);
    this.#operation = result.then(() => undefined, () => undefined);
    return await result;
  }
}
