package claudesmoke

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestResolveSelector(t *testing.T) {
	for _, tc := range []struct {
		name, selector, metadata, version string
		wantErr                           bool
	}{
		{"floor", ExactFloor, `{"version":"2.1.203","dist.integrity":"sha512-floor"}`, "2.1.203", false},
		{"stable", "stable", `{"version":"2.2.0","dist.integrity":"sha512-stable"}`, "2.2.0", false},
		{"floor mismatch", ExactFloor, `{"version":"2.2.0","dist.integrity":"x"}`, "", true},
		{"forbidden", "latest", `{"version":"2.2.0","dist.integrity":"x"}`, "", true},
		{"malformed", "stable", `{`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := ResolveSelector(tc.selector, tc.metadata)
			if (err != nil) != tc.wantErr || got != tc.version {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestAllowedCommandRejectsInteractiveClaudeAndUnknownPacky(t *testing.T) {
	p, c := "/x/packy", "/x/claude"
	if !AllowedCommand(p, c, []string{c, "--version"}) {
		t.Fatal("version rejected")
	}
	for _, argv := range [][]string{{c}, {c, "--print", "hello"}, {c, "mcp", "list"}, {p, "pack", "list"}, {p, "doctor", "--json"}, {"sh", "-c", "true"}} {
		if AllowedCommand(p, c, argv) {
			t.Fatalf("allowed %#v", argv)
		}
	}
}

func TestRestrictedEnvIsAllowlistAndScrubsCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "super-secret")
	env := RestrictedEnv("/sandbox", "/sandbox/npm/bin")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "super-secret") || strings.Contains(joined, "ANTHROPIC") {
		t.Fatal("credential leaked")
	}
	for _, key := range []string{"HOME=/sandbox/home", "CLAUDE_CONFIG_DIR=/sandbox/home", "TMPDIR=/sandbox/tmp"} {
		if !strings.Contains(joined, key) {
			t.Fatalf("missing %s", key)
		}
	}
	for _, key := range []string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTOUPDATER=1"} {
		if !strings.Contains(joined, key) {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestAcquisitionEnvUsesOnlyDisposableNPMState(t *testing.T) {
	t.Setenv("NPM_TOKEN", "operator-secret")
	env := strings.Join(acquisitionEnv("/sandbox", "/runtime/bin/npm"), "\n")
	for _, want := range []string{"HOME=/sandbox/acquisition/home", "XDG_CONFIG_HOME=/sandbox/acquisition/config", "NPM_CONFIG_CACHE=/sandbox/acquisition/cache", "NPM_CONFIG_USERCONFIG=/sandbox/acquisition/npmrc"} {
		if !strings.Contains(env, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(env, "operator-secret") || strings.Contains(env, "NPM_TOKEN") {
		t.Fatal("acquisition inherited npm credential")
	}
}

func TestAcquisitionCommandCannotObserveCallerNPMRC(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	caller := filepath.Join(root, "caller")
	acquisition := filepath.Join(root, "acquisition", "home")
	for _, dir := range []string{caller, acquisition, filepath.Join(root, "acquisition", "config"), filepath.Join(root, "acquisition", "cache"), filepath.Join(root, "acquisition", "tmp")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(caller, ".npmrc"), []byte("//registry/:_authToken=CALLER_SECRET"), 0600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(root, "fake-npm")
	if err := writeStub(fake, "#!/bin/sh\npwd\n[ ! -e .npmrc ] || cat .npmrc\n"); err != nil {
		t.Fatal(err)
	}
	out, err := sandboxOutput(context.Background(), root, acquisition, acquisitionEnv(root, fake), fake, "view")
	if err != nil {
		t.Fatal(err)
	}
	realAcquisition, err := filepath.EvalSymlinks(acquisition)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "CALLER_SECRET") || strings.TrimSpace(out) != realAcquisition {
		t.Fatalf("caller npmrc observed: %q", out)
	}
}

func TestSandboxBoundaryAllowsOnlyConfiguredRootWrites(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec is macOS-specific")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	inside := filepath.Join(root, "inside")
	cmd, err := sandboxCommand(context.Background(), root, "/usr/bin/touch", inside)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("inside write: %v: %s", err, out)
	}
	cmd, err = sandboxCommand(context.Background(), root, "/usr/bin/touch", outside)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Run(); err == nil {
		t.Fatal("outside write escaped sandbox")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("outside file was created")
	}
}

func TestManifestDeterministicAndContentBound(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "b"), []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	one, err := Manifest(root)
	if err != nil {
		t.Fatal(err)
	}
	two, err := Manifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, two) || one[0].Path != "a" || one[0].SHA256 == "" {
		t.Fatalf("non-deterministic: %#v %#v", one, two)
	}
}

func TestDirectoryAbsentOrEmptyFailsClosed(t *testing.T) {
	root := t.TempDir()
	empty, err := directoryAbsentOrEmpty(filepath.Join(root, "missing"))
	if err != nil || !empty {
		t.Fatalf("missing: %v %v", empty, err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := directoryAbsentOrEmpty(file); err == nil {
		t.Fatal("non-directory was treated as empty")
	}
}

func TestClassicSkillTopologyUsesRecursiveSkillLeaves(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "bundle", "skills")
	home := filepath.Join(root, "home")
	for _, rel := range []string{"engineering/review", "productivity/plan", "in-progress/loop-me"} {
		dir := filepath.Join(source, rel)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	links := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(links, 0700); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"review": filepath.Join(source, "engineering", "review"), "plan": filepath.Join(source, "productivity", "plan"), "loop-me": filepath.Join(source, "in-progress", "loop-me")} {
		if err := os.Symlink(target, filepath.Join(links, name)); err != nil {
			t.Fatal(err)
		}
	}
	if !classicSkillTopologyExact(home, source) {
		t.Fatal("recursive leaf topology rejected")
	}
}

func TestValidationFailureStillWritesDiagnosticEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.json")
	evidence := validEvidence()
	evidence.Assertions.DryRunsUnchanged = false
	if err := validateAndWriteEvidence(path, evidence); err == nil {
		t.Fatal("invalid evidence accepted")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"dry_runs_unchanged": false`)) {
		t.Fatalf("failed diagnostic evidence missing assertion: %s", b)
	}
}

func validEvidence() Evidence {
	args := [][]string{{"--version"}, {"version"}, {"init", "--home", "h", "--source-root", "s", "--repository-url", "r", "--repository-ref", "ref"}, {"install", "--dry-run"}, {"install"}, {"doctor"}, {"update", "--dry-run"}, {"update"}, {"uninstall", "--dry-run"}, {"uninstall"}, {"doctor"}}
	commands := make([]CommandEvidence, len(args))
	for i := range args {
		commands[i] = CommandEvidence{Name: "packy", Args: args[i], ExitCode: 0}
	}
	return Evidence{SchemaVersion: 1, PackyVersion: "v1", PackyRef: "v1", PackySHA: strings.Repeat("a", 40), RequestedClaudeVersion: ExactFloor, ResolvedClaudeVersion: ExactFloor, ClaudeIntegrity: "sha512-x", ClaudeDigest: strings.Repeat("b", 64), Commands: commands, Safety: SafetyEvidence{DisposableSandbox: true, AllowlistEnvironment: true, CredentialsScrubbed: true, CommandAllowlist: true, CheckoutUnchanged: true, ConfiguredWritableRootsConfined: true, EvidencePathOutsideSandbox: true, NoInteractiveClaude: true, WriteBoundaryEnforced: true}, Assertions: AssertionEvidence{ForeignContentPreserved: true, InstallCreatedManagedState: true, InstallCreatedManagedProjections: true, InstallProjectedClaudeMCP: true, DryRunsUnchanged: true, UninstallRemovedManagedState: true, UninstallRemovedManagedProjections: true, ResidualManagedArtifactsAbsent: true, EngramStubProtocolVerified: true, SensitiveFixtureRedacted: true, ForeignMCPExactAfterInstall: true, ForeignMCPExactAfterUpdate: true, ForeignMCPExactAfterUninstall: true}}
}
func TestValidateEvidenceRejectsTampering(t *testing.T) {
	e := validEvidence()
	if err := ValidateEvidence(e); err != nil {
		t.Fatal(err)
	}
	e.Commands[0].ExitCode = 1
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted failed command")
	}
	e = validEvidence()
	e.Safety.CheckoutUnchanged = false
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted checkout mutation")
	}
	e = validEvidence()
	e.Safety.ConfiguredWritableRootsConfined = false
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted unproved sandbox confinement")
	}
	e = validEvidence()
	e.Commands[0].Stdout = "ANTHROPIC_API_KEY"
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted credential marker")
	}
	e = validEvidence()
	e.Commands[4].Args = []string{"install", "--dry-run"}
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted tampered lifecycle sequence")
	}
	e = validEvidence()
	e.Commands = append(e.Commands, CommandEvidence{Name: "claude", Args: []string{"login"}, ExitCode: 0})
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted unsafe normalized Claude command")
	}
	e = validEvidence()
	e.Assertions.InstallCreatedManagedProjections = false
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted no-op lifecycle assertions")
	}
}

func TestClaudeInterposerRecordsSafeNestedCommands(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "reached")
	real := filepath.Join(root, "real-claude")
	if err := writeStub(real, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+marker+"\nexit 0\n"); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "log")
	wrapper := filepath.Join(root, "claude")
	if err := createClaudeInterposer(wrapper, real, log); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--version"}, {"mcp", "list"}, {"mcp", "get", "engram"}, {"mcp", "add", "engram", "--scope", "user", "--", "engram", "mcp"}, {"mcp", "remove", "engram", "--scope", "user"}} {
		if out, err := exec.CommandContext(context.Background(), wrapper, args...).CombinedOutput(); err != nil {
			t.Fatalf("safe %v: %v: %s", args, err, out)
		}
	}
	got := readClaudeInvocations(log)
	if len(got) != 5 {
		t.Fatalf("nested evidence = %#v", got)
	}
	for _, command := range got {
		if command.Name != "claude" || command.ExitCode != 0 || len(command.Args) != 1 {
			t.Fatalf("unsafe evidence detail: %#v", command)
		}
	}
}

func TestClaudeInterposerBlocksForbiddenShapesBeforeRealBinary(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "reached")
	real := filepath.Join(root, "real-claude")
	if err := writeStub(real, "#!/bin/sh\ntouch "+marker+"\n"); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "claude")
	if err := createClaudeInterposer(wrapper, real, filepath.Join(root, "log")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"--print", "hello"}, {"login"}, {"auth"}, {"model", "opus"}, {"mcp", "add", "x", "--scope", "project", "--", "engram"}, {"mcp", "remove", "x"}, {"mcp", "list", "extra"}} {
		if err := exec.Command(wrapper, args...).Run(); err == nil {
			t.Fatalf("forbidden shape succeeded: %v", args)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("forbidden shape reached real binary: %v", args)
		}
	}
}

func TestPackyTriggeredClaudeInvocationIsRecorded(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real-claude")
	if err := writeStub(real, "#!/bin/sh\nexit 0\n"); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "claude.log")
	stubBin := filepath.Join(root, "stub-bin")
	if err := os.Mkdir(stubBin, 0700); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(stubBin, "claude")
	if err := createClaudeInterposer(claude, real, log); err != nil {
		t.Fatal(err)
	}
	packy := filepath.Join(root, "packy")
	if err := writeStub(packy, "#!/bin/sh\nclaude mcp list\n"); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + stubBin + ":/usr/bin:/bin"}
	outer := runAllowed(context.Background(), root, root, env, packy, claude, []string{packy, "install"})
	if outer.ExitCode != 0 {
		t.Fatalf("fake Packy failed: %#v", outer)
	}
	nested := readClaudeInvocations(log)
	if len(nested) != 1 || !reflect.DeepEqual(nested[0].Args, []string{"mcp-list"}) || nested[0].ExitCode != 0 {
		t.Fatalf("Packy-triggered evidence = %#v", nested)
	}
}

func TestPrepareInstallableSourceAdaptsFullSHAWithoutMutatingCheckout(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	runGit(source, "init")
	runGit(source, "config", "user.name", "Smoke Test")
	runGit(source, "config", "user.email", "smoke@example.invalid")
	if err := os.WriteFile(filepath.Join(source, "README"), []byte("proved\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(source, "add", "README")
	runGit(source, "commit", "-m", "proved source")
	sha := runGit(source, "rev-parse", "HEAD")
	statusBefore := runGit(source, "status", "--porcelain=v1", "--untracked-files=all")
	if err := os.Mkdir(filepath.Join(root, "work"), 0700); err != nil {
		t.Fatal(err)
	}
	repository, ref, resolved, err := prepareInstallableSource(context.Background(), root, acquisitionEnv(root, "/usr/bin/npm"), source, sha, filepath.Join(root, "installable"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved != sha || ref == sha {
		t.Fatalf("repository=%q ref=%q sha=%q", repository, ref, resolved)
	}
	if got := runGit(repository, "rev-parse", ref+"^{commit}"); got != sha {
		t.Fatalf("synthetic ref = %q, want %q", got, sha)
	}
	if got := runGit(source, "status", "--porcelain=v1", "--untracked-files=all"); got != statusBefore {
		t.Fatalf("source checkout changed: before %q after %q", statusBefore, got)
	}
	if got := runGit(source, "rev-parse", "HEAD"); got != sha {
		t.Fatalf("source HEAD changed to %q", got)
	}
}
