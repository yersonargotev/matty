package ci_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	jobLinePattern        = regexp.MustCompile(`^  ([a-zA-Z0-9_-]+):\s*(?:#.*)?$`)
	permissionLinePattern = regexp.MustCompile(`^([a-zA-Z0-9_-]+):\s*(read|write)$`)
	usesLinePattern       = regexp.MustCompile(`^(?:-\s+)?uses:\s*([^\s#]+)(?:\s+#\s*(\S.*))?$`)
	externalUsesPattern   = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_./-]+)?@([0-9a-f]{40})$`)
	versionCommentPattern = regexp.MustCompile(`^v[0-9]+(?:\.[0-9]+){1,2}(?:[-+][0-9A-Za-z.-]+)?$`)
)

type workflowDocument struct {
	path    string
	content string
	lines   []string
	jobs    map[string][]string
}

type checkoutBoundary struct {
	job                string
	repository         string
	persistCredentials string
}

func TestWorkflowTrustBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatalf("find workflows: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no GitHub Actions workflows found")
	}
	sort.Strings(paths)

	for _, path := range paths {
		workflow := readWorkflowDocument(t, root, path)
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			assertImmutableExternalUses(t, workflow)
			assertMinimumPermissions(t, workflow)
			assertCheckoutCredentials(t, workflow)
			assertPullRequestTargetDoesNotExecutePRHead(t, workflow)
			assertTrustedPrivilegedExecution(t, workflow)
		})
	}
}

func readWorkflowDocument(t *testing.T, root, path string) workflowDocument {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	jobs := make(map[string][]string)
	inJobs := false
	currentJob := ""
	for _, line := range lines {
		if line == "jobs:" {
			inJobs = true
			continue
		}
		if !inJobs {
			continue
		}
		if matches := jobLinePattern.FindStringSubmatch(line); matches != nil {
			currentJob = matches[1]
			jobs[currentJob] = []string{line}
			continue
		}
		if currentJob != "" {
			jobs[currentJob] = append(jobs[currentJob], line)
		}
	}
	if len(jobs) == 0 {
		t.Fatalf("%s has no jobs", path)
	}
	return workflowDocument{
		path:    filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))),
		content: content,
		lines:   lines,
		jobs:    jobs,
	}
}

func assertImmutableExternalUses(t *testing.T, workflow workflowDocument) {
	t.Helper()
	for index, line := range workflow.lines {
		matches := usesLinePattern.FindStringSubmatch(strings.TrimSpace(line))
		if matches == nil {
			continue
		}
		reference := strings.Trim(matches[1], `"'`)
		if strings.HasPrefix(reference, "./") {
			continue
		}
		pin := externalUsesPattern.FindStringSubmatch(reference)
		if pin == nil {
			t.Errorf("%s:%d external uses reference %q is not pinned to a full lowercase 40-character SHA", workflow.path, index+1, reference)
			continue
		}
		if !versionCommentPattern.MatchString(strings.TrimSpace(matches[2])) {
			t.Errorf("%s:%d immutable reference %q needs a readable version comment such as # v5.0.0", workflow.path, index+1, reference)
		}
	}
}

func assertMinimumPermissions(t *testing.T, workflow workflowDocument) {
	t.Helper()
	permissions, declaration := permissionBlock(workflow.lines, 0)
	wantReadOnly := map[string]string{"contents": "read"}
	if declaration != "permissions: {}" && !reflect.DeepEqual(permissions, wantReadOnly) {
		t.Errorf("%s workflow permissions = %v; want permissions: {} or only contents: read", workflow.path, permissions)
	}

	wantJobs, registered := minimumJobPermissions[workflow.path]
	if !registered {
		t.Errorf("%s has no reviewed minimum job-permission contract", workflow.path)
		return
	}
	for job, lines := range workflow.jobs {
		want, registered := wantJobs[job]
		if !registered {
			t.Errorf("%s job %q has no reviewed minimum permission contract", workflow.path, job)
			continue
		}
		got, declaration := permissionBlock(lines, 4)
		if declaration == "" {
			t.Errorf("%s job %q does not explicitly declare permissions", workflow.path, job)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s job %q permissions = %v; want reviewed minimum %v", workflow.path, job, got, want)
		}
	}
	for job := range wantJobs {
		if _, found := workflow.jobs[job]; !found {
			t.Errorf("%s reviewed job %q is missing", workflow.path, job)
		}
	}
}

func permissionBlock(lines []string, indent int) (map[string]string, string) {
	prefix := strings.Repeat(" ", indent)
	for index, line := range lines {
		if !strings.HasPrefix(line, prefix+"permissions:") || leadingSpaces(line) != indent {
			continue
		}
		declaration := strings.TrimSpace(line)
		permissions := make(map[string]string)
		if declaration == "permissions: {}" {
			return permissions, declaration
		}
		for _, entry := range lines[index+1:] {
			if strings.TrimSpace(entry) == "" || strings.HasPrefix(strings.TrimSpace(entry), "#") {
				continue
			}
			if leadingSpaces(entry) <= indent {
				break
			}
			if leadingSpaces(entry) != indent+2 {
				continue
			}
			matches := permissionLinePattern.FindStringSubmatch(strings.TrimSpace(entry))
			if matches != nil {
				permissions[matches[1]] = matches[2]
			}
		}
		return permissions, declaration
	}
	return nil, ""
}

