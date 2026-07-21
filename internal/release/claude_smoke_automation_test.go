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
		"--claude-version 2.1.203",
		"--packy-ref \"$GITHUB_SHA\"",
		"actions/upload-artifact@v4",
		"if-no-files-found: error",
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
	for _, want := range []string{"2.1.203", `"stable"`, "--packy-binary", "internal/tools/claudesmoke", `--source-ref "$packy_ref"`, `--evidence "$evidence_dir/evidence.json"`} {
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
		"--claude-version stable",
		"actions/upload-artifact@v4",
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
		"needs: claude-smoke",
		"runner: macos-15-intel",
		"arch: amd64",
		"runner: macos-15",
		"arch: arm64",
		"claude: 2.1.203",
		"claude: stable",
		"scripts/build-release-artifacts.sh",
		"packy_${{ steps.release.outputs.tag }}_darwin_${{ matrix.arch }}",
		"--packy-ref \"${{ steps.release.outputs.tag }}\"",
		"actions/upload-artifact@v4",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow missing %q", want)
		}
	}
	if strings.Index(text, "claude-smoke:") > strings.Index(text, "release:") {
		t.Fatal("release smoke must be declared before publication")
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
