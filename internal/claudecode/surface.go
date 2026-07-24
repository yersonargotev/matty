package claudecode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

const (
	ActionSkillLink               capabilitypack.ProjectionActionKind = "claude-skill-link"
	ActionSkillFile               capabilitypack.ProjectionActionKind = "claude-skill-file"
	ActionSkillTree               capabilitypack.ProjectionActionKind = "claude-skill-tree"
	ActionInstructionContribution capabilitypack.ProjectionActionKind = "claude-instruction-contribution"
	ActionAgentFile               capabilitypack.ProjectionActionKind = "claude-agent-file"
	ActionCommandHook             capabilitypack.ProjectionActionKind = "claude-command-hook"
	ActionUserMCP                 capabilitypack.ProjectionActionKind = "claude-user-mcp"
)

type RuntimeEvidenceObserver interface {
	ObserveRuntimeEvidence(context.Context) []RuntimeEvidence
}

type SurfaceAdapter struct {
	layout                            CanonicalLayout
	bundleRoot, stateRoot, executable string
	runner                            Runner
	ownership                         OwnershipSnapshotProvider
	authorization                     AuthorizationObserver
	runtimeEvidence                   RuntimeEvidenceObserver
}

func (a *SurfaceAdapter) WithRuntimeEvidence(observer RuntimeEvidenceObserver) *SurfaceAdapter {
	a.runtimeEvidence = observer
	return a
}

func NewSurfaceAdapterWithAuthorization(bundleRoot string, layout CanonicalLayout, stateRoot, executable string, runner Runner, ownership OwnershipSnapshotProvider, authorization AuthorizationObserver) *SurfaceAdapter {
	a := NewSurfaceAdapter(bundleRoot, layout, stateRoot, executable, runner, ownership)
	a.authorization = authorization
	return a
}

func NewSurfaceAdapter(bundleRoot string, layout CanonicalLayout, stateRoot, executable string, runner Runner, ownership OwnershipSnapshotProvider) *SurfaceAdapter {
	return &SurfaceAdapter{bundleRoot: bundleRoot, layout: layout, stateRoot: stateRoot, executable: executable, runner: runner, ownership: ownership}
}

