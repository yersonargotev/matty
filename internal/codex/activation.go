package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/matty/internal/capabilitypack"
	"github.com/yersonargotev/matty/internal/localprojection"
)

const (
	instructionStart = "<!-- matty:pack:matty-guidance:start -->"
	instructionEnd   = "<!-- matty:pack:matty-guidance:end -->"
)

type ActivationAdapter struct {
	bundleRoot string
	skillsDir  string
	promptFile string
}

func NewActivationAdapter(bundleRoot, skillsDir, promptFile string) *ActivationAdapter {
	return &ActivationAdapter{bundleRoot: bundleRoot, skillsDir: skillsDir, promptFile: promptFile}
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
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillLink, Source: source, Target: target, Description: fmt.Sprintf("link skill %s at %s", resource.ID, target)}})
			revisionParts = append(revisionParts, id+"="+observed)
		case "instruction":
			content, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read instruction %q: %w", resource.ID, err)
			}
			desiredBlock := instructionStart + "\n" + strings.TrimSpace(string(content)) + "\n" + instructionEnd
			current, err := os.ReadFile(a.promptFile)
			if err != nil && !os.IsNotExist(err) {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read Codex instructions: %w", err)
			}
			fragment, exists := extractBlock(string(current))
			observed := "missing"
			if exists {
				observed = localprojection.FingerprintBytes([]byte(fragment))
			}
			desired := localprojection.FingerprintBytes([]byte(desiredBlock))
			merged := mergeBlock(string(current), desiredBlock)
			id := "instruction:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionInstructionFile, Target: a.promptFile, Content: merged, Description: fmt.Sprintf("write instruction %s in %s", resource.ID, a.promptFile)}})
			revisionParts = append(revisionParts, "prompt="+localprojection.FingerprintBytes(current))
		}
	}
	sort.Strings(revisionParts)
	return capabilitypack.ActivationObservation{Revision: localprojection.FingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections}, nil
}

func (a *ActivationAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) error {
	executor := localprojection.Executor{
		Host:         "Codex",
		SymlinkKinds: map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionSkillLink: true},
		FileKinds:    map[capabilitypack.ProjectionActionKind]bool{capabilitypack.ActionInstructionFile: true},
	}
	return executor.Apply(actions)
}

func extractBlock(content string) (string, bool) {
	start := strings.Index(content, instructionStart)
	if start < 0 {
		return "", false
	}
	relEnd := strings.Index(content[start:], instructionEnd)
	if relEnd < 0 {
		return "", false
	}
	end := start + relEnd + len(instructionEnd)
	return content[start:end], true
}
func mergeBlock(content, block string) string {
	if existing, ok := extractBlock(content); ok {
		return strings.Replace(content, existing, block, 1)
	}
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}
