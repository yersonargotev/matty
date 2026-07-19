package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

// SurfaceAdapter translates portable pack resources into OpenCode-owned
// filesystem and JSONC projections. Lifecycle policy remains in capabilitypack.
type SurfaceAdapter struct {
	bundleRoot string
	skillsDir  string
	configFile string
	promptFile string
}

func NewSurfaceAdapter(bundleRoot, skillsDir, configFile, promptFile string) *SurfaceAdapter {
	return &SurfaceAdapter{bundleRoot: bundleRoot, skillsDir: skillsDir, configFile: configFile, promptFile: promptFile}
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
	applyRecordedOccupancyOwnership(&observation, transition.CurrentOwnership)
	observation.Readiness, err = a.inspectReadiness(ctx, transition.Desired, observation, transition.ResolvedExecutables)
	return observation, err
}

func (a *SurfaceAdapter) inspectReadiness(_ context.Context, pack capabilitypack.Pack, observation capabilitypack.SurfaceInspection, _ []capabilitypack.ExecutableResolution) (capabilitypack.ReadinessObservation, error) {
	if pack.ID != "matty" {
		return capabilitypack.ReadinessObservation{AuthorizationObserved: true, PendingHumanActions: observation.PendingHumanActions, Evidence: []string{"OpenCode permissions and runtime loading are not yet observed"}}, nil
	}
	return capabilitypack.ReadinessObservation{AuthorizationObserved: true, Authorized: true, PendingHumanActions: []string{"reload OpenCode and verify the capability in a new runtime session"}, Evidence: []string{"OpenCode filesystem and config discovery paths inspected; runtime loading is not observable without a host signal"}}, nil
}

