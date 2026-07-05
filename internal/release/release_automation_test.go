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
