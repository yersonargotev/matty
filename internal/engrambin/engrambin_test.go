package engrambin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestObserveSetupOwnsExecutableTopologyAndIndependentFacts(t *testing.T) {
	home := t.TempDir()
	prefix := filepath.Join(home, "homebrew")
	canonical := writeVersionExecutableAt(t, filepath.Join(prefix, "bin", "engram"), "printf 'engram version 1.19.0\\n'")
	local := writeVersionExecutableAt(t, filepath.Join(home, "bin", "engram"), "printf 'engram version 1.18.0\\n'")
	lookups := 0
	processErr := errors.New("process inspection failed")
	observation := ObserveSetup(
		NewSetupLayout(home, prefix),
		strings.Join([]string{filepath.Dir(local), filepath.Dir(canonical)}, string(os.PathListSeparator)),
		func(string) (string, error) { lookups++; return local, nil },
		Facts{
			Version: func(path string) (string, error) {
				if path == local {
					return "1.18.0", nil
				}
				return "1.19.0", nil
			},
			ServeProcesses: func() ([]Process, error) { return nil, processErr },
		},
	)

	if lookups != 2 || observation.LookupErr() != nil || observation.ProcessErr() != processErr {
		t.Fatalf("lookups=%d lookupErr=%v processErr=%v", lookups, observation.LookupErr(), observation.ProcessErr())
	}
	executables := observation.Executables()
	if len(executables) != 2 || executables[0].Path != local || !executables[1].Canonical {
		t.Fatalf("executables = %#v", executables)
	}
	if observation.ExpectedPath() != canonical || observation.Canonical() == nil || observation.PathExecutable() == nil {
		t.Fatalf("observation = %#v", observation)
	}
	if len(observation.LocalBinDiagnoses()) != 1 {
		t.Fatalf("local bin diagnoses = %#v", observation.LocalBinDiagnoses())
	}
}

func TestDiagnoseExecutableSetOwnsMismatchAndHomebrewShadowing(t *testing.T) {
	canonical := NewCanonical("/homebrew/bin/engram")
	executables := []Executable{
		NewExecutable("/custom/bin/engram", canonical, "1.18.0", nil),
		NewExecutable(canonical.Path, canonical, "1.19.0", nil),
	}

	mismatch := DiagnoseVersionMismatch(executables)
	shadowing := DiagnoseHomebrewShadowing(executables)

	if mismatch == nil || !strings.Contains(mismatch.Detail, "different versions") || shadowing == nil || !strings.Contains(shadowing.Detail, "/custom/bin/engram appears before Homebrew Engram") {
		t.Fatalf("mismatch=%#v shadowing=%#v", mismatch, shadowing)
	}
}

func TestVersionWithContextReportsExecutableVersion(t *testing.T) {
	path := writeVersionExecutable(t, "printf 'engram version 1.19.0\\n'")

	version, err := versionWithContext(context.Background(), path)

	if err != nil || version != "1.19.0" {
		t.Fatalf("Version() = %q, %v; want 1.19.0, nil", version, err)
	}
}

func TestVersionProductionTimeoutCancelsBlockedExecution(t *testing.T) {
	started := make(chan struct{})
	_, err := version("unused", func(ctx context.Context, _ string) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	<-started
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Version() error = %v, want production deadline", err)
	}
}

func TestVersionWithContextReportsExecutableFailure(t *testing.T) {
	path := writeVersionExecutable(t, "exit 23")

	if _, err := versionWithContext(context.Background(), path); err == nil {
		t.Fatal("Version() error = nil, want command failure")
	}
}

func TestVersionWithContextCancelsBlockedExecutable(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	pidFile := filepath.Join(dir, "pid")
	if err := syscall.Mkfifo(ready, 0o600); err != nil {
		t.Fatalf("create ready fifo: %v", err)
	}
	fifo, err := os.OpenFile(ready, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open ready fifo: %v", err)
	}
	defer fifo.Close()
	path := writeVersionExecutable(t, `printf '%s' "$$" > "$PID_FILE"; printf x > "$READY"; exec sleep 60`)
	t.Setenv("READY", ready)
	t.Setenv("PID_FILE", pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := versionWithContext(ctx, path)
		done <- err
	}()

	readyResult := make(chan error, 1)
	go func() {
		var signal [1]byte
		_, err := fifo.Read(signal[:])
		readyResult <- err
	}()
	select {
	case err := <-readyResult:
		if err != nil {
			t.Fatalf("wait for blocked executable: %v", err)
		}
	case <-time.After(5 * time.Second):
		cancel()
		_, _ = fifo.Write([]byte{'x'})
		t.Fatal("blocked executable did not signal readiness")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("Version() error = %v, context error = %v; want canceled blocked command", err, ctx.Err())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked executable did not exit after cancellation")
	}
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read blocked executable pid: %v", err)
	}
	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		t.Fatalf("parse blocked executable pid: %v", err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("blocked executable pid %d still exists: %v", pid, err)
	}
}