func (a *SurfaceAdapter) inspectDesired(_ context.Context, pack capabilitypack.Pack, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
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
			name, ok := openCodeBindingName(resource, "skill")
			if !ok {
				continue
			}
			target := filepath.Join(a.skillsDir, name)
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			id := "skill:" + name
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeSkillLink, Source: source, Target: target, Description: fmt.Sprintf("link OpenCode skill %s at %s", resource.ID, target)}})
			revisionParts = append(revisionParts, id+"="+observed)
			assets, err := a.consumerAssetProjections(pack, resource, id, filepath.Join(a.skillsDir, ".packy-assets"))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			for _, asset := range assets {
				projections = append(projections, asset)
				revisionParts = append(revisionParts, asset.ID+"="+asset.ObservedFingerprint)
			}
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

			currentConfig, err := readOptionalSurfaceFile(a.configFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			if !configLoaded {
				desiredConfig = currentConfig
				configLoaded = true
			}
			inspection, err := Inspect(a.configFile, promptFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			merged, err := MergeInstructionProjection(currentConfig, a.configFile, promptFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			desiredConfig, err = MergeInstructionProjection(desiredConfig, a.configFile, promptFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			refID := "opencode-instruction-reference:" + resource.ID
			refDesired := localprojection.FingerprintBytes([]byte(promptFile))
			refObserved := "missing"
			if inspection.HasPackyInstruction {
				refObserved = refDesired
			}
			projections = append(projections, capabilitypack.ObservedProjection{ID: refID, Exists: inspection.HasPackyInstruction, ObservedFingerprint: refObserved, DesiredFingerprint: refDesired, Action: capabilitypack.ProjectionAction{ID: refID, Kind: capabilitypack.ActionOpenCodeConfigReference, Target: a.configFile, Content: merged, Description: fmt.Sprintf("add OpenCode instruction reference in %s", a.configFile)}})
			revisionParts = append(revisionParts, "prompt="+localprojection.FingerprintBytes(currentPrompt), "config="+localprojection.FingerprintBytes([]byte(currentConfig)))
		case "mcp_server":
			command := capabilitypack.ResolvedExecutablePath(resource.Command, resolutions)
			currentConfig, err := readOptionalSurfaceFile(a.configFile)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			if !configLoaded {
				desiredConfig = currentConfig
				configLoaded = true
			}
			inspection, err := InspectMCPContent(currentConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			merged, err := MergeMCPProjection(currentConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			desiredConfig, err = MergeMCPProjection(desiredConfig, a.configFile, resource.ID, command, resource.Args)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			id := "mcp_server:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: inspection.Exists, ObservedFingerprint: inspection.ObservedFingerprint, DesiredFingerprint: inspection.DesiredFingerprint, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeMCPConfig, Target: a.configFile, Content: merged, Command: command, Args: append([]string(nil), resource.Args...), Description: fmt.Sprintf("configure OpenCode MCP server %s in %s", resource.ID, a.configFile)}})
			revisionParts = append(revisionParts, "config="+localprojection.FingerprintBytes([]byte(currentConfig)))
		case "agent":
			name, ok := openCodeBindingName(resource, "agent")
			if !ok {
				continue
			}
			prompt, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(resource.Source)))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read agent %q: %w", resource.ID, err)
			}
			target := filepath.Join(filepath.Dir(a.configFile), "agents", name+".md")
			projection, revision, err := fileProjection("agent:"+name, capabilitypack.ActionOpenCodeAgentFile, target, openCodeAgentMarkdown(pack, resource, prompt), fmt.Sprintf("write OpenCode agent %s at %s", name, target))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projections = append(projections, projection)
			revisionParts = append(revisionParts, revision)
			assets, err := a.consumerAssetProjections(pack, resource, "agent:"+name, filepath.Join(filepath.Dir(a.configFile), "agents"))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			for _, asset := range assets {
				projections = append(projections, asset)
				revisionParts = append(revisionParts, asset.ID+"="+asset.ObservedFingerprint)
			}
		case "command":
			name, ok := openCodeBindingName(resource, "command")
			if !ok {
				continue
			}
			prompt, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(resource.Source)))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read command %q: %w", resource.ID, err)
			}
			target := filepath.Join(filepath.Dir(a.configFile), "commands", name+".md")
			projection, revision, err := fileProjection("command:"+name, capabilitypack.ActionOpenCodeCommandFile, target, openCodeCommandMarkdown(pack, resource, prompt), fmt.Sprintf("write OpenCode command %s at %s", name, target))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projections = append(projections, projection)
			revisionParts = append(revisionParts, revision)
			assets, err := a.consumerAssetProjections(pack, resource, "command:"+name, filepath.Dir(target))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			for _, asset := range assets {
				projections = append(projections, asset)
				revisionParts = append(revisionParts, asset.ID+"="+asset.ObservedFingerprint)
			}
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
	for i, projection := range projections {
		for _, prior := range projections[:i] {
			if (!exclusivePathKind(projection.Action.Kind) && !exclusivePathKind(prior.Action.Kind)) || projection.Action.Target == "" || prior.Action.Target == "" {
				continue
			}
			if pathsOverlap(prior.Action.Target, projection.Action.Target) {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("OpenCode projections %s and %s have overlapping targets %s and %s", prior.ID, projection.ID, prior.Action.Target, projection.Action.Target)
			}
		}
	}
	sort.Slice(projections, func(i, j int) bool { return projections[i].ID < projections[j].ID })
	sort.Strings(revisionParts)
	occupied, err := a.inspectOccupiedNames()
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	for _, name := range occupied {
		revisionParts = append(revisionParts, "occupied:"+name.Namespace+":"+name.Name+"="+name.OwnerType+":"+name.OwnerID+":"+name.Fingerprint)
	}
	sort.Strings(revisionParts)
	return capabilitypack.SurfaceInspection{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections, OccupiedNames: occupied, PendingHumanActions: pendingActions(pack)}, nil
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
	configContent, err := readOptionalSurfaceFile(a.configFile)
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
		case capabilitypack.ActionOpenCodeSkillLink, capabilitypack.ActionOpenCodeInstructionFile, capabilitypack.ActionOpenCodeAgentFile, capabilitypack.ActionOpenCodeCommandFile, capabilitypack.ActionOpenCodeAssetFile:
			mode = capabilitypack.ProjectionDeleteTarget
		case capabilitypack.ActionOpenCodeConfigReference:
			id := strings.TrimPrefix(projection.ID, "opencode-instruction-reference:")
			configContent, err = RemoveInstructionProjection(configContent, a.configFile, a.instructionPath(id))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projection.Action.Content = configContent
		case capabilitypack.ActionOpenCodeMCPConfig:
			id := strings.TrimPrefix(projection.ID, "mcp_server:")
			configContent, err = RemoveMCPProjection(configContent, a.configFile, id)
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

