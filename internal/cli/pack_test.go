package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/matty/internal/capabilitypack"
)

type alwaysUsableInspector struct{}

func (alwaysUsableInspector) InspectReadiness(context.Context, capabilitypack.Pack, capabilitypack.ActivationObservation, []capabilitypack.ExecutableResolution) (capabilitypack.ReadinessObservation, error) {
	return capabilitypack.ReadinessObservation{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true, Evidence: []string{"fake runtime loaded capability"}}, nil
}

func TestPackHelpDocumentsSupportedRolloutCommands(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{})
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"matty pack list", "matty pack show matty", "matty pack status",
		"status engram --surface codex --require usable",
		"activate matty --surface codex --dry-run", "update matty --surface codex",
		"reconcile matty --surface codex", "reconcile --surface codex",
		"deactivate matty --surface codex", "Approvals", "repeat the original lifecycle",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pack help missing %q:\n%s", want, out)
		}
	}
}

func TestPackRecoveryDryRunRendersTruthfulHistoryWithoutPromptsOrEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _, runner := engramActivationOptions(t, terminal)
	setup := runner.path["engram"] + " setup codex"
	runner.fail = map[string]error{setup: errors.New("setup interrupted")}
	if _, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex"); err == nil || !strings.Contains(err.Error(), "recovery is required") {
		t.Fatalf("initial failure = %v", err)
	}
	before := snapshotTree(t, home)
	previousCalls := len(runner.calls)
	terminal.calls, terminal.prompts = 0, nil
	delete(runner.fail, setup)

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatalf("recovery dry-run: %v\n%s", err, out)
	}
	for _, want := range []string{"Recovery: fresh activate Preview", "Historical outcome: recovery-required", "Completed:", "Failed: external:engram:setup:codex", "Not started: none", "historical plan", "is not replayed", "repeat `matty pack activate engram --surface codex`", "new Preview and approvals are required"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 0 || len(runner.calls) != previousCalls || snapshotTree(t, home) != before {
		t.Fatalf("dry-run caused effects: prompts=%d calls=%v", terminal.calls, runner.calls[previousCalls:])
	}
	terminal.interactive = false
	if _, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex"); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("non-TTY recovery = %v", err)
	}
	if len(runner.calls) != previousCalls || snapshotTree(t, home) != before {
		t.Fatal("non-TTY recovery caused effects")
	}
	terminal.interactive, terminal.approve, terminal.calls = true, false, 0
	if _, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex"); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("cancelled recovery = %v", err)
	}
	if terminal.calls != 1 || len(runner.calls) != previousCalls || snapshotTree(t, home) != before {
		t.Fatal("cancelled recovery caused effects")
	}
}

