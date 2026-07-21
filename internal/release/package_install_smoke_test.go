package release_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPackageInstallSmokeLifecycleWithLocalReleaseBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and executes a local release binary")
	}

	root := repoRoot(t)
	assertSourceRepositoryExcludesExternalReferenceTrees(t, root)
	sandbox := t.TempDir()
	home := filepath.Join(sandbox, "home")
	xdgConfigHome := filepath.Join(sandbox, "xdg")
	outsideCheckout := filepath.Join(sandbox, "outside-checkout")
	stubBin := filepath.Join(sandbox, "bin")
	homebrewPrefix := sandbox
	externalLog := filepath.Join(sandbox, "external-calls.log")
	for _, dir := range []string{home, xdgConfigHome, outsideCheckout, stubBin} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	binary := buildLocalReleaseBinary(t, root, sandbox, "v0.99.0")
	sourceRepo := createSmokeSourceRepo(t, sandbox, "v0.98.0")
	appendSmokeSourceTag(t, sourceRepo, sandbox, "v0.99.0")
	writeSmokeStub(t, stubBin, "engram", externalLog)
	writeSmokeStub(t, stubBin, "brew", externalLog)

	env := append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+xdgConfigHome,
		"PATH="+stubBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOMEBREW_PREFIX="+homebrewPrefix,
		"GIT_CONFIG_NOSYSTEM=1",
	)
	help := runSmokeCommand(t, binary, outsideCheckout, env, "--help")
	for _, want := range []string{"Install and configure the Packy AI coding workflow", "version", "init", "install", "doctor", "update", "uninstall"} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help output missing %q:\n%s", want, help)
		}
	}
	versionFlag := runSmokeCommand(t, binary, outsideCheckout, env, "--version")
	versionCommand := runSmokeCommand(t, binary, outsideCheckout, env, "version")
	if want := "packy version v0.99.0\n"; versionFlag != want || versionCommand != want {
		t.Fatalf("version outputs = flag %q, command %q; want %q", versionFlag, versionCommand, want)
	}
	assertSmokeExternalCalls(t, externalLog, nil)

	runSmokeCommand(t, binary, outsideCheckout, env, "init", "--repository-url", sourceRepo, "--repository-ref", "v0.98.0")
	assertInitializedSourceExcludesExternalReferenceTrees(t, home)
	beforeStaleUpdateDryRun := snapshotSmokeTree(t, home)
	out, err := runSmokeCommandAllowError(t, binary, outsideCheckout, env, "update", "--dry-run")
	if err == nil {
		t.Fatalf("stale update --dry-run unexpectedly succeeded:\n%s", out)
	}
	for _, want := range []string{"stale", "v0.99.0", "run packy init"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stale update --dry-run output missing %q:\n%s", want, out)
		}
	}
	if after := snapshotSmokeTree(t, home); after != beforeStaleUpdateDryRun {
		t.Fatalf("stale update --dry-run mutated sandbox home\nbefore:\n%s\nafter:\n%s", beforeStaleUpdateDryRun, after)
	}
	assertSmokeExternalCalls(t, externalLog, nil)

	runSmokeCommand(t, binary, outsideCheckout, env, "init", "--repository-url", sourceRepo)

	runSmokeCommand(t, binary, outsideCheckout, env, "install", "--dry-run")
	assertSmokePathMissing(t, filepath.Join(home, ".packy"), "install --dry-run must not write state")
	assertSmokePathMissing(t, filepath.Join(home, ".agents"), "install --dry-run must not write skill links")
	assertSmokeExternalCalls(t, externalLog, nil)

	runSmokeCommand(t, binary, outsideCheckout, env, "install")
	assertSmokePathExists(t, filepath.Join(home, ".packy", "config.json"), "install should write state")
	assertSmokePathExists(t, filepath.Join(home, ".agents", "skills", "wayfinder"), "install should link bundled skills")
	beforeRepeatInstall := snapshotSmokeTreeIgnoringInstallObservations(t, home)
	beforeInstallCheck := readSmokeInstallCheck(t, home)
	runSmokeCommand(t, binary, outsideCheckout, env, "install")
	afterInstallCheck := readSmokeInstallCheck(t, home)
	if afterInstallCheck.Before(beforeInstallCheck) {
		t.Fatalf("repeat install observation regressed from %s to %s", beforeInstallCheck, afterInstallCheck)
	}
	if after := snapshotSmokeTreeIgnoringInstallObservations(t, home); after != beforeRepeatInstall {
		t.Fatalf("repeat install changed content other than install observations\nbefore:\n%s\nafter:\n%s", beforeRepeatInstall, after)
	}
	doctorJSON := runSmokeCommand(t, binary, outsideCheckout, env, "doctor", "--json")
	var doctor struct {
		SchemaVersion int               `json:"schema_version"`
		Report        string            `json:"report"`
		Checks        []json.RawMessage `json:"checks"`
		Summary       struct {
			Status string `json:"status"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(doctorJSON), &doctor); err != nil {
		t.Fatalf("doctor --json emitted invalid JSON: %v\n%s", err, doctorJSON)
	}
	if doctor.SchemaVersion != 1 || doctor.Report != "doctor" || len(doctor.Checks) == 0 || doctor.Summary.Status == "" {
		t.Fatalf("doctor --json emitted unexpected shape: %#v", doctor)
	}

	beforeUpdateDryRun := snapshotSmokeTree(t, home)
	runSmokeCommand(t, binary, outsideCheckout, env, "update", "--dry-run")
	if after := snapshotSmokeTree(t, home); after != beforeUpdateDryRun {
		t.Fatalf("update --dry-run mutated sandbox home\nbefore:\n%s\nafter:\n%s", beforeUpdateDryRun, after)
	}
	runSmokeCommand(t, binary, outsideCheckout, env, "update")

	beforeUninstallDryRun := snapshotSmokeTree(t, home)
	runSmokeCommand(t, binary, outsideCheckout, env, "uninstall", "--dry-run")
	if after := snapshotSmokeTree(t, home); after != beforeUninstallDryRun {
		t.Fatalf("uninstall --dry-run mutated sandbox home\nbefore:\n%s\nafter:\n%s", beforeUninstallDryRun, after)
	}
	runSmokeCommand(t, binary, outsideCheckout, env, "uninstall")
	assertSmokePathMissing(t, filepath.Join(home, ".packy", "config.json"), "uninstall should remove state")
	assertSmokePathMissing(t, filepath.Join(home, ".agents", "skills", "wayfinder"), "uninstall should remove managed skill links")
	afterUninstall := snapshotSmokeTree(t, home)
	runSmokeCommand(t, binary, outsideCheckout, env, "uninstall")
	if after := snapshotSmokeTree(t, home); after != afterUninstall {
		t.Fatalf("repeat uninstall mutated sandbox home\nbefore:\n%s\nafter:\n%s", afterUninstall, after)
	}

	runSmokeCommand(t, binary, outsideCheckout, env, "doctor")
	assertSmokePathExists(t, filepath.Join(home, ".local", "share", "packy", "bundle", "skills"), "uninstall should keep initialized source")
	assertSmokeExternalCalls(t, externalLog, []string{
		"engram setup codex",
		"engram setup opencode",
		"engram setup codex",
		"engram setup opencode",
		"engram --version",
		"brew update",
		"brew upgrade engram",
		"engram setup codex",
		"engram setup opencode",
		"engram --version",
	})
}

func buildLocalReleaseBinary(t *testing.T, root, sandbox, version string) string {
	t.Helper()
	binary := filepath.Join(sandbox, "packy")
	cmd := exec.Command("go", "build", "-ldflags", "-X github.com/yersonargotev/packy/internal/version.Value="+version, "-o", binary, "./cmd/packy")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+filepath.Join(sandbox, "go-home"),
		"XDG_CONFIG_HOME="+filepath.Join(sandbox, "go-xdg"),
		"GOCACHE="+filepath.Join(sandbox, "go-cache"),
		"GOMODCACHE="+goEnv(t, "GOMODCACHE"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build local release binary: %v\n%s", err, output)
	}
	return binary
}

func createSmokeSourceRepo(t *testing.T, sandbox, version string) string {
	t.Helper()
	repo := filepath.Join(sandbox, "source-repo")
	for _, rel := range []string{
		"bundle/skills/engineering/ask-matt/SKILL.md",
		"bundle/skills/engineering/codebase-design/SKILL.md",
		"bundle/skills/productivity/grilling/SKILL.md",
		"bundle/skills/productivity/handoff/SKILL.md",
		"bundle/skills/in-progress/loop-me/SKILL.md",
		"bundle/skills/engineering/wayfinder/SKILL.md",
	} {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir source repo fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte("---\nname: fixture\n---\n"), 0o600); err != nil {
			t.Fatalf("write source repo fixture: %v", err)
		}
	}
	runSmokeGit(t, repo, sandbox, "init")
	runSmokeGit(t, repo, sandbox, "add", ".")
	runSmokeGit(t, repo, sandbox, "-c", "user.name=Packy Smoke", "-c", "user.email=packy-smoke@example.test", "commit", "-m", "fixture source")
	runSmokeGit(t, repo, sandbox, "tag", version)
	return repo
}

func appendSmokeSourceTag(t *testing.T, repo, sandbox, version string) {
	t.Helper()
	path := filepath.Join(repo, "bundle", "skills", "engineering", "ask-matt", "CHANGELOG.md")
	if err := os.WriteFile(path, []byte(version+" fixture\n"), 0o600); err != nil {
		t.Fatalf("write newer smoke source fixture: %v", err)
	}
	runSmokeGit(t, repo, sandbox, "add", ".")
	runSmokeGit(t, repo, sandbox, "-c", "user.name=Packy Smoke", "-c", "user.email=packy-smoke@example.test", "commit", "-m", "fixture source "+version)
	runSmokeGit(t, repo, sandbox, "tag", version)
}

func runSmokeGit(t *testing.T, repo, sandbox string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(),
		"HOME="+filepath.Join(sandbox, "git-home"),
		"XDG_CONFIG_HOME="+filepath.Join(sandbox, "git-xdg"),
		"GIT_CONFIG_NOSYSTEM=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func writeSmokeStub(t *testing.T, dir, name, logPath string) {
	t.Helper()
	script := "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> " + shellQuote(logPath) + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write %s stub: %v", name, err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func runSmokeCommand(t *testing.T, binary, dir string, env []string, args ...string) string {
	t.Helper()
	output, err := runSmokeCommandAllowError(t, binary, dir, env, args...)
	if err != nil {
		t.Fatalf("packy %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func runSmokeCommandAllowError(t *testing.T, binary, dir string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func assertSourceRepositoryExcludesExternalReferenceTrees(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"engram", "gentle-ai", "skills"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("Packy source must not track external reference tree %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect external reference tree %s: %v", name, err)
		}
	}
}

func assertInitializedSourceExcludesExternalReferenceTrees(t *testing.T, home string) {
	t.Helper()
	root := filepath.Join(home, ".local", "share", "packy")
	for _, name := range []string{"engram", "gentle-ai", "skills"} {
		assertSmokePathMissing(t, filepath.Join(root, name), "initialized source must exclude external reference tree "+name)
	}
}

func assertSmokeExternalCalls(t *testing.T, logPath string, want []string) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if len(want) == 0 {
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Fatalf("read external call log: %v", err)
		}
		if strings.TrimSpace(string(data)) != "" {
			t.Fatalf("expected no external calls, got:\n%s", data)
		}
		return
	}
	if err != nil {
		t.Fatalf("read external call log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("external calls mismatch\nwant:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
}

func assertSmokePathExists(t *testing.T, path, reason string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s: %s: %v", reason, path, err)
	}
}

func assertSmokePathMissing(t *testing.T, path, reason string) {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		t.Fatalf("%s: %s exists", reason, path)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("%s: stat %s: %v", reason, path, err)
	}
}

func snapshotSmokeTree(t *testing.T, root string) string {
	return snapshotSmokeTreeWithInstallObservations(t, root, true)
}

func snapshotSmokeTreeIgnoringInstallObservations(t *testing.T, root string) string {
	return snapshotSmokeTreeWithInstallObservations(t, root, false)
}

func snapshotSmokeTreeWithInstallObservations(t *testing.T, root string, includeInstallObservations bool) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries = append(entries, rel+" symlink "+target)
		case entry.IsDir():
			entries = append(entries, rel+" dir")
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !includeInstallObservations && rel == filepath.Join(".packy", "config.json") {
				var state map[string]json.RawMessage
				if err := json.Unmarshal(data, &state); err != nil {
					return err
				}
				delete(state, "last_install_check")
				delete(state, "latest_attempt")
				data, err = json.Marshal(state)
				if err != nil {
					return err
				}
			}
			entries = append(entries, rel+" file "+string(data))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return strings.Join(entries, "\n")
}

func readSmokeInstallCheck(t *testing.T, home string) time.Time {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".packy", "config.json"))
	if err != nil {
		t.Fatalf("read Packy state: %v", err)
	}
	var state struct {
		LastInstallCheck string `json:"last_install_check"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode Packy state: %v", err)
	}
	observedAt, err := time.Parse(time.RFC3339, state.LastInstallCheck)
	if err != nil {
		t.Fatalf("parse last_install_check %q: %v", state.LastInstallCheck, err)
	}
	return observedAt
}
