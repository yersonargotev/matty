package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yersonargotev/matty/internal/opencode"
	"github.com/yersonargotev/matty/internal/prompt"
)

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
	checks = append(checks, engramChecks(runner, state, stateFound)...)
	checks = append(checks, codexChecks(paths)...)
	openCodeChecks, err := openCodeChecks(paths)
	if err != nil {
		checks = append(checks, doctorCheck{status: doctorFail, name: "opencode", detail: err.Error() + "; inspect the config or run matty install"})
	} else {
		checks = append(checks, openCodeChecks...)
	}

	for _, check := range checks {
		if _, err := fmt.Fprintf(w, "%s %s: %s\n", check.status, check.name, check.detail); err != nil {
			return err
		}
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
		return []doctorCheck{{status: doctorWarn, name: "skill-symlinks", detail: "state has no managed skills; run matty install"}}
	}
	var missing, changed []string
	for _, skill := range state.ManagedSkills {
		info, err := os.Lstat(skill.LinkPath)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, skill.Name)
				continue
			}
			changed = append(changed, fmt.Sprintf("%s (%v)", skill.Name, err))
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			changed = append(changed, skill.Name+" is not a symlink")
			continue
		}
		target, err := os.Readlink(skill.LinkPath)
		if err != nil || !sameSymlinkTarget(skill.LinkPath, target, skill.SourcePath) {
			changed = append(changed, skill.Name)
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

func engramChecks(runner Runner, state State, stateFound bool) []doctorCheck {
	checks := []doctorCheck{}
	if EngramInstalled(runner) {
		checks = append(checks, doctorCheck{status: doctorPass, name: "engram-binary", detail: "engram is available on PATH"})
	} else {
		checks = append(checks, doctorCheck{status: doctorFail, name: "engram-binary", detail: "engram is not available; run matty install"})
	}
	if !stateFound {
		checks = append(checks, doctorCheck{status: doctorWarn, name: "engram-setup", detail: "state is missing, so delegated setup cannot be confirmed; run matty install"})
		return checks
	}
	if hasSurface(state, "codex") && hasSurface(state, "opencode") {
		checks = append(checks, doctorCheck{status: doctorPass, name: "engram-setup", detail: "state records Codex and OpenCode surfaces; run matty update if Engram setup drifted"})
	} else {
		checks = append(checks, doctorCheck{status: doctorWarn, name: "engram-setup", detail: "state does not record both Codex and OpenCode surfaces; run matty update"})
	}
	return checks
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
