package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullRequestsBlockOnExactClaudeFloorAndRetainEvidence(t *testing.T) {
	text := readWorkflowFile(t, "ci.yml")
	for _, want := range []string{
		"claude-floor-smoke:",
		"if: github.event_name == 'pull_request'",
		"runs-on: macos-15",
		"--claude-version 2.1.203",
		"--packy-ref \"$GITHUB_SHA\"",
		"actions/upload-artifact@",
		"if-no-files-found: error",
		"retention-days: 90",
		"--addy-qualification synthetic",
		"--addy-workflow .github/workflows/ci.yml",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pull-request workflow missing %q", want)
		}
	}
}

func TestClaudeSmokeWrapperIsSyntacticallyValidAndPinsSafeSelectors(t *testing.T) {
	path := filepath.Join(repoRoot(t), "scripts", "run-claude-smoke.sh")
	if output, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"2.1.203", `"stable"`, "--packy-binary", `go build -trimpath -o "$build_root/claudesmoke" ./internal/tools/claudesmoke`, `"$build_root/claudesmoke"`, `--source-ref "$packy_ref"`, `--evidence "$evidence_dir/evidence.json"`, "qualify-addy", "--addy-qualification", "--addy-workflow"} {
		if !strings.Contains(text, want) {
			t.Fatalf("smoke wrapper missing %q", want)
		}
	}
	for _, forbidden := range []string{"claude --print", "claude -p", "claude login", "ANTHROPIC_API_KEY"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("smoke wrapper contains forbidden operation %q", forbidden)
		}
	}
}

func TestStableCanaryIsIndependentFromPullRequestsAndOpensCompatibilityWork(t *testing.T) {
	text := readWorkflowFile(t, "claude-canary.yml")
	for _, want := range []string{
		"schedule:",
		"workflow_dispatch:",
		"runs-on: macos-15",
		"--claude-version stable",
		"actions/upload-artifact@",
		"issues: write",
		"gh issue create",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("canary workflow missing %q", want)
		}
	}
	if strings.Contains(text, "pull_request:") {
		t.Fatal("moving-stable canary must not attach to unrelated pull requests")
	}
}

func TestReleaseBlocksPublicationOnBothClaudeVariantsAndDarwinArchitectures(t *testing.T) {
	text := readWorkflowFile(t, "release.yml")
	for _, want := range []string{
		"Validate exact tag commit",
		"./scripts/validate-packy.sh",
		"name: packy-release-${{ steps.release.outputs.tag }}",
		"commit: ${{ steps.release.outputs.commit }}",
		"needs: build",
		"needs: [build, claude-smoke]",
		"runner: macos-15-intel",
		"arch: amd64",
		"runner: macos-15",
		"arch: arm64",
		"claude: 2.1.203",
		"claude: stable",
		"scripts/build-release-artifacts.sh",
		"actions/download-artifact@",
		"packy_${{ needs.build.outputs.tag }}_darwin_${{ matrix.arch }}",
		"ref: ${{ needs.build.outputs.commit }}",
		"--packy-ref \"${{ needs.build.outputs.commit }}\"",
		`tag_commit="$(git rev-parse --verify "${RELEASE_TAG}^{commit}")"`,
		`[[ "$head" == "$main" && "$head" == "$tag_commit" ]]`,
		"Create or resume exact draft and publish once",
		"actions/upload-artifact@",
		"Gate exact-tag Addy promotion evidence",
		"./scripts/gate-addy-release.sh",
		"retention-days: 90",
		"--addy-qualification synthetic",
		"--addy-workflow .github/workflows/release.yml",
		"--addy-tag \"${{ needs.build.outputs.tag }}\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow missing %q", want)
		}
	}
	if strings.Index(text, "claude-smoke:") > strings.Index(text, "publish-github:") {
		t.Fatal("release smoke must be declared before publication")
	}
	publication := text[strings.LastIndex(text, "  publish-github:"):]
	if strings.Contains(publication, "scripts/build-release-artifacts.sh") {
		t.Fatal("publication must consume the proved candidate instead of rebuilding artifacts")
	}
}

func TestExactTagAddyGateRejectsMissingOrForeignEvidence(t *testing.T) {
	path := filepath.Join(repoRoot(t), "scripts", "gate-addy-release.sh")
	if output, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"exact release tag and candidate commit diverge",
		"exact-tag Addy promotion requires same-run evidence",
		"--tag=\"$tag\"",
		"--run-id=\"${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}\"",
		"--workflow-digest=\"$workflow_digest\"",
		"promotion evidence must be a regular same-run artifact",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("exact-tag Addy gate missing %q", want)
		}
	}
	for _, forbidden := range []string{"gh release", "git push", "npm publish"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("exact-tag Addy gate contains publishing effect %q", forbidden)
		}
	}
}

func readWorkflowFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
