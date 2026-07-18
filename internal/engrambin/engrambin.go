package engrambin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

const Formula = "gentleman-programming/tap/engram"

const versionTimeout = 2 * time.Second

type Identity struct {
	Path         string
	ResolvedPath string
}

type Canonical struct {
	Path         string
	ResolvedPath string
}

type Executable struct {
	Path         string
	ResolvedPath string
	Version      string
	VersionErr   error
	Canonical    bool
}

// Topology owns the ordered Homebrew candidates and their observation for one
// captured workstation invocation.
type Topology struct {
	prefixes []string
}

type Observation struct {
	homebrew  *Canonical
	installed bool
}

func NewTopology(homebrewPrefix string) Topology {
	return Topology{prefixes: HomebrewPrefixes(homebrewPrefix)}
}

func (t Topology) Candidates() []string {
	return HomebrewCandidatePaths(t.prefixes)
}

func (t Topology) ExpectedPath() string {
	return ExpectedHomebrewPathFromPrefixes(t.prefixes)
}

func (t Topology) Observe(lookPath func(string) (string, error)) Observation {
	homebrew := DiscoverHomebrewFromPrefixes(t.prefixes)
	if homebrew != nil {
		return Observation{homebrew: homebrew, installed: true}
	}
	if lookPath != nil {
		resolved, err := lookPath("engram")
		if err == nil && IsExpectedHomebrewPath(resolved, t.ExpectedPath()) {
			return Observation{installed: true}
		}
	}
	return Observation{}
}

func (o Observation) Homebrew() *Canonical {
	if o.homebrew == nil {
		return nil
	}
	copy := *o.homebrew
	return &copy
}

func (o Observation) Installed() bool { return o.installed }

// Resolver adapts the existing Homebrew/path identity checks to the
// capability-pack executable-resolution seam. It never executes the tool;
// callers that need a version or runtime observation use a separate seam.
type Resolver struct {
	HomebrewPrefixEnv string
	LookPath          func(string) (string, error)
}

func NewResolver(homebrewPrefixEnv string, lookPath func(string) (string, error)) Resolver {
	return Resolver{HomebrewPrefixEnv: homebrewPrefixEnv, LookPath: lookPath}
}

func (r Resolver) Resolve(_ context.Context, tool string) (capabilitypack.ExecutableResolution, error) {
	if tool != "engram" {
		return capabilitypack.ExecutableResolution{}, fmt.Errorf("unsupported executable requirement %q", tool)
	}
	acquisitionSupported := strings.TrimSpace(r.HomebrewPrefixEnv) != ""
	acquisitionCommand := ""
	var acquisitionArgs []string
	if acquisitionSupported {
		acquisitionCommand = "brew"
		acquisitionArgs = []string{"install", Formula}
	}
	canonical := DiscoverHomebrew(r.HomebrewPrefixEnv)
	if canonical != nil {
		return capabilitypack.ExecutableResolution{
			Tool:                 tool,
			Available:            true,
			Path:                 canonical.Path,
			ResolvedPath:         canonical.ResolvedPath,
			Origin:               "homebrew",
			AcquisitionSupported: acquisitionSupported,
			AcquisitionCommand:   acquisitionCommand,
			AcquisitionArgs:      acquisitionArgs,
			Precondition:         executablePrecondition(canonical.Path, canonical.ResolvedPath),
		}, nil
	}

	path := ""
	if r.LookPath != nil {
		resolved, err := r.LookPath(tool)
		if err == nil {
			path = resolved
		}
	}
	expected := ExpectedHomebrewPath(r.HomebrewPrefixEnv)
	if path != "" && IsExpectedHomebrewPath(path, expected) {
		identity := NewIdentity(path)
		return capabilitypack.ExecutableResolution{
			Tool:                 tool,
			Available:            true,
			Path:                 path,
			ResolvedPath:         identity.ResolvedPath,
			Origin:               "homebrew",
			AcquisitionSupported: acquisitionSupported,
			AcquisitionCommand:   acquisitionCommand,
			AcquisitionArgs:      acquisitionArgs,
			Precondition:         executablePrecondition(identity.Path, identity.ResolvedPath),
		}, nil
	}

	return capabilitypack.ExecutableResolution{
		Tool:                 tool,
		Available:            false,
		Path:                 expected,
		Origin:               "homebrew",
		AcquisitionSupported: acquisitionSupported,
		AcquisitionCommand:   acquisitionCommand,
		AcquisitionArgs:      acquisitionArgs,
		Precondition:         "missing|" + expected,
	}, nil
}

