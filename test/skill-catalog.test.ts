import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import { dirname, join, relative, resolve, sep } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  type SkillCatalogFile,
  validateSkillCatalog,
} from "../src/domain/skill-catalog.ts";

const repositoryRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "..",
);
const manifestPath = join(repositoryRoot, "skills", "catalog.json");
const packagePath = join(repositoryRoot, "package.json");
const catalogRoots = [
  join(repositoryRoot, "skills", "engineering"),
  join(repositoryRoot, "skills", "productivity"),
];

async function listFiles(root: string): Promise<SkillCatalogFile[]> {
  const files: SkillCatalogFile[] = [];
  async function visit(directory: string): Promise<void> {
    const entries = await readdir(directory, { withFileTypes: true });
    entries.sort((left, right) => left.name.localeCompare(right.name));
    for (const entry of entries) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) {
        await visit(path);
      } else if (entry.isFile()) {
        files.push({
          path: relative(repositoryRoot, path).split(sep).join("/"),
          contents: await readFile(path),
        });
      }
    }
  }
  await visit(root);
  return files;
}

async function loadCatalog(): Promise<{
  manifest: Record<string, unknown>;
  files: SkillCatalogFile[];
}> {
  return {
    manifest: JSON.parse(await readFile(manifestPath, "utf8")),
    files: (
      await Promise.all(
        catalogRoots.map(async (root) => await listFiles(root)),
      )
    ).flat(),
  };
}

function replaceContents(
  files: readonly SkillCatalogFile[],
  path: string,
  transform: (contents: string) => string,
): SkillCatalogFile[] {
  return files.map((file) =>
    file.path === path
      ? {
        ...file,
        contents: Buffer.from(
          transform(Buffer.from(file.contents).toString("utf8")),
        ),
      }
      : file
  );
}

test("the imported snapshot is one valid 22-skill Matty catalog", async () => {
  const { manifest, files } = await loadCatalog();
  const validation = validateSkillCatalog(manifest, files);

  assert.equal(files.length, 68);
  assert.equal(validation.state, "valid");
  assert.equal(validation.memberNames.length, 22);
  assert.deepEqual(validation.issues, []);
  assert.deepEqual(validation.provenance, {
    repository: "https://github.com/mattpocock/skills",
    commit: "2ab958093e83e0ec752e6c1c5932da465bf23e0c",
    snapshotSha256:
      "e5cf6d080ecfe4c3c197d2268704cb3bc42c00455703a8fe2be2f3b061bc6509",
  });
});

test("catalog validation rejects missing, extra, and duplicate members", async () => {
  const { manifest, files } = await loadCatalog();
  const missing = files.filter(
    (file) =>
      file.path !== "skills/engineering/code-review/SKILL.md",
  );
  const extra = [
    ...files,
    {
      path: "skills/engineering/not-approved/SKILL.md",
      contents: Buffer.from("---\nname: not-approved\n---\n"),
    },
  ];
  const duplicate = replaceContents(
    files,
    "skills/engineering/code-review/SKILL.md",
    (contents) => contents.replace("name: code-review", "name: ask-matt"),
  );

  assert.deepEqual(
    validateSkillCatalog(manifest, missing).issues,
    [
      "content-digest-mismatch",
      "missing-member",
      "wrong-file-count",
    ],
  );
  assert.deepEqual(
    validateSkillCatalog(manifest, extra).issues,
    [
      "content-digest-mismatch",
      "extra-member",
      "wrong-file-count",
    ],
  );
  assert.deepEqual(
    validateSkillCatalog(manifest, duplicate).issues,
    [
      "content-digest-mismatch",
      "duplicate-member",
      "name-path-mismatch",
    ],
  );
});

test("ask-matt cannot route to an undeclared capability", async () => {
  const { manifest, files } = await loadCatalog();
  for (const instruction of [
    "Use `/not-in-the-catalog`.",
    "Run /not-in-the-catalog now.",
  ]) {
    const invalidRouter = replaceContents(
      files,
      "skills/engineering/ask-matt/SKILL.md",
      (contents) => `${contents}\n${instruction}\n`,
    );

    assert.deepEqual(
      validateSkillCatalog(manifest, invalidRouter).issues,
      [
        "content-digest-mismatch",
        "invalid-ask-matt-route",
      ],
    );
  }
});

test("catalog provenance is closed over the reviewed upstream snapshot", async () => {
  const { manifest, files } = await loadCatalog();
  const altered = structuredClone(manifest);
  const upstream = altered.upstream as Record<string, unknown>;
  upstream.repository = "https://github.com/example/not-the-reviewed-source";

  assert.deepEqual(
    validateSkillCatalog(altered, files).issues,
    ["invalid-manifest"],
  );
});

test("a package filter that removes one member is never a valid catalog", async () => {
  const { manifest, files } = await loadCatalog();
  const individuallyFiltered = files.filter(
    (file) =>
      file.path !== "skills/productivity/teach/SKILL.md",
  );

  assert.equal(
    validateSkillCatalog(manifest, individuallyFiltered).state,
    "invalid",
  );
});

test("the package ships the staged catalog without activating it", async () => {
  const packageManifest = JSON.parse(
    await readFile(packagePath, "utf8"),
  ) as {
    files?: unknown;
    pi?: Record<string, unknown>;
  };

  assert.ok(
    Array.isArray(packageManifest.files) &&
      packageManifest.files.includes("skills"),
  );
  assert.deepEqual(packageManifest.pi, {
    extensions: ["./dist/adapters/pi-extension.js"],
  });
  assert.equal("skills" in (packageManifest.pi ?? {}), false);
});
