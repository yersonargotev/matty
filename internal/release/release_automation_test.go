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
)

var supportedReleasePlatforms = []string{
	"darwin/amd64",
	"darwin/arm64",
	"linux/amd64",
	"linux/arm64",
}

func TestReleaseWorkflowPublishesPackyArtifactsAndTapFormula(t *testing.T) {
	root := repoRoot(t)
	text := readReleaseWorkflow(t, root)

	for _, want := range []string{
		"workflow_dispatch:",
		"tag:",
		"push:",
		"- 'v0.*.*'",
		"required: true",
		"actions/checkout@",
		"fetch-depth: 0",
		"actions/setup-go@",
		"go-version-file: go.mod",
		"git checkout --detach \"$tag\"",
		`echo "commit=$(git rev-parse HEAD)" >> "$GITHUB_OUTPUT"`,
		"scripts/build-release-artifacts.sh",
		"--out-dir dist",
		"HOMEBREW_TAP_TOKEN",
		"yersonargotev/homebrew-tap",
		"scripts/generate-homebrew-formula.sh",
		"--checksums dist/checksums.txt",
		"--out release-metadata/packy.rb",
		"--repo yersonargotev/packy",
		"gh release upload",
		"dist/* --clobber",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow should contain %q so GitHub Releases and the Homebrew tap stay in sync", want)
		}
	}
}

func TestReleaseWorkflowCreatesOrVerifiesReleaseWithProvedNotes(t *testing.T) {
	root := repoRoot(t)
	step := releaseWorkflowStep(t, parseReleaseWorkflow(t, readReleaseWorkflow(t, root)), "Create GitHub Release if needed")

	if !strings.Contains(step.Text, "gh release view") {
		t.Fatalf("release creation step should be idempotent by checking whether the release exists; step:\n%s", step.Text)
	}
	if !strings.Contains(step.Text, "gh release create") {
		t.Fatalf("release creation step should create the GitHub Release; step:\n%s", step.Text)
	}
	if !strings.Contains(step.Text, "--notes-file release-metadata/release-notes.md") {
		t.Fatalf("release creation should use the validated notes candidate; step:\n%s", step.Text)
	}
	if !strings.Contains(step.Text, "cmp") || !strings.Contains(step.Text, "--json body") {
		t.Fatalf("an existing immutable release should fail closed when its notes differ; step:\n%s", step.Text)
	}
	if strings.Contains(step.Text, "--generate-notes") {
		t.Fatalf("release creation must not bypass validated release notes; step:\n%s", step.Text)
	}
}

func TestReleaseWorkflowValidatesCompleteEvidenceBeforePublication(t *testing.T) {
	text := readReleaseWorkflow(t, repoRoot(t))
	for _, want := range []string{
		"validate-release-evidence:",
		"needs: [build, claude-smoke]",
		"pattern: claude-release-*",
		"scripts/verify-release-evidence.sh",
		"docs/release-notes/next.md",
		"scripts/generate-homebrew-formula.sh",
		"name: packy-release-metadata-${{ needs.build.outputs.tag }}",
		"needs: [build, validate-release-evidence]",
		"name: packy-release-metadata-${{ steps.release.outputs.tag }}",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow missing fail-closed evidence gate %q", want)
		}
	}
	publication := text[strings.LastIndex(text, "  release:"):]
	if strings.Contains(publication, "scripts/generate-homebrew-formula.sh") {
		t.Fatal("publication must consume the proved formula instead of regenerating it")
	}
}

