import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const workflow = await readFile(
  join(root, ".github/workflows/release-candidate.yml"),
  "utf8",
);
const certifier = await readFile(
  join(root, "scripts/release/certify-candidate.mjs"),
  "utf8",
);
const tagValidator = await readFile(
  join(root, "scripts/release/validate-release-tag.mjs"),
  "utf8",
);
const publicVerifier = await readFile(
  join(root, "scripts/release/verify-public-release.mjs"),
  "utf8",
);
const manifest = JSON.parse(await readFile(join(root, "package.json"), "utf8"));
const trustedPublisher = JSON.parse(
  await readFile(join(root, "release/trusted-publisher.json"), "utf8"),
);

assert.equal(manifest.name, "@yargote/matty");
assert.equal(manifest.license, "MIT");
assert.deepEqual(manifest.repository, {
  type: "git",
  url: "git+https://github.com/yersonargotev/matty.git",
});
assert.equal(
  manifest.homepage,
  "https://github.com/yersonargotev/matty#readme",
);
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

const includes = (fragment) =>
  assert.ok(
    workflow.includes(fragment),
    `missing workflow invariant: ${fragment}`,
  );
assert.match(workflow, /^on:\n  push:\n    tags:\n      - ["']v\*["']$/m);
assert.doesNotMatch(workflow, /workflow_dispatch:/);
includes("group: release-${{ github.ref }}");
includes("node scripts/release/validate-release-tag.mjs");
includes("environment: release-candidate");
includes("environment: npm-stage");
includes("RELEASE-EVIDENCE.json");
includes(
  'npm stage publish "${{ steps.verify.outputs.tarball }}" --tag latest',
);
includes("npm stage list @yargote/matty");
includes("npm stage view <stage-id>");
includes("npm stage download <stage-id>");
includes("npm stage approve <stage-id>");
includes("NPM-STAGE-EVIDENCE.json");
includes("stage-id: ${{ steps.publish.outputs.stage-id }}");
includes("--timeout-seconds 21600");
includes("node scripts/release/verify-public-release.mjs");
includes("public-release-evidence");
includes('gh release create "$TAG" "$ARTIFACT_DIR"/*');
includes("--verify-tag --draft");
includes("npm audit signatures");
includes('gh release upload "$TAG" "$EVIDENCE_DIR"/* --clobber');
includes('gh release edit "$TAG" --draft=false --latest');
assert.equal(
  workflow.match(/gh api "repos\/\$REPOSITORY\/git\/ref\/tags\/\$TAG"/g)
    ?.length,
  2,
  "the live tag target must be checked before draft creation and publication",
);

for (const pin of [
  "actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803",
  "actions/setup-node@249970729cb0ef3589644e2896645e5dc5ba9c38",
  "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
  "actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093",
])
  includes(pin);
assert.doesNotMatch(
  workflow,
  /uses:\s+actions\/(?:checkout|setup-node|upload-artifact|download-artifact)@v\d/,
);
assert.doesNotMatch(
  workflow,
  /\bnpm\s+publish\b/,
  "direct npm publish is forbidden",
);
assert.doesNotMatch(
  workflow,
  /\b(?:NODE_AUTH_TOKEN|NPM_TOKEN)\b/,
  "npm write credentials are forbidden",
);
assert.equal(
  workflow.match(/^\s+npm stage publish .+$/gm)?.length,
  1,
  "exactly one stage publish is required",
);
assert.equal(
  workflow.match(/^\s+id-token:\s*write\s*$/gm)?.length,
  1,
  "OIDC must be isolated to the stage job",
);
assert.equal(
  workflow.match(/^\s+environment:\s*release-candidate\s*$/gm)?.length,
  1,
  "certification has one protected approval",
);
assert.equal(
  workflow.match(/^\s+environment:\s*npm-stage\s*$/gm)?.length,
  1,
  "trusted publisher identity must remain exact",
);
assert.equal(
  workflow.match(/^\s+contents:\s*write\s*$/gm)?.length,
  2,
  "only draft and final verification jobs may write releases",
);

assert.match(
  certifier,
  /delete process\.env\.PI_AUTH_JSON|PI_AUTH_JSON: _secret/,
);
assert.match(certifier, /--reference-auth-stdin/);
assert.doesNotMatch(workflow, /--reference-auth-path/);
assert.match(certifier, /writeFile\(referenceAuthPath, referenceAuth/);
assert.match(certifier, /scripts\/acceptance\/t04-reference-web\.mjs/);
assert.match(certifier, /RELEASE-EVIDENCE\.json/);
assert.doesNotMatch(
  certifier,
  /assert\.equal\(metadata\.version,\s*"\d+\.\d+\.\d+"/,
);

assert.match(tagValidator, /GITHUB_REF_PROTECTED/);
assert.match(tagValidator, /cat-file", "-t"/);
assert.match(tagValidator, /merge-base", "--is-ancestor"/);
assert.match(tagValidator, /lock\.packages\?\.\[""\]\?\.version/);
assert.match(publicVerifier, /https:\/\/slsa\.dev\/provenance\/v1/);
assert.match(publicVerifier, /subject\.digest\?\.sha512/);
assert.match(publicVerifier, /source\.digest\?\.gitCommit/);
assert.match(publicVerifier, /invocationId/);

console.log("Tag-triggered staged release-chain invariants verified.");
