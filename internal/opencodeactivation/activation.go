package opencodeactivation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/matty/internal/capabilitypack"
	"github.com/yersonargotev/matty/internal/localprojection"
	"github.com/yersonargotev/matty/internal/opencode"
)

// ActivationAdapter translates portable pack resources into OpenCode-owned
// filesystem and JSONC projections. Lifecycle policy remains in capabilitypack.
type ActivationAdapter struct {
	bundleRoot string
	skillsDir  string
	configFile string
	promptFile string
}

func NewActivationAdapter(bundleRoot, skillsDir, configFile, promptFile string) *ActivationAdapter {
	return &ActivationAdapter{bundleRoot: bundleRoot, skillsDir: skillsDir, configFile: configFile, promptFile: promptFile}
}

func (a *ActivationAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
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

func (a *ActivationAdapter) inspectReadiness(_ context.Context, pack capabilitypack.Pack, observation capabilitypack.SurfaceInspection, _ []capabilitypack.ExecutableResolution) (capabilitypack.ReadinessObservation, error) {
	if pack.ID != "matty" {
		return capabilitypack.ReadinessObservation{AuthorizationObserved: true, PendingHumanActions: observation.PendingHumanActions, Evidence: []string{"OpenCode permissions and runtime loading are not yet observed"}}, nil
	}
	return capabilitypack.ReadinessObservation{AuthorizationObserved: true, Authorized: true, PendingHumanActions: []string{"reload OpenCode and verify the capability in a new runtime session"}, Evidence: []string{"OpenCode filesystem and config discovery paths inspected; runtime loading is not observable without a host signal"}}, nil
}

func (a *ActivationAdapter) inspectDesired(_ context.Context, pack capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	var projections []capabilitypack.ObservedProjection
	var revisionParts []string
	desiredConfig := ""
	configLoaded := false
	for _, resource := range pack.Resources {
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
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeSkillLink, Source: source, Target: target, Description: fmt.Sprintf("link OpenCode skill %s at %s", resource.ID, target)}})
			revisionParts = append(revisionParts, id+"="+observed)
		case "instruction":
			source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
			content, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read instruction %q: %w", resource.ID, err)
			}
			desiredContent := strings.TrimSpace(string(content)) + "\n"
			promptFile := a.instructionPath(resource.ID)
			currentPrompt, err := os.ReadFile(promptFile)
			if err != nil && !os.IsNotExist(err) {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read OpenCode instruction file: %w", err)
			}
			promptObserved := "missing"
			promptExists := err == nil
			if promptExists {
				promptObserved = localprojection.FingerprintBytes(currentPrompt)
			}
			promptID := "instruction:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: promptID, Exists: promptExists, ObservedFingerprint: promptObserved, DesiredFingerprint: localprojection.FingerprintBytes([]byte(desiredContent)), Action: capabilitypack.ProjectionAction{ID: promptID, Kind: capabilitypack.ActionOpenCodeInstructionFile, Target: promptFile, Content: desiredContent, Description: fmt.Sprintf("write OpenCode instruction %s at %s", resource.ID, promptFile)}})

			currentConfig, err := readOptionalActivationFile(a.configFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			if !configLoaded {
				desiredConfig = currentConfig
				configLoaded = true
			}
			inspection, err := opencode.Inspect(a.configFile, promptFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			merged, err := opencode.MergeInstructionProjection(currentConfig, a.configFile, promptFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			desiredConfig, err = opencode.MergeInstructionProjection(desiredConfig, a.configFile, promptFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			refID := "opencode-instruction-reference:" + resource.ID
			refDesired := localprojection.FingerprintBytes([]byte(promptFile))
			refObserved := "missing"
			if inspection.HasMattyInstruction {
				refObserved = refDesired
			}
			projections = append(projections, capabilitypack.ObservedProjection{ID: refID, Exists: inspection.HasMattyInstruction, ObservedFingerprint: refObserved, DesiredFingerprint: refDesired, Action: capabilitypack.ProjectionAction{ID: refID, Kind: capabilitypack.ActionOpenCodeConfigReference, Target: a.configFile, Content: merged, Description: fmt.Sprintf("add OpenCode instruction reference in %s", a.configFile)}})
			revisionParts = append(revisionParts, "prompt="+localprojection.FingerprintBytes(currentPrompt), "config="+localprojection.FingerprintBytes([]byte(currentConfig)))
		case "mcp_server":
			command := capabilitypack.ResolvedExecutablePath(resource.Command, resolutions)
			currentConfig, err := readOptionalActivationFile(a.configFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			if !configLoaded {
				desiredConfig = currentConfig
				configLoaded = true
			}
			inspection, err := opencode.InspectMCPContent(currentConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			merged, err := opencode.MergeMCPProjection(currentConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			desiredConfig, err = opencode.MergeMCPProjection(desiredConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			id := "mcp_server:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: inspection.Exists, ObservedFingerprint: inspection.ObservedFingerprint, DesiredFingerprint: inspection.DesiredFingerprint, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeMCPConfig, Target: a.configFile, Content: merged, Command: command, Args: append([]string(nil), resource.Args...), Description: fmt.Sprintf("configure OpenCode MCP server %s in %s", resource.ID, a.configFile)}})
			revisionParts = append(revisionParts, "config="+localprojection.FingerprintBytes([]byte(currentConfig)))
		}
	}
	if configLoaded {
		for i := range projections {
			if projections[i].Action.Target == a.configFile {
				projections[i].Action.Content = desiredConfig
			}
		}
	}
	for i := range projections {
		projections[i].Goal = capabilitypack.ProjectionPresent
	}
	sort.Strings(revisionParts)
	return capabilitypack.SurfaceInspection{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections, PendingHumanActions: pendingActions(pack)}, nil
}

func (a *ActivationAdapter) inspectPriorTransition(ctx context.Context, active, desired capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
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
	configContent, err := readOptionalActivationFile(a.configFile)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	candidateStart := len(result.Projections)
	for _, projection := range current.Projections {
		if retained[projection.ID] {
			continue
		}
		mode := capabilitypack.ProjectionRemoveContent
		projection.Action.Content = ""
		switch projection.Action.Kind {
		case capabilitypack.ActionOpenCodeSkillLink, capabilitypack.ActionOpenCodeInstructionFile:
			mode = capabilitypack.ProjectionDeleteTarget
		case capabilitypack.ActionOpenCodeConfigReference:
			id := strings.TrimPrefix(projection.ID, "opencode-instruction-reference:")
			configContent, err = opencode.RemoveInstructionProjection(configContent, a.configFile, a.instructionPath(id))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projection.Action.Content = configContent
		case capabilitypack.ActionOpenCodeMCPConfig:
			id := strings.TrimPrefix(projection.ID, "mcp_server:")
			configContent, err = opencode.RemoveMCPProjection(configContent, a.configFile, id)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projection.Action.Content = configContent
		}
		projection = capabilitypack.RemovalCandidate(projection, mode, projection.Action.Content, fmt.Sprintf("remove OpenCode projection %s", projection.ID))
		result.Projections = append(result.Projections, projection)
	}
	for i := candidateStart; i < len(result.Projections); i++ {
		if result.Projections[i].Action.Target == a.configFile {
			result.Projections[i].Action.Content = configContent
		}
	}
	sort.Slice(result.Projections, func(i, j int) bool { return result.Projections[i].ID < result.Projections[j].ID })
	result.Revision = localprojection.FingerprintBytes([]byte(current.Revision + "\n" + result.Revision))
	return result, nil
}

func (a *ActivationAdapter) inspectOwnershipResidual(ctx context.Context, desired capabilitypack.Pack, ownership []capabilitypack.ProjectionOwnership, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	result, err := a.inspectDesired(ctx, desired, resolutions)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	retained := make(map[string]bool, len(result.Projections))
	for _, projection := range result.Projections {
		retained[projection.ID] = true
	}
	configContent, err := readOptionalActivationFile(a.configFile)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	candidateStart := len(result.Projections)
	for _, owner := range ownership {
		if retained[owner.ID] {
			continue
		}
		projection, ok, inspectErr := a.inspectOwnedProjection(owner.ID, configContent)
		if inspectErr != nil {
			return capabilitypack.SurfaceInspection{}, inspectErr
		}
		if ok {
			result.Projections = append(result.Projections, projection)
			if projection.Action.Target == a.configFile {
				configContent = projection.Action.Content
			}
		}
	}
	for i := candidateStart; i < len(result.Projections); i++ {
		if result.Projections[i].Action.Target == a.configFile {
			result.Projections[i].Action.Content = configContent
		}
	}
	sort.Slice(result.Projections, func(i, j int) bool { return result.Projections[i].ID < result.Projections[j].ID })
	return result, nil
}

func (a *ActivationAdapter) inspectOwnedProjection(id, configContent string) (capabilitypack.ObservedProjection, bool, error) {
	projection := capabilitypack.ObservedProjection{ID: id, DesiredFingerprint: "missing", ObservedFingerprint: "missing"}
	switch {
	case strings.HasPrefix(id, "skill:"):
		target := filepath.Join(a.skillsDir, strings.TrimPrefix(id, "skill:"))
		observed, exists, err := localprojection.FingerprintPath(target)
		projection.Exists, projection.ObservedFingerprint = exists, observed
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeSkillLink, Target: target}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", fmt.Sprintf("remove OpenCode projection %s", id)), true, err
	case strings.HasPrefix(id, "instruction:"):
		resourceID := strings.TrimPrefix(id, "instruction:")
		target := a.instructionPath(resourceID)
		observed, exists, err := localprojection.FingerprintPath(target)
		projection.Exists, projection.ObservedFingerprint = exists, observed
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeInstructionFile, Target: target}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", fmt.Sprintf("remove OpenCode projection %s", id)), true, err
	case strings.HasPrefix(id, "opencode-instruction-reference:"):
		resourceID := strings.TrimPrefix(id, "opencode-instruction-reference:")
		target := a.instructionPath(resourceID)
		inspection, err := opencode.Inspect(a.configFile, target)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		projection.Exists = inspection.HasMattyInstruction
		if projection.Exists {
			projection.ObservedFingerprint = localprojection.FingerprintBytes([]byte(target))
		}
		content, err := opencode.RemoveInstructionProjection(configContent, a.configFile, target)
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeConfigReference, Target: a.configFile}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionRemoveContent, content, fmt.Sprintf("remove OpenCode projection %s", id)), true, err
	case strings.HasPrefix(id, "mcp_server:"):
		resourceID := strings.TrimPrefix(id, "mcp_server:")
		inspection, err := opencode.InspectMCPContent(configContent, a.configFile, resourceID, "", nil)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		projection.Exists, projection.ObservedFingerprint = inspection.Exists, inspection.ObservedFingerprint
		content, err := opencode.RemoveMCPProjection(configContent, a.configFile, resourceID)
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeMCPConfig, Target: a.configFile}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionRemoveContent, content, fmt.Sprintf("remove OpenCode projection %s", id)), true, err
	default:
		return capabilitypack.ObservedProjection{}, false, nil
	}
}

