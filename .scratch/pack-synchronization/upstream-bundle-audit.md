# Upstream audit: Matty's `mattpocock/skills` bundle

Research date: 2026-07-13 (America/Bogota)

## Question and conclusion

This audit answers what release, layout, and provenance contract `mattpocock/skills` exposes, and what differs byte-for-byte between Matty's selected bundle resources and the latest stable upstream release.

The latest stable upstream release is **`v1.1.0`**, an annotated tag whose tag object is `eabea89380927aadb93abf6e290a19334d249292` and whose immutable target commit is **`d574778f94cf620fcc8ce741584093bc650a61d3`**. Matty selects 23 upstream skill directories. Every selected directory still exists at that commit, and there are no selected file additions or removals relative to it. Of 45 selected files, 40 are byte-identical and 5 are locally modified; the five changes span three selected resources. Matty's lock file describes only `wayfinder`, pins no tag or commit, hashes only its `SKILL.md`, and its recorded hash matches upstream `v1.1.0` rather than the locally bundled bytes.

## Method

I used first-party Git and GitHub data rather than inferred release names:

- queried GitHub Releases and Tags with `gh api repos/mattpocock/skills/releases` and `gh api repos/mattpocock/skills/tags`;
- checked advertised tag refs with `git ls-remote --tags https://github.com/mattpocock/skills.git`;
- cloned the upstream repository, force-fetched tags, and peeled each annotated tag with `git rev-parse '<tag>^{}'`;
- exported `skills/` from `v1.1.0` with `git archive`, compared every selected directory with `diff -qr`, and computed SHA-256 over the exact file bytes with `shasum -a 256`;
- inspected path changes with `git diff --find-renames --name-status`; and
- searched the Matty repository for consumers of `skills-lock.json` and its field names using `rg`.

All upstream file citations below are pinned to the peeled `v1.1.0` commit. Matty citations are pinned to repository commit `68aec8969374fa9e9a6ea86b33e6719646b999f8`.

## Upstream release contract

### Published history

