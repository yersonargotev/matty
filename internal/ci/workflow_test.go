package ci_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkflowRunsAllGoPackagesWithRaceDetector(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	contents, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	workflow := string(contents)
	raceJob := strings.SplitN(workflow, "  race:\n", 2)
	if len(raceJob) != 2 {
		t.Fatal("CI workflow missing race job")
	}
	for _, required := range []string{
		"name: Race detector (all Go packages)",
		"timeout-minutes: 15",
		"run: go test -race -timeout 10m ./...",
		"uses: actions/setup-go@v6",
		"go-version-file: go.mod",
		"cache: true",
	} {
		if !strings.Contains(raceJob[1], required) {
			t.Fatalf("CI workflow missing observable race behavior %q", required)
		}
	}
}
