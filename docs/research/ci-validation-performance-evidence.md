# CI validation performance evidence

This note records three successful post-optimization executions of the GitHub
Actions **Validate Packy-owned code** job. Durations are observations, not a
timing threshold or a new CI acceptance criterion.

## Observed runs

| CI run | Total Validate job | `Validate allowlisted Packy-owned code` step |
| --- | ---: | ---: |
| [29717665008](https://github.com/yersonargotev/packy/actions/runs/29717665008) | 142s | 120s |
| [29719076007](https://github.com/yersonargotev/packy/actions/runs/29719076007) | 146s | 121s |
| [29719190010](https://github.com/yersonargotev/packy/actions/runs/29719190010) | 138s | 115s |
| **Median** | **142s** | **120s** |

Against the [recorded 588s pre-optimization baseline](https://github.com/yersonargotev/packy/issues/101),
the median total job time is 446s lower, a 75.9% reduction. This comparison
describes these runs only; normal runner and cache variation means it must not
be treated as a performance gate.

## Validation shape

Each run's validator log showed the same bounded execution shape:

| Run | Format | Build | Vet | Tests | Race | Addy package markers | `internal/release` in tests | `internal/release` in race |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 29717665008 | 1 | 1 | 1 | 1 | 1 | 7 | 1 | 0 |
| 29719076007 | 1 | 1 | 1 | 1 | 1 | 7 | 1 | 0 |
| 29719190010 | 1 | 1 | 1 | 1 | 1 | 7 | 1 | 0 |

Thus, each run invoked the expensive release-package tests once rather than
repeating them under race instrumentation.

The workflow has no path filter, so code-affecting pull requests and pushes to
`main` continue to expose the single **Validate Packy-owned code** result. Its
validation step calls `./scripts/validate-packy.sh`, not the focused developer
command. `actions/setup-go` retains `cache: true`, and no generic cache action
was added.

## Contributor workflow

During iteration, contributors may use `./scripts/validate-changed.sh` for fast,
focused feedback on the complete base-to-working-tree change. That focused
result is non-authoritative. Before final delivery, contributors must run the
exhaustive `./scripts/validate-packy.sh`; CI calls the same exhaustive command.
