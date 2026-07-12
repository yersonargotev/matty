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

func NewActivationAdapter(bundleRoot, skillsDir, promptFile string) *ActivationAdapter {
	return NewActivationAdapterWithConfig(bundleRoot, skillsDir, promptFile, filepath.Join(filepath.Dir(promptFile), "config.toml"))
}

func NewActivationAdapterWithConfig(bundleRoot, skillsDir, promptFile, configFile string) *ActivationAdapter {
	return &ActivationAdapter{bundleRoot: bundleRoot, skillsDir: skillsDir, promptFile: promptFile, configFile: configFile}
}

// InspectReadiness is filesystem-only and side-effect-free. The initial matty
// pack has no authentication boundary and its file-discovered resources are
// usable as soon as every required projection is loadable at its host path.
func (a *ActivationAdapter) InspectReadiness(_ context.Context, pack capabilitypack.Pack, observation capabilitypack.ActivationObservation, _ []capabilitypack.ExecutableResolution) (capabilitypack.ReadinessObservation, error) {
	if pack.ID != "matty" {
		return capabilitypack.ReadinessObservation{AuthorizationObserved: true, PendingHumanActions: observation.PendingHumanActions, Evidence: []string{"Codex trust and runtime loading are not yet observed"}}, nil
	}
	return capabilitypack.ReadinessObservation{AuthorizationObserved: true, Authorized: true, PendingHumanActions: []string{"reload Codex and verify the capability in a new runtime session"}, Evidence: []string{"Codex filesystem discovery paths inspected; runtime loading is not observable without a host signal"}}, nil
}

func (a *ActivationAdapter) InspectActivation(ctx context.Context, pack capabilitypack.Pack) (capabilitypack.ActivationObservation, error) {
	return a.InspectActivationWithResolution(ctx, pack, nil)
}

func (a *ActivationAdapter) InspectActivationWithResolution(_ context.Context, pack capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.ActivationObservation, error) {
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
			command := capabilitypack.ResolvedExecutablePath(resource.Command, resolutions)
			desiredBlock := mcpBlock(resource, command)
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
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionCodexMCPConfig, Target: a.configFile, Content: mergeBlock(current, desiredBlock, start, end), Command: command, Args: append([]string(nil), resource.Args...), Description: fmt.Sprintf("configure Codex MCP server %s in %s", resource.ID, a.configFile)}})
			revisionParts = append(revisionParts, "config="+localprojection.FingerprintBytes([]byte(current)))
		}
	}
	sort.Strings(revisionParts)
	return capabilitypack.ActivationObservation{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections, PendingHumanActions: pendingActions(pack)}, nil
}

func (a *ActivationAdapter) InspectDeactivation(ctx context.Context, active, desired capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.ActivationObservation, error) {
	current, err := a.InspectActivationWithResolution(ctx, active, resolutions)
	if err != nil {
		return capabilitypack.ActivationObservation{}, err
	}
	result, err := a.InspectActivationWithResolution(ctx, desired, resolutions)
	if err != nil {
		return capabilitypack.ActivationObservation{}, err
	}
	retained := map[string]bool{}
	for _, projection := range result.Projections {
		retained[projection.ID] = true
	}
	promptContent, err := readOptionalFile(a.promptFile)
	if err != nil {
		return capabilitypack.ActivationObservation{}, err
	}
	configContent, err := readOptionalFile(a.configFile)
	if err != nil {
		return capabilitypack.ActivationObservation{}, err
	}
	for _, projection := range current.Projections {
		if retained[projection.ID] {
			continue
		}
		mode := capabilitypack.ProjectionRemoveContent
		content := ""
		switch projection.Action.Kind {
		case capabilitypack.ActionSkillLink:
			mode = capabilitypack.ProjectionDeleteTarget
		case capabilitypack.ActionInstructionFile:
			id := strings.TrimPrefix(projection.ID, "instruction:")
			start, end := instructionMarkers(id)
			promptContent = removeBlock(promptContent, start, end)
			content = promptContent
		case capabilitypack.ActionCodexMCPConfig:
			id := strings.TrimPrefix(projection.ID, "mcp_server:")
			start, end := mcpMarkers(id)
			configContent = removeBlock(configContent, start, end)
			content = configContent
		}
		projection = capabilitypack.RemovalCandidate(projection, mode, content, fmt.Sprintf("remove Codex projection %s", projection.ID))
		result.RemovalCandidates = append(result.RemovalCandidates, projection)
	}
	for i := range result.RemovalCandidates {
		switch result.RemovalCandidates[i].Action.Target {
		case a.promptFile:
			result.RemovalCandidates[i].Action.Content = promptContent
		case a.configFile:
			result.RemovalCandidates[i].Action.Content = configContent
		}
	}
	sort.Slice(result.RemovalCandidates, func(i, j int) bool { return result.RemovalCandidates[i].ID < result.RemovalCandidates[j].ID })
	result.Revision = localprojection.FingerprintBytes([]byte(current.Revision + "\n" + result.Revision))
	return result, nil
}

