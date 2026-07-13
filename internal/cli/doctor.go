package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yersonargotev/matty/internal/corelifecycle"
	"github.com/yersonargotev/matty/internal/engrambin"
	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/prompt"
)

// ErrDoctorUnhealthy identifies a completed diagnostic report containing one
// or more failed health checks. Warnings alone do not make a report unhealthy.
var ErrDoctorUnhealthy = errors.New("doctor found failed health checks")

type doctorHealthError struct{ failedChecks int }

func (err doctorHealthError) Error() string {
	return fmt.Sprintf("%s: %d", ErrDoctorUnhealthy, err.failedChecks)
}

func (err doctorHealthError) Unwrap() error { return ErrDoctorUnhealthy }

type doctorStatus string

const (
	doctorPass doctorStatus = "PASS"
	doctorWarn doctorStatus = "WARN"
	doctorFail doctorStatus = "FAIL"
)

type doctorCheck struct {
	status doctorStatus
	name   string
	detail string
}

type DoctorSummary struct {
	Status   string `json:"status"`
	Passes   int    `json:"passes"`
	Warnings int    `json:"warnings"`
	Failures int    `json:"failures"`
}

type DoctorReport struct {
	SchemaVersion int           `json:"schema_version"`
	Report        string        `json:"report"`
	Checks        []doctorCheck `json:"-"`
	Summary       DoctorSummary `json:"summary"`
	header        doctorHeader
}

type doctorHeader struct {
	home, configHome, stateFile, stateStatus, agentSkills string
}

type doctorJSONCheck struct {
	Name     string       `json:"name"`
	Severity doctorStatus `json:"severity"`
	Detail   string       `json:"detail"`
}

type doctorJSONReport struct {
	SchemaVersion int               `json:"schema_version"`
	Report        string            `json:"report"`
	Checks        []doctorJSONCheck `json:"checks"`
	Summary       DoctorSummary     `json:"summary"`
}

func BuildDoctorReport(paths Paths, runner Runner) DoctorReport {
	return buildDoctorReport(paths, runner, engrambin.SystemFacts())
}

func buildDoctorReport(paths Paths, runner Runner, facts engrambin.Facts) DoctorReport {
	state := corelifecycle.ObserveState(paths.StateFile)
	checks := []doctorCheck{stateCheck(paths, state)}
	checks = append(checks, skillChecks(paths, state)...)
	checks = append(checks, engramChecks(runner, paths, state, facts.WithDefaults())...)
	checks = append(checks, codexChecks(paths)...)
	openCodeChecks, openCodeErr := openCodeChecks(paths)
	if openCodeErr != nil {
		checks = append(checks, doctorCheck{status: doctorFail, name: "opencode-config", detail: openCodeErr.Error() + "; inspect the config or run matty install"})
	} else {
		checks = append(checks, openCodeChecks...)
	}
	summary := DoctorSummary{Status: "healthy"}
	for _, check := range checks {
		switch check.status {
		case doctorPass:
			summary.Passes++
		case doctorWarn:
			summary.Warnings++
		case doctorFail:
			summary.Failures++
		}
	}
	if summary.Failures > 0 {
		summary.Status = "failures"
	} else if summary.Warnings > 0 {
		summary.Status = "warnings"
	}
	stateStatus := "missing"
	if state.Found() {
		stateStatus = "present"
	}
	return DoctorReport{SchemaVersion: 1, Report: "doctor", Checks: checks, Summary: summary, header: doctorHeader{paths.HomeDir, paths.ConfigHome, paths.StateFile, stateStatus, paths.AgentSkillsDir}}
}

func (report DoctorReport) HealthError() error {
	if report.Summary.Failures > 0 {
		return doctorHealthError{failedChecks: report.Summary.Failures}
	}
	return nil
}

