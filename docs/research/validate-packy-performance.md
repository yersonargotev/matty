# `validate-packy.sh` performance investigation

Date: 2026-07-19

Historical status: this document records the pre-optimization investigation at
the revisions named below. Packy's validator has since been optimized; see
[`ci-validation-performance-evidence.md`](ci-validation-performance-evidence.md)
for the post-optimization measurements and current contributor guidance.

## Conclusion

At the investigated revisions, `./scripts/validate-packy.sh` was slow mainly
because a test inside the suite ran the entire validator again. The outer
validator ran that test once under normal `go test` and once under
`go test -race`; each nested validator then repeated the Addy cohort, build, vet,
normal tests, and race tests. Expensive release cross-build tests consequently
executed up to six times in one top-level run.

Full validation remains the correct final gate before reporting or committing a
completed change and in CI. It is not necessary after every edit: iteration
should use formatting plus tests for the changed package and known dependents,
then run the authoritative full validator once at the end.

## Evidence

### CI history

The latest eight successful `Validate Packy-owned code` jobs took 585, 606, 459,
625, 603, 591, 582, and 585 seconds: median **588 seconds (9m48s)**. In three
sampled jobs, the `Validate allowlisted Packy-owned code` step itself took 604,
580, and 567 seconds, so checkout and Go setup are not the primary cost.

Historical GitHub Actions evidence shows the regression's shape:

| Revision | Job duration | Relevant state |
| --- | ---: | --- |
| parent `42a2ef6` | 93s | before validation hardening |
| `0f8435c` | 417s | hostile-content/nested-validator proof present |
| `fa2e97e` | 518s | before the Addy cohort |
| `9bd564b` | 540s | Addy cohort added |

### Local measurements

A full run passed in 264.21s real time (909.50s user, 388.83s system). Its first
roughly 60 seconds overlapped an accidentally concurrent, later-aborted benchmark,
so the total is conservative rather than a clean baseline. The package timings
still expose the bottleneck:

| Phase | `internal/ci` | `internal/release` | `internal/cli` |
| --- | ---: | ---: | ---: |
| normal tests | 124.979s | 33.068s | 7.759s |
| race tests | 107.933s | 32.428s | 15.413s |

With `PACKY_VALIDATION_NESTED=1`, which skips only the recursive validator test,
an uncached `go test -count=1 ./internal/ci` passed in **2.56s real** (2.059s
reported by Go). This isolates almost all of `internal/ci`'s 108-125 second cost
to `TestValidationEntrypointIgnoresHostileUnownedGoContent`.

An uncached isolated `internal/release` run took **25.50s real**. Its release
artifact cross-build test took 17.57s and its package-install smoke lifecycle
took 6.64s.

## Root causes

### 1. Recursive full-suite execution

`scripts/validate-packy.sh` runs all allowlisted packages normally and again with
the race detector. `internal/ci/validation_test.go`'s hostile-content test copies
the repository and launches that same script. `PACKY_VALIDATION_NESTED=1` stops
infinite recursion, but it does not make the nested validator smaller. Therefore
one top-level run contains:

1. the outer normal suite;
2. one complete nested validator from the normal `internal/ci` test;
3. the outer race suite; and
4. another complete nested validator from the raced `internal/ci` test.

The nested validators each run both normal and race suites. This is also why CPU
time is much larger than wall time and why CI, with less parallel CPU capacity
than the development machine, is particularly slow.

### 2. Addy acceptance launches 52 Go test processes

`scripts/validate-addy-acceptance.sh` has 26 `run_row` calls. Every row executes
one `go test -list` and one cache-disabled `go test -count=1`: **52 Go test
processes** across seven packages. The matrix contains 39 package/test references
but only 29 unique pairs, so ten references repeat tests already executed by a
different acceptance row. The nested-validator structure repeats this pre-phase
three times per top-level validation.

