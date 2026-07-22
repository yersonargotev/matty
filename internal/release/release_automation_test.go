package release_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/release"
	"golang.org/x/sys/unix"
)

var supportedReleasePlatforms = []string{
	"darwin/amd64",
	"darwin/arm64",
	"linux/amd64",
	"linux/arm64",
}

func TestReleaseWorkflowBuildsOneManualMainCandidate(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))

	for _, want := range []string{
		"workflow_dispatch:",
		"dry_run:",
		"type: boolean",
		"github.ref == 'refs/heads/main'",
		"git fetch --force origin main 'refs/tags/*:refs/tags/*'",
		`[[ "$head" == "$main" && "$head" == "$tag_commit" ]]`,
		"scripts/build-release-artifacts.sh",
		"sbom.spdx.json",
		"SHA256SUMS",
		"Retain the one built candidate",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow should contain %q", want)
		}
	}
	if strings.Contains(text, "push:\n    tags:") {
		t.Fatal("publication must remain manual-only until protected-tag enforcement is enabled")
	}
	if got := strings.Count(text, "scripts/build-release-artifacts.sh"); got != 1 {
		t.Fatalf("release candidate must be built exactly once; got %d build invocations", got)
	}
}

func TestReleaseWorkflowSealsAndVerifiesProvenanceBeforePublishing(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	workflow := parseReleaseWorkflow(t, text)

	seal := releaseWorkflowStepIndex(t, workflow, "Create immutable candidate and provenance metadata", []string{
		"releasecandidate create",
		"--ref refs/heads/main",
		"--permission attestations=write",
		"--permission contents=write",
		"--permission id-token=write",
	})
	plan := releaseWorkflowStepIndex(t, workflow, "Seal exact publication plan and draft base", []string{
		"formula_sha",
		`repository:"yersonargotev/homebrew-tap"`,
		`path:"Formula/packy.rb"`,
		"sha256:$formula_sha",
	})
	attest := releaseWorkflowStepIndex(t, workflow, "Attest exact retained candidate", []string{
		"actions/attest-build-provenance@977bb373ede98d70efdf65b84cb5f73e068dcc2a",
		"subject-path: 'dist/*'",
	})
	refBeforeAttest := releaseWorkflowStepIndex(t, workflow, "Revalidate refs immediately before OIDC issuance", []string{
		"needs.inspect-release.outputs.has_bundle != 'true'",
		"resolve_ref_commit",
		"diverged before OIDC issuance",
	})
	verify := releaseWorkflowStepIndex(t, workflow, "Verify bundle offline against exact workflow and subjects", []string{
		"gh attestation trusted-root",
		"--bundle \"$bundle\"",
		"--signer-workflow \"$GITHUB_REPOSITORY/.github/workflows/release.yml\"",
		"--source-ref refs/heads/main",
		"--source-digest \"${{ needs.build.outputs.commit }}\"",
		"--signer-digest \"${{ needs.build.outputs.commit }}\"",
		"--custom-trusted-root",
	})
	envelope := releaseWorkflowStepIndex(t, workflow, "Bind attestation and destination plan into the immutable release set", []string{
		"draft-base.json",
		"attestation.bundle.jsonl",
		"bundle_base64",
		"jq -cnS --slurpfile base",
		"release_set_id",
		"release-body.md",
	})
	publish := releaseWorkflowStepIndex(t, workflow, "Create or verify draft, upload only missing assets, and publish once", []string{
		"gh api graphql",
		"release(tagName:$tag){id}",
		`if [[ -z "$release_id" ]]`,
		"gh release create",
		"--draft",
		"verify-state",
		"--mode draft",
		"gh release upload",
		"assert_server_hashes",
		"gh release edit",
		"--draft=false",
	})

	assertReleaseWorkflowStepBefore(t, seal, plan, "candidate identity must precede the complete destination plan")
	assertReleaseWorkflowStepBefore(t, plan, attest, "the complete destination plan must be sealed before OIDC provenance is issued")
	assertReleaseWorkflowStepBefore(t, refBeforeAttest, attest, "refs must be revalidated immediately before OIDC provenance is issued")
	assertReleaseWorkflowStepBefore(t, attest, verify, "the generated bundle must be verified before publication")
	assertReleaseWorkflowStepBefore(t, verify, envelope, "the verified bundle must be bound into the final release set")
	assertReleaseWorkflowStepBefore(t, envelope, publish, "the complete release set must reach the exact draft before one-time publication")
	for _, forbidden := range []string{"--clobber", "gh release delete", "git tag -", "git push origin refs/tags"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("immutable publication workflow must not contain %q", forbidden)
		}
	}
	if strings.Contains(text, "if ! gh release view") || strings.Contains(text, "targetCommitish") {
		t.Fatal("a failed release lookup is ambiguous; absence must be proved by a successful API query")
	}
	publishStep := releaseWorkflowStep(t, workflow, "Create or verify draft, upload only missing assets, and publish once").Text
	firstRefCheck := strings.Index(publishStep, "assert_ref_identity >/dev/null")
	create := strings.Index(publishStep, "gh release create")
	finalRefCheck := strings.LastIndex(publishStep, "assert_ref_identity >/dev/null")
	publishEdit := strings.Index(publishStep, "gh release edit")
	if firstRefCheck < 0 || create < 0 || finalRefCheck < 0 || publishEdit < 0 || !(firstRefCheck < create && create < finalRefCheck && finalRefCheck < publishEdit) {
		t.Fatal("tag and protected main must be revalidated immediately before draft creation and publication")
	}
	if got := strings.Count(publishStep, "assert_ref_identity >/dev/null"); got < 4 {
		t.Fatalf("every draft creation, asset upload phase, and publication needs an adjacent ref recheck; got %d", got)
	}
}

