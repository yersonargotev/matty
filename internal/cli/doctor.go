package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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

func RunDoctor(w io.Writer, paths Paths, runner Runner) error {
	state, stateFound, err := LoadState(paths.StateFile)
	if err != nil {
		state = State{}
		stateFound = false
	}
	stateStatus := "missing"
	if stateFound {
		stateStatus = "present"
	}
	if _, writeErr := fmt.Fprintf(w, "HOME=%s\nCONFIG_HOME=%s\nMATTY_STATE=%s\nMATTY_STATE_STATUS=%s\nAGENT_SKILLS=%s\n", paths.HomeDir, paths.ConfigHome, paths.StateFile, stateStatus, paths.AgentSkillsDir); writeErr != nil {
		return writeErr
	}

	checks := []doctorCheck{stateCheck(paths, stateFound, err)}
	checks = append(checks, skillChecks(paths, state, stateFound)...)
	checks = append(checks, engramChecks(runner, paths, state, stateFound)...)
	checks = append(checks, codexChecks(paths)...)
	openCodeChecks, err := openCodeChecks(paths)
	if err != nil {
		checks = append(checks, doctorCheck{status: doctorFail, name: "opencode-config", detail: err.Error() + "; inspect the config or run matty install"})
	} else {
		checks = append(checks, openCodeChecks...)
	}

	failedChecks := 0
	for _, check := range checks {
		if _, err := fmt.Fprintf(w, "%s %s: %s\n", check.status, check.name, check.detail); err != nil {
			return err
		}
		if check.status == doctorFail {
			failedChecks++
		}
	}
	if failedChecks > 0 {
		return doctorHealthError{failedChecks: failedChecks}
	}
	return nil
}

func stateCheck(paths Paths, found bool, loadErr error) doctorCheck {
	if loadErr != nil {
		return doctorCheck{status: doctorFail, name: "matty-state", detail: loadErr.Error() + "; inspect or remove the corrupt state, then run matty install"}
	}
	if !found {
		return doctorCheck{status: doctorWarn, name: "matty-state", detail: "missing at " + paths.StateFile + "; run matty install"}
	}
	return doctorCheck{status: doctorPass, name: "matty-state", detail: "present at " + paths.StateFile}
}

func skillChecks(paths Paths, state State, stateFound bool) []doctorCheck {
	if !stateFound {
		return []doctorCheck{{status: doctorWarn, name: "skill-symlinks", detail: "state is missing, so Matty-owned skill links are unknown; run matty install"}}
	}
	if len(state.ManagedSkills) == 0 {
		return []doctorCheck{{status: doctorWarn, name: "skill-symlinks", detail: zeroManagedSkillsDetail(paths)}}
	}
	var missing, changed []string
	for _, skill := range state.ManagedSkills {
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
		return []doctorCheck{{status: doctorPass, name: "skill-symlinks", detail: fmt.Sprintf("%d managed links under %s", len(state.ManagedSkills), paths.AgentSkillsDir)}}
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
	plan, err := BuildInstallPlan(paths, time.Now(), true)
	if err != nil {
		return detail + "; could not inspect expected skill links: " + err.Error()
	}
	summary, ok := unmanagedSymlinkSkipSummary(plan)
	if !ok {
		return detail
	}
	return fmt.Sprintf("state has no managed skills, but %d expected skill symlinks are unmanaged by current Matty state; setup may be incomplete. Example: %s -> %s. %s", summary.count, summary.example.Path, summary.example.Target, unmanagedSymlinkRecoveryAdvice())
}

func engramChecks(runner Runner, paths Paths, state State, stateFound bool) []doctorCheck {
	checks := engramBinaryChecks(runner, paths)
	canonical := engrambin.DiscoverHomebrew(paths.HomebrewPrefixEnv)
	checks = append(checks, engramRuntimeChecks(canonical, pathEngramExecutable(runner, canonical))...)
	if !stateFound {
		checks = append(checks, doctorCheck{status: doctorWarn, name: "engram-setup", detail: "state is missing, so delegated setup cannot be confirmed; run matty install"})
		return checks
	}
	if hasSurface(state, "codex") && hasSurface(state, "opencode") {
		checks = append(checks, doctorCheck{status: doctorPass, name: "engram-setup", detail: "state records Codex and OpenCode setup expectations; run matty update if Engram setup drifted"})
	} else {
		checks = append(checks, doctorCheck{status: doctorFail, name: "engram-setup", detail: "state does not record both Codex and OpenCode setup expectations; run matty update"})
	}
	return checks
}

func engramBinaryChecks(runner Runner, paths Paths) []doctorCheck {
	return engramBinaryChecksWithHomebrewPrefixes(runner, paths.PathEnv, paths.LocalBinEngram, engrambin.HomebrewPrefixes(paths.HomebrewPrefixEnv))
}

func engramBinaryChecksWithHomebrewPrefixes(runner Runner, pathEnv, localBinEngram string, homebrewPrefixes []string) []doctorCheck {
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
		version, versionErr := engrambin.Version(path)
		executables = append(executables, engrambin.NewExecutable(path, canonical, version, versionErr))
	}
	pathEngram := executables[0]
	checks := []doctorCheck{engramPathCheck(pathEngram, canonical, homebrewPrefixes)}
	checks = append(checks, engramVersionMismatchChecks(executables)...)
	if shadowing := engramHomebrewShadowingCheck(executables); shadowing != nil {
		checks = append(checks, *shadowing)
	}
	checks = append(checks, engramLocalBinChecks(localBinEngram, canonical)...)
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

func engramRuntimeChecks(canonical *engrambin.Canonical, pathEngram *engrambin.Executable) []doctorCheck {
	processes, err := engrambin.FindServeProcesses()
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

func hasSurface(state State, want string) bool {
	for _, surface := range state.ConfiguredSurfaces {
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