The Go tool documents that package-list tests can reuse successful results, while
`-count=1` explicitly disables test-result caching. Build caching is already
supported by Go, and `.github/workflows/ci.yml` already enables `actions/setup-go`
caching, so adding another generic cache is not the first fix. See the official
[Go test documentation](https://pkg.go.dev/cmd/go/internal/test),
[Go build/test cache documentation](https://pkg.go.dev/cmd/go), and
[`actions/setup-go` caching documentation](https://github.com/actions/setup-go#caching-dependency-files-and-build-outputs).

### 3. Expensive integration tests are repeated under `-race`

`internal/release` builds artifacts for multiple platforms and exercises a local
package-install lifecycle. These are valuable integration checks, but the current
shape runs them in both the normal and race suites and, because of recursion, in
both modes inside both nested validators too.

The race detector is intentionally expensive: the Go project documents typical
execution overhead of 2-20x and explicitly supports excluding tests that take too
long under race via the `race` build tag. It only finds races in executed,
instrumented code paths. See the official
[Go race detector guide](https://go.dev/doc/articles/race_detector).

## Recommended validation model

### During implementation

Use the smallest check that can reject the current edit quickly:

- Go source: `gofmt` the changed files and run `go test` for the owning package;
- shared package/API change: also test known reverse dependents;
- workflow/schema/validator change: run the relevant `internal/ci` tests;
- Addy contract change: run only the affected acceptance tests;
- concurrency-sensitive change: run `go test -race` for the affected package.

This is an iteration loop, not the merge proof. A future helper may calculate
changed Go packages and reverse dependents, but non-Go contracts require explicit
path-to-test mappings and safe fallbacks. It must never silently replace the full
gate.

### Before completion and in CI

Run `./scripts/validate-packy.sh` once against the final tree. Keep CI exhaustive
for code-affecting changes. A docs-only fast path is possible, but only if the
required check still reports success and the repository first proves that no
tests treat those documents as executable contracts.

## Prioritized optimization plan

1. **Remove full-validator recursion while preserving the hostile-content proof.**
   Split package-selection verification from execution, or give the copied-repo
   contract test a narrow mode that verifies the exact commands/allowlist without
   running normal and race suites. The already-existing structural assertions in
   `TestValidationEntrypointOwnsTheExactPackageAllowlist` should remain. This is
   the highest-value change and should recover minutes, not seconds.
2. **Do not run the nested process test under the outer race suite.** The test
   primarily verifies shell orchestration and filesystem selection; racing a Go
   test that launches an uninstrumented shell does not add equivalent coverage.
   Preserve at least one real copied-repository proof in the normal suite.
3. **Batch Addy acceptance by package.** Validate every row-to-test mapping, then
   execute the union of unique tests once per package rather than once per row.
   Keep row-level diagnostic output and `-count=1` if freshness is contractual.
4. **Separate race-worthy unit tests from expensive subprocess/cross-build
   integration tests.** Run integration tests once and race the packages/tests
   whose in-process concurrency paths matter. This requires an explicit coverage
   review; do not simply drop `internal/release` from race.
5. **Add a non-authoritative `validate-changed` developer command.** It should
   compute changed packages plus reverse dependents and fall back to full
   validation for `go.mod`, validator scripts, workflows, schemas, bundle
   contracts, or unknown paths.
6. **Only then consider parallel CI jobs.** Parallel jobs can reduce elapsed wait
   but may increase total compute and duplicate cold compilation. Fixing recursive
   work first yields the larger and simpler gain.

## Safety boundaries

- Keep `scripts/validate-packy.sh` as the single exhaustive repository authority.
- Preserve the explicit Packy-owned package allowlist; never replace it with
  `go test ./...`, because vendored/unowned content is intentionally inert.
- Preserve sandboxed `HOME` and `XDG_CONFIG_HOME` in all local and CI validation.
- Measure the same final tree before and after each optimization and compare both
  wall time and which tests actually executed.