func TestReleaseEvidenceVerifierRequiresExactCandidateParity(t *testing.T) {
	root := repoRoot(t)
	tag := "v0.99.0"
	commit := strings.Repeat("a", 40)
	dist := filepath.Join(t.TempDir(), "dist")
	if err := os.MkdirAll(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	var checksumLines []string
	for _, asset := range releaseAssets(tag) {
		path := filepath.Join(dist, asset)
		if err := os.WriteFile(path, []byte("candidate "+asset), 0o700); err != nil {
			t.Fatal(err)
		}
		checksumLines = append(checksumLines, sha256File(t, path)+"  "+asset)
	}
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte(strings.Join(checksumLines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(t.TempDir(), "metadata")
	formula := filepath.Join(metadata, "packy.rb")
	generate := exec.Command("bash", filepath.Join(root, "scripts", "generate-homebrew-formula.sh"), "--version", tag, "--checksums", filepath.Join(dist, "checksums.txt"), "--out", formula)
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
	fakeGo := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CANONICAL_LOG\"\n[ \"${FAIL_CANONICAL:-0}\" != 1 ]\n"
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
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "CANONICAL_LOG="+canonicalLog, "FAIL_CANONICAL="+fail)
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
	if output, err := run(true); err == nil {
		t.Fatalf("release verifier ignored canonical evidence rejection:\n%s", output)
	}
}

func TestReleaseWorkflowProvesTapAccessBeforePublishingReleaseAssets(t *testing.T) {
	root := repoRoot(t)
	text := readReleaseWorkflow(t, root)
	workflow := parseReleaseWorkflow(t, text)

	resolveTagIndex := releaseWorkflowStepIndex(t, workflow, "Verify release tag remains bound", []string{
		`candidate="${{ needs.build.outputs.commit }}"`,
		`current="$(git rev-parse "${tag}^{commit}")"`,
		`git checkout --detach "$candidate"`,
	})
	setupGoIndex := releaseWorkflowStepIndex(t, workflow, "Set up Go from proved commit", []string{
		"uses: actions/setup-go@",
		"go-version-file: go.mod",
	})
	provedArtifactIndex := releaseWorkflowStepIndex(t, workflow, "Download proved release artifacts and checksums.txt", []string{
		"uses: actions/download-artifact@", "path: dist",
	})
	requireTapTokenIndex := releaseWorkflowStepIndex(t, workflow, "Require Homebrew tap token", []string{
		"HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}",
		"HOMEBREW_TAP_TOKEN is required",
		"yersonargotev/homebrew-tap",
	})
	tapCheckoutIndex := releaseWorkflowStepIndex(t, workflow, "Check out Homebrew tap", []string{
		"uses: actions/checkout@",
		"repository: yersonargotev/homebrew-tap",
		"path: homebrew-tap",
		"token: ${{ secrets.HOMEBREW_TAP_TOKEN }}",
	})
	formulaIndex := releaseWorkflowStepIndex(t, workflow, "Install proved Homebrew formula candidate", []string{
		"cp release-metadata/packy.rb homebrew-tap/Formula/packy.rb",
	})
	prepareTapIndex := releaseWorkflowStepIndex(t, workflow, "Prepare Homebrew tap formula update", []string{
		"id: prepare_tap",
		"working-directory: homebrew-tap",
		`git config user.name "github-actions[bot]"`,
		`git config user.email "github-actions[bot]@users.noreply.github.com"`,
		"git rm --ignore-unmatch Formula/matty.rb",
		"git add Formula/packy.rb",
		"git diff --cached --quiet",
		`echo "changed=false" >> "$GITHUB_OUTPUT"`,
		`echo "changed=true" >> "$GITHUB_OUTPUT"`,
		`git commit -m "feat: update packy formula to ${RELEASE_TAG}"`,
	})
	tapPushAccessProofIndex := releaseWorkflowStepIndex(t, workflow, "Prove Homebrew tap push permission", []string{
		"working-directory: homebrew-tap",
		"git push --dry-run origin HEAD:main",
	})
	reverifyTagIndex := releaseWorkflowStepIndex(t, workflow, "Reverify release tag before publication", []string{
		`current="$(git rev-parse "${{ needs.build.outputs.tag }}^{commit}")"`,
		`candidate="${{ needs.build.outputs.commit }}"`,
	})
	createReleaseIndex := releaseWorkflowStepIndex(t, workflow, "Create GitHub Release if needed", []string{
		"GH_TOKEN: ${{ github.token }}",
		"gh release create",
		"--notes-file release-metadata/release-notes.md",
	})
	uploadIndex := releaseWorkflowStepIndex(t, workflow, "Upload release assets", []string{
		"GH_TOKEN: ${{ github.token }}",
		"gh release upload",
		"dist/* --clobber",
	})
	pushTapIndex := releaseWorkflowStepIndex(t, workflow, "Push prepared Homebrew tap formula update", []string{
		"working-directory: homebrew-tap",
		"TAP_UPDATE_CHANGED: ${{ steps.prepare_tap.outputs.changed }}",
		`[[ "$TAP_UPDATE_CHANGED" != "true" ]]`,
		"git push origin HEAD:main",
	})

	prepareTapStep := releaseWorkflowStep(t, workflow, "Prepare Homebrew tap formula update").Text
	for _, forbidden := range []string{"formula_renames.json", "FormulaRenames", "yersonargotev/matty", "Formula/matty.rb =>"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("release workflow must not contain legacy formula rename metadata or Matty distribution identity %q", forbidden)
		}
	}
	if !strings.Contains(prepareTapStep, "git rm --ignore-unmatch Formula/matty.rb") {
		t.Fatal("tap update must remove the legacy Matty formula in the same commit as Formula/packy.rb")
	}

	if strings.Contains(releaseWorkflowStep(t, workflow, "Prove Homebrew tap push permission").Text, "git commit") {
		t.Fatalf("tap push proof must dry-run the prepared local commit without creating another commit")
	}
	if strings.Contains(releaseWorkflowStep(t, workflow, "Push prepared Homebrew tap formula update").Text, "git push --dry-run") {
		t.Fatalf("final tap push must be mutating, not another dry run")
	}

	assertReleaseWorkflowStepBefore(t, resolveTagIndex, setupGoIndex, "Go must be set up from the checked-out release tag, not the workflow dispatch ref")
	assertReleaseWorkflowStepBefore(t, setupGoIndex, provedArtifactIndex, "proved artifacts should be downloaded after the release tag checkout and Go setup")
	assertReleaseWorkflowStepBefore(t, provedArtifactIndex, formulaIndex, "the proved formula must be installed alongside the exact proved artifacts")
	assertReleaseWorkflowStepBefore(t, requireTapTokenIndex, tapCheckoutIndex, "the workflow must reject a missing HOMEBREW_TAP_TOKEN before falling back to anonymous tap checkout")
	assertReleaseWorkflowStepBefore(t, requireTapTokenIndex, createReleaseIndex, "a missing HOMEBREW_TAP_TOKEN must fail before creating a GitHub Release")
	assertReleaseWorkflowStepBefore(t, requireTapTokenIndex, uploadIndex, "a missing HOMEBREW_TAP_TOKEN must fail before re-uploading release assets")
	assertReleaseWorkflowStepBefore(t, tapCheckoutIndex, formulaIndex, "the tap checkout must exist before writing Formula/packy.rb into it")
	assertReleaseWorkflowStepBefore(t, formulaIndex, prepareTapIndex, "the proved formula must be staged before preparing a local tap commit")
	assertReleaseWorkflowStepBefore(t, prepareTapIndex, tapPushAccessProofIndex, "the workflow must dry-run push the already-prepared local tap state, not the untouched checkout")
	assertReleaseWorkflowStepBefore(t, tapPushAccessProofIndex, createReleaseIndex, "token-backed tap push permission must be proven before creating a GitHub Release")
	assertReleaseWorkflowStepBefore(t, tapPushAccessProofIndex, reverifyTagIndex, "the candidate tag should be reverified only after every publication precondition passes")
	assertReleaseWorkflowStepBefore(t, reverifyTagIndex, createReleaseIndex, "the immutable tag binding must be checked immediately before publication")
	assertReleaseWorkflowStepBefore(t, tapPushAccessProofIndex, uploadIndex, "token-backed tap push permission must be proven before re-uploading release assets")
	assertReleaseWorkflowStepBefore(t, uploadIndex, pushTapIndex, "the tap update must not be published until release assets exist")
	assertReleaseWorkflowStepBefore(t, tapPushAccessProofIndex, pushTapIndex, "token-backed tap push permission must be proven before the mutating tap push")
	assertReleaseWorkflowStepBefore(t, prepareTapIndex, pushTapIndex, "the final tap push must publish the already-prepared commit instead of creating a new commit after assets upload")
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
	wantEntries := append(append([]string{}, wantAssets...), "checksums.txt")
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

	checksums := readChecksums(t, filepath.Join(outDir, "checksums.txt"))
	for _, asset := range wantAssets {
		gotChecksum, ok := checksums[asset]
		if !ok {
			t.Fatalf("checksums.txt missing checksum for %s", asset)
		}
		if gotChecksum != sha256File(t, filepath.Join(outDir, asset)) {
			t.Fatalf("checksum for %s does not match artifact bytes", asset)
		}
	}
	if len(checksums) != len(wantAssets) {
		t.Fatalf("checksums.txt should contain exactly release artifacts; got %d entries", len(checksums))
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
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go-build.log")
	goPath := filepath.Join(dir, "go")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> %q
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
`, logPath)
	if err := os.WriteFile(goPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, logPath
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
