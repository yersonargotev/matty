import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { basename, join, resolve } from "node:path";

function requiredOption(name) {
  const index = process.argv.indexOf(name);
  const value = index >= 0 ? process.argv[index + 1] : undefined;
  assert.ok(value && !value.startsWith("--"), `${name} is required`);
  return value;
}

const artifactDirectory = resolve(requiredOption("--artifact-dir"));
const outputDirectory = resolve(requiredOption("--output-dir"));
const timeoutValue = requiredOption("--timeout-seconds");
assert.match(
  timeoutValue,
  /^\d+$/,
  "--timeout-seconds must be a positive integer",
);
const timeoutSeconds = Number(timeoutValue);
assert.ok(timeoutSeconds > 0, "--timeout-seconds must be a positive integer");

const certified = JSON.parse(
  await readFile(join(artifactDirectory, "RELEASE-EVIDENCE.json"), "utf8"),
);
const checksumLine = (
  await readFile(join(artifactDirectory, "SHA256SUMS"), "utf8")
).trim();
const checksumParts = checksumLine.split(/\s+/);
assert.equal(
  checksumParts.length,
  2,
  "SHA256SUMS must contain exactly one entry",
);
const [certifiedSha256, filename] = checksumParts;
assert.equal(certifiedSha256, certified.package.sha256);
assert.equal(filename, certified.package.filename);
assert.equal(certified.release.ref, `refs/tags/${certified.release.tag}`);
assert.equal(certified.release.tag, `v${certified.package.version}`);
assert.equal(certified.github.repository, "yersonargotev/matty");

await mkdir(outputDirectory, { recursive: true });
assert.deepEqual(
  await readdir(outputDirectory),
  [],
  "public evidence output directory must be empty",
);

const sleep = (milliseconds) =>
  new Promise((resolveSleep) => setTimeout(resolveSleep, milliseconds));
const packagePath = certified.package.name.replace("/", "%2f");
const versionUrl = `https://registry.npmjs.org/${packagePath}/${certified.package.version}`;
const deadline = Date.now() + timeoutSeconds * 1000;
let metadata;
while (Date.now() < deadline) {
  const response = await fetch(versionUrl, {
    headers: { accept: "application/json" },
  });
  if (response.ok) {
    metadata = await response.json();
    break;
  }
  if (response.status !== 404) {
    throw new Error(`npm registry returned ${response.status}`);
  }
  process.stdout.write(
    `${certified.package.name}@${certified.package.version} is not public yet; ` +
      "retrying in 30 seconds\n",
  );
  await sleep(30_000);
}
assert.ok(
  metadata,
  `public version did not appear within ${timeoutSeconds} seconds`,
);
assert.equal(metadata.name, certified.package.name);
assert.equal(metadata.version, certified.package.version);

const tarballResponse = await fetch(metadata.dist.tarball);
assert.ok(
  tarballResponse.ok,
  `public tarball download failed (${tarballResponse.status})`,
);
const tarball = Buffer.from(await tarballResponse.arrayBuffer());
const sha256 = createHash("sha256").update(tarball).digest("hex");
const shasum = createHash("sha1").update(tarball).digest("hex");
const sha512 = createHash("sha512").update(tarball).digest("hex");
assert.equal(
  sha256,
  certifiedSha256,
  "public tarball differs from certified artifact",
);
assert.equal(
  shasum,
  metadata.dist.shasum,
  "registry SHA-1 shasum is incorrect",
);
const [integrityAlgorithm, integrityDigest] = metadata.dist.integrity.split(
  "-",
  2,
);
assert.equal(
  integrityAlgorithm,
  "sha512",
  "registry integrity must use SHA-512",
);
assert.equal(
  createHash(integrityAlgorithm).update(tarball).digest("base64"),
  integrityDigest,
  "registry integrity is incorrect",
);

