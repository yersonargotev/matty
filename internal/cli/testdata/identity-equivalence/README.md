# Frozen Matty runtime baseline

`matty-0e8971a.json` is the identity-normalized runtime transcript captured
from the frozen pre-rename base
`0e8971ad4ccacad5f99ec97d05ed963830b58070`. Its SHA-256 is
`4c8e26cc7f08b19966d3a85ace6cb59d154f8fd52bca49e52fec3783eba9515f`.

The fixture was generated once on 2026-07-18 by running the same test harness
in a detached worktree at that exact commit:

```sh
base=0e8971ad4ccacad5f99ec97d05ed963830b58070
candidate=/path/to/packy-candidate
base_worktree="$(mktemp -d /tmp/matty-base-equivalence.XXXXXX)"
rmdir "$base_worktree"
git worktree add --detach "$base_worktree" "$base"
cp "$candidate/internal/cli/identity_equivalence_test.go" \
  "$base_worktree/internal/cli/identity_equivalence_test.go"
(
  cd "$base_worktree"
  PACKY_EQUIVALENCE_UPDATE="$candidate/internal/cli/testdata/identity-equivalence/matty-0e8971a.json" \
    go test ./internal/cli -run TestPackyRuntimeMatchesFrozenMattyBaseline -count=1
)
shasum -a 256 \
  "$candidate/internal/cli/testdata/identity-equivalence/matty-0e8971a.json"
```

Normal tests only read this checked-in fixture. They do not inspect Git
history, require the base commit, or access the network. Regeneration is only
valid from the exact frozen base above; candidate output must never be used as
the baseline.

Normalization is limited to classified product-owned fields: executable and
command prose, repository/source/state paths, state and environment keys,
prompt markers/references, volatile plan IDs and digests, and sandbox roots.
The `matty` pack ID, operands, contributors, resources, and pack instruction
content remain literal. The sole command-shape adaptation is explicit in the
harness: frozen Matty's `--version` observation is compared with the required
Packy `version` command, and Packy's added `version` help row is excluded from
the otherwise equivalent root-help observation.
