package codex

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
			name, ok := codexBindingName(resource, "skill")
			if !ok {
				continue
			}
			target := filepath.Join(a.skillsDir, name)
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			id := "skill:" + name
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
		case "agent":
			name, ok := codexBindingName(resource, "agent")
			if !ok {
				continue
			}
			source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
			prompt, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read agent %q: %w", resource.ID, err)
			}
			content := codexAgentTOML(resource, name, prompt)
			target := filepath.Join(filepath.Dir(a.promptFile), "agents", name+".toml")
			projection, revision, err := fileProjection("agent:"+name, capabilitypack.ActionCodexAgentFile, target, content, fmt.Sprintf("write Codex agent %s at %s", name, target))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projections = append(projections, projection)
			revisionParts = append(revisionParts, revision)
		case "command":
			name, ok := codexDegradedWorkflowName(resource)
			if !ok {
				continue
			}
			source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
			prompt, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("read command %q: %w", resource.ID, err)
			}
			content := codexWorkflowSkill(resource, name, prompt)
			target := filepath.Join(a.skillsDir, name, "SKILL.md")
			projection, revision, err := fileProjection("workflow:"+name, capabilitypack.ActionCodexWorkflowSkill, target, content, fmt.Sprintf("write degraded Codex workflow skill %s at %s", name, target))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			projections = append(projections, projection)
			revisionParts = append(revisionParts, revision)
			assets, err := a.consumerAssetProjections(pack, resource, "workflow:"+name, filepath.Dir(target))
			if err != nil {
				return capabilitypack.SurfaceInspection{}, err
			}
			for _, asset := range assets {
				projections = append(projections, asset)
				revisionParts = append(revisionParts, asset.ID+"="+asset.ObservedFingerprint)
			}
		}
	}
	for i := range projections {
		projections[i].Goal = capabilitypack.ProjectionPresent
	}
	for i, projection := range projections {
		for _, prior := range projections[:i] {
			if !exclusivePathKind(projection.Action.Kind) && !exclusivePathKind(prior.Action.Kind) {
				continue
			}
			if projection.Action.Target != "" && prior.Action.Target != "" && pathsOverlap(prior.Action.Target, projection.Action.Target) && prior.ID != projection.ID {
				return capabilitypack.SurfaceInspection{}, fmt.Errorf("Codex projections %s and %s have overlapping targets %s and %s", prior.ID, projection.ID, prior.Action.Target, projection.Action.Target)
			}
		}
	}
	sort.Strings(revisionParts)
	sort.Slice(projections, func(i, j int) bool { return projections[i].ID < projections[j].ID })
	occupied, err := a.inspectOccupiedNames(pack, projections)
	if err != nil {
		return capabilitypack.SurfaceInspection{}, err
	}
	for _, name := range occupied {
		revisionParts = append(revisionParts, "occupied:"+name.Namespace+":"+name.Name+"="+name.OwnerType+":"+name.OwnerID+":"+name.Fingerprint)
	}
	sort.Strings(revisionParts)
	return capabilitypack.SurfaceInspection{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections, OccupiedNames: occupied, PendingHumanActions: pendingActions(pack)}, nil
}

func (a *SurfaceAdapter) inspectOccupiedNames(pack capabilitypack.Pack, projections []capabilitypack.ObservedProjection) ([]capabilitypack.OccupiedName, error) {
	desired := map[string]capabilitypack.ObservedProjection{}
	for _, projection := range projections {
		if projection.Action.Target != "" {
			desired[filepath.Clean(projection.Action.Target)] = projection
		}
	}
	var result []capabilitypack.OccupiedName
	for _, namespace := range []struct{ name, dir, suffix string }{{"skill", a.skillsDir, ""}, {"agent", filepath.Join(filepath.Dir(a.promptFile), "agents"), ".toml"}} {
		entries, err := os.ReadDir(namespace.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect Codex %s namespace: %w", namespace.name, err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || (namespace.suffix != "" && filepath.Ext(entry.Name()) != namespace.suffix) {
				continue
			}
			path := filepath.Join(namespace.dir, entry.Name())
			fingerprint, exists, err := localprojection.FingerprintPath(path)
			if err != nil {
				return nil, err
			}
			if !exists {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), namespace.suffix)
			occupied := capabilitypack.OccupiedName{Namespace: namespace.name, Name: name, OwnerType: "unmanaged", Fingerprint: fingerprint}
			if projection, ok := desired[filepath.Clean(path)]; ok && projection.DesiredFingerprint == fingerprint {
				occupied.OwnerType, occupied.OwnerID = "packy", pack.ID
			}
			result = append(result, occupied)
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
		case capabilitypack.ActionCodexAgentFile, capabilitypack.ActionCodexWorkflowSkill, capabilitypack.ActionCodexAssetFile:
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
	case strings.HasPrefix(id, "agent:"):
		target := filepath.Join(filepath.Dir(a.promptFile), "agents", strings.TrimPrefix(id, "agent:")+".toml")
		observed, exists, err := localprojection.FingerprintPath(target)
		projection.Exists, projection.ObservedFingerprint = exists, observed
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionCodexAgentFile, Target: target}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", fmt.Sprintf("remove Codex projection %s", id)), true, err
	case strings.HasPrefix(id, "workflow:"):
		target := filepath.Join(a.skillsDir, strings.TrimPrefix(id, "workflow:"), "SKILL.md")
		observed, exists, err := localprojection.FingerprintPath(target)
		projection.Exists, projection.ObservedFingerprint = exists, observed
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionCodexWorkflowSkill, Target: target}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", fmt.Sprintf("remove Codex projection %s", id)), true, err
	case strings.HasPrefix(id, "asset:workflow:"):
		parts := strings.Split(id, ":")
		if len(parts) != 5 {
			return capabilitypack.ObservedProjection{}, false, nil
		}
		target := filepath.Join(a.skillsDir, parts[2], parts[4])
		observed, exists, err := localprojection.FingerprintPath(target)
		projection.Exists, projection.ObservedFingerprint = exists, observed
		projection.Action = capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionCodexAssetFile, Target: target}
		return capabilitypack.RemovalCandidate(projection, capabilitypack.ProjectionDeleteTarget, "", fmt.Sprintf("remove Codex projection %s", id)), true, err
	default:
		return capabilitypack.ObservedProjection{}, false, nil
	}
}