func TestCapabilityPackRolloutRecoveryMatrixUsesFreshPreview(t *testing.T) {
	for _, packID := range []string{"matty", "engram"} {
		for _, surface := range []string{"codex", "opencode"} {
			t.Run(packID+"-"+surface, func(t *testing.T) {
				terminal := &fakeTerminal{interactive: true, approve: true}
				opts, home, repoRoot, runner := engramActivationOptions(t, terminal)
				bundle := copyPackBundleForUpdate(t, repoRoot)
				opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
				if packID == "matty" {
					manifest := `{"schema_version":1,"id":"matty","version":"1.0.0","provides":[],"requires":{"capabilities":[],"tools":["engram"]},"conflicts":[],"resources":[{"kind":"instruction","id":"engram-memory","source":"instructions/engram-memory.md"},{"kind":"mcp_server","id":"engram","command":"engram","args":["mcp","--tools=agent"]},{"kind":"lifecycle","id":"engram-memory"}]}`
					if err := os.WriteFile(filepath.Join(bundle, "packs", "matty", "pack.json"), []byte(manifest), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				setup := runner.path["engram"] + " setup " + surface
				runner.fail = map[string]error{setup: errors.New("sandboxed setup interruption")}
				if _, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", packID, "--surface", surface); err == nil || !strings.Contains(err.Error(), "recovery is required") {
					t.Fatalf("initial partial attempt = %v", err)
				}
				before := snapshotTree(t, home)
				calls := len(runner.calls)
				delete(runner.fail, setup)
				out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", packID, "--surface", surface, "--dry-run")
				if err != nil {
					t.Fatalf("fresh recovery preview: %v\n%s", err, out)
				}
				for _, want := range []string{"Recovery: fresh activate Preview", "Historical outcome: recovery-required", "is not replayed", "new Preview and approvals are required"} {
					if !strings.Contains(out, want) {
						t.Fatalf("recovery output missing %q:\n%s", want, out)
					}
				}
				if snapshotTree(t, home) != before || len(runner.calls) != calls {
					t.Fatal("recovery Preview mutated state or reran the external action")
				}
				if out, err = executeCommand(t, NewRootCommand(opts), "pack", "activate", packID, "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
					t.Fatalf("fresh recovery Apply: %v\n%s", err, out)
				}
			})
		}
	}
}

func TestCapabilityPackRolloutMatrixStaysInsideSandbox(t *testing.T) {
	operatorHome := os.Getenv("HOME")
	for _, packID := range []string{"matty", "engram"} {
		for _, surface := range []string{"codex", "opencode"} {
			t.Run(packID+"-"+surface, func(t *testing.T) {
				root := t.TempDir()
				home := filepath.Join(root, "home")
				source := filepath.Join(root, "source")
				for _, dir := range []string{"skills", "instructions", "packs"} {
					if err := os.CopyFS(filepath.Join(source, dir), os.DirFS(filepath.Join("..", "..", "bundle", dir))); err != nil {
						t.Fatal(err)
					}
				}
				terminal := &fakeTerminal{interactive: true, approve: true}
				runner := &fakeRunner{}
				env := MapEnv{
					"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "",
					"MATTY_SKILLS_SOURCE": filepath.Join(source, "skills"), "OPENCODE_CONFIG": "",
					"OPENCODE_CONFIG_CONTENT": "", "OPENCODE_CONFIG_DIR": "",
				}
				if packID == "engram" {
					prefix := filepath.Join(root, "homebrew")
					engram := writeEngramExecutable(t, filepath.Join(prefix, "bin"), "engram version 1.19.0")
					runner.path = map[string]string{"engram": engram}
					env["HOMEBREW_PREFIX"], env["PATH"] = prefix, filepath.Dir(engram)
					configureEngramCodexSetupFixture(t, runner, env, engram)
				}
				opts := Options{Env: env, Runner: runner, Terminal: terminal}
				paths, err := ResolvePaths(env)
				if err != nil {
					t.Fatal(err)
				}
				for _, managedPath := range []string{paths.MattyDir, paths.AgentSkillsDir, paths.CodexConfigFile, paths.CodexPromptFile, paths.OpenCodeConfigFile, paths.OpenCodePromptFile} {
					if !pathInside(root, managedPath) {
						t.Fatalf("resolved path escaped sandbox: %s", managedPath)
					}
				}
				if err := os.MkdirAll(filepath.Dir(paths.CodexPromptFile), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths.CodexPromptFile, []byte("operator-owned Codex guidance\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(paths.OpenCodeConfigFile), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths.OpenCodeConfigFile, []byte("{\n  // operator-owned\n  \"model\": \"test/model\"\n}\n"), 0o600); err != nil {
					t.Fatal(err)
				}

				for _, args := range [][]string{{"pack", "list"}, {"pack", "show", packID}, {"pack", "status"}, {"pack", "status", packID, "--surface", surface}} {
					before := snapshotTree(t, root)
					if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
						t.Fatalf("inspection %v: %v\n%s", args, err, out)
					}
					if got := snapshotTree(t, root); got != before {
						t.Fatalf("inspection %v mutated sandbox", args)
					}
				}

				manifestPath := filepath.Join(source, "packs", packID, "pack.json")
				originalManifest := readFileString(t, manifestPath)
				terminal.onApprove = func() {
					changed := strings.Replace(originalManifest, `"version": "1.0.0"`, `"version": "1.0.1"`, 1)
					_ = os.WriteFile(manifestPath, []byte(changed), 0o600)
				}
				if _, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", packID, "--surface", surface); err == nil || !strings.Contains(strings.ToLower(err.Error()), "stale") {
					t.Fatalf("stale activation = %v", err)
				}
				if exists(paths.PackStateFile) {
					t.Fatal("stale activation wrote pack state")
				}
				terminal.onApprove = nil
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", packID, "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
					t.Fatalf("activate: %v\n%s", err, out)
				}
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", packID, "--surface", surface, "--require", "usable"); err == nil || !strings.Contains(out, "Readiness:") {
					t.Fatalf("pending readiness gate: err=%v\n%s", err, out)
				}

				manifest := strings.Replace(readFileString(t, manifestPath), `"version": "1.0.1"`, `"version": "2.0.0"`, 1)
				if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
					t.Fatal(err)
				}
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", packID, "--surface", surface); err != nil || !strings.Contains(out, "catalog-current") {
					t.Fatalf("update: %v\n%s", err, out)
				}
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "reconcile", packID, "--surface", surface); err != nil || (!strings.Contains(out, "Already converged") && !strings.Contains(out, "Verified plan")) {
					t.Fatalf("targeted reconcile: %v\n%s", err, out)
				}
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "reconcile", "--surface", surface); err != nil || (!strings.Contains(out, "Already converged") && !strings.Contains(out, "Verified plan")) {
					t.Fatalf("surface reconcile: %v\n%s", err, out)
				}

				opts.ReadinessInspectors = map[capabilitypack.Surface]capabilitypack.ReadinessInspector{
					capabilitypack.Surface(surface): alwaysUsableInspector{},
				}
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", packID, "--surface", surface, "--require", "usable"); err != nil || !strings.Contains(out, "usable=yes") {
					t.Fatalf("usable readiness gate: %v\n%s", err, out)
				}
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", packID, "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
					t.Fatalf("deactivate: %v\n%s", err, out)
				}
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "list"); err != nil || !strings.Contains(out, "matty") {
					t.Fatalf("Matty core unavailable after deactivation: %v\n%s", err, out)
				}
				if got := readFileString(t, paths.CodexPromptFile); !strings.Contains(got, "operator-owned Codex guidance") {
					t.Fatalf("unmanaged Codex guidance was not preserved: %q", got)
				}
				if got := readFileString(t, paths.OpenCodeConfigFile); !strings.Contains(got, "operator-owned") || !strings.Contains(got, "test/model") {
					t.Fatalf("unmanaged OpenCode config was not preserved: %q", got)
				}
				if operatorHome != "" && strings.HasPrefix(root, filepath.Clean(operatorHome)+string(os.PathSeparator)) {
					t.Fatalf("sandbox unexpectedly nested in operator HOME: %s", root)
				}
			})
		}
	}
}

func pathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

type fakeTerminal struct {
	interactive bool
	approve     bool
	calls       int
	onApprove   func()
	answers     []bool
	prompts     []string
}

func (f *fakeTerminal) Interactive(io.Reader) bool { return f.interactive }
func (f *fakeTerminal) Approve(_ io.Reader, _ io.Writer, prompt string) (bool, error) {
	f.calls++
	f.prompts = append(f.prompts, prompt)
	if f.onApprove != nil {
		f.onApprove()
	}
	if len(f.answers) >= f.calls {
		return f.answers[f.calls-1], nil
	}
	return f.approve, nil
}

func packActivationOptions(t *testing.T, terminal Terminal) (Options, string, string) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	return Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "", "MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}, Runner: &fakeRunner{}, Terminal: terminal}, home, repoRoot
}

func engramActivationOptions(t *testing.T, terminal Terminal) (Options, string, string, *fakeRunner) {
	t.Helper()
	opts, home, repoRoot := packActivationOptions(t, terminal)
	prefix := filepath.Join(t.TempDir(), "homebrew")
	engram := writeEngramExecutable(t, filepath.Join(prefix, "bin"), "engram version 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": engram}}
	opts.Runner = runner
	env := opts.Env.(MapEnv)
	env["HOMEBREW_PREFIX"] = prefix
	env["PATH"] = filepath.Dir(engram)
	env["OPENCODE_CONFIG"] = ""
	env["OPENCODE_CONFIG_CONTENT"] = ""
	env["OPENCODE_CONFIG_DIR"] = ""
	configureEngramCodexSetupFixture(t, runner, env, engram)
	return opts, home, repoRoot, runner
}

