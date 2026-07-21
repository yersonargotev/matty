// Package claudesmoke runs the package-installed Packy lifecycle against Claude
// Code without allowing either program to see the operator's workstation state.
package claudesmoke

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/skillbundle"
)

const ExactFloor = "2.1.203"

type Config struct {
	Packy, SourceRepo, SourceRef, ClaudeSelector, EvidencePath, NPM string
}

type CommandEvidence struct {
	Name     string   `json:"name"`
	Args     []string `json:"args"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
}
type FileEvidence struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
}
type SafetyEvidence struct {
	DisposableSandbox               bool `json:"disposable_sandbox"`
	AllowlistEnvironment            bool `json:"allowlist_environment"`
	CredentialsScrubbed             bool `json:"credentials_scrubbed"`
	CommandAllowlist                bool `json:"command_allowlist"`
	CheckoutUnchanged               bool `json:"checkout_unchanged"`
	ConfiguredWritableRootsConfined bool `json:"configured_writable_roots_confined"`
	EvidencePathOutsideSandbox      bool `json:"evidence_path_outside_sandbox"`
	NoInteractiveClaude             bool `json:"no_interactive_claude"`
}
type AssertionEvidence struct {
	ForeignContentPreserved            bool `json:"foreign_content_preserved"`
	InstallCreatedManagedState         bool `json:"install_created_managed_state"`
	InstallCreatedManagedProjections   bool `json:"install_created_managed_projections"`
	InstallProjectedClaudeMCP          bool `json:"install_projected_claude_mcp"`
	DryRunsUnchanged                   bool `json:"dry_runs_unchanged"`
	UninstallRemovedManagedState       bool `json:"uninstall_removed_managed_state"`
	UninstallRemovedManagedProjections bool `json:"uninstall_removed_managed_projections"`
	ResidualManagedArtifactsAbsent     bool `json:"residual_managed_artifacts_absent"`
	EngramStubProtocolVerified         bool `json:"engram_stub_protocol_verified"`
}
type Evidence struct {
	SchemaVersion          int               `json:"schema_version"`
	PackyVersion           string            `json:"packy_version"`
	PackyRef               string            `json:"packy_ref"`
	PackySHA               string            `json:"packy_sha"`
	OS                     string            `json:"os"`
	Arch                   string            `json:"arch"`
	RequestedClaudeVersion string            `json:"requested_claude_version"`
	ResolvedClaudeVersion  string            `json:"resolved_claude_version"`
	ClaudeIntegrity        string            `json:"claude_npm_integrity"`
	ClaudeDigest           string            `json:"claude_executable_sha256"`
	Sandbox                string            `json:"sandbox"`
	Commands               []CommandEvidence `json:"commands"`
	Before                 []FileEvidence    `json:"before"`
	After                  []FileEvidence    `json:"after"`
	Safety                 SafetyEvidence    `json:"safety"`
	Assertions             AssertionEvidence `json:"assertions"`
}

func ResolveSelector(selector, npmOutput string) (version, integrity string, err error) {
	selector = strings.TrimSpace(selector)
	if selector != ExactFloor && selector != "stable" {
		return "", "", fmt.Errorf("Claude selector must be %q or stable", ExactFloor)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(npmOutput), &metadata); err != nil {
		var plain string
		if e := json.Unmarshal([]byte(npmOutput), &plain); e != nil {
			return "", "", fmt.Errorf("parse npm metadata: %w", err)
		}
		metadata = map[string]json.RawMessage{"version": json.RawMessage(strconv.Quote(plain))}
	}
	_ = json.Unmarshal(metadata["version"], &version)
	_ = json.Unmarshal(metadata["dist.integrity"], &integrity)
	if integrity == "" {
		_ = json.Unmarshal(metadata["integrity"], &integrity)
	}
	if version == "" {
		return "", "", errors.New("npm metadata omitted Claude version")
	}
	if selector == ExactFloor && version != ExactFloor {
		return "", "", fmt.Errorf("exact Claude version resolved to %q", version)
	}
	if integrity == "" {
		return "", "", errors.New("npm metadata omitted dist.integrity")
	}
	return version, integrity, nil
}

func Run(ctx context.Context, cfg Config) (Evidence, error) {
	if cfg.Packy == "" || cfg.SourceRepo == "" || cfg.SourceRef == "" || cfg.EvidencePath == "" {
		return Evidence{}, errors.New("packy, source repo/ref, and evidence path are required")
	}
	if cfg.NPM == "" {
		cfg.NPM = "npm"
	}
	npmExecutable, err := exec.LookPath(cfg.NPM)
	if err != nil {
		return Evidence{}, fmt.Errorf("locate npm: %w", err)
	}
	packy, err := filepath.Abs(cfg.Packy)
	if err != nil {
		return Evidence{}, err
	}
	repo, err := filepath.Abs(cfg.SourceRepo)
	if err != nil {
		return Evidence{}, err
	}
	for _, p := range []string{packy, repo} {
		if _, err := os.Stat(p); err != nil {
			return Evidence{}, err
		}
	}
	head, err := hostOutput(ctx, repo, "git", "rev-parse", "HEAD")
	if err != nil {
		return Evidence{}, err
	}
	status, err := hostOutput(ctx, repo, "git", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return Evidence{}, err
	}
	sandbox, err := os.MkdirTemp("", "packy-claude-smoke-")
	if err != nil {
		return Evidence{}, err
	}
	defer os.RemoveAll(sandbox)
	roots := []string{"home", "config", "cache", "data", "tmp", "stub-bin", "homebrew/bin", "npm", "installed-source", "work", "acquisition/home", "acquisition/config", "acquisition/cache", "acquisition/tmp"}
	for _, root := range roots {
		if err := os.MkdirAll(filepath.Join(sandbox, root), 0700); err != nil {
			return Evidence{}, err
		}
	}
	userConfig := filepath.Join(sandbox, "acquisition", "npmrc")
	if err := os.WriteFile(userConfig, nil, 0600); err != nil {
		return Evidence{}, err
	}
	acquireEnv := acquisitionEnv(sandbox, npmExecutable)
	meta, err := hostOutputEnv(ctx, "", acquireEnv, npmExecutable, "view", "@anthropic-ai/claude-code@"+cfg.ClaudeSelector, "version", "dist.integrity", "--json")
	if err != nil {
		return Evidence{}, err
	}
	resolved, integrity, err := ResolveSelector(cfg.ClaudeSelector, meta)
	if err != nil {
		return Evidence{}, err
	}
	installRepo, installRef, sourceSHA, err := prepareInstallableSource(ctx, repo, cfg.SourceRef, filepath.Join(sandbox, "source-repository"))
	if err != nil {
		return Evidence{}, err
	}
	install := exec.CommandContext(ctx, npmExecutable, "install", "--prefix", filepath.Join(sandbox, "npm"), "--no-audit", "--no-fund", "@anthropic-ai/claude-code@"+resolved)
	install.Env = acquireEnv
	var installOut bytes.Buffer
	install.Stdout = &installOut
	install.Stderr = &installOut
	if err := install.Run(); err != nil {
		return Evidence{}, fmt.Errorf("install Claude: %w: %s", err, installOut.String())
	}
	claude := filepath.Join(sandbox, "npm", "node_modules", ".bin", "claude")
	digest, err := fileDigest(claude)
	if err != nil {
		return Evidence{}, fmt.Errorf("digest Claude: %w", err)
	}
	engramStub := `#!/bin/sh
case "${1-}" in
  setup) exit 0 ;;
  mcp)
    [ "${2-}" = "--tools=agent" ] || exit 64
    while IFS= read -r request; do
      case "$request" in
        *'"method":"initialize"'*) printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"engram-inert","version":"1"}}}' ;;
        *'"method":"tools/list"'*) printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}' ;;
        *'"method":"tools/call"'*) printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"content":[],"isError":false}}' ;;
      esac
    done ;;
  *) exit 64 ;;
