package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cliSource struct {
	name string
	text string
}

func cliGoSources(t *testing.T) []cliSource {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]cliSource, 0, len(files))
	for _, file := range files {
		if file == "architecture_test.go" {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, cliSource{name: file, text: string(source)})
	}
	return sources
}

func TestClassicLifecycleDeletionDoesNotRedistributePolicyInCLI(t *testing.T) {
	for _, obsolete := range []string{"plan.go", "skills.go"} {
		if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
			t.Fatalf("obsolete CLI lifecycle module %s still exists", obsolete)
		}
	}

	for _, source := range cliGoSources(t) {
		file := source.name
		for _, forbidden := range []string{
			"type Plan struct",
			"type PlannedAction struct",
			"type ActionKind string",
			"func DiscoverManagedSkills(",
			"func plannedSkillLinkAction(",
			"func inspectSkillLink(",
			"skillLinkBehaviors",
			"unmanagedSymlinkSkipSummary",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s retained or redistributed obsolete classic lifecycle structure %q", file, forbidden)
			}
		}
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, forbidden := range []string{"os.Lstat(", "os.Readlink(", "os.Symlink(", "os.Remove(", "skillbundle.Discover(", "corelifecycle.LoadState(", "corelifecycle.SaveState("} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s redistributed classic lifecycle policy through %q", file, forbidden)
			}
		}
	}

	root, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	for call, want := range map[string]int{
		"corelifecycle.NewFacade(": 3,
		"lifecycle.Preview(":       3,
		"lifecycle.Apply(":         3,
	} {
		if got := strings.Count(string(root), call); got != want {
			t.Fatalf("root.go has %d occurrences of %q, want one route for each of three classic operations", got, call)
		}
	}
	for _, operation := range []string{"corelifecycle.Install", "corelifecycle.Update", "corelifecycle.Uninstall"} {
		if got := strings.Count(string(root), operation); got != 1 {
			t.Fatalf("root.go has %d production routes for %s, want 1", got, operation)
		}
	}
}

func TestSetupHealthDeletionDoesNotRedistributeDiagnosisPolicyInCLI(t *testing.T) {
	for _, obsolete := range []string{"doctor.go", "doctor_test.go"} {
		if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
			t.Fatalf("obsolete CLI setup-health file %s still exists", obsolete)
		}
	}

	for _, source := range cliGoSources(t) {
		file := source.name
		for _, forbidden := range []string{
			"BuildDoctorReport",
			"buildDoctorReport",
			"RunDoctor",
			"type DoctorReport",
			"type DoctorSummary",
			"DoctorReport =",
			"DoctorSummary =",
			"doctorCheck",
			"doctorStatus",
			"stateCheck(",
			"skillChecks(",
			"engramChecks(",
			"codexChecks(",
			"openCodeChecks(",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s retained or redistributed obsolete setup-health structure %q", file, forbidden)
			}
		}
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, forbidden := range []string{
			"corelifecycle.ObserveState(",
			"corelifecycle.ObserveManagedSkillLinks(",
			"engrambin.Diagnose",
			"opencode.Inspect(",
			"prompt.DetectExternalManagedBlocks(",
			"\"matty-state\"",
			"\"engram-binary\"",
			"\"codex-config\"",
			"\"opencode-config\"",
		} {
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s redistributed setup-health diagnosis policy through %q", file, forbidden)
			}
		}
	}

	root, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	for call, want := range map[string]int{
		"setuphealth.New(":          1,
		"opts.SetupHealthDiagnose(": 1,
	} {
		if got := strings.Count(string(root), call); got != want {
			t.Fatalf("root.go has %d occurrences of %q, want %d", got, call, want)
		}
	}
}
