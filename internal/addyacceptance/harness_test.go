package addyacceptance

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPromotionHarnessRunsStableRowsDeterministically(t *testing.T) {
	root := t.TempDir()
	h := PromotionHarness{Root: root, Context: harnessContext(), Mode: PromotionHarnessSynthetic, Evaluate: func(row PromotionRow, child string) (PromotionRowResult, error) {
		if err := os.WriteFile(filepath.Join(child, "evidence.json"), []byte(row.ID), 0o600); err != nil {
			t.Fatal(err)
		}
		return PromotionRowResult{Evidence: map[string]any{"id": row.ID, "ok": true}, PermittedDiff: []string{"evidence.json"}}, nil
	}}
	first, err := h.Run()
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !first.Qualified || len(first.Rows) != 14 {
		t.Fatalf("unexpected report: %#v", first)
	}
	for i, want := range PromotionRows() {
		got := first.Rows[i]
		if got.ID != want.ID || got.Number != want.Number || got.Gate != want.Gate || got.OwningTest != want.OwningTest || got.EvidenceSHA256 == "" || !got.Proof.ExactPermittedDiff {
			t.Fatalf("row %d lost stable identity or proof: %#v", i, got)
		}
	}
	one, _ := first.CanonicalJSON()
	two, _ := second.CanonicalJSON()
	if string(one) != string(two) {
		t.Fatal("canonical report changed across identical rerun")
	}
}

func TestPromotionHarnessFailureSuppressesEveryLaterGate(t *testing.T) {
	for gate := 1; gate <= 6; gate++ {
		t.Run(string(rune('0'+gate)), func(t *testing.T) {
			calls := map[int]int{}
			h := PromotionHarness{Root: t.TempDir(), Context: harnessContext(), Mode: PromotionHarnessSynthetic, Evaluate: func(row PromotionRow, child string) (PromotionRowResult, error) {
				calls[row.Gate]++
				return PromotionRowResult{Evidence: row.ID}, nil
			}, Boundary: func(g int, rows []PromotionHarnessRow) error {
				if g == gate {
					return os.ErrInvalid
				}
				return nil
			}}
			r, err := h.Run()
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range r.Rows {
				if row.Gate > gate && row.Result != "suppressed" {
					t.Fatalf("later row was not suppressed: %#v", row)
				}
			}
			for later := gate + 1; later <= 6; later++ {
				if calls[later] != 0 {
					t.Fatalf("gate %d evaluator ran after failure", later)
				}
			}
		})
	}
}

