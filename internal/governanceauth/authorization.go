// Package governanceauth validates the trusted metadata that authorizes a
// Packy pull request. It deliberately has no GitHub client or mutation surface.
package governanceauth

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const ApprovedLabel = "status:approved"

var classificationLabels = map[string]struct{}{
	"status:needs-review": {},
	ApprovedLabel:         {},
	"needs-info":          {},
	"ready-for-human":     {},
	"wontfix":             {},
}

// Event is the pull_request_target metadata used for authorization.
type Event struct {
	Action     string `json:"action"`
	Repository struct {
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
	} `json:"repository"`
	PullRequest struct {
		Number    int    `json:"number"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		Author    string `json:"author"`
		HeadRef   string `json:"head_ref"`
		Base      struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"base"`
	} `json:"pull_request"`
}

// Metadata contains only GitHub issue metadata collected with a read-only
// token by the trusted base workflow.
type Metadata struct {
	ClosingIssuesReferences []IssueReference `json:"closingIssuesReferences"`
	Issues                  []Issue          `json:"issues"`
	Exception               *ExceptionRecord `json:"exception"`
}

type ExceptionRecord struct {
	Type       string `json:"type"`
	URL        string `json:"url"`
	Kind       string `json:"kind"`
	Repository string `json:"repository"`
	State      string `json:"state"`
	Conclusion string `json:"conclusion,omitempty"`
	Accessible bool   `json:"accessible"`
	CreatedAt  string `json:"created_at,omitempty"`
	PRNumber   int    `json:"pull_request_number,omitempty"`
	Workflow   string `json:"workflow,omitempty"`
	Path       string `json:"path,omitempty"`
	Event      string `json:"event,omitempty"`
	HeadBranch string `json:"head_branch,omitempty"`
	HeadSHA    string `json:"head_sha,omitempty"`
	Actor      string `json:"actor,omitempty"`
}

type ExceptionDeclaration struct {
	Type string
	URL  string
}