func TestFindServeProcessesUsesCommandOutput(t *testing.T) {
	processes, err := findServeProcesses(context.Background(), func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "ps" || strings.Join(args, " ") != "-axo pid=,args=" {
			t.Fatalf("command = %s %v", name, args)
		}
		return []byte("42 /tmp/engram serve\n"), nil
	}, func(int) string { return "/tmp/engram" })
	if err != nil || len(processes) != 1 || processes[0].PID != 42 {
		t.Fatalf("FindServeProcesses() = %#v, %v", processes, err)
	}
}

func TestFindServeProcessesPreservesInspectionError(t *testing.T) {
	want := errors.New("ps unavailable")
	_, err := findServeProcesses(context.Background(), func(context.Context, string, ...string) ([]byte, error) {
		return nil, want
	}, func(int) string { return "" })
	if !errors.Is(err, want) {
		t.Fatalf("FindServeProcesses() error = %v, want %v", err, want)
	}
}

func TestFindServeProcessesReportsNoProcesses(t *testing.T) {
	processes, err := findServeProcesses(context.Background(), func(context.Context, string, ...string) ([]byte, error) {
		return []byte("42 /tmp/engram setup codex\n"), nil
	}, func(int) string { return "" })
	if err != nil || len(processes) != 0 {
		t.Fatalf("FindServeProcesses() = %#v, %v; want no processes", processes, err)
	}
}

func writeVersionExecutable(t *testing.T, body string) string {
	t.Helper()
	return writeVersionExecutableAt(t, filepath.Join(t.TempDir(), "engram"), body)
}

func writeVersionExecutableAt(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create executable directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func TestResolverUsesHomebrewIdentityWithoutExecutingEngram(t *testing.T) {
	prefix := t.TempDir()
	path := filepath.Join(prefix, "bin", "engram")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not executed"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(prefix, func(string) (string, error) {
		return path, nil
	})
	resolution, err := resolver.Resolve(context.Background(), "engram")
	if err != nil {
		t.Fatal(err)
	}
	if !resolution.Available || resolution.Path != path || resolution.Origin != "homebrew" || resolution.Precondition == "" {
		t.Fatalf("resolution = %+v", resolution)
	}
}

func TestResolverReportsSupportedHomebrewAcquisitionWhenMissing(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "homebrew")
	resolver := NewResolver(prefix, func(string) (string, error) { return "", os.ErrNotExist })
	resolution, err := resolver.Resolve(context.Background(), "engram")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Available || !resolution.AcquisitionSupported || resolution.AcquisitionCommand != "brew" || !strings.Contains(strings.Join(resolution.AcquisitionArgs, " "), Formula) {
		t.Fatalf("missing resolution = %+v", resolution)
	}
	if resolution.Path != filepath.Join(prefix, "bin", "engram") {
		t.Fatalf("missing path = %q", resolution.Path)
	}
}

func TestDiscoverHomebrewUsesOnlyExplicitPrefixAndFallsBackAcrossCandidates(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	if err := os.MkdirAll(filepath.Join(first, "bin", "engram"), 0o700); err != nil {
		t.Fatalf("mkdir directory candidate: %v", err)
	}
	path := filepath.Join(second, "bin", "engram")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir executable candidate: %v", err)
	}
	if err := os.WriteFile(path, []byte("not executed"), 0o700); err != nil {
		t.Fatalf("write executable candidate: %v", err)
	}

	canonical := DiscoverHomebrewFromPrefixes([]string{first, second})
	if canonical == nil || canonical.Path != path {
		t.Fatalf("DiscoverHomebrewFromPrefixes() = %#v, want %s", canonical, path)
	}
	if got := HomebrewPrefixes(first); len(got) != 1 || got[0] != first {
		t.Fatalf("HomebrewPrefixes(explicit) = %#v, want only %s", got, first)
	}
}

func TestHomebrewPrefixesPreserveProductionFallbackOrder(t *testing.T) {
	if got, want := HomebrewPrefixes(""), []string{"/opt/homebrew", "/usr/local"}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("HomebrewPrefixes(defaults) = %#v, want %#v", got, want)
	}
}