var minimumJobPermissions = map[string]map[string]map[string]string{
	".github/workflows/ci.yml": {
		"validate":           {"contents": "read"},
		"claude-floor-smoke": {"contents": "read"},
	},
	".github/workflows/claude-canary.yml": {
		"stable-smoke": {"contents": "read", "issues": "write"},
	},
	".github/workflows/governance.yml": {
		"targets":                {"actions": "read", "contents": "read", "issues": "read", "pull-requests": "read"},
		"validate-authorization": {"actions": "read", "contents": "read", "issues": "read", "pull-requests": "read", "statuses": "write"},
	},
	".github/workflows/release.yml": {
		"build":                     {"contents": "read"},
		"claude-smoke":              {"contents": "read"},
		"validate-release-evidence": {"contents": "read"},
		"release":                   {"contents": "write"},
	},
	".github/workflows/security.yml": {
		"codeql":            {"contents": "read", "packages": "read", "security-events": "write"},
		"dependency-review": {"contents": "read"},
	},
	".github/workflows/sync-pack-source.yml": {
		"inspect":  {"contents": "read"},
		"classify": {"contents": "read", "models": "read"},
		"validate": {"contents": "read"},
		"publish":  {"contents": "write", "pull-requests": "write"},
	},
}

func assertCheckoutCredentials(t *testing.T, workflow workflowDocument) {
	t.Helper()
	for job, lines := range workflow.jobs {
		for _, checkout := range checkoutBoundaries(job, lines) {
			key := strings.Join([]string{workflow.path, checkout.job, checkout.repository}, "|")
			reason, admittedWrite := admittedWriteCheckouts[key]
			want := "false"
			if admittedWrite {
				want = "true"
			}
			if checkout.persistCredentials != want {
				if admittedWrite {
					t.Errorf("%s job %q checkout of %q must explicitly set persist-credentials: true for its admitted write boundary (%s)", workflow.path, job, checkoutRepository(checkout.repository), reason)
				} else {
					t.Errorf("%s job %q checkout of %q must explicitly set persist-credentials: false", workflow.path, job, checkoutRepository(checkout.repository))
				}
			}
		}
	}
}

// These are the only checkouts admitted to retain a credential: synchronization
// publishes its owned proposal branch, and release publishes the proved formula
// to the dedicated Homebrew tap. Every new exception requires review here.
var admittedWriteCheckouts = map[string]string{
	".github/workflows/sync-pack-source.yml|publish|":                  "publish only the canonical automation-owned proposal branch and matching pull request",
	".github/workflows/release.yml|release|yersonargotev/homebrew-tap": "publish only the proved formula to the dedicated Homebrew tap",
}

func checkoutBoundaries(job string, lines []string) []checkoutBoundary {
	var boundaries []checkoutBoundary
	for index, line := range lines {
		matches := usesLinePattern.FindStringSubmatch(strings.TrimSpace(line))
		if matches == nil || !strings.HasPrefix(strings.Trim(matches[1], `"'`), "actions/checkout@") {
			continue
		}
		boundary := checkoutBoundary{job: job}
		stepIndent := leadingSpaces(line)
		for _, candidate := range lines[index+1:] {
			trimmed := strings.TrimSpace(candidate)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if leadingSpaces(candidate) <= stepIndent && strings.HasPrefix(trimmed, "-") {
				break
			}
			if strings.HasPrefix(trimmed, "repository:") {
				boundary.repository = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "repository:")), `"'`)
			}
			if strings.HasPrefix(trimmed, "persist-credentials:") {
				boundary.persistCredentials = strings.TrimSpace(strings.TrimPrefix(trimmed, "persist-credentials:"))
			}
		}
		boundaries = append(boundaries, boundary)
	}
	return boundaries
}

func checkoutRepository(repository string) string {
	if repository == "" {
		return "the current repository"
	}
	return repository
}

func assertPullRequestTargetDoesNotExecutePRHead(t *testing.T, workflow workflowDocument) {
	t.Helper()
	if !regexp.MustCompile(`(?m)^\s{2}pull_request_target:\s*$`).MatchString(workflow.content) {
		return
	}
	for _, forbidden := range []string{
		"github.head_ref",
		"github.event.pull_request.head.sha",
		"github.event.pull_request.head.ref",
		"github.event.pull_request.head.repo",
		"refs/pull/",
	} {
		if strings.Contains(workflow.content, forbidden) {
			t.Errorf("%s pull_request_target workflow references untrusted PR-head execution input %q", workflow.path, forbidden)
		}
	}
}

func assertTrustedPrivilegedExecution(t *testing.T, workflow workflowDocument) {
	t.Helper()
	for key, markers := range trustedExecutionMarkers {
		parts := strings.SplitN(key, "|", 2)
		if parts[0] != workflow.path {
			continue
		}
		job := parts[1]
		lines, found := workflow.jobs[job]
		if !found {
			continue
		}
		block := strings.Join(lines, "\n")
		for _, marker := range markers {
			if !strings.Contains(block, marker) {
				t.Errorf("%s privileged job %q lacks trusted execution gate %q", workflow.path, job, marker)
			}
		}
	}
}

// Write-capable or secret-bearing jobs must name the trusted repository and a
// protected ref boundary in their job-level gate. Governance is event-driven
// from the protected base/default branch and additionally checks out github.sha,
// never the proposed head. Release admits only protected main or its v0 tags.
var trustedExecutionMarkers = map[string][]string{
	".github/workflows/claude-canary.yml|stable-smoke": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
	},
	".github/workflows/governance.yml|validate-authorization": {
		"github.repository == 'yersonargotev/packy'",
		"ref: ${{ github.sha }}",
	},
	".github/workflows/release.yml|release": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
		"refs/tags/v0.",
	},
	".github/workflows/sync-pack-source.yml|publish": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
	},
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
