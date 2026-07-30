import { createHash } from "node:crypto";

export interface SkillCatalogFile {
  path: string;
  contents: Uint8Array;
}

interface SkillCatalogMember {
  name: string;
  path: string;
}

interface SkillCatalogManifest {
  schemaVersion: 1;
  upstream: {
    repository: string;
    commit: string;
    roots: string[];
    fileCount: number;
    snapshotSha256: string;
  };
  releaseContentSha256: string;
  members: SkillCatalogMember[];
  askMatt: {
    path: string;
    builtInRoutes: string[];
    optionalRoutes: string[];
  };
}

export type SkillCatalogIssue =
  | "invalid-manifest"
  | "wrong-member-count"
  | "missing-member"
  | "extra-member"
  | "duplicate-member"
  | "name-path-mismatch"
  | "wrong-file-count"
  | "content-digest-mismatch"
  | "invalid-ask-matt-route";

export interface SkillCatalogValidation {
  state: "valid" | "invalid";
  memberNames: string[];
  issues: SkillCatalogIssue[];
  provenance?: {
    repository: string;
    commit: string;
    snapshotSha256: string;
  };
}

const skillNamePattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const sha256Pattern = /^[a-f0-9]{64}$/;
const pinnedUpstream = {
  repository: "https://github.com/mattpocock/skills",
  commit: "2ab958093e83e0ec752e6c1c5932da465bf23e0c",
  roots: ["skills/engineering", "skills/productivity"],
  fileCount: 68,
  snapshotSha256:
    "e5cf6d080ecfe4c3c197d2268704cb3bc42c00455703a8fe2be2f3b061bc6509",
} as const;

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) &&
    value.every((entry) => typeof entry === "string");
}

function parseManifest(value: unknown): SkillCatalogManifest | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }

  const manifest = value as Record<string, unknown>;
  const upstream = manifest.upstream;
  const askMatt = manifest.askMatt;
  const members = manifest.members;
  if (
    manifest.schemaVersion !== 1 ||
    typeof manifest.releaseContentSha256 !== "string" ||
    !sha256Pattern.test(manifest.releaseContentSha256) ||
    !upstream ||
    typeof upstream !== "object" ||
    Array.isArray(upstream) ||
    !askMatt ||
    typeof askMatt !== "object" ||
    Array.isArray(askMatt) ||
    !Array.isArray(members)
  ) {
    return undefined;
  }

  const upstreamRecord = upstream as Record<string, unknown>;
  const askMattRecord = askMatt as Record<string, unknown>;
  if (
    typeof upstreamRecord.repository !== "string" ||
    typeof upstreamRecord.commit !== "string" ||
    !/^[a-f0-9]{40}$/.test(upstreamRecord.commit) ||
    !isStringArray(upstreamRecord.roots) ||
    typeof upstreamRecord.fileCount !== "number" ||
    !Number.isSafeInteger(upstreamRecord.fileCount) ||
    typeof upstreamRecord.snapshotSha256 !== "string" ||
    !sha256Pattern.test(upstreamRecord.snapshotSha256) ||
    upstreamRecord.repository !== pinnedUpstream.repository ||
    upstreamRecord.commit !== pinnedUpstream.commit ||
    upstreamRecord.fileCount !== pinnedUpstream.fileCount ||
    upstreamRecord.snapshotSha256 !== pinnedUpstream.snapshotSha256 ||
    upstreamRecord.roots.length !== pinnedUpstream.roots.length ||
    upstreamRecord.roots.some(
      (root, index) => root !== pinnedUpstream.roots[index],
    ) ||
    typeof askMattRecord.path !== "string" ||
    !isStringArray(askMattRecord.builtInRoutes) ||
    !isStringArray(askMattRecord.optionalRoutes)
  ) {
    return undefined;
  }

  const parsedMembers: SkillCatalogMember[] = [];
  for (const member of members) {
    if (!member || typeof member !== "object" || Array.isArray(member)) {
      return undefined;
    }
    const record = member as Record<string, unknown>;
    if (
      typeof record.name !== "string" ||
      !skillNamePattern.test(record.name) ||
      typeof record.path !== "string"
    ) {
      return undefined;
    }
    parsedMembers.push({ name: record.name, path: record.path });
  }

  return {
    schemaVersion: 1,
    upstream: {
      repository: upstreamRecord.repository,
      commit: upstreamRecord.commit,
      roots: upstreamRecord.roots,
      fileCount: upstreamRecord.fileCount,
      snapshotSha256: upstreamRecord.snapshotSha256,
    },
    releaseContentSha256: manifest.releaseContentSha256,
    members: parsedMembers,
    askMatt: {
      path: askMattRecord.path,
      builtInRoutes: askMattRecord.builtInRoutes,
      optionalRoutes: askMattRecord.optionalRoutes,
    },
  };
}

