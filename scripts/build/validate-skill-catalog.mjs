import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const manifestPath = join(repositoryRoot, "skills", "catalog.json");
const skillsRoot = join(repositoryRoot, "skills");
const roots = [
  join(repositoryRoot, "skills", "engineering"),
  join(repositoryRoot, "skills", "productivity"),
];

async function listFiles(root) {
  const files = [];
  async function visit(directory) {
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
      } else {
        throw new Error(`Unsupported catalog entry: ${path}`);
      }
    }
  }
  await visit(root);
  return files;
}

const { validateSkillCatalog } = await import(
  pathToFileURL(
    join(repositoryRoot, "dist", "domain", "skill-catalog.js"),
  ).href
);
const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
const skillsEntries = await readdir(skillsRoot, { withFileTypes: true });
assert.deepEqual(
  skillsEntries
    .map((entry) => entry.name)
    .sort(),
  ["catalog.json", "engineering", "productivity"],
  "skills/ contains an undeclared top-level entry",
);
const files = (
  await Promise.all(roots.map(async (root) => await listFiles(root)))
).flat();
const validation = validateSkillCatalog(manifest, files);

assert.equal(
  validation.state,
  "valid",
  `Shared Skill Catalog is invalid: ${validation.issues.join(", ")}`,
);
assert.equal(validation.memberNames.length, 22);

process.stdout.write(
  `Shared Skill Catalog valid: ${validation.memberNames.length} skills, ${files.length} files\n`,
);