func (a *SurfaceAdapter) InspectSurface(ctx context.Context, transition capabilitypack.SurfaceTransition) (capabilitypack.SurfaceInspection, error) {
	ownership := OwnershipSnapshot{}
	if a.ownership != nil {
		var err error
		ownership, err = a.ownership.ObserveOwnership(ctx)
		if err != nil {
			return capabilitypack.SurfaceInspection{}, fmt.Errorf("observe Claude ownership: %w", err)
		}
	}
	pack := transition.Desired
	var result capabilitypack.SurfaceInspection
	revision := []string{}
	var instructionDocument, instructionOriginal []byte
	var instructionLoaded bool
	settingsObservation := ObserveSettings(a.layout.SettingsFile, nil)
	if settingsObservation.Err != nil {
		return result, settingsObservation.Err
	}
	settingsOriginal := append([]byte(nil), settingsObservation.Raw...)
	settingsDocument := append([]byte(nil), settingsOriginal...)
	hooksContainerCreated := ownedHooksContainerCreated(ownership, a.layout.SettingsFile)
	createdHookEvents := ownedHookEventsCreated(ownership, a.layout.SettingsFile)
	for _, r := range pack.Resources {
		b, ok := claudeBinding(r)
		if !ok {
			continue
		}
		id := r.Kind + ":" + b.Name
		switch b.Projection {
		case "skill":
			if isAddyCompositeProjection(pack, r, b) {
				composite, err := addyCompositeSkill(pack, r, b, a.bundleRoot)
				if err != nil {
					return result, err
				}
				payload, err := canonicalCompositeSkillPayload(composite)
				if err != nil {
					return result, err
				}
				provenance, err := canonicalCompositeOwnership(composite.Ownership)
				if err != nil {
					return result, err
				}
				target := filepath.Join(a.layout.SkillsDir, b.Name)
				observed, exists, err := observeCompositeTree(target)
				if err != nil {
					return result, err
				}
				action := capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillTree, Target: target, Content: string(payload), AdapterProvenance: provenance, Description: "write exact Addy Claude composite skill " + b.Name}
				result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: composite.TreeFingerprint, Action: action})
				revision = append(revision, id+observed)
				continue
			}
			if r.Kind == "command" {
				content, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(r.Source)))
				if err != nil {
					return result, err
				}
				target := filepath.Join(a.layout.SkillsDir, b.Name, "SKILL.md")
				rendered := claudeCommandSkill(r, b.Name, content)
				observed, exists, err := localprojection.FingerprintPath(target)
				if err != nil {
					return result, err
				}
				desired := localprojection.FingerprintBytes([]byte(rendered))
				result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillFile, Target: target, Content: rendered, Description: "write Claude Code personal command skill " + b.Name}})
				revision = append(revision, id+observed)
				assets, err := a.consumerAssets(pack, r, id, filepath.Dir(target))
				if err != nil {
					return result, err
				}
				for _, asset := range assets {
					result.Projections = append(result.Projections, asset)
					revision = append(revision, asset.ID+asset.ObservedFingerprint)
				}
				continue
			}
			source := filepath.Join(a.bundleRoot, filepath.Clean(r.Source))
			if err := a.validateSkillAssetClosure(pack, r, source); err != nil {
				return result, err
			}
			expectedSource, err := canonicalPath(source)
			if err != nil {
				return result, err
			}
			desired, err := localprojection.FingerprintTree(source)
			if err != nil {
				return result, err
			}
			target := filepath.Join(a.layout.SkillsDir, b.Name)
			skill := ObserveSkill(target, expectedSource)
			if skill.Err != nil {
				return result, skill.Err
			}
			exists := skill.Kind != PathMissing
			observed := "missing"
			if exists {
				observed = skill.TreeFingerprint
				if observed == "" {
					observed = string(skill.Kind)
				}
			}
			action := capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillLink, Source: source, Target: target, Description: "link Claude Code skill " + b.Name}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: action})
			revision = append(revision, id+observed)
		case "instruction":
			content, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(r.Source)))
			if err != nil {
				return result, err
			}
			if !instructionLoaded {
				instructionOriginal, err = readOptional(a.layout.InstructionsFile)
				if err != nil {
					return result, fmt.Errorf("read Claude instructions: %w", err)
				}
				instructionDocument = append([]byte(nil), instructionOriginal...)
				instructionLoaded = true
			}
			merged, err := UpsertInstructionContribution(string(instructionDocument), InstructionContribution{ContributorID: "pack:" + pack.ID + ":" + r.ID, Content: string(content)})
			if err != nil {
				return result, err
			}
			instructionDocument = []byte(merged)
			desired := Fingerprint([]byte(strings.TrimSpace(string(content))))
			action := capabilitypack.ProjectionAction{ID: id, Kind: ActionInstructionContribution, Target: a.layout.InstructionsFile, Command: Fingerprint(instructionOriginal), Description: "merge Claude Code instruction " + r.ID}
			io := ObserveInstructions(a.layout.InstructionsFile)
			if io.Err != nil {
				return result, io.Err
			}
			contributor := "pack:" + pack.ID + ":" + r.ID
			observed, exists := io.Contributions[contributor]
			if !exists {
				observed = "missing"
			}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: action})
			revision = append(revision, "instructions="+Fingerprint(instructionOriginal))
		case "agent":
			content, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(r.Source)))
			if err != nil {
				return result, err
			}
			content, err = a.embedConsumerAssets(pack, r, content)
			if err != nil {
				return result, err
			}
			target := filepath.Join(a.layout.AgentsDir, b.Name+".md")
			observed, exists, err := localprojection.FingerprintPath(target)
			if err != nil {
				return result, err
			}
			if b.AgentAuthority == nil && (len(r.Tools) > 0 || len(r.Permissions) > 0) {
				return result, fmt.Errorf("Claude agent %s is missing explicit authority translations", r.ID)
			}
			authority := capabilitypack.AgentAuthority{}
			if b.AgentAuthority != nil {
				authority = *b.AgentAuthority
				content = []byte(claudeAgentDocument(r, b.Name, authority, content))
			}
			desired := localprojection.FingerprintBytes(content)
			current, err := readOptional(target)
			if err != nil {
				return result, err
			}
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionAgentFile, Target: target, Content: string(content), Command: Fingerprint(current), Description: "write Claude Code agent " + b.Name}})
			revision = append(revision, id+observed)
		case "command_hook":
			if b.Hook == nil {
				return result, errors.New("Claude command hook is missing typed definition")
			}
			hook := fromBindingHook(b)
			settings := settingsDocument
			provenance := HookMergeProvenance{CreatedHooksContainer: hooksContainerCreated, CreatedEvent: createdHookEvents[hook.Event]}
			for _, record := range ownership.Records {
				if record.ID == id && record.Kind == string(ActionCommandHook) {
					provenance.CreatedEvent = provenance.CreatedEvent || ParseHookMergeProvenance(record.HookProvenance).CreatedEvent
				}
			}
			merged, provenance, err := MergeCommandHookWithProvenance(settings, hook, false, provenance)
			if err != nil {
				return result, err
			}
			hooksContainerCreated = hooksContainerCreated || provenance.CreatedHooksContainer
			createdHookEvents[hook.Event] = createdHookEvents[hook.Event] || provenance.CreatedEvent
			settingsDocument = append([]byte(nil), merged...)
			ho := EnrichHookObservation(settingsObservation, hook)
			if ho.Err != nil {
				return result, ho.Err
			}
			observed := "missing"
			if len(ho.MatchingEntries) > 0 {
				observed = HookOwnershipFingerprint(hook.Event, ho.EntryFingerprint)
			}
			desired := HookOwnershipFingerprint(hook.Event, canonicalFingerprint(hookJSON(hook)))
			description := fmt.Sprintf("configure Claude Code command hook %s: event=%s matcher=%q command=%s %s timeout=%ds blocking=%t failure=%s authorities=%s", b.Name, b.Hook.Event, b.Hook.Matcher, b.Hook.Command, strings.Join(b.Hook.Args, " "), b.Hook.TimeoutSeconds, b.Hook.Blocking, b.Hook.Failure, strings.Join(b.Hook.Authorities, ","))
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: len(ho.MatchingEntries) > 0, ObservedFingerprint: observed, DesiredFingerprint: desired, ExternallyManaged: ho.Disabled || ho.Shadowed, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionCommandHook, Target: a.layout.SettingsFile, Content: string(merged), Source: provenance.Seal(), Command: Fingerprint(settingsOriginal), Consent: capabilitypack.ConsentExecutableExternal, Description: description}})
			revision = append(revision, "settings="+Fingerprint(settingsOriginal))
		case "mcp_server":
			o := ObserveUserMCP(a.layout.UserMCPFile, b.Name)
			if o.Err != nil {
				return result, fmt.Errorf("observe Claude user MCP %s: %w", b.Name, o.Err)
			}
			identity := NewMCPIdentity(b.Name, r.Command, r.Args, map[string]string{})
			args := []string{"mcp", "add", b.Name, "--scope", "user", "--", r.Command}
			args = append(args, r.Args...)
			description := "configure redacted Claude Code user MCP through official command: claude " + strings.Join(args, " ") + " (environment values redacted)"
			result.Projections = append(result.Projections, capabilitypack.ObservedProjection{ID: id, Exists: o.Present, ObservedFingerprint: o.DefinitionFingerprint, DesiredFingerprint: canonicalFingerprint(identity), Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionUserMCP, Target: b.Name, Command: a.executable, Args: args, Content: canonicalFingerprint(identity), Consent: capabilitypack.ConsentExecutableExternal, Description: description}})
			revision = append(revision, id+o.DefinitionFingerprint)
		}
	}
	if transition.Prior.ID != "" {
		for _, r := range transition.Prior.Resources {
			b, ok := claudeBinding(r)
			if !ok || resourcePresent(transition.Desired, r.Kind, r.ID) {
				continue
			}
			projection, part, err := a.inspectRemoval(transition.Prior, r, b, ownership)
			if err != nil {
				return result, err
			}
			if b.Projection == "instruction" {
				if !instructionLoaded {
					instructionOriginal, err = readOptional(a.layout.InstructionsFile)
					if err != nil {
						return result, fmt.Errorf("read Claude instructions: %w", err)
					}
					instructionDocument = append([]byte(nil), instructionOriginal...)
					instructionLoaded = true
				}
				merged, err := RemoveInstructionContribution(string(instructionDocument), "pack:"+transition.Prior.ID+":"+r.ID)
				if err != nil {
					return result, err
				}
				instructionDocument = []byte(merged)
			}
			result.Projections = append(result.Projections, projection)
			revision = append(revision, part)
			if r.Kind == "command" && b.Projection == "skill" && !isAddyCompositeProjection(transition.Prior, r, b) {
				assets, err := a.consumerAssets(transition.Prior, r, projection.ID, filepath.Join(a.layout.SkillsDir, b.Name))
				if err != nil {
					return result, err
				}
				for _, asset := range assets {
					asset.Goal = capabilitypack.ProjectionAbsent
					asset.DesiredFingerprint = ""
					asset.Action.Content = ""
					asset.Action.Mode = capabilitypack.ProjectionDeleteTarget
					asset.Action.Description = "remove Claude command dependency asset " + asset.ID
					result.Projections = append(result.Projections, asset)
					revision = append(revision, asset.ID+asset.ObservedFingerprint)
				}
			}
		}
	}
	if instructionLoaded {
		for i := range result.Projections {
			if result.Projections[i].Action.Kind == ActionInstructionContribution {
				result.Projections[i].Action.Content = string(instructionDocument)
				result.Projections[i].Action.Command = Fingerprint(instructionOriginal)
			}
		}
	}
	for i := range result.Projections {
		if result.Projections[i].Action.Kind == ActionCommandHook && result.Projections[i].Goal != capabilitypack.ProjectionAbsent {
			result.Projections[i].Action.Content = string(settingsDocument)
			result.Projections[i].Action.Command = Fingerprint(settingsOriginal)
		}
	}
	for i := range result.Projections {
		if result.Projections[i].Goal == "" {
			result.Projections[i].Goal = capabilitypack.ProjectionPresent
		}
	}
	sort.Strings(revision)
	result.Revision = Fingerprint([]byte(strings.Join(revision, "\n")))
	version := ObserveVersion(ctx, a.executable, a.runner)
	versionClass := ClassifyVersion(version)
	supported := versionClass == CompatibilitySupported
	versionObserved := versionClass != CompatibilityUnreadable && versionClass != CompatibilityFailed && versionClass != CompatibilityTimedOut
	configured := true
	for _, p := range result.Projections {
		if p.Goal == capabilitypack.ProjectionPresent && (!p.Exists || p.ObservedFingerprint != p.DesiredFingerprint || p.ExternallyManaged) {
			configured = false
		}
	}
	auth := AuthorizationObservation{}
	if a.authorization != nil {
		auth = a.authorization.ObserveAuthorization(ctx)
	}
	authorizationObserved := versionObserved && auth.Err == nil && auth.PolicyObserved && auth.ToolPermissionObserved
	authorized := authorizationObserved && configured && supported && !auth.Disabled && !auth.Shadowed
	pending := []string{}
	if !supported {
		pending = append(pending, ClassifyVersion(version).Remediation())
	}
	if auth.Err != nil || !auth.PolicyObserved || !auth.ToolPermissionObserved || auth.Disabled || auth.Shadowed {
		pending = append(pending, "provide explicit observable Claude Code policy and tool-permission evidence")
	}
	if !configured {
		pending = append(pending, "converge every exact desired Claude Code projection")
	}
	usable, usabilityObserved, runtimeFacts := a.runtimeReadiness(ctx, pack, result.Projections, version, auth)
	if !usabilityObserved {
		pending = append(pending, "supply explicit current Claude Code runtime evidence for every included resource")
	}
	result.Readiness = capabilitypack.ReadinessObservation{AuthorizationObserved: authorizationObserved, Authorized: authorized, UsabilityObserved: usabilityObserved, Usable: authorized && usable, PendingHumanActions: pending, Evidence: append([]string{"filesystem and static user MCP definitions inspected; runtime use was not invoked"}, runtimeFacts...)}
	return result, nil
}