func TestDiscoverHomebrewRejectsMissingNonExecutableAndDirectoryCandidates(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	nonExecutable := filepath.Join(t.TempDir(), "non-executable")
	directory := filepath.Join(t.TempDir(), "directory")
	for _, prefix := range []string{nonExecutable, directory} {
		if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o700); err != nil {
			t.Fatalf("mkdir candidate bin: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(nonExecutable, "bin", "engram"), []byte("not executable"), 0o600); err != nil {
		t.Fatalf("write non-executable candidate: %v", err)
	}
	if err := os.Mkdir(filepath.Join(directory, "bin", "engram"), 0o700); err != nil {
		t.Fatalf("mkdir directory candidate: %v", err)
	}

	if canonical := DiscoverHomebrewFromPrefixes([]string{missing, nonExecutable, directory}); canonical != nil {
		t.Fatalf("DiscoverHomebrewFromPrefixes() = %#v, want nil", canonical)
	}
}

func TestUniquePathsObservesEachExecutableIdentityOnce(t *testing.T) {
	canonical := writeVersionExecutable(t, "printf 'engram version 1.19.0\\n'")
	linkDir := t.TempDir()
	link := filepath.Join(linkDir, "engram")
	if err := os.Symlink(canonical, link); err != nil {
		t.Fatalf("symlink canonical: %v", err)
	}

	paths := UniquePaths(link, strings.Join([]string{linkDir, filepath.Dir(canonical)}, string(os.PathListSeparator)), nil)
	if len(paths) != 1 || paths[0] != link {
		t.Fatalf("UniquePaths() = %#v, want one path for canonical identity via %s", paths, link)
	}
}

func TestParseServeProcessesIgnoresNonServeEngramCommands(t *testing.T) {
	processes := ParseServeProcessesWithResolver("42 /tmp/engram setup serve codex\n43 /tmp/engram serve\n", func(int) string { return "" })
	if len(processes) != 1 || processes[0].PID != 43 {
		t.Fatalf("ParseServeProcessesWithResolver() = %#v, want only direct serve command", processes)
	}
}

func TestResolverDoesNotTreatNonHomebrewPathAsOwned(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "homebrew")
	other := writeVersionExecutable(t, "printf 'engram version 1.19.0\\n'")
	resolution, err := NewResolver(prefix, func(string) (string, error) { return other, nil }).Resolve(context.Background(), "engram")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Available || resolution.Origin != "homebrew" || resolution.Path != filepath.Join(prefix, "bin", "engram") {
		t.Fatalf("non-Homebrew resolution = %+v", resolution)
	}
}

func TestDiagnoseRuntimeProcessKeepsPathAndCanonicalEvidenceIndependent(t *testing.T) {
	canonicalPath := writeVersionExecutable(t, "printf '1.19.0\\n'")
	pathExecutable := writeVersionExecutable(t, "printf '1.18.0\\n'")
	canonical := NewCanonical(canonicalPath)
	pathEngram := NewExecutable(pathExecutable, canonical, "1.18.0", nil)

	diagnosis := DiagnoseRuntimeProcess(Process{PID: 42, ExecutablePath: canonicalPath, Command: canonicalPath + " serve"}, canonical, &pathEngram)
	if diagnosis.OK() || len(diagnosis.Problems) != 1 || !strings.Contains(diagnosis.Problems[0], "different from PATH Engram "+pathExecutable) {
		t.Fatalf("DiagnoseRuntimeProcess() = %#v, want canonical process distinguished only from shadowing PATH", diagnosis)
	}
}

func TestDiagnoseVersionReportsStableExecutableProblems(t *testing.T) {
	wantErr := errors.New("command failed")
	for _, tt := range []struct {
		name       string
		executable Executable
		want       string
	}{
		{name: "command failure", executable: Executable{Path: "/tmp/engram", VersionErr: wantErr}, want: "could not inspect /tmp/engram version: command failed"},
		{name: "empty output", executable: Executable{Path: "/tmp/engram"}, want: "/tmp/engram returned an empty version"},
		{name: "reported version", executable: Executable{Path: "/tmp/engram", Version: "1.19.0"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			diagnosis := DiagnoseVersion(tt.executable)
			if tt.want == "" {
				if diagnosis != nil {
					t.Fatalf("DiagnoseVersion() = %#v, want nil", diagnosis)
				}
				return
			}
			if diagnosis == nil || diagnosis.Detail != tt.want {
				t.Fatalf("DiagnoseVersion() = %#v, want %q", diagnosis, tt.want)
			}
		})
	}
}
