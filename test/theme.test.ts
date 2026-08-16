import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import test from "node:test";
import {
  loadThemeFromPath,
} from "../node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/theme/theme.js";
import { Compile } from "typebox/compile";

const root = resolve(import.meta.dirname, "..");
const themePath = join(root, "themes", "matty-catppuccin-mocha.json");
const schemaPath = join(
  root,
  "node_modules",
  "@earendil-works",
  "pi-coding-agent",
  "dist",
  "modes",
  "interactive",
  "theme",
  "theme-schema.json",
);

test("Matty Catppuccin Mocha satisfies the exact Pi 0.84.2 theme contract", async () => {
  const manifest = JSON.parse(await readFile(join(root, "package.json"), "utf8"));
  const themeJson = JSON.parse(await readFile(themePath, "utf8"));
  const schema = JSON.parse(await readFile(schemaPath, "utf8"));
  const validate = Compile(schema);

  assert.equal(
    manifest.peerDependencies["@earendil-works/pi-coding-agent"],
    "0.84.2",
  );
  assert.deepEqual(manifest.pi.themes, [
    "./themes/matty-catppuccin-mocha.json",
  ]);
  assert.equal(themeJson.name, "matty-catppuccin-mocha");
  assert.equal(
    validate.Check(themeJson),
    true,
    [...validate.Errors(themeJson)].join("\n"),
  );

  const supportedColorTokens = Object.keys(
    schema.properties.colors.properties,
  ).sort();
  assert.deepEqual(Object.keys(themeJson.colors).sort(), supportedColorTokens);
  assert.deepEqual(Object.keys(themeJson.export).sort(), [
    "cardBg",
    "infoBg",
    "pageBg",
  ]);

  for (const [name, value] of Object.entries(themeJson.vars)) {
    assert.match(value as string, /^#[0-9a-f]{6}$/i, `invalid variable ${name}`);
  }
  for (const [token, value] of Object.entries(themeJson.colors)) {
    assert.ok(
      value === "" ||
        typeof value === "number" ||
        (typeof value === "string" &&
          (value.startsWith("#") || value in themeJson.vars)),
      `unresolved color ${token}: ${String(value)}`,
    );
  }

  assert.doesNotThrow(() => loadThemeFromPath(themePath, "truecolor"));
  assert.doesNotThrow(() => loadThemeFromPath(themePath, "256color"));
});
