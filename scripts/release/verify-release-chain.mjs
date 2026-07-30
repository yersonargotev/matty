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
includes("description: One-time publish of @yargote/matty@0.1.0");
includes("permissions:\n  contents: read");
includes(
  "if: github.repository == 'yersonargotev/matty' && github.ref == 'refs/heads/main'",
);
includes("runs-on: macos-15");
includes("environment: release-candidate");
includes("node-version: 24");
includes("npm install --global @colbymchenry/codegraph@0.9.9");
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
  "if: github.repository == 'yersonargotev/matty' && github.ref == 'refs/heads/main' && inputs.stage && !inputs.bootstrap",
);
includes("needs: certify");
includes("environment: npm-stage");
includes("id-token: write");
includes("npm install --global npm@11.18.0");
includes("uses: actions/download-artifact@v4");
includes("shasum -a 256 -c SHA256SUMS");
includes('run: npm stage publish "${{ steps.verify.outputs.tarball }}"');

includes(
  "if: github.repository == 'yersonargotev/matty' && github.ref == 'refs/heads/main' && inputs.bootstrap && !inputs.stage",
);
includes("environment: npm-bootstrap");
includes("registry-url: https://registry.npmjs.org");
includes("package-manager-cache: false");
includes('if [[ "$identity" != "@yargote/matty@0.1.0" ]]');
includes("NODE_AUTH_TOKEN: ${{ secrets.NPM_BOOTSTRAP_TOKEN }}");
includes(
  'run: npm publish "${{ steps.verify-bootstrap.outputs.tarball }}" --provenance --access public',
);
assert.doesNotMatch(workflow, /\bNPM_TOKEN\b/, "generic npm token is forbidden");
assert.doesNotMatch(
  workflow,
  /^\s+(?:contents|packages|actions|attestations):\s*write\s*$/m,
  "only OIDC permissions may be writable",
);
assert.equal(
  workflow.match(/^\s*run:\s*npm stage publish .+$/gm)?.length,
  1,
  "the workflow must contain exactly one npm stage publish command",
);
assert.equal(
  workflow.match(/^\s*run:\s*npm publish .+$/gm)?.length,
  1,
  "the workflow must contain exactly one one-time npm publish command",
);
assert.equal(
  workflow.match(/^\s+NODE_AUTH_TOKEN:\s*\$\{\{ secrets\.NPM_BOOTSTRAP_TOKEN \}\}\s*$/gm)
    ?.length,
  1,
  "the bootstrap token must appear exactly once as a protected secret",
);

console.log("Release chain workflow invariants verified.");
