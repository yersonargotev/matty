import { createHash } from "node:crypto";
import { realpath } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import {
  createChildPiRunner,
  type DelegatedTaskRunner,
} from "../../src/application/child-pi-runtime.ts";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const fixture = resolve(repositoryRoot, "test/fixtures/child-pi-rpc-fixture.mjs");
const canonicalRoot = await realpath(repositoryRoot);
const authenticationMarker = "role-seam-auth-marker";

export const roleSeamAuthenticationDigest = createHash("sha256")
  .update(authenticationMarker)
  .digest("hex");

export function createRoleSeamChildRunner(): DelegatedTaskRunner {
  return createChildPiRunner({
    invocation: { command: process.execPath, arguments: [fixture] },
    parent: {
      provider: "controlled-provider",
      model: "controlled-model",
      thinking: "high",
      cwd: canonicalRoot,
    },
    authentication: {
      provider: "controlled-provider",
      environment: {
        PATH: process.env.PATH,
        MATTY_TEST_AUTH: authenticationMarker,
      },
    },
    terminationGraceMs: 1_000,
  });
}
