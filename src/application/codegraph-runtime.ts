import type { CodeGraph as CodeGraphInstanceType } from "@colbymchenry/codegraph";
import { createHash } from "node:crypto";
import {
  mkdir,
  readFile,
  realpath,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { realpathSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import { parse, join, resolve, sep } from "node:path";
import { createRequire } from "node:module";

export type CodeGraphInitializationResult = {
  status: "already-initialized" | "initialized";
  root: string;
};

interface CodeGraphInstance {
  close(): void;
}

export interface CodeGraphRuntimeDependencies {
  canonicalize(path: string): Promise<string>;
  findNearestRoot(path: string): string | null;
  unsafeRootReason(path: string): string | null;
  initialize(path: string): Promise<CodeGraphInstance>;
  withProjectLock<T>(path: string, operation: () => Promise<T>): Promise<T>;
}

const require = createRequire(import.meta.url);
type CodeGraphSdk = {
  CodeGraph: typeof CodeGraphInstanceType;
  findNearestCodeGraphRoot(path: string): string | null;
};
let codeGraphSdk: CodeGraphSdk | undefined;
function loadCodeGraphSdk(): CodeGraphSdk {
  codeGraphSdk ??= require("@colbymchenry/codegraph") as CodeGraphSdk;
  return codeGraphSdk;
}

const LOCK_POLL_MS = 200;
const LOCK_WAIT_TIMEOUT_MS = 15 * 60_000;
const OWNER_FILE = "owner.json";

function processExists(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return (error as NodeJS.ErrnoException).code === "EPERM";
  }
}

async function lockOwnerIsAlive(lockPath: string): Promise<boolean> {
  try {
    const owner = JSON.parse(
      await readFile(join(lockPath, OWNER_FILE), "utf8"),
    ) as { pid?: unknown };
    return typeof owner.pid === "number" && processExists(owner.pid);
  } catch {
    try {
      const info = await stat(lockPath);
      return Date.now() - info.mtimeMs < 30_000;
    } catch {
      return false;
    }
  }
}

async function withFilesystemProjectLock<T>(
  projectRoot: string,
  operation: () => Promise<T>,
): Promise<T> {
  const identity = `${projectRoot}\0${process.env.CODEGRAPH_DIR ?? ".codegraph"}`;
  const key = createHash("sha256").update(identity).digest("hex");
  const locksRoot = join(tmpdir(), "matty-codegraph-locks");
  const lockPath = join(locksRoot, key);
  const ownershipToken = createHash("sha256")
    .update(`${process.pid}\0${Date.now()}\0${Math.random()}`)
    .digest("hex");
  const waitStartedAt = Date.now();
  await mkdir(locksRoot, { recursive: true });

  while (true) {
    try {
      await mkdir(lockPath);
      await writeFile(
        join(lockPath, OWNER_FILE),
        JSON.stringify({
          pid: process.pid,
          createdAt: Date.now(),
          token: ownershipToken,
        }),
        "utf8",
      );
      break;
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "EEXIST") {
        throw error;
      }
      if (!(await lockOwnerIsAlive(lockPath))) {
        await rm(lockPath, { recursive: true, force: true });
        continue;
      }
      if (Date.now() - waitStartedAt >= LOCK_WAIT_TIMEOUT_MS) {
        throw new Error(
          `Timed out waiting for CodeGraph initialization lock for ${projectRoot}.`,
        );
      }
      await new Promise((resolve) => setTimeout(resolve, LOCK_POLL_MS));
    }
  }

  try {
    return await operation();
  } finally {
    try {
      const owner = JSON.parse(
        await readFile(join(lockPath, OWNER_FILE), "utf8"),
      ) as { token?: unknown };
      if (owner.token === ownershipToken) {
        await rm(lockPath, { recursive: true, force: true });
      }
    } catch {
      // A missing or replaced lock is no longer owned by this operation.
    }
  }
}

function resolveExistingPath(path: string): string {
  try {
    return realpathSync(resolve(path));
  } catch {
    return resolve(path);
  }
}

export function unsafeCodeGraphRootReason(path: string): string | null {
  const resolved = resolveExistingPath(path);
  if (resolved === parse(resolved).root) {
    return "the filesystem root";
  }
  const home = resolveExistingPath(homedir());
  const normalize = (value: string) =>
    process.platform === "darwin" || process.platform === "win32"
      ? value.toLowerCase()
      : value;
  const normalizedRoot = normalize(resolved);
  const normalizedHome = normalize(home);
  if (normalizedRoot === normalizedHome) {
    return "your home directory";
  }
  if (normalizedHome.startsWith(`${normalizedRoot}${sep}`)) {
    return "a parent of your home directory";
  }
  return null;
}

const defaultDependencies: CodeGraphRuntimeDependencies = {
  canonicalize: realpath,
  findNearestRoot(path) {
    return loadCodeGraphSdk().findNearestCodeGraphRoot(path);
  },
  unsafeRootReason: unsafeCodeGraphRootReason,
  async initialize(path) {
    return loadCodeGraphSdk().CodeGraph.init(path, { index: true });
  },
  withProjectLock: withFilesystemProjectLock,
};

export function createCodeGraphProjectInitializer(
  dependencies: CodeGraphRuntimeDependencies = defaultDependencies,
): (cwd: string) => Promise<CodeGraphInitializationResult> {
  const inFlight = new Map<string, Promise<CodeGraphInitializationResult>>();

  return async (cwd) => {
    const canonicalRoot = await dependencies.canonicalize(cwd);
    const existing = inFlight.get(canonicalRoot);
    if (existing) {
      return existing;
    }

    const initialization = (async () => {
      const nearestRoot = dependencies.findNearestRoot(canonicalRoot);
      if (nearestRoot) {
        return { status: "already-initialized", root: nearestRoot } as const;
      }

      const unsafeReason = dependencies.unsafeRootReason(canonicalRoot);
      if (unsafeReason) {
        throw new Error(
          `CodeGraph initialization refused because ${canonicalRoot} looks like ${unsafeReason}.`,
        );
      }

      return dependencies.withProjectLock(canonicalRoot, async () => {
        const initializedWhileWaiting = dependencies.findNearestRoot(
          canonicalRoot,
        );
        if (initializedWhileWaiting) {
          return {
            status: "already-initialized",
            root: initializedWhileWaiting,
          } as const;
        }

        let graph: CodeGraphInstance | undefined;
        try {
          graph = await dependencies.initialize(canonicalRoot);
          return { status: "initialized", root: canonicalRoot } as const;
        } finally {
          graph?.close();
        }
      });
    })();

    inFlight.set(canonicalRoot, initialization);
    try {
      return await initialization;
    } finally {
      inFlight.delete(canonicalRoot);
    }
  };
}

export const initializeCodeGraphProject = createCodeGraphProjectInitializer();
