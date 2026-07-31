# Tag-triggered staged npm release automation

## Scope and conclusion

A supported design is:

1. An authorized maintainer pushes a protected release tag.
2. A tag-filtered GitHub Actions workflow builds/tests, creates a **draft** GitHub Release, and runs `npm stage publish` using npm trusted publishing (OIDC).
3. A human reviews the staged tarball and runs `npm stage approve <stage-id>` with npm 2FA.
4. The human then approves a protected GitHub Actions environment job (or manually runs a finalizer), which publishes the existing GitHub Release by changing `draft` to `false`.

The important boundary is that npm approval is deliberately interactive. npm documents that OIDC can run `npm stage publish`, but **cannot** run `npm stage list`, `view`, `approve`, or `reject`; those commands require interactive authentication where applicable. No npm-approval webhook or GitHub Actions event is documented in the official sources reviewed. Consequently, an environment approval can provide a GitHub-side gate, but it cannot cryptographically prove that npm approval happened. The reviewer must perform npm approval first and then approve GitHub finalization.

## Exact supported npm staged-publishing commands

The authoritative command set is in [`npm stage`](https://docs.npmjs.com/cli/v11/commands/npm-stage/) and the [staged-publishing guide](https://docs.npmjs.com/staged-publishing):

| Purpose                                   | Command                              | 2FA |
| ----------------------------------------- | ------------------------------------ | --- |
| Stage current directory or a package spec | `npm stage publish [<package-spec>]` | No  |
| List accessible staged versions           | `npm stage list [<package-spec>]`    | No  |
| Inspect metadata                          | `npm stage view <stage-id>`          | No  |
| Download tarball for inspection           | `npm stage download <stage-id>`      | No  |
| Approve and publish live                  | `npm stage approve <stage-id>`       | Yes |
| Reject and permanently remove stage       | `npm stage reject <stage-id>`        | Yes |

Supported staging examples:

```sh
# Stable release; latest is the documented default
npm stage publish
# Equivalent explicit stable tag
npm stage publish --tag latest

# Prerelease/non-latest stream; tag must be explicit
npm stage publish --tag next

# Human review and decision
npm stage list <package-name>
npm stage view <stage-id>
npm stage download <stage-id>
npm stage approve <stage-id>
# or
npm stage reject <stage-id>
```

Exact behavior and prerequisites:

- Requires **npm CLI 11.15.0 or later** and **Node 22.14.0 or later**.
- The package must already exist in the npm registry; staged publishing cannot create a brand-new package.
- The actor must have package write/publish access and have npm 2FA enabled.
- Staged and published versions share the same unique package/version index. A staged version blocks another publish of that same version, while other versions may still be published.
- Multiple versions of one package may be staged.
- `npm stage publish` respects `"private": true` and refuses to stage such a package.
- `--tag` follows `npm publish`: default is `latest`; prerelease and non-latest versions require an explicit tag. The staged tag is immutable. To change it, reject and re-stage.
- `npm stage` is documented as “unaware of workspaces,” despite the generated command page listing workspace-related flags under `stage publish`. **Uncertainty:** do not rely on workspace flags until verified against the exact installed CLI; invoke staging separately from each package directory.
- For comparison, direct publishing uses `npm publish --tag <tag>`, and later dist-tag movement uses `npm dist-tag add <package>@<version> <tag>` ([publish](https://docs.npmjs.com/cli/v11/commands/npm-publish/), [dist-tag](https://docs.npmjs.com/cli/v11/commands/npm-dist-tag/)). These are not substitutes for the staged approval flow.

## npm trusted-publisher setup

Configure the package’s GitHub Actions trusted publisher as documented in [Trusted publishing for npm packages](https://docs.npmjs.com/trusted-publishers/):

- Exact GitHub owner/organization and repository.
- Exact workflow **filename** (including `.yml`/`.yaml`, without `.github/workflows/`).
- Optional environment name, if one is used; it must match.
- Allowed action: enable **`npm stage publish`**. For the strongest boundary, do **not** enable direct `npm publish`.
- Use a GitHub-hosted runner; self-hosted runners are not currently supported.
- Use npm >=11.15.0 for staging (trusted publishing alone has a lower documented minimum of npm 11.5.1).
- Grant `id-token: write`. No long-lived npm write token is needed.
- The package `repository.url` must exactly match the GitHub repository. npm says these values are case-sensitive.
- npm recommends package publishing access “Require two-factor authentication and disallow tokens” after trusted publishing is working.

OIDC supports only `npm publish` and `npm stage publish`. The other stage subcommands require interactive authentication and cannot use the OIDC token. Trusted publishing from a public GitHub repository to a public package also generates provenance automatically; no `--provenance` flag is needed under that flow.

## GitHub Actions primitives

### Tag trigger

GitHub’s supported tag filter is [`on.push.tags`](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onpushbranchestagsbranches-ignoretags-ignore):

```yaml
on:
  push:
    tags:
      - "v*"
```

A `tags`-only filter does not run for branch pushes. Path filters are not evaluated for tag pushes. GitHub also documents that tag events are not created when more than three tags are pushed at once, so release tags should be pushed individually. See the [`push` event](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#push).

### Token permissions

Use least privilege ([`GITHUB_TOKEN` permissions](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication)):

```yaml
permissions:
  contents: write # checkout/read plus create/edit GitHub Release
  id-token: write # npm trusted-publishing OIDC token
```

The GitHub Releases REST API documents **Contents: write** for creating and updating releases ([Create/update a release](https://docs.github.com/en/rest/releases/releases)). `id-token: write` is npm’s critical OIDC requirement.

### Draft and final GitHub Release

Official GitHub CLI commands:

```sh
# Existing pushed tag only; do not let release creation synthesize a tag
GH_TOKEN="$GITHUB_TOKEN" gh release create "$GITHUB_REF_NAME" \
  --verify-tag --draft --generate-notes

# After npm approval, publish the existing draft
GH_TOKEN="$GITHUB_TOKEN" gh release edit "$GITHUB_REF_NAME" --draft=false

# For a stable release, optionally force the Latest label
GH_TOKEN="$GITHUB_TOKEN" gh release edit "$GITHUB_REF_NAME" \
  --draft=false --latest
```

`--verify-tag` is important: [`gh release create`](https://cli.github.com/manual/gh_release_create) otherwise creates a missing tag from the default branch (or `--target`). [`gh release edit TAG --draft=false`](https://cli.github.com/manual/gh_release_edit) is the documented command to publish a prior draft.

Equivalent REST primitives are:

- `POST /repos/{owner}/{repo}/releases` with `tag_name` and `draft: true`.
- `PATCH /repos/{owner}/{repo}/releases/{release_id}` with `draft: false`; the API explicitly says `false` publishes the release.
- `prerelease: true` marks a prerelease. Drafts and prereleases cannot be Latest; `make_latest` accepts `true`, `false`, or `legacy`.

GitHub recommends draft-first creation when immutable releases are enabled, because assets can be attached before publication ([Managing releases](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)).

### Human finalization gate

A job that references a GitHub environment cannot start or access environment secrets until its protection rules pass. An environment can have required reviewers, prevent self-review, disallow admin bypass, and restrict deployment to selected tag patterns ([Managing environments](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments)).

Recommended procedural gate:

1. `stage` job creates the draft and stages npm.
2. `finalize` job has `needs: stage` and `environment: npm-release-finalize`.
3. Configure that environment with required reviewers, prevent self-review, disable admin bypass if policy requires it, and allow only release tags.
4. Reviewer uses `npm stage list/view/download`, then `npm stage approve <stage-id>` with 2FA.
5. Only after successful npm approval, reviewer approves the environment job; it runs `gh release edit ... --draft=false`.

Illustrative supported skeleton (build steps intentionally project-specific):

```yaml
name: Staged npm release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write
  id-token: write

jobs:
  stage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-node@v6
        with:
          node-version: "24"
          registry-url: "https://registry.npmjs.org"
          package-manager-cache: false
      - run: npm ci
      - run: npm test
      - name: Create draft GitHub Release for existing tag
        run: gh release create "$GITHUB_REF_NAME" --verify-tag --draft --generate-notes
        env:
          GH_TOKEN: ${{ github.token }}
      - name: Stage npm package
        run: npm stage publish --tag latest

  finalize:
    needs: stage
    runs-on: ubuntu-latest
    environment: npm-release-finalize
    steps:
      - name: Publish GitHub Release after reviewer has approved npm stage
        run: gh release edit "$GITHUB_REF_NAME" --draft=false --latest
        env:
          GH_TOKEN: ${{ github.token }}
```

For prerelease tags, replace npm tag `latest` with an explicit non-latest dist-tag (for example `next`), create/mark the GitHub Release as `--prerelease`, and do not use `--latest`.

**Failure-order uncertainty:** creating the draft first can leave a draft if npm staging fails; staging first can leave an unpublished npm stage if draft creation fails. Neither system documents a cross-service transaction. Choose an order and document cleanup (`npm stage reject <stage-id>` and/or delete the draft). The skeleton prioritizes publishing a visible draft audit object before staging.

## Tag ruleset primitives

Create an **active tag ruleset** under Settings → Rules → Rulesets, target release tags (for example `v*`), and use these documented rules ([About rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets), [Available rules](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets), [Creating rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/creating-rulesets-for-a-repository)):

- **Restrict creations:** only bypass actors can create matching tags.
- **Restrict updates:** only bypass actors can move matching tags.
- **Restrict deletions:** only bypass actors can delete matching tags (selected by default).
- **Block force pushes:** enabled by default; prevents force-updating targeted tags.
- **Bypass list:** limit to release maintainers/a release team or a dedicated GitHub App. Do not broadly grant admin bypass.

The workflow does not need tag-ruleset bypass because the tag already exists and `gh release create --verify-tag` prevents it from creating/moving the tag.

Rulesets use `fnmatch`, not regular expressions; `v*` is a broad prefix policy, not semantic-version validation. **Uncertainty:** the reviewed ruleset documentation does not provide a rule that validates complete SemVer tag syntax or requires a cryptographically signed tag object. “Require signed commits” concerns commits, so it should not be represented as signed-tag enforcement.

Do not make the post-tag release workflow itself a required status check for creating that same tag without validating the behavior first: the tag-triggered check cannot run until the tag exists, creating a potential bootstrap cycle. The official pages reviewed do not document this particular design as supported.

## Event caveats

- A [`release: published`](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#release) workflow runs for stable and prereleases published from drafts; draft `created`, `edited`, and `deleted` activities do not trigger workflows.
- However, GitHub states that events caused by the repository’s [`GITHUB_TOKEN`](https://docs.github.com/en/actions/concepts/security/github_token#when-github_token-triggers-workflow-runs) generally do **not** create a new workflow run (except documented dispatch/PR cases). Therefore, if the finalizer publishes the release with `GITHUB_TOKEN`, do not rely on a separate `release: published` workflow firing. Keep required final work in the same workflow, or deliberately use an appropriately scoped GitHub App/PAT after assessing the recursion and security implications.
- If a release’s resolved target commit modifies `.github/workflows/` relative to the default branch, the Releases API documents additional workflow authorization requirements and says `GITHUB_TOKEN` cannot be authorized for that special case. For a normal pre-existing release tag pointing at a commit already on the default branch, this caveat should not apply, but verify repository-specific tag targets.

## Marked uncertainties / operational checks

1. **Feature/runtime availability:** official npm docs currently specify npm >=11.15.0 and Node >=22.14.0. Pin or explicitly install a CLI meeting that minimum; do not assume the runner’s bundled npm is sufficient.
2. **Workspace contradiction:** the `npm stage` page says the command is unaware of workspaces while generated flags list workspace options. Stage each package from its own directory unless tested with the pinned CLI.
3. **No automatic npm→GitHub approval signal:** official sources reviewed expose no npm stage-approved webhook or GitHub event. Finalization is a human attestation unless another independently verified mechanism is introduced.
4. **No cross-registry transaction:** draft creation and npm staging/approval can partially succeed. Define rollback and rerun procedures.
5. **Ruleset pattern strength:** `fnmatch` can protect a prefix such as `v*`, but is not full SemVer validation, and documented signed-commit rules are not signed-tag rules.
6. **Downstream release event suppression:** publishing with `GITHUB_TOKEN` generally does not trigger another workflow; test any design that expects `release: published` automation or use the same run.
