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

func TestReleaseWorkflowPublishesMattyArtifactsAndTapFormula(t *testing.T) {
	root := repoRoot(t)
	text := readReleaseWorkflow(t, root)

	for _, want := range []string{
		"workflow_dispatch:",
		"tag:",
		"push:",
		"- 'v0.*.*'",
		"required: true",
		"actions/checkout@v5",
		"fetch-depth: 0",
		"actions/setup-go@v6",
		"go-version-file: go.mod",
		"git checkout --detach \"$tag\"",
		"scripts/build-release-artifacts.sh",
		"--out-dir dist",
		"HOMEBREW_TAP_TOKEN",
		"yersonargotev/homebrew-tap",
		"scripts/generate-homebrew-formula.sh",
		"--checksums dist/checksums.txt",
		"--out homebrew-tap/Formula/matty.rb",
		"--repo yersonargotev/matty",
		"gh release upload",
		"dist/* --clobber",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow should contain %q so GitHub Releases and the Homebrew tap stay in sync", want)
		}
	}
}

func TestReleaseWorkflowCreatesReleaseWithGeneratedNotes(t *testing.T) {
	root := repoRoot(t)
	step := releaseWorkflowStep(t, parseReleaseWorkflow(t, readReleaseWorkflow(t, root)), "Create GitHub Release if needed")

	if !strings.Contains(step.Text, "gh release view") {
		t.Fatalf("release creation step should be idempotent by checking whether the release exists; step:\n%s", step.Text)
	}
	if !strings.Contains(step.Text, "gh release create") {
		t.Fatalf("release creation step should create the GitHub Release; step:\n%s", step.Text)
	}
	if !strings.Contains(step.Text, "--generate-notes") {
		t.Fatalf("release creation should ask GitHub to generate per-tag notes; step:\n%s", step.Text)
	}
	if strings.Contains(step.Text, "--notes") {
		t.Fatalf("release creation should not pass static release notes; step:\n%s", step.Text)
	}
}

