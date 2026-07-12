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

func writeVersionExecutable(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engram")
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
