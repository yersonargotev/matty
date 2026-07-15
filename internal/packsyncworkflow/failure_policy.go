package packsyncworkflow

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type FailureArtifactContext struct {
	SourceID     string
	PlanID       string
	BaseSHA      string
	CandidateSHA string
	RunURL       string
	Blockers     []string
}

func NewFailureArtifact(context FailureArtifactContext, err error) FailureArtifact {
	kind := FailureIntegrity
	var failure Failure
	if errors.As(err, &failure) {
		kind = failure.Kind
	}
	blockers := append([]string(nil), context.Blockers...)
	if len(blockers) == 0 {
		if failure.Blocker != "" {
			blockers = []string{failure.Blocker}
		} else {
			blockers = []string{publicBlocker(kind)}
		}
	}
	recovery := publicRecovery(kind)
	if failure.Recovery != "" {
		recovery = failure.Recovery
	}
	return FailureArtifact{SchemaVersion: 1, State: "blocked", SourceID: context.SourceID, PlanID: context.PlanID, BaseSHA: context.BaseSHA, CandidateSHA: context.CandidateSHA, Blockers: blockers, Recovery: []string{recovery}, RunURL: context.RunURL, ContainsSecrets: false, ContainsUpstreamBytes: false}
}

func publicBlocker(kind FailureKind) string {
	switch kind {
	case FailureTransient:
		return "A bounded transient transport operation remained unavailable."
	case FailureProvenance:
		return "Exact configured source provenance could not be revalidated."
	case FailureClassification:
		return "Classification evidence is unavailable, invalid, or stale."
	case FailureValidation:
		return "The complete Matty-owned validation suite did not pass."
	case FailureOwnership:
		return "Automation ownership of the stable branch or pull request is absent or ambiguous."
	case FailureDivergence:
		return "Repository, branch, or pull-request state diverged from the sealed publication identity."
	default:
		return "Canonical integrity validation failed closed."
	}
}

func publicRecovery(kind FailureKind) string {
	switch kind {
	case FailureTransient:
		return "Start a new dispatch pinned to the resolved exact commit; pre-resolution failures may select latest-stable again."
	case FailureProvenance:
		return "Review and explicitly re-establish configured source provenance; never refresh moved identity automatically."
	case FailureClassification:
		return "Provide valid evidence for the exact plan, candidate and base; human evidence requires a fresh inspection-first dispatch."
	case FailureValidation:
		return "Correct the Matty-owned validation failure in a separate implementation change, then start a fresh dispatch."
	case FailureOwnership, FailureDivergence:
		return "Restore the expected automation-owned metadata and head or close the owned PR; never overwrite reviewer work or open a competitor."
	default:
		return "Restore canonical sealed inputs and local bundle integrity, then begin with a fresh Check."
	}
}

func ClassifyHTTPFailure(statusCode int, retryAfter string, err error) error {
	if statusCode == http.StatusRequestTimeout || statusCode == http.StatusTooManyRequests || statusCode >= 500 {
		return Failure{Kind: FailureTransient, RetryAfter: ParseRetryAfter(retryAfter, time.Now()), Err: err}
	}
	return Failure{Kind: FailureProvenance, Err: err}
}

func ClassifyNetworkFailure(err error) error {
	var network net.Error
	if errors.As(err, &network) && (network.Timeout() || network.Temporary()) {
		return Failure{Kind: FailureTransient, Err: errors.New("transport unavailable")}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "connection reset") || strings.Contains(message, "temporary failure") || strings.Contains(message, "http 429") || strings.Contains(message, "rate limit") || strings.Contains(message, "http 5") {
		return Failure{Kind: FailureTransient, RetryAfter: retryAfterFromText(message), Err: errors.New("transport unavailable")}
	}
	return Failure{Kind: FailureIntegrity, Err: err}
}

func ParseRetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(strings.TrimSpace(value)); err == nil {
		if delay := deadline.Sub(now); delay > 0 {
			return delay
		}
	}
	return 0
}

func retryAfterFromText(message string) time.Duration {
	index := strings.Index(message, "retry-after:")
	if index < 0 {
		return 0
	}
	value := strings.Fields(message[index+len("retry-after:"):])
	if len(value) == 0 {
		return 0
	}
	return ParseRetryAfter(value[0], time.Now())
}