func configureEngramCodexSetupFixture(t *testing.T, runner *fakeRunner, env MapEnv, engram string) {
	t.Helper()
	instructionsGolden, err := os.ReadFile(filepath.Join("..", "codex", "testdata", "engram-1.19.0", "engram-instructions.md"))
	if err != nil {
		t.Fatal(err)
	}
	compactGolden, err := os.ReadFile(filepath.Join("..", "codex", "testdata", "engram-1.19.0", "engram-compact-prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	key := engram + " setup codex"
	if runner.after == nil {
		runner.after = map[string]func(){}
	}
	runner.after[key] = func() {
		dir := filepath.Join(env["HOME"], ".codex")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		instructions := filepath.Join(dir, "engram-instructions.md")
		compact := filepath.Join(dir, "engram-compact-prompt.md")
		config := `model_instructions_file = "` + instructions + `"
experimental_compact_prompt_file = "` + compact + `"
[mcp_servers.engram]
command = "` + engram + `"
args = ["mcp", "--tools=agent"]

[marketplaces.engram]
last_updated = "volatile"
source_type = "git"
source = "https://github.com/Gentleman-Programming/engram.git"
ref = "main"

[plugins."engram@engram"]
enabled = true
`
		for path, content := range map[string][]byte{
			filepath.Join(dir, "config.toml"): []byte(config),
			instructions:                      instructionsGolden,
			compact:                           compactGolden,
		} {
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestPackActivateCodexDryRunIsCompletelySideEffectFree(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := packActivationOptions(t, terminal)
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	for _, want := range []string{"Activation dry-run plan plan-", "Digest:", "Phase: reversible-local", "link skill ask-matt", "write instruction matty-guidance"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 0 {
		t.Fatalf("dry-run requested approval %d times", terminal.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("dry-run mutated HOME:\n%s", got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("dry-run mutated source bundle")
	}
}

func TestPackActivateCodexRejectsNonTTYBeforeEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: false, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex")
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("error = %v\n%s", err, out)
	}
	if terminal.calls != 0 {
		t.Fatalf("non-TTY requested approval")
	}
	if _, err := os.Stat(filepath.Join(home, ".matty", "packs.json")); !os.IsNotExist(err) {
		t.Fatalf("state written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("skills changed: %v", err)
	}
}

func TestPackActivateCodexAppliesApprovedPlanAndRepeatIsNoOp(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("activate failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || !strings.Contains(out, "Verified plan") || !strings.Contains(out, "24 Codex projections") {
		t.Fatalf("unexpected interaction/output: calls=%d\n%s", terminal.calls, out)
	}
	if target, err := os.Readlink(filepath.Join(home, ".agents", "skills", "ask-matt")); err != nil || !strings.HasSuffix(target, "bundle/skills/engineering/ask-matt") {
		t.Fatalf("ask-matt link = %q err=%v", target, err)
	}
	prompt, err := os.ReadFile(filepath.Join(home, ".codex", "AGENTS.md"))
	if err != nil || !strings.Contains(string(prompt), "matty:pack:matty-guidance:start") {
		t.Fatalf("prompt = %q err=%v", prompt, err)
	}
	state, err := os.ReadFile(filepath.Join(home, ".matty", "packs.json"))
	if err != nil || !strings.Contains(string(state), `"contributors": [`) || strings.Contains(string(state), "applying_journal") {
		t.Fatalf("state = %s err=%v", state, err)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("repeat failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || !strings.Contains(out, "Already converged") {
		t.Fatalf("repeat was not approval-free no-op: calls=%d\n%s", terminal.calls, out)
	}
}

func TestPackActivateCodexStalePlanExecutesNoActions(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	terminal.onApprove = func() {
		_ = os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
		_ = os.WriteFile(filepath.Join(home, ".codex", "AGENTS.md"), []byte("concurrent change\n"), 0o600)
	}

	_, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex")
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".matty", "packs.json")); !os.IsNotExist(err) {
		t.Fatalf("stale plan wrote state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("stale plan wrote skills: %v", err)
	}
}

func TestPackListAndShowAreSideEffectFree(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	runner := &fakeRunner{}
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "", "MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}, Runner: runner}
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "list")
	if err != nil {
		t.Fatalf("list failed: %v\n%s", err, out)
	}
	for _, want := range []string{"PACK", "engram", "matty", "Persistent memory", "codex, opencode"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list missing %q:\n%s", want, out)
		}
	}
	show, err := executeCommand(t, NewRootCommand(opts), "pack", "show", "engram")
	if err != nil {
		t.Fatalf("show failed: %v\n%s", err, show)
	}
	for _, want := range []string{"Provides capabilities: memory:persistent", "Requires global tools: engram", "Conflicts with capabilities: none", "0 skill, 1 instruction, 1 mcp_server, 1 lifecycle"} {
		if !strings.Contains(show, want) {
			t.Fatalf("show missing %q:\n%s", want, show)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("external calls = %v", runner.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("HOME changed\nbefore:\n%s\nafter:\n%s", beforeHome, got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("bundle changed during discovery")
	}
	if _, err := os.Stat(filepath.Join(home, ".matty", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("state file exists: %v", err)
	}
}

func TestPackShowRejectsUnknownPack(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	opts := Options{Env: MapEnv{"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills")}}
	opts.Env.(MapEnv)["PATH"] = ""
	_, err := executeCommand(t, NewRootCommand(opts), "pack", "show", "mobile")
	if err == nil || !strings.Contains(err.Error(), "unknown capability pack") {
		t.Fatalf("error = %v", err)
	}
}

func TestPackStatusRendersBaselineWithoutSideEffects(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	runner := &fakeRunner{}
	opts := Options{Env: MapEnv{
		"HOME": home, "XDG_CONFIG_HOME": xdg, "PATH": "",
		"MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills"),
	}, Runner: runner}
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	overview, err := executeCommand(t, NewRootCommand(opts), "pack", "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, overview)
	}
	for _, want := range []string{
		"PACK", "SURFACE", "INTENT", "ATTEMPT", "CONFIGURED", "AUTHORIZED", "USABLE", "ACTION",
		"engram  codex", "engram  opencode", "matty   codex", "matty   opencode", "inactive",
	} {
		if !strings.Contains(overview, want) {
			t.Fatalf("overview missing %q:\n%s", want, overview)
		}
	}

	detail, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "engram", "--surface", "codex")
	if err != nil {
		t.Fatalf("targeted status failed: %v\n%s", err, detail)
	}
	for _, want := range []string{
		"engram 1.0.0 on codex", "Intent: inactive", "Latest attempt: none",
		"Readiness: configured=no, authorized=no, usable=no",
		"Projections: 0 verified; 0 drifted; 0 ambiguous", "Pending human actions: none",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail missing %q:\n%s", want, detail)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("external calls = %v", runner.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("HOME changed\nbefore:\n%s\nafter:\n%s", beforeHome, got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("bundle changed during status")
	}
	if _, err := os.Stat(filepath.Join(home, ".matty", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("state file exists: %v", err)
	}
}

func TestPackStatusRequiresCompleteTarget(t *testing.T) {
	repoRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	home := t.TempDir()
	opts := Options{Env: MapEnv{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "",
		"MATTY_SKILLS_SOURCE": filepath.Join(repoRoot, "bundle", "skills"),
	}}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"pack", "status", "engram"}, "--surface is required"},
		{[]string{"pack", "status", "--surface", "codex"}, "a pack is required"},
		{[]string{"pack", "status", "engram", "--surface", "vscode"}, "does not support CLI surface"},
		{[]string{"pack", "status", "missing", "--surface", "codex"}, "unknown capability pack"},
	} {
		_, err := executeCommand(t, NewRootCommand(opts), tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v error = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestPackStatusRequireUsableIsIndependentNonInteractiveGate(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	opts.ReadinessInspectors = map[capabilitypack.Surface]capabilitypack.ReadinessInspector{capabilitypack.SurfaceCodex: alwaysUsableInspector{}}
	if _, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--require", "usable"); err == nil || !strings.Contains(err.Error(), "not freshly observed usable") {
		t.Fatalf("inactive gate error=%v", err)
	}
	if terminal.calls != 0 || exists(filepath.Join(home, ".matty", "packs.json")) {
		t.Fatal("failed status gate prompted or persisted")
	}
	if _, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
		t.Fatal(err)
	}
	prompts := terminal.calls
	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--require", "usable")
	if err != nil || !strings.Contains(out, "configured=yes, authorized=yes, usable=yes") {
		t.Fatalf("gate err=%v\n%s", err, out)
	}
	if terminal.calls != prompts || snapshotTree(t, home) != before {
		t.Fatal("successful status gate prompted or mutated files")
	}
	for _, args := range [][]string{{"pack", "status", "--require", "usable"}, {"pack", "status", "matty", "--surface", "codex", "--require", "authorized"}} {
		if _, err := executeCommand(t, NewRootCommand(opts), args...); err == nil || !strings.Contains(err.Error(), "valid only") {
			t.Fatalf("%v error=%v", args, err)
		}
	}
}

func TestPackActivateOpenCodeDryRunIsCompletelySideEffectFree(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := packActivationOptions(t, terminal)
	opts.Env.(MapEnv)["OPENCODE_CONFIG"] = ""
	opts.Env.(MapEnv)["OPENCODE_CONFIG_CONTENT"] = ""
	opts.Env.(MapEnv)["OPENCODE_CONFIG_DIR"] = ""
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "opencode", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	for _, want := range []string{"Activation dry-run plan plan-", "Surface: opencode", "link OpenCode skill ask-matt", "write OpenCode instruction matty-guidance", "add OpenCode instruction reference"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 0 {
		t.Fatalf("dry-run requested approval")
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("dry-run mutated HOME:\n%s", got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("dry-run mutated source bundle")
	}
}

func TestPackActivateOpenCodeRejectsNonTTYBeforeEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: false, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	_, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "opencode")
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("error = %v", err)
	}
	if terminal.calls != 0 {
		t.Fatal("non-TTY requested approval")
	}
	for _, path := range []string{filepath.Join(home, ".matty", "packs.json"), filepath.Join(home, ".agents"), filepath.Join(home, "xdg", "opencode")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("non-TTY wrote %s: %v", path, err)
		}
	}
}

func TestPackActivateOpenCodePreservesUnmanagedContentAndDoesNotMutateCodex(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	xdg := filepath.Join(home, "xdg", "opencode")
	if err := os.MkdirAll(xdg, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(xdg, "opencode.json")
	existing := "// keep host syntax\n{\n  \"model\": \"anthropic/test\",\n  \"mcp\": {\"jira\": {\"enabled\": true,},},\n  \"instructions\": [\"CONTRIBUTING.md\",],\n}\n"
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	codexPath := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(codexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexPath, []byte("unmanaged Codex guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "opencode")
	if err != nil {
		t.Fatalf("activate failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || !strings.Contains(out, "25 OpenCode projections") {
		t.Fatalf("interaction/output calls=%d\n%s", terminal.calls, out)
	}
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"// keep host syntax", `"model": "anthropic/test"`, `"jira"`, `"CONTRIBUTING.md"`, filepath.Join(xdg, "matty.md")} {
		if !strings.Contains(string(updated), want) {
			t.Fatalf("OpenCode config lost %q:\n%s", want, updated)
		}
	}
	codex, err := os.ReadFile(codexPath)
	if err != nil || string(codex) != "unmanaged Codex guidance\n" {
		t.Fatalf("Codex mutated: %q err=%v", codex, err)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "opencode")
	if err != nil {
		t.Fatalf("repeat failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || !strings.Contains(out, "Already converged") {
		t.Fatalf("repeat not no-op: calls=%d\n%s", terminal.calls, out)
	}
}

func TestPackActivationKeepsCodexAndOpenCodeIndependentAndConverged(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	for _, args := range [][]string{
		{"pack", "activate", "matty", "--surface", "codex"},
		{"pack", "activate", "matty", "--surface", "opencode"},
	} {
		if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
	if terminal.calls != 2 {
		t.Fatalf("approvals = %d, want one per surface", terminal.calls)
	}
	for _, path := range []string{filepath.Join(home, ".codex", "AGENTS.md"), filepath.Join(home, "xdg", "opencode", "opencode.json"), filepath.Join(home, "xdg", "opencode", "matty.md")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing host projection %s: %v", path, err)
		}
	}
	for _, surface := range []string{"codex", "opencode"} {
		out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", surface)
		if err != nil || !strings.Contains(out, "Already converged") {
			t.Fatalf("%s repeat failed/no-op missing: %v\n%s", surface, err, out)
		}
	}
	if terminal.calls != 2 {
		t.Fatalf("converged repeats requested approval: %d", terminal.calls)
	}
	state, err := os.ReadFile(filepath.Join(home, ".matty", "packs.json"))
	if err != nil || !strings.Contains(string(state), `"surface": "codex"`) || !strings.Contains(string(state), `"surface": "opencode"`) {
		t.Fatalf("state did not preserve both surfaces: %s err=%v", state, err)
	}
}

func TestPackActivateEngramDryRunShowsGlobalResolutionAndNoEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot, runner := engramActivationOptions(t, terminal)
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	for _, want := range []string{"Pack: engram 1.0.0", "Phase: executable-external", "engram setup codex", "Phase: host-follow-up", "/hooks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 0 || len(runner.calls) != 0 {
		t.Fatalf("dry-run requested effects: prompts=%d calls=%v", terminal.calls, runner.calls)
	}
	if got := snapshotTree(t, home); got != beforeHome {
		t.Fatalf("dry-run mutated HOME:\n%s", got)
	}
	if got := snapshotTree(t, filepath.Join(repoRoot, "bundle")); got != beforeBundle {
		t.Fatal("dry-run mutated source bundle")
	}
}

func TestPackActivateEngramPromptsForExternalAuthorityAndReportsPendingActions(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _, runner := engramActivationOptions(t, terminal)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex")
	if err != nil {
		t.Fatalf("activate failed: %v\n%s", err, out)
	}
	if terminal.calls != 1 || len(terminal.prompts) != 1 || !strings.Contains(terminal.prompts[0], "executable-external") {
		t.Fatalf("prompts = %#v calls=%d", terminal.prompts, terminal.calls)
	}
	if len(runner.calls) != 1 || !strings.Contains(callStrings(runner.calls)[0], "setup codex") {
		t.Fatalf("external calls = %#v", runner.calls)
	}
	for _, want := range []string{"Readiness: configured=yes, authorized=no, usable=no", "Pending human actions:", "/hooks", "reload Codex"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); err != nil {
		t.Fatalf("Codex MCP projection missing: %v", err)
	}
}

func TestPackActivateEngramNonTTYAndExternalCancellationAreSideEffectFree(t *testing.T) {
	nonTTY := &fakeTerminal{interactive: false, approve: true}
	opts, home, _, runner := engramActivationOptions(t, nonTTY)
	_, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex")
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("non-TTY error = %v", err)
	}
	if len(runner.calls) != 0 || exists(filepath.Join(home, ".matty", "packs.json")) || exists(filepath.Join(home, ".codex")) {
		t.Fatalf("non-TTY caused effects: calls=%v", runner.calls)
	}

	cancel := &fakeTerminal{interactive: true, approve: true, answers: []bool{false}}
	opts, home, _, runner = engramActivationOptions(t, cancel)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex")
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("cancellation error = %v\n%s", err, out)
	}
	if cancel.calls != 1 || len(runner.calls) != 0 || exists(filepath.Join(home, ".matty", "packs.json")) || exists(filepath.Join(home, ".codex")) {
		t.Fatalf("cancellation caused effects: prompts=%v calls=%v", cancel.prompts, runner.calls)
	}
}

func TestPackActivateEngramSurfacesRemainIndependent(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _, runner := engramActivationOptions(t, terminal)
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex"); err != nil {
		t.Fatalf("Codex activation failed: %v\n%s", err, out)
	}
	codexConfig := readFileString(t, filepath.Join(home, ".codex", "config.toml"))
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "opencode"); err != nil {
		t.Fatalf("OpenCode activation failed: %v\n%s", err, out)
	}
	if strings.Contains(readFileString(t, filepath.Join(home, ".codex", "config.toml")), "opencode") || readFileString(t, filepath.Join(home, ".codex", "config.toml")) != codexConfig {
		t.Fatal("OpenCode activation mutated Codex configuration")
	}
	openCodeConfig := readFileString(t, filepath.Join(home, "xdg", "opencode", "opencode.json"))
	for _, want := range []string{"engram-memory.md", `"engram"`, "mcp"} {
		if !strings.Contains(openCodeConfig, want) {
			t.Fatalf("OpenCode config missing %q:\n%s", want, openCodeConfig)
		}
	}
	if terminal.calls != 3 || len(runner.calls) != 2 {
		t.Fatalf("surface approvals/calls = %d/%d", terminal.calls, len(runner.calls))
	}
	state := readFileString(t, filepath.Join(home, ".matty", "packs.json"))
	if !strings.Contains(state, `"surface": "codex"`) || !strings.Contains(state, `"surface": "opencode"`) {
		t.Fatalf("state did not preserve both surfaces:\n%s", state)
	}
}

func TestPackCompositionDryRunRendersRequestedAndRequiredWithoutPrompts(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	bundle := writeCompositionBundle(t, false)
	opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	before := snapshotTree(t, home)
	for _, surface := range []string{"codex", "opencode"} {
		out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", surface, "--dry-run")
		if err != nil {
			t.Fatalf("%s dry-run: %v\n%s", surface, err, out)
		}
		for _, want := range []string{"Activation: requested matty 1.0.0", "Activation: required engram 1.0.0"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s missing %q:\n%s", surface, want, out)
			}
		}
	}
	if terminal.calls != 0 || snapshotTree(t, home) != before {
		t.Fatal("composition dry-run prompted or mutated HOME")
	}
}

func TestPackCompositionBlockedPreviewRendersAllBlockersWithoutPromptOrEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	bundle := writeCompositionBundle(t, true)
	opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex")
	if err != nil {
		t.Fatalf("seed dependency: %v\n%s", err, out)
	}
	prompts := terminal.calls
	before := snapshotTree(t, home)
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("blocked preview: %v\n%s", err, out)
	}
	for _, want := range []string{"Cannot apply activation: 2 blockers", "capability-conflict", "dependency cap:missing"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != prompts || snapshotTree(t, home) != before {
		t.Fatal("blocked preview prompted or mutated HOME")
	}
}

