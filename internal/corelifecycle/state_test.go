package corelifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/matty/internal/ownedcontainer"
)

func TestStateStorePublishesInitialState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	want := DesiredState(StateConfig{StateFile: path, AgentSkillsDir: filepath.Join(dir, "skills")}, time.Unix(1, 0), nil)
	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}
	got, found, err := LoadState(path)
	if err != nil || !found || got.LastInstallCheck != want.LastInstallCheck {
		t.Fatalf("LoadState = %#v, %v, %v", got, found, err)
	}
	assertStateFileModeAndNoTemps(t, path)
}

func TestStateStorePublishesCompleteReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("previous state bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	want := DesiredState(StateConfig{StateFile: path, AgentSkillsDir: filepath.Join(dir, "skills")}, time.Unix(1, 0), nil)
	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}
	got, found, err := LoadState(path)
	if err != nil || !found || got.LastInstallCheck != want.LastInstallCheck {
		t.Fatalf("LoadState = %#v, %v, %v", got, found, err)
	}
	assertStateFileModeAndNoTemps(t, path)
}

func TestStateStorePreservesPreviousBytesWhenTempWriteFails(t *testing.T) {
	path, old := existingStateFile(t)
	previous := writeStateTemp
	writeStateTemp = func(*os.File, []byte) error { return errors.New("injected write failure") }
	t.Cleanup(func() { writeStateTemp = previous })

	err := SaveState(path, DesiredState(StateConfig{StateFile: path}, time.Unix(2, 0), nil))
	if err == nil || !strings.Contains(err.Error(), "write Matty state temporary file") || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v", err)
	}
	assertPreviousStateAndNoTemps(t, path, old)
}

func TestStateStorePreservesPreviousBytesWhenPublicationFails(t *testing.T) {
	path, old := existingStateFile(t)
	previous := publishStateTemp
	publishStateTemp = func(_, _ string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { publishStateTemp = previous })

	err := SaveState(path, DesiredState(StateConfig{StateFile: path}, time.Unix(3, 0), nil))
	if err == nil || !strings.Contains(err.Error(), "publish Matty state") || !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %v", err)
	}
	assertPreviousStateAndNoTemps(t, path, old)
}

func TestObserveStateDistinguishesStateConditions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	missing := ObserveState(path)
	if missing.Condition() != StateMissing || missing.Found() || missing.Err() != nil {
		t.Fatalf("missing observation = condition %q found %v err %v", missing.Condition(), missing.Found(), missing.Err())
	}

	if err := os.WriteFile(path, []byte(`{"schema_version":1`), 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt := ObserveState(path)
	if corrupt.Condition() != StateCorrupt || corrupt.Found() || corrupt.Err() == nil {
		t.Fatalf("corrupt observation = condition %q found %v err %v", corrupt.Condition(), corrupt.Found(), corrupt.Err())
	}

	validState := DesiredState(StateConfig{StateFile: path, AgentSkillsDir: filepath.Join(dir, "skills")}, time.Unix(4, 0), nil)
	if err := SaveState(path, validState); err != nil {
		t.Fatal(err)
	}
	valid := ObserveState(path)
	if valid.Condition() != StateValid || !valid.Found() || valid.Err() != nil {
		t.Fatalf("valid observation = condition %q found %v err %v", valid.Condition(), valid.Found(), valid.Err())
	}

	validState.InstallStatus = InstallRecoveryRequired
	if err := SaveState(path, validState); err != nil {
		t.Fatal(err)
	}
	recovery := ObserveState(path)
	if recovery.Condition() != StateRecoveryRequired || !recovery.Found() || recovery.Err() != nil {
		t.Fatalf("recovery observation = condition %q found %v err %v", recovery.Condition(), recovery.Found(), recovery.Err())
	}
}

func TestObserveStateReportsLegacyStateAndRecordedOwnershipReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"schema_version":1,"matty_version":"legacy","managed_skills":[{"name":"ask-matt","source_path":"/source","link_path":"/link"}],"configured_surfaces":["codex"],"paths":{"state_file":"legacy","agent_skills_dir":"legacy"},"created_containers":[{"path":"/owned","kind":"directory"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	observation := ObserveState(path)
	if observation.Condition() != StateValid || !observation.Found() || observation.Err() != nil {
		t.Fatalf("legacy observation = condition %q found %v err %v", observation.Condition(), observation.Found(), observation.Err())
	}
	ownership := observation.Ownership()
	if len(ownership.ManagedSkills) != 1 || ownership.ManagedSkills[0].Name != "ask-matt" {
		t.Fatalf("managed ownership = %#v", ownership.ManagedSkills)
	}
	if len(ownership.CreatedContainers) != 1 || ownership.CreatedContainers[0] != (ownedcontainer.Record{Path: "/owned", Kind: ownedcontainer.Directory}) {
		t.Fatalf("container ownership = %#v", ownership.CreatedContainers)
	}
	if got := observation.ConfiguredSurfaces(); len(got) != 1 || got[0] != "codex" {
		t.Fatalf("configured surfaces = %#v", got)
	}

	ownership.ManagedSkills[0].Name = "changed"
	ownership.CreatedContainers[0].Path = "/changed"
	surfaces := observation.ConfiguredSurfaces()
	surfaces[0] = "changed"
	again := observation.Ownership()
	if again.ManagedSkills[0].Name != "ask-matt" || again.CreatedContainers[0].Path != "/owned" || observation.ConfiguredSurfaces()[0] != "codex" {
		t.Fatal("observation exposed mutable recorded ownership")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("observation changed state file:\n%s", after)
	}
	assertNoStateTemps(t, path)
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
	assertNoStateTemps(t, path)
}

func assertNoStateTemps(t *testing.T, path string) {
	t.Helper()
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".matty-state-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("abandoned state temporaries: %v", temps)
	}
}
