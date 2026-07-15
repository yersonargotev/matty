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

type SurfaceAdapter struct {
	bundleRoot string
	skillsDir  string
	promptFile string
	configFile string
}

func NewSurfaceAdapter(bundleRoot, skillsDir, promptFile string) *SurfaceAdapter {
	return NewSurfaceAdapterWithConfig(bundleRoot, skillsDir, promptFile, filepath.Join(filepath.Dir(promptFile), "config.toml"))
}

func NewSurfaceAdapterWithConfig(bundleRoot, skillsDir, promptFile, configFile string) *SurfaceAdapter {
	return &SurfaceAdapter{bundleRoot: bundleRoot, skillsDir: skillsDir, promptFile: promptFile, configFile: configFile}
}

func (a *SurfaceAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	var (
		observation capabilitypack.SurfaceInspection
		err         error
	)
	if len(transition.ResidualOwnership) > 0 {
		observation, err = a.inspectOwnershipResidual(ctx, transition.Desired, transition.ResidualOwnership, transition.ResolvedExecutables)
	} else if transition.Prior.ID != "" {
		observation, err = a.inspectPriorTransition(ctx, transition.Prior, transition.Desired, transition.ResolvedExecutables)
	} else {
		observation, err = a.inspectDesired(ctx, transition.Desired, transition.ResolvedExecutables)
	}
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	observation.Readiness, err = a.inspectReadiness(ctx, transition.Desired, observation, transition.ResolvedExecutables)
	return observation, err
}

// InspectReadiness is filesystem-only and side-effect-free. The initial matty
// pack has no authentication boundary and its file-discovered resources are
// usable as soon as every required projection is loadable at its host path.
func (a *SurfaceAdapter) inspectReadiness(_ context.Context, pack capabilitypack.Pack, observation capabilitypack.SurfaceInspection, _ []capabilitypack.ExecutableResolution) (capabilitypack.ReadinessObservation, error) {
	if pack.ID != "matty" {
		return capabilitypack.ReadinessObservation{AuthorizationObserved: true, PendingHumanActions: observation.PendingHumanActions, Evidence: []string{"Codex trust and runtime loading are not yet observed"}}, nil
	}
	return capabilitypack.ReadinessObservation{AuthorizationObserved: true, Authorized: true, PendingHumanActions: []string{"reload Codex and verify the capability in a new runtime session"}, Evidence: []string{"Codex filesystem discovery paths inspected; runtime loading is not observable without a host signal"}}, nil
}

func (a *SurfaceAdapter) inspectDesired(_ context.Context, pack capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	var projections []capabilitypack.ObservedProjection
	var revisionParts []string
	var desiredPrompt string
	promptLoaded := false
	engramOwned := hasEngramCodexSetupResources(pack)
	if engramOwned {
		config, err := readOptionalFile(a.configFile)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		engramProjections, err := a.inspectEngramContract(config, resolutions)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, err
		}
		projections = append(projections, engramProjections...)
		for _, projection := range engramProjections {
			revisionParts = append(revisionParts, projection.ID+"="+projection.ObservedFingerprint)
		}
	}
	for _, resource := range pack.Resources {
		if engramOwned && isEngramOwnedResource(resource) {
			continue
		}
		switch resource.Kind {
		case "skill":
			source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
			desired, err := localprojection.FingerprintTree(source)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("fingerprint skill %q: %w", resource.ID, err)
			}
			target := filepath.Join(a.skillsDir, resource.ID)
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			id := "skill:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillLink, Source: source, Target: target, Description: fmt.Sprintf("link skill %s at %s", resource.ID, target)}})
			revisionParts = append(revisionParts, id+"="+observed)
		case "instruction":
			source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
			content, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read instruction %q: %w", resource.ID, err)
			}
			start, end := instructionMarkers(resource.ID)
			desiredBlock := start + "\n" + strings.TrimSpace(string(content)) + "\n" + end
			current, err := os.ReadFile(a.promptFile)
			if err != nil && !os.IsNotExist(err) {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read Codex instructions: %w", err)
			}
			if !promptLoaded {
				desiredPrompt = string(current)
				promptLoaded = true
			}
			fragment, exists := extractBlock(string(current), start, end)
			observed := "missing"
			if exists {
				observed = localprojection.FingerprintBytes([]byte(fragment))
			}
			desired := localprojection.FingerprintBytes([]byte(desiredBlock))
			desiredPrompt = mergeBlock(desiredPrompt, desiredBlock, start, end)
			id := "instruction:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionInstructionFile, Target: a.promptFile, Content: desiredPrompt, Description: fmt.Sprintf("write instruction %s in %s", resource.ID, a.promptFile)}})
			revisionParts = append(revisionParts, "prompt="+localprojection.FingerprintBytes(current))
		case "mcp_server":
			current, err := readOptionalFile(a.configFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
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
	for i := range projections {
		projections[i].Goal = capabilitypack.ProjectionPresent
	}
	sort.Strings(revisionParts)
	return capabilitypack.SurfaceInspection{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections, PendingHumanActions: pendingActions(pack)}, nil
}