func TestPackUpdateRendersVersionsAndRetainedSharedResourcesOnBothSurfaces(t *testing.T) {
	for _, surface := range []string{"codex", "opencode"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, terminal)
			bundle := writeUpdateBundle(t, "1.0.0")
			opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", surface); err != nil {
				t.Fatalf("seed activation: %v\n%s", err, out)
			}
			writeUpdateManifest(t, bundle, "2.0.0")
			before := snapshotTree(t, home)
			prompts := terminal.calls
			out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--surface", surface, "--dry-run")
			if err != nil {
				t.Fatalf("update dry-run: %v\n%s", err, out)
			}
			for _, want := range []string{"Update dry-run plan plan-", "Version: 1.0.0 -> 2.0.0 (catalog-current)", "Intent revision:", "Retained shared projection:", "engram, matty", "no rewrite"} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q:\n%s", want, out)
				}
			}
			if terminal.calls != prompts || snapshotTree(t, home) != before {
				t.Fatal("update dry-run prompted or mutated HOME")
			}
			out, err = executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--surface", surface)
			if err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("update apply: %v\n%s", err, out)
			}
			out, err = executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--surface", surface)
			if err != nil || !strings.Contains(out, "Already converged") {
				t.Fatalf("update no-op: %v\n%s", err, out)
			}
		})
	}
}