func ownedHooksContainerCreated(snapshot OwnershipSnapshot, target string) bool {
	for _, record := range snapshot.Records {
		if record.Kind != string(ActionCommandHook) || filepath.Clean(record.Target) != filepath.Clean(target) || record.HookProvenance == "" {
			continue
		}
		if ParseHookMergeProvenance(record.HookProvenance).CreatedHooksContainer {
			return true
		}
	}
	return false
}

func ownedHookEventsCreated(snapshot OwnershipSnapshot, target string) map[string]bool {
	result := map[string]bool{}
	for _, record := range snapshot.Records {
		if record.Kind == string(ActionCommandHook) && filepath.Clean(record.Target) == filepath.Clean(target) && record.HookEvent != "" && ParseHookMergeProvenance(record.HookProvenance).CreatedEvent {
			result[record.HookEvent] = true
		}
	}
	return result
}

func resourcePresent(pack capabilitypack.Pack, kind, id string) bool {
	for _, r := range pack.Resources {
		if r.Kind == kind && r.ID == id {
			_, ok := claudeBinding(r)
			return ok
		}
	}
	return false
}
func (a *SurfaceAdapter) inspectRemoval(pack capabilitypack.Pack, r capabilitypack.Resource, b capabilitypack.Binding, ownership OwnershipSnapshot) (capabilitypack.ObservedProjection, string, error) {
	id := r.Kind + ":" + b.Name
	switch b.Projection {
	case "skill":
		if isAddyCompositeProjection(pack, r, b) {
			target := filepath.Join(a.layout.SkillsDir, b.Name)
			fp, exists, err := observeCompositeTree(target)
			return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillTree, Target: target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove exact Addy Claude composite skill " + b.Name}}, id + fp, err
		}
		if r.Kind == "command" {
			target := filepath.Join(a.layout.SkillsDir, b.Name, "SKILL.md")
			fp, exists, err := localprojection.FingerprintPath(target)
			return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillFile, Target: target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove Claude Code personal command skill " + b.Name}}, id + fp, err
		}
		target := filepath.Join(a.layout.SkillsDir, b.Name)
		fp, exists, err := localprojection.FingerprintPath(target)
		return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillLink, Target: target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove Claude Code skill " + b.Name}}, id + fp, err
	case "agent":
		target := filepath.Join(a.layout.AgentsDir, b.Name+".md")
		fp, exists, err := localprojection.FingerprintPath(target)
		return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionAgentFile, Target: target, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove Claude Code agent " + b.Name}}, id + fp, err
	case "instruction":
		current, err := readOptional(a.layout.InstructionsFile)
		if err != nil {
			return capabilitypack.ObservedProjection{}, "", err
		}
		contributor := "pack:" + pack.ID + ":" + r.ID
		merged, err := RemoveInstructionContribution(string(current), contributor)
		if err != nil {
			return capabilitypack.ObservedProjection{}, "", err
		}
		o := ObserveInstructions(a.layout.InstructionsFile)
		if o.Err != nil {
			return capabilitypack.ObservedProjection{}, "", o.Err
		}
		fp, exists := o.Contributions[contributor]
		if !exists {
			fp = "missing"
		}
		return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionInstructionContribution, Target: a.layout.InstructionsFile, Content: merged, Command: Fingerprint(current), Mode: capabilitypack.ProjectionRemoveContent, Description: "remove Claude Code instruction " + r.ID}}, id + fp, nil
	case "command_hook":
		if b.Hook == nil {
			return capabilitypack.ObservedProjection{}, "", errors.New("missing typed hook")
		}
		settingsObservation := ObserveSettings(a.layout.SettingsFile, nil)
		if settingsObservation.Err != nil {
			return capabilitypack.ObservedProjection{}, "", settingsObservation.Err
		}
		settings := settingsObservation.Raw
		hook := fromBindingHook(b)
		provenance := HookMergeProvenance{}
		for _, record := range ownership.Records {
			if record.ID == id && record.Kind == string(ActionCommandHook) {
				provenance = ParseHookMergeProvenance(record.HookProvenance)
			}
		}
		merged, _, err := MergeCommandHookWithProvenance(settings, hook, true, provenance)
		if err != nil {
			return capabilitypack.ObservedProjection{}, "", err
		}
		o := EnrichHookObservation(settingsObservation, hook)
		if o.Err != nil {
			return capabilitypack.ObservedProjection{}, "", o.Err
		}
		fp := "missing"
		exists := len(o.MatchingEntries) > 0
		if exists {
			fp = HookOwnershipFingerprint(hook.Event, o.EntryFingerprint)
		}
		return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: exists, ObservedFingerprint: fp, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionCommandHook, Target: a.layout.SettingsFile, Content: string(merged), Source: provenance.Seal(), Command: Fingerprint(settings), Mode: capabilitypack.ProjectionRemoveContent, Description: "remove Claude Code hook " + b.Name}}, id + fp, nil
	case "mcp_server":
		o := ObserveUserMCP(a.layout.UserMCPFile, b.Name)
		if o.Err != nil {
			return capabilitypack.ObservedProjection{}, "", o.Err
		}
		return capabilitypack.ObservedProjection{ID: id, Goal: capabilitypack.ProjectionAbsent, Exists: o.Present, ObservedFingerprint: o.DefinitionFingerprint, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionUserMCP, Target: b.Name, Command: a.executable, Args: []string{"mcp", "remove", b.Name, "--scope", "user"}, Mode: capabilitypack.ProjectionDeleteTarget, Description: "remove redacted Claude Code user MCP " + b.Name}}, id + o.DefinitionFingerprint, nil
	}
	return capabilitypack.ObservedProjection{}, "", fmt.Errorf("unsupported Claude projection %q", b.Projection)
}