func executablePrecondition(path, resolved string) string {
	info, err := os.Stat(path)
	if err != nil {
		return path + "|" + resolved + "|unstatable:" + err.Error()
	}
	return fmt.Sprintf("%s|%s|%d|%d|%o", path, resolved, info.Size(), info.ModTime().UnixNano(), info.Mode().Perm())
}

type Process struct {
	PID            int
	ExecutablePath string
	Command        string
}

type LocalBinDiagnosis struct {
	OK     bool
	Detail string
}

type VersionDiagnosis struct {
	Detail string
}

type RuntimeDiagnosis struct {
	Process     Process
	Problems    []string
	Remediation string
}

func (diagnosis RuntimeDiagnosis) OK() bool { return len(diagnosis.Problems) == 0 }

// Facts supplies the two subprocess-backed observations used by doctor.
// Keeping them separate from PATH lookup preserves version and runtime evidence
// as independent facts while allowing callers to isolate workstation state.
type Facts struct {
	Version        func(string) (string, error)
	ServeProcesses func() ([]Process, error)
}

func SystemFacts() Facts {
	return Facts{Version: Version, ServeProcesses: FindServeProcesses}
}

func (facts Facts) WithDefaults() Facts {
	defaults := SystemFacts()
	if facts.Version == nil {
		facts.Version = defaults.Version
	}
	if facts.ServeProcesses == nil {
		facts.ServeProcesses = defaults.ServeProcesses
	}
	return facts
}

func HomebrewPrefixes(prefixEnv string) []string {
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
	if prefixEnv == "" {
		add("/opt/homebrew")
		add("/usr/local")
	}
	return prefixes
}

func DiscoverHomebrew(prefixEnv string) *Canonical {
	return DiscoverHomebrewFromPrefixes(HomebrewPrefixes(prefixEnv))
}

func DiscoverHomebrewFromPrefixes(prefixes []string) *Canonical {
	for _, candidate := range HomebrewCandidatePaths(prefixes) {
		if IsExecutable(candidate) {
			return NewCanonical(candidate)
		}
	}
	return nil
}

func HomebrewCandidatePaths(prefixes []string) []string {
	candidates := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		candidates = append(candidates, filepath.Join(prefix, "bin", "engram"))
	}
	return candidates
}

func ExpectedHomebrewPath(prefixEnv string) string {
	return ExpectedHomebrewPathFromPrefixes(HomebrewPrefixes(prefixEnv))
}

func ExpectedHomebrewPathFromPrefixes(prefixes []string) string {
	candidates := HomebrewCandidatePaths(prefixes)
	for _, candidate := range candidates {
		if IsExecutable(candidate) {
			return candidate
		}
	}
	if len(candidates) == 0 {
		return filepath.Join("/opt/homebrew", "bin", "engram")
	}
	return candidates[0]
}

