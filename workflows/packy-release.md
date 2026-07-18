# Packy Release

Status: Active

## Goal

Publish or recover one Packy release from an immutable commit in fetched
`origin/main` history, then prove that every published surface agrees before
announcing success.

## Skill shape

The implementation strengthens the existing project-local, model-invoked skill
`release-packy` at `.agents/skills/release-packy/SKILL.md`; it does not create a
second release skill. Fresh publication and existing-tag recovery are branches
of this one release gate. Consultations and release audits without a publication
request do not trigger it.

This workflow is the orchestration source of truth. `docs/release.md` remains
authoritative for the release, artifact, support, and smoke-test contracts, and
`.github/workflows/release.yml` remains authoritative for GitHub Actions
publication behavior. The skill points to these sources instead of copying
their details.

Implementation aligns the `docs/release.md` checklist with this contract: the
controlled package-install smoke is required, while a real Homebrew installation
is an optional verification that requires explicit user consent.

Per repository delegation policy, the primary agent retains the candidate and
version decisions, Git tags, GitHub and tap mutations, integration, and final
verification. Safe read-only investigation and independent verification may be
delegated.

## Workflow

### Trigger

The user explicitly asks to publish Packy from `main`, optionally naming a
version, or to recover publication for one named existing tag. Classify the run
as fresh publication or existing-tag recovery before any external mutation.

Record the operator checkout's initial branch, HEAD, and status. Read this
workflow, `docs/release.md`, and `.github/workflows/release.yml`; fetch `origin`
with tags and pruning; and inspect local tags, remote tags, and GitHub Releases.

### 1. Establish

For fresh publication, freeze the freshly fetched `origin/main` SHA. Use a
user-specified version when present; otherwise select the next patch after the
highest valid published tag or release. An explicit version may advance patch
or minor, but must be a valid `v0.x.y`, be monotonically newer than every
existing tag and GitHub Release, and be absent locally, remotely, and on GitHub.

For recovery, resolve the named existing remote tag and require its local tag,
remote tag, and GitHub Release target, when present, to agree on one unchanged
SHA that remains in `origin/main` history. Recovery repairs publication for that
exact tag and source; it never selects another candidate or version.

Use the operator checkout only for a fresh publication when it is clean, on
`main`, and exactly at the frozen `origin/main` SHA. Otherwise create a clean
temporary worktree at the candidate SHA without stashing, resetting, cleaning,
switching, or otherwise changing the operator checkout. Run every proof and
publication command from the selected workspace.

Confirm GitHub authentication. For fresh publication and dispatch recovery,
also confirm the observable presence of every secret required by the release
contract. The tap token is available only inside GitHub Actions, so its actual
write permission remains a publication gate exercised by the Release workflow.
Note-only recovery instead requires release-edit permission and discloses no tap
mutation. Prepare the exact support statement that the contract requires in the
published release notes.

**Complete when:** the branch, exact tag, immutable candidate SHA, isolated
workspace, initial operator state, required support text, authentication, and
every branch-applicable secret or permission preflight are known; version or tag
state has no conflict or ambiguity; and no external state has changed.

### 2. Prove

From the selected workspace, run every repository-required check, require the
repository CI workflow to have completed successfully for the exact candidate
SHA, and run the controlled package-install lifecycle smoke named by
`docs/release.md` with disposable `HOME` and `XDG_CONFIG_HOME`.

Before publication, use the selected version to build the release artifacts and
`checksums.txt` locally, generate the Homebrew formula from that manifest, and
verify that the disposable outputs satisfy the current release contract. Keep
all proof outputs outside tracked project and operator configuration paths.

**Complete when:** all local checks, exact-SHA CI, the controlled smoke, the
artifact and checksum build, and formula generation have passed, and the
evidence is sufficient for the publication brief.

### 3. Approve

Present one publication brief immediately before the first external publication
mutation: the tag push for a fresh run, or manual dispatch or release edit for a
recovery run. The brief contains the branch, version, immutable SHA, ancestry,
local checks, exact-SHA CI, controlled smoke, disposable artifact/formula proof,
prepared support text, and every external effect approval authorizes. It
discloses, when the branch will run the Release workflow, that tap write
permission cannot be proven from the pre-publication workspace and will first
be exercised inside that workflow. A note-only brief instead states that no tap
or artifact mutation will occur.

Offer a real Homebrew installation as a separate opt-in that names the impact
and controlled target environment. Only an explicit affirmative answer
authorizes it; silence or general release approval keeps the controlled smoke as
the sole package-install gate.

