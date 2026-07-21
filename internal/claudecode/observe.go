package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/localprojection"
)

type PathKind string

const (
	PathMissing   PathKind = "missing"
	PathSymlink   PathKind = "symlink"
	PathFile      PathKind = "file"
	PathDirectory PathKind = "directory"
	PathOther     PathKind = "other"
)

type SkillObservation struct {
	Path                                                    string
	Kind                                                    PathKind
	Target, ResolvedTarget, ExpectedSource, TreeFingerprint string
	Err                                                     error
}
type InstructionObservation struct {
	Path, Revision          string
	MarkerCardinality       int
	Contributions           map[string]string
	ForeignContentPreserved bool
	Err                     error
}
type AgentObservation struct {
	Path        string
	Kind        PathKind
	Fingerprint string
	Err         error
}
type HookObservation struct {
	Path, Revision     string
	Parseable          bool
	MatchingEntries    []string
	Disabled, Shadowed bool
	EntryFingerprint   string
	Err                error
}
type MCPObservation struct {
	Name                  string
	Present               bool
	Identity              MCPIdentity
	DefinitionFingerprint string
	Err                   error
}
type SetupObservation struct {
	Version         VersionObservation
	Skills          []SkillObservation
	Instructions    InstructionObservation
	Agents          []AgentObservation
	Hooks           HookObservation
	MCP             []MCPObservation
	Authorization   AuthorizationObservation
	RuntimeEvidence []RuntimeEvidence
}
type RuntimeEvidence struct{ Kind, ID, Signal, Revision string }
type AuthorizationObservation struct {
	PolicyObserved, ToolPermissionObserved bool
	Disabled, Shadowed                     bool
	Err                                    error
}
type AuthorizationObserver interface {
	ObserveAuthorization(context.Context) AuthorizationObservation
}
type AuthorizationObserverFunc func(context.Context) AuthorizationObservation

func (f AuthorizationObserverFunc) ObserveAuthorization(ctx context.Context) AuthorizationObservation {
	return f(ctx)
}

func ObserveSkill(path, expectedSource string) SkillObservation {
	o := SkillObservation{Path: path, ExpectedSource: expectedSource}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		o.Kind = PathMissing
		return o
	}
	if err != nil {
		o.Err = err
		return o
	}
	if info.Mode()&os.ModeSymlink != 0 {
		o.Kind = PathSymlink
		o.Target, o.Err = os.Readlink(path)
		if o.Err == nil {
			o.ResolvedTarget, _ = filepath.EvalSymlinks(path)
			if o.ResolvedTarget != "" {
				o.TreeFingerprint, o.Err = localprojection.FingerprintTree(o.ResolvedTarget)
			}
		}
		return o
	}
	if info.IsDir() {
		o.Kind = PathDirectory
	} else if info.Mode().IsRegular() {
		o.Kind = PathFile
	} else {
		o.Kind = PathOther
	}
	return o
}
func ObserveAgent(path string) AgentObservation {
	fp, exists, err := localprojection.FingerprintPath(path)
	if err != nil {
		return AgentObservation{Path: path, Err: err}
	}
	if !exists {
		return AgentObservation{Path: path, Kind: PathMissing}
	}
	return AgentObservation{Path: path, Kind: PathFile, Fingerprint: fp}
}

const instructionStart = "<!-- packy:claude:start -->"
const instructionEnd = "<!-- packy:claude:end -->"

func ObserveInstructions(path string) InstructionObservation {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return InstructionObservation{Path: path, Revision: Fingerprint(nil), Contributions: map[string]string{}, ForeignContentPreserved: true}
	}
	if err != nil {
		return InstructionObservation{Path: path, Err: err}
	}
	s := string(b)
	starts, ends := strings.Count(s, instructionStart), strings.Count(s, instructionEnd)
	o := InstructionObservation{Path: path, Revision: Fingerprint(b), MarkerCardinality: starts, Contributions: map[string]string{}, ForeignContentPreserved: true}
	if err := validateMarkerPair(s, instructionStart, instructionEnd, "Packy instruction"); err != nil {
		o.Err = fmt.Errorf("%w: start=%d end=%d", err, starts, ends)
		return o
	}
	if starts == 0 {
		return o
	}
	inside := strings.SplitN(strings.SplitN(s, instructionStart, 2)[1], instructionEnd, 2)[0]
	for {
		i := strings.Index(inside, "<!-- contributor:")
		if i < 0 {
			break
		}
		markerEnd := strings.Index(inside[i:], " -->")
		if markerEnd < 0 {
			o.Err = errors.New("invalid contributor marker")
			return o
		}
		id := inside[i+len("<!-- contributor:") : i+markerEnd]
		startMarker := "<!-- contributor:" + id + " -->"
		endMarker := "<!-- /contributor:" + id + " -->"
		if _, duplicate := o.Contributions[id]; duplicate || strings.Count(inside, startMarker) != 1 || strings.Count(inside, endMarker) != 1 {
			o.Err = fmt.Errorf("duplicate or invalid contributor %q", id)
			return o
		}
		bodyStart := i + len(startMarker)
		bodyEndRel := strings.Index(inside[bodyStart:], endMarker)
		if bodyEndRel < 0 {
			o.Err = fmt.Errorf("missing end marker for contributor %q", id)
			return o
		}
		bodyEnd := bodyStart + bodyEndRel
		o.Contributions[id] = Fingerprint([]byte(strings.TrimSpace(inside[bodyStart:bodyEnd])))
		inside = inside[bodyEnd+len(endMarker):]
	}
	return o
}