func UniquePaths(resolved, pathEnv string, homebrewPrefixes []string) []string {
	identities := []Identity{}
	paths := []string{}
	add := func(path string) {
		if path == "" {
			return
		}
		identity := NewIdentity(path)
		for _, seen := range identities {
			if identity.Matches(seen) {
				return
			}
		}
		identities = append(identities, identity)
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
	if IsExecutable(path) {
		add(path)
	}
}

func IsExecutable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func NewCanonical(path string) *Canonical {
	identity := NewIdentity(path)
	return &Canonical{Path: identity.Path, ResolvedPath: identity.ResolvedPath}
}

func NewExecutable(path string, canonical *Canonical, version string, versionErr error) Executable {
	identity := NewIdentity(path)
	executable := Executable{Path: identity.Path, ResolvedPath: identity.ResolvedPath, Version: version, VersionErr: versionErr}
	executable.Canonical = identity.MatchesCanonical(canonical)
	return executable
}

func NewIdentity(path string) Identity {
	return Identity{Path: filepath.Clean(path), ResolvedPath: ResolvedPath(path)}
}

func IdentityFrom(path, resolved string) Identity {
	return Identity{Path: filepath.Clean(path), ResolvedPath: filepath.Clean(resolved)}
}

func ResolvedPath(path string) string {
	cleaned := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return cleaned
	}
	return filepath.Clean(resolved)
}

func MatchCanonical(path, resolved string, canonical *Canonical) bool {
	return IdentityFrom(path, resolved).MatchesCanonical(canonical)
}

func (identity Identity) MatchesCanonical(canonical *Canonical) bool {
	if canonical == nil {
		return false
	}
	return identity.Matches(Identity{Path: canonical.Path, ResolvedPath: canonical.ResolvedPath})
}

func (identity Identity) Matches(other Identity) bool {
	return identity.Path == other.Path || identity.Path == other.ResolvedPath || identity.ResolvedPath == other.Path || identity.ResolvedPath == other.ResolvedPath
}

func IsExpectedHomebrewPath(path, expected string) bool {
	return MatchCanonical(path, ResolvedPath(path), NewCanonical(expected))
}

func SameExecutable(path, resolved string, executable Executable) bool {
	return IdentityFrom(path, resolved).SameExecutable(executable)
}

func (identity Identity) SameExecutable(executable Executable) bool {
	return identity.Matches(Identity{Path: executable.Path, ResolvedPath: executable.ResolvedPath})
}

func Version(path string) (string, error) {
	return version(path, versionCommandOutput)
}

func version(path string, output func(context.Context, string) ([]byte, error)) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	out, err := output(ctx, path)
	if err != nil {
		return "", err
	}
	return CleanVersion(string(out)), nil
}

func versionWithContext(ctx context.Context, path string) (string, error) {
	out, err := versionCommandOutput(ctx, path)
	if err != nil {
		return "", err
	}
	return CleanVersion(string(out)), nil
}

func versionCommandOutput(ctx context.Context, path string) ([]byte, error) {
	return exec.CommandContext(ctx, path, "--version").CombinedOutput()
}

func CleanVersion(output string) string {
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

func Detail(executable Executable) string {
	if executable.Version != "" {
		return fmt.Sprintf("%s version %s", executable.Path, executable.Version)
	}
	if executable.VersionErr != nil {
		return fmt.Sprintf("%s (version unavailable: %v)", executable.Path, executable.VersionErr)
	}
	return executable.Path + " (version empty)"
}

func DiagnoseVersion(executable Executable) *VersionDiagnosis {
	switch {
	case executable.VersionErr != nil:
		return &VersionDiagnosis{Detail: fmt.Sprintf("could not inspect %s version: %v", executable.Path, executable.VersionErr)}
	case executable.Version == "":
		return &VersionDiagnosis{Detail: executable.Path + " returned an empty version"}
	default:
		return nil
	}
}

func FindServeProcesses() ([]Process, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return findServeProcesses(ctx, commandOutput, processExecutablePath)
}

func findServeProcesses(ctx context.Context, output func(context.Context, string, ...string) ([]byte, error), resolveExecutable func(int) string) ([]Process, error) {
	out, err := output(ctx, "ps", "-axo", "pid=,args=")
	if err != nil {
		return nil, err
	}
	return ParseServeProcessesWithResolver(string(out), resolveExecutable), nil
}

func commandOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func ParseServeProcesses(output string) []Process {
	return ParseServeProcessesWithResolver(output, processExecutablePath)
}

func ParseServeProcessesWithResolver(output string, resolveExecutable func(int) string) []Process {
	var processes []Process
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		executable := fields[1]
		command := strings.Join(fields[1:], " ")
		if filepath.Base(executable) != "engram" || !commandHasServeArg(command) {
			continue
		}
		if resolvedExecutable := resolveExecutable(pid); resolvedExecutable != "" {
			executable = resolvedExecutable
		}
		processes = append(processes, Process{PID: pid, ExecutablePath: filepath.Clean(executable), Command: command})
	}
	return processes
}

