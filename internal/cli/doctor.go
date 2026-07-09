package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	checks = append(checks, engramChecks(runner, paths.PathEnv, paths.HomebrewPrefixEnv, state, stateFound)...)
	checks = append(checks, codexChecks(paths)...)
	openCodeChecks, err := openCodeChecks(paths)
	if err != nil {
		checks = append(checks, doctorCheck{status: doctorFail, name: "opencode-config", detail: err.Error() + "; inspect the config or run matty install"})
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

func engramChecks(runner Runner, pathEnv, homebrewPrefixEnv string, state State, stateFound bool) []doctorCheck {
	checks := engramBinaryChecks(runner, pathEnv, homebrewPrefixEnv)
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

type engramExecutable struct {
	path       string
	version    string
	versionErr error
}

func engramBinaryChecks(runner Runner, pathEnv, homebrewPrefixEnv string) []doctorCheck {
	return engramBinaryChecksWithHomebrewPrefixes(runner, pathEnv, homebrewPrefixes(homebrewPrefixEnv))
}

func engramBinaryChecksWithHomebrewPrefixes(runner Runner, pathEnv string, homebrewPrefixes []string) []doctorCheck {
	resolved, err := runner.LookPath("engram")
	if err != nil {
		return []doctorCheck{{status: doctorFail, name: "engram-binary", detail: "engram is not available; run matty install"}}
	}

	paths := uniqueEngramPaths(resolved, pathEnv, homebrewPrefixes)
	executables := make([]engramExecutable, 0, len(paths))
	for _, path := range paths {
		version, versionErr := engramVersion(path)
		executables = append(executables, engramExecutable{path: path, version: version, versionErr: versionErr})
	}
	checks := []doctorCheck{{status: doctorPass, name: "engram-binary", detail: engramBinaryDetail(executables[0])}}
	checks = append(checks, engramVersionMismatchChecks(executables)...)
	if shadowing := engramHomebrewShadowingCheck(executables); shadowing != nil {
		checks = append(checks, *shadowing)
	}
	return checks
}

func uniqueEngramPaths(resolved, pathEnv string, homebrewPrefixes []string) []string {
	seen := map[string]bool{}
	paths := []string{}
	add := func(path string) {
		if path == "" {
			return
		}
		key := filepath.Clean(path)
		if seen[key] {
			return
		}
		seen[key] = true
		paths = append(paths, path)
	}

	add(resolved)
	for _, dir := range filepath.SplitList(pathEnv) {
		addIfExecutable(filepath.Join(dir, "engram"), add)
	}
	for _, prefix := range homebrewPrefixes {
		addIfExecutable(filepath.Join(prefix, "bin", "engram"), add)
	}
	return paths
}

func addIfExecutable(path string, add func(string)) {
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return
	}
	add(path)
}

func homebrewPrefixes(prefixEnv string) []string {
	prefixes := []string{}
	seen := map[string]bool{}
	add := func(prefix string) {
		if prefix == "" {
			return
		}
		key := filepath.Clean(prefix)
		if seen[key] {
			return
		}
		seen[key] = true
		prefixes = append(prefixes, prefix)
	}
	add(prefixEnv)
	add("/opt/homebrew")
	add("/usr/local")
	return prefixes
}

func engramVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	return cleanEngramVersion(string(out)), nil
}

func cleanEngramVersion(output string) string {
	firstLine, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	version := strings.TrimSpace(firstLine)
	lower := strings.ToLower(version)
	for _, prefix := range []string{"engram version ", "engram ", "version "} {
		if strings.HasPrefix(lower, prefix) {
			version = strings.TrimSpace(version[len(prefix):])
			break
		}
	}
	return strings.TrimPrefix(version, "v")
}

func engramBinaryDetail(executable engramExecutable) string {
	if executable.version != "" {
		return fmt.Sprintf("%s version %s", executable.path, executable.version)
	}
	if executable.versionErr != nil {
		return fmt.Sprintf("%s (version unavailable: %v)", executable.path, executable.versionErr)
	}
	return executable.path + " (version empty)"
}

func engramVersionMismatchChecks(executables []engramExecutable) []doctorCheck {
	versionByPath := []string{}
	versions := map[string]bool{}
	for _, executable := range executables {
		if executable.version == "" {
			continue
		}
		versions[executable.version] = true
		versionByPath = append(versionByPath, fmt.Sprintf("%s version %s", executable.path, executable.version))
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

func engramHomebrewShadowingCheck(executables []engramExecutable) *doctorCheck {
	if len(executables) < 2 {
		return nil
	}
	resolved := executables[0]
	for _, executable := range executables[1:] {
		if !isHomebrewEngramPath(executable.path) {
			continue
		}
		detail := fmt.Sprintf("%s appears before Homebrew Engram at %s", resolved.path, executable.path)
		if resolved.version != "" {
			detail += " and reports version " + resolved.version
		}
		if executable.version != "" {
			detail += "; Homebrew reports version " + executable.version
		}
		return &doctorCheck{status: doctorWarn, name: "engram-path-shadowing", detail: detail}
	}
	return nil
}

func isHomebrewEngramPath(path string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	return strings.HasSuffix(cleaned, "/homebrew/bin/engram") ||
		strings.HasSuffix(cleaned, "/usr/local/bin/engram")
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
