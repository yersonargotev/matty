// Package governanceauth validates the trusted metadata that authorizes a
// Packy pull request. It deliberately has no GitHub client or mutation surface.
package governanceauth

import (
	"errors"
	"fmt"
)

const ApprovedLabel = "status:approved"

// Event is the pull_request_target metadata used for authorization.
type Event struct {
	Action     string `json:"action"`
	Repository struct {
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
	} `json:"repository"`
	PullRequest struct {
		Number int `json:"number"`
		Base   struct {
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
		statusLabels := 0
		for _, label := range issue.Labels {
			switch label.Name {
			case ApprovedLabel:
				approved = true
				statusLabels++
			case "status:needs-review":
				statusLabels++
			}
		}
		if !approved || statusLabels != 1 {
			return fmt.Errorf("closing issue #%d does not have exactly one approved delivery status", issue.Number)
		}
	}

	return nil
}