func (a *SurfaceAdapter) ApplyProjections(ctx context.Context, actions []capabilitypack.ProjectionAction) *capabilitypack.ProjectionActionError {
	if err := a.validateActions(actions); err != nil {
		return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
	}
	unlock, err := a.lock()
	if err != nil {
		return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
	}
	defer unlock()
	snapshot := OwnershipSnapshot{}
	if a.ownership != nil {
		snapshot, err = a.ownership.ObserveOwnership(ctx)
		if err != nil {
			return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: fmt.Errorf("observe fresh Claude ownership: %w", err)}
		}
	}
	if id, err := a.preflight(actions, snapshot); err != nil {
		return &capabilitypack.ProjectionActionError{ID: id, Err: err}
	}
	treeChanges := make([]localprojection.TreeChange, 0)
	for _, action := range actions {
		if action.Kind != ActionSkillTree {
			continue
		}
		change := localprojection.TreeChange{ID: action.ID, Target: action.Target, Delete: action.Mode == capabilitypack.ProjectionDeleteTarget}
		if !change.Delete {
			composite, err := decodeCompositeSkillPayload([]byte(action.Content))
			if err != nil {
				return &capabilitypack.ProjectionActionError{ID: action.ID, Err: err}
			}
			change.Files = compositeTreeFiles(composite)
			change.ExpectedFingerprint = composite.TreeFingerprint
		}
		treeChanges = append(treeChanges, change)
	}
	if err := localprojection.ReplaceTrees(treeChanges); err != nil {
		var actionErr capabilitypack.ProjectionActionError
		if errors.As(err, &actionErr) {
			return &actionErr
		}
		return &capabilitypack.ProjectionActionError{ID: firstID(actions), Err: err}
	}
	appliedShared := map[string]string{}
	for _, action := range actions {
		if action.Kind == ActionSkillTree {
			continue
		}
		if action.Kind == ActionInstructionContribution || action.Kind == ActionCommandHook {
			if content, ok := appliedShared[action.Target]; ok && content == action.Content {
				continue
			}
			appliedShared[action.Target] = action.Content
		}
		if err := a.apply(ctx, action); err != nil {
			return &capabilitypack.ProjectionActionError{ID: action.ID, Err: err}
		}
	}
	return nil
}
func (a *SurfaceAdapter) validateActions(actions []capabilitypack.ProjectionAction) error {
	seen := map[string]bool{}
	targets := map[string]capabilitypack.ProjectionAction{}
	for _, x := range actions {
		if x.ID == "" || seen[x.ID] {
			return errors.New("duplicate or empty Claude projection identity")
		}
		seen[x.ID] = true
		switch x.Kind {
		case ActionSkillLink:
			if !directChild(x.Target, a.layout.SkillsDir) {
				return errors.New("Claude skill target must be a direct child of the canonical skills directory")
			}
			if x.Mode != capabilitypack.ProjectionDeleteTarget {
				info, err := os.Stat(x.Source)
				if err != nil || !info.IsDir() {
					return errors.New("Claude skill source must be an existing directory")
				}
				if !filepath.IsAbs(x.Source) {
					return errors.New("Claude skill source must be absolute")
				}
			}
			if exclusiveSkillTargetConflict(targets, x) {
				return errors.New("overlapping exclusive Claude skill target")
			}
			targets[x.Target] = x
		case ActionSkillTree:
			if !directChild(x.Target, a.layout.SkillsDir) {
				return errors.New("Claude composite skill target must be a direct child of the canonical skills directory")
			}
			if x.Mode == capabilitypack.ProjectionDeleteTarget {
				if x.Content != "" || x.AdapterProvenance != "" {
					return errors.New("Claude composite skill removal must not carry replacement content")
				}
			} else {
				composite, err := decodeCompositeSkillPayload([]byte(x.Content))
				if err != nil {
					return err
				}
				provenance, err := decodeCompositeOwnership(x.AdapterProvenance)
				if err != nil {
					return err
				}
				if composite.Ownership != provenance || provenance.EffectiveName != filepath.Base(x.Target) || provenance.TreeFingerprint != composite.TreeFingerprint {
					return errors.New("Claude composite skill action identity does not match its target and payload")
				}
			}
			if exclusiveSkillTargetConflict(targets, x) {
				return errors.New("overlapping exclusive Claude skill target")
			}
			targets[x.Target] = x
		case ActionSkillFile:
			if filepath.Dir(filepath.Dir(filepath.Clean(x.Target))) != filepath.Clean(a.layout.SkillsDir) {
				return errors.New("Claude command skill file must be beneath one canonical personal skill directory")
			}
			if exclusiveSkillTargetConflict(targets, x) {
				return errors.New("overlapping exclusive Claude skill target")
			}
			targets[x.Target] = x
		case ActionAgentFile:
			if !directChild(x.Target, a.layout.AgentsDir) || filepath.Ext(x.Target) != ".md" {
				return errors.New("Claude agent target must be one Markdown file in the canonical agents directory")
			}
			if _, ok := targets[x.Target]; ok {
				return errors.New("overlapping exclusive Claude agent target")
			}
			targets[x.Target] = x
		case ActionInstructionContribution:
			if filepath.Clean(x.Target) != filepath.Clean(a.layout.InstructionsFile) {
				return errors.New("Claude instruction target must be canonical CLAUDE.md")
			}
			if prior, ok := targets[x.Target]; ok && prior.Content != x.Content {
				return errors.New("shared Claude instruction actions must be aggregated into one sealed document")
			}
			targets[x.Target] = x
		case ActionCommandHook:
			if filepath.Clean(x.Target) != filepath.Clean(a.layout.SettingsFile) {
				return errors.New("Claude hook target must be canonical settings.json")
			}
			if prior, ok := targets[x.Target]; ok && prior.Content != x.Content {
				return errors.New("shared Claude hook actions must be aggregated into one sealed document")
			}
			targets[x.Target] = x
		case ActionUserMCP:
			if x.Command == "" || a.runner == nil {
				return errors.New("Claude executable is required for user MCP effect")
			}
			name, remove := mcpActionName(x.Args)
			if name == "" || !hasUserScope(x.Args) {
				return errors.New("Claude user MCP action must be an official named add/remove with --scope user")
			}
			if remove && x.Mode != capabilitypack.ProjectionDeleteTarget {
				return errors.New("Claude user MCP removal must be sealed as delete-target")
			}
		default:
			return fmt.Errorf("unsealed Claude projection action kind %q", x.Kind)
		}
	}
	return nil
}
func (a *SurfaceAdapter) apply(ctx context.Context, x capabilitypack.ProjectionAction) error {
	if x.Mode == capabilitypack.ProjectionDeleteTarget && x.Kind != ActionUserMCP {
		if _, err := os.Lstat(x.Target); os.IsNotExist(err) {
			return nil
		}
	}
	if x.Kind == ActionInstructionContribution || x.Kind == ActionAgentFile || x.Kind == ActionSkillFile || x.Kind == ActionCommandHook {
		current, err := os.ReadFile(x.Target)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if x.Mode != capabilitypack.ProjectionDeleteTarget && string(current) == x.Content {
			return nil
		}
		if x.Command != "" && Fingerprint(current) != x.Command {
			return errors.New("stale Claude shared document revision")
		}
		if x.Mode == capabilitypack.ProjectionRemoveContent && string(current) == x.Content {
			return nil
		}
	}
	switch x.Kind {
	case ActionSkillLink:
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			return removeExact(x.Target)
		}
		expected, err := canonicalPath(x.Source)
		if err != nil {
			return err
		}
		sourceFP, err := localprojection.FingerprintTree(expected)
		if err != nil {
			return err
		}
		observed := ObserveSkill(x.Target, expected)
		if observed.Err != nil {
			return observed.Err
		}
		if observed.Kind == PathSymlink && observed.ResolvedTarget == expected && observed.TreeFingerprint == sourceFP {
			return nil
		}
		return replaceSymlink(x.Source, x.Target)
	case ActionSkillTree:
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			return removeExactTree(x.Target)
		}
		composite, err := decodeCompositeSkillPayload([]byte(x.Content))
		if err != nil {
			return err
		}
		return localprojection.ReplaceTree(x.Target, compositeTreeFiles(composite), composite.TreeFingerprint)
	case ActionAgentFile, ActionSkillFile, ActionInstructionContribution, ActionCommandHook:
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			return removeExact(x.Target)
		}
		return atomicWrite(x.Target, []byte(x.Content), 0644)
	case ActionUserMCP:
		name, remove := mcpActionName(x.Args)
		if remove {
			_, observed, err := observeMCPRemoval(a.layout.UserMCPFile, x.Args)
			if err != nil {
				return errors.New("Claude Code user MCP verification failed")
			}
			if !observed.Present {
				return nil
			}
		} else {
			observed := ObserveUserMCP(a.layout.UserMCPFile, name)
			if observed.Err != nil {
				return errors.New("Claude Code user MCP verification failed")
			}
			if observed.Present && x.Content != "" && observed.DefinitionFingerprint == x.Content {
				return nil
			}
		}
		r := a.runner.Run(ctx, Command{Executable: x.Command, Args: append([]string(nil), x.Args...), Timeout: 15_000_000_000, Description: x.Description})
		if r.TimedOut {
			return context.DeadlineExceeded
		}
		if r.Err != nil {
			return errors.New("Claude Code user MCP command failed")
		}
		if r.ExitCode != 0 {
			return fmt.Errorf("Claude Code user MCP command failed with status %d", r.ExitCode)
		}
		observed := ObserveUserMCP(a.layout.UserMCPFile, name)
		if observed.Err != nil {
			return errors.New("Claude Code user MCP verification failed")
		}
		if remove && observed.Present {
			return errors.New("Claude Code user MCP removal did not converge")
		}
		if !remove && !observed.Present {
			return errors.New("Claude Code user MCP addition did not converge")
		}
		if !remove && x.Content != "" && observed.DefinitionFingerprint != x.Content {
			return errors.New("Claude Code user MCP definition did not match the sealed action")
		}
		return nil
	}
	return nil
}