type IssueReference struct {
	Number     int `json:"number"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

type Issue struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// Validate fails closed unless every issue that closes the pull request is open,
// belongs to this repository, and has exactly the approved delivery status.
func Validate(event Event, metadata Metadata) error {
	if event.Repository.FullName == "" || event.Repository.DefaultBranch == "" {
		return errors.New("repository identity is incomplete")
	}
	if event.PullRequest.Number <= 0 || event.PullRequest.Base.SHA == "" {
		return errors.New("pull request identity is incomplete")
	}
	if event.PullRequest.Base.Ref != event.Repository.DefaultBranch {
		return fmt.Errorf("pull request base %q is not default branch %q", event.PullRequest.Base.Ref, event.Repository.DefaultBranch)
	}

	declaration, declared, err := ParseExceptionDeclaration(event.PullRequest.Body)
	if err != nil {
		return err
	}
	if declared {
		if len(metadata.ClosingIssuesReferences) != 0 || len(metadata.Issues) != 0 {
			return errors.New("approved-issue and exception authorization cannot be mixed")
		}
		return validateException(event, declaration, metadata.Exception)
	}
	if metadata.Exception != nil {
		return errors.New("exception metadata exists without a declaration")
	}
	if len(metadata.ClosingIssuesReferences) == 0 {
		return errors.New("no closing issue reference found")
	}
	if len(metadata.Issues) != len(metadata.ClosingIssuesReferences) {
		return errors.New("trusted issue snapshots do not match closing issue references")
	}

	issues := make(map[int]Issue, len(metadata.Issues))
	for _, issue := range metadata.Issues {
		if issue.Number <= 0 {
			return errors.New("trusted issue snapshot has no number")
		}
		if _, duplicate := issues[issue.Number]; duplicate {
			return fmt.Errorf("duplicate trusted snapshot for issue #%d", issue.Number)
		}
		issues[issue.Number] = issue
	}

	seen := make(map[int]struct{}, len(metadata.ClosingIssuesReferences))
	for _, reference := range metadata.ClosingIssuesReferences {
		referenceRepository := reference.Repository.Owner.Login + "/" + reference.Repository.Name
		if referenceRepository != event.Repository.FullName {
			return fmt.Errorf("closing issue repository %q does not match %q", referenceRepository, event.Repository.FullName)
		}
		if reference.Number <= 0 {
			return errors.New("closing issue reference has no number")
		}
		if _, duplicate := seen[reference.Number]; duplicate {
			return fmt.Errorf("duplicate closing reference for issue #%d", reference.Number)
		}
		seen[reference.Number] = struct{}{}

		issue, ok := issues[reference.Number]
		if !ok {
			return fmt.Errorf("trusted snapshot for closing issue #%d is absent", reference.Number)
		}
		if issue.State != "OPEN" {
			return fmt.Errorf("closing issue #%d is not open", issue.Number)
		}

		approved := false
		classifications := 0
		for _, label := range issue.Labels {
			if _, classified := classificationLabels[label.Name]; classified {
				classifications++
			}
			if label.Name == ApprovedLabel {
				approved = true
			}
		}
		if !approved || classifications != 1 {
			return fmt.Errorf("closing issue #%d does not have exactly one approved delivery status", issue.Number)
		}
	}

	return nil
}

// ParseExceptionDeclaration recognizes the closed exception grammar from the
// protected-change policy. It rejects partial, duplicate, and unknown headers.
func ParseExceptionDeclaration(body string) (ExceptionDeclaration, bool, error) {
	var types []string
	var records []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Authorization-Exception:"):
			types = append(types, strings.TrimSpace(strings.TrimPrefix(line, "Authorization-Exception:")))
		case strings.HasPrefix(line, "Authorization-Record:"):
			records = append(records, strings.TrimSpace(strings.TrimPrefix(line, "Authorization-Record:")))
		}
	}
	if len(types) == 0 && len(records) == 0 {
		return ExceptionDeclaration{}, false, nil
	}
	if len(types) != 1 || len(records) != 1 || types[0] == "" || records[0] == "" {
		return ExceptionDeclaration{}, false, errors.New("exception declaration requires exactly one type and one record")
	}
	switch types[0] {
	case "private-security", "urgent-revert", "automation":
	default:
		return ExceptionDeclaration{}, false, fmt.Errorf("unknown authorization exception %q", types[0])
	}
	return ExceptionDeclaration{Type: types[0], URL: records[0]}, true, nil
}

func validateException(event Event, declaration ExceptionDeclaration, record *ExceptionRecord) error {
	if declaration.Type == "private-security" {
		return errors.New("private-security exception is unavailable without immediate advisory-change recomputation")
	}
	if record == nil || !record.Accessible {
		return errors.New("declared exception record is inaccessible")
	}
	if record.Type != declaration.Type || record.URL != declaration.URL {
		return errors.New("trusted exception record does not match declaration")
	}
	if record.Repository != event.Repository.FullName {
		return fmt.Errorf("exception repository %q does not match %q", record.Repository, event.Repository.FullName)
	}
	if err := validateCanonicalRecordURL(event.Repository.FullName, declaration); err != nil {
		return err
	}

	switch declaration.Type {
	case "urgent-revert":
		if record.Kind != "issue" || record.State != "OPEN" {
			return errors.New("urgent-revert record is not an open retrospective issue")
		}
		if record.PRNumber != event.PullRequest.Number {
			return errors.New("urgent-revert retrospective is not bound to this pull request")
		}
		created, createdErr := time.Parse(time.RFC3339, record.CreatedAt)
		prCreated, prCreatedErr := time.Parse(time.RFC3339, event.PullRequest.CreatedAt)
		if createdErr != nil || prCreatedErr != nil || created.Before(prCreated) || created.After(prCreated.Add(24*time.Hour)) {
			return errors.New("urgent-revert retrospective was not opened within 24 hours of the pull request")
		}
	case "automation":
		switch record.Kind {
		case "dependabot-pr":
			if record.State != "OPEN" || record.PRNumber != event.PullRequest.Number || event.PullRequest.Author != "app/dependabot" || !strings.HasPrefix(event.PullRequest.HeadRef, "dependabot/") {
				return errors.New("automation record is not this Dependabot pull request")
			}
		case "actions-run":
			if record.State != "completed" || record.Conclusion != "success" {
				return errors.New("automation record is not a successful completed workflow run")
			}
			if record.Event != "workflow_dispatch" || record.HeadBranch != event.Repository.DefaultBranch || record.HeadSHA == "" || record.Actor != "yersonargotev" || event.PullRequest.Author != "app/github-actions" {
				return errors.New("automation run lacks the protected workflow identity")
			}
			allowed := false
			switch {
			case record.Workflow == "Synchronize pack source" && record.Path == ".github/workflows/sync-pack-source.yml" && strings.HasPrefix(event.PullRequest.HeadRef, "sync/"):
				allowed = true
			case record.Workflow == "Release" && record.Path == ".github/workflows/release.yml" && strings.HasPrefix(event.PullRequest.HeadRef, "release/"):
				allowed = true
			}
			if !allowed {
				return errors.New("automation run is not an authorized proposal workflow for this branch")
			}
		default:
			return errors.New("automation record kind is not authorized")
		}
	}
	return nil
}

func validateCanonicalRecordURL(repository string, declaration ExceptionDeclaration) error {
	parsed, err := url.Parse(declaration.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("exception record is not a canonical GitHub URL")
	}
	prefix := "/" + repository + "/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return errors.New("exception record URL does not belong to this repository")
	}
	remainder := strings.TrimPrefix(parsed.Path, prefix)
	var identifier string
	switch declaration.Type {
	case "private-security":
		identifier = strings.TrimPrefix(remainder, "security/advisories/")
		if identifier == remainder || !strings.HasPrefix(identifier, "GHSA-") || strings.Contains(identifier, "/") {
			return errors.New("private-security record URL is not canonical")
		}
	case "urgent-revert":
		identifier = strings.TrimPrefix(remainder, "issues/")
		if identifier == remainder || strings.Contains(identifier, "/") {
			return errors.New("urgent-revert record URL is not canonical")
		}
		if number, err := strconv.Atoi(identifier); err != nil || number <= 0 {
			return errors.New("urgent-revert record URL has no issue number")
		}
	case "automation":
		prefix := "actions/runs/"
		if strings.HasPrefix(remainder, "pull/") {
			prefix = "pull/"
		}
		identifier = strings.TrimPrefix(remainder, prefix)
		if identifier == remainder || strings.Contains(identifier, "/") {
			return errors.New("automation record URL is not canonical")
		}
		if number, err := strconv.ParseInt(identifier, 10, 64); err != nil || number <= 0 {
			return errors.New("automation record URL has no run identifier")
		}
	}
	return nil
}
