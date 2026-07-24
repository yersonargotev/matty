package claudecode

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

// CapabilityPackOwnershipProvider translates lifecycle-owned activation facts
// into Claude host identities. The CLI only wires its immutable catalog view.
type CapabilityPackOwnershipProvider struct {
	store      capabilitypack.ActivationStore
	packs      map[string]capabilitypack.Pack
	layout     CanonicalLayout
	bundleRoot string
}

func NewCapabilityPackOwnershipProvider(store capabilitypack.ActivationStore, packs map[string]capabilitypack.Pack, layout CanonicalLayout, bundleRoot string) CapabilityPackOwnershipProvider {
	return CapabilityPackOwnershipProvider{store: store, packs: packs, layout: layout, bundleRoot: bundleRoot}
}

func (o CapabilityPackOwnershipProvider) ObserveOwnership(ctx context.Context) (OwnershipSnapshot, error) {
	state, err := o.store.Load(ctx, capabilitypack.SurfaceClaude)
	if err != nil {
		return OwnershipSnapshot{}, err
	}
	intents := activeClaudeIntents(state)
	if len(intents) == 0 && state.Journal == nil {
		return NewOwnershipSnapshot(), nil
	}
	owners := make(map[string]capabilitypack.ProjectionOwnership, len(state.Ownership))
	for _, owner := range state.Ownership {
		owners[owner.ID] = owner
	}
	records := []OwnershipRecord{}
	recorded := map[string]bool{}
	recoveryActions := map[string]bool{}
	if state.Journal != nil {
		for _, id := range state.Journal.Actions {
			recoveryActions[id] = true
		}
	}
	for _, intent := range intents {
		pack, ok := o.packs[intent.PackID]
		if !ok || pack.Version != intent.Version {
			return OwnershipSnapshot{}, fmt.Errorf("Claude ownership intent %s@%s has no exact registered adapter contract", intent.PackID, intent.Version)
		}
		for _, portableResource := range pack.Resources {
			resource := resourceWithAliases(portableResource, intent.Aliases)
			if resource.Kind != "skill" && resource.Kind != "command" && resource.Kind != "instruction" && resource.Kind != "agent" && resource.Kind != "lifecycle" && resource.Kind != "mcp_server" {
				continue
			}
			name := resource.ID
			var binding *capabilitypack.Binding
			for _, candidate := range resource.Bindings {
				if candidate.Surface == capabilitypack.SurfaceClaude {
					name = candidate.Name
					copy := candidate
					binding = &copy
					break
				}
			}
			if binding == nil {
				continue
			}
			id := resource.Kind + ":" + name
			if recorded[id] {
				continue
			}
			owner, retained := owners[id]
			recoverable := state.Journal != nil && state.Journal.PackID == intent.PackID && recoveryActions[id]
			if !retained && !recoverable {
				continue
			}
			contributors := append([]string(nil), owner.Contributors...)
			if len(contributors) == 0 {
				contributors = []string{intent.PackID}
			}
			contributorID := intent.PackID
			if len(contributors) > 0 {
				contributorID = contributors[0]
			}
			record := OwnershipRecord{StateOwner: "capabilitypack", ContributorID: contributorID, ID: id, Fingerprint: owner.Fingerprint, Contributors: contributors, DeletionAuthorized: owner.DeletionAuthorized()}
			if isAddyCompositeProjection(pack, resource, *binding) {
				composite, err := addyCompositeSkill(pack, resource, *binding, o.bundleRoot)
				if err != nil {
					return OwnershipSnapshot{}, err
				}
				expectedProvenance, err := canonicalCompositeOwnership(composite.Ownership)
				if err != nil {
					return OwnershipSnapshot{}, err
				}
				if retained && owner.AdapterProvenance != expectedProvenance {
					return OwnershipSnapshot{}, errors.New("persisted Claude composite ownership does not match the exact Addy adapter contract")
				}
				record.Kind, record.Target = string(ActionSkillTree), filepath.Join(o.layout.SkillsDir, name)
				record.Fingerprint = composite.TreeFingerprint
				record.Composite = composite.Ownership
			} else if resource.Kind == "instruction" {
				record.Kind, record.Target = string(ActionInstructionContribution), o.layout.InstructionsFile
				observation := ObserveInstructions(record.Target)
				record.ContributorID = "pack:" + intent.PackID + ":" + resource.ID
				record.Contributors = []string{record.ContributorID}
				record.Fingerprint = observation.Contributions[record.ContributorID]
			} else if resource.Kind == "agent" {
				record.Kind, record.Target = string(ActionAgentFile), filepath.Join(o.layout.AgentsDir, name+".md")
				observed, _, err := localprojection.FingerprintPath(record.Target)
				if err != nil {
					return OwnershipSnapshot{}, err
				}
				record.Fingerprint = observed
			} else if resource.Kind == "command" {
				record.Kind, record.Target = string(ActionSkillFile), filepath.Join(o.layout.SkillsDir, name, "SKILL.md")
				observed, _, err := localprojection.FingerprintPath(record.Target)
				if err != nil {
					return OwnershipSnapshot{}, err
				}
				record.Fingerprint = observed
			} else if resource.Kind == "lifecycle" {
				if binding.Hook == nil {
					continue
				}
				record.Kind, record.Target = string(ActionCommandHook), o.layout.SettingsFile
				hook := fromBindingHook(*binding)
				observation := EnrichHookObservation(ObserveSettings(record.Target, nil), hook)
				if observation.Err != nil {
					return OwnershipSnapshot{}, observation.Err
				}
				record.Fingerprint = HookOwnershipFingerprint(hook.Event, observation.EntryFingerprint)
				record.HookProvenance = owner.AdapterProvenance
				record.HookEvent = hook.Event
			} else if resource.Kind == "mcp_server" {
				record.Kind, record.Target = string(ActionUserMCP), name
				observation := ObserveUserMCP(o.layout.UserMCPFile, name)
				if observation.Err != nil {
					return OwnershipSnapshot{}, observation.Err
				}
				record.Fingerprint = observation.DefinitionFingerprint
				identity := NewMCPIdentity(name, resource.Command, resource.Args, map[string]string{})
				record.Command, record.Args, record.EnvironmentKeys, record.EnvironmentFingerprint = identity.Command, identity.Args, identity.EnvironmentKeys, identity.EnvironmentFingerprint
			} else {
				record.Kind, record.Target = string(ActionSkillLink), filepath.Join(o.layout.SkillsDir, name)
				source := filepath.Join(o.bundleRoot, filepath.FromSlash(resource.Source))
				expectedSource, err := filepath.EvalSymlinks(source)
				if err != nil {
					return OwnershipSnapshot{}, fmt.Errorf("resolve Claude skill source %s: %w", resource.ID, err)
				}
				expectedSource = filepath.Clean(expectedSource)
				observed := ObserveSkill(record.Target, expectedSource)
				record.Fingerprint = observed.TreeFingerprint
				record.Skill = SkillIdentity{Surface: "claude", ProjectionID: id, Path: record.Target, SymlinkType: "directory", ResolvedTarget: observed.ResolvedTarget, ExpectedSource: expectedSource, SourceTreeFingerprint: observed.TreeFingerprint}
			}
			records = append(records, record)
			recorded[id] = true
			if resource.Kind == "command" && !isAddyCompositeProjection(pack, resource, *binding) {
				for _, asset := range dependencyAssets(pack, resource) {
					assetID := "asset:" + id + ":" + asset.ID
					if recorded[assetID] {
						continue
					}
					assetOwner, retained := owners[assetID]
					recoverable := state.Journal != nil && state.Journal.PackID == intent.PackID && recoveryActions[assetID]
					if !retained && !recoverable {
						continue
					}
					target := filepath.Join(o.layout.SkillsDir, name, filepath.Base(asset.Source))
					fingerprint, _, err := localprojection.FingerprintPath(target)
					if err != nil {
						return OwnershipSnapshot{}, err
					}
					contributors := append([]string(nil), assetOwner.Contributors...)
					if len(contributors) == 0 {
						contributors = []string{intent.PackID}
					}
					records = append(records, OwnershipRecord{StateOwner: "capabilitypack", ContributorID: contributors[0], ID: assetID, Kind: string(ActionSkillFile), Target: target, Fingerprint: fingerprint, Contributors: contributors, DeletionAuthorized: assetOwner.DeletionAuthorized()})
					recorded[assetID] = true
				}
			}
		}
	}
	return NewOwnershipSnapshot(records...), nil
}

