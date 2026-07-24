package addyacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const PromotionHarnessSchema = "addy-promotion-harness.v1"

type PromotionHarnessMode string

const (
	PromotionHarnessSynthetic      PromotionHarnessMode = "synthetic_qualification"
	PromotionHarnessExactCandidate PromotionHarnessMode = "exact_evaluated_candidate"
)

type PromotionRowResult struct {
	Evidence      any
	PermittedDiff []string
}

type PromotionRowEvaluator func(PromotionRow, string) (PromotionRowResult, error)
type PromotionGateBoundary func(int, []PromotionHarnessRow) error

type PromotionHarness struct {
	Root     string
	Context  PromotionValidationContext
	Mode     PromotionHarnessMode
	Evaluate PromotionRowEvaluator
	Boundary PromotionGateBoundary
}

type PromotionMutationProof struct {
	ObservedDiff       []string `json:"observed_diff"`
	PermittedDiff      []string `json:"permitted_diff"`
	ZeroMutation       bool     `json:"zero_mutation"`
	ExactPermittedDiff bool     `json:"exact_permitted_diff"`
}

type PromotionHarnessRow struct {
	ID             string                 `json:"id"`
	Number         int                    `json:"number"`
	Gate           int                    `json:"gate"`
	OwningTest     string                 `json:"owning_test"`
	Result         string                 `json:"result"`
	Diagnostic     string                 `json:"diagnostic"`
	EvidenceSHA256 string                 `json:"evidence_sha256"`
	Proof          PromotionMutationProof `json:"proof"`
}

type PromotionHarnessReport struct {
	Schema         string                `json:"schema"`
	Mode           PromotionHarnessMode  `json:"mode"`
	Repository     string                `json:"repository"`
	CommitSHA      string                `json:"commit_sha"`
	WorkflowDigest string                `json:"workflow_digest"`
	RunID          string                `json:"run_id"`
	Qualified      bool                  `json:"qualified"`
	Rows           []PromotionHarnessRow `json:"rows"`
}