func (a *SurfaceAdapter) inspectOwnershipResidual(ctx context.Context, desired capabilitypack.Pack, ownership []capabilitypack.ProjectionOwnership, resolutions []capabilitypack.ExecutableResolution) (capabilitypack.SurfaceInspection, error) {
	result, err := a.inspectDesired(ctx, desired, resolutions)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	retained := make(map[string]bool, len(result.Projections))
	for _, projection := range result.Projections {
		retained[projection.ID] = true
	}
	configContent, err := readOptionalSurfaceFile(a.configFile)
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

func (a *SurfaceAdapter) inspectOwnedProjection(id, configContent string) (capabilitypack.ObservedProjection, bool, error) {
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
		inspection, err := Inspect(a.configFile, target)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		projection.Exists = inspection.HasPackyInstruction
		if projection.Exists {
			projection.ObservedFingerprint = localprojection.FingerprintBytes([]byte(target))
		}
		content, err := RemoveInstructionProjection(configContent, a.configFile, target)
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeConfigReference, Target: a.configFile}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionRemoveContent, content, fmt.Sprintf("remove OpenCode projection %s", id)), true, err
	case strings.HasPrefix(id, "mcp_server:"):
		resourceID := strings.TrimPrefix(id, "mcp_server:")
		inspection, err := InspectMCPContent(configContent, a.configFile, resourceID, "", nil)
		if err != nil {
			return capabilitypack.ObservedProjection{}, false, err
		}
		projection.Exists, projection.ObservedFingerprint = inspection.Exists, inspection.ObservedFingerprint
		content, err := RemoveMCPProjection(configContent, a.configFile, resourceID)
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionOpenCodeMCPConfig, Target: a.configFile}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionRemoveContent, content, fmt.Sprintf("remove OpenCode projection %s", id)), true, err
	case strings.HasPrefix(id, "agent:"):
		target := filepath.Join(filepath.Dir(a.configFile), "agents", strings.TrimPrefix(id, "agent:")+".md")
		return a.ownedFileRemoval(projection, capabilitypack.ActionOpenCodeAgentFile, target)
	case strings.HasPrefix(id, "command:"):
		target := filepath.Join(filepath.Dir(a.configFile), "commands", strings.TrimPrefix(id, "command:")+".md")
		return a.ownedFileRemoval(projection, capabilitypack.ActionOpenCodeCommandFile, target)
	case strings.HasPrefix(id, "asset:skill:"), strings.HasPrefix(id, "asset:agent:"), strings.HasPrefix(id, "asset:command:"):
		parts := strings.Split(id, ":")
		if len(parts) != 5 {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		var target string
		switch parts[1] {
		case "skill":
			target = filepath.Join(a.skillsDir, ".packy-assets", parts[2], parts[4])
		case "agent", "command":
			target = filepath.Join(filepath.Dir(a.configFile), parts[1]+"s", parts[2], parts[4])
		}
		return a.ownedFileRemoval(projection, capabilitypack.ActionOpenCodeAssetFile, target)
	default:
		return capabilitypack.ObservedProjection{}, false, nil
	}
}

func (a *SurfaceAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	for _, action := range actions {
		switch action.Kind {
		case capabilitypack.ActionOpenCodeConfigReference:
			resourceID := strings.TrimPrefix(action.ID, "opencode-instruction-reference:")
			if action.Mode == capabilitypack.ProjectionRemoveContent {
				if err := ValidateInstructionRemoval(action.Content, a.configFile, a.instructionPath(resourceID)); err != nil {
					return &capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged OpenCode config removal: %w", err)}
				}
				continue
			}
			if err := ValidateInstructionProjection(action.Content, a.instructionPath(resourceID)); err != nil {
				return &capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged OpenCode config: %w", err)}
			}
		case capabilitypack.ActionOpenCodeMCPConfig:
			resourceID := strings.TrimPrefix(action.ID, "mcp_server:")
			if action.Mode == capabilitypack.ProjectionRemoveContent {
				inspection, err := InspectMCPContent(action.Content, a.configFile, resourceID, action.Command, action.Args)
				if err != nil || inspection.Exists {
					return &capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged OpenCode MCP removal: %v", err)}
				}
				continue
			}
			if err := ValidateMCPProjection(action.Content, a.configFile, resourceID, action.Command, action.Args); err != nil {
				return &capabilitypack.ProjectionActionError{ID: action.ID, Err: fmt.Errorf("validate staged OpenCode MCP config: %w", err)}
			}
		}
	}
	executor := localprojection.Executor{
		Host:         "OpenCode",
		SymlinkKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionOpenCodeSkillLink: true},
		FileKinds: map[capabilitypack.ProjectionActionKind]bool{
			capabilitypack.ActionOpenCodeInstructionFile: true, capabilitypack.ActionOpenCodeConfigReference: true, capabilitypack.ActionOpenCodeMCPConfig: true,
			capabilitypack.ActionOpenCodeAgentFile: true, capabilitypack.ActionOpenCodeCommandFile: true, capabilitypack.ActionOpenCodeAssetFile: true,
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

func (a *SurfaceAdapter) ownedFileRemoval(projection capabilitypack.ObservedProjection, kind capabilitypack.ProjectionActionKind, target string) (capabilitypack.ObservedProjection, bool, error) {
	observed, exists, err := localprojection.FingerprintPath(target)
	projection.Exists, projection.ObservedFingerprint = exists, observed
	projection.Action = capabilitypack.ProjectionAction{ID: projection.ID, Kind: kind, Target: target}
	return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", fmt.Sprintf("remove OpenCode projection %s", projection.ID)), true, err
}

func openCodeBindingName(resource capabilitypack.Resource, projection string) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceOpenCode && binding.Projection == projection {
			return binding.Name, true
		}
	}
	if len(resource.Bindings) == 0 && resource.Kind == projection {
		return resource.ID, true
	}
	return "", false
}

