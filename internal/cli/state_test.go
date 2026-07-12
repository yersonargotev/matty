package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveStatePublishesInitialState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	want := DesiredState(Paths{StateFile: path, AgentSkillsDir: filepath.Join(dir, "skills")}, time.Unix(1, 0), nil)
	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}
	got, found, err := LoadState(path)
	if err != nil || !found || got.LastInstallCheck != want.LastInstallCheck {
		t.Fatalf("LoadState = %#v, %v, %v", got, found, err)
	}
	assertStateFileModeAndNoTemps(t, path)
}

func TestSaveStatePublishesCompleteReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	old := []byte("previous state bytes\n")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}

	want := DesiredState(Paths{StateFile: path, AgentSkillsDir: filepath.Join(dir, "skills")}, time.Unix(1, 0), nil)
	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}
	got, found, err := LoadState(path)
	if err != nil || !found || got.LastInstallCheck != want.LastInstallCheck {
		t.Fatalf("LoadState = %#v, %v, %v", got, found, err)
	}
	assertStateFileModeAndNoTemps(t, path)
}

func TestSaveStatePreservesPreviousBytesWhenTempWriteFails(t *testing.T) {
	path, old := existingStateFile(t)
	previous := writeStateTemp
	writeStateTemp = func(*os.File, []byte) error { return errors.New("injected write failure") }
	t.Cleanup(func() { writeStateTemp = previous })

	err := SaveState(path, DesiredState(Paths{StateFile: path}, time.Unix(2, 0), nil))
	if err == nil || !strings.Contains(err.Error(), "write Matty state temporary file") || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v", err)
	}
	assertPreviousStateAndNoTemps(t, path, old)
}

func TestSaveStatePreservesPreviousBytesWhenPublicationFails(t *testing.T) {
	path, old := existingStateFile(t)
	previous := publishStateTemp
	publishStateTemp = func(_, _ string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { publishStateTemp = previous })

	err := SaveState(path, DesiredState(Paths{StateFile: path}, time.Unix(3, 0), nil))
	if err == nil || !strings.Contains(err.Error(), "publish Matty state") || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v", err)
	}
	assertPreviousStateAndNoTemps(t, path, old)
}

func existingStateFile(t *testing.T) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	old := []byte("{\n  \"schema_version\": 1,\n  \"matty_version\": \"old\",\n  \"managed_skills\": [],\n  \"configured_surfaces\": [],\n  \"paths\": {\"state_file\": \"old\", \"agent_skills_dir\": \"old\"}\n}\n")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, old
}

func assertPreviousStateAndNoTemps(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("live state changed after failed save:\n%s", got)
	}
	assertStateFileModeAndNoTemps(t, path)
}

func assertStateFileModeAndNoTemps(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("state mode = %o, want 600", got)
	}
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".matty-state-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("abandoned state temporaries: %v", temps)
	}
}
