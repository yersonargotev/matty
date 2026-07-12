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

func (a *ActivationAdapter) InspectActivation(_ context.Context, pack capabilitypack.Pack) (capabilitypack.ActivationObservation, error) {
	var projections []capabilitypack.ObservedProjection
	var revisionParts []string
	for _, resource := range pack.Resources {
		source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
		switch resource.Kind {
		case "skill":
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
			content, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read instruction %q: %w", resource.ID, err)
			}
			desiredContent := strings.TrimSpace(string(content)) + "\n"
			currentPrompt, err := os.ReadFile(a.promptFile)
			if err != nil && !os.IsNotExist(err) {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read OpenCode instruction file: %w", err)
			}
			promptObserved := "missing"
			promptExists := err == nil
			if promptExists {
				promptObserved = localprojection.FingerprintBytes(currentPrompt)
			}
			promptID := "instruction:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: promptID, Exists: promptExists, ObservedFingerprint: promptObserved, DesiredFingerprint: localprojection.FingerprintBytes([]byte(desiredContent)), Action: capabilitypack.ProjectionAction{ID: promptID, Kind: capabilitypack.ActionOpenCodeInstructionFile, Target: a.promptFile, Content: desiredContent, Description: fmt.Sprintf("write OpenCode instruction %s at %s", resource.ID, a.promptFile)}})

			currentConfig, err := readOptionalActivationFile(a.configFile)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			inspection, err := opencode.Inspect(a.configFile, a.promptFile)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			merged, err := opencode.MergeInstructionProjection(currentConfig, a.configFile, a.promptFile)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			refID := "opencode-instruction-reference:" + resource.ID
			refDesired := localprojection.FingerprintBytes([]byte(a.promptFile))
			refObserved := "missing"
			if inspection.HasMattyInstruction {
				refObserved = refDesired
			}
			projections = append(projections, capabilitypack.ObservedProjection{ID: refID, Exists: inspection.HasMattyInstruction, ObservedFingerprint: refObserved, DesiredFingerprint: refDesired, Action: capabilitypack.ProjectionAction{ID: refID, Kind: capabilitypack.ActionOpenCodeConfigReference, Target: a.configFile, Content: merged, Description: fmt.Sprintf("add OpenCode instruction reference in %s", a.configFile)}})
			revisionParts = append(revisionParts, "prompt="+localprojection.FingerprintBytes(currentPrompt), "config="+localprojection.FingerprintBytes([]byte(currentConfig)))
		}
	}
	sort.Strings(revisionParts)
	return capabilitypack.ActivationObservation{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections}, nil
}

func (a *ActivationAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) error {
	for _, action := range actions {
		if action.Kind == capabilitypack.ActionOpenCodeConfigReference {
			if err := opencode.ValidateInstructionProjection(action.Content, a.promptFile); err != nil {
				return fmt.Errorf("validate staged OpenCode config: %w", err)
			}
		}
	}
	executor := localprojection.Executor{
		Host:         "OpenCode",
		SymlinkKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionOpenCodeSkillLink: true},
		FileKinds: map[capabilitypack.ProjectionActionKind]bool{
			capabilitypack.ActionOpenCodeInstructionFile: true, capabilitypack.ActionOpenCodeConfigReference: true,
		},
	}
	return executor.Apply(actions)
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