func (a *ActivationAdapter) InspectReconcile(ctx context.Context, desired capabilitypack.Pack, ownership []capabilitypack.ProjectionOwnership, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.ActivationObservation, error) {
	result, err := a.InspectActivationWithResolution(ctx, desired, resolutions)
	if err != nil {
		return capabilitypack.ActivationObservation{}, err
	}
	retained := make(map[string]bool, len(result.Projections))
	for _, projection := range result.Projections {
		retained[projection.ID] = true
	}
	promptContent, err := readOptionalFile(a.promptFile)
	if err != nil {
		return capabilitypack.ActivationObservation{}, err
	}
	configContent, err := readOptionalFile(a.configFile)
	if err != nil {
		return capabilitypack.ActivationObservation{}, err
	}
	for _, owner := range ownership {
		if retained[owner.ID] {
			continue
		}
		projection, ok, inspectErr := a.inspectOwnedProjection(owner.ID, promptContent, configContent)
		if inspectErr != nil {
			return capabilitypack.ActivationObservation{}, inspectErr
		}
		if ok {
			result.RemovalCandidates = append(result.RemovalCandidates, projection)
			switch projection.Action.Target {
			case a.promptFile:
				promptContent = projection.Action.Content
			case a.configFile:
				configContent = projection.Action.Content
			}
		}
	}
	for i := range result.RemovalCandidates {
		switch result.RemovalCandidates[i].Action.Target {
		case a.promptFile:
			result.RemovalCandidates[i].Action.Content = promptContent
		case a.configFile:
			result.RemovalCandidates[i].Action.Content = configContent
		}
	}
	sort.Slice(result.RemovalCandidates, func(i, j int) bool { return result.RemovalCandidates[i].ID < result.RemovalCandidates[j].ID })
	return result, nil
}

func (a *ActivationAdapter) inspectOwnedProjection(id, promptContent, configContent string) (capabilitypack.ObservedProjection, bool, error) {
	projection := capabilitypack.ObservedProjection{ID: id, DesiredFingerprint: "missing"}
	switch {
	case strings.HasPrefix(id, "skill:"):
		target := filepath.Join(a.skillsDir, strings.TrimPrefix(id, "skill:"))
		observed, exists, err := localprojection.FingerprintPath(target)
		projection.Exists, projection.ObservedFingerprint = exists, observed
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillLink, Target: target}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", fmt.Sprintf("remove Codex projection %s", id)), true, err
	case strings.HasPrefix(id, "instruction:"):
		resourceID := strings.TrimPrefix(id, "instruction:")
		start, end := instructionMarkers(resourceID)
		fragment, exists := extractBlock(promptContent, start, end)
		projection.Exists = exists
		projection.ObservedFingerprint = "missing"
		if exists {
			projection.ObservedFingerprint = localprojection.FingerprintBytes([]byte(fragment))
		}
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionInstructionFile, Target: a.promptFile}
		content := removeBlock(promptContent, start, end)
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionRemoveContent, content, fmt.Sprintf("remove Codex projection %s", id)), true, nil
	case strings.HasPrefix(id, "mcp_server:"):
		resourceID := strings.TrimPrefix(id, "mcp_server:")
		start, end := mcpMarkers(resourceID)
		fragment, exists := extractBlock(configContent, start, end)
		projection.Exists = exists
		projection.ObservedFingerprint = "missing"
		if exists {
			projection.ObservedFingerprint = localprojection.FingerprintBytes([]byte(fragment))
		} else if codexMCPTableExists(configContent, resourceID) {
			projection.Exists = true
			projection.ObservedFingerprint = localprojection.FingerprintBytes([]byte("unmanaged:" + resourceID))
		}
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionCodexMCPConfig, Target: a.configFile}
		content := removeBlock(configContent, start, end)
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionRemoveContent, content, fmt.Sprintf("remove Codex projection %s", id)), true, nil
	default:
		return capabilitypack.ObservedProjection{}, false, nil
	}
}

func (a *ActivationAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	executor := localprojection.Executor{
		Host:         "Codex",
		SymlinkKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionSkillLink: true},
		FileKinds: map[capabilitypack.ProjectionActionKind]bool{
			capabilitypack.ActionInstructionFile: true,
			capabilitypack.ActionCodexMCPConfig:  true,
		},
	}
	err := executor.Apply(actions)
	if err == nil {
		return nil
	}
	if actionErr, ok := err.(capabilitypack.ProjectionActionError); ok {
		return &actionErr
	}
	return &capabilitypack.ProjectionActionError{ID: actions[0].ID, Err: err}
}

func instructionMarkers(id string) (string, string) {
	return "<!-- matty:pack:" + id + ":start -->", "<!-- matty:pack:" + id + ":end -->"
}

func mcpMarkers(id string) (string, string) {
	return "# matty:pack:" + id + ":start", "# matty:pack:" + id + ":end"
}

func mcpBlock(resource capabilitypack.Resource, command string) string {
	start, end := mcpMarkers(resource.ID)
	encodedCommand, _ := json.Marshal(command)
	args := make([]string, 0, len(resource.Args))
	for _, arg := range resource.Args {
		encoded, _ := json.Marshal(arg)
		args = append(args, string(encoded))
	}
	return fmt.Sprintf("%s\n[mcp_servers.%s]\ncommand = %s\nargs = [%s]\n%s", start, resource.ID, encodedCommand, strings.Join(args, ", "), end)
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

func removeBlock(content, startMarker, endMarker string) string {
	existing, ok := extractBlock(content, startMarker, endMarker)
	if !ok {
		return content
	}
	updated := strings.Replace(content, existing, "", 1)
	for strings.Contains(updated, "\n\n\n") {
		updated = strings.ReplaceAll(updated, "\n\n\n", "\n\n")
	}
	return updated
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

func pendingActions(pack capabilitypack.Pack) []string {
	if pack.ID != "engram" {
		return nil
	}
	return []string{
		"review and trust the Engram integration in Codex through /hooks; Matty will not bypass hook trust",
		"reload Codex so the configured Engram MCP server becomes available at runtime",
	}
}