func activeClaudeIntents(state capabilitypack.ActivationState) []capabilitypack.ActivationIntent {
	if len(state.Intents) == 0 {
		if state.Intent.Active || state.Journal != nil {
			return []capabilitypack.ActivationIntent{state.Intent}
		}
		return nil
	}
	result := []capabilitypack.ActivationIntent{}
	for _, intent := range state.Intents {
		if intent.Active && intent.Surface == capabilitypack.SurfaceClaude {
			result = append(result, intent)
		}
	}
	if state.Journal != nil && state.Intent.Surface == capabilitypack.SurfaceClaude && !state.Intent.Active {
		result = append(result, state.Intent)
	}
	return result
}

func resourceWithAliases(resource capabilitypack.Resource, aliases []capabilitypack.SurfaceAlias) capabilitypack.Resource {
	for _, alias := range aliases {
		if alias.Kind != resource.Kind || alias.ID != resource.ID {
			continue
		}
		resource.Bindings = append([]capabilitypack.Binding(nil), resource.Bindings...)
		for i := range resource.Bindings {
			if resource.Bindings[i].Surface == capabilitypack.SurfaceClaude {
				resource.Bindings[i].Name = alias.Name
			}
		}
		break
	}
	return resource
}

func dependencyAssets(pack capabilitypack.Pack, consumer capabilitypack.Resource) []capabilitypack.Resource {
	resources := map[string]capabilitypack.Resource{}
	for _, resource := range pack.Resources {
		resources[resource.Kind+":"+resource.ID] = resource
	}
	seen := map[string]bool{}
	result := []capabilitypack.Resource{}
	var visit func(string)
	visit = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		resource, ok := resources[id]
		if !ok {
			return
		}
		if resource.Kind == "asset" {
			result = append(result, resource)
		}
		for _, child := range resource.Requires {
			visit(child)
		}
	}
	for _, id := range consumer.Requires {
		visit(id)
	}
	return result
}
