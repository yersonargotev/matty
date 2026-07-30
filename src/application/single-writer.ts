import { createHash, randomUUID } from "node:crypto";
import {
  link,
  mkdir,
  open,
  readFile,
  unlink,
} from "node:fs/promises";
import { join } from "node:path";

function errorCode(error: unknown): string | undefined {
  return typeof error === "object" && error !== null && "code" in error
    ? String((error as { code?: unknown }).code)
    : undefined;
}

function processIsAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return errorCode(error) === "EPERM";
  }
}

export function singleWriterStatePath(stateRoot: string): string {
  return join(stateRoot, "matty-single-writer-v1");
}

export async function acquireRepositoryWriter(
  workingTree: string,
  stateRoot: string,
): Promise<(() => Promise<void>) | undefined> {
  const lockRoot = singleWriterStatePath(stateRoot);
  await mkdir(lockRoot, { recursive: true, mode: 0o700 });
  const repositoryId = createHash("sha256")
    .update(workingTree)
    .digest("hex");
  const lockPath = join(lockRoot, repositoryId);

  for (let attempt = 0; attempt < 2; attempt += 1) {
    const token = `${process.pid}:${randomUUID()}`;
    const candidate = join(lockRoot, `.${repositoryId}.${randomUUID()}.tmp`);
    const handle = await open(candidate, "wx", 0o600);
    try {
      await handle.writeFile(token);
    } finally {
      await handle.close();
    }

    try {
      await link(candidate, lockPath);
      return async () => {
        try {
          if (await readFile(lockPath, "utf8") === token) {
            await unlink(lockPath);
          }
        } catch (error) {
          if (errorCode(error) !== "ENOENT") {
            throw error;
          }
        }
      };
    } catch (error) {
      if (errorCode(error) !== "EEXIST") {
        throw error;
      }
    } finally {
      await unlink(candidate).catch((error: unknown) => {
        if (errorCode(error) !== "ENOENT") {
          throw error;
        }
      });
    }

    try {
      const owner = await readFile(lockPath, "utf8");
      const ownerPid = Number.parseInt(owner.split(":", 1)[0] ?? "", 10);
      if (Number.isInteger(ownerPid) && processIsAlive(ownerPid)) {
        return undefined;
      }
      await unlink(lockPath);
    } catch (error) {
      if (errorCode(error) !== "ENOENT") {
        throw error;
      }
    }
  }
  return undefined;
}