func openCodeAgentMarkdown(pack capabilitypack.Pack, resource capabilitypack.Resource, prompt []byte) string {
	description, _ := json.Marshal(resource.Description)
	permissions := append(append([]string(nil), resource.Tools...), resource.Permissions...)
	sort.Strings(permissions)
	permissionLines := make([]string, 0, len(permissions))
	for i, permission := range permissions {
		if i > 0 && permission == permissions[i-1] {
			continue
		}
		key, _ := json.Marshal(permission)
		permissionLines = append(permissionLines, "  "+string(key)+": allow")
	}
	permissionBlock := ""
	if len(permissionLines) > 0 {
		permissionBlock = "permission:\n" + strings.Join(permissionLines, "\n") + "\n"
	}
	_, composition := openCodeComposition(pack, resource)
	return fmt.Sprintf("---\ndescription: %s\nmode: %s\n%s---\n\nPacky agent policy (required): tools=%s; permissions=%s; composition=%s. Preserve these constraints and load the named native skills when executing.\n\n%s\n", description, resource.Mode, permissionBlock, strings.Join(resource.Tools, ","), strings.Join(resource.Permissions, ","), composition, strings.TrimSpace(string(prompt)))
}

func openCodeCommandMarkdown(pack capabilitypack.Pack, resource capabilitypack.Resource, prompt []byte) string {
	description := resource.Description
	if description == "" {
		description = "Run the " + resource.ID + " command."
	}
	argumentPolicy := "This command accepts no arguments."
	if resource.Arguments.Mode == "freeform" {
		argumentPolicy = "Treat $ARGUMENTS as the caller's free-form command input; preserve it without narrowing or reinterpretation."
	}
	encodedDescription, _ := json.Marshal(description)
	agent, composition := openCodeComposition(pack, resource)
	agentLine := ""
	if agent != "" {
		agentLine = "agent: " + agent + "\n"
	}
	return fmt.Sprintf("---\ndescription: %s\n%s---\n\n%s\n\nPacky composition (required): %s. Invoke the named native skills and agent as part of this workflow.\n\n%s\n", encodedDescription, agentLine, argumentPolicy, composition, strings.TrimSpace(string(prompt)))
}

func openCodeComposition(pack capabilitypack.Pack, consumer capabilitypack.Resource) (string, string) {
	byIdentity := make(map[string]capabilitypack.Resource, len(pack.Resources))
	for _, resource := range pack.Resources {
		byIdentity[resource.Kind+":"+resource.ID] = resource
	}
	var agent string
	var translated []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(requirement string) {
		if seen[requirement] {
			return
		}
		seen[requirement] = true
		resource, ok := byIdentity[requirement]
		if !ok {
			return
		}
		if resource.Kind == "asset" {
			return
		}
		name, bound := openCodeBindingName(resource, resource.Kind)
		if !bound {
			translated = append(translated, requirement)
			return
		}
		if resource.Kind == "agent" && agent == "" {
			agent = name
		}
		translated = append(translated, resource.Kind+":"+name)
		for _, nested := range resource.Requires {
			visit(nested)
		}
	}
	for _, requirement := range consumer.Requires {
		visit(requirement)
	}
	return agent, strings.Join(translated, ", ")
}