func TestReleaseWorkflowKeepsDryRunAndDestinationAuthoritySeparate(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	dryRun := releaseWorkflowJob(t, text, "dry-run")
	inspect := releaseWorkflowJob(t, text, "inspect-release")
	attest := releaseWorkflowJob(t, text, "attest")
	github := releaseWorkflowJob(t, text, "publish-github")
	homebrew := releaseWorkflowJob(t, text, "homebrew")

	for _, want := range []string{
		"if: inputs.dry_run == true",
		"RELEASE_EFFECTS: ${{ needs.inspect-release.outputs.effects }}",
		"Exact effects a real run would perform from the observed state",
		"Dry-run stopped before OIDC issuance or any GitHub Release/Homebrew mutation",
	} {
		if !strings.Contains(dryRun, want) {
			t.Fatalf("dry-run job should contain %q", want)
		}
	}
	for _, forbidden := range []string{"id-token: write", "attest-build-provenance", "gh release create", "gh release upload", "git push origin"} {
		if strings.Contains(dryRun, forbidden) {
			t.Fatalf("dry-run job must stop before mutation authority %q", forbidden)
		}
	}
	for _, want := range []string{"Download built candidate for read-only attestation checks", "gh attestation verify", "--custom-trusted-root", "canonical-body.md", "effects=$effects"} {
		if !strings.Contains(inspect, want) {
			t.Fatalf("read-only inspection should verify an available existing bundle with %q", want)
		}
	}
	if strings.Contains(inspect, "id-token: write") || strings.Contains(inspect, "attest-build-provenance") {
		t.Fatal("read-only existing-bundle verification must not request or issue OIDC")
	}
	for _, want := range []string{"contents: read", "id-token: write", "attestations: write"} {
		if !strings.Contains(attest, want) {
			t.Fatalf("attestation job should have narrow permission %q", want)
		}
	}
	if strings.Contains(attest, "HOMEBREW_TAP_TOKEN") || strings.Contains(attest, "contents: write") {
		t.Fatal("attestation authority must not receive release or Homebrew write authority")
	}
	if !strings.Contains(github, "contents: write") || strings.Contains(github, "id-token: write") || strings.Contains(github, "HOMEBREW_TAP_TOKEN") {
		t.Fatal("GitHub publication must have only its contents write boundary")
	}
	for _, want := range []string{"contents: read", "HOMEBREW_TAP_TOKEN", "repository: yersonargotev/homebrew-tap", "persist-credentials: true"} {
		if !strings.Contains(homebrew, want) {
			t.Fatalf("Homebrew job should contain isolated tap boundary %q", want)
		}
	}
	if strings.Contains(homebrew, "id-token: write") || strings.Contains(homebrew, "contents: write") {
		t.Fatal("Homebrew job must not receive GitHub release or attestation authority")
	}
	if got := strings.Count(homebrew, "HOMEBREW_TAP_TOKEN"); got != 1 {
		t.Fatalf("Homebrew token must appear only in the post-readback tap checkout; got %d references", got)
	}
}