func TestPackUpdateCancellationNonTTYAndStalePlanHaveZeroEffects(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal *fakeTerminal
		stale    bool
	}{
		{name: "cancel", terminal: &fakeTerminal{interactive: true, approve: false}},
		{name: "non-tty", terminal: &fakeTerminal{interactive: false, approve: true}},
		{name: "stale", terminal: &fakeTerminal{interactive: true, approve: true}, stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, home, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
			bundle := writeUpdateBundle(t, "1.0.0")
			opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
				t.Fatalf("seed: %v\n%s", err, out)
			}
			writeUpdateManifest(t, bundle, "2.0.0")
			if err := os.WriteFile(filepath.Join(bundle, "instructions/shared.md"), []byte("shared v2\n"), 0600); err != nil {
				t.Fatal(err)
			}
			opts.Terminal = tc.terminal
			if tc.stale {
				tc.terminal.onApprove = func() { writeUpdateManifest(t, bundle, "3.0.0") }
			}
			before := snapshotTree(t, home)
			_, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--surface", "codex")
			if err == nil {
				t.Fatal("unsafe update unexpectedly succeeded")
			}
			if snapshotTree(t, home) != before {
				t.Fatalf("%s mutated HOME before safe Apply", tc.name)
			}
		})
	}
}

func TestPackUpdateRendersConsolidatedBlockersWithoutPrompts(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	bundle := writeCompositionBundle(t, false)
	opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	for _, pack := range []string{"engram", "matty"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", pack, "--surface", "codex"); err != nil {
			t.Fatalf("seed %s: %v\n%s", pack, err, out)
		}
	}
	blocked := `{"schema_version":1,"id":"matty","version":"2.0.0","provides":[],"requires":{"capabilities":["cap:missing"],"tools":[]},"conflicts":["cap:dep"],"resources":[{"kind":"instruction","id":"matty","source":"instructions/app.md"}]}`
	if err := os.WriteFile(filepath.Join(bundle, "packs/matty/pack.json"), []byte(blocked), 0600); err != nil {
		t.Fatal(err)
	}
	prompts := terminal.calls
	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("blocked update: %v\n%s", err, out)
	}
	for _, want := range []string{"Cannot apply update: 2 blockers", "capability-conflict", "dependency cap:missing"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != prompts || snapshotTree(t, home) != before {
		t.Fatal("blocked update prompted or mutated HOME")
	}
}