function extractFrontmatterName(contents: Uint8Array): string | undefined {
  const text = Buffer.from(contents).toString("utf8");
  const frontmatter = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/.exec(text)?.[1];
  if (!frontmatter) {
    return undefined;
  }
  return /^name:\s*(?:"([^"]+)"|'([^']+)'|([^\r\n]+))\s*$/m.exec(
    frontmatter,
  )?.slice(1).find((value) => value !== undefined)?.trim();
}

function expectedNameFromPath(path: string): string | undefined {
  return /^skills\/(?:engineering|productivity)\/([^/]+)\/SKILL\.md$/.exec(
    path,
  )?.[1];
}

function digestFiles(files: readonly SkillCatalogFile[]): string {
  const digest = createHash("sha256");
  for (const file of [...files].sort((left, right) =>
    left.path < right.path ? -1 : left.path > right.path ? 1 : 0
  )) {
    digest.update(file.path);
    digest.update("\0");
    digest.update(file.contents);
    digest.update("\0");
  }
  return digest.digest("hex");
}

function extractAskMattRoutes(contents: Uint8Array): string[] {
  const text = Buffer.from(contents).toString("utf8");
  const inlineRoutes = [...text.matchAll(
    /`\/(?:skill:)?([a-z0-9]+(?:-[a-z0-9]+)*)`/g,
  )];
  const prose = text
    .replaceAll(/https?:\/\/[^\s)]+/g, "")
    .replaceAll(/`[^`]*`/g, "");
  const proseRoutes = [...prose.matchAll(
    /(?:^|[\s([{:>—])\/(?:skill:)?([a-z0-9]+(?:-[a-z0-9]+)*)/gm,
  )];
  return [
    ...new Set(
      [...inlineRoutes, ...proseRoutes]
        .map((match) => match[1])
        .filter((route): route is string => route !== undefined),
    ),
  ].sort();
}

function addIssue(
  issues: Set<SkillCatalogIssue>,
  issue: SkillCatalogIssue,
): void {
  issues.add(issue);
}

export function validateSkillCatalog(
  manifestValue: unknown,
  files: readonly SkillCatalogFile[],
): SkillCatalogValidation {
  const manifest = parseManifest(manifestValue);
  if (!manifest) {
    return {
      state: "invalid",
      memberNames: [],
      issues: ["invalid-manifest"],
    };
  }

  const issues = new Set<SkillCatalogIssue>();
  const expectedByPath = new Map(
    manifest.members.map((member) => [member.path, member.name]),
  );
  const expectedNames = manifest.members.map((member) => member.name).sort();
  if (manifest.members.length !== 22) {
    addIssue(issues, "wrong-member-count");
  }
  if (new Set(expectedNames).size !== expectedNames.length) {
    addIssue(issues, "duplicate-member");
  }

  const skillFiles = files.filter((file) => file.path.endsWith("/SKILL.md"));
  const actualPaths = new Set(skillFiles.map((file) => file.path));
  for (const expectedPath of expectedByPath.keys()) {
    if (!actualPaths.has(expectedPath)) {
      addIssue(issues, "missing-member");
    }
  }
  for (const actualPath of actualPaths) {
    if (!expectedByPath.has(actualPath)) {
      addIssue(issues, "extra-member");
    }
  }

  const actualNames: string[] = [];
  for (const file of skillFiles) {
    const name = extractFrontmatterName(file.contents);
    const pathName = expectedNameFromPath(file.path);
    if (name) {
      actualNames.push(name);
    }
    if (!name || !pathName || name !== pathName) {
      addIssue(issues, "name-path-mismatch");
      continue;
    }
    const expectedName = expectedByPath.get(file.path);
    if (expectedName !== undefined && expectedName !== name) {
      addIssue(issues, "name-path-mismatch");
    }
  }
  if (new Set(actualNames).size !== actualNames.length) {
    addIssue(issues, "duplicate-member");
  }

  if (files.length !== manifest.upstream.fileCount) {
    addIssue(issues, "wrong-file-count");
  }
  if (digestFiles(files) !== manifest.releaseContentSha256) {
    addIssue(issues, "content-digest-mismatch");
  }

  const askMatt = files.find((file) => file.path === manifest.askMatt.path);
  if (!askMatt) {
    addIssue(issues, "missing-member");
  } else {
    const allowedRoutes = new Set([
      ...expectedNames,
      ...manifest.askMatt.builtInRoutes,
      ...manifest.askMatt.optionalRoutes,
    ]);
    if (
      extractAskMattRoutes(askMatt.contents).some(
        (route) => !allowedRoutes.has(route),
      )
    ) {
      addIssue(issues, "invalid-ask-matt-route");
    }
  }

  return {
    state: issues.size === 0 ? "valid" : "invalid",
    memberNames: expectedNames,
    issues: [...issues].sort(),
    provenance: {
      repository: manifest.upstream.repository,
      commit: manifest.upstream.commit,
      snapshotSha256: manifest.upstream.snapshotSha256,
    },
  };
}
