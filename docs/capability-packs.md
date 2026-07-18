# Capability packs and manual transition

Capability packs are opt-in additions managed by **Packy core**. Packy core is
the always-available `packy` installer/configurator; it is independent of the
optional pack named `matty`. Leaving that pack inactive—or deactivating it—does
not remove the CLI or prevent pack inspection and management.

The initial Packy-owned catalog is deliberately limited:

| Pack | Purpose |
| --- | --- |
| `matty` | Matty workflow skills and behavioral guidance |
| `engram` | Engram memory guidance, MCP declaration, and lifecycle intent |

Both packs support only the `codex` and `opencode` surfaces. Remote and
third-party sources, marketplaces, signing, `web`, `mobile`, version selection,
downgrades, unattended Apply, and background runtime management are not
supported.

## Inspect before changing anything

Discovery and status are inspection-only. Run these before deciding whether to
activate a pack:

```sh
packy pack list
packy pack show matty
packy pack show engram
packy pack status
packy pack status matty --surface codex
packy pack status engram --surface opencode
```

The overview covers every pack/surface pair. Targeted status adds activation
intent, the latest reconciliation attempt, projection evidence, blockers,
readiness, and pending human actions.

## Manual clean cutover

There is **no automatic migration** from legacy product state.
Packy does not translate old state, write transition configuration, or adopt
existing files merely because they resemble pack output. Inspect first, then
explicitly preview and activate the pack on each surface you choose:

```sh
packy pack activate matty --surface codex --dry-run
packy pack activate matty --surface codex

packy pack activate matty --surface opencode --dry-run
packy pack activate matty --surface opencode
```

`--dry-run` creates only a fresh Preview: it requests no approval and performs no
mutation. Apply requires an interactive terminal. The rendered plan separates
reversible local work, executable/external effects, and destructive cleanup;
Packy asks for a plan-bound approval for each required consent kind. There is no
generic `--yes` approval.

If status reports unmanaged, ambiguous, or drifted content, preserve it and
resolve the reported pending human action. Activation does not silently adopt
such content. Do not delete or rewrite the old installation merely to force an
Apply; decide explicitly which content remains user-managed.

## Lifecycle examples

Every mutation is scoped to one surface; activating on Codex never implies
activation on OpenCode.

```sh
# Explicit activation
packy pack activate engram --surface codex --dry-run
packy pack activate engram --surface codex

# Update means the current version in the local Packy-owned catalog
packy pack update engram --surface codex --dry-run
packy pack update engram --surface codex

# Repair one active pack, or all active packs on one surface
packy pack reconcile engram --surface codex --dry-run
packy pack reconcile --surface codex --dry-run

# Contributor-safe removal
packy pack deactivate engram --surface codex --dry-run
packy pack deactivate engram --surface codex
```

Approvals are tied to the exact immutable plan and consent phase. If Apply
rejects a stale plan, it executes no actions. Inspect the changed precondition,
then repeat the same verb (for example, `packy pack update engram --surface
codex`) to obtain a fresh Preview and provide fresh approvals. Packy does not
retry automatically.

After a partial attempt marked `recovery-required`, repeat the originating
`activate`, `update`, or `deactivate` command. Packy freshly inspects the host and
previews a recovery plan; it does not replay the historical plan or reuse its
approvals.

## Apply success is not readiness

A verified Apply can exit successfully while authorization, trust, reload, or
runtime loading is still pending. Readiness is derived separately from fresh host
observation:

- **configured**: the Packy-owned projections are reconciled;
- **authorized**: required login, trust, and permissions are observed complete;
- **usable**: the host has loaded the capability under its runtime permissions.

Inspect pending actions and use the independent automation gate:

```sh
packy pack status engram --surface codex
packy pack status engram --surface codex --require usable
```

The second command exits nonzero until usability is freshly observed. Login,
trust, permissions, reload/restart, runtime loading, and external installation or
setup are human/host boundaries. Packy reports them as pending human actions; it
does not represent them as generic approvals, receipts, or verified Apply
actions, and it does not complete them automatically.

## Ownership and cleanup limits

Packy updates or removes only projections whose ownership and unchanged
fingerprint it has verified. It preserves unmanaged, ambiguous, and drifted
content for human review. Shared resources remain while any active pack is still
a contributor; destructive cleanup is limited to unchanged, verified,
last-contributor projections and requires its own typed approval.

Deactivation never deletes host credentials, Engram memory (including Engram's
data directory), or external data. Logout, credential removal, and external-tool
uninstallation remain separate actions owned by the relevant host or tool.