let attestationUrl;
let attestations;
let slsaAttestation;
while (Date.now() < deadline && !slsaAttestation) {
  attestationUrl = metadata.dist.attestations?.url;
  if (!attestationUrl) {
    const refreshed = await fetch(versionUrl, {
      headers: { accept: "application/json" },
    });
    assert.ok(
      refreshed.ok,
      `npm registry refresh failed (${refreshed.status})`,
    );
    metadata = await refreshed.json();
    attestationUrl = metadata.dist.attestations?.url;
  }
  if (attestationUrl) {
    const response = await fetch(attestationUrl, {
      headers: { accept: "application/json" },
    });
    if (response.ok) {
      attestations = await response.json();
      slsaAttestation = attestations.attestations?.find(
        (entry) => entry.predicateType === "https://slsa.dev/provenance/v1",
      );
    } else if (response.status !== 404) {
      throw new Error(`attestation download failed (${response.status})`);
    }
  }
  if (!slsaAttestation) {
    process.stdout.write(
      "npm provenance is not available yet; retrying in 30 seconds\n",
    );
    await sleep(30_000);
  }
}
assert.ok(
  slsaAttestation,
  "SLSA provenance v1 attestation did not become available",
);
const encodedPayload = slsaAttestation.bundle?.dsseEnvelope?.payload;
assert.ok(encodedPayload, "SLSA attestation has no DSSE payload");
const statement = JSON.parse(
  Buffer.from(encodedPayload, "base64").toString("utf8"),
);
assert.equal(statement.predicateType, "https://slsa.dev/provenance/v1");
const expectedSubject =
  `pkg:npm/${certified.package.name.replace("@", "%40")}@` +
  certified.package.version;
const subject = statement.subject?.find(
  (entry) => entry.name === expectedSubject,
);
assert.ok(subject, `provenance subject is not ${expectedSubject}`);
assert.equal(
  subject.digest?.sha512,
  sha512,
  "provenance subject digest is incorrect",
);

const predicate = statement.predicate;
assert.equal(
  predicate.runDetails?.builder?.id,
  "https://github.com/actions/runner/github-hosted",
  "provenance builder is not GitHub-hosted",
);
const workflow = predicate.buildDefinition?.externalParameters?.workflow;
assert.deepEqual(workflow, {
  ref: certified.release.ref,
  repository: `https://github.com/${certified.github.repository}`,
  path: "/.github/workflows/release-candidate.yml",
});
assert.equal(
  predicate.buildDefinition?.internalParameters?.github?.event_name,
  "push",
  "provenance event is not the release-tag push",
);
const expectedDependency = `git+https://github.com/${certified.github.repository}@${certified.release.ref}`;
const source = predicate.buildDefinition?.resolvedDependencies?.find(
  (entry) => entry.uri === expectedDependency,
);
assert.ok(source, `provenance source is not ${expectedDependency}`);
assert.equal(
  source.digest?.gitCommit,
  certified.release.commit,
  "provenance source commit differs from certification",
);
const expectedInvocation =
  `https://github.com/${certified.github.repository}/actions/runs/` +
  `${certified.github.runId}/attempts/${certified.github.runAttempt}`;
assert.equal(
  predicate.runDetails?.metadata?.invocationId,
  expectedInvocation,
  "provenance invocation differs from certification",
);

const publicTarball = join(outputDirectory, basename(metadata.dist.tarball));
await writeFile(publicTarball, tarball);
const evidence = {
  schemaVersion: 1,
  verifiedAt: new Date().toISOString(),
  certified,
  registry: {
    versionUrl,
    tarball: metadata.dist.tarball,
    sha256,
    shasum,
    integrity: metadata.dist.integrity,
    attestations: attestationUrl,
  },
  provenance: {
    subject: expectedSubject,
    subjectSha512: sha512,
    repository: workflow.repository,
    workflow: workflow.path,
    ref: workflow.ref,
    commit: source.digest.gitCommit,
    invocation: expectedInvocation,
    builder: predicate.runDetails.builder.id,
  },
};
await writeFile(
  join(outputDirectory, "PUBLIC-RELEASE-EVIDENCE.json"),
  `${JSON.stringify(evidence, null, 2)}\n`,
);
await writeFile(
  join(outputDirectory, "NPM-ATTESTATIONS.json"),
  `${JSON.stringify(attestations, null, 2)}\n`,
);
process.stdout.write(
  `Verified public release ${certified.package.name}@` +
    `${certified.package.version} (${sha256}).\n`,
);
