import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const workflow = await readFile(
  join(repositoryRoot, ".github/workflows/release-candidate.yml"),
  "utf8",
);
const manifest = JSON.parse(
  await readFile(join(repositoryRoot, "package.json"), "utf8"),
);
const trustedPublisher = JSON.parse(
  await readFile(
    join(repositoryRoot, "release/trusted-publisher.json"),
    "utf8",
  ),
);

assert.equal(manifest.name, "@yargote/matty");
assert.equal(manifest.license, "MIT");
assert.deepEqual(manifest.repository, {
  type: "git",
  url: "git+https://github.com/yersonargotev/matty.git",
});
assert.equal(manifest.homepage, "https://github.com/yersonargotev/matty#readme");
assert.deepEqual(manifest.bugs, {
  url: "https://github.com/yersonargotev/matty/issues",
});
assert.deepEqual(manifest.publishConfig, {
  access: "public",
  provenance: true,
});
assert.deepEqual(trustedPublisher, {
  schemaVersion: 1,
  provider: "github",
  package: "@yargote/matty",
  repository: "yersonargotev/matty",
  workflowFile: "release-candidate.yml",
  environment: "npm-stage",
  allowedActions: ["stage-publish"],
  activationIssue: 17,
});

function includes(fragment, message = `missing workflow invariant: ${fragment}`) {
  assert.ok(workflow.includes(fragment), message);
}

includes("workflow_dispatch:");
includes("type: boolean");
includes("permissions:\n  contents: read");
includes(
  "if: github.repository == 'yersonargotev/matty' && github.ref == 'refs/heads/main'",
);
includes("runs-on: macos-15");
includes("environment: release-candidate");
includes("node-version: 24");
includes("npm ci --ignore-scripts");
assert.doesNotMatch(
  workflow,
  /^\s+cache:\s*npm\s*$/m,
  "release workflow must not enable the npm package cache",
);
includes("PI_AUTH_JSON: ${{ secrets.PI_AUTH_JSON }}");
includes("MATTY_REFERENCE_AUTH_PATH: ${{ runner.temp }}/matty-reference-auth.json");
includes('install -m 600 /dev/null "$MATTY_REFERENCE_AUTH_PATH"');
includes('npm run certify:candidate -- --output-dir "$CANDIDATE_OUTPUT_DIR"');
assert.match(
  workflow,
  /^\s*npm run certify:candidate -- --output-dir "\$CANDIDATE_OUTPUT_DIR"\s*$/m,
  "candidate certification must run in full with only its explicit output directory",
);
includes('checksum="$CANDIDATE_OUTPUT_DIR/SHA256SUMS"');
includes('(cd "$CANDIDATE_OUTPUT_DIR" && shasum -a 256 -c SHA256SUMS)');
includes("uses: actions/upload-artifact@v4");
includes("${{ steps.certify.outputs.tarball }}");
includes("${{ steps.certify.outputs.checksum }}");

includes(
  "if: github.repository == 'yersonargotev/matty' && github.ref == 'refs/heads/main' && inputs.stage",
);
includes("needs: certify");
includes("environment: npm-stage");
includes("id-token: write");
includes("npm install --global npm@11.18.0");
includes("uses: actions/download-artifact@v4");
includes("shasum -a 256 -c SHA256SUMS");
includes('run: npm stage publish "${{ steps.verify.outputs.tarball }}"');

assert.doesNotMatch(workflow, /\bnpm\s+publish\b/, "npm publish is forbidden");
assert.doesNotMatch(
  workflow,
  /\b(?:NODE_AUTH_TOKEN|NPM_TOKEN)\b/,
  "npm write credentials are forbidden",
);
assert.doesNotMatch(
  workflow,
  /^\s+(?:contents|packages|actions|attestations):\s*write\s*$/m,
  "only the staging OIDC permission may be writable",
);
assert.equal(
  workflow.match(/^\s*run:\s*npm stage publish .+$/gm)?.length,
  1,
  "the workflow must contain exactly one npm stage publish command",
);

console.log("Release chain workflow invariants verified.");
