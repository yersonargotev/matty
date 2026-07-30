import { randomUUID } from "node:crypto";
import {
  constants,
  lstat,
  mkdir,
  open,
  readdir,
  readFile,
  realpath,
  rm,
} from "node:fs/promises";
import { dirname, isAbsolute, relative, resolve, sep } from "node:path";

import {
  isPathWithin,
  isResearchRunId,
} from "./research-paths.ts";

export const RESEARCH_WORKSPACE_MARKER = ".matty-research-workspace.json";
const RESEARCH_WORKSPACE_MAX_AGE_MS = 24 * 60 * 60 * 1_000;

export interface ResearchWorkspace {
  temporaryRoot: string;
  projectRoot: string;
  workspace: string;
  report: string;
}

async function validatedScope(
  scope: ResearchWorkspace,
): Promise<ResearchWorkspace> {
  const temporaryRoot = await realpath(scope.temporaryRoot);
  const projectRoot = await realpath(scope.projectRoot);
  const workspace = await realpath(scope.workspace);
  if (
    temporaryRoot !== scope.temporaryRoot ||
    projectRoot !== scope.projectRoot ||
    workspace !== scope.workspace ||
    !isPathWithin(temporaryRoot, workspace) ||
    workspace === temporaryRoot ||
    !isPathWithin(projectRoot, scope.report) ||
    !scope.report.endsWith(".md")
  ) {
    throw new Error("research workspace scope is invalid");
  }
  if (!(await validatedMarker(workspace))) {
    throw new Error("research workspace scope marker is invalid");
  }
  return { ...scope, temporaryRoot, projectRoot, workspace };
}

interface ResearchWorkspaceMarker {
  schemaVersion: 1;
  runId: string;
  workspace: string;
}

async function assertNoSymlinkPath(
  root: string,
  target: string,
  label: string,
): Promise<void> {
  if (!isPathWithin(root, target)) {
    throw new Error(`${label} escapes its validated root`);
  }
  const path = relative(root, target);
  let current = root;
  for (const segment of path.split(sep).filter(Boolean)) {
    current = resolve(current, segment);
    try {
      const stats = await lstat(current);
      if (stats.isSymbolicLink()) {
        throw new Error(`${label} contains a symlink`);
      }
    } catch (error) {
      if (
        typeof error === "object" &&
        error !== null &&
        "code" in error &&
        error.code === "ENOENT"
      ) {
        return;
      }
      throw error;
    }
  }
}

async function writeExclusive(path: string, content: string): Promise<void> {
  let handle;
  try {
    handle = await open(
      path,
      constants.O_WRONLY |
        constants.O_CREAT |
        constants.O_EXCL |
        constants.O_NOFOLLOW,
      0o600,
    );
    await handle.writeFile(content, "utf8");
  } catch (error) {
    if (
      typeof error === "object" &&
      error !== null &&
      "code" in error &&
      error.code === "EEXIST"
    ) {
      throw new Error("research artifact overwrite is not authorized");
    }
    throw error;
  } finally {
    await handle?.close();
  }
}

export async function createResearchWorkspace(input: {
  temporaryRoot: string;
  projectRoot: string;
  report: string;
}): Promise<ResearchWorkspace> {
  await mkdir(input.temporaryRoot, { recursive: true, mode: 0o700 });
  const temporaryRoot = await realpath(input.temporaryRoot);
  const suppliedProjectRoot = resolve(input.projectRoot);
  const projectRoot = await realpath(input.projectRoot);
  const suppliedReport = resolve(suppliedProjectRoot, input.report);
  const report = isPathWithin(suppliedProjectRoot, suppliedReport)
    ? resolve(projectRoot, relative(suppliedProjectRoot, suppliedReport))
    : suppliedReport;
  if (!isPathWithin(projectRoot, report) || !report.endsWith(".md")) {
    throw new Error(
      "research report must be a Markdown path inside the validated project",
    );
  }
  await assertNoSymlinkPath(projectRoot, report, "research report path");
  try {
    await lstat(report);
    throw new Error("research artifact overwrite is not authorized");
  } catch (error) {
    if (
      !(
        typeof error === "object" &&
        error !== null &&
        "code" in error &&
        error.code === "ENOENT"
      )
    ) {
      throw error;
    }
  }

  const runId = randomUUID();
  const workspace = resolve(temporaryRoot, runId);
  if (!isPathWithin(temporaryRoot, workspace)) {
    throw new Error("research workspace path escapes its validated root");
  }
  await mkdir(workspace, { mode: 0o700 });
  const normalizedWorkspace = await realpath(workspace);
  const marker: ResearchWorkspaceMarker = {
    schemaVersion: 1,
    runId,
    workspace: normalizedWorkspace,
  };
  await writeExclusive(
    resolve(normalizedWorkspace, RESEARCH_WORKSPACE_MARKER),
    `${JSON.stringify(marker)}\n`,
  );
  return {
    temporaryRoot,
    projectRoot,
    workspace: normalizedWorkspace,
    report,
  };
}