esac
`
	if err := writeStub(filepath.Join(sandbox, "stub-bin", "engram"), engramStub); err != nil {
		return Evidence{}, err
	}
	if err := writeStub(filepath.Join(sandbox, "homebrew", "bin", "engram"), engramStub); err != nil {
		return Evidence{}, err
	}
	if err := writeStub(filepath.Join(sandbox, "stub-bin", "brew"), "#!/bin/sh\nexit 0\n"); err != nil {
		return Evidence{}, err
	}
	claudeLog := filepath.Join(sandbox, "claude-invocations.log")
	claudeInterposer := filepath.Join(sandbox, "stub-bin", "claude")
	if err := createClaudeInterposer(claudeInterposer, claude, claudeLog); err != nil {
		return Evidence{}, err
	}
	env := restrictedEnv(sandbox, filepath.Dir(claude), filepath.Dir(npmExecutable))
	foreignInstructionPath := filepath.Join(sandbox, "home", ".claude", "CLAUDE.md")
	foreignInstruction := []byte("FOREIGN-BYTE-EXACT-INSTRUCTION\n")
	foreignMCPPath := filepath.Join(sandbox, "home", ".claude.json")
	foreignMCPMarker := "FOREIGN-BYTE-EXACT-MCP"
	foreignMCP := []byte("{\"mcpServers\":{\"foreign\":{\"type\":\"stdio\",\"command\":\"/bin/echo\",\"args\":[\"FOREIGN-BYTE-EXACT-MCP\"]}}}\n")
	if err := os.MkdirAll(filepath.Dir(foreignInstructionPath), 0700); err != nil {
		return Evidence{}, err
	}
	if err := os.WriteFile(foreignInstructionPath, foreignInstruction, 0600); err != nil {
		return Evidence{}, err
	}
	if err := os.WriteFile(foreignMCPPath, foreignMCP, 0600); err != nil {
		return Evidence{}, err
	}
	engramProbe, probeErr := probeEngramStub(ctx, filepath.Join(sandbox, "stub-bin", "engram"), env)
	before, err := Manifest(sandbox)
	if err != nil {
		return Evidence{}, err
	}
	e := Evidence{SchemaVersion: 1, PackyRef: cfg.SourceRef, PackySHA: sourceSHA, OS: runtime.GOOS, Arch: runtime.GOARCH, RequestedClaudeVersion: cfg.ClaudeSelector, ResolvedClaudeVersion: resolved, ClaudeIntegrity: integrity, ClaudeDigest: digest, Sandbox: sandbox, Before: before}
	e.Assertions.EngramStubProtocolVerified = probeErr == nil && engramProbe
	e.Safety = SafetyEvidence{DisposableSandbox: true, AllowlistEnvironment: true, CredentialsScrubbed: true, CommandAllowlist: true, NoInteractiveClaude: true}
	commands := [][]string{
		{packy, "version"},
		{packy, "init", "--home", filepath.Join(sandbox, "home"), "--source-root", filepath.Join(sandbox, "installed-source"), "--repository-url", installRepo, "--repository-ref", installRef},
		{packy, "install", "--dry-run"}, {packy, "install"}, {packy, "doctor"},
		{packy, "update", "--dry-run"}, {packy, "update"}, {packy, "uninstall", "--dry-run"}, {packy, "uninstall"}, {packy, "doctor"},
	}
	// Claude version is the only direct Claude invocation. Packy's user-scoped
	// MCP calls are constrained by PATH/HOME and captured as part of its command.
	commands = append([][]string{{claudeInterposer, "--version"}}, commands...)
	var dryBefore []FileEvidence
	for index, argv := range commands {
		if index == 3 || index == 6 || index == 8 {
			dryBefore, _ = Manifest(filepath.Join(sandbox, "home"))
		}
		ce := runAllowed(ctx, filepath.Join(sandbox, "work"), env, packy, claudeInterposer, argv)
		e.Commands = append(e.Commands, ce)
		if ce.ExitCode != 0 {
			e.Commands = append(e.Commands, readClaudeInvocations(claudeLog)...)
			e.After, _ = Manifest(sandbox)
			_ = writeEvidence(cfg.EvidencePath, e)
			return e, fmt.Errorf("%s exited %d", ce.Name, ce.ExitCode)
		}
		if index == 3 || index == 6 || index == 8 {
			dryAfter, _ := Manifest(filepath.Join(sandbox, "home"))
			e.Assertions.DryRunsUnchanged = (index == 3 || e.Assertions.DryRunsUnchanged) && reflect.DeepEqual(dryBefore, dryAfter)
		}
		if index == 4 {
			_, stateErr := os.Stat(filepath.Join(sandbox, "home", ".packy", "config.json"))
			e.Assertions.InstallCreatedManagedState = stateErr == nil
			e.Assertions.InstallCreatedManagedProjections = classicSkillTopologyExact(filepath.Join(sandbox, "home"), filepath.Join(sandbox, "installed-source", "bundle", "skills")) && fileContains(filepath.Join(sandbox, "home", ".claude", "CLAUDE.md"), []byte("<!-- packy:"))
			e.Assertions.InstallProjectedClaudeMCP = containsClaudeOperation(claudeLog, "mcp-add")
			installedInstruction, _ := os.ReadFile(foreignInstructionPath)
			installedMCP, _ := os.ReadFile(foreignMCPPath)
			e.Assertions.ForeignContentPreserved = bytes.Contains(installedInstruction, foreignInstruction) && bytes.Contains(installedMCP, []byte(foreignMCPMarker))
		}
		if index == 9 {
			_, stateErr := os.Stat(filepath.Join(sandbox, "home", ".packy", "config.json"))
			e.Assertions.UninstallRemovedManagedState = os.IsNotExist(stateErr)
			e.Assertions.UninstallRemovedManagedProjections = !hasEntries(filepath.Join(sandbox, "home", ".agents", "skills")) && bytes.Equal(mustReadFile(foreignInstructionPath), foreignInstruction) && !bytes.Contains(mustReadFile(foreignMCPPath), []byte(`"engram"`)) && containsClaudeOperation(claudeLog, "mcp-remove")
		}
	}
	gotInstruction, _ := os.ReadFile(foreignInstructionPath)
	gotMCP, _ := os.ReadFile(foreignMCPPath)
	e.Assertions.ForeignContentPreserved = e.Assertions.ForeignContentPreserved && bytes.Equal(gotInstruction, foreignInstruction) && bytes.Contains(gotMCP, []byte(foreignMCPMarker))
	e.Assertions.ResidualManagedArtifactsAbsent = !fileExists(filepath.Join(sandbox, "home", ".packy", "config.json")) && !hasEntries(filepath.Join(sandbox, "home", ".agents", "skills")) && !bytes.Contains(gotMCP, []byte(`"engram"`))
	e.Commands = append(e.Commands, readClaudeInvocations(claudeLog)...)
	e.PackyVersion = parsePackyVersion(e.Commands[1].Stdout)
	afterStatus, err := hostOutput(ctx, repo, "git", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return e, err
	}
	afterHead, err := hostOutput(ctx, repo, "git", "rev-parse", "HEAD")
	if err != nil {
		return e, err
	}
	e.Safety.CheckoutUnchanged = status == afterStatus && strings.TrimSpace(head) == strings.TrimSpace(afterHead)
	e.Safety.ConfiguredWritableRootsConfined = configuredRootsConfined(sandbox)
	e.Safety.EvidencePathOutsideSandbox = e.Safety.ConfiguredWritableRootsConfined && e.Safety.CheckoutUnchanged && !pathWithin(sandbox, cfg.EvidencePath)
	e.After, err = Manifest(sandbox)
	if err != nil {
		return e, err
	}
	if err := validateAndWriteEvidence(cfg.EvidencePath, e); err != nil {
		return e, err
	}
	return e, nil
}

func runAllowed(ctx context.Context, dir string, env []string, packy, claude string, argv []string) CommandEvidence {
	ce := CommandEvidence{Name: filepath.Base(argv[0]), Args: append([]string(nil), argv[1:]...), ExitCode: -1}
	if !AllowedCommand(packy, claude, argv) {
		ce.Stderr = "forbidden command"
		return ce
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	ce.Stdout = out.String()
	ce.Stderr = stderr.String()
	if err == nil {
		ce.ExitCode = 0
	} else if x, ok := err.(*exec.ExitError); ok {
		ce.ExitCode = x.ExitCode()
	}
	return ce
}

func AllowedCommand(packy, claude string, argv []string) bool {
	if len(argv) < 2 {
		return false
	}
	if argv[0] == claude {
		return len(argv) == 2 && argv[1] == "--version"
	}
	if argv[0] != packy {
		return false
	}
	switch argv[1] {
	case "version", "doctor":
		return len(argv) == 2
	case "init":
		return len(argv) == 10
	case "install", "update", "uninstall":
		return len(argv) == 2 || (len(argv) == 3 && argv[2] == "--dry-run")
	}
	return false
}

func restrictedEnv(root, claudeBin string, runtimeBin ...string) []string {
	pathEntries := []string{filepath.Join(root, "stub-bin"), claudeBin}
	pathEntries = append(pathEntries, runtimeBin...)
	pathEntries = append(pathEntries, "/usr/bin", "/bin")
	return []string{
		"HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "CLAUDE_CONFIG_DIR=" + filepath.Join(root, "home"),
		"XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "TMPDIR=" + filepath.Join(root, "tmp"),
		"PATH=" + strings.Join(pathEntries, string(os.PathListSeparator)), "LANG=C", "LC_ALL=C", "NO_COLOR=1",
		"HOMEBREW_PREFIX=" + filepath.Join(root, "homebrew"),
		"PACKY_SKILLS_SOURCE=" + filepath.Join(root, "installed-source", "bundle", "skills"),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTOUPDATER=1",
	}
}
func RestrictedEnv(root, claudeBin string) []string { return restrictedEnv(root, claudeBin) }
func acquisitionEnv(root, npmExecutable string) []string {
	out := []string{
		"HOME=" + filepath.Join(root, "acquisition", "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "acquisition", "config"),
		"TMPDIR=" + filepath.Join(root, "acquisition", "tmp"),
		"NPM_CONFIG_CACHE=" + filepath.Join(root, "acquisition", "cache"),
		"NPM_CONFIG_USERCONFIG=" + filepath.Join(root, "acquisition", "npmrc"),
		"PATH=" + filepath.Dir(npmExecutable) + string(os.PathListSeparator) + "/usr/bin:/bin",
	}
	for _, k := range []string{"SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func Manifest(root string) ([]FileEvidence, error) {
	var out []FileEvidence
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		item := FileEvidence{Path: filepath.ToSlash(rel), Mode: uint32(info.Mode()), Size: info.Size()}
		if info.Mode().IsRegular() {
			item.SHA256, _ = fileDigest(path)
		}
		out = append(out, item)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

func configuredRootsConfined(sandbox string) bool {
	for _, relative := range []string{"home", "config", "cache", "data", "tmp", "stub-bin", "homebrew", "npm", "installed-source", "work", "source-repository", "acquisition"} {
		path := filepath.Join(sandbox, relative)
		rel, err := filepath.Rel(sandbox, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return false
		}
	}
	return true
}
func pathWithin(root, path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func ValidateEvidence(e Evidence) error {
	if e.SchemaVersion != 1 || e.PackyVersion == "" || e.PackyRef == "" || len(e.PackySHA) != 40 || e.ResolvedClaudeVersion == "" || e.ClaudeIntegrity == "" || len(e.ClaudeDigest) != 64 {
		return errors.New("missing or malformed canonical evidence")
	}
	s := e.Safety
	if !s.DisposableSandbox || !s.AllowlistEnvironment || !s.CredentialsScrubbed || !s.CommandAllowlist || !s.CheckoutUnchanged || !s.ConfiguredWritableRootsConfined || !s.EvidencePathOutsideSandbox || !s.NoInteractiveClaude {
		return errors.New("unsafe evidence")
	}
	if len(e.Commands) == 0 {
		return errors.New("evidence has no commands")
	}
	want := [][]string{{"--version"}, {"version"}, {"init"}, {"install", "--dry-run"}, {"install"}, {"doctor"}, {"update", "--dry-run"}, {"update"}, {"uninstall", "--dry-run"}, {"uninstall"}, {"doctor"}}
	if len(e.Commands) < len(want) {
		return errors.New("evidence command sequence is incomplete")
	}
	for i, args := range want {
		got := e.Commands[i].Args
		// init has confined path arguments; its operation is the stable part.
		if i == 2 {
			if len(got) != 9 || got[0] != "init" {
				return errors.New("evidence command sequence is malformed")
			}
			continue
		}
		if !reflect.DeepEqual(got, args) {
			return errors.New("evidence command sequence is malformed")
		}
	}
	for _, c := range e.Commands[len(want):] {
		if c.Name != "claude" || len(c.Args) != 1 || !map[string]bool{"version": true, "mcp-list": true, "mcp-get": true, "mcp-add": true, "mcp-remove": true}[c.Args[0]] {
			return errors.New("unsafe normalized Claude operation")
		}
	}
	if e.RequestedClaudeVersion != ExactFloor && e.RequestedClaudeVersion != "stable" {
		return errors.New("unsafe Claude selector evidence")
	}
	if e.RequestedClaudeVersion == ExactFloor && e.ResolvedClaudeVersion != ExactFloor {
		return errors.New("exact Claude selector mismatch")
	}
	_, digestErr := hex.DecodeString(e.ClaudeDigest)
	if !strings.HasPrefix(e.ClaudeIntegrity, "sha") || digestErr != nil {
		return errors.New("malformed Claude acquisition evidence")
	}
	a := e.Assertions
	if !a.ForeignContentPreserved || !a.InstallCreatedManagedState || !a.InstallCreatedManagedProjections || !a.InstallProjectedClaudeMCP || !a.DryRunsUnchanged || !a.UninstallRemovedManagedState || !a.UninstallRemovedManagedProjections || !a.ResidualManagedArtifactsAbsent || !a.EngramStubProtocolVerified {
		return errors.New("lifecycle assertions are incomplete")
	}
	for _, c := range e.Commands {
		if c.ExitCode != 0 {
			return errors.New("evidence contains failed command")
		}
	}
	raw, _ := json.Marshal(e)
	for _, needle := range []string{"ANTHROPIC_API_KEY", "AWS_SECRET_ACCESS_KEY", "OPENAI_API_KEY"} {
		if strings.Contains(string(raw), needle) {
			return errors.New("evidence contains credential material")
		}
	}
	return nil
}
func fileExists(path string) bool                  { info, err := os.Stat(path); return err == nil && !info.IsDir() }
func fileContains(path string, marker []byte) bool { return bytes.Contains(mustReadFile(path), marker) }
func mustReadFile(path string) []byte              { b, _ := os.ReadFile(path); return b }
func hasEntries(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}
func classicSkillTopologyExact(home, source string) bool {
	linkRoot := filepath.Join(home, ".agents", "skills")
	skills, err := skillbundle.Discover(source, linkRoot, "")
	if err != nil {
		return false
	}
	expected := make(map[string]string, len(skills))
	for _, skill := range skills {
		expected[skill.Name] = skill.SourcePath
	}
	actual, err := os.ReadDir(linkRoot)
	if err != nil {
		return false
	}
	if len(actual) != len(expected) {
		return false
	}
	for _, entry := range actual {
		want, ok := expected[entry.Name()]
		if !ok {
			return false
		}
		target, err := os.Readlink(filepath.Join(home, ".agents", "skills", entry.Name()))
		if err != nil {
			return false
		}
		if filepath.Clean(target) != filepath.Clean(want) {
			return false
		}
	}
	return true
}
func validateAndWriteEvidence(path string, evidence Evidence) error {
	validationErr := ValidateEvidence(evidence)
	writeErr := writeEvidence(path, evidence)
	if validationErr != nil {
		if writeErr != nil {
			return fmt.Errorf("%w; write failed evidence: %v", validationErr, writeErr)
		}
		return validationErr
	}
	return writeErr
}
func containsClaudeOperation(path, operation string) bool {
	for _, c := range readClaudeInvocations(path) {
		if len(c.Args) == 1 && c.Args[0] == operation && c.ExitCode == 0 {
			return true
		}
	}
	return false
}
func probeEngramStub(ctx context.Context, executable string, env []string) (bool, error) {
	cmd := exec.CommandContext(ctx, executable, "mcp", "--tools=agent")
	cmd.Env = env
	cmd.Stdin = strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\n")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return len(lines) == 2 && strings.Contains(lines[0], `"name":"engram-inert"`) && strings.Contains(lines[1], `"tools":[]`), nil
}
func writeEvidence(path string, e Evidence) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0600)
}
func fileDigest(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func writeStub(path, body string) error { return os.WriteFile(path, []byte(body), 0700) }

// createClaudeInterposer makes the command policy independently enforceable for
// Claude calls made inside Packy. The log contains only operation names and exit
// codes, never MCP definitions, command arguments, or environment values.
func createClaudeInterposer(path, realClaude, logPath string) error {
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	body := `#!/bin/sh
set -u
real=` + quote(realClaude) + `
log=` + quote(logPath) + `
op=
case "$#:$1" in
  1:--version) op=version ;;
  *)
    if [ "$1" != mcp ] || [ "$#" -lt 2 ]; then exit 126; fi
    case "$2" in
      list) [ "$#" -eq 2 ] || exit 126; op=mcp-list ;;
      get) [ "$#" -eq 3 ] && [ -n "$3" ] || exit 126; op=mcp-get ;;
      remove) [ "$#" -eq 5 ] && [ -n "$3" ] && [ "$4" = --scope ] && [ "$5" = user ] || exit 126; op=mcp-remove ;;
      add)
        [ "$#" -ge 8 ] && [ -n "$3" ] && [ "$4" = --scope ] && [ "$5" = user ] && [ "$6" = -- ] && [ -n "$7" ] || exit 126
        op=mcp-add ;;
      *) exit 126 ;;
    esac ;;
