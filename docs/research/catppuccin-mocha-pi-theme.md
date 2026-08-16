# Catppuccin Mocha theme strategy for Matty

**Status basis:** Pi **0.84.2**, released **2026-08-14**; `ujj/pi-catppuccin` npm/GitHub state described below is dated **2026-02-13** (its last code publication). Sources are primary and links target exact files or release metadata.

## Recommendation

**Author and ship one Matty-owned JSON theme in Matty's existing Pi package.** Name it `matty-catppuccin-mocha` to avoid colliding with the generic `catppuccin-mocha` name.

Do **not** depend on, bundle, or vendor `ujj/pi-catppuccin` now. Its published Mocha JSON is invalid under Pi 0.84.2's strict schema, its semantic palette mapping has fidelity errors, and its very short maintenance history is not enough to justify another publisher/update boundary. A single static JSON file in Matty adds no new executable or transitive dependency and can evolve alongside the Pi version Matty supports.

If Matty does not want theming to be part of its product experience, the fallback is merely to document ujj as an **optional separate install after upstream fixes**—not to make it a Matty dependency.

## Findings

### Pi 0.84.2 compatibility

Pi discovers themes from package `themes/` directories or `pi.themes`, requires a unique slash-free name, and documents 51 required color tokens. `thinkingMax`, `scrollbarThumb`, `searchMatchBg`, and `searchMatchText` are optional with fallbacks ([v0.84.2 theme docs](https://github.com/earendil-works/pi/blob/v0.84.2/packages/coding-agent/docs/themes.md); [exact schema](https://github.com/earendil-works/pi/blob/v0.84.2/packages/coding-agent/src/modes/interactive/theme/theme-schema.json)).

Ujj's published file has every required color and may legally omit those four optional tokens, but its `export` object adds `text` and `muted` ([Mocha JSON at published commit](https://github.com/ujj/pi-catppuccin/blob/b261416db56e963933ce81f5818139b7410b9e67/themes/catppuccin-mocha.json)). Pi 0.84.2 permits only `pageBg`, `cardBg`, and `infoBg` and disallows additional properties. The runtime compiles that schema, throws on failed validation, ignores invalid themes during discovery, and falls back to `dark` after a failed selection/init ([v0.84.2 `theme.ts`](https://github.com/earendil-works/pi/blob/v0.84.2/packages/coding-agent/src/modes/interactive/theme/theme.ts)). **Therefore npm version 1.0.0 is not Pi 0.84.2-compatible as published.** Its `$schema` also tracks the old `badlogic/pi-mono/main` URL rather than a release-pinned schema.

### Installation, updates, and security exposure

A separate ujj install is easy (`pi install npm:@ujjwalgrover/pi-catppuccin`) and independently updateable. Unpinned npm packages update through `pi update --extensions`/`--all`; pinned versions are skipped. Trusted project settings can auto-install a missing package. However, that is a second install, update, publisher, and failure surface ([Pi package docs](https://github.com/earendil-works/pi/blob/v0.84.2/packages/coding-agent/docs/packages.md); [ujj README](https://github.com/ujj/pi-catppuccin/blob/b261416db56e963933ce81f5818139b7410b9e67/README.md)).

The current ujj package is unusually low-risk for npm: its manifest contains no scripts, dependencies, or executable entry—only four themes—and registry metadata reports seven files and no shrinkwrap ([package.json](https://github.com/ujj/pi-catppuccin/blob/b261416db56e963933ce81f5818139b7410b9e67/package.json); [npm registry metadata](https://registry.npmjs.org/@ujjwalgrover/pi-catppuccin)). Still, Pi warns that packages have full-system-access potential and runs npm installation; an unpinned future publication could add code. A Matty-owned static file introduces no additional trust principal or executable path.

Making ujj a normal Matty dependency is not clean reuse: Pi says other Pi packages must be bundled and explicitly referenced through `node_modules`. That is operationally vendoring with extra indirection ([dependency rules](https://github.com/earendil-works/pi/blob/v0.84.2/packages/coding-agent/docs/packages.md#dependencies)).

### Maintenance and release maturity

The repository was created on **2026-02-13** and has three unsigned commits, all within roughly ten minutes that day; npm has only version `1.0.0`, also published that day. There are no repository tags or GitHub releases ([commit history API](https://api.github.com/repos/ujj/pi-catppuccin/commits?per_page=20); [tags API](https://api.github.com/repos/ujj/pi-catppuccin/tags); [releases API](https://api.github.com/repos/ujj/pi-catppuccin/releases); [npm metadata](https://registry.npmjs.org/@ujjwalgrover/pi-catppuccin)). This is not evidence of abandonment, but it is too little maintenance history to offset the current compatibility defect.

### Visual fidelity

Ujj uses genuine Mocha hex values and gets Blue links, Green success, Red errors, and most syntax choices broadly right. But several semantic aliases are shifted: its `surface2` is official **Overlay 0**, `surface1` is official **Surface 2**, `surface0` is official **Surface 1**, and `surface` is official **Surface 0**. It omits distinct Subtext 0/1 and Overlay 1/2, maps warnings to Peach rather than the recommended Yellow, and its comment color resolves to Overlay 0 rather than recommended Overlay 2 ([ujj Mocha JSON](https://github.com/ujj/pi-catppuccin/blob/b261416db56e963933ce81f5818139b7410b9e67/themes/catppuccin-mocha.json); [official palette data](https://github.com/catppuccin/palette/blob/main/palette.json); [official style guide](https://github.com/catppuccin/catppuccin/blob/main/docs/style-guide.md)).

Official guidance maps Base to the main pane; Crust/Mantle to secondary panes; Surface 0–2 to surface elements; Text/Subtext/Overlay to descending text prominence; Blue/Green/Yellow/Red to links/success/warning/error; Rosewater to the cursor; Lavender/Overlay 0 to active/inactive borders. Selection uses Overlay 2 at 20–30% opacity. Pi accepts opaque six-digit RGB only, so Matty should use a documented opaque blend for selection/status backgrounds and prioritize legibility, as the style guide explicitly allows.

### Licensing and scope

Both works are MIT. Copying or modifying ujj's mapping requires retaining **Copyright (c) 2026 Ujjwal** and its permission notice ([ujj license](https://github.com/ujj/pi-catppuccin/blob/b261416db56e963933ce81f5818139b7410b9e67/LICENSE)). Independently authoring from the official palette avoids that fork obligation, but Matty should preserve/attribute **Copyright (c) 2021 Catppuccin** under Catppuccin's MIT terms ([Catppuccin license](https://github.com/catppuccin/catppuccin/blob/main/LICENSE)).

A four-flavor fork would broaden Matty into maintaining a general theme port. One Mocha file is the smallest cohesive scope if Mocha is part of Matty's intended Pi experience. Its coupling is explicit: Matty owns schema updates and releases the theme with the package, rather than silently inheriting upstream changes.

## Option comparison

| Option | Advantages | Decisive drawbacks | Verdict |
|---|---|---|---|
| Separate/reference ujj package | Independent ownership and updates; no copied source; current tarball has no code/dependencies | Extra install/trust/update boundary; 1.0.0 currently invalid on Pi 0.84.2; immature history; fidelity defects | Document only after upstream repair, if theming is out of Matty scope |
| Vendor/fork ujj | Matty can repair schema/fidelity and ship one install | Must retain ujj notice; owns a divergent four-theme fork; effectively duplicates a tiny package | Reject |
| Matty-owned JSON in existing package | One install/update; no new executable/transitive exposure; exact Pi support and product styling; minimal one-file scope | Matty owns token/schema testing and Catppuccin attribution | **Choose** |

## Implementation outline

1. Add `themes/matty-catppuccin-mocha.json` and expose it through the existing package's `pi.themes` entry (or its conventional `themes/` directory).
2. Build from the official Mocha palette with correctly named variables. Map `warning` to Yellow, links to Blue, success/error to Green/Red, normal/active borders to Overlay 0/Lavender, and syntax tokens per the official editor guidance.
3. Include all optional Pi 0.84.2 tokens explicitly. Restrict `export` to `pageBg`, `cardBg`, and `infoBg`. Use a canonical schema URL and a unique `name`.
4. Add Catppuccin's MIT notice to Matty's third-party notices/license documentation. Do not copy ujj's token mapping; then ujj attribution is not legally required (a README acknowledgment remains optional courtesy).
5. Validate against Pi 0.84.2's JSON schema and runtime loader; test theme discovery/selection, 256-color fallback, tool states/diffs, markdown/syntax, search, thinking levels, and HTML export. Manually check contrast on a terminal whose background is Mocha Base, because Pi themes do not set the terminal's main background.
6. Document `/settings` selection and the exact supported Pi baseline. Re-run schema validation whenever Matty raises its Pi compatibility floor.
