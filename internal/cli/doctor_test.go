package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/matty/internal/engrambin"
)

func TestEngramBinaryChecks_NoEngram(t *testing.T) {
	checks := engramBinaryChecksWithHomebrewPrefixes(&fakeRunner{}, "", "", nil)

	assertDoctorCheck(t, checks, doctorFail, "engram-binary", "engram is not available")
	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertNoDoctorCheck(t, checks, "engram-path-shadowing")
}

func TestEngramBinaryChecks_SingleEngramReportsPathAndVersion(t *testing.T) {
	bin := t.TempDir()
	engram := writeEngramExecutable(t, bin, "engram version 1.19.0")

	checks := engramDiagnosticChecks([]engrambin.Executable{engrambin.NewExecutable(engram, nil, "1.19.0", nil)}, "", nil, nil)

	assertDoctorCheck(t, checks, doctorWarn, "engram-binary", "PATH resolves to non-Homebrew Engram "+engram+" version 1.19.0")
	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertNoDoctorCheck(t, checks, "engram-path-shadowing")
}

func TestEngramBinaryChecks_MultipleSameVersionEngramsDoNotWarn(t *testing.T) {
	firstBin := t.TempDir()
	secondBin := t.TempDir()
	first := writeEngramExecutable(t, firstBin, "1.19.0")
	writeEngramExecutable(t, secondBin, "engram 1.19.0")

	checks := engramDiagnosticChecks([]engrambin.Executable{
		engrambin.NewExecutable(first, nil, "1.19.0", nil),
		engrambin.NewExecutable(filepath.Join(secondBin, "engram"), nil, "1.19.0", nil),
	}, "", nil, nil)

	assertDoctorCheck(t, checks, doctorWarn, "engram-binary", "PATH resolves to non-Homebrew Engram "+first+" version 1.19.0")
	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertNoDoctorCheck(t, checks, "engram-path-shadowing")
}

func TestEngramBinaryChecks_ShadowedOlderEngramWarns(t *testing.T) {
	localBin := t.TempDir()
	homebrewBin := filepath.Join(t.TempDir(), "opt", "homebrew", "bin")
	local := writeEngramExecutable(t, localBin, "engram version 1.17.0")
	homebrew := writeEngramExecutable(t, homebrewBin, "engram version 1.19.0")

	canonical := engrambin.NewCanonical(homebrew)
	checks := engramDiagnosticChecks([]engrambin.Executable{
		engrambin.NewExecutable(local, canonical, "1.17.0", nil),
		engrambin.NewExecutable(homebrew, canonical, "1.19.0", nil),
	}, "", canonical, []string{filepath.Dir(homebrewBin)})

	assertDoctorCheck(t, checks, doctorWarn, "engram-binary", "PATH resolves to non-Homebrew Engram "+local+" version 1.17.0")
	assertDoctorCheck(t, checks, doctorWarn, "engram-version-mismatch", local+" version 1.17.0")
	assertDoctorCheck(t, checks, doctorWarn, "engram-version-mismatch", homebrew+" version 1.19.0")
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", local+" appears before Homebrew Engram at "+homebrew)
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", "reports version 1.17.0")
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", "Homebrew reports version 1.19.0")
}

func TestEngramBinaryChecks_HomebrewEngramOutsidePathStillWarnsWhenShadowed(t *testing.T) {
	localBin := t.TempDir()
	homebrewPrefix := filepath.Join(t.TempDir(), "opt", "homebrew")
	homebrewBin := filepath.Join(homebrewPrefix, "bin")
	local := writeEngramExecutable(t, localBin, "engram version 1.19.0")
	homebrew := writeEngramExecutable(t, homebrewBin, "engram version 1.19.0")

	canonical := engrambin.NewCanonical(homebrew)
	checks := engramDiagnosticChecks([]engrambin.Executable{
		engrambin.NewExecutable(local, canonical, "1.19.0", nil),
		engrambin.NewExecutable(homebrew, canonical, "1.19.0", nil),
	}, "", canonical, []string{homebrewPrefix})

	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", local+" appears before Homebrew Engram at "+homebrew)
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", "reports version 1.19.0")
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", "Homebrew reports version 1.19.0")
}

func TestEngramBinaryChecks_HomebrewEngramOnPathPasses(t *testing.T) {
	homebrewPrefix := filepath.Join(t.TempDir(), "opt", "homebrew")
	homebrewBin := filepath.Join(homebrewPrefix, "bin")
	homebrew := writeEngramExecutable(t, homebrewBin, "engram version 1.19.0")

	canonical := engrambin.NewCanonical(homebrew)
	checks := engramDiagnosticChecks([]engrambin.Executable{engrambin.NewExecutable(homebrew, canonical, "1.19.0", nil)}, "", canonical, []string{homebrewPrefix})

	assertDoctorCheck(t, checks, doctorPass, "engram-binary", "PATH resolves to canonical Homebrew Engram: "+homebrew+" version 1.19.0")
	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertNoDoctorCheck(t, checks, "engram-path-shadowing")
}

