package claudesmoke

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func validEvidence() Evidence {
	return Evidence{SchemaVersion: 1, PackyVersion: "v1", PackyRef: "v1", PackySHA: strings.Repeat("a", 40), ResolvedClaudeVersion: ExactFloor, ClaudeIntegrity: "sha512-x", ClaudeDigest: strings.Repeat("b", 64), Commands: []CommandEvidence{{Name: "packy", ExitCode: 0}}, Safety: SafetyEvidence{DisposableSandbox: true, AllowlistEnvironment: true, CredentialsScrubbed: true, CommandAllowlist: true, CheckoutUnchanged: true, NoOutsideSandboxWrites: true, NoInteractiveClaude: true}}
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
	e.Safety.NoOutsideSandboxWrites = false
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted unproved sandbox confinement")
	}
	e = validEvidence()
	e.Commands[0].Stdout = "ANTHROPIC_API_KEY"
	if err := ValidateEvidence(e); err == nil {
		t.Fatal("accepted credential marker")
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
	outer := runAllowed(context.Background(), root, env, packy, claude, []string{packy, "install"})
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
	repository, ref, resolved, err := prepareInstallableSource(context.Background(), source, sha, filepath.Join(root, "installable"))
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
