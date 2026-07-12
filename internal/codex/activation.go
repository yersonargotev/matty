package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/matty/internal/capabilitypack"
	"github.com/yersonargotev/matty/internal/localprojection"
)

type ActivationAdapter struct {
	bundleRoot string
	skillsDir  string
	promptFile string
	configFile string
}

func NewActivationAdapter(bundleRoot, skillsDir, promptFile string, configFiles ...string) *ActivationAdapter {
	configFile := filepath.Join(filepath.Dir(promptFile), "config.toml")
	if len(configFiles) > 0 && configFiles[0] != "" {
		configFile = configFiles[0]
	}
	return &ActivationAdapter{bundleRoot: bundleRoot, skillsDir: skillsDir, promptFile: promptFile, configFile: configFile}
}

func (a *ActivationAdapter) InspectActivation(_ context.Context, pack capabilitypack.Pack) (capabilitypack.ActivationObservation, error) {
	var projections []capabilitypack.ObservedProjection
	var revisionParts []string
	for _, resource := range pack.Resources {
		switch resource.Kind {
		case "skill":
			source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
			desired, err := localprojection.FingerprintTree(source)
			if err != nil {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("fingerprint skill %q: %w", resource.ID, err)
			}
			target := filepath.Join(a.skillsDir, resource.ID)
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			id := "skill:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillLink, Source: source, Target: target, Description: fmt.Sprintf("link skill %s at %s", resource.ID, target)}})
			revisionParts = append(revisionParts, id+"="+observed)
		case "instruction":
			source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
			content, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read instruction %q: %w", resource.ID, err)
			}
			start, end := instructionMarkers(resource.ID)
			desiredBlock := start + "\n" + strings.TrimSpace(string(content)) + "\n" + end
			current, err := os.ReadFile(a.promptFile)
			if err != nil && !os.IsNotExist(err) {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read Codex instructions: %w", err)
			}
			fragment, exists := extractBlock(string(current), start, end)
			observed := "missing"
			if exists {
				observed = localprojection.FingerprintBytes([]byte(fragment))
			}
			desired := localprojection.FingerprintBytes([]byte(desiredBlock))
			merged := mergeBlock(string(current), desiredBlock, start, end)
			id := "instruction:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionInstructionFile, Target: a.promptFile, Content: merged, Description: fmt.Sprintf("write instruction %s in %s", resource.ID, a.promptFile)}})
			revisionParts = append(revisionParts, "prompt="+localprojection.FingerprintBytes(current))
		case "mcp_server":
			current, err := readOptionalFile(a.configFile)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			desiredBlock := mcpBlock(resource)
			start, end := mcpMarkers(resource.ID)
			fragment, exists := extractBlock(current, start, end)
			observed := "missing"
			if exists {
				observed = localprojection.FingerprintBytes([]byte(fragment))
			} else if codexMCPTableExists(current, resource.ID) {
				exists = true
				observed = localprojection.FingerprintBytes([]byte("unmanaged:" + resource.ID))
			}
			desired := localprojection.FingerprintBytes([]byte(desiredBlock))
			id := "mcp_server:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionCodexMCPConfig, Target: a.configFile, Content: mergeBlock(current, desiredBlock, start, end), Description: fmt.Sprintf("configure Codex MCP server %s in %s", resource.ID, a.configFile)}})
			revisionParts = append(revisionParts, "config="+localprojection.FingerprintBytes([]byte(current)))
		}
	}
	sort.Strings(revisionParts)
	return capabilitypack.ActivationObservation{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections, Readiness: readinessFor(pack), PendingHumanActions: pendingActions(pack)}, nil
}

func (a *ActivationAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) error {
	executor := localprojection.Executor{
		Host:         "Codex",
		SymlinkKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionSkillLink: true},
		FileKinds: map[capabilitypack.ProjectionActionKind]bool{
			capabilitypack.ActionInstructionFile: true,
			capabilitypack.ActionCodexMCPConfig:  true,
		},
	}
	return executor.Apply(actions)
}

func instructionMarkers(id string) (string, string) {
	return "<!-- matty:pack:" + id + ":start -->", "<!-- matty:pack:" + id + ":end -->"
}

func mcpMarkers(id string) (string, string) {
	return "# matty:pack:" + id + ":start", "# matty:pack:" + id + ":end"
}

func mcpBlock(resource capabilitypack.Resource) string {
	start, end := mcpMarkers(resource.ID)
	command, _ := json.Marshal(resource.Command)
	args := make([]string, 0, len(resource.Args))
	for _, arg := range resource.Args {
		encoded, _ := json.Marshal(arg)
		args = append(args, string(encoded))
	}
	return fmt.Sprintf("%s\n[mcp_servers.%s]\ncommand = %s\nargs = [%s]\n%s", start, resource.ID, command, strings.Join(args, ", "), end)
}

func extractBlock(content, startMarker, endMarker string) (string, bool) {
	start := strings.Index(content, startMarker)
	if start < 0 {
		return "", false
	}
	relEnd := strings.Index(content[start:], endMarker)
	if relEnd < 0 {
		return "", false
	}
	end := start + relEnd + len(endMarker)
	return content[start:end], true
}
func mergeBlock(content, block, startMarker, endMarker string) string {
	if existing, ok := extractBlock(content, startMarker, endMarker); ok {
		return strings.Replace(content, existing, block, 1)
	}
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}

func codexMCPTableExists(content, id string) bool {
	want := "[mcp_servers." + id + "]"
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func readOptionalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func readinessFor(pack capabilitypack.Pack) capabilitypack.ReadinessStatus {
	if pack.ID == "matty" {
		return capabilitypack.ReadinessStatus{Authorized: true, Usable: true}
	}
	return capabilitypack.ReadinessStatus{}
}

func pendingActions(pack capabilitypack.Pack) []string {
	if pack.ID != "engram" {
		return nil
	}
	return []string{
		"review and trust the Engram integration in Codex through /hooks; Matty will not bypass hook trust",
		"reload Codex so the configured Engram MCP server becomes available at runtime",
	}
}