func processExecutablePath(pid int) string {
	for _, lsof := range []string{"lsof", "/usr/sbin/lsof"} {
		path := processExecutablePathWithLsof(pid, lsof)
		if path != "" {
			return path
		}
	}
	return ""
}

func processExecutablePathWithLsof(pid int, lsof string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, lsof, "-a", "-p", strconv.Itoa(pid), "-d", "txt", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		path := strings.TrimPrefix(line, "n")
		if filepath.Base(path) == "engram" {
			return path
		}
	}
	return ""
}

func DiagnoseLocalBin(localBinEngram string, canonical *Canonical) []LocalBinDiagnosis {
	if localBinEngram == "" {
		return nil
	}
	info, err := os.Lstat(localBinEngram)
	if err != nil {
		if os.IsNotExist(err) {
			return []LocalBinDiagnosis{{OK: true, Detail: "no ~/.local/bin/engram compatibility symlink is present; Homebrew remains the Engram owner"}}
		}
		return []LocalBinDiagnosis{{Detail: fmt.Sprintf("cannot inspect %s: %v", localBinEngram, err)}}
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return []LocalBinDiagnosis{{Detail: localBinEngram + " exists but is not a symlink; Packy will not install a second Engram binary there"}}
	}
	target, err := os.Readlink(localBinEngram)
	if err != nil {
		return []LocalBinDiagnosis{{Detail: fmt.Sprintf("cannot read symlink %s: %v", localBinEngram, err)}}
	}
	targetPath := target
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(filepath.Dir(localBinEngram), targetPath)
	}
	if canonical == nil {
		return []LocalBinDiagnosis{{Detail: fmt.Sprintf("%s is a symlink to %s, but no Homebrew Engram was found to verify ownership", localBinEngram, target)}}
	}
	if NewIdentity(targetPath).MatchesCanonical(canonical) {
		return []LocalBinDiagnosis{{OK: true, Detail: fmt.Sprintf("%s -> %s points to Homebrew Engram at %s", localBinEngram, target, canonical.Path)}}
	}
	return []LocalBinDiagnosis{{Detail: fmt.Sprintf("%s -> %s does not point to Homebrew Engram at %s; replace it with a symlink if PATH compatibility is needed", localBinEngram, target, canonical.Path)}}
}

func DiagnoseRuntimeProcess(process Process, canonical *Canonical, pathEngram *Executable) RuntimeDiagnosis {
	identity := NewIdentity(process.ExecutablePath)
	var problems []string
	if pathEngram != nil && !identity.SameExecutable(*pathEngram) {
		problems = append(problems, fmt.Sprintf("different from PATH Engram %s", pathEngram.Path))
	}
	if canonical == nil {
		problems = append(problems, "no Homebrew Engram found to verify canonical ownership")
	} else if !identity.MatchesCanonical(canonical) {
		problems = append(problems, "does not match canonical Homebrew Engram "+canonical.Path)
	}
	return RuntimeDiagnosis{Process: process, Problems: problems, Remediation: RuntimeRemediation(canonical)}
}

func RuntimeRemediation(canonical *Canonical) string {
	if canonical == nil {
		return "safe remediation: pkill -f 'engram serve' && packy update"
	}
	return fmt.Sprintf("safe remediation: pkill -f 'engram serve' && %s serve", canonical.Path)
}

func commandHasServeArg(command string) bool {
	fields := strings.Fields(command)
	return len(fields) >= 2 && fields[1] == "serve"
}