func TestEngramBinaryChecks_LocalBinSymlinkToHomebrewDoesNotShadow(t *testing.T) {
	home := t.TempDir()
	localBin := filepath.Join(home, ".local", "bin")
	homebrewPrefix := filepath.Join(t.TempDir(), "opt", "homebrew")
	homebrew := writeEngramExecutable(t, filepath.Join(homebrewPrefix, "bin"), "engram version 1.19.0")
	if err := os.MkdirAll(localBin, 0o700); err != nil {
		t.Fatalf("mkdir local bin: %v", err)
	}
	local := filepath.Join(localBin, "engram")
	if err := os.Symlink(homebrew, local); err != nil {
		t.Fatalf("symlink local engram: %v", err)
	}

	canonical := engrambin.NewCanonical(homebrew)
	checks := engramDiagnosticChecks([]engrambin.Executable{engrambin.NewExecutable(local, canonical, "1.19.0", nil)}, local, canonical, []string{homebrewPrefix})

	assertDoctorCheck(t, checks, doctorPass, "engram-binary", "PATH resolves to canonical Homebrew Engram: "+local+" version 1.19.0")
	assertDoctorCheck(t, checks, doctorPass, "engram-local-bin", local+" -> "+homebrew+" points to Homebrew Engram")
	assertNoDoctorCheck(t, checks, "engram-path-shadowing")
}

func TestEngramLocalBinChecksWarnsForSecondBinary(t *testing.T) {
	home := t.TempDir()
	localBin := filepath.Join(home, ".local", "bin")
	local := writeEngramExecutable(t, localBin, "engram version 1.19.0")
	homebrew := writeEngramExecutable(t, filepath.Join(t.TempDir(), "opt", "homebrew", "bin"), "engram version 1.19.0")
	canonical := engrambin.NewCanonical(homebrew)

	checks := engramLocalBinChecks(local, canonical)

	assertDoctorCheck(t, checks, doctorWarn, "engram-local-bin", "exists but is not a symlink")
	assertDoctorCheck(t, checks, doctorWarn, "engram-local-bin", "Matty will not install a second Engram binary there")
}

func TestParseEngramServeProcesses(t *testing.T) {
	output := "  101 /opt/homebrew/bin/engram serve\n" +
		"  102 /usr/bin/grep engram serve\n" +
		"  103 /tmp/engram setup codex\n"

	processes := engrambin.ParseServeProcesses(output)

	if len(processes) != 1 {
		t.Fatalf("processes = %#v, want one engram serve", processes)
	}
	if processes[0].PID != 101 || processes[0].ExecutablePath != "/opt/homebrew/bin/engram" {
		t.Fatalf("unexpected process: %#v", processes[0])
	}
}

func TestParseEngramServeProcessesPrefersResolvedProcessExecutable(t *testing.T) {
	output := "  101 /opt/homebrew/bin/engram serve\n"
	cellar := "/opt/homebrew/Cellar/engram/1.20.0/bin/engram"

	processes := engrambin.ParseServeProcessesWithResolver(output, func(pid int) string {
		if pid != 101 {
			t.Fatalf("resolver pid = %d, want 101", pid)
		}
		return cellar
	})

	if len(processes) != 1 {
		t.Fatalf("processes = %#v, want one engram serve", processes)
	}
	if processes[0].ExecutablePath != cellar {
		t.Fatalf("ExecutablePath = %q, want resolved process executable %q", processes[0].ExecutablePath, cellar)
	}
}

func TestEngramRuntimeChecksWarnsForDifferentDaemonExecutable(t *testing.T) {
	homebrew := writeEngramExecutable(t, filepath.Join(t.TempDir(), "opt", "homebrew", "bin"), "engram version 1.19.0")
	other := writeEngramExecutable(t, filepath.Join(t.TempDir(), "other", "bin"), "engram version 1.18.0")
	canonical := engrambin.NewCanonical(homebrew)
	pathEngram := engrambin.NewExecutable(homebrew, canonical, "1.19.0", nil)

	checks := engramRuntimeChecksForProcesses([]engrambin.Process{{PID: 42, ExecutablePath: other, Command: other + " serve"}}, canonical, &pathEngram)

	assertDoctorCheck(t, checks, doctorWarn, "engram-runtime", "pid 42 running "+other)
	assertDoctorCheck(t, checks, doctorWarn, "engram-runtime", "different from PATH Engram "+homebrew)
	assertDoctorCheck(t, checks, doctorWarn, "engram-runtime", "does not match canonical Homebrew Engram "+homebrew)
	assertDoctorCheck(t, checks, doctorWarn, "engram-runtime", "safe remediation: pkill -f 'engram serve' && "+homebrew+" serve")
}

func writeEngramExecutable(t *testing.T, dir, versionOutput string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir executable dir: %v", err)
	}
	path := filepath.Join(dir, "engram")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo '" + versionOutput + "'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func assertDoctorCheck(t *testing.T, checks []doctorCheck, status doctorStatus, name, detailContains string) {
	t.Helper()
	for _, check := range checks {
		if check.status == status && check.name == name && strings.Contains(check.detail, detailContains) {
			return
		}
	}
	t.Fatalf("missing %s %s containing %q in checks: %#v", status, name, detailContains, checks)
}

func assertNoDoctorCheck(t *testing.T, checks []doctorCheck, name string) {
	t.Helper()
	for _, check := range checks {
		if check.name == name {
			t.Fatalf("unexpected %s check: %#v", name, check)
		}
	}
}
