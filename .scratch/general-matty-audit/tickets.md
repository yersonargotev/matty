# Tickets: General Matty audit remediation

These tickets remediate the correctness, durability, automation, and test-confidence findings from the sandboxed end-to-end Matty audit performed on 2026-07-12.

Work the **frontier**: any ticket whose blockers are all done. Several tickets can start immediately; integrate and verify one ticket at a time in a fresh implementation session.

## Report truthful Matty pack readiness after Apply

**What to build:** Make Matty pack lifecycle results distinguish verified local configuration from host runtime usability. After activating the Matty pack, Apply output and a fresh targeted status must agree that usability remains pending until the host provides an observable runtime signal.

**Blocked by:** None — can start immediately.

- [x] Activating the Matty pack can succeed after its exact approved plan is applied and its owned projections verify, without claiming that the host has loaded the capability.
- [x] Apply output reports configured, authorized, and usable readiness using the same meanings as fresh status inspection.
- [x] An immediate `status --require usable` result does not contradict the readiness rendered by Apply.
- [x] Pending reload or runtime-verification actions are explicit and actionable.
- [x] Focused lifecycle and CLI tests reproduce the former contradiction and protect both supported surfaces where applicable.

## Expose doctor failures to automation

**What to build:** Give automation a stable way to distinguish a healthy diagnostic report from one containing hard failures, while preserving warnings as advisory information and retaining useful human-readable output.

**Blocked by:** None — can start immediately.

- [x] A doctor report containing at least one `FAIL` returns a documented non-success result.
- [x] A report containing only `PASS` and `WARN` checks follows an explicit, tested success policy.
- [x] Corrupt configuration and missing required executable cases are covered at the public command boundary.
- [x] The command remains inspection-only and does not mutate user configuration.
- [x] Compatibility implications of changing the default exit status are resolved explicitly; if a separate health gate is chosen, its name and behavior are stable and documented.

## Persist classic lifecycle state atomically

**What to build:** Make classic install, update, and uninstall state replacement resilient to interruption and write failures so the previous valid ownership authority remains readable unless the complete replacement is durable.

**Blocked by:** None — can start immediately.

- [x] State replacement uses an atomic same-filesystem publication strategy rather than truncating the live file in place.
- [x] Failed writes or publication leave the previous valid state intact and return an actionable error.
- [x] File permissions and parent-directory behavior remain compatible with existing installations.
- [x] Tests inject failures at relevant persistence boundaries and prove that no corrupt live state is published.
- [x] Classic lifecycle smoke tests continue to pass with fully sandboxed configuration paths.

## Recover truthfully from interrupted classic installs

**What to build:** Represent an interrupted classic installation truthfully so durable creation provenance is retained, incomplete work is not presented as a fully committed installation, and doctor or update can guide recovery without taking ownership of unmanaged artifacts.

**Blocked by:** Persist classic lifecycle state atomically.

- [x] Durable state distinguishes the provenance needed for safe cleanup from confirmed completed installation state.
- [x] A failure after state preparation but before all local and external actions complete leaves an explicit recoverable condition.
- [x] Doctor reports the partial condition and provides a safe next action.
- [x] Update or reinstall can converge the interrupted installation without losing ownership evidence or silently adopting conflicting content.
- [x] Uninstall remains contributor-safe and removes only artifacts and empty containers whose ownership can still be proven.
- [x] Failure-injection tests cover interruption before local writes, between local and external work, and before final commit.

## Add machine-readable health and pack status output

**What to build:** Add stable structured output for doctor and pack status so scripts can consume checks, readiness, blockers, evidence, and pending human actions without parsing presentation-oriented text.

**Blocked by:** Report truthful Matty pack readiness after Apply; Expose doctor failures to automation.

- [x] Doctor exposes a documented versioned structured representation of every check and its severity.
- [x] Pack overview and targeted status expose versioned structured representations of intent, latest attempt, projection summary, readiness, blockers, evidence, and pending human actions.
- [x] Structured output preserves the same exit semantics as human-readable output and readiness gates.
- [x] Human-readable output remains the default and does not regress.
- [x] Output ordering and absent/unknown values are deterministic enough for automation and covered by command-level tests.

## Make the test suite race-safe and enable race CI

**What to build:** Make executable-inspection tests deterministic under Go race instrumentation and add a continuous race check so concurrency regressions are detected without timing-based flakes.

**Blocked by:** None — can start immediately.

- [ ] The executable-version diagnostic test no longer fails because race instrumentation exceeds a fixed wall-clock assumption.
- [ ] The full race-enabled test suite passes repeatedly in an isolated environment.
- [ ] CI runs the race detector with an explicit scope and timeout appropriate for the repository.
- [ ] A race failure fails CI, while ordinary slow process scheduling does not create false failures.
- [ ] Existing formatting, vet, build, and normal test checks remain unchanged and passing.

## Protect installed skill-bundle discovery with boundary tests

**What to build:** Strengthen confidence in skill-bundle discovery across repository, package-installed, and explicit development sources so missing, malformed, or stale sources fail safely with actionable guidance.

**Blocked by:** None — can start immediately.

- [ ] Tests cover repository discovery, initialized Installed Source discovery, and explicit source override precedence.
- [ ] Missing default Installed Source and malformed bundle cases fail before lifecycle mutation with actionable messages.
- [ ] Stale package-installed source behavior remains distinct from the explicit development/test override.
- [ ] Bundle resource validation covers representative valid and invalid skill trees without depending on external clones.
- [ ] Coverage improvement is demonstrated on the previously under-tested discovery boundary, without a percentage-only test target.

## Protect Engram executable discovery with boundary tests

**What to build:** Strengthen confidence in Engram discovery and diagnosis so Homebrew ownership, PATH shadowing, version mismatches, process inspection, and failure cases remain deterministic and safe across supported workstation states.

**Blocked by:** None — can start immediately.

- [ ] Tests cover canonical Homebrew discovery, non-Homebrew PATH resolution, shadowing, and multiple reported versions.
- [ ] Missing executables, failed version commands, and process-inspection errors produce stable diagnoses without mutation.
- [ ] Active process evidence is classified independently from PATH ownership and configured intent.
- [ ] Test fixtures do not rely on the operator's installed Engram binary, real Homebrew prefix, or active processes.
- [ ] Coverage improvement is demonstrated on the previously under-tested executable-discovery boundary, without a percentage-only test target.