**Complete when:** the user approves publication of the exact briefed tag and
SHA, and the Homebrew opt-in is recorded as approved or declined. Rejection or a
requested change returns to **Establish** without publishing.

### 4. Publish once

Immediately before mutation, fetch again and verify that the approved candidate
still has the required ancestry and identity. Later `origin/main` commits do not
invalidate a frozen candidate while it remains an ancestor of `origin/main`.
History divergence, a rewritten candidate, a newly occupied tag, or any mismatch
with the approved SHA invalidates the gate.

For fresh publication, create the exact tag at the approved SHA, push only that
tag once, locate its tag-triggered Release workflow, and wait for a terminal
result. The published tag is immutable: never delete it for reuse, recreate it,
or move it locally or remotely.

For recovery, use the documented manual-dispatch path only to complete or repair
release artifacts or tap state for the existing tag's unchanged target SHA.
Rebuilt outputs must satisfy the contract for that same tagged source. When the
published notes are the only nonconforming surface and every other final-gate
check already passes, skip dispatch and repair the body directly with
`gh release edit`.

For either workflow-running branch, require its tap permission dry-run and tap
publication step to succeed. A reported no-op is valid only when the step
records that the generated formula already matches; the final gate still reads
the published tap state independently.

After the workflow creates or recovers the GitHub Release, preserve its generated
change notes and use `gh release edit` to add or correct the prepared support
statement. Verify the published body rather than treating prepared text as
evidence.

Diagnose technical failures autonomously. Idempotent retries and external-state
repairs may continue for the same tag and SHA when their effects were disclosed
in the publication brief. If safe repair requires changing code, scripts, or a
workflow file, stop with an exception brief and route that repository change
through issue delivery. After that change merges, begin a new release with a
monotonically newer version; never repair versioned content by moving or reusing
the failed tag.

**Complete when:** the exact approved tag exists at the approved SHA; the Release
workflow reports `completed/success` for that tag; and the published release
notes satisfy the support contract.

### 5. Verify and close

Read every result back from its published boundary. Require the tag and GitHub
Release target to resolve to the approved SHA; the successful workflow run to
name the same tag; the release to contain every contract-required artifact and
checksum entry; and the checksum manifest to validate the downloaded host
binary. Run that binary's version command and require the approved version.

Read the published Homebrew formula and its tap commit through GitHub. Require
its version, URLs, and checksums to match the release and manifest, and record
the exact tap commit containing that formula. Re-read the published release body
and require the support statement. When the user opted into real Homebrew
verification, run it only in the disclosed controlled environment and record
its result.

Verify that the operator checkout retains its initial branch, HEAD, and
pre-existing status. Retain evidence, remove any temporary worktree, and confirm
that the selected release workspace leaves no uncommitted release output.

**Complete when:** workflow, tag, candidate SHA, GitHub Release, published notes,
artifacts, checksum manifest, host binary checksum and version, Homebrew formula,
and tap commit all agree; the controlled smoke passed; any approved real
Homebrew check passed; temporary state is removed; operator state is preserved;
and the success brief records links and evidence for every assertion.

## Checkpoints and exceptions

There is one routine checkpoint: **Approve**. Approval authorizes the disclosed
publication and verification work without further routine checkpoints.

Before approval, any failed proof remains diagnosis and produces no publication
mutation. At any phase, conflicting version or tag state, lost candidate
identity, a required repository change, a product decision, materially broader
scope, real-user configuration, or an undisclosed external effect produces one
decision-ready exception brief. The workflow resumes only from the earliest
phase invalidated by the decision.

## Briefs

The publication brief is the compact approval asset defined in **Approve**. It
links to detailed evidence rather than reproducing raw commands or logs.

The success brief reports the tag and candidate SHA; GitHub Release, workflow,
and tap commit links; release-note result; repository checks and exact-SHA CI;
controlled and optional real Homebrew smoke results; artifact, manifest, host
binary, and formula agreement; temporary-worktree cleanup; and preservation of
operator state.

An exception brief presents the evidence, the invalidated phase, why safe
continuation is impossible, concrete options, and one recommended decision. It
links the relevant run or asset and omits raw logs.

## Definition of done

A release run succeeds only when **Verify and close** is complete. An exception
brief pauses an incomplete run until the user's decision resumes it from the
invalidated phase. This specification is ready when an implementer can
strengthen `release-packy` without asking another question.
