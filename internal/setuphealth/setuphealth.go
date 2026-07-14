// Package setuphealth owns read-only diagnosis of the base Matty setup.
package setuphealth

import (
	"fmt"
	"strings"

	"github.com/yersonargotev/matty/internal/codex"
	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
)

type Severity string

const (
	Pass Severity = "PASS"
	Warn Severity = "WARN"
	Fail Severity = "FAIL"
)

type Check struct {
	Name     string
	Severity Severity
	Detail   string
}

type Summary struct {
	Status   string
	Passes   int
	Warnings int
	Failures int
}

type Context struct {
	HomeDir        string
	ConfigHome     string
	StateFile      string
	StateStatus    string
	AgentSkillsDir string
}

type Report struct {
	SchemaVersion int
	Kind          string
	Context       Context
	Checks        []Check
	Summary       Summary
}

// Diagnose builds the stable setup-health report from detached observations
// supplied by the modules that own each artifact.
func Diagnose(homeDir, configHome string, lifecycle corelifecycle.SetupObservation, engram engrambin.SetupObservation, codexObservation codex.SetupObservation, openCodeObservation opencode.SetupObservation) Report {
	state := lifecycle.State()
	checks := []Check{stateCheck(lifecycle)}
	checks = append(checks, skillChecks(lifecycle)...)
	checks = append(checks, engramChecks(engram, state)...)
	checks = append(checks, codexChecks(codexObservation)...)
	checks = append(checks, openCodeChecks(openCodeObservation)...)
	summary := summarize(checks)
	stateStatus := "missing"
	if state.Found() {
		stateStatus = "present"
	}
	return Report{
		SchemaVersion: 1,
		Kind:          "doctor",
		Context: Context{
			HomeDir:        homeDir,
			ConfigHome:     configHome,
			StateFile:      lifecycle.StateFile(),
			StateStatus:    stateStatus,
			AgentSkillsDir: lifecycle.SkillsRoot(),
		},
		Checks:  checks,
		Summary: summary,
	}
}

func summarize(checks []Check) Summary {
	summary := Summary{Status: "healthy"}
	for _, check := range checks {
		switch check.Severity {
		case Pass:
			summary.Passes++
		case Warn:
			summary.Warnings++
		case Fail:
			summary.Failures++
		}
	}
	if summary.Failures > 0 {
		summary.Status = "failures"
	} else if summary.Warnings > 0 {
		summary.Status = "warnings"
	}
	return summary
}

func stateCheck(lifecycle corelifecycle.SetupObservation) Check {
	state := lifecycle.State()
	if state.Condition() == corelifecycle.StateCorrupt {
		return Check{Severity: Fail, Name: "matty-state", Detail: state.Err().Error() + "; inspect or remove the corrupt state, then run matty install"}
	}
	if state.Condition() == corelifecycle.StateMissing {
		return Check{Severity: Warn, Name: "matty-state", Detail: "missing at " + lifecycle.StateFile() + "; run matty install"}
	}
	if state.Condition() == corelifecycle.StateRecoveryRequired {
		return Check{Severity: Fail, Name: "matty-state", Detail: "classic installation was interrupted and requires recovery; run matty install or matty update to retry safely, or matty uninstall to remove only verified Matty-owned artifacts"}
	}
	return Check{Severity: Pass, Name: "matty-state", Detail: "present at " + lifecycle.StateFile()}
}

func skillChecks(lifecycle corelifecycle.SetupObservation) []Check {
	state := lifecycle.State()
	if !state.Found() {
		return []Check{{Severity: Warn, Name: "skill-symlinks", Detail: "state is missing, so Matty-owned skill links are unknown; run matty install"}}
	}
	managedLinks := lifecycle.ManagedSkillLinks()
	if len(managedLinks) == 0 {
		return []Check{{Severity: Warn, Name: "skill-symlinks", Detail: zeroManagedSkillsDetail(lifecycle)}}
	}
	var missing, changed []string
	for _, link := range managedLinks {
		switch {
		case link.Err() != nil:
			changed = append(changed, fmt.Sprintf("%s (%v)", link.Name(), link.Err()))
		case link.Condition() == corelifecycle.SkillLinkMissing:
			missing = append(missing, link.Name())
		case link.Condition() == corelifecycle.SkillLinkUnmanagedPath:
			changed = append(changed, link.Name()+" is not a symlink")
		case link.Condition() == corelifecycle.SkillLinkUnmanagedSymlink:
			changed = append(changed, link.Name())
		case link.Condition() != corelifecycle.SkillLinkManaged:
			changed = append(changed, fmt.Sprintf("%s (unknown link status %s)", link.Name(), link.Condition()))
		}
	}
	if len(missing) == 0 && len(changed) == 0 {
		return []Check{{Severity: Pass, Name: "skill-symlinks", Detail: fmt.Sprintf("%d managed links under %s", len(managedLinks), lifecycle.SkillsRoot())}}
	}
	detail := "managed skill links need repair"
	if len(missing) > 0 {
		detail += "; missing: " + strings.Join(missing, ", ")
	}
	if len(changed) > 0 {
		detail += "; changed: " + strings.Join(changed, ", ")
	}
	return []Check{{Severity: Fail, Name: "skill-symlinks", Detail: detail + "; run matty update"}}
}