func (a *SurfaceAdapter) validateFreshOwnership(x capabilitypack.ProjectionAction, snapshot OwnershipSnapshot) error {
	if snapshot.Revision != "" && snapshot.Revision != canonicalFingerprint(snapshot.Records) {
		return errors.New("stale composite Claude ownership snapshot")
	}
	var record *OwnershipRecord
	ambiguous := false
	for i := range snapshot.Records {
		r := &snapshot.Records[i]
		if r.ID == x.ID {
			if record != nil {
				ambiguous = true
			}
			record = r
		}
	}
	if ambiguous {
		return errors.New("ambiguous Claude ownership")
	}
	if record != nil {
		if record.StateOwner == "" || record.ContributorID == "" || record.Kind != string(x.Kind) || filepath.Clean(record.Target) != filepath.Clean(x.Target) || !slices.Contains(record.Contributors, record.ContributorID) {
			return errors.New("Claude ownership identity does not exactly match the sealed action")
		}
		if x.Kind == ActionCommandHook && x.Mode == capabilitypack.ProjectionRemoveContent && record.HookProvenance != x.Source {
			return errors.New("Claude hook creation provenance does not match ownership")
		}
		if x.Kind == ActionInstructionContribution {
			o := ObserveInstructions(x.Target)
			if o.Err != nil {
				return o.Err
			}
			fp, ok := o.Contributions[record.ContributorID]
			if ok && !fingerprintsEqual(fp, record.Fingerprint) {
				return errors.New("owned Claude instruction contribution changed; preserving it")
			}
		}
		if x.Kind == ActionCommandHook {
			ok, err := settingsContainsFingerprint(x.Target, record.Fingerprint)
			if err != nil {
				return err
			}
			if !ok && x.Mode != capabilitypack.ProjectionDeleteTarget && x.Mode != capabilitypack.ProjectionRemoveContent {
				return errors.New("owned Claude command hook changed; preserving it")
			}
		}
	}
	if x.Kind == ActionSkillTree {
		observed, exists, err := freshCompositeTreeFingerprint(x.Target)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		if record == nil {
			return errors.New("foreign Claude composite skill collision")
		}
		recordProvenance, err := canonicalCompositeOwnership(record.Composite)
		if err != nil {
			return err
		}
		if _, err := decodeCompositeOwnership(recordProvenance); err != nil || record.Composite.TreeFingerprint != record.Fingerprint {
			return errors.New("invalid Claude composite ownership identity")
		}
		if !fingerprintsEqual(observed, record.Fingerprint) {
			return errors.New("owned Claude composite skill changed; preserving it")
		}
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			if !record.DeletionAuthorized || len(record.Contributors) > 1 {
				return errors.New("Claude composite skill deletion is not authorized")
			}
			return nil
		}
		next, err := decodeCompositeOwnership(x.AdapterProvenance)
		if err != nil {
			return err
		}
		if record.Composite.PackID != next.PackID || record.Composite.PortableKind != next.PortableKind || record.Composite.PortableID != next.PortableID || record.Composite.EffectiveName != next.EffectiveName || record.Composite.TargetType != next.TargetType {
			return errors.New("Claude composite replacement changes its owned portable identity")
		}
		return nil
	}
	if x.Kind == ActionUserMCP {
		if x.Mode == capabilitypack.ProjectionDeleteTarget && (record == nil || !record.DeletionAuthorized || len(record.Contributors) > 1) {
			return errors.New("Claude user MCP deletion is not authorized by the fresh composite ownership snapshot")
		}
		return nil
	}
	if x.Kind == ActionSkillLink {
		o := ObserveSkill(x.Target, "")
		if o.Err != nil {
			return o.Err
		}
		exists := o.Kind != PathMissing
		if !exists {
			return nil
		}
		if record == nil {
			return errors.New("foreign Claude skill collision")
		}
		if !record.MatchesSkill("claude", x.ID, x.Target, record.Skill.ExpectedSource, o) {
			return errors.New("owned Claude skill identity changed; preserving it")
		}
		if x.Mode == capabilitypack.ProjectionDeleteTarget {
			if !record.DeletionAuthorized || len(record.Contributors) > 1 {
				return errors.New("Claude skill deletion is not authorized")
			}
			return nil
		}
		if _, err := canonicalPath(x.Source); err != nil {
			return err
		}
		return nil
	}
	fp, exists, err := localprojection.FingerprintPath(x.Target)
	if err != nil {
		return err
	}
	if x.Mode == capabilitypack.ProjectionDeleteTarget {
		if !exists {
			return nil
		}
		if record == nil || !record.DeletionAuthorized || len(record.Contributors) > 1 {
			return errors.New("Claude deletion is not authorized by the fresh composite ownership snapshot")
		}
		if !fingerprintsEqual(fp, record.Fingerprint) {
			return errors.New("owned Claude projection changed; preserving it")
		}
		if x.Kind == ActionInstructionContribution || x.Kind == ActionCommandHook {
			return errors.New("shared Claude documents cannot be deleted as exclusive targets")
		}
		return nil
	}
	if exists && (x.Kind == ActionSkillLink || x.Kind == ActionAgentFile) {
		if record == nil {
			return errors.New("foreign Claude projection collision")
		}
		if !fingerprintsEqual(fp, record.Fingerprint) {
			return errors.New("owned Claude projection changed; preserving it")
		}
	}
	return nil
}