func (r PromotionHarnessReport) CanonicalJSON() ([]byte, error) {
	if r.Rows == nil {
		return nil, errors.New("promotion harness rows must be non-null")
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (h PromotionHarness) Run() (PromotionHarnessReport, error) {
	report := PromotionHarnessReport{Schema: PromotionHarnessSchema, Mode: h.Mode, Repository: h.Context.Repository, CommitSHA: contextCommit(h.Context), WorkflowDigest: h.Context.WorkflowDigest, RunID: h.Context.RunID, Rows: []PromotionHarnessRow{}}
	if err := validatePromotionContext(h.Context); err != nil {
		return report, fmt.Errorf("validate promotion harness context: %w", err)
	}
	if h.Mode != PromotionHarnessSynthetic && h.Mode != PromotionHarnessExactCandidate {
		return report, errors.New("promotion harness mode is invalid")
	}
	if h.Evaluate == nil {
		return report, errors.New("promotion harness evaluator is required")
	}
	if !filepath.IsAbs(h.Root) || filepath.Clean(h.Root) != h.Root {
		return report, errors.New("promotion harness root must be a clean absolute path")
	}
	entries, err := os.ReadDir(h.Root)
	if err != nil {
		return report, fmt.Errorf("read promotion harness root: %w", err)
	}
	if len(entries) != 0 {
		return report, errors.New("promotion harness root must be empty")
	}

	blocked := false
	rows := PromotionRows()
	for gate := 1; gate <= 6; gate++ {
		start := len(report.Rows)
		for _, row := range rows {
			if row.Gate != gate {
				continue
			}
			if blocked {
				report.Rows = append(report.Rows, suppressedHarnessRow(row))
				continue
			}
			got := h.runRow(row)
			report.Rows = append(report.Rows, got)
		}
		if blocked {
			continue
		}
		gateFailed := false
		for _, row := range report.Rows[start:] {
			if row.Result != PromotionPassed {
				gateFailed = true
			}
		}
		if !gateFailed && h.Boundary != nil {
			if err := h.Boundary(gate, append([]PromotionHarnessRow(nil), report.Rows[start:]...)); err != nil {
				gateFailed = true
				for i := start; i < len(report.Rows); i++ {
					report.Rows[i].Result = "failed"
					report.Rows[i].Diagnostic = fmt.Sprintf("ADDY-CLAUDE-PROMOTION-GATE-%02d-BOUNDARY", gate)
				}
			}
		}
		blocked = gateFailed
	}
	report.Qualified = true
	for _, row := range report.Rows {
		if row.Result != PromotionPassed {
			report.Qualified = false
		}
	}
	if remaining, err := os.ReadDir(h.Root); err != nil || len(remaining) != 0 {
		return report, errors.New("promotion harness did not restore its disposable root")
	}
	return report, nil
}

func (h PromotionHarness) runRow(row PromotionRow) PromotionHarnessRow {
	out := PromotionHarnessRow{ID: row.ID, Number: row.Number, Gate: row.Gate, OwningTest: row.OwningTest, Result: PromotionPassed, Proof: PromotionMutationProof{ObservedDiff: []string{}, PermittedDiff: []string{}, ZeroMutation: true, ExactPermittedDiff: true}}
	child := filepath.Join(h.Root, fmt.Sprintf("gate-%d-row-%02d", row.Gate, row.Number))
	if err := os.Mkdir(child, 0o700); err != nil {
		out.Result, out.Diagnostic = "failed", row.BlockedDiagnostic
		return out
	}
	result, evalErr := h.Evaluate(row, child)
	observed, scanErr := relativeFiles(child)
	permitted, permitErr := canonicalPaths(result.PermittedDiff)
	out.Proof.ObservedDiff, out.Proof.PermittedDiff = observed, permitted
	out.Proof.ZeroMutation = len(observed) == 0
	out.Proof.ExactPermittedDiff = equalStrings(observed, permitted)
	evidence, marshalErr := json.Marshal(result.Evidence)
	if marshalErr == nil {
		sum := sha256.Sum256(evidence)
		out.EvidenceSHA256 = hex.EncodeToString(sum[:])
	}
	_ = os.RemoveAll(child)
	if evalErr != nil || scanErr != nil || permitErr != nil || marshalErr != nil || !out.Proof.ExactPermittedDiff {
		out.Result = "failed"
		cause := firstError(evalErr, scanErr, permitErr, marshalErr)
		if cause == nil {
			cause = errors.New("observed diff does not exactly match permitted diff")
		}
		_ = cause
		out.Diagnostic = row.BlockedDiagnostic
	}
	return out
}

func suppressedHarnessRow(row PromotionRow) PromotionHarnessRow {
	return PromotionHarnessRow{ID: row.ID, Number: row.Number, Gate: row.Gate, OwningTest: row.OwningTest, Result: "suppressed", Diagnostic: row.BlockedDiagnostic + ": suppressed by earlier gate", Proof: PromotionMutationProof{ObservedDiff: []string{}, PermittedDiff: []string{}, ZeroMutation: true, ExactPermittedDiff: true}}
}

func relativeFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(paths)
	if paths == nil {
		paths = []string{}
	}
	return paths, err
}
func canonicalPaths(in []string) ([]string, error) {
	out := append([]string(nil), in...)
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)
	for i, p := range out {
		if p == "" || filepath.IsAbs(p) || filepath.ToSlash(filepath.Clean(p)) != p || strings.HasPrefix(p, "../") || (i > 0 && out[i-1] == p) {
			return out, fmt.Errorf("permitted diff path %q is invalid", p)
		}
	}
	return out, nil
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// PromotionAggregateCandidate supplies production-only facts not derivable from row reports.
type PromotionAggregateCandidate struct {
	PackageCandidate string
	ClaudeIdentities []string
	AtomicitySHA256  string
}

func (r PromotionHarnessReport) BuildAggregate(context PromotionValidationContext, candidate PromotionAggregateCandidate) (PromotionEvidence, error) {
	exactPR := context.PullRequest > 0 && context.EvaluatedMergeSHA != "" && context.Tag == ""
	exactTag := context.PullRequest == 0 && context.EvaluatedMergeSHA == "" && context.Tag != ""
	if r.Mode != PromotionHarnessExactCandidate || !r.Qualified || !context.PromotionChange || context.FoundationChange || (!exactPR && !exactTag) {
		return PromotionEvidence{}, errors.New("promotion aggregate requires an exact evaluated-candidate report")
	}
	if r.Repository != context.Repository || r.CommitSHA != contextCommit(context) || r.WorkflowDigest != context.WorkflowDigest || r.RunID != context.RunID || len(r.Rows) != len(PromotionRows()) {
		return PromotionEvidence{}, errors.New("promotion harness report does not match trusted evaluated candidate")
	}
	e := newEmptyPromotionEvidence(context, PromotionApplicable)
	e.PackageCandidate, e.ClaudeIdentities, e.AtomicitySHA256 = candidate.PackageCandidate, append([]string(nil), candidate.ClaudeIdentities...), candidate.AtomicitySHA256
	e.Rows = make([]PromotionRowEvidence, len(r.Rows))
	for i, row := range r.Rows {
		if row.ID != PromotionRows()[i].ID || row.Result != PromotionPassed || !row.Proof.ExactPermittedDiff {
			return PromotionEvidence{}, errors.New("promotion harness report is incomplete or synthetic")
		}
		e.Rows[i] = PromotionRowEvidence{ID: row.ID, Result: PromotionPassed, EvidenceSHA256: row.EvidenceSHA256, CommitSHA: contextCommit(context), WorkflowDigest: context.WorkflowDigest, RunID: context.RunID, CollectedAt: context.Now.UTC()}
	}
	if err := ValidatePromotionEvidence(e, context); err != nil {
		return PromotionEvidence{}, err
	}
	return e, nil
}