func zeroManagedSkillsDetail(lifecycle corelifecycle.SetupObservation) string {
	detail := "state has no managed skills; run matty install"
	links, err := lifecycle.ExpectedSkillLinks(), lifecycle.ExpectedSkillLinksErr()
	if err != nil {
		return detail + "; could not inspect expected skill links: " + err.Error()
	}
	var unmanaged []corelifecycle.SkillLinkObservation
	for _, link := range links {
		if link.Err() != nil {
			return detail + "; could not inspect expected skill links: " + link.Err().Error()
		}
		if link.Condition() == corelifecycle.SkillLinkUnmanagedSymlink {
			unmanaged = append(unmanaged, link)
		}
	}
	if len(links) == 0 || len(unmanaged)*2 <= len(links) {
		return detail
	}
	example := unmanaged[0]
	return fmt.Sprintf("state has no managed skills, but %d expected skill symlinks are unmanaged by current Matty state; setup may be incomplete. Example: %s -> %s. %s", len(unmanaged), example.LinkPath(), example.Target(), unmanagedSymlinkRecoveryAdvice())
}

func unmanagedSymlinkRecoveryAdvice() string {
	return "Safe recovery: verify these are stale Matty-created links, remove them, then run matty install; Matty will not overwrite arbitrary files or links."
}

func engramChecks(observation engrambin.SetupObservation, state corelifecycle.StateObservation) []Check {
	checks := engramBinaryChecks(observation)
	checks = append(checks, engramRuntimeChecks(observation)...)
	if !state.Found() {
		return append(checks, Check{Severity: Warn, Name: "engram-setup", Detail: "state is missing, so delegated setup cannot be confirmed; run matty install"})
	}
	configuredSurfaces := state.ConfiguredSurfaces()
	if hasSurface(configuredSurfaces, "codex") && hasSurface(configuredSurfaces, "opencode") {
		return append(checks, Check{Severity: Pass, Name: "engram-setup", Detail: "state records Codex and OpenCode setup expectations; run matty update if Engram setup drifted"})
	}
	return append(checks, Check{Severity: Fail, Name: "engram-setup", Detail: "state does not record both Codex and OpenCode setup expectations; run matty update"})
}

func engramBinaryChecks(observation engrambin.SetupObservation) []Check {
	canonical := observation.Canonical()
	if observation.LookupErr() != nil {
		detail := "engram is not available on PATH; run matty install"
		if canonical != nil {
			detail = fmt.Sprintf("engram is not available on PATH; Homebrew Engram exists at %s; add it to PATH or run matty install", canonical.Path)
		}
		checks := []Check{{Severity: Fail, Name: "engram-binary", Detail: detail}}
		return append(checks, engramLocalBinChecks(observation.LocalBinDiagnoses())...)
	}
	return engramDiagnosticChecks(observation.Executables(), observation.LocalBinDiagnoses(), canonical, observation.ExpectedPath())
}

func engramDiagnosticChecks(executables []engrambin.Executable, localBin []engrambin.LocalBinDiagnosis, canonical *engrambin.Canonical, expectedPath string) []Check {
	pathEngram := executables[0]
	checks := []Check{engramPathCheck(pathEngram, canonical, expectedPath)}
	for _, executable := range executables {
		if diagnosis := engrambin.DiagnoseVersion(executable); diagnosis != nil {
			checks = append(checks, Check{Severity: Warn, Name: "engram-version", Detail: diagnosis.Detail})
		}
	}
	if mismatch := engrambin.DiagnoseVersionMismatch(executables); mismatch != nil {
		checks = append(checks, Check{Severity: Warn, Name: "engram-version-mismatch", Detail: mismatch.Detail})
	}
	if shadowing := engrambin.DiagnoseHomebrewShadowing(executables); shadowing != nil {
		checks = append(checks, Check{Severity: Warn, Name: "engram-path-shadowing", Detail: shadowing.Detail})
	}
	return append(checks, engramLocalBinChecks(localBin)...)
}

