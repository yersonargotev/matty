package claudecode

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/capabilitypack"
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
	if !state.Intent.Active && state.Journal == nil {
		return NewOwnershipSnapshot(), nil
	}
	pack, ok := o.packs[state.Intent.PackID]
	if !ok || pack.Version != state.Intent.Version {
		return OwnershipSnapshot{}, fmt.Errorf("Claude ownership intent %s@%s has no exact registered adapter contract", state.Intent.PackID, state.Intent.Version)
	}
	owners := make(map[string]capabilitypack.ProjectionOwnership, len(state.Ownership))
	for _, owner := range state.Ownership {
		owners[owner.ID] = owner
	}
	records := make([]OwnershipRecord, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		if resource.Kind != "skill" && resource.Kind != "instruction" {
			continue
		}
		name := resource.ID
		for _, binding := range resource.Bindings {
			if binding.Surface == capabilitypack.SurfaceClaude {
				name = binding.Name
				break
			}
		}
		id := resource.Kind + ":" + name
		owner, retained := owners[id]
		if !retained && state.Journal == nil {
			continue
		}
		contributors := append([]string(nil), owner.Contributors...)
		if len(contributors) == 0 {
			contributors = []string{state.Intent.PackID}
		}
		record := OwnershipRecord{StateOwner: "capabilitypack", ContributorID: state.Intent.PackID, ID: id, Fingerprint: owner.Fingerprint, Contributors: contributors, DeletionAuthorized: owner.DeletionAuthorized()}
		if resource.Kind == "instruction" {
			record.Kind, record.Target = string(ActionInstructionContribution), o.layout.InstructionsFile
			observation := ObserveInstructions(record.Target)
			record.ContributorID = "pack:" + state.Intent.PackID + ":" + resource.ID
			record.Contributors = []string{record.ContributorID}
			record.Fingerprint = observation.Contributions[record.ContributorID]
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
	}
	return NewOwnershipSnapshot(records...), nil
}
