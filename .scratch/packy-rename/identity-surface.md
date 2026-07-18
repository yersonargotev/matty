# Matty identity-surface classification

Research asset for [Classify the complete Matty identity surface](https://github.com/yersonargotev/matty/issues/50). This is an implementation boundary, not a rename implementation.

## Conclusion

The clean cutover has three ownership classes:

1. **Product-owned identity becomes Packy.** Rename current repository/module/CLI/distribution/configuration/state/automation/maintainer identity, including matching symbols, schemas, fixtures, and tests.
2. **The capability pack stays `matty`.** Preserve its semantic ID, resource IDs, current manifest/bindings, compatibility contract, and immutable history.
3. **Authentic history stays Matty.** Do not rewrite accepted ADRs, completed `.scratch/` planning/prototypes, old releases/tags, or hashed compatibility evidence. Record the rename in a new ADR instead.

A blind case-insensitive replacement is invalid because current files mix product and pack ownership.

## Survey and source of truth

At the audited `main` baseline, 370 tracked files contained 2,472 case-sensitive textual matches (`MATTY`: 124, `Matty`: 857, `matty`: 1,491) in 218 files. Adding tracked paths whose names encode the identity but whose contents do not yields 263 affected files. The exhaustive partition is 127 historical, 4 pack-only, 24 mixed, and 108 product-only files.

Evidence came from the tracked repository itself:

```sh
git grep -I -i -l matty
git grep -I -o MATTY
git grep -I -o Matty
git grep -I -o matty
git ls-files | grep -i matty
git remote -v
```

CodeGraph identified the owning runtime seams before source classification; targeted reads and `git grep` supplied runtime strings, manifests, docs, configuration, scripts, and tests. The audit artifact itself is excluded from the baseline counts above.

## Product-owned identity: rename to Packy

The following semantic families are product-owned everywhere they occur in live source, tests, fixtures, docs, or automation:

| Surface | Current identity | Packy destination |
| --- | --- | --- |
| Repository/module | `yersonargotev/matty`, `github.com/yersonargotev/matty` | in-place Packy repository and `github.com/yersonargotev/packy` module/imports |
| Executable/source | `cmd/matty`, Cobra `Use: "matty"`, command examples/output | `cmd/packy`, `packy` |
| Distribution | `matty_*` artifacts, `Formula/matty.rb`, `class Matty`, Homebrew binary/formula | Packy equivalents |
| Installed source/state | `~/.local/share/matty`, `~/.matty`, `MattyHome`, `mattyHome`, `matty_version` | Packy equivalents; no migration or alias |
| Overrides/tools | `MATTY_SKILLS_SOURCE`, synchronization `MATTY_*`, temp/staging prefixes, HTTP user agent | `PACKY_*` and Packy-owned prefixes |
| Host projections | OpenCode `matty.md`, `<!-- matty:* -->`, `# matty:*`, `HasMatty*` product observations | Packy file, markers, and symbols; do not recognize legacy aliases |
| Synchronization contract | `matty-pack-sync`, `matty-packsync`, `MattySuite` / `matty_suite` | Packy-owned names and schema fields |
| Maintainer surfaces | `deliver-matty-issue`, `release-matty`, `validate-matty.sh`, Matty workflow docs | Packy names and references |
| Narrative | “Matty core/product/CLI/state/owned/managed/suite/repository” | Packy wording |

Tests and fixtures follow the owner of the contract they assert: product paths, output, fields, markers, schema IDs, environment variables, artifacts, and executable names must change with the implementation.

### Product-only path inventory

Every Matty occurrence in these paths is product-owned:

- `.agents/skills/deliver-matty-issue/SKILL.md`
- `.agents/skills/deliver-matty-issue/agents/openai.yaml`
- `.agents/skills/release-matty/SKILL.md`
- `.agents/skills/release-matty/agents/openai.yaml`
- `.agents/skills/sync-pack-source/REQUESTS.md`
- `.agents/skills/sync-pack-source/SKILL.md`
- `.agents/skills/sync-pack-source/agents/openai.yaml`
- `.agents/skills/sync-pack-source/scripts/dispatch.sh`
- `.agents/skills/sync-pack-source/scripts/result-state.sh`
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `.github/workflows/sync-pack-source.yml`
- `AGENTS.md`
- `NOTES.md`
- `cmd/matty/main.go`
- `docs/agents/domain.md`
- `docs/release.md`
- `docs/structured-output.md`
- `go.mod`
- `internal/bootstrap/bootstrap.go`
- `internal/bootstrap/installed_source.go`
- `internal/bootstrap/installed_source_test.go`
- `internal/capabilitypack/activation.go`
- `internal/capabilitypack/engram_activation_test.go`
- `internal/capabilitypack/layout.go`
- `internal/capabilitypack/layout_test.go`
- `internal/capabilitypack/reconcile_test.go`
- `internal/capabilitypack/status.go`
- `internal/ci/testdata/sync-pack-source-skill.json`
- `internal/ci/validation_test.go`
- `internal/cli/architecture_test.go`
- `internal/cli/doctor_contract_test.go`
- `internal/cli/env.go`
- `internal/cli/layout_fixture_test.go`
- `internal/cli/prompt_test.go`
- `internal/cli/root.go`
- `internal/cli/root_test.go`
- `internal/cli/setup_health_adapter.go`
- `internal/cli/setup_health_adapter_test.go`
- `internal/codex/engram_contract.go`
- `internal/codex/layout_test.go`
- `internal/codex/setup_observation.go`
- `internal/codex/surface_test.go`
- `internal/codex/testdata/engram-1.19.0/README.md`
- `internal/corelifecycle/install.go`
- `internal/corelifecycle/install_test.go`
- `internal/corelifecycle/layout.go`
- `internal/corelifecycle/layout_test.go`
- `internal/corelifecycle/setup_observation.go`
- `internal/corelifecycle/state.go`
- `internal/corelifecycle/state_test.go`
- `internal/corelifecycle/uninstall.go`
- `internal/corelifecycle/uninstall_test.go`
- `internal/corelifecycle/update.go`
- `internal/corelifecycle/update_test.go`
- `internal/engrambin/engrambin.go`
- `internal/localprojection/localprojection.go`
- `internal/localprojection/localprojection_test.go`
- `internal/opencode/layout.go`
- `internal/opencode/layout_test.go`
- `internal/opencode/opencode.go`
- `internal/opencode/opencode_test.go`
- `internal/ownedcontainer/ownedcontainer.go`
- `internal/ownedcontainer/ownedcontainer_test.go`
- `internal/packclassification/orchestrator.go`
- `internal/packclassification/orchestrator_test.go`
- `internal/packsync/check.go`
- `internal/packsync/classification_test.go`
- `internal/packsync/githubsource/github.go`
- `internal/packsync/githubsource/github_test.go`
- `internal/packsync/transaction.go`
- `internal/packsync/types.go`
- `internal/packsyncworkflow/brief.go`
- `internal/packsyncworkflow/failure_policy.go`
- `internal/packsyncworkflow/orchestrator.go`
- `internal/packsyncworkflow/ownership.go`
- `internal/packsyncworkflow/publication.go`
- `internal/packsyncworkflow/types.go`
- `internal/packsyncworkflow/workflow_test.go`
- `internal/prompt/prompt.go`
- `internal/prompt/prompt_test.go`
- `internal/release/package_install_smoke_test.go`
- `internal/release/release_automation_test.go`
- `internal/setuphealth/setuphealth.go`
- `internal/setuphealth/setuphealth_test.go`
- `internal/skillbundle/bundle.go`
- `internal/skillbundle/bundle_test.go`
- `internal/tools/syncpacksource/full_lifecycle_test.go`
- `internal/tools/syncpacksource/main.go`
- `internal/tools/syncpacksource/main_test.go`
- `internal/tools/syncpacksource/model.go`
- `internal/tools/syncpacksource/publication_lifecycle_test.go`
- `internal/tools/syncpacksource/publish.go`
- `internal/tools/syncpacksource/source_retry.go`
- `internal/version/version.go`
- `internal/workstation/workstation.go`
- `internal/workstation/workstation_test.go`
- `scripts/build-release-artifacts.sh`
- `scripts/generate-homebrew-formula.sh`
- `scripts/validate-matty.sh`
- `workflows/matty-issue-delivery.md`
- `workflows/matty-release.md`
- `workflows/pack-source-synchronization.md`
- `workflows/schemas/pack-source-dispatch.schema.json`
- `workflows/schemas/pack-source-noop.schema.json`
- `workflows/schemas/pack-source-operational-artifact.schema.json`
- `workflows/schemas/pack-source-publication.schema.json`
- `workflows/schemas/pack-source-validation.schema.json`

## Surviving capability-pack identity: preserve

These current bundle paths are wholly pack-owned and remain named `matty`:

- `bundle/instructions/matty-guidance.md`
- `bundle/instructions/matty-workflow-conventions.md`
- `bundle/packs/matty/pack.json`
- `bundle/sources.json`

Preserve all of these semantics wherever mirrored in code/tests/docs:

- pack ID `matty`, manifest directory `bundle/packs/matty`, `pack_id: "matty"`, and `workflow:matty`;
- pack resource IDs and paths `matty-guidance`, `matty-workflow-conventions`, and their instruction files;
- CLI operands and output that identify the pack, for example `pack show matty` and `pack activate matty` (the executable prefix still becomes `packy`);
- ownership contributors whose value is the pack ID `matty`;
- special host readiness checks whose condition is `pack.ID ==/!= "matty"`;
- historical intent keys such as `matty@1.0.0` and the corresponding compatibility tuple.

## Authentic historical and compatibility identity: preserve

The following path scopes are authentic history and remain unchanged:

- `bundle/history/matty/**` — immutable pack artifacts, including bytes covered by recorded hashes;
- `bundle/compatibility/matty/**` — compatibility evidence for the surviving pack;
- `docs/adr/0001-*.md` through `docs/adr/0009-*.md` — accepted decision records; add a new rename ADR rather than editing them;
- `.scratch/**` — completed maps, research, prototypes, and implementation planning history. `docs/roadmap.md` explicitly identifies completed `.scratch/` artifacts as preserved planning history rather than active/runtime docs;
- already-published tags/releases and their artifacts (external verification and exact cutover handling belong to the external-constraints ticket).

This class covers 127 affected tracked files at the baseline. Whole-subtree rules are intentional: they are safer and more complete than enumerating only files whose contents happen to spell Matty.

Runtime code that verifies history is mixed rather than historical: preserve the identity/hash constants while renaming its product module imports or product narrative.

## Mixed paths: classify token by token

These paths contain both product-owned and pack/historical identity:

- `CONTEXT.md`
- `README.md`
- `bundle/sources.lock.json`
- `docs/capability-packs.md`
- `docs/product/matty-v0.md`
- `docs/roadmap.md`
- `internal/capabilitypack/activation_test.go`
- `internal/capabilitypack/catalog.go`
- `internal/capabilitypack/catalog_test.go`
- `internal/capabilitypack/composition_test.go`
- `internal/capabilitypack/history.go`
- `internal/capabilitypack/history_test.go`
- `internal/capabilitypack/recovery_test.go`
- `internal/capabilitypack/state_store_test.go`
- `internal/capabilitypack/status_test.go`
- `internal/capabilitypack/statusjson_test.go`
- `internal/cli/pack.go`
- `internal/cli/pack_test.go`
- `internal/codex/surface.go`
- `internal/opencode/surface.go`
- `internal/opencode/surface_test.go`
- `internal/packsync/check_test.go`
- `internal/packsync/compatibility.go`
- `internal/packsync/transaction_test.go`

The decisive cases are:

| Path/shape | Rename | Preserve |
| --- | --- | --- |
| `bundle/sources.lock.json` | generator `matty-packsync` | every `pack_id: "matty"` and upstream provenance |
| `README.md`, `CONTEXT.md`, `docs/capability-packs.md`, `docs/product/matty-v0.md`, `docs/roadmap.md` | product/core/CLI/state/ownership names and the active product-scope filename | explicit optional pack ID `matty`; authentic links to history |
| `internal/cli/pack.go` and tests | executable prefix and Packy product prose | pack operands/IDs/resource IDs |
| `internal/codex/surface.go`, `internal/opencode/surface.go` | `matty:pack:` **ownership prefix**, OpenCode product prompt path/symbols | interpolated pack/resource ID, including `matty` / `matty-guidance` |
| `internal/capabilitypack/**` mixed files | module import path and current product narrative | pack IDs, contributors, historical intent key/hash |
| `internal/packsync/compatibility.go` and tests | product module imports/current product validation prose | exact `matty` compatibility tuple/evidence hash and history paths |
| synchronization publication marker | `<!-- matty-pack-sync:` is product workflow ownership, so becomes Packy | no legacy marker parser |
| schemas/publication types | `MattySuite` / `matty_suite` is product validation authority, so becomes Packy | `pack_id: "matty"` when it identifies the pack |

## Implementation invariants

1. **No global replacement.** Mixed paths require semantic, line-level edits.
2. **No compatibility bridge.** Do not retain a `matty` binary, `MATTY_*` env fallback, old state-path lookup, old host marker parsing, or automatic migration.
3. **Pack identity is stable.** `matty`, `workflow:matty`, its resource IDs, bindings, contributors, compatibility tuple, and history paths are allowlisted only when they denote the pack.
4. **History is stable.** Never mutate bytes below `bundle/history/matty/**`; never rewrite accepted ADRs or completed `.scratch/` artifacts merely to erase the old product name.
5. **Generated/current contracts move atomically.** Producer, consumer, schemas, workflow YAML, scripts, fixtures, and tests must agree on Packy names in the same implementation cutover.
6. **State schemas may break cleanly.** `matty_version` and other product-owned persisted/structured names become Packy without legacy decoding; operator cleanup is a separate deliberate task.
7. **Verification must be contextual.** A final `Matty|matty|MATTY` scan is expected to return only the pack/historical allowlist above, not zero matches.

## Boundaries for later tickets

- External GitHub repository rename, release/tag facts, Homebrew/tap state, and remote workflow behavior are verified by **Verify the external Packy cutover constraints**.
- Irreversible sequencing and exact implementation slicing are decided after this inventory and the external facts are both resolved.
- Personal uninstall/cleanup commands are not inferred here; they depend on the verified installed footprint.