func (a *SurfaceAdapter) inspectPriorTransition(ctx context.Context, active, desired capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	current, err := a.inspectDesired(ctx, active, resolutions)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	result, err := a.inspectDesired(ctx, desired, resolutions)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	retained := map[string]bool{}
	for _, projection := range result.Projections {
		retained[projection.ID] = true
	}
	promptContent, err := readOptionalFile(a.promptFile)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	configContent, err := readOptionalFile(a.configFile)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	candidateStart := len(result.Projections)
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
		result.Projections = append(result.Projections, projection)
	}
	for i := candidateStart; i < len(result.Projections); i++ {
		switch result.Projections[i].Action.Target {
		case a.promptFile:
			result.Projections[i].Action.Content = promptContent
		case a.configFile:
			result.Projections[i].Action.Content = configContent
		}
	}
	sort.Slice(result.Projections, func(i, j int) bool { return result.Projections[i].ID < result.Projections[j].ID })
	result.Revision = localprojection.FingerprintBytes([]byte(current.Revision + "\n" + result.Revision))
	return result, nil
}

func (a *SurfaceAdapter) inspectOwnershipResidual(ctx context.Context, desired capabilitypack.Pack, ownership []capabilitypack.ProjectionOwnership, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	result, err := a.inspectDesired(ctx, desired, resolutions)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	retained := make(map[string]bool, len(result.Projections))
	for _, projection := range result.Projections {
		retained[projection.ID] = true
	}
	promptContent, err := readOptionalFile(a.promptFile)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	configContent, err := readOptionalFile(a.configFile)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	candidateStart := len(result.Projections)
	for _, owner := range ownership {
		if retained[owner.ID] {
			continue
		}
		projection, ok, inspectErr := a.inspectOwnedProjection(owner.ID, promptContent, configContent)
		if inspectErr != nil {
			return capabilitypack.SurfaceInspection{}, inspectErr
		}
		if ok {
			result.Projections = append(result.Projections, projection)
			switch projection.Action.Target {
			case a.promptFile:
				promptContent = projection.Action.Content
			case a.configFile:
				configContent = projection.Action.Content
			}
		}
	}
	for i := candidateStart; i < len(result.Projections); i++ {
		switch result.Projections[i].Action.Target {
		case a.promptFile:
			result.Projections[i].Action.Content = promptContent
		case a.configFile:
			result.Projections[i].Action.Content = configContent
		}
	}
	sort.Slice(result.Projections, func(i, j int) bool { return result.Projections[i].ID < result.Projections[j].ID })
	return result, nil
}

func (a *SurfaceAdapter) inspectOwnedProjection(id, promptContent, configContent string) (capabilitypack.ObservedProjection, bool, error) {
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

func (a *SurfaceAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
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