func (a *SurfaceAdapter) preflight(actions []capabilitypack.ProjectionAction, snapshot OwnershipSnapshot) (string, error) {
	for _, x := range actions {
		if x.Kind == ActionUserMCP && x.Mode == capabilitypack.ProjectionDeleteTarget {
			_, o, err := observeMCPRemoval(a.layout.UserMCPFile, x.Args)
			if err != nil {
				return x.ID, err
			}
			if !o.Present {
				continue
			}
		}
		if err := a.validateFreshOwnership(x, snapshot); err != nil {
			return x.ID, err
		}
		if x.Mode == capabilitypack.ProjectionDeleteTarget && x.Kind != ActionUserMCP {
			if _, err := os.Lstat(x.Target); os.IsNotExist(err) {
				continue
			}
		}
		if x.Kind == ActionInstructionContribution || x.Kind == ActionAgentFile || x.Kind == ActionSkillFile || x.Kind == ActionCommandHook {
			current, err := readOptional(x.Target)
			if err != nil {
				return x.ID, err
			}
			if x.Command != "" && Fingerprint(current) != x.Command && string(current) != x.Content {
				return x.ID, errors.New("stale Claude shared document revision")
			}
		}
		if x.Kind == ActionUserMCP {
			name, _ := mcpActionName(x.Args)
			o := ObserveUserMCP(a.layout.UserMCPFile, name)
			if o.Err != nil {
				return x.ID, o.Err
			}
			var record *OwnershipRecord
			for i := range snapshot.Records {
				if snapshot.Records[i].ID == x.ID {
					record = &snapshot.Records[i]
				}
			}
			if o.Present && record == nil {
				return x.ID, errors.New("foreign Claude user MCP collision")
			}
			if o.Present && record != nil && !fingerprintsEqual(o.DefinitionFingerprint, record.Fingerprint) {
				return x.ID, errors.New("owned Claude user MCP changed; preserving it")
			}
		}
	}
	return "", nil
}