func TestPackUpdateKeepsOtherSurfaceIntentOwnershipAndConfigIndependent(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	bundle := writeUpdateBundle(t, "1.0.0")
	opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	for _, surface := range []string{"codex", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", surface); err != nil {
			t.Fatalf("seed %s: %v\n%s", surface, err, out)
		}
	}
	openCodeConfig := readFileString(t, filepath.Join(home, "xdg", "opencode", "opencode.json"))
	statePath := filepath.Join(home, ".matty", "packs.json")
	openCodeOwnership := ownershipForSurface(t, statePath, "opencode")
	writeUpdateManifest(t, bundle, "2.0.0")
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("Codex update: %v\n%s", err, out)
	}
	if got := readFileString(t, filepath.Join(home, "xdg", "opencode", "opencode.json")); got != openCodeConfig {
		t.Fatal("Codex update mutated OpenCode configuration")
	}
	state := readFileString(t, statePath)
	if !strings.Contains(state, `"version": "2.0.0"`) || !strings.Contains(state, `"version": "1.0.0"`) || !strings.Contains(state, `"surface": "opencode"`) {
		t.Fatalf("surface intents were not independent:\n%s", state)
	}
	if got := ownershipForSurface(t, statePath, "opencode"); got != openCodeOwnership {
		t.Fatalf("Codex update mutated OpenCode ownership:\nbefore=%s\nafter=%s", openCodeOwnership, got)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--surface", "opencode", "--dry-run"); err != nil || !strings.Contains(out, "Version: 1.0.0 -> 2.0.0") {
		t.Fatalf("OpenCode intent was unexpectedly changed: %v\n%s", err, out)
	}
}

func ownershipForSurface(t *testing.T, statePath, surface string) string {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(readFileString(t, statePath)), &document); err != nil {
		t.Fatal(err)
	}
	for _, raw := range document["activations"].([]any) {
		activation := raw.(map[string]any)
		intent := activation["intent"].(map[string]any)
		if intent["surface"] == surface {
			encoded, err := json.Marshal(activation["ownership"])
			if err != nil {
				t.Fatal(err)
			}
			return string(encoded)
		}
	}
	t.Fatalf("missing %s activation", surface)
	return ""
}

func TestPackUpdateExternalCancellationHasNoEffects(t *testing.T) {
	seedTerminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot, runner := engramActivationOptions(t, seedTerminal)
	bundle := copyPackBundleForUpdate(t, repoRoot)
	opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex"); err != nil {
		t.Fatalf("seed: %v\n%s", err, out)
	}
	manifestPath := filepath.Join(bundle, "packs", "engram", "pack.json")
	manifest := readFileString(t, manifestPath)
	manifest = strings.Replace(manifest, `"version": "1.0.0"`, `"version": "2.0.0"`, 1)
	manifest = strings.Replace(manifest, `"--tools=agent"`, `"--tools=agent,update"`, 1)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(home, ".matty", "packs.json")
	var document map[string]any
	if err := json.Unmarshal([]byte(readFileString(t, statePath)), &document); err != nil {
		t.Fatal(err)
	}
	for _, raw := range document["activations"].([]any) {
		activation := raw.(map[string]any)
		intent := activation["intent"].(map[string]any)
		if intent["surface"] == "codex" {
			delete(activation, "external_effects")
		}
	}
	encoded, _ := json.MarshalIndent(document, "", "  ")
	if err := os.WriteFile(statePath, append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	cancel := &fakeTerminal{interactive: true, answers: []bool{false}}
	opts.Terminal = cancel
	before := snapshotTree(t, home)
	calls := len(runner.calls)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "engram", "--surface", "codex")
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("cancel error=%v\n%s", err, out)
	}
	if len(cancel.prompts) != 1 || !strings.Contains(cancel.prompts[0], "executable-external") {
		t.Fatalf("prompts=%#v", cancel.prompts)
	}
	if snapshotTree(t, home) != before || len(runner.calls) != calls {
		t.Fatal("cancelled multi-phase update caused effects")
	}
}

