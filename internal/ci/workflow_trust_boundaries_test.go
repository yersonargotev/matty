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

type errorReporter interface {
	Helper()
	Errorf(string, ...any)
}

type errorCollector struct {
	errors []string
}

func (collector *errorCollector) Helper() {}

func (collector *errorCollector) Errorf(format string, args ...any) {
	collector.errors = append(collector.errors, strings.TrimSpace(format))
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

func TestWorkflowTrustBoundaryMutationsFailClosed(t *testing.T) {
	root := repositoryRoot(t)
	tests := []struct {
		name     string
		workflow string
		mutate   func(string) string
		check    func(errorReporter, workflowDocument)
	}{
		{name: "mutable action", workflow: "ci.yml", mutate: func(text string) string {
			return strings.Replace(text, "actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5.1.0", "actions/checkout@v5", 1)
		}, check: assertImmutableExternalUses},
		{name: "missing version annotation", workflow: "ci.yml", mutate: func(text string) string { return strings.Replace(text, " # v5.1.0", "", 1) }, check: assertImmutableExternalUses},
		{name: "broadened default permission", workflow: "ci.yml", mutate: func(text string) string {
			return strings.Replace(text, "permissions: {}", "permissions:\n  contents: write", 1)
		}, check: assertMinimumPermissions},
		{name: "persisted pull request credential", workflow: "ci.yml", mutate: func(text string) string {
			return strings.Replace(text, "persist-credentials: false", "persist-credentials: true", 1)
		}, check: assertCheckoutCredentials},
		{name: "pull request head execution", workflow: "governance.yml", mutate: func(text string) string { return text + "\n# ${{ github.event.pull_request.head.sha }}\n" }, check: assertPullRequestTargetDoesNotExecutePRHead},
		{name: "manual branch privilege", workflow: "claude-canary.yml", mutate: func(text string) string {
			return strings.Replace(text, "github.ref == 'refs/heads/main'", "github.ref != ''", 1)
		}, check: assertTrustedPrivilegedExecution},
		{name: "pull request CodeQL upload", workflow: "security-pr.yml", mutate: func(text string) string { return strings.Replace(text, "upload: false", "upload: true", 1) }, check: assertTrustedPrivilegedExecution},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, ".github", "workflows", test.workflow)
			workflow := readWorkflowDocument(t, root, path)
			workflow.content = test.mutate(workflow.content)
			workflow.lines = strings.Split(workflow.content, "\n")
			for job, lines := range workflow.jobs {
				workflow.jobs[job] = strings.Split(test.mutate(strings.Join(lines, "\n")), "\n")
			}
			collector := &errorCollector{}
			test.check(collector, workflow)
			if len(collector.errors) == 0 {
				t.Fatal("unsafe workflow mutation was accepted")
			}
		})
	}
}

