package claudecode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type classicRunner func(Command) Result

func (f classicRunner) Run(_ context.Context, c Command) Result { return f(c) }

func TestInspectClassicIsInertAndLeavesUnsupportedMCPPending(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("skill"), 0600); err != nil {
		t.Fatal(err)
	}
	l := NewCanonicalLayout(home)
	a := NewSurfaceAdapter("", l, filepath.Join(t.TempDir(), "state"), "", nil, nil)
	p, err := a.InspectClassic(context.Background(), ClassicRequest{Goal: ClassicPresent, Desired: ClassicDesired{Skills: []ClassicSkill{{ID: "skill:x", Name: "x", SourcePath: source}}, Instruction: &ClassicInstruction{ID: "instruction:classic", Content: "hello"}, MCP: &ClassicMCP{ID: "mcp:engram", Name: "engram", Command: "engram", Args: []string{"mcp"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(l.SkillsDir, "x")); !os.IsNotExist(err) {
		t.Fatalf("inspection mutated skill: %v", err)
	}
	if _, err := os.Stat(l.InstructionsFile); !os.IsNotExist(err) {
		t.Fatalf("inspection mutated instructions: %v", err)
	}
	if len(p.Actions()) != 2 || p.Actions()[0].External || p.Actions()[1].External {
		t.Fatalf("safe local actions = %#v", p.Actions())
	}
	if len(p.PendingPrerequisites()) != 1 || p.Compatibility() != CompatibilityMissing {
		t.Fatalf("pending=%v compatibility=%s", p.PendingPrerequisites(), p.Compatibility())
	}
	if got := p.DesiredOwnership(); len(got) != 3 || got[2].EnvironmentFingerprint == "" {
		t.Fatalf("ownership=%#v", got)
	}
}

func TestInspectClassicCollisionPlansZeroEffectsForCollision(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "skill")
	_ = os.MkdirAll(source, 0700)
	_ = os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("x"), 0600)
	l := NewCanonicalLayout(home)
	_ = os.MkdirAll(l.SkillsDir, 0700)
	_ = os.WriteFile(filepath.Join(l.SkillsDir, "x"), []byte("foreign"), 0600)
	a := NewSurfaceAdapter("", l, "", "", nil, nil)
	p, err := a.InspectClassic(context.Background(), ClassicRequest{Goal: ClassicPresent, Desired: ClassicDesired{Skills: []ClassicSkill{{ID: "x", Name: "x", SourcePath: source}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Actions()) != 0 || len(p.Blockers()) != 1 || len(p.Preserved()) != 1 {
		t.Fatalf("plan actions=%v blockers=%v preserved=%v", p.Actions(), p.Blockers(), p.Preserved())
	}
}

func TestInspectClassicNeverAdoptsExactPreexistingFragments(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "skill")
	_ = os.MkdirAll(source, 0o700)
	_ = os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("exact"), 0o600)
	layout := NewCanonicalLayout(home)
	_ = os.MkdirAll(layout.SkillsDir, 0o700)
	_ = os.Symlink(source, filepath.Join(layout.SkillsDir, "exact"))
	document, err := UpsertInstructionContribution("", InstructionContribution{ContributorID: "classic", Content: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(layout.InstructionsFile, []byte(document), 0o600)
	_ = os.WriteFile(layout.UserMCPFile, []byte(`{"mcpServers":{"engram":{"command":"engram","args":["mcp"],"env":{}}}}`), 0o600)
	a := NewSurfaceAdapter("", layout, filepath.Join(home, "state"), "claude", classicRunner(func(c Command) Result {
		if len(c.Args) == 1 && c.Args[0] == "--version" {
			return Result{Stdout: "2.1.203"}
		}
		return Result{}
	}), StaticOwnershipSnapshot(OwnershipSnapshot{}))
	plan, err := a.InspectClassic(context.Background(), ClassicRequest{Goal: ClassicPresent, Desired: ClassicDesired{
		Skills:      []ClassicSkill{{ID: "classic:skill:exact", Name: "exact", SourcePath: source}},
		Instruction: &ClassicInstruction{ID: "classic:instruction", Content: "exact"},
		MCP:         &ClassicMCP{ID: "classic:mcp:engram", Name: "engram", Command: "engram", Args: []string{"mcp"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers()) != 3 || len(plan.Actions()) != 0 || len(plan.DesiredOwnership()) != 0 {
		t.Fatalf("exact foreign fragments were adopted: blockers=%v actions=%v ownership=%v", plan.Blockers(), plan.Actions(), plan.DesiredOwnership())
	}
}

func TestApplyClassicReportsOrderedFailureJournalAndRejectsForeignPlan(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "skill")
	_ = os.MkdirAll(source, 0700)
	_ = os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("x"), 0600)
	runner := classicRunner(func(c Command) Result {
		if len(c.Args) == 1 && c.Args[0] == "--version" {
			return Result{Stdout: "2.1.203"}
		}
		return Result{Err: errors.New("secret=must-not-leak")}
	})
	l := NewCanonicalLayout(home)
	a := NewSurfaceAdapter("", l, filepath.Join(t.TempDir(), "state"), "claude", runner, nil)
	p, err := a.InspectClassic(context.Background(), ClassicRequest{Goal: ClassicPresent, Desired: ClassicDesired{Skills: []ClassicSkill{{ID: "skill:x", Name: "x", SourcePath: source}}, MCP: &ClassicMCP{ID: "mcp:engram", Name: "engram", Command: "engram", Args: []string{"mcp"}, Environment: map[string]string{"TOKEN": "must-not-leak"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Actions(); len(got) != 2 || got[0].External || !got[1].External {
		t.Fatalf("action order=%#v", got)
	}
	r, err := a.ApplyClassic(context.Background(), p)
	if err == nil {
		t.Fatal("expected MCP failure")
	}
	if len(r.Completed) != 1 || r.Completed[0] != "skill:x" || r.Failed != "mcp:engram" || len(r.NotStarted) != 0 {
		t.Fatalf("journal=%#v", r)
	}
	if _, err := os.Lstat(filepath.Join(l.SkillsDir, "x")); err != nil {
		t.Fatalf("local action did not complete before MCP: %v", err)
	}
	other := NewSurfaceAdapter("", l, "", "claude", runner, nil)
	if _, err := other.ApplyClassic(context.Background(), p); !errors.Is(err, ErrForeignClassicPlan) {
		t.Fatalf("foreign plan error=%v", err)
	}
}

func TestApplyClassicRestoresExactPriorStateAfterLocalFailure(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "skill")
	_ = os.MkdirAll(source, 0o700)
	_ = os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("x"), 0o600)
	layout := NewCanonicalLayout(home)
	a := NewSurfaceAdapter("", layout, filepath.Join(home, "state"), "claude", classicRunner(func(command Command) Result {
		return Result{Stdout: "2.1.203"}
	}), StaticOwnershipSnapshot(OwnershipSnapshot{}))
	plan, err := a.InspectClassic(context.Background(), ClassicRequest{Goal: ClassicPresent, Desired: ClassicDesired{
		Skills:      []ClassicSkill{{ID: "classic:skill:x", Name: "x", SourcePath: source}},
		Instruction: &ClassicInstruction{ID: "classic:instruction", Content: "instructions"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	original := applyClassicAction
	calls := 0
	applyClassicAction = func(adapter *SurfaceAdapter, ctx context.Context, action capabilitypack.ProjectionAction) error {
		calls++
		if calls == 2 {
			return errors.New("injected local failure")
		}
		return adapter.apply(ctx, action)
	}
	t.Cleanup(func() { applyClassicAction = original })
	result, err := a.ApplyClassic(context.Background(), plan)
	if err == nil || !result.RolledBack || result.RollbackFailed || len(result.Completed) != 0 || result.Failed != "classic:instruction" {
		t.Fatalf("rollback result=%+v err=%v", result, err)
	}
	if _, err := os.Lstat(filepath.Join(layout.SkillsDir, "x")); !os.IsNotExist(err) {
		t.Fatalf("skill prior state not restored: %v", err)
	}
	if _, err := os.Stat(layout.InstructionsFile); !os.IsNotExist(err) {
		t.Fatalf("instruction prior state not restored: %v", err)
	}
}
