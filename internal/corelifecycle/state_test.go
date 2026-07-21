package corelifecycle

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/ownedcontainer"
	"github.com/yersonargotev/packy/internal/skillbundle"
)

func TestObserveSetupSuppliesStateAndManagedSkillFactsReadOnly(t *testing.T) {
	home := t.TempDir()
	stateLayout := NewLayout(filepath.Join(home, ".packy"))
	skills := skillbundle.NewGlobalLayout(home)
	source := skillbundle.Source{Root: filepath.Join(home, "source", "skills")}
	managedSource := filepath.Join(source.Root, "engineering", "managed")
	managedLink := skills.Skill("managed")
	if err := os.MkdirAll(managedSource, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(managedLink), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(managedSource, managedLink); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(stateLayout.StateFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	state := DesiredState(StateConfig{StateFile: stateLayout.StateFile(), AgentSkillsDir: skills.Root()}, time.Unix(1, 0), []ManagedSkill{{Name: "managed", SourcePath: managedSource, LinkPath: managedLink}})
	if err := SaveState(stateLayout.StateFile(), state); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(stateLayout.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	observation := ObserveSetup(stateLayout, skills, source)

	if observation.StateFile() != stateLayout.StateFile() || observation.SkillsRoot() != skills.Root() || !observation.State().Found() {
		t.Fatalf("observation = %#v", observation)
	}
	links := observation.ManagedSkillLinks()
	if len(links) != 1 || links[0].Condition() != SkillLinkManaged {
		t.Fatalf("managed links = %#v", links)
	}
	after, err := os.ReadFile(stateLayout.StateFile())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("setup observation mutated classic state")
	}
}

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
	if err == nil || !strings.Contains(err.Error(), "write Packy state temporary file") || !strings.Contains(err.Error(), path) {
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
	if err == nil || !strings.Contains(err.Error(), "publish Packy state") || !strings.Contains(err.Error(), path) {
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
	legacy := `{"schema_version":1,"packy_version":"legacy","managed_skills":[{"name":"ask-matt","source_path":"/source","link_path":"/link"}],"configured_surfaces":["codex","opencode"],"paths":{"state_file":"legacy","agent_skills_dir":"legacy"},"created_containers":[{"path":"/owned","kind":"directory"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	observation := ObserveState(path)
	if observation.Condition() != StateLegacy || !observation.Found() || observation.Err() != nil {
		t.Fatalf("legacy observation = condition %q found %v err %v", observation.Condition(), observation.Found(), observation.Err())
	}
	ownership := observation.Ownership()
	if len(ownership.ManagedSkills) != 1 || ownership.ManagedSkills[0].Name != "ask-matt" {
		t.Fatalf("managed ownership = %#v", ownership.ManagedSkills)
	}
	if len(ownership.CreatedContainers) != 1 || ownership.CreatedContainers[0] != (ownedcontainer.Record{Path: "/owned", Kind: ownedcontainer.Directory}) {
		t.Fatalf("container ownership = %#v", ownership.CreatedContainers)
	}
	if got := observation.ConfiguredSurfaces(); len(got) != 2 || got[0] != "codex" || got[1] != "opencode" {
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

func TestObserveStateCopiesClaudeOwnershipDeeply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	state := DesiredState(StateConfig{StateFile: path, AgentSkillsDir: "/skills"}, time.Unix(4, 0), nil)
	state.ClaudeOwnership = []ClaudeOwnership{{
		ID: "claude-mcp", Kind: ClaudeOwnershipMCP, Target: "engram", Contributors: []string{"classic"},
		Args: []string{"serve"}, EnvironmentKeys: []string{"TOKEN"},
	}}
	if err := SaveState(path, state); err != nil {
		t.Fatal(err)
	}

	observation := ObserveState(path)
	ownership := observation.Ownership()
	if len(ownership.ClaudeOwnership) != 1 || ownership.ClaudeOwnership[0].ID != "claude-mcp" {
		t.Fatalf("Claude ownership = %#v", ownership.ClaudeOwnership)
	}
	ownership.ClaudeOwnership[0].Contributors[0] = "changed"
	ownership.ClaudeOwnership[0].Args[0] = "changed"
	ownership.ClaudeOwnership[0].EnvironmentKeys[0] = "changed"

	again := observation.Ownership().ClaudeOwnership[0]
	if again.Contributors[0] != "classic" || again.Args[0] != "serve" || again.EnvironmentKeys[0] != "TOKEN" {
		t.Fatalf("observation exposed mutable Claude ownership: %#v", again)
	}
	snapshot := observation.ClaudeOwnershipSnapshot()
	if len(snapshot.Records) != 1 || snapshot.Records[0].Kind != string(claudecode.ActionUserMCP) || snapshot.Records[0].Target != "engram" {
		t.Fatalf("Claude ownership snapshot = %#v", snapshot)
	}
}

func TestClassicStateOwnershipUsesCanonicalPersistedKinds(t *testing.T) {
	records := classicStateOwnership([]claudecode.OwnershipRecord{
		{Kind: string(claudecode.ActionAgentFile), ID: "agent"},
		{Kind: string(claudecode.ActionCommandHook), ID: "hook", Args: []string{"session"}},
	})
	if len(records) != 2 || records[0].Kind != ClaudeOwnershipAgent || records[1].Kind != ClaudeOwnershipHook {
		t.Fatalf("persisted kinds = %#v", records)
	}
	data, err := json.Marshal(records[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"kind":"hook"`) || !strings.Contains(string(data), `"args":["session"]`) {
		t.Fatalf("hook wire contract = %s", data)
	}
}

func TestObserveManagedSkillLinksReportsReadOnlyFilesystemFacts(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	links := filepath.Join(root, "links")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(links, 0o700); err != nil {
		t.Fatal(err)
	}

	skills := []ManagedSkill{
		{Name: "missing", SourcePath: filepath.Join(source, "missing"), LinkPath: filepath.Join(links, "missing")},
		{Name: "managed", SourcePath: filepath.Join(source, "managed"), LinkPath: filepath.Join(links, "managed")},
		{Name: "path", SourcePath: filepath.Join(source, "path"), LinkPath: filepath.Join(links, "path")},
		{Name: "symlink", SourcePath: filepath.Join(source, "symlink"), LinkPath: filepath.Join(links, "symlink")},
	}
	if err := os.Symlink(filepath.Join("..", "source", "managed"), skills[1].LinkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skills[2].LinkPath, []byte("unmanaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, "foreign")
	if err := os.Symlink(foreign, skills[3].LinkPath); err != nil {
		t.Fatal(err)
	}

	observations := ObserveManagedSkillLinks(skills)
	want := []SkillLinkCondition{SkillLinkMissing, SkillLinkManaged, SkillLinkUnmanagedPath, SkillLinkUnmanagedSymlink}
	if len(observations) != len(want) {
		t.Fatalf("observations = %#v", observations)
	}
	for i, condition := range want {
		if observations[i].Name() != skills[i].Name || observations[i].LinkPath() != skills[i].LinkPath || observations[i].Condition() != condition || observations[i].Err() != nil {
			t.Fatalf("observation[%d] = name %q path %q condition %q err %v", i, observations[i].Name(), observations[i].LinkPath(), observations[i].Condition(), observations[i].Err())
		}
	}
	if observations[3].Target() != foreign {
		t.Fatalf("unmanaged target = %q, want %q", observations[3].Target(), foreign)
	}

	observations[0] = SkillLinkObservation{}
	again := ObserveManagedSkillLinks(skills)
	if again[0].Name() != "missing" || again[0].Condition() != SkillLinkMissing {
		t.Fatalf("caller mutation changed later observation: %#v", again[0])
	}
}

func TestObserveExpectedManagedSkillLinksUsesLifecycleDiscovery(t *testing.T) {
	config := installTestConfig(t)
	observations, err := ObserveExpectedManagedSkillLinks(Config{
		AgentSkillsDir:         config.Skills.Root(),
		SkillSourceRoot:        config.SkillSource.Root,
		SkillSourceMissingHint: config.SkillSource.MissingHint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 6 {
		t.Fatalf("expected observations = %#v", observations)
	}
	for _, observation := range observations {
		if observation.Condition() != SkillLinkMissing {
			t.Fatalf("expected observation = %#v", observation)
		}
	}
}

func existingStateFile(t *testing.T) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	old := []byte("{\n  \"schema_version\": 1,\n  \"packy_version\": \"old\",\n  \"managed_skills\": [],\n  \"configured_surfaces\": [],\n  \"paths\": {\"state_file\": \"old\", \"agent_skills_dir\": \"old\"}\n}\n")
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
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".packy-state-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("abandoned state temporaries: %v", temps)
	}
}