func RenderDoctorHuman(w io.Writer, report DoctorReport) error {
	h := report.header
	if _, err := fmt.Fprintf(w, "HOME=%s\nCONFIG_HOME=%s\nMATTY_STATE=%s\nMATTY_STATE_STATUS=%s\nAGENT_SKILLS=%s\n", h.home, h.configHome, h.stateFile, h.stateStatus, h.agentSkills); err != nil {
		return err
	}
	for _, check := range report.Checks {
		if _, err := fmt.Fprintf(w, "%s %s: %s\n", check.status, check.name, check.detail); err != nil {
			return err
		}
	}
	return nil
}

func RenderDoctorJSON(w io.Writer, report DoctorReport) error {
	checks := make([]doctorJSONCheck, 0, len(report.Checks))
	for _, check := range report.Checks {
		checks = append(checks, doctorJSONCheck{Name: check.name, Severity: check.status, Detail: check.detail})
	}
	return json.NewEncoder(w).Encode(doctorJSONReport{SchemaVersion: report.SchemaVersion, Report: report.Report, Checks: checks, Summary: report.Summary})
}

func RunDoctor(w io.Writer, paths Paths, runner Runner) error {
	report := BuildDoctorReport(paths, runner)
	if err := RenderDoctorHuman(w, report); err != nil {
		return err
	}
	return report.HealthError()
}

func stateCheck(paths Paths, state corelifecycle.StateObservation) doctorCheck {
	if state.Condition() == corelifecycle.StateCorrupt {
		return doctorCheck{status: doctorFail, name: "matty-state", detail: state.Err().Error() + "; inspect or remove the corrupt state, then run matty install"}
	}
	if state.Condition() == corelifecycle.StateMissing {
		return doctorCheck{status: doctorWarn, name: "matty-state", detail: "missing at " + paths.StateFile + "; run matty install"}
	}
	if state.Condition() == corelifecycle.StateRecoveryRequired {
		return doctorCheck{status: doctorFail, name: "matty-state", detail: "classic installation was interrupted and requires recovery; run matty install or matty update to retry safely, or matty uninstall to remove only verified Matty-owned artifacts"}
	}
	return doctorCheck{status: doctorPass, name: "matty-state", detail: "present at " + paths.StateFile}
}

func skillChecks(paths Paths, state corelifecycle.StateObservation) []doctorCheck {
	if !state.Found() {
		return []doctorCheck{{status: doctorWarn, name: "skill-symlinks", detail: "state is missing, so Matty-owned skill links are unknown; run matty install"}}
	}
	managedSkills := state.Ownership().ManagedSkills
	if len(managedSkills) == 0 {
		return []doctorCheck{{status: doctorWarn, name: "skill-symlinks", detail: zeroManagedSkillsDetail(paths)}}
	}
	var missing, changed []string
	for _, skill := range managedSkills {
		link, err := inspectSkillLink(skill)
		if err != nil {
			changed = append(changed, fmt.Sprintf("%s (%v)", skill.Name, err))
			continue
		}
		behavior, ok := skillLinkBehaviors[link.status]
		if !ok {
			changed = append(changed, fmt.Sprintf("%s (unknown link status %s)", skill.Name, link.status))
			continue
		}
		problem, hasProblem := behavior.doctorProblem(skill, link)
		if !hasProblem {
			continue
		}
		if problem.missing {
			missing = append(missing, problem.detail)
		} else {
			changed = append(changed, problem.detail)
		}
	}
	if len(missing) == 0 && len(changed) == 0 {
		return []doctorCheck{{status: doctorPass, name: "skill-symlinks", detail: fmt.Sprintf("%d managed links under %s", len(managedSkills), paths.AgentSkillsDir)}}
	}
	detail := "managed skill links need repair"
	if len(missing) > 0 {
		detail += "; missing: " + strings.Join(missing, ", ")
	}
	if len(changed) > 0 {
		detail += "; changed: " + strings.Join(changed, ", ")
	}
	return []doctorCheck{{status: doctorFail, name: "skill-symlinks", detail: detail + "; run matty update"}}
}