GitHub currently exposes four non-draft, non-prerelease releases ([GitHub Releases API](https://api.github.com/repos/mattpocock/skills/releases)):

| Release | Published (UTC) | Annotated tag object | Peeled commit |
| --- | --- | --- | --- |
| `mattpocock-skills@1.0.0` | 2026-06-17 14:45:43 | `ad9690ac3e219cdf1cee50c2f58740507f7c0d8e` | `00ff03cac21a8a845b06256d826d183122f58c5e` |
| `v1.0.0` | 2026-06-17 14:46:57 | `dcfc2322f2f978113b1ec2dbbf50c00eda824519` | `bddb833cbaa322ff89d07e490530860aa73a4293` |
| `v1.0.1` | 2026-06-17 22:07:52 | `385c6009a7f7b9295b8ab9f7e3e78cbe67fa1296` | `2454c95dc305c158b21a0cdafeb728879dd0359a` |
| `v1.1.0` | 2026-07-08 13:20:57 | `eabea89380927aadb93abf6e290a19334d249292` | `d574778f94cf620fcc8ce741584093bc650a61d3` |

The duplicate-looking initial `mattpocock-skills@1.0.0` and `v1.0.0` names point to different commits. A synchronizer therefore cannot safely normalize or guess tag names from the package version; it must retain the exact selected tag and the peeled commit.

The repository is a private npm package (`"private": true`) whose `package.json` version is `1.1.0`; Changesets is configured to version and tag private packages. The push-to-`main` release workflow runs `changesets/action` with `publish: npx changeset tag`, and grants `contents: write` plus `pull-requests: write` ([`package.json`](https://github.com/mattpocock/skills/blob/d574778f94cf620fcc8ce741584093bc650a61d3/package.json), [Changesets config](https://github.com/mattpocock/skills/blob/d574778f94cf620fcc8ce741584093bc650a61d3/.changeset/config.json), [release workflow](https://github.com/mattpocock/skills/blob/d574778f94cf620fcc8ce741584093bc650a61d3/.github/workflows/release.yml)). The dependable acquisition boundary is thus the Git tag/commit and repository tree, not an npm artifact.

GitHub's release objects report `target_commitish: main`; the immutable identity comes from peeling the actual tag, not from that mutable release field. All four tags are annotated tag objects. The tag objects inspected locally contain no embedded `gpgsig` block; trust/authenticity policy remains a separate decision.

### Stable and unstable layout signals

At `v1.1.0`, upstream has 17 directories under `skills/engineering`, 5 under `skills/productivity`, and 6 under `skills/in-progress`. Upstream explicitly says in-progress skills are not ready to ship, may break or be abandoned, and are excluded from the plugin until graduation ([in-progress README](https://github.com/mattpocock/skills/blob/d574778f94cf620fcc8ce741584093bc650a61d3/skills/in-progress/README.md)).

The strongest machine-readable promoted set is `.claude-plugin/plugin.json`, which allowlists 21 skill directories ([plugin manifest](https://github.com/mattpocock/skills/blob/d574778f94cf620fcc8ce741584093bc650a61d3/.claude-plugin/plugin.json)). Directory membership alone is not equivalent to that promoted set: `skills/engineering/resolving-merge-conflicts` exists in the engineering bucket but is absent from the `v1.1.0` plugin manifest, while every other engineering and productivity directory is listed. Upstream fixed that wiring only after `v1.1.0`, in commit [`d327545`](https://github.com/mattpocock/skills/commit/d3275456a453ae93260abb9c945af60c259cdc6d).

Consequently, later selection-schema work must distinguish at least:

1. explicit Matty allowlisting;
2. upstream directory discovery;
3. upstream's promoted/plugin set; and
4. upstream's explicitly unstable buckets.

There is no per-skill version or manifest in the selected directories beyond the skill files themselves. Release versioning is repository-wide: one root package version/tag covers the entire tree.

## Matty's selected paths

Matty's pack manifest explicitly selects 23 skill resources ([`bundle/packs/matty/pack.json`](https://github.com/yersonargotev/matty/blob/68aec8969374fa9e9a6ea86b33e6719646b999f8/bundle/packs/matty/pack.json)):

- **Engineering (17):** `ask-matt`, `code-review`, `codebase-design`, `diagnosing-bugs`, `domain-modeling`, `grill-with-docs`, `implement`, `improve-codebase-architecture`, `prototype`, `research`, `resolving-merge-conflicts`, `setup-matt-pocock-skills`, `tdd`, `to-spec`, `to-tickets`, `triage`, and `wayfinder`.
- **Productivity (5):** `grill-me`, `grilling`, `handoff`, `teach`, and `writing-great-skills`.
- **In progress (1):** `loop-me`.

This equals every `v1.1.0` engineering and productivity directory plus `in-progress/loop-me`. Relative to the 21-entry upstream plugin manifest, Matty additionally selects `engineering/resolving-merge-conflicts` and unstable `in-progress/loop-me`.

The pack also selects one Matty-owned instruction resource, `instructions/matty-guidance.md`; it is not part of this upstream comparison. The bundle contains category `README.md` files under `bundle/skills/engineering` and `bundle/skills/productivity`, but those files are outside every selected resource directory and are not separately declared in the pack manifest.

## Additions, moves, and removals

### From the previous stable release to `v1.1.0`

The upstream name-status diff from `v1.0.1` to `v1.1.0` records the layout transition directly ([GitHub compare](https://github.com/mattpocock/skills/compare/v1.0.1...v1.1.0)):

- **Added selected paths:** `engineering/research`, `engineering/to-tickets`, `engineering/wayfinder`, and `in-progress/loop-me`.
- **Moved/renamed into a selected path:** `in-progress/review` → `engineering/code-review` (Git detected a 54% similarity rename) and `engineering/to-prd` → `engineering/to-spec` (78% similarity).
- **Removed/replaced:** `engineering/to-issues` was deleted and replaced functionally by `to-tickets`; `in-progress/decision-mapping` was deleted while the reframed/promoted `engineering/wayfinder` was added; `engineering/tdd/refactoring.md` was deleted.
- **Added but not selected by Matty:** `in-progress/claude-handoff` and `in-progress/wizard` (including `wizard/template.sh`).

The upstream `v1.1.0` changelog declares the same promotions and replacements, including `review` → `code-review`, `to-prd` → `to-spec`, `to-issues` → `to-tickets`, and `decision-mapping` → `wayfinder` ([changelog](https://github.com/mattpocock/skills/blob/d574778f94cf620fcc8ce741584093bc650a61d3/CHANGELOG.md)).

### Current bundle versus `v1.1.0`

There are **no directory or file additions, moves, or removals within the 23 selected resource trees**. All 23 expected directories exist on both sides and contain the same 45 relative file paths. Drift is content-only in five files.

### Unreleased upstream `main`

For forward-looking discovery only, upstream `main` was `66898f60e8c744e269f8ce06c2b2b99ce7660d5f` at audit time, while root `package.json` and `CHANGELOG.md` still reported `1.1.0`. Relative to the stable tag, `main` adds `agents/openai.yaml` to every selected skill, changes selected skill content, and adds the unselected `in-progress/setup-ts-deep-modules` directory. These are unreleased changes, not a newer stable version ([stable-to-main compare at observed head](https://github.com/mattpocock/skills/compare/d574778f94cf620fcc8ce741584093bc650a61d3...66898f60e8c744e269f8ce06c2b2b99ce7660d5f)). A stable-release synchronizer should report such discoveries separately but must not silently treat mutable `main` as the next release.

## Byte-level drift from `v1.1.0`

The comparison covers the complete contents of all 23 selected directories: **45 files total, 40 exact, 5 different, 0 added, 0 removed**. Three resources drift (`setup-matt-pocock-skills`, `to-tickets`, and `wayfinder`); the other 20 resources are entirely byte-identical.

| Relative file | Upstream SHA-256 | Matty SHA-256 | Line delta | Material local change |
| --- | --- | --- | ---: | --- |
| `skills/engineering/setup-matt-pocock-skills/issue-tracker-github.md` | `52e9f9f1ec0f6d47c6785ac500708ac0521f34abb4cc5475b5dc747165dab17e` | `273c1d57c36426d80df362b20288d8575d3d9ba5aeb7334bd4c966cd73eae863` | `+1/-1` | “Issues and PRDs” → “Specs and tickets” |
| `skills/engineering/setup-matt-pocock-skills/issue-tracker-gitlab.md` | `4470f2f64d015fba01af877233296b401bb0d44b5acb9572ffc3ff6b30e8de88` | `07fc81c40f529e5574f1d9cc20bb6196679efe910c66ffc3217d4ed65031c1e3` | `+1/-1` | “Issues and PRDs” → “Specs and tickets” |
| `skills/engineering/setup-matt-pocock-skills/issue-tracker-local.md` | `e0dd9835e3658909132058e9a5fdd851e972220bbadaf78a57e3c6220d470922` | `1ad2ce603d6bdb7b8db99417a8e1b4b8a32837369a87d525fec696d12322fcad` | `+9/-9` | renames PRD/issues layout to `spec.md`/tickets, changes local paths and map headings |
| `skills/engineering/to-tickets/SKILL.md` | `918bdefab9313100cb1f7ccb412e2a773fe2f2801dd20d44f6b2acf7a42ca456` | `87376aa0b0e0f2e5bcfe51d8686b27637bfe0a42ba6430943602dc3f14c20823` | `+1/-1` | local output moves from root `tickets.md` to `.scratch/<feature-slug>/tickets.md` |
| `skills/engineering/wayfinder/SKILL.md` | `bef437de697fb6984a8a90b7fd82f128609148d6e02f635ce419d03555b351e1` | `6ea93ad3760821ca8f21e8d5f2c7eeb2a812eb8c4d7ed6349494787fc5e7f522` | `+5/-5` | generalizes map identity, labels, claiming, and blocking so local Markdown is first-class |

The combined textual delta is `+17/-17` lines. These are deliberate Matty-local semantics, not formatting-only drift. Under the planned byte-identical vendoring rule they cannot remain inside synchronized resource directories; the later design must either adopt upstream behavior, move Matty-specific behavior to a separately owned instruction/configuration seam, or explicitly reject the update. A generic patch mechanism was already ruled outside the initial destination.

## `skills-lock.json` audit

The current lock is version 1 and contains exactly one entry, `wayfinder` ([`skills-lock.json`](https://github.com/yersonargotev/matty/blob/68aec8969374fa9e9a6ea86b33e6719646b999f8/skills-lock.json)). Its fields are:

- `source: "mattpocock/skills"`
- `sourceType: "github"`
- `skillPath: "skills/engineering/wayfinder/SKILL.md"`
- `computedHash: "bef437de...b351e1"`

The gaps are concrete:

1. **Coverage:** 1 of 23 selected upstream resources is represented; 22 have no provenance record.
2. **Immutable identity:** no readable tag, annotated tag object, peeled commit, branch, or fetch timestamp is recorded.
3. **Incomplete content coverage:** the hash covers one `SKILL.md`, not a resource directory/file manifest; it could not detect changes to companion files.
4. **Stale relative to installed bytes:** the recorded hash is exactly the SHA-256 of upstream `v1.1.0`'s `wayfinder/SKILL.md`, but Matty's bundled file hashes to `6ea93a...f522`. The lock therefore does not describe the current vendored bytes.
5. **No lifecycle state:** it cannot record discovered-but-unselected paths, upstream moves/removals, local drift disposition, or the one-source/many-resource atomic update boundary.
6. **No enforcement:** a repository-wide search finds no code, tests, workflow, or script reading `skills-lock.json` or the field names `computedHash`, `sourceType`, or `skillPath`. It is informational metadata today, not a verified lock.

## Facts that constrain later schema and versioning decisions

- **Pin both tag and peeled commit.** Upstream publishes annotated tags, GitHub release metadata says only `main`, and the initial 1.0.0 names are ambiguous. Store the exact tag plus its immutable peeled commit.
- **Version per source, update atomically.** Upstream has one repository-wide root version and release; selected skills do not version independently. All resources selected from this source should resolve to the same commit in a generated lock/update.
- **Selection cannot be inferred from one upstream signal.** Folder membership, the plugin manifest, and stability buckets disagree at `v1.1.0`. Keep Matty's explicit allowlist canonical, and report upstream discoveries/moves/removals for human resolution.
- **Record every file in every selected directory.** Selected skills may contain companion Markdown today and `agents/openai.yaml` tomorrow. Hashing only `SKILL.md` is insufficient.
- **Treat moves as identity questions, not automatic adds/deletes.** The `v1.1.0` transition contains real renames/promotions with partial Git similarity and semantic replacements that Git reports as delete/add. Stable resource IDs and explicit path mappings are both required.
- **Separate stable-release and branch discovery.** Unreleased `main` can change while still claiming package version `1.1.0`; tag detection and mutable-branch observation are different states.
- **Local modifications are a blocking invariant violation.** Five current files prove that a byte-identical policy needs deterministic pre-update drift detection and a clear, non-patching resolution path.
- **The lock should be generated and enforced.** It needs source-level release/commit identity plus complete per-resource/per-file paths and hashes, and validation must actually consume it.

## Reproduction snapshot

At the time of the audit:

```text
latest stable tag:        v1.1.0
annotated tag object:     eabea89380927aadb93abf6e290a19334d249292
peeled release commit:    d574778f94cf620fcc8ce741584093bc650a61d3
observed upstream main:   66898f60e8c744e269f8ce06c2b2b99ce7660d5f
Matty commit audited:     68aec8969374fa9e9a6ea86b33e6719646b999f8
selected resources:       23
selected files:           45
byte-identical files:     40
modified files:           5
added/removed files:      0 / 0
lock entries:             1
```