func (a *SurfaceAdapter) consumerAssetProjections(pack capabilitypack.Pack, consumer capabilitypack.Resource, consumerID, targetDir string) ([]capabilitypack.ObservedProjection, error) {
	byIdentity := map[string]capabilitypack.Resource{}
	for _, resource := range pack.Resources {
		byIdentity[resource.Kind+":"+resource.ID] = resource
	}
	seen := map[string]bool{}
	var assets []capabilitypack.Resource
	var visit func(string)
	visit = func(identity string) {
		if seen[identity] {
			return
		}
		seen[identity] = true
		resource, ok := byIdentity[identity]
		if !ok {
			return
		}
		if resource.Kind == "asset" {
			assets = append(assets, resource)
		}
		for _, requirement := range resource.Requires {
			visit(requirement)
		}
	}
	for _, requirement := range consumer.Requires {
		visit(requirement)
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].ID < assets[j].ID })
	result := make([]capabilitypack.ObservedProjection, 0, len(assets))
	for _, asset := range assets {
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(asset.Source)))
		if err != nil {
			return nil, fmt.Errorf("read asset %q for %s: %w", asset.ID, consumerID, err)
		}
		filename := filepath.Base(asset.Source)
		id := "asset:" + consumerID + ":" + asset.ID + ":" + filename
		_, consumerName, _ := strings.Cut(consumerID, ":")
		projection, _, err := fileProjection(id, capabilitypack.ActionOpenCodeAssetFile, filepath.Join(targetDir, consumerName, filename), string(content), fmt.Sprintf("materialize dependency asset %s for %s", asset.ID, consumerID))
		if err != nil {
			return nil, err
		}
		result = append(result, projection)
	}
	return result, nil
}

func fileProjection(id string, kind capabilitypack.ProjectionActionKind, target, content, description string) (capabilitypack.ObservedProjection, string, error) {
	observed, exists, err := localprojection.FingerprintPath(target)
	if err != nil {
		return capabilitypack.ObservedProjection{}, "", err
	}
	desired := localprojection.FingerprintBytes([]byte(content))
	return capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: kind, Target: target, Content: content, Description: description}}, id + "=" + observed, nil
}

func pathsOverlap(left, right string) bool {
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
func exclusivePathKind(kind capabilitypack.ProjectionActionKind) bool {
	return kind == capabilitypack.ActionOpenCodeSkillLink || kind == capabilitypack.ActionOpenCodeAgentFile || kind == capabilitypack.ActionOpenCodeCommandFile || kind == capabilitypack.ActionOpenCodeAssetFile
}

func (a *SurfaceAdapter) inspectOccupiedNames() ([]capabilitypack.OccupiedName, error) {
	result := []capabilitypack.OccupiedName{
		{Namespace: "agent", Name: "build", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "agent", Name: "explore", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "agent", Name: "general", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "agent", Name: "plan", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "agent", Name: "scout", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "command", Name: "help", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "command", Name: "init", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "command", Name: "redo", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "command", Name: "share", OwnerType: "reserved", Fingerprint: "native"},
		{Namespace: "command", Name: "undo", OwnerType: "reserved", Fingerprint: "native"},
	}
	for _, namespace := range []struct{ name, dir, suffix string }{{"skill", a.skillsDir, ""}, {"agent", filepath.Join(filepath.Dir(a.configFile), "agents"), ".md"}, {"command", filepath.Join(filepath.Dir(a.configFile), "commands"), ".md"}} {
		entries, err := os.ReadDir(namespace.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect OpenCode %s namespace: %w", namespace.name, err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || (namespace.suffix != "" && filepath.Ext(entry.Name()) != namespace.suffix) {
				continue
			}
			fingerprint, exists, err := localprojection.FingerprintPath(filepath.Join(namespace.dir, entry.Name()))
			if err != nil {
				return nil, err
			}
			if exists && !hasOccupiedName(result, namespace.name, strings.TrimSuffix(entry.Name(), namespace.suffix)) {
				result = append(result, capabilitypack.OccupiedName{Namespace: namespace.name, Name: strings.TrimSuffix(entry.Name(), namespace.suffix), OwnerType: "unmanaged", Fingerprint: fingerprint})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func hasOccupiedName(values []capabilitypack.OccupiedName, namespace, name string) bool {
	for _, value := range values {
		if value.Namespace == namespace && value.Name == name {
			return true
		}
	}
	return false
}

func applyRecordedOccupancyOwnership(observation *capabilitypack.SurfaceInspection, ownership []capabilitypack.ProjectionOwnership) {
	byID := map[string]capabilitypack.ProjectionOwnership{}
	for _, owner := range ownership {
		byID[owner.ID] = owner
	}
	for i := range observation.OccupiedNames {
		occupied := &observation.OccupiedNames[i]
		for _, projection := range observation.Projections {
			if !projection.Exists || projection.ObservedFingerprint != occupied.Fingerprint {
				continue
			}
			namespace, name, ok := strings.Cut(projection.ID, ":")
			if !ok || namespace != occupied.Namespace || name != occupied.Name {
				continue
			}
			owner, recorded := byID[projection.ID]
			if recorded && owner.Fingerprint == occupied.Fingerprint {
				occupied.OwnerType, occupied.OwnerID = "packy", strings.Join(owner.Contributors, ",")
			}
		}
	}
}

func (a *SurfaceAdapter) instructionPath(id string) string {
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

func readOptionalSurfaceFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}