func zeroManagedSkillsDetail(paths Paths) string {
	detail := "state has no managed skills; run matty install"
	plan, err := buildDoctorExpectedSkillPlan(paths)
	if err != nil {
		return detail + "; could not inspect expected skill links: " + err.Error()
	}
	summary, ok := unmanagedSymlinkSkipSummary(plan)
	if !ok {
		return detail
	}
	return fmt.Sprintf("state has no managed skills, but %d expected skill symlinks are unmanaged by current Matty state; setup may be incomplete. Example: %s -> %s. %s", summary.count, summary.example.Path, summary.example.Target, unmanagedSymlinkRecoveryAdvice())
}

func engramChecks(runner Runner, paths Paths, state corelifecycle.StateObservation, facts engrambin.Facts) []doctorCheck {
	checks := engramBinaryChecks(runner, paths, facts)
	canonical := engrambin.DiscoverHomebrew(paths.HomebrewPrefixEnv)
	checks = append(checks, engramRuntimeChecks(canonical, pathEngramExecutable(runner, canonical), facts)...)
	if !state.Found() {
		checks = append(checks, doctorCheck{status: doctorWarn, name: "engram-setup", detail: "state is missing, so delegated setup cannot be confirmed; run matty install"})
		return checks
	}
	configuredSurfaces := state.ConfiguredSurfaces()
	if hasSurface(configuredSurfaces, "codex") && hasSurface(configuredSurfaces, "opencode") {
		checks = append(checks, doctorCheck{status: doctorPass, name: "engram-setup", detail: "state records Codex and OpenCode setup expectations; run matty update if Engram setup drifted"})
	} else {
		checks = append(checks, doctorCheck{status: doctorFail, name: "engram-setup", detail: "state does not record both Codex and OpenCode setup expectations; run matty update"})
	}
	return checks
}

func engramBinaryChecks(runner Runner, paths Paths, facts engrambin.Facts) []doctorCheck {
	return engramBinaryChecksWithHomebrewPrefixes(runner, paths.PathEnv, paths.LocalBinEngram, engrambin.HomebrewPrefixes(paths.HomebrewPrefixEnv), facts)
}

func engramBinaryChecksWithHomebrewPrefixes(runner Runner, pathEnv, localBinEngram string, homebrewPrefixes []string, facts engrambin.Facts) []doctorCheck {
	canonical := engrambin.DiscoverHomebrewFromPrefixes(homebrewPrefixes)
	resolved, err := runner.LookPath("engram")
	if err != nil {
		detail := "engram is not available on PATH; run matty install"
		if canonical != nil {
			detail = fmt.Sprintf("engram is not available on PATH; Homebrew Engram exists at %s; add it to PATH or run matty install", canonical.Path)
		}
		checks := []doctorCheck{{status: doctorFail, name: "engram-binary", detail: detail}}
		checks = append(checks, engramLocalBinChecks(localBinEngram, canonical)...)
		return checks
	}

	executablePaths := engrambin.UniquePaths(resolved, pathEnv, homebrewPrefixes)
	executables := make([]engrambin.Executable, 0, len(executablePaths))
	for _, path := range executablePaths {
		version, versionErr := facts.Version(path)
		executables = append(executables, engrambin.NewExecutable(path, canonical, version, versionErr))
	}
	return engramDiagnosticChecks(executables, localBinEngram, canonical, homebrewPrefixes)
}

func engramDiagnosticChecks(executables []engrambin.Executable, localBinEngram string, canonical *engrambin.Canonical, homebrewPrefixes []string) []doctorCheck {
	pathEngram := executables[0]
	checks := []doctorCheck{engramPathCheck(pathEngram, canonical, homebrewPrefixes)}
	checks = append(checks, engramVersionInspectionChecks(executables)...)
	checks = append(checks, engramVersionMismatchChecks(executables)...)
	if shadowing := engramHomebrewShadowingCheck(executables); shadowing != nil {
		checks = append(checks, *shadowing)
	}
	checks = append(checks, engramLocalBinChecks(localBinEngram, canonical)...)
	return checks
}

