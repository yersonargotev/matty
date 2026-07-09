package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngramBinaryChecks_NoEngram(t *testing.T) {
	checks := engramBinaryChecksWithHomebrewPrefixes(&fakeRunner{}, "", nil)

	assertDoctorCheck(t, checks, doctorFail, "engram-binary", "engram is not available")
	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertNoDoctorCheck(t, checks, "engram-path-shadowing")
}

func TestEngramBinaryChecks_SingleEngramReportsPathAndVersion(t *testing.T) {
	bin := t.TempDir()
	engram := writeEngramExecutable(t, bin, "engram version 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": engram}}

	checks := engramBinaryChecksWithHomebrewPrefixes(runner, bin, nil)

	assertDoctorCheck(t, checks, doctorPass, "engram-binary", engram+" version 1.19.0")
	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertNoDoctorCheck(t, checks, "engram-path-shadowing")
}

func TestEngramBinaryChecks_MultipleSameVersionEngramsDoNotWarn(t *testing.T) {
	firstBin := t.TempDir()
	secondBin := t.TempDir()
	first := writeEngramExecutable(t, firstBin, "1.19.0")
	writeEngramExecutable(t, secondBin, "engram 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": first}}

	checks := engramBinaryChecksWithHomebrewPrefixes(runner, strings.Join([]string{firstBin, secondBin}, string(os.PathListSeparator)), nil)

	assertDoctorCheck(t, checks, doctorPass, "engram-binary", first+" version 1.19.0")
	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertNoDoctorCheck(t, checks, "engram-path-shadowing")
}

func TestEngramBinaryChecks_ShadowedOlderEngramWarns(t *testing.T) {
	localBin := t.TempDir()
	homebrewBin := filepath.Join(t.TempDir(), "opt", "homebrew", "bin")
	local := writeEngramExecutable(t, localBin, "engram version 1.17.0")
	homebrew := writeEngramExecutable(t, homebrewBin, "engram version 1.19.0")
	runner := &fakeRunner{path: map[string]string{"engram": local}}

	checks := engramBinaryChecksWithHomebrewPrefixes(runner, strings.Join([]string{localBin, homebrewBin}, string(os.PathListSeparator)), []string{filepath.Dir(filepath.Dir(homebrewBin))})

	assertDoctorCheck(t, checks, doctorPass, "engram-binary", local+" version 1.17.0")
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
	runner := &fakeRunner{path: map[string]string{"engram": local}}

	checks := engramBinaryChecksWithHomebrewPrefixes(runner, localBin, []string{homebrewPrefix})

	assertNoDoctorCheck(t, checks, "engram-version-mismatch")
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", local+" appears before Homebrew Engram at "+homebrew)
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", "reports version 1.19.0")
	assertDoctorCheck(t, checks, doctorWarn, "engram-path-shadowing", "Homebrew reports version 1.19.0")
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