export async function writeResearchFile(
  scope: ResearchWorkspace,
  input:
    | {
        destination: "workspace";
        path: string;
        content: string;
      }
    | {
        destination: "report";
        content: string;
      },
): Promise<{ path: string }> {
  scope = await validatedScope(scope);
  let root: string;
  let target: string;
  let label: string;
  if (input.destination === "workspace") {
    if (
      input.path.length === 0 ||
      isAbsolute(input.path) ||
      input.path.split(/[\\/]/).some((segment) =>
        segment === "." || segment === ".."
      )
    ) {
      throw new Error("research workspace path must be relative and contained");
    }
    root = scope.workspace;
    target = resolve(root, input.path);
    label = "research workspace path";
    if (target === resolve(root, RESEARCH_WORKSPACE_MARKER)) {
      throw new Error("research workspace path cannot replace its marker");
    }
  } else {
    root = scope.projectRoot;
    target = scope.report;
    label = "research report path";
  }

  await assertNoSymlinkPath(root, target, label);
  await mkdir(dirname(target), { recursive: true, mode: 0o700 });
  await assertNoSymlinkPath(root, target, label);
  await writeExclusive(target, input.content);
  return { path: target };
}

export async function cleanupResearchWorkspace(
  scope: ResearchWorkspace,
): Promise<void> {
  scope = await validatedScope(scope);
  const { workspace } = scope;
  const marker = await validatedMarker(workspace);
  if (!marker) {
    throw new Error("research workspace cleanup marker is invalid");
  }
  await rm(workspace, { recursive: true });
}

async function validatedMarker(
  workspace: string,
): Promise<ResearchWorkspaceMarker | undefined> {
  try {
    const stats = await lstat(resolve(workspace, RESEARCH_WORKSPACE_MARKER));
    if (!stats.isFile() || stats.isSymbolicLink()) {
      return undefined;
    }
    const value = JSON.parse(
      await readFile(resolve(workspace, RESEARCH_WORKSPACE_MARKER), "utf8"),
    ) as Partial<ResearchWorkspaceMarker>;
    if (
      value.schemaVersion !== 1 ||
      typeof value.runId !== "string" ||
      !isResearchRunId(value.runId) ||
      value.runId !== workspace.split(sep).at(-1) ||
      value.workspace !== workspace
    ) {
      return undefined;
    }
    return value as ResearchWorkspaceMarker;
  } catch {
    return undefined;
  }
}

export async function cleanupStaleResearchWorkspaces(input: {
  temporaryRoot: string;
  now?: Date;
}): Promise<string[]> {
  try {
    const temporaryRoot = await realpath(input.temporaryRoot);
    const entries = await readdir(temporaryRoot, { withFileTypes: true });
    const removed: string[] = [];
    for (const entry of entries) {
      if (!entry.isDirectory() || entry.isSymbolicLink()) {
        continue;
      }
      const workspace = resolve(temporaryRoot, entry.name);
      const normalized = await realpath(workspace);
      if (
        normalized !== workspace ||
        !isPathWithin(temporaryRoot, normalized) ||
        !(await validatedMarker(normalized))
      ) {
        continue;
      }
      const markerStats = await lstat(
        resolve(normalized, RESEARCH_WORKSPACE_MARKER),
      );
      const age = (input.now ?? new Date()).getTime() - markerStats.mtimeMs;
      if (age <= RESEARCH_WORKSPACE_MAX_AGE_MS) {
        continue;
      }
      await rm(normalized, { recursive: true });
      removed.push(normalized);
    }
    return removed;
  } catch (error) {
    if (
      typeof error === "object" &&
      error !== null &&
      "code" in error &&
      error.code === "ENOENT"
    ) {
      return [];
    }
    throw error;
  }
}
