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

func (a *ActivationAdapter) InspectActivation(ctx context.Context, pack capabilitypack.Pack) (capabilitypack.ActivationObservation, error) {
	return a.InspectActivationWithResolution(ctx, pack, nil)
}

func (a *ActivationAdapter) InspectActivationWithResolution(_ context.Context, pack capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.ActivationObservation, error) {
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
				return capabilitypack.ActivationObservation{}, fmt.Errorf("fingerprint skill %q: %w", resource.ID, err)
			}
			target := filepath.Join(a.skillsDir, resource.ID)
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			id := "skill:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeSkillLink, Source: source, Target: target, Description: fmt.Sprintf("link OpenCode skill %s at %s", resource.ID, target)}})
			revisionParts = append(revisionParts, id+"="+observed)
		case "instruction":
			source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
			content, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read instruction %q: %w", resource.ID, err)
			}
			desiredContent := strings.TrimSpace(string(content)) + "\n"
			promptFile := a.instructionPath(resource.ID)
			currentPrompt, err := os.ReadFile(promptFile)
			if err != nil && !os.IsNotExist(err) {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read OpenCode instruction file: %w", err)
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
				return capabilitypack.ActivationObservation{}, err
			}
			if !configLoaded {
				desiredConfig = currentConfig
				configLoaded = true
			}
			inspection, err := opencode.Inspect(a.configFile, promptFile)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			merged, err := opencode.MergeInstructionProjection(currentConfig, a.configFile, promptFile)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			desiredConfig, err = opencode.MergeInstructionProjection(desiredConfig, a.configFile, promptFile)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
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
				return capabilitypack.ActivationObservation{}, err
			}
			if !configLoaded {
				desiredConfig = currentConfig
				configLoaded = true
			}
			inspection, err := opencode.InspectMCPContent(currentConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			merged, err := opencode.MergeMCPProjection(currentConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			desiredConfig, err = opencode.MergeMCPProjection(desiredConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
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
	configContent, err := readOptionalActivationFile(a.configFile)
	if err != nil {
		return capabilitypack.ActivationObservation{}, err
	}
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
				return capabilitypack.ActivationObservation{}, err
			}
			projection.Action.Content = configContent
		case capabilitypack.ActionOpenCodeMCPConfig:
			id := strings.TrimPrefix(projection.ID, "mcp_server:")
			configContent, err = opencode.RemoveMCPProjection(configContent, a.configFile, id)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			projection.Action.Content = configContent
		}
		projection = capabilitypack.RemovalCandidate(projection, mode, projection.Action.Content, fmt.Sprintf("remove OpenCode projection %s", projection.ID))
		result.RemovalCandidates = append(result.RemovalCandidates, projection)
	}
	for i := range result.RemovalCandidates {
		if result.RemovalCandidates[i].Action.Target == a.configFile {
			result.RemovalCandidates[i].Action.Content = configContent
		}
	}
	sort.Slice(result.RemovalCandidates, func(i, j int) bool { return result.RemovalCandidates[i].ID < result.RemovalCandidates[j].ID })
	result.Revision = localprojection.FingerprintBytes([]byte(current.Revision + "\n" + result.Revision))
	return result, nil
}

func (a *ActivationAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) error {
	for _, action := range actions {
		switch action.Kind {
		case capabilitypack.ActionOpenCodeConfigReference:
			resourceID := strings.TrimPrefix(action.ID, "opencode-instruction-reference:")
			if action.Mode == capabilitypack.ProjectionRemoveContent {
				if err := opencode.ValidateInstructionRemoval(action.Content, a.configFile, a.instructionPath(resourceID)); err != nil {
					return fmt.Errorf("validate staged OpenCode config removal: %w", err)
				}
				continue
			}
			if err := opencode.ValidateInstructionProjection(action.Content, a.instructionPath(resourceID)); err != nil {
				return fmt.Errorf("validate staged OpenCode config: %w", err)
			}
		case capabilitypack.ActionOpenCodeMCPConfig:
			resourceID := strings.TrimPrefix(action.ID, "mcp_server:")
			if action.Mode == capabilitypack.ProjectionRemoveContent {
				inspection, err := opencode.InspectMCPContent(action.Content, a.configFile, resourceID, action.Command, action.Args)
				if err != nil || inspection.Exists {
					return fmt.Errorf("validate staged OpenCode MCP removal: %v", err)
				}
				continue
			}
			if err := opencode.ValidateMCPProjection(action.Content, a.configFile, resourceID, action.Command, action.Args); err != nil {
				return fmt.Errorf("validate staged OpenCode MCP config: %w", err)
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
	return executor.Apply(actions)
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