func TestWorkflowActorRefPermissionMatrix(t *testing.T) {
	root := repositoryRoot(t)
	workflows := make(map[string]workflowDocument)
	for _, name := range []string{"ci.yml", "claude-canary.yml", "governance.yml", "governance-drift.yml", "release.yml", "security.yml", "security-pr.yml", "sync-pack-source.yml"} {
		workflows[name] = readWorkflowDocument(t, root, filepath.Join(root, ".github", "workflows", name))
	}

	ci := workflows["ci.yml"]
	if !strings.Contains(ci.content, "pull_request:") {
		t.Fatal("fork and Dependabot contributions lack the read-only pull-request path")
	}
	for job, lines := range ci.jobs {
		permissions, _ := permissionBlock(lines, 4)
		if !reflect.DeepEqual(permissions, map[string]string{"contents": "read"}) || strings.Contains(strings.Join(lines, "\n"), "secrets:") || strings.Contains(strings.Join(lines, "\n"), "environment:") {
			t.Fatalf("CI job %q does not keep fork and Dependabot work read-only and secretless", job)
		}
	}

	for _, boundary := range []struct {
		workflow string
		job      string
	}{
		{workflow: "claude-canary.yml", job: "stable-smoke"},
		{workflow: "sync-pack-source.yml", job: "inspect"},
		{workflow: "sync-pack-source.yml", job: "publish"},
	} {
		block := strings.Join(workflows[boundary.workflow].jobs[boundary.job], "\n")
		for _, marker := range []string{"github.repository == 'yersonargotev/packy'", "github.ref == 'refs/heads/main'"} {
			if !strings.Contains(block, marker) {
				t.Errorf("%s job %q does not fail closed outside %q", boundary.workflow, boundary.job, marker)
			}
		}
		if strings.Contains(block, "github.actor ==") {
			t.Errorf("%s job %q replaces workflow_dispatch's Owner/delegated-write authorization with a hard-coded actor", boundary.workflow, boundary.job)
		}
	}

	for name, workflow := range workflows {
		for _, forbidden := range []string{"pages: write", "actions/deploy-pages"} {
			if strings.Contains(workflow.content, forbidden) {
				t.Errorf("%s grants unintegrated Pages authority %q", name, forbidden)
			}
		}
	}

	for _, fixture := range []string{"approved.json", "dependabot-exception.json", "automation-exception.json", "unapproved.json", "wrong-base.json", "cross-repository.json", "partial-exception.json"} {
		if _, err := os.Stat(filepath.Join(root, "internal", "governanceauth", "testdata", fixture)); err != nil {
			t.Errorf("actor/ref negative fixture %q is missing: %v", fixture, err)
		}
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

func assertImmutableExternalUses(t errorReporter, workflow workflowDocument) {
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

func assertMinimumPermissions(t errorReporter, workflow workflowDocument) {
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
		"addy-promotion-gate":        {"contents": "read"},
		"validate":                   {"contents": "read"},
		"claude-floor-smoke":         {"contents": "read"},
		"addy-promotion-main-replay": {"contents": "read"},
	},
	".github/workflows/claude-canary.yml": {
		"stable-smoke": {"contents": "read", "issues": "write"},
	},
	".github/workflows/governance.yml": {
		"targets":                {"actions": "read", "contents": "read", "issues": "read", "pull-requests": "read"},
		"validate-authorization": {"actions": "read", "contents": "read", "issues": "read", "pull-requests": "read", "statuses": "write"},
	},
	".github/workflows/governance-drift.yml": {
		"observe": {"actions": "read", "contents": "read", "deployments": "read"},
		"report":  {"actions": "read", "contents": "read", "issues": "write"},
	},
	".github/workflows/release.yml": {
		"governance-drift":          {"actions": "read", "contents": "read", "deployments": "read", "issues": "read"},
		"build":                     {"contents": "read"},
		"claude-smoke":              {"contents": "read"},
		"validate-release-evidence": {"contents": "read"},
		"dry-run":                   {"contents": "read"},
		"inspect-release":           {"contents": "read"},
		"attest":                    {"attestations": "write", "contents": "read", "id-token": "write"},
		"publish-github":            {"contents": "write"},
		"homebrew":                  {"contents": "read"},
	},
	".github/workflows/security.yml": {
		"codeql": {"contents": "read", "packages": "read", "security-events": "write"},
	},
	".github/workflows/security-pr.yml": {
		"codeql":            {"contents": "read", "packages": "read"},
		"dependency-review": {"contents": "read"},
	},
	".github/workflows/sync-pack-source.yml": {
		"governance-drift": {"actions": "read", "contents": "read", "deployments": "read", "issues": "read"},
		"inspect":          {"contents": "read"},
		"classify":         {"contents": "read", "models": "read"},
		"validate":         {"contents": "read"},
		"publish":          {"contents": "write", "pull-requests": "write"},
	},
}

func assertCheckoutCredentials(t errorReporter, workflow workflowDocument) {
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
	".github/workflows/sync-pack-source.yml|publish|":                   "publish only the canonical automation-owned proposal branch and matching pull request",
	".github/workflows/release.yml|homebrew|yersonargotev/homebrew-tap": "publish only the proved formula to the dedicated Homebrew tap",
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

func assertPullRequestTargetDoesNotExecutePRHead(t errorReporter, workflow workflowDocument) {
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

func assertTrustedPrivilegedExecution(t errorReporter, workflow workflowDocument) {
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
		for _, marker := range forbiddenTrustedExecutionMarkers[key] {
			if strings.Contains(block, marker) {
				t.Errorf("%s privileged job %q contains forbidden execution boundary %q", workflow.path, job, marker)
			}
		}
	}
}

// Write-capable or secret-bearing jobs must name the trusted repository and a
// protected ref boundary in their job-level gate. Governance is event-driven
// from the protected base/default branch and additionally checks out github.sha,
// never the proposed head. Release publication admits protected main only.
var trustedExecutionMarkers = map[string][]string{
	".github/workflows/claude-canary.yml|stable-smoke": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
	},
	".github/workflows/governance.yml|validate-authorization": {
		"github.repository == 'yersonargotev/packy'",
		"ref: ${{ github.sha }}",
	},
	".github/workflows/governance-drift.yml|report": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
	},
	".github/workflows/release.yml|attest": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
		"inputs.dry_run == false",
		"environment: release",
	},
	".github/workflows/release.yml|publish-github": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
		"inputs.dry_run == false",
		"environment: release",
	},
	".github/workflows/release.yml|homebrew": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
		"inputs.dry_run == false",
		"environment: homebrew",
	},
	".github/workflows/security.yml|codeql": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
		"build-mode: autobuild",
		"Autobuild trusted main",
		"persist-credentials: false",
	},
	".github/workflows/security-pr.yml|codeql": {
		"upload: false",
		"persist-credentials: false",
	},
	".github/workflows/sync-pack-source.yml|publish": {
		"github.repository == 'yersonargotev/packy'",
		"refs/heads/main",
	},
}

var forbiddenTrustedExecutionMarkers = map[string][]string{
	".github/workflows/security-pr.yml|codeql": {"upload: true", "security-events: write", "secrets:", "environment:"},
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