esac
"$real" "$@"
code=$?
printf '%s|%s\n' "$op" "$code" >> "$log"
exit "$code"
`
	return writeStub(path, body)
}

func readClaudeInvocations(path string) []CommandEvidence {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []CommandEvidence
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}
		code, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		out = append(out, CommandEvidence{Name: "claude", Args: []string{parts[0]}, ExitCode: code})
	}
	return out
}
func hostOutput(ctx context.Context, dir, name string, args ...string) (string, error) {
	return hostOutputEnv(ctx, dir, nil, name, args...)
}
func hostOutputEnv(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w: %s", name, err, out.String())
	}
	return out.String(), nil
}

// prepareInstallableSource leaves the proved checkout untouched while adapting
// arbitrary Git object names (notably CI's full GITHUB_SHA) to bootstrap's
// git-clone --branch contract.
func prepareInstallableSource(ctx context.Context, sourceRepo, requestedRef, destination string) (repository, ref, sha string, err error) {
	resolved, err := hostOutput(ctx, sourceRepo, "git", "rev-parse", "--verify", "--end-of-options", requestedRef+"^{commit}")
	if err != nil {
		return "", "", "", fmt.Errorf("resolve requested source ref %q: %w", requestedRef, err)
	}
	resolved = strings.TrimSpace(resolved)
	if len(resolved) != 40 {
		return "", "", "", fmt.Errorf("requested source ref resolved to malformed SHA %q", resolved)
	}
	if _, err := hostOutput(ctx, "", "git", "clone", "--no-checkout", "--local", sourceRepo, destination); err != nil {
		return "", "", "", fmt.Errorf("create disposable source repository: %w", err)
	}
	const syntheticRef = "packy-smoke-proved-source"
	if _, err := hostOutput(ctx, destination, "git", "branch", "--force", syntheticRef, resolved); err != nil {
		return "", "", "", fmt.Errorf("create installable source ref: %w", err)
	}
	return destination, syntheticRef, resolved, nil
}

func parsePackyVersion(s string) string {
	f := strings.Fields(s)
	if len(f) >= 3 && f[0] == "packy" && f[1] == "version" {
		return f[2]
	}
	return ""
}