func (a *SurfaceAdapter) runtimeReadiness(ctx context.Context, pack capabilitypack.Pack, projections []capabilitypack.ObservedProjection, version VersionObservation, auth AuthorizationObservation) (bool, bool, []string) {
	if a.runtimeEvidence == nil {
		return false, false, nil
	}
	evidence := a.runtimeEvidence.ObserveRuntimeEvidence(ctx)
	policy := canonicalFingerprint(struct{ Disabled, Shadowed, Policy, Tools bool }{auth.Disabled, auth.Shadowed, auth.PolicyObserved, auth.ToolPermissionObserved})
	byID := make(map[string]RuntimeEvidence, len(evidence))
	for _, item := range evidence {
		byID[item.ID] = item
	}
	facts := []string{}
	participants := 0
	for _, projection := range projections {
		if projection.Goal != capabilitypack.ProjectionPresent || strings.HasPrefix(projection.ID, "asset:") {
			continue
		}
		participants++
		signal := "loading"
		switch projection.Action.Kind {
		case ActionUserMCP:
			signal = "connection"
		case ActionCommandHook:
			signal = "firing"
		}
		item, ok := byID[projection.ID]
		wantRevision := runtimeEvidenceRevision(pack, projection, version.Version, policy)
		if !ok || item.Kind != string(projection.Action.Kind) || item.Signal != signal || item.Revision != wantRevision {
			return false, false, facts
		}
		facts = append(facts, projection.ID+": current Claude Code runtime "+signal+" evidence")
	}
	if participants == 0 {
		return false, false, nil
	}
	sort.Strings(facts)
	return true, true, facts
}

func runtimeEvidenceRevision(pack capabilitypack.Pack, projection capabilitypack.ObservedProjection, hostVersion, policy string) string {
	return canonicalFingerprint(struct{ PackID, PackVersion, ProjectionID, Projection, Definition, HostVersion, Policy string }{pack.ID, pack.Version, projection.ID, projection.ObservedFingerprint, projection.DesiredFingerprint, hostVersion, policy})
}

func NewRuntimeEvidence(pack capabilitypack.Pack, projection capabilitypack.ObservedProjection, hostVersion string, auth AuthorizationObservation, signal string) RuntimeEvidence {
	return RuntimeEvidence{Kind: string(projection.Action.Kind), ID: projection.ID, Signal: signal, Revision: runtimeEvidenceRevision(pack, projection, hostVersion, RuntimeEvidencePolicyFingerprint(auth))}
}

func RuntimeEvidencePolicyFingerprint(auth AuthorizationObservation) string {
	return canonicalFingerprint(struct{ Disabled, Shadowed, Policy, Tools bool }{auth.Disabled, auth.Shadowed, auth.PolicyObserved, auth.ToolPermissionObserved})
}

func claudeCommandSkill(resource capabilitypack.Resource, name string, prompt []byte) string {
	description := resource.Description
	if description == "" {
		description = "Run the " + name + " command."
	}
	return fmt.Sprintf("---\nname: %s\ndescription: %q\n---\n\n%s\n", name, description, strings.TrimSpace(string(prompt)))
}

func claudeAgentDocument(resource capabilitypack.Resource, name string, authority capabilitypack.AgentAuthority, prompt []byte) string {
	toolSet := map[string]bool{}
	contract := make([]string, 0, len(authority.Authorities))
	for _, record := range authority.Authorities {
		for _, tool := range record.ClaudeTools {
			toolSet[tool] = true
		}
		declarations := strings.Join(record.Declarations, ", ")
		if declarations == "" {
			declarations = "none"
		}
		claudeTools := strings.Join(record.ClaudeTools, ", ")
		if claudeTools == "" {
			claudeTools = "none"
		}
		contract = append(contract, fmt.Sprintf("- %s: declarations=[%s]; outcome=%s; claude_tools=[%s]; fallback=%s", record.Portable, declarations, record.Outcome, claudeTools, record.Fallback))
	}
	claudeTools := make([]string, 0, len(toolSet))
	for tool := range toolSet {
		claudeTools = append(claudeTools, tool)
	}
	sort.Strings(claudeTools)
	tools := strings.Join(claudeTools, ", ")
	if tools == "" {
		tools = "[]"
	}
	return fmt.Sprintf("---\nname: %s\ndescription: %q\npermissionMode: %s\ntools: %s\n---\n\n## Packy authority contract\n\n- permission_mode: %s\n%s\n\n%s\n", name, resource.Description, authority.PermissionMode, tools, authority.PermissionMode, strings.Join(contract, "\n"), strings.TrimSpace(string(prompt)))
}