func TestReleaseWorkflowProvesTapAccessBeforePublishingReleaseAssets(t *testing.T) {
	root := repoRoot(t)
	workflow := parseReleaseWorkflow(t, readReleaseWorkflow(t, root))

	resolveTagIndex := releaseWorkflowStepIndex(t, workflow, "Resolve release tag", []string{
		"git checkout --detach \"$tag\"",
	})
	setupGoIndex := releaseWorkflowStepIndex(t, workflow, "Set up Go", []string{
		"uses: actions/setup-go@v6",
		"go-version-file: go.mod",
	})
	buildIndex := releaseWorkflowStepIndex(t, workflow, "Build release artifacts and checksums.txt", []string{
		"scripts/build-release-artifacts.sh", "--out-dir dist",
	})
	requireTapTokenIndex := releaseWorkflowStepIndex(t, workflow, "Require Homebrew tap token", []string{
		"HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}",
		"HOMEBREW_TAP_TOKEN is required",
		"yersonargotev/homebrew-tap",
	})
	tapCheckoutIndex := releaseWorkflowStepIndex(t, workflow, "Check out Homebrew tap", []string{
		"uses: actions/checkout@v5",
		"repository: yersonargotev/homebrew-tap",
		"path: homebrew-tap",
		"token: ${{ secrets.HOMEBREW_TAP_TOKEN }}",
	})
	formulaIndex := releaseWorkflowStepIndex(t, workflow, "Generate Homebrew formula from release checksums", []string{
		"scripts/generate-homebrew-formula.sh",
		"--checksums dist/checksums.txt",
		"--out homebrew-tap/Formula/matty.rb",
	})
	prepareTapIndex := releaseWorkflowStepIndex(t, workflow, "Prepare Homebrew tap formula update", []string{
		"id: prepare_tap",
		"working-directory: homebrew-tap",
		`git config user.name "github-actions[bot]"`,
		`git config user.email "github-actions[bot]@users.noreply.github.com"`,
		"git add Formula/matty.rb",
		"git diff --cached --quiet",
		`echo "changed=false" >> "$GITHUB_OUTPUT"`,
		`echo "changed=true" >> "$GITHUB_OUTPUT"`,
		`git commit -m "feat: update matty formula to ${RELEASE_TAG}"`,
	})
	tapPushAccessProofIndex := releaseWorkflowStepIndex(t, workflow, "Prove Homebrew tap push permission", []string{
		"working-directory: homebrew-tap",
		"git push --dry-run origin HEAD:main",
	})
	createReleaseIndex := releaseWorkflowStepIndex(t, workflow, "Create GitHub Release if needed", []string{
		"GH_TOKEN: ${{ github.token }}",
		"gh release create",
		"--generate-notes",
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

	if strings.Contains(releaseWorkflowStep(t, workflow, "Prove Homebrew tap push permission").Text, "git commit") {
		t.Fatalf("tap push proof must dry-run the prepared local commit without creating another commit")
	}
	if strings.Contains(releaseWorkflowStep(t, workflow, "Push prepared Homebrew tap formula update").Text, "git push --dry-run") {
		t.Fatalf("final tap push must be mutating, not another dry run")
	}

	assertReleaseWorkflowStepBefore(t, resolveTagIndex, setupGoIndex, "Go must be set up from the checked-out release tag, not the workflow dispatch ref")
	assertReleaseWorkflowStepBefore(t, setupGoIndex, buildIndex, "release artifacts should build after the release tag checkout and Go setup")
	assertReleaseWorkflowStepBefore(t, buildIndex, formulaIndex, "formula generation must consume freshly built artifacts and dist/checksums.txt")
	assertReleaseWorkflowStepBefore(t, requireTapTokenIndex, tapCheckoutIndex, "the workflow must reject a missing HOMEBREW_TAP_TOKEN before falling back to anonymous tap checkout")
	assertReleaseWorkflowStepBefore(t, requireTapTokenIndex, createReleaseIndex, "a missing HOMEBREW_TAP_TOKEN must fail before creating a GitHub Release")
	assertReleaseWorkflowStepBefore(t, requireTapTokenIndex, uploadIndex, "a missing HOMEBREW_TAP_TOKEN must fail before re-uploading release assets")
	assertReleaseWorkflowStepBefore(t, tapCheckoutIndex, formulaIndex, "the tap checkout must exist before writing Formula/matty.rb into it")
	assertReleaseWorkflowStepBefore(t, formulaIndex, prepareTapIndex, "the generated formula must be staged before preparing a local tap commit")
	assertReleaseWorkflowStepBefore(t, prepareTapIndex, tapPushAccessProofIndex, "the workflow must dry-run push the already-prepared local tap state, not the untouched checkout")
	assertReleaseWorkflowStepBefore(t, tapPushAccessProofIndex, createReleaseIndex, "token-backed tap push permission must be proven before creating a GitHub Release")
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
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "build-release-artifacts.sh"), "--version", "v0.99.0", "--out-dir", outDir)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "GOCACHE="+t.TempDir(), "GOMODCACHE="+goEnv(t, "GOMODCACHE"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build release artifacts: %v\n%s", err, output)
	}

	wantAssets := releaseAssets("v0.99.0")

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	var gotAssets []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "checksums.txt" {
			continue
		}
		gotAssets = append(gotAssets, entry.Name())
	}
	sort.Strings(gotAssets)
	if strings.Join(gotAssets, "\n") != strings.Join(wantAssets, "\n") {
		t.Fatalf("release assets mismatch\nwant:\n%s\ngot:\n%s", strings.Join(wantAssets, "\n"), strings.Join(gotAssets, "\n"))
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
				wantLdflag := "-X github.com/yersonargotev/matty/internal/version.Value=" + version
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
	outputPath := filepath.Join(t.TempDir(), "Formula", "matty.rb")

	cmd := exec.Command(
		"bash",
		filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
		"--version", "v0.99.0",
		"--checksums", checksumsPath,
		"--out", outputPath,
		"--repo", "yersonargotev/matty",
		"--homepage", "https://github.com/yersonargotev/matty",
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
		"class Matty < Formula",
		`desc "AI coding workflow installer"`,
		`homepage "https://github.com/yersonargotev/matty"`,
		`version "0.99.0"`,
		`url "https://github.com/yersonargotev/matty/releases/download/v0.99.0/matty_v0.99.0_darwin_amd64", using: :nounzip`,
		`sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`url "https://github.com/yersonargotev/matty/releases/download/v0.99.0/matty_v0.99.0_darwin_arm64", using: :nounzip`,
		`sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`,
		`url "https://github.com/yersonargotev/matty/releases/download/v0.99.0/matty_v0.99.0_linux_amd64", using: :nounzip`,
		`sha256 "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`,
		`url "https://github.com/yersonargotev/matty/releases/download/v0.99.0/matty_v0.99.0_linux_arm64", using: :nounzip`,
		`sha256 "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"`,
		`downloaded_binary = Dir["matty_*"].first`,
		`odie "downloaded matty binary not found" if downloaded_binary.nil?`,
		`bin.install downloaded_binary => "matty"`,
		`system "#{bin}/matty", "--version"`,
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
	outputPath := filepath.Join(t.TempDir(), "Formula", "matty.rb")

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
	if !strings.Contains(string(output), "missing checksum entry for matty_v0.99.0_linux_arm64") {
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
			extraLine: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee  matty_v0.99.0_linux_386",
			wantError: "unexpected checksum entry for matty_v0.99.0_linux_386",
		},
		{
			name:      "rejects duplicate expected artifact",
			extraLine: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff  matty_v0.99.0_darwin_amd64",
			wantError: "duplicate checksum entry for matty_v0.99.0_darwin_amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksums := append(append([]string{}, baseChecksums...), tt.extraLine)
			checksumsPath := writeChecksumManifest(t, checksums)
			outputPath := filepath.Join(t.TempDir(), "Formula", "matty.rb")

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
		fmt.Sprintf("%s  matty_%s_darwin_amd64", strings.Repeat("a", sha256.Size*2), version),
		fmt.Sprintf("%s  matty_%s_darwin_arm64", strings.Repeat("b", sha256.Size*2), version),
		fmt.Sprintf("%s  matty_%s_linux_amd64", strings.Repeat("c", sha256.Size*2), version),
		fmt.Sprintf("%s  matty_%s_linux_arm64", strings.Repeat("d", sha256.Size*2), version),
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
		assets = append(assets, fmt.Sprintf("matty_%s_%s_%s", version, parts[0], parts[1]))
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