func copyPackBundleForUpdate(t *testing.T, repoRoot string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"skills", "instructions"} {
		if err := os.CopyFS(filepath.Join(root, dir), os.DirFS(filepath.Join(repoRoot, "bundle", dir))); err != nil {
			t.Fatal(err)
		}
	}
	for _, pack := range []string{"matty", "engram"} {
		dir := filepath.Join(root, "packs", pack)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(repoRoot, "bundle", "packs", pack, "pack.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pack.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeUpdateBundle(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"skills", "packs/matty", "packs/engram", "instructions"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "instructions/shared.md"), []byte("shared\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dep := `{"schema_version":1,"id":"engram","version":"1.0.0","provides":["cap:dep"],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"instruction","id":"shared","source":"instructions/shared.md"}]}`
	if err := os.WriteFile(filepath.Join(root, "packs/engram/pack.json"), []byte(dep), 0600); err != nil {
		t.Fatal(err)
	}
	writeUpdateManifest(t, root, version)
	return root
}

func writeUpdateManifest(t *testing.T, root, version string) {
	t.Helper()
	app := `{"schema_version":1,"id":"matty","version":"` + version + `","provides":[],"requires":{"capabilities":["cap:dep"],"tools":[]},"conflicts":[],"resources":[{"kind":"instruction","id":"shared","source":"instructions/shared.md"}]}`
	if err := os.WriteFile(filepath.Join(root, "packs/matty/pack.json"), []byte(app), 0600); err != nil {
		t.Fatal(err)
	}
}

func writeCompositionBundle(t *testing.T, blocked bool) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"skills", "packs/matty", "packs/engram", "instructions"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "instructions/app.md"), []byte("app\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "instructions/dep.md"), []byte("dep\n"), 0600); err != nil {
		t.Fatal(err)
	}
	requires := "[\"cap:dep\"]"
	conflicts := "[]"
	if blocked {
		requires = "[\"cap:missing\"]"
		conflicts = "[\"cap:dep\"]"
	}
	app := `{"schema_version":1,"id":"matty","version":"1.0.0","provides":[],"requires":{"capabilities":` + requires + `,"tools":[]},"conflicts":` + conflicts + `,"resources":[{"kind":"instruction","id":"matty","source":"instructions/app.md"}]}`
	dep := `{"schema_version":1,"id":"engram","version":"1.0.0","provides":["cap:dep"],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"instruction","id":"engram","source":"instructions/dep.md"}]}`
	for path, data := range map[string]string{"packs/matty/pack.json": app, "packs/engram/pack.json": dep} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestPackDeactivateDryRunApplyAndInactiveNoOpOnBothSurfaces(t *testing.T) {
	for _, surface := range []string{"codex", "opencode"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, terminal)
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", surface); err != nil {
				t.Fatalf("seed: %v\n%s", err, out)
			}
			before := snapshotTree(t, home)
			prompts := terminal.calls
			out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", surface, "--dry-run")
			if err != nil {
				t.Fatalf("dry-run: %v\n%s", err, out)
			}
			for _, want := range []string{"Deactivation dry-run plan plan-", "Active version: 1.0.0", "Intent revision:", "Contributor removed:", "Phase: destructive-cleanup"} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q:\n%s", want, out)
				}
			}
			if terminal.calls != prompts || snapshotTree(t, home) != before {
				t.Fatal("deactivation dry-run prompted or mutated HOME")
			}
			out, err = executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", surface)
			if err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("apply: %v\n%s", err, out)
			}
			out, err = executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", surface)
			if err != nil || !strings.Contains(out, "Already converged") {
				t.Fatalf("no-op: %v\n%s", err, out)
			}
		})
	}
}

func TestPackDeactivateRequiredPackIsBlockedWithoutPromptOrCascade(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	bundle := writeCompositionBundle(t, false)
	opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	for _, pack := range []string{"engram", "matty"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", pack, "--surface", "codex"); err != nil {
			t.Fatalf("seed %s: %v\n%s", pack, err, out)
		}
	}
	before := snapshotTree(t, home)
	prompts := terminal.calls
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "engram", "--surface", "codex")
	if err != nil {
		t.Fatalf("blocked preview: %v\n%s", err, out)
	}
	for _, want := range []string{"Cannot apply deactivation", "active-dependent", "matty", "cap:dep", "no automatic cascade"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != prompts || snapshotTree(t, home) != before {
		t.Fatal("blocked deactivation prompted, mutated, or cascaded")
	}
}

func TestPackDeactivateCancellationAndNonTTYHaveZeroEffects(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal *fakeTerminal
	}{{"cancel", &fakeTerminal{interactive: true, approve: false}}, {"non-tty", &fakeTerminal{interactive: false, approve: true}}} {
		t.Run(tc.name, func(t *testing.T) {
			seed := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, seed)
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
				t.Fatalf("seed: %v\n%s", err, out)
			}
			opts.Terminal = tc.terminal
			before := snapshotTree(t, home)
			_, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", "codex")
			if err == nil {
				t.Fatal("unsafe deactivation succeeded")
			}
			if snapshotTree(t, home) != before {
				t.Fatal("cancel/non-TTY deactivation caused effects")
			}
			if tc.name == "cancel" && (len(tc.terminal.prompts) != 1 || !strings.Contains(tc.terminal.prompts[0], "destructive-cleanup")) {
				t.Fatalf("prompts=%v", tc.terminal.prompts)
			}
		})
	}
}

func TestPackDeactivateRendersRemovedAndRetainedSharedContributors(t *testing.T) {
	for _, surface := range []string{"codex", "opencode"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, terminal)
			bundle := writeUpdateBundle(t, "1.0.0")
			opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
			for _, pack := range []string{"engram", "matty"} {
				if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", pack, "--surface", surface); err != nil {
					t.Fatalf("seed %s: %v\n%s", pack, err, out)
				}
			}
			before := snapshotTree(t, home)
			prompts := terminal.calls
			out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", surface, "--dry-run")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"Contributor removed: instruction:shared <- matty", "Retained shared projection: instruction:shared <- engram (no rewrite)", "Contributors: instruction:shared <- engram"} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q:\n%s", want, out)
				}
			}
			if terminal.calls != prompts || snapshotTree(t, home) != before {
				t.Fatal("shared dry-run prompted or mutated")
			}
			if out, err = executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", surface); err != nil || !strings.Contains(out, "Retained shared projection") {
				t.Fatalf("contributor-safe Apply: %v\n%s", err, out)
			}
			paths, _ := ResolvePaths(opts.Env)
			projection := paths.CodexPromptFile
			if surface == "opencode" {
				projection = paths.OpenCodeConfigFile
			}
			if !exists(projection) || !strings.Contains(readFileString(t, projection), "shared") {
				t.Fatalf("shared projection removed with remaining contributor: %s", projection)
			}
		})
	}
}