func (a *SurfaceAdapter) consumerAssets(pack capabilitypack.Pack, consumer capabilitypack.Resource, consumerID, targetDir string) ([]capabilitypack.ObservedProjection, error) {
	resources := map[string]capabilitypack.Resource{}
	for _, resource := range pack.Resources {
		resources[resource.Kind+":"+resource.ID] = resource
	}
	seen := map[string]bool{}
	assets := []capabilitypack.Resource{}
	var visit func(string)
	visit = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		r, ok := resources[id]
		if !ok {
			return
		}
		if r.Kind == "asset" {
			assets = append(assets, r)
		}
		for _, dependency := range r.Requires {
			visit(dependency)
		}
	}
	for _, dependency := range consumer.Requires {
		visit(dependency)
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].ID < assets[j].ID })
	result := make([]capabilitypack.ObservedProjection, 0, len(assets))
	for _, asset := range assets {
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(asset.Source)))
		if err != nil {
			return nil, err
		}
		target := filepath.Join(targetDir, filepath.Base(asset.Source))
		observed, exists, err := localprojection.FingerprintPath(target)
		if err != nil {
			return nil, err
		}
		desired := localprojection.FingerprintBytes(content)
		id := "asset:" + consumerID + ":" + asset.ID
		result = append(result, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: ActionSkillFile, Target: target, Content: string(content), Description: "materialize Claude command dependency asset " + asset.ID}})
	}
	return result, nil
}

func (a *SurfaceAdapter) validateSkillAssetClosure(pack capabilitypack.Pack, consumer capabilitypack.Resource, source string) error {
	root, err := canonicalPath(source)
	if err != nil {
		return err
	}
	for _, asset := range dependencyAssets(pack, consumer) {
		path, err := canonicalPath(filepath.Join(a.bundleRoot, filepath.Clean(asset.Source)))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("Claude skill %s dependency asset %s is outside its complete source tree", consumer.ID, asset.ID)
		}
	}
	return nil
}

func (a *SurfaceAdapter) embedConsumerAssets(pack capabilitypack.Pack, consumer capabilitypack.Resource, prompt []byte) ([]byte, error) {
	assets := dependencyAssets(pack, consumer)
	if len(assets) == 0 {
		return prompt, nil
	}
	result := strings.TrimSpace(string(prompt))
	for _, asset := range assets {
		content, err := os.ReadFile(filepath.Join(a.bundleRoot, filepath.Clean(asset.Source)))
		if err != nil {
			return nil, err
		}
		result += fmt.Sprintf("\n\n## Packy dependency asset: %s\n\n%s", asset.ID, strings.TrimSpace(string(content)))
	}
	return []byte(result + "\n"), nil
}

func fingerprintsEqual(a, b string) bool { return a == b || "sha256:"+a == b || a == "sha256:"+b }
func directChild(path, root string) bool {
	return filepath.Dir(filepath.Clean(path)) == filepath.Clean(root) && filepath.Base(path) != "." && filepath.Base(path) != ".."
}
func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}
func mcpActionName(args []string) (string, bool) {
	if len(args) >= 3 && args[0] == "mcp" && (args[1] == "add" || args[1] == "remove") {
		return args[2], args[1] == "remove"
	}
	return "", false
}
func observeMCPRemoval(path string, args []string) (string, MCPObservation, error) {
	name, remove := mcpActionName(args)
	if !remove {
		return name, MCPObservation{}, errors.New("not a Claude user MCP removal")
	}
	o := ObserveUserMCP(path, name)
	return name, o, o.Err
}
func hasUserScope(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--scope" && args[i+1] == "user" {
			return true
		}
	}
	return false
}
func (a *SurfaceAdapter) lock() (func(), error) {
	if a.stateRoot == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(a.stateRoot, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(a.stateRoot, "claude-host-effect.lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("acquire Claude host-effect lock: %w", err)
	}
	f.Close()
	return func() { _ = os.Remove(p) }, nil
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".packy-claude-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func replaceSymlink(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp := target + ".packy-new"
	_ = os.Remove(tmp)
	if err := os.Symlink(source, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp)
	return os.Rename(tmp, target)
}
func removeExact(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func removeExactTree(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Claude composite skill target changed type; preserving it")
	}
	return os.RemoveAll(path)
}

func observeCompositeTree(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "type:symlink", true, nil
	}
	if !info.IsDir() {
		return "type:" + info.Mode().Type().String(), true, nil
	}
	fingerprint, err := localprojection.FingerprintExactTree(path)
	if err != nil {
		return "invalid:" + Fingerprint([]byte(err.Error())), true, nil
	}
	return fingerprint, true, nil
}

func freshCompositeTreeFingerprint(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", true, errors.New("Claude composite skill target has an unexpected path type")
	}
	fingerprint, err := localprojection.FingerprintExactTree(path)
	return fingerprint, true, err
}

func isAddyCompositeProjection(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding) bool {
	return pack.ID == "addy" && pack.Version == "1.1.0" && (resource.Kind == "skill" || resource.Kind == "command") && binding.Surface == capabilitypack.SurfaceClaude && binding.Projection == "skill"
}

func exclusiveSkillTargetConflict(targets map[string]capabilitypack.ProjectionAction, action capabilitypack.ProjectionAction) bool {
	target := filepath.Clean(action.Target)
	for existingTarget, existing := range targets {
		existingTarget = filepath.Clean(existingTarget)
		if existingTarget == target {
			return true
		}
		if action.Kind != ActionSkillTree && existing.Kind != ActionSkillTree {
			continue
		}
		if pathContains(target, existingTarget) || pathContains(existingTarget, target) {
			return true
		}
	}
	return false
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}
func settingsContainsFingerprint(path, fingerprint string) (bool, error) {
	settings := ObserveSettings(path, nil)
	if settings.Err != nil {
		return false, settings.Err
	}
	if len(settings.Raw) == 0 {
		return false, nil
	}
	for _, entry := range observedHookEntries(settings) {
		if fingerprintsEqual(HookOwnershipFingerprint(entry.event, entry.fingerprint), fingerprint) {
			return true, nil
		}
	}
	return false, nil
}
func firstID(a []capabilitypack.ProjectionAction) string {
	if len(a) > 0 {
		return a[0].ID
	}
	return "batch"
}
func claudeBinding(r capabilitypack.Resource) (capabilitypack.Binding, bool) {
	for _, b := range r.Bindings {
		if b.Surface == capabilitypack.SurfaceClaude {
			return b, true
		}
	}
	return capabilitypack.Binding{}, false
}
func fromBindingHook(b capabilitypack.Binding) CommandHookEntry {
	h := b.Hook
	return CommandHookEntry{Type: h.Type, Event: h.Event, Matcher: h.Matcher, Command: h.Command, Args: append([]string{}, h.Args...), TimeoutSeconds: h.TimeoutSeconds, Blocking: h.Blocking, Failure: h.Failure, Authorities: append([]string{}, h.Authorities...)}
}