func TestPromotionHarnessEveryRowHasOneFactTwinAndCanonicalDiagnostic(t *testing.T) {
	for _, twin := range PromotionRows() {
		t.Run(twin.ID, func(t *testing.T) {
			run := func(permitted string) PromotionHarnessReport {
				h := PromotionHarness{Root: t.TempDir(), Context: harnessContext(), Mode: PromotionHarnessSynthetic, Evaluate: func(row PromotionRow, child string) (PromotionRowResult, error) {
					if row.Number == twin.Number {
						_ = os.WriteFile(filepath.Join(child, "fact"), []byte("x"), 0o600)
						return PromotionRowResult{Evidence: row.ID, PermittedDiff: []string{permitted}}, nil
					}
					return PromotionRowResult{Evidence: row.ID}, nil
				}}
				r, err := h.Run()
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			positive, negative := run("fact"), run("other")
			if !positive.Qualified || negative.Qualified {
				t.Fatal("one-fact twin did not distinguish exact permitted diff")
			}
			if got := negative.Rows[twin.Number-1].Diagnostic; got != twin.BlockedDiagnostic {
				t.Fatalf("non-canonical diagnostic %q", got)
			}
		})
	}
}

func TestPromotionHarnessBoundaryDiagnosticIsStable(t *testing.T) {
	run := func(detail string) PromotionHarnessReport {
		h := PromotionHarness{Root: t.TempDir(), Context: harnessContext(), Mode: PromotionHarnessSynthetic, Evaluate: func(row PromotionRow, child string) (PromotionRowResult, error) {
			return PromotionRowResult{Evidence: row.ID}, nil
		}, Boundary: func(gate int, rows []PromotionHarnessRow) error {
			if gate == 2 {
				return errors.New(detail)
			}
			return nil
		}}
		r, err := h.Run()
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if first, second := run("/private/root/a"), run("/other/root/b"); first.Rows[3].Diagnostic != second.Rows[3].Diagnostic {
		t.Fatalf("boundary diagnostic included ambient detail: %q != %q", first.Rows[3].Diagnostic, second.Rows[3].Diagnostic)
	}
}

func TestPromotionHarnessRejectsNonEmptyRootAndLeavesOutsideUntouched(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "sentinel")
	if err := os.WriteFile(outside, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := PromotionHarness{Root: root, Context: harnessContext(), Mode: PromotionHarnessSynthetic, Evaluate: func(row PromotionRow, child string) (PromotionRowResult, error) {
		return PromotionRowResult{Evidence: row.ID}, nil
	}}
	if _, err := h.Run(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(outside)
	if string(got) != "same" {
		t.Fatal("harness mutated outside its injected root")
	}
	if err := os.WriteFile(filepath.Join(root, "occupied"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Run(); err == nil {
		t.Fatal("non-empty root accepted")
	}
}

func TestPromotionHarnessAggregateRequiresExactCandidate(t *testing.T) {
	ctx := harnessContext()
	ctx.PromotionChange = true
	ctx.PullRequest = 7
	ctx.EvaluatedMergeSHA = strings.Repeat("d", 40)
	run := func(mode PromotionHarnessMode) PromotionHarnessReport {
		h := PromotionHarness{Root: t.TempDir(), Context: ctx, Mode: mode, Evaluate: func(row PromotionRow, child string) (PromotionRowResult, error) {
			return PromotionRowResult{Evidence: row.ID}, nil
		}}
		r, err := h.Run()
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	facts := PromotionAggregateCandidate{PackageCandidate: strings.Repeat("a", 64), ClaudeIdentities: []string{"claude"}, AtomicitySHA256: strings.Repeat("b", 64)}
	if _, err := run(PromotionHarnessSynthetic).BuildAggregate(ctx, facts); err == nil {
		t.Fatal("synthetic rows 11-14 authorized production")
	}
	exact := run(PromotionHarnessExactCandidate)
	evidence, err := exact.BuildAggregate(ctx, facts)
	if err != nil {
		t.Fatal(err)
	}
	changed := ctx
	changed.RunID = "other"
	if err := ValidatePromotionEvidence(evidence, changed); err == nil {
		t.Fatal("cross-run aggregate accepted")
	}
	changed = ctx
	changed.EvaluatedMergeSHA = strings.Repeat("e", 40)
	if err := ValidatePromotionEvidence(evidence, changed); err == nil {
		t.Fatal("cross-commit aggregate accepted")
	}
	if !reflect.DeepEqual(exact.Rows[10].ID, PromotionRows()[10].ID) || !reflect.DeepEqual(exact.Rows[13].ID, PromotionRows()[13].ID) {
		t.Fatal("production rows missing")
	}

	tagContext := ctx
	tagContext.PullRequest, tagContext.EvaluatedMergeSHA, tagContext.Tag = 0, "", "v0.9.0"
	tagReport := exact
	tagReport.CommitSHA = tagContext.HeadSHA
	if _, err := tagReport.BuildAggregate(tagContext, facts); err != nil {
		t.Fatalf("exact-tag aggregate rejected: %v", err)
	}
}

func harnessContext() PromotionValidationContext {
	return PromotionValidationContext{Repository: "owner/repo", PullRequest: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), Workflow: "promotion.yml", WorkflowDigest: strings.Repeat("c", 64), MatrixVersion: PromotionMatrixVersion, RunID: "run-1", Now: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC), MaxAge: time.Hour, Inputs: IndependentPromotionInputs{BaseSHA256: strings.Repeat("1", 64), HeadSHA256: strings.Repeat("2", 64), HistorySHA256: strings.Repeat("3", 64), DiffSHA256: strings.Repeat("4", 64)}}
}
