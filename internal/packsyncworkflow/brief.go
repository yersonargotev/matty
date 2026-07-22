package packsyncworkflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/yersonargotev/packy/internal/packsync"
)

type ClassifierTrace struct {
	PackID                 string `json:"pack_id"`
	Model                  string `json:"model"`
	PromptSHA256           string `json:"prompt_sha256"`
	CanonicalInputSHA256   string `json:"canonical_input_sha256"`
	StructuredOutputSHA256 string `json:"structured_output_sha256"`
}

type ReviewBrief struct {
	SchemaVersion           int                               `json:"schema_version"`
	Actor                   string                            `json:"actor"`
	RunID                   string                            `json:"run_id"`
	RunAttempt              string                            `json:"run_attempt"`
	RunURL                  string                            `json:"run_url"`
	Repository              string                            `json:"repository"`
	Request                 DispatchRequest                   `json:"request"`
	Candidate               packsync.Candidate                `json:"candidate"`
	PlanID                  string                            `json:"plan_id"`
	BaseSHA                 string                            `json:"base_sha"`
	HeadSHA                 string                            `json:"head_sha"`
	ResultTreeSHA           string                            `json:"result_tree_sha"`
	Branch                  string                            `json:"branch"`
	PullRequest             int                               `json:"pull_request,omitempty"`
	Changes                 []packsync.Change                 `json:"changes"`
	Discoveries             []string                          `json:"unselected_discoveries"`
	SelectedResources       []packsync.ResourceEvidence       `json:"selected_resources"`
	PreviousSnapshotSHA256  string                            `json:"previous_snapshot_sha256,omitempty"`
	ProposedSnapshotSHA256  string                            `json:"proposed_snapshot_sha256"`
	Classification          []packsync.ClassificationEvidence `json:"classification"`
	ClassifierTrace         []ClassifierTrace                 `json:"classifier_trace,omitempty"`
	ApplyStatus             string                            `json:"apply_status"`
	Validation              ValidationGates                   `json:"validation"`
	UpstreamContentExecuted bool                              `json:"upstream_content_executed"`
	Blockers                []string                          `json:"blockers"`
	DecisionReady           bool                              `json:"decision_ready"`
	AutoMerge               bool                              `json:"auto_merge"`
	ManualMergeRequired     bool                              `json:"manual_merge_required"`
	InvalidationConditions  []string                          `json:"invalidation_conditions"`
	Recovery                []string                          `json:"recovery"`
}

func (brief ReviewBrief) CanonicalJSON() ([]byte, error) {
	validPreviousSnapshot := len(brief.PreviousSnapshotSHA256) == 64 || (brief.Request.Operation == OperationRegister && brief.PreviousSnapshotSHA256 == "")
	repository, runID, validRun := parseActionsRunURL(brief.RunURL)
	if brief.SchemaVersion != 1 || brief.Request.Validate() != nil || !validRun || repository != brief.Repository || runID != brief.RunID || brief.PlanID == "" || brief.Branch != "sync/"+brief.Request.SourceID || requireFullSHA("base", brief.BaseSHA) != nil || requireFullSHA("head", brief.HeadSHA) != nil || requireFullSHA("result tree", brief.ResultTreeSHA) != nil || len(brief.SelectedResources) == 0 || !validPreviousSnapshot || len(brief.ProposedSnapshotSHA256) != 64 || !brief.Validation.Complete() || brief.UpstreamContentExecuted || brief.AutoMerge || !brief.ManualMergeRequired {
		return nil, fmt.Errorf("review brief is incomplete or contradicts synchronization policy")
	}
	data, err := json.Marshal(brief)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := json.Indent(&output, data, "", "  "); err != nil {
		return nil, err
	}
	output.WriteByte('\n')
	return output.Bytes(), nil
}

var (
	githubOwnerPattern      = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	githubRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)
)

func parseActionsRunURL(value string) (string, string, bool) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.String() != value || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" || parsed.RawPath != "" {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(parts) != 5 || !githubOwnerPattern.MatchString(parts[0]) || strings.Contains(parts[0], "--") || !githubRepositoryPattern.MatchString(parts[1]) || parts[1] == "." || parts[1] == ".." || parts[2] != "actions" || parts[3] != "runs" {
		return "", "", false
	}
	runID, err := strconv.ParseUint(parts[4], 10, 64)
	if err != nil || runID == 0 || strconv.FormatUint(runID, 10) != parts[4] {
		return "", "", false
	}
	return parts[0] + "/" + parts[1], parts[4], true
}

// Markdown renders only the canonical JSON plus a fixed summary. It never
// recomputes plan, provenance, classification, or readiness facts.
func (brief ReviewBrief) Markdown() (string, error) {
	canonical, err := brief.CanonicalJSON()
	if err != nil {
		return "", err
	}
	status := "blocked"
	if brief.DecisionReady {
		status = "decision-ready"
	}
	return fmt.Sprintf("## Packy pack synchronization\n\n- Source: `%s`\n- Candidate: `%s`\n- Plan: `%s`\n- Base/head/tree: `%s` / `%s` / `%s`\n- State: **%s**\n- Auto-merge: disabled; manual merge required.\n\nAuthorization-Exception: automation\nAuthorization-Record: %s\n\n<details><summary>Canonical synchronization evidence</summary>\n\n```json\n%s```\n</details>\n", brief.Request.SourceID, brief.Candidate.Commit, brief.PlanID, brief.BaseSHA, brief.HeadSHA, brief.ResultTreeSHA, status, brief.RunURL, strings.TrimSuffix(string(canonical), "\n")+"\n"), nil
}
