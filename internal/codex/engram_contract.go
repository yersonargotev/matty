package codex

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/matty/internal/capabilitypack"
	"github.com/yersonargotev/matty/internal/localprojection"
)

const (
	engramInstructionsFingerprint = "74176fb0847b06fb725ae8992c9a5fa12022ff347ca3ee2ef3e77c6d318d5fb3"
	engramCompactFingerprint      = "c779d9584c8ca16331ebb31a753f7fbb5bcb8193b229572a54da189ffaa97fd1"
)

func hasEngramCodexSetupResources(pack capabilitypack.Pack) bool {
	hasInstruction, hasMCP := false, false
	for _, resource := range pack.Resources {
		hasInstruction = hasInstruction || resource.Kind == "instruction" && resource.ID == "engram-memory"
		hasMCP = hasMCP || resource.Kind == "mcp_server" && resource.ID == "engram"
	}
	return hasInstruction && hasMCP
}

func isEngramOwnedResource(resource capabilitypack.Resource) bool {
	return resource.Kind == "instruction" && resource.ID == "engram-memory" || resource.Kind == "mcp_server" && resource.ID == "engram"
}

func (a *ActivationAdapter) inspectEngramContract(config string, resolutions []capabilitypack.ExecutableResolution) ([]capabilitypack.ObservedProjection, error) {
	dir := filepath.Dir(a.configFile)
	instructionsPath := filepath.Join(dir, "engram-instructions.md")
	compactPath := filepath.Join(dir, "engram-compact-prompt.md")
	instructions, err := readOptionalFile(instructionsPath)
	if err != nil {
		return nil, err
	}
	compact, err := readOptionalFile(compactPath)
	if err != nil {
		return nil, err
	}
	command := capabilitypack.ResolvedExecutablePath("engram", resolutions)
	checks := []struct {
		id       string
		valid    bool
		evidence string
	}{
		{"mcp", tomlSectionHas(config, "mcp_servers.engram", map[string]string{"command": command, "args": `["mcp", "--tools=agent"]`}), config},
		{"instructions", tomlSectionHas(config, "", map[string]string{"model_instructions_file": instructionsPath}) && localprojection.FingerprintBytes([]byte(instructions)) == engramInstructionsFingerprint, config + "\n" + instructions},
		{"compact-prompt", tomlSectionHas(config, "", map[string]string{"experimental_compact_prompt_file": compactPath}) && localprojection.FingerprintBytes([]byte(compact)) == engramCompactFingerprint, config + "\n" + compact},
		{"marketplace", tomlSectionHas(config, "marketplaces.engram", map[string]string{"source_type": "git", "source": "https://github.com/Gentleman-Programming/engram.git", "ref": "main"}), config},
		{"plugin", tomlSectionHas(config, `plugins."engram@engram"`, map[string]string{"enabled": "true"}), config},
	}
	result := make([]capabilitypack.ObservedProjection, 0, len(checks))
	for _, check := range checks {
		id := "external_setup:engram:codex:" + check.id
		desired := localprojection.FingerprintBytes([]byte("engram-codex-contract-v1:" + check.id))
		observed := localprojection.FingerprintBytes([]byte(check.evidence))
		if check.valid {
			observed = desired
		}
		result = append(result, capabilitypack.ObservedProjection{
			ID: id, Exists: check.valid, ObservedFingerprint: observed, DesiredFingerprint: desired, ExternallyManaged: true,
			Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionCodexMCPConfig, Target: a.configFile, Description: fmt.Sprintf("observe Engram-owned Codex %s configuration", check.id)},
		})
	}
	return result, nil
}

func tomlSectionHas(content, section string, expected map[string]string) bool {
	found := map[string]string{}
	current := ""
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			continue
		}
		if current != section {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			found[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	for key, value := range expected {
		if found[key] != value {
			return false
		}
	}
	return true
}