func (a *SurfaceAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	executor := localprojection.Executor{
		Host:         "Codex",
		SymlinkKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionSkillLink: true},
		FileKinds: map[capabilitypack.ProjectionActionKind]bool{
			capabilitypack.ActionInstructionFile:    true,
			capabilitypack.ActionCodexMCPConfig:     true,
			capabilitypack.ActionCodexAgentFile:     true,
			capabilitypack.ActionCodexWorkflowSkill: true,
			capabilitypack.ActionCodexAssetFile:     true,
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
	return "<!-- packy:pack:" + id + ":start -->", "<!-- packy:pack:" + id + ":end -->"
}

func mcpMarkers(id string) (string, string) {
	return "# packy:pack:" + id + ":start", "# packy:pack:" + id + ":end"
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

func codexBindingName(resource capabilitypack.Resource, projection string) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceCodex && binding.Projection == projection {
			return binding.Name, true
		}
	}
	// Manifest v1 resources predate explicit bindings.
	if len(resource.Bindings) == 0 && resource.Kind == projection {
		return resource.ID, true
	}
	return "", false
}

func codexDegradedWorkflowName(resource capabilitypack.Resource) (string, bool) {
	for _, binding := range resource.Bindings {
		if binding.Surface == capabilitypack.SurfaceCodex && binding.Projection == "skill" && binding.Mode == "degraded" && binding.Degradation == "codex-command-as-workflow-skill" {
			return binding.Name, true
		}
	}
	return "", false
}

func codexAgentTOML(resource capabilitypack.Resource, name string, prompt []byte) string {
	quote := func(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
	values := func(items []string) string {
		encoded := make([]string, len(items))
		for i, item := range items {
			encoded[i] = quote(item)
		}
		return "[" + strings.Join(encoded, ", ") + "]"
	}
	policy := fmt.Sprintf("Packy agent policy (required): mode=%s; tools=%s; permissions=%s. Preserve these constraints when executing.\n\n", resource.Mode, values(resource.Tools), values(resource.Permissions))
	return fmt.Sprintf("name = %s\ndescription = %s\ndeveloper_instructions = %s\n", quote(name), quote(resource.Description), quote(policy+string(prompt)))
}

func codexWorkflowSkill(resource capabilitypack.Resource, name string, prompt []byte) string {
	description := resource.Description
	if description == "" {
		description = "Run the " + name + " workflow."
	}
	argumentNotice := "This workflow accepts no arguments."
	if resource.Arguments.Mode == "freeform" {
		argumentNotice = "Treat $ARGUMENTS as the caller's free-form workflow input; preserve it without narrowing or reinterpretation."
	}
	encodedDescription, _ := json.Marshal(description)
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n> Codex degradation: invoke this workflow as `$%s`; this does not provide or claim `/%s`.\n\n%s\n\n%s\n", name, encodedDescription, name, name, argumentNotice, strings.TrimSpace(string(prompt)))
}

func (a *SurfaceAdapter) consumerAssetProjections(pack capabilitypack.Pack, consumer capabilitypack.Resource, consumerID, targetDir string) ([]capabilitypack.ObservedProjection, error) {
	byIdentity := make(map[string]capabilitypack.Resource, len(pack.Resources))
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
		projection, _, err := fileProjection(id, capabilitypack.ActionCodexAssetFile, filepath.Join(targetDir, filename), string(content), fmt.Sprintf("materialize dependency asset %s for %s", asset.ID, consumerID))
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
	return kind == capabilitypack.ActionSkillLink || kind == capabilitypack.ActionCodexWorkflowSkill || kind == capabilitypack.ActionCodexAssetFile
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
		"review and trust the Engram integration in Codex through /hooks; Packy will not bypass hook trust",
		"reload Codex so the configured Engram MCP server becomes available at runtime",
	}
}
