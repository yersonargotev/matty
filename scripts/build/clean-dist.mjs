import { copyFile, mkdir, rm } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const distRoot = join(repositoryRoot, "dist");

if (dirname(distRoot) !== repositoryRoot || distRoot === repositoryRoot) {
  throw new Error(`Refusing to clean unexpected build path: ${distRoot}`);
}

await rm(distRoot, { recursive: true, force: true });
await mkdir(join(distRoot, "domain"), { recursive: true });
await copyFile(
  join(repositoryRoot, "src/domain/issue-delivery-guidance-v1.md"),
  join(distRoot, "domain/issue-delivery-guidance-v1.md"),
);