func TestPackDeactivateKeepsOtherSurfaceIntentOwnershipAndConfigIndependent(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	for _, surface := range []string{"codex", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", surface); err != nil {
			t.Fatalf("seed %s: %v\n%s", surface, err, out)
		}
	}
	statePath := filepath.Join(home, ".matty", "packs.json")
	beforeOwnership := ownershipForSurface(t, statePath, "opencode")
	beforeConfig := readFileString(t, filepath.Join(home, "xdg", "opencode", "opencode.json"))
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("deactivate: %v\n%s", err, out)
	}
	if got := ownershipForSurface(t, statePath, "opencode"); got != beforeOwnership {
		t.Fatal("Codex deactivation mutated OpenCode ownership")
	}
	if got := readFileString(t, filepath.Join(home, "xdg", "opencode", "opencode.json")); got != beforeConfig {
		t.Fatal("Codex deactivation mutated OpenCode config")
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", "opencode", "--dry-run"); err != nil || strings.Contains(out, "Already converged") {
		t.Fatalf("OpenCode intent changed: %v\n%s", err, out)
	}
}

func TestPackReconcileTargetedAndSurfaceWideRenderSealedDesiredState(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	bundle := writeUpdateBundle(t, "1.0.0")
	opts.Env.(MapEnv)["MATTY_SKILLS_SOURCE"] = filepath.Join(bundle, "skills")
	for _, pack := range []string{"engram", "matty"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", pack, "--surface", "codex"); err != nil {
			t.Fatalf("seed %s: %v\n%s", pack, err, out)
		}
	}
	if err := os.Remove(filepath.Join(home, ".codex", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}

	before := snapshotTree(t, home)
	prompts := terminal.calls
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"targeted", []string{"pack", "reconcile", "matty", "--surface", "codex", "--dry-run"}, []string{"Reconcile dry-run plan plan-", "Scope: targeted", "Intent revision:", "Contributors: instruction:shared <- engram, matty", "Phase: reversible-local", "write instruction shared"}},
		{"surface-wide", []string{"pack", "reconcile", "--surface", "codex", "--dry-run"}, []string{"Reconcile dry-run plan plan-", "Scope: surface-wide", "Activation:", "Contributors: instruction:shared <- engram, matty", "Phase: reversible-local", "write instruction shared"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := executeCommand(t, NewRootCommand(opts), tc.args...)
			if err != nil {
				t.Fatalf("reconcile preview: %v\n%s", err, out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q:\n%s", want, out)
				}
			}
		})
	}
	if terminal.calls != prompts || snapshotTree(t, home) != before {
		t.Fatal("reconcile dry-run prompted or caused effects")
	}
}

func TestPackReconcileCancellationNonTTYAndStaleHaveZeroEffects(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal *fakeTerminal
		stale    bool
	}{
		{"cancel", &fakeTerminal{interactive: true, approve: false}, false},
		{"non-tty", &fakeTerminal{interactive: false, approve: true}, false},
		{"stale", &fakeTerminal{interactive: true, approve: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seed := &fakeTerminal{interactive: true, approve: true}
			opts, home, _ := packActivationOptions(t, seed)
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex"); err != nil {
				t.Fatalf("seed: %v\n%s", err, out)
			}
			target := filepath.Join(home, ".agents", "skills", "ask-matt")
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			opts.Terminal = tc.terminal
			if tc.stale {
				tc.terminal.onApprove = func() {
					_ = os.WriteFile(filepath.Join(home, ".codex", "AGENTS.md"), []byte("concurrent unmanaged edit\n"), 0o600)
				}
			}
			beforeState := readFileString(t, filepath.Join(home, ".matty", "packs.json"))
			out, err := executeCommand(t, NewRootCommand(opts), "pack", "reconcile", "matty", "--surface", "codex")
			if err == nil {
				t.Fatalf("unsafe reconcile succeeded:\n%s", out)
			}
			if tc.stale {
				message := strings.ToLower(err.Error())
				if !strings.Contains(message, "stale") || !strings.Contains(message, "rerun") {
					t.Fatalf("stale error must direct an explicit rerun: %v", err)
				}
				if strings.Contains(out, "replacement preview") {
					t.Fatalf("stale reconcile silently previewed a replacement:\n%s", out)
				}
			}
			if exists(target) || readFileString(t, filepath.Join(home, ".matty", "packs.json")) != beforeState {
				t.Fatal("cancel/non-TTY/stale reconcile repaired projection or changed intent state")
			}
		})
	}
}

func TestPackReconcileDriftFreeIsApprovalFreeNoOp(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "opencode"); err != nil {
		t.Fatalf("seed: %v\n%s", err, out)
	}
	before := snapshotTree(t, home)
	prompts := terminal.calls
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "reconcile", "matty", "--surface", "opencode")
	if err != nil || !strings.Contains(out, "Scope: targeted") || !strings.Contains(out, "Already converged") {
		t.Fatalf("drift-free reconcile: %v\n%s", err, out)
	}
	if terminal.calls != prompts || snapshotTree(t, home) != before {
		t.Fatal("drift-free reconcile prompted or mutated state")
	}
}

func TestPackReconcileExternalConsentStopsOnCancellation(t *testing.T) {
	seed := &fakeTerminal{interactive: true, approve: true}
	opts, home, _, runner := engramActivationOptions(t, seed)
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "engram", "--surface", "codex"); err != nil {
		t.Fatalf("seed: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(home, ".codex", "config.toml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(runner.path["engram"]); err != nil {
		t.Fatal(err)
	}
	runner.path = map[string]string{}
	terminal := &fakeTerminal{interactive: true, answers: []bool{false}}
	opts.Terminal = terminal
	beforeState := readFileString(t, filepath.Join(home, ".matty", "packs.json"))
	beforeCalls := len(runner.calls)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "reconcile", "engram", "--surface", "codex")
	if err == nil {
		t.Fatalf("cancelled reconcile succeeded:\n%s", out)
	}
	if len(terminal.prompts) != 1 || !strings.Contains(terminal.prompts[0], "executable-external") {
		t.Fatalf("external consent prompt missing: %v\n%s", terminal.prompts, out)
	}
	if exists(filepath.Join(home, ".codex", "config.toml")) || len(runner.calls) != beforeCalls || readFileString(t, filepath.Join(home, ".matty", "packs.json")) != beforeState {
		t.Fatal("cancellation before Apply caused local, external, or state effects")
	}
}