func (a *ActivationAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	for _, action := range actions {
		switch action.Kind {
		case capabilitypack.ActionOpenCodeConfigReference:
			resourceID := strings.TrimPrefix(action.ID, "opencode-instruction-reference:")
			if action.Mode == capabilitypack.ProjectionRemoveContent {
				if err := opencode.ValidateInstructionRemoval(action.Content, a.configFile, a.instructionPath(resourceID)); err != nil {
					return &capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged OpenCode config removal: %w", err)}
				}
				continue
			}
			if err := opencode.ValidateInstructionProjection(action.Content, a.instructionPath(resourceID)); err != nil {
				return &capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged OpenCode config: %w", err)}
			}
		case capabilitypack.ActionOpenCodeMCPConfig:
			resourceID := strings.TrimPrefix(action.ID, "mcp_server:")
			if action.Mode == capabilitypack.ProjectionRemoveContent {
				inspection, err := opencode.InspectMCPContent(action.Content, a.configFile, resourceID, action.Command, action.Args)
				if err != nil || inspection.Exists {
					return &capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged OpenCode MCP removal: %v", err)}
				}
				continue
			}
			if err := opencode.ValidateMCPProjection(action.Content, a.configFile, resourceID, action.Command, action.Args); err != nil {
				return &capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged OpenCode MCP config: %w", err)}
			}
		}
	}
	executor := localprojection.Executor{
		Host:         "OpenCode",
		SymlinkKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionOpenCodeSkillLink: true},
		FileKinds: map[capabilitypack.ProjectionActionKind]bool{
			capabilitypack.ActionOpenCodeInstructionFile: true, capabilitypack.ActionOpenCodeConfigReference: true, capabilitypack.ActionOpenCodeMCPConfig: true,
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

func (a *ActivationAdapter) instructionPath(id string) string {
	if id == "matty-guidance" {
		return a.promptFile
	}
	return filepath.Join(filepath.Dir(a.promptFile), id+".md")
}

func pendingActions(pack capabilitypack.Pack) []string {
	if pack.ID != "engram" {
		return nil
	}
	return []string{
		"review OpenCode permissions for Engram if the host asks for tool access",
		"reload OpenCode so the configured Engram MCP server becomes available at runtime",
	}
}

func readOptionalActivationFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}