func TestReleaseEvidenceVerifierRequiresExactCandidateParity(t *testing.T) {
	root := repoRoot(t)
	tag := "v0.99.0"
	commit := gitOutput(t, root, "rev-parse", "HEAD")
	dist := filepath.Join(t.TempDir(), "dist")
	if err := os.MkdirAll(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, asset := range releaseAssets(tag) {
		path := filepath.Join(dist, asset)
		if err := os.WriteFile(path, []byte("candidate "+asset), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	sbomSource := filepath.Join(t.TempDir(), release.SBOMName)
	generateSBOM := exec.Command("go", "run", "./internal/tools/releasesbom", "--version", tag, "--created", gitOutput(t, root, "show", "-s", "--format=%cI", commit), "--dist", dist, "--out", sbomSource)
	generateSBOM.Dir = root
	generateSBOM.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	if output, err := generateSBOM.CombinedOutput(); err != nil {
		t.Fatalf("generate SBOM: %v\n%s", err, output)
	}
	sbom, err := os.ReadFile(sbomSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, release.SBOMName), sbom, 0o600); err != nil {
		t.Fatal(err)
	}
	var checksumLines []string
	for _, name := range append(releaseAssets(tag), release.SBOMName) {
		checksumLines = append(checksumLines, sha256File(t, filepath.Join(dist, name))+"  "+name)
	}
	sort.Strings(checksumLines)
	if err := os.WriteFile(filepath.Join(dist, release.ChecksumsName), []byte(strings.Join(checksumLines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(t.TempDir(), "metadata")
	formula := filepath.Join(metadata, "packy.rb")
	generate := exec.Command("bash", filepath.Join(root, "scripts", "generate-homebrew-formula.sh"), "--version", tag, "--checksums", filepath.Join(dist, release.ChecksumsName), "--out", formula)
	generate.Dir = root
	generate.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate formula: %v\n%s", err, output)
	}

	evidenceRoot := filepath.Join(t.TempDir(), "evidence")
	if err := os.MkdirAll(evidenceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeBin := t.TempDir()
	canonicalLog := filepath.Join(t.TempDir(), "canonical-validator.log")
	fakeGo := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CANONICAL_LOG"
if [ "${1:-}" = run ] && [ "${2:-}" = ./internal/tools/releasesbom ]; then
  out=""
  while [ "$#" -gt 0 ]; do
    if [ "$1" = --out ]; then out="$2"; break; fi
    shift
  done
  cp "$FAKE_SBOM" "$out"
  exit 0
fi
[ "${FAIL_CANONICAL:-0}" != 1 ]
`
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte(fakeGo), 0o755); err != nil {
		t.Fatal(err)
	}

	notes := filepath.Join(metadata, "release-notes.md")
	run := func(failCanonical bool) ([]byte, error) {
		cmd := exec.Command("bash", filepath.Join(root, "scripts", "verify-release-evidence.sh"),
			"--tag", tag, "--commit", commit, "--dist", dist, "--evidence-root", evidenceRoot,
			"--formula", formula, "--notes-template", filepath.Join(root, "docs", "release-notes", "next.md"), "--notes-output", notes)
		cmd.Dir = root
		fail := "0"
		if failCanonical {
			fail = "1"
		}
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "CANONICAL_LOG="+canonicalLog, "FAKE_SBOM="+filepath.Join(dist, release.SBOMName), "FAIL_CANONICAL="+fail)
		return cmd.CombinedOutput()
	}
	if output, err := run(false); err != nil {
		t.Fatalf("verify complete candidate: %v\n%s", err, output)
	}
	if rendered, err := os.ReadFile(notes); err != nil || !strings.Contains(string(rendered), "# "+tag) || strings.Contains(string(rendered), "{{TAG}}") {
		t.Fatalf("rendered notes are not tag-bound: %v\n%s", err, rendered)
	}
	invocation, err := os.ReadFile(canonicalLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"run ./internal/tools/claudesmoke verify-release", "--evidence-root " + evidenceRoot, "--packy-version " + tag, "--packy-sha " + commit} {
		if !strings.Contains(string(invocation), want) {
			t.Fatalf("release verifier did not delegate %q to the canonical owner:\n%s", want, invocation)
		}
	}
	unexpectedDir := filepath.Join(dist, "unexpected-directory")
	if err := os.Mkdir(unexpectedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := run(false); err == nil || !strings.Contains(string(output), "incomplete or unexpected") {
		t.Fatalf("release verifier accepted unexpected directory: %v\n%s", err, output)
	}
	if err := os.Remove(unexpectedDir); err != nil {
		t.Fatal(err)
	}
	unexpectedLink := filepath.Join(dist, "unexpected-link")
	if err := os.Symlink(filepath.Join(dist, releaseAssets(tag)[0]), unexpectedLink); err != nil {
		t.Fatal(err)
	}
	if output, err := run(false); err == nil || !strings.Contains(string(output), "incomplete or unexpected") {
		t.Fatalf("release verifier accepted unexpected symlink: %v\n%s", err, output)
	}
	if err := os.Remove(unexpectedLink); err != nil {
		t.Fatal(err)
	}
	unexpectedFIFO := filepath.Join(dist, "unexpected-fifo")
	if err := unix.Mkfifo(unexpectedFIFO, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := run(false); err == nil || !strings.Contains(string(output), "incomplete or unexpected") {
		t.Fatalf("release verifier accepted unexpected FIFO: %v\n%s", err, output)
	}
	if err := os.Remove(unexpectedFIFO); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(dist, releaseAssets(tag)[0])
	victimBytes, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(victim); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dist, releaseAssets(tag)[1]), victim); err != nil {
		t.Fatal(err)
	}
	if output, err := run(false); err == nil || !strings.Contains(string(output), "not a regular non-symlink") {
		t.Fatalf("release verifier accepted expected-name symlink: %v\n%s", err, output)
	}
	if err := os.Remove(victim); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, victimBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := run(true); err == nil {
		t.Fatalf("release verifier ignored canonical evidence rejection:\n%s", output)
	}
}

func TestReleaseWorkflowVerifiesPublishedGitHubBytesBeforeHomebrew(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	homebrew := releaseWorkflowJob(t, text, "homebrew")
	for _, want := range []string{
		"needs: [build, validate-release-evidence, publish-github]",
		"needs.publish-github.outputs.published == 'true'",
		"Independently read back exact published GitHub assets",
		`resolve_ref_commit "tags/$RELEASE_TAG"`,
		"resolve_ref_commit heads/main",
		"cmp attestation/release-body.md",
		"attestation.bundle.jsonl",
		"cmp \"$RUNNER_TEMP/expected-assets\" \"$RUNNER_TEMP/actual-assets\"",
		"sha256sum --check SHA256SUMS",
		"publication_plan.homebrew.sha256",
		"diverged before tap push",
		"diverged after tap push proof",
		"tap remote formula does not match the sealed destination plan",
		"git push --dry-run origin HEAD:main",
		"git push origin HEAD:main",
	} {
		if !strings.Contains(homebrew, want) {
			t.Fatalf("Homebrew publication should contain %q", want)
		}
	}
	readBack := strings.Index(homebrew, "Independently read back exact published GitHub assets")
	checkout := strings.Index(homebrew, "Check out Homebrew tap with only its scoped credential")
	push := strings.Index(homebrew, "git push origin HEAD:main")
	if readBack < 0 || checkout < 0 || push < 0 || !(readBack < checkout && checkout < push) {
		t.Fatal("published GitHub bytes must be independently verified before tap checkout and push")
	}
	for _, forbidden := range []string{"formula_renames.json", "FormulaRenames", "yersonargotev/matty", "Formula/matty.rb =>"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("release workflow must not contain legacy distribution identity %q", forbidden)
		}
	}
}

func TestBuildReleaseArtifactsCreatesChecksummedSupportedPlatforms(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles release artifacts")
	}

	root := repoRoot(t)
	outDir := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "build-release-artifacts.sh"), "--version", "v0.1.7", "--out-dir", outDir)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "GOCACHE="+t.TempDir(), "GOMODCACHE="+goEnv(t, "GOMODCACHE"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build release artifacts: %v\n%s", err, output)
	}

	wantAssets := releaseAssets("v0.1.7")
	wantEntries := append(append([]string{}, wantAssets...), release.ChecksumsName, release.SBOMName)
	sort.Strings(wantEntries)

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	var gotEntries []string
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("release output contains unexpected directory %s", entry.Name())
		}
		gotEntries = append(gotEntries, entry.Name())
	}
	sort.Strings(gotEntries)
	if strings.Join(gotEntries, "\n") != strings.Join(wantEntries, "\n") {
		t.Fatalf("v0.1.7 release directory mismatch\nwant:\n%s\ngot:\n%s", strings.Join(wantEntries, "\n"), strings.Join(gotEntries, "\n"))
	}

	checksums := readChecksums(t, filepath.Join(outDir, release.ChecksumsName))
	for _, asset := range wantAssets {
		gotChecksum, ok := checksums[asset]
		if !ok {
			t.Fatalf("SHA256SUMS missing checksum for %s", asset)
		}
		if gotChecksum != sha256File(t, filepath.Join(outDir, asset)) {
			t.Fatalf("checksum for %s does not match artifact bytes", asset)
		}
	}
	if got := checksums[release.SBOMName]; got != sha256File(t, filepath.Join(outDir, release.SBOMName)) {
		t.Fatalf("SHA256SUMS does not bind the SBOM: %q", got)
	}
	if len(checksums) != len(wantAssets)+1 {
		t.Fatalf("SHA256SUMS should contain exactly binaries and SBOM; got %d entries", len(checksums))
	}
}

func TestBuildReleaseArtifactsValidatesReleaseVersionBeforeBuilding(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "build-release-artifacts.sh")

	t.Run("accepts v0 x y release tags", func(t *testing.T) {
		for _, version := range []string{"v0.1.0", "v0.2.3", "v0.99.0"} {
			t.Run(version, func(t *testing.T) {
				fakeBin, logPath := fakeGoBuild(t)
				outDir := t.TempDir()
				cmd := exec.Command("bash", script, "--version", version, "--out-dir", outDir)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())

				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("build release artifacts should accept %s: %v\n%s", version, err, output)
				}
				if _, err := os.Stat(logPath); err != nil {
					t.Fatalf("expected accepted version %s to reach go build: %v", version, err)
				}
				log, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatalf("read go build log: %v", err)
				}
				wantLdflag := "-X github.com/yersonargotev/packy/internal/version.Value=" + version
				if !strings.Contains(string(log), wantLdflag) {
					t.Fatalf("release build should inject version with ldflags %q\nlog:\n%s", wantLdflag, log)
				}
			})
		}
	})

	t.Run("rejects non-v0 and malformed versions before building", func(t *testing.T) {
		for _, version := range []string{"v1.0.0", "v0.2", "v0.1.0-rc.1", "0.1.0", "main", ""} {
			t.Run(fmt.Sprintf("%q", version), func(t *testing.T) {
				fakeBin, logPath := fakeGoBuild(t)
				outDir := t.TempDir()
				cmd := exec.Command("bash", script, "--version", version, "--out-dir", outDir)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())

				output, err := cmd.CombinedOutput()
				if err == nil {
					t.Fatalf("build release artifacts should reject version %q\n%s", version, output)
				}
				if !strings.Contains(string(output), "Release version must be a v0.x.y tag") {
					t.Fatalf("rejection should explain v0.x.y requirement, got:\n%s", output)
				}
				if _, err := os.Stat(logPath); !os.IsNotExist(err) {
					t.Fatalf("invalid version %q should fail before go build; stat error: %v", version, err)
				}
			})
		}
	})
}

func TestGenerateHomebrewFormulaUsesChecksummedReleaseArtifacts(t *testing.T) {
	root := repoRoot(t)
	checksumsPath := writeChecksumManifest(t, validFormulaChecksumLines("v0.99.0"))
	outputPath := filepath.Join(t.TempDir(), "Formula", "packy.rb")

	cmd := exec.Command(
		"bash",
		filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
		"--version", "v0.99.0",
		"--checksums", checksumsPath,
		"--out", outputPath,
		"--repo", "yersonargotev/packy",
		"--homepage", "https://github.com/yersonargotev/packy",
		"--desc", "AI coding workflow installer",
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate formula: %v\n%s", err, output)
	}

	formula, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(formula)
	for _, want := range []string{
		"class Packy < Formula",
		`desc "AI coding workflow installer"`,
		`homepage "https://github.com/yersonargotev/packy"`,
		`version "0.99.0"`,
		`url "https://github.com/yersonargotev/packy/releases/download/v0.99.0/packy_v0.99.0_darwin_amd64", using: :nounzip`,
		`sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`url "https://github.com/yersonargotev/packy/releases/download/v0.99.0/packy_v0.99.0_darwin_arm64", using: :nounzip`,
		`sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`,
		`url "https://github.com/yersonargotev/packy/releases/download/v0.99.0/packy_v0.99.0_linux_amd64", using: :nounzip`,
		`sha256 "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`,
		`url "https://github.com/yersonargotev/packy/releases/download/v0.99.0/packy_v0.99.0_linux_arm64", using: :nounzip`,
		`sha256 "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"`,
		`downloaded_binary = Dir["packy_*"].first`,
		`odie "downloaded packy binary not found" if downloaded_binary.nil?`,
		`bin.install downloaded_binary => "packy"`,
		`system "#{bin}/packy", "--version"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formula should contain %q\nformula:\n%s", want, text)
		}
	}
	if got := strings.Count(text, "using: :nounzip"); got != len(supportedReleasePlatforms) {
		t.Fatalf("formula should mark every raw executable URL as using: :nounzip; got %d occurrences in:\n%s", got, text)
	}
}

func TestGenerateHomebrewFormulaFailsClearlyWhenChecksumEntryIsMissing(t *testing.T) {
	root := repoRoot(t)
	checksumsPath := writeChecksumManifest(t, validFormulaChecksumLines("v0.99.0")[:3])
	outputPath := filepath.Join(t.TempDir(), "Formula", "packy.rb")

	cmd := exec.Command(
		"bash",
		filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
		"--version", "v0.99.0",
		"--checksums", checksumsPath,
		"--out", outputPath,
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("generate formula should fail when a checksum entry is missing\n%s", output)
	}
	if !strings.Contains(string(output), "missing checksum entry for packy_v0.99.0_linux_arm64") {
		t.Fatalf("failure should name the missing artifact, got:\n%s", output)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("formula should not be written with incomplete checksums; stat error: %v", err)
	}
}

func TestGenerateHomebrewFormulaFailsClearlyWhenChecksumManifestIsNotExact(t *testing.T) {
	root := repoRoot(t)
	baseChecksums := validFormulaChecksumLines("v0.99.0")

	tests := []struct {
		name      string
		extraLine string
		wantError string
	}{
		{
			name:      "rejects unexpected release artifact",
			extraLine: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee  packy_v0.99.0_linux_386",
			wantError: "unexpected checksum entry for packy_v0.99.0_linux_386",
		},
		{
			name:      "rejects duplicate expected artifact",
			extraLine: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff  packy_v0.99.0_darwin_amd64",
			wantError: "duplicate checksum entry for packy_v0.99.0_darwin_amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksums := append(append([]string{}, baseChecksums...), tt.extraLine)
			checksumsPath := writeChecksumManifest(t, checksums)
			outputPath := filepath.Join(t.TempDir(), "Formula", "packy.rb")

			cmd := exec.Command(
				"bash",
				filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
				"--version", "v0.99.0",
				"--checksums", checksumsPath,
				"--out", outputPath,
			)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("generate formula should fail when the checksum manifest is not exact\n%s", output)
			}
			if !strings.Contains(string(output), tt.wantError) {
				t.Fatalf("failure should explain the manifest mismatch, got:\n%s", output)
			}
			if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
				t.Fatalf("formula should not be written with invalid checksum manifest; stat error: %v", err)
			}
		})
	}
}

func readReleaseWorkflow(t *testing.T, root string) string {
	t.Helper()
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(workflow)
}

func releaseWorkflowJob(t *testing.T, workflow, name string) string {
	t.Helper()
	marker := "\n  " + name + ":\n"
	start := strings.Index(workflow, marker)
	if start < 0 {
		t.Fatalf("release workflow missing job %q", name)
	}
	start++
	end := len(workflow)
	for offset, line := range strings.Split(workflow[start:], "\n") {
		if offset > 0 && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(line, ":") {
			candidate := strings.Index(workflow[start:], "\n"+line+"\n")
			if candidate >= 0 {
				end = start + candidate
				break
			}
		}
	}
	return workflow[start:end]
}

type workflowStep struct {
	Name  string
	Index int
	Text  string
}

func parseReleaseWorkflow(t *testing.T, workflow string) []workflowStep {
	t.Helper()
	const stepMarker = "\n      - name: "
	var steps []workflowStep
	searchFrom := 0
	for {
		markerIndex := strings.Index(workflow[searchFrom:], stepMarker)
		if markerIndex < 0 {
			break
		}
		stepStart := searchFrom + markerIndex + 1
		nameStart := stepStart + len("      - name: ")
		nameEnd := strings.IndexByte(workflow[nameStart:], '\n')
		if nameEnd < 0 {
			t.Fatalf("release workflow step at byte %d is missing a newline after its name", stepStart)
		}
		nameEnd += nameStart
		nextMarker := strings.Index(workflow[nameEnd:], stepMarker)
		stepEnd := len(workflow)
		if nextMarker >= 0 {
			stepEnd = nameEnd + nextMarker
		}
		steps = append(steps, workflowStep{
			Name:  strings.TrimSpace(workflow[nameStart:nameEnd]),
			Index: stepStart,
			Text:  workflow[stepStart:stepEnd],
		})
		searchFrom = stepEnd
	}
	if len(steps) == 0 {
		t.Fatal("release workflow contains no job steps")
	}
	return steps
}

func releaseWorkflowStepIndex(t *testing.T, workflow []workflowStep, name string, requiredFragments []string) int {
	t.Helper()
	step := releaseWorkflowStep(t, workflow, name)
	for _, fragment := range requiredFragments {
		if !strings.Contains(step.Text, fragment) {
			t.Fatalf("release workflow step %q should contain %q\nstep:\n%s", name, fragment, step.Text)
		}
	}
	return step.Index
}

func releaseWorkflowStep(t *testing.T, workflow []workflowStep, name string) workflowStep {
	t.Helper()
	for _, step := range workflow {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("release workflow missing step %q", name)
	return workflowStep{}
}

func assertReleaseWorkflowStepBefore(t *testing.T, earlier, later int, reason string) {
	t.Helper()
	if earlier < 0 || later < 0 {
		t.Fatalf("cannot compare missing workflow steps: earlier=%d later=%d", earlier, later)
	}
	if earlier >= later {
		t.Fatalf("release workflow ordering violation: %s", reason)
	}
}

func validFormulaChecksumLines(version string) []string {
	return []string{
		fmt.Sprintf("%s  packy_%s_darwin_amd64", strings.Repeat("a", sha256.Size*2), version),
		fmt.Sprintf("%s  packy_%s_darwin_arm64", strings.Repeat("b", sha256.Size*2), version),
		fmt.Sprintf("%s  packy_%s_linux_amd64", strings.Repeat("c", sha256.Size*2), version),
		fmt.Sprintf("%s  packy_%s_linux_arm64", strings.Repeat("d", sha256.Size*2), version),
		fmt.Sprintf("%s  %s", strings.Repeat("e", sha256.Size*2), release.SBOMName),
	}
}

func writeChecksumManifest(t *testing.T, lines []string) string {
	t.Helper()
	checksumsPath := filepath.Join(t.TempDir(), "checksums.txt")
	checksums := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(checksumsPath, []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	return checksumsPath
}

func fakeGoBuild(t *testing.T) (string, string) {
	t.Helper()
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go-build.log")
	goPath := filepath.Join(dir, "go")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> %q
if [[ "${1:-}" == "run" ]]; then
  exec %q "$@"
fi
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    out="${2:-}"
    break
  fi
  shift
done
if [[ -n "$out" ]]; then
  mkdir -p "$(dirname "$out")"
  printf 'fake binary for %%s\n' "$(basename "$out")" > "$out"
fi
`, logPath, realGo)
	if err := os.WriteFile(goPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, logPath
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output))
}

func releaseAssets(version string) []string {
	assets := make([]string, 0, len(supportedReleasePlatforms))
	for _, platform := range supportedReleasePlatforms {
		parts := strings.Split(platform, "/")
		assets = append(assets, fmt.Sprintf("packy_%s_%s_%s", version, parts[0], parts[1]))
	}
	sort.Strings(assets)
	return assets
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	cmd := exec.Command("go", "env", key)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env %s: %v", key, err)
	}
	return strings.TrimSpace(string(output))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readChecksums(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("checksum line should be '<sha256>  <asset>', got %q", scanner.Text())
		}
		if len(fields[0]) != sha256.Size*2 {
			t.Fatalf("checksum for %s should be SHA-256 hex, got %q", fields[1], fields[0])
		}
		if strings.Contains(fields[1], string(os.PathSeparator)) {
			t.Fatalf("checksum entry should use asset filename only, got %q", fields[1])
		}
		if _, ok := checksums[fields[1]]; ok {
			t.Fatalf("duplicate checksum entry for %s", fields[1])
		}
		checksums[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return checksums
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