func engramVersionInspectionChecks(executables []engrambin.Executable) []doctorCheck {
	checks := []doctorCheck{}
	for _, executable := range executables {
		if diagnosis := engrambin.DiagnoseVersion(executable); diagnosis != nil {
			checks = append(checks, doctorCheck{status: doctorWarn, name: "engram-version", detail: diagnosis.Detail})
		}
	}
	return checks
}

func pathEngramExecutable(runner Runner, canonical *engrambin.Canonical) *engrambin.Executable {
	resolved, err := runner.LookPath("engram")
	if err != nil {
		return nil
	}
	executable := engrambin.NewExecutable(resolved, canonical, "", nil)
	return &executable
}

func engramPathCheck(pathEngram engrambin.Executable, canonical *engrambin.Canonical, prefixes []string) doctorCheck {
	if pathEngram.Canonical {
		return doctorCheck{status: doctorPass, name: "engram-binary", detail: "PATH resolves to canonical Homebrew Engram: " + engrambin.Detail(pathEngram)}
	}
	expected := engrambin.ExpectedHomebrewPathFromPrefixes(prefixes)
	if canonical != nil {
		expected = canonical.Path
	}
	return doctorCheck{
		status: doctorWarn,
		name:   "engram-binary",
		detail: fmt.Sprintf("PATH resolves to non-Homebrew Engram %s; expected Homebrew-managed Engram at %s", engrambin.Detail(pathEngram), expected),
	}
}

func engramVersionMismatchChecks(executables []engrambin.Executable) []doctorCheck {
	versionByPath := []string{}
	versions := map[string]bool{}
	for _, executable := range executables {
		if executable.Version == "" {
			continue
		}
		versions[executable.Version] = true
		versionByPath = append(versionByPath, fmt.Sprintf("%s version %s", executable.Path, executable.Version))
	}
	if len(versions) <= 1 {
		return nil
	}
	return []doctorCheck{{
		status: doctorWarn,
		name:   "engram-version-mismatch",
		detail: "multiple engram executables report different versions: " + strings.Join(versionByPath, ", "),
	}}
}

func engramHomebrewShadowingCheck(executables []engrambin.Executable) *doctorCheck {
	if len(executables) < 2 || executables[0].Canonical {
		return nil
	}
	resolved := executables[0]
	for _, executable := range executables[1:] {
		if !executable.Canonical {
			continue
		}
		detail := fmt.Sprintf("%s appears before Homebrew Engram at %s", resolved.Path, executable.Path)
		if resolved.Version != "" {
			detail += " and reports version " + resolved.Version
		}
		if executable.Version != "" {
			detail += "; Homebrew reports version " + executable.Version
		}
		return &doctorCheck{status: doctorWarn, name: "engram-path-shadowing", detail: detail}
	}
	return nil
}

func engramLocalBinChecks(localBinEngram string, canonical *engrambin.Canonical) []doctorCheck {
	diagnoses := engrambin.DiagnoseLocalBin(localBinEngram, canonical)
	checks := make([]doctorCheck, 0, len(diagnoses))
	for _, diagnosis := range diagnoses {
		status := doctorWarn
		if diagnosis.OK {
			status = doctorPass
		}
		checks = append(checks, doctorCheck{status: status, name: "engram-local-bin", detail: diagnosis.Detail})
	}
	return checks
}

func engramRuntimeChecks(canonical *engrambin.Canonical, pathEngram *engrambin.Executable, facts engrambin.Facts) []doctorCheck {
	processes, err := facts.ServeProcesses()
	if err != nil {
		return []doctorCheck{{status: doctorWarn, name: "engram-runtime", detail: "could not inspect active engram serve processes: " + err.Error()}}
	}
	return engramRuntimeChecksForProcesses(processes, canonical, pathEngram)
}