type SettingsObservation struct {
	Path, Revision                string
	Raw                           []byte
	Parseable, Disabled, Shadowed bool
	Root                          map[string]any
	Err                           error
}

func ObserveSettings(path string, higherPrecedence []byte) SettingsObservation {
	o := SettingsObservation{Path: path}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		o.Parseable = true
		o.Revision = Fingerprint(nil)
		o.Raw = []byte{}
		return o
	}
	if err != nil {
		o.Err = err
		return o
	}
	o.Raw = append([]byte(nil), b...)
	o.Revision = Fingerprint(b)
	if err = json.Unmarshal(b, &o.Root); err != nil {
		o.Err = fmt.Errorf("invalid Claude settings JSON: %w", err)
		return o
	}
	o.Parseable = true
	if disabled, ok := o.Root["disableAllHooks"].(bool); ok {
		o.Disabled = disabled
	}
	if len(higherPrecedence) > 0 {
		var policy map[string]any
		if err = json.Unmarshal(higherPrecedence, &policy); err != nil {
			o.Err = fmt.Errorf("invalid higher-precedence Claude policy JSON: %w", err)
			return o
		}
		if v, ok := policy["disableAllHooks"].(bool); ok && v {
			o.Disabled = true
			o.Shadowed = true
		}
		if policy["hooks"] != nil {
			o.Shadowed = true
		}
	}
	return o
}

// ObserveHooks statically classifies one exact typed hook. Policy may be
// supplied from a higher-precedence settings document and is never mutated.
func ObserveHooks(path string, wanted CommandHookEntry, higherPrecedence []byte) HookObservation {
	return EnrichHookObservation(ObserveSettings(path, higherPrecedence), wanted)
}
func EnrichHookObservation(settings SettingsObservation, wanted CommandHookEntry) HookObservation {
	o := HookObservation{Path: settings.Path, Revision: settings.Revision, Parseable: settings.Parseable, Disabled: settings.Disabled, Shadowed: settings.Shadowed, Err: settings.Err}
	if o.Err != nil {
		return o
	}
	hooks, _ := settings.Root["hooks"].(map[string]any)
	entries, _ := hooks[wanted.Event].([]any)
	target := canonicalFingerprint(hookJSON(wanted))
	for _, e := range entries {
		if canonicalFingerprint(e) == target {
			o.MatchingEntries = append(o.MatchingEntries, target)
			o.EntryFingerprint = target
		}
	}
	return o
}

// ObserveUserMCP performs only a static read of the mixed user store. It never invokes Claude.
func ObserveUserMCP(path, name string) MCPObservation {
	o := MCPObservation{Name: name}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return o
	}
	if err != nil {
		o.Err = err
		return o
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(b, &root); err != nil {
		o.Err = err
		return o
	}
	var servers map[string]json.RawMessage
	for _, key := range []string{"mcpServers", "mcp_servers"} {
		if raw := root[key]; raw != nil {
			if err = json.Unmarshal(raw, &servers); err != nil {
				o.Err = fmt.Errorf("invalid Claude user MCP registry: %w", err)
				return o
			}
			break
		}
	}
	raw, ok := servers[name]
	if !ok {
		return o
	}
	var def struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	if err = json.Unmarshal(raw, &def); err != nil {
		o.Err = err
		return o
	}
	o.Present = true
	o.Identity = NewMCPIdentity(name, def.Command, def.Args, def.Env)
	o.DefinitionFingerprint = canonicalFingerprint(o.Identity)
	return o
}