func engramPathCheck(pathEngram engrambin.Executable, canonical *engrambin.Canonical, expectedPath string) Check {
	if pathEngram.Canonical {
		return Check{Severity: Pass, Name: "engram-binary", Detail: "PATH resolves to canonical Homebrew Engram: " + engrambin.Detail(pathEngram)}
	}
	expected := expectedPath
	if canonical != nil {
		expected = canonical.Path
	}
	return Check{Severity: Warn, Name: "engram-binary", Detail: fmt.Sprintf("PATH resolves to non-Homebrew Engram %s; expected Homebrew-managed Engram at %s", engrambin.Detail(pathEngram), expected)}
}

func engramLocalBinChecks(diagnoses []engrambin.LocalBinDiagnosis) []Check {
	checks := make([]Check, 0, len(diagnoses))
	for _, diagnosis := range diagnoses {
		severity := Warn
		if diagnosis.OK {
			severity = Pass
		}
		checks = append(checks, Check{Severity: severity, Name: "engram-local-bin", Detail: diagnosis.Detail})
	}
	return checks
}

func engramRuntimeChecks(observation engrambin.SetupObservation) []Check {
	if observation.ProcessErr() != nil {
		return []Check{{Severity: Warn, Name: "engram-runtime", Detail: "could not inspect active engram serve processes: " + observation.ProcessErr().Error()}}
	}
	processes := observation.Processes()
	if len(processes) == 0 {
		return []Check{{Severity: Pass, Name: "engram-runtime", Detail: "no active engram serve process found"}}
	}
	checks := make([]Check, 0, len(processes))
	for _, process := range processes {
		diagnosis := engrambin.DiagnoseRuntimeProcess(process, observation.Canonical(), observation.PathExecutable())
		detail := fmt.Sprintf("pid %d running %s", process.PID, process.ExecutablePath)
		if diagnosis.OK() {
			checks = append(checks, Check{Severity: Pass, Name: "engram-runtime", Detail: detail + " (matches PATH and canonical Homebrew Engram)"})
		} else {
			checks = append(checks, Check{Severity: Warn, Name: "engram-runtime", Detail: detail + "; " + strings.Join(diagnosis.Problems, "; ") + "; " + diagnosis.Remediation})
		}
	}
	return checks
}

func hasSurface(configuredSurfaces []string, want string) bool {
	for _, surface := range configuredSurfaces {
		if surface == want {
			return true
		}
	}
	return false
}

func codexChecks(observation codex.SetupObservation) []Check {
	if observation.Err() != nil {
		return []Check{{Severity: Fail, Name: "codex-config", Detail: fmt.Sprintf("cannot read %s: %v; inspect permissions", observation.PromptFile(), observation.Err())}}
	}
	if !observation.Exists() {
		return []Check{{Severity: Warn, Name: "codex-config", Detail: "missing Matty Codex prompt markers at " + observation.PromptFile() + "; run matty install"}}
	}
	checks := []Check{}
	if observation.HasMattyMarkers() {
		checks = append(checks, Check{Severity: Pass, Name: "codex-config", Detail: "Matty prompt markers are present"})
	} else {
		checks = append(checks, Check{Severity: Warn, Name: "codex-config", Detail: "Matty prompt markers are missing; run matty install"})
	}
	for _, warning := range observation.Warnings() {
		if strings.Contains(warning, "gentle-ai") {
			checks = append(checks, Check{Severity: Warn, Name: "codex-conflict", Detail: warning + "; inspect duplicate global instructions"})
		}
	}
	return checks
}

func openCodeChecks(observation opencode.SetupObservation) []Check {
	if observation.Err() != nil {
		return []Check{{Severity: Fail, Name: "opencode-config", Detail: observation.Err().Error() + "; inspect the config or run matty install"}}
	}
	inspection := observation.Inspection()
	checks := []Check{}
	switch {
	case inspection.HasMattyInstruction && inspection.PromptExists:
		checks = append(checks, Check{Severity: Pass, Name: "opencode-config", Detail: "Matty instruction reference and prompt file are present"})
	case !inspection.ConfigExists:
		checks = append(checks, Check{Severity: Warn, Name: "opencode-config", Detail: "missing OpenCode config; run matty install"})
	case !inspection.HasMattyInstruction:
		checks = append(checks, Check{Severity: Warn, Name: "opencode-config", Detail: "Matty instruction reference is missing; run matty install"})
	default:
		checks = append(checks, Check{Severity: Warn, Name: "opencode-config", Detail: "Matty prompt file is missing; run matty update"})
	}
	for _, warning := range inspection.Warnings {
		checks = append(checks, Check{Severity: Warn, Name: "opencode-conflict", Detail: warning + "; inspect duplicate OpenCode overlays"})
	}
	return checks
}
