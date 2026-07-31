import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const tag = process.env.GITHUB_REF_NAME ?? process.argv[2];
assert.match(
  tag ?? "",
  /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/,
  "release tag must be strict vX.Y.Z",
);
const version = tag.slice(1);

const manifest = JSON.parse(await readFile(join(root, "package.json"), "utf8"));
const lock = JSON.parse(
  await readFile(join(root, "package-lock.json"), "utf8"),
);
assert.equal(manifest.name, "@yargote/matty");
assert.equal(manifest.version, version, "tag and package.json versions differ");
assert.equal(
  lock.version,
  version,
  "tag and package-lock.json root versions differ",
);
assert.equal(
  lock.packages?.[""]?.version,
  version,
  "tag and package-lock root package versions differ",
);

if (process.env.GITHUB_EVENT_PATH) {
  const event = JSON.parse(
    await readFile(process.env.GITHUB_EVENT_PATH, "utf8"),
  );
  assert.equal(
    event.ref,
    `refs/tags/${tag}`,
    "event must contain exactly the current tag ref",
  );
  assert.equal(
    event.created,
    true,
    "release tags must be newly and individually pushed, not moved",
  );
  assert.equal(
    event.deleted,
    false,
    "release tag deletion is not a release trigger",
  );
}

function run(command, args, env) {
  return new Promise((resolveRun, reject) => {
    const child = spawn(command, args, {
      cwd: root,
      env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.stderr.on("data", (chunk) => (stderr += chunk));
    child.once("error", reject);
    child.once("close", (code) =>
      code === 0
        ? resolveRun(stdout.trim())
        : reject(
            new Error(`${command} ${args.join(" ")} failed: ${stderr.trim()}`),
          ),
    );
  });
}

const sandbox = await mkdtemp(join(tmpdir(), "matty-tag-validation-"));
const env = {
  ...process.env,
  HOME: join(sandbox, "home"),
  XDG_CONFIG_HOME: join(sandbox, "home", ".config"),
  TMPDIR: join(sandbox, "tmp"),
  npm_config_cache: join(sandbox, "npm-cache"),
  npm_config_userconfig: join(sandbox, "home", ".npmrc"),
};
delete env.PI_AUTH_JSON;
delete env.MATTY_REFERENCE_AUTH_PATH;
try {
  await Promise.all(
    [env.HOME, env.XDG_CONFIG_HOME, env.TMPDIR, env.npm_config_cache].map(
      (path) => mkdir(path, { recursive: true }),
    ),
  );
  if (process.env.GITHUB_ACTIONS === "true") {
    assert.equal(
      process.env.GITHUB_REF_PROTECTED,
      "true",
      "release tag must be covered by an active GitHub ruleset",
    );
    await run(
      "git",
      [
        "fetch",
        "--no-tags",
        "origin",
        "+refs/heads/main:refs/remotes/origin/main",
      ],
      env,
    );
    assert.equal(
      await run("git", ["cat-file", "-t", `refs/tags/${tag}`], env),
      "commit",
      "release tag must be lightweight so its target identity is unambiguous",
    );
    const tagCommit = await run("git", ["rev-parse", `refs/tags/${tag}`], env);
    assert.equal(
      tagCommit,
      process.env.GITHUB_SHA,
      "tag must resolve to this workflow's source commit",
    );
    await run(
      "git",
      ["merge-base", "--is-ancestor", tagCommit, "refs/remotes/origin/main"],
      env,
    );
  }

  const encodedName = manifest.name.replace("/", "%2f");
  const response = await fetch(
    `https://registry.npmjs.org/${encodedName}/${version}`,
    { headers: { accept: "application/json" } },
  );
  assert.equal(
    response.status,
    404,
    `${manifest.name}@${version} is already public or registry validation failed (${response.status})`,
  );
  if (process.env.GITHUB_OUTPUT) {
    await import("node:fs/promises").then(({ appendFile }) =>
      appendFile(
        process.env.GITHUB_OUTPUT,
        `version=${version}\npackage=${manifest.name}\n`,
      ),
    );
  }
  console.log(
    `Validated new release ${tag} (${manifest.name}@${version}) on origin/main.`,
  );
} finally {
  await rm(sandbox, { recursive: true, force: true });
}