func engramRuntimeChecksForProcesses(processes []engrambin.Process, canonical *engrambin.Canonical, pathEngram *engrambin.Executable) []doctorCheck {
	if len(processes) == 0 {
		return []doctorCheck{{status: doctorPass, name: "engram-runtime", detail: "no active engram serve process found"}}
	}
	checks := make([]doctorCheck, 0, len(processes))
	for _, process := range processes {
		checks = append(checks, engramRuntimeCheckForProcess(process, canonical, pathEngram))
	}
	return checks
}

func engramRuntimeCheckForProcess(process engrambin.Process, canonical *engrambin.Canonical, pathEngram *engrambin.Executable) doctorCheck {
	diagnosis := engrambin.DiagnoseRuntimeProcess(process, canonical, pathEngram)
	detail := fmt.Sprintf("pid %d running %s", process.PID, process.ExecutablePath)
	if diagnosis.OK() {
		return doctorCheck{status: doctorPass, name: "engram-runtime", detail: detail + " (matches PATH and canonical Homebrew Engram)"}
	}
	return doctorCheck{status: doctorWarn, name: "engram-runtime", detail: detail + "; " + strings.Join(diagnosis.Problems, "; ") + "; " + diagnosis.Remediation}
}

func hasSurface(configuredSurfaces []string, want string) bool {
	for _, surface := range configuredSurfaces {
		if surface == want {
			return true
		}
	}
	return false
}

func codexChecks(paths Paths) []doctorCheck {
	data, err := os.ReadFile(paths.CodexPromptFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []doctorCheck{{status: doctorWarn, name: "codex-config", detail: "missing Matty Codex prompt markers at " + paths.CodexPromptFile + "; run matty install"}}
		}
		return []doctorCheck{{status: doctorFail, name: "codex-config", detail: fmt.Sprintf("cannot read %s: %v; inspect permissions", paths.CodexPromptFile, err)}}
	}
	content := string(data)
	checks := []doctorCheck{}
	if strings.Contains(content, "<!-- matty:skills-router -->") && strings.Contains(content, "<!-- /matty:skills-router -->") {
		checks = append(checks, doctorCheck{status: doctorPass, name: "codex-config", detail: "Matty prompt markers are present"})
	} else {
		checks = append(checks, doctorCheck{status: doctorWarn, name: "codex-config", detail: "Matty prompt markers are missing; run matty install"})
	}
	for _, warning := range prompt.DetectExternalManagedBlocks(content) {
		if strings.Contains(warning, "gentle-ai") {
			checks = append(checks, doctorCheck{status: doctorWarn, name: "codex-conflict", detail: warning + "; inspect duplicate global instructions"})
		}
	}
	return checks
}

func openCodeChecks(paths Paths) ([]doctorCheck, error) {
	inspection, err := opencode.Inspect(paths.OpenCodeConfigFile, paths.OpenCodePromptFile)
	if err != nil {
		return nil, err
	}
	checks := []doctorCheck{}
	switch {
	case inspection.HasMattyInstruction && inspection.PromptExists:
		checks = append(checks, doctorCheck{status: doctorPass, name: "opencode-config", detail: "Matty instruction reference and prompt file are present"})
	case !inspection.ConfigExists:
		checks = append(checks, doctorCheck{status: doctorWarn, name: "opencode-config", detail: "missing OpenCode config; run matty install"})
	case !inspection.HasMattyInstruction:
		checks = append(checks, doctorCheck{status: doctorWarn, name: "opencode-config", detail: "Matty instruction reference is missing; run matty install"})
	default:
		checks = append(checks, doctorCheck{status: doctorWarn, name: "opencode-config", detail: "Matty prompt file is missing; run matty update"})
	}
	for _, warning := range inspection.Warnings {
		checks = append(checks, doctorCheck{status: doctorWarn, name: "opencode-conflict", detail: warning + "; inspect duplicate OpenCode overlays"})
	}
	return checks, nil
}
