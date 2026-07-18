package packsync

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type compatibilityEvidence struct {
	SchemaVersion  int    `json:"schema_version"`
	PackID         string `json:"pack_id"`
	FromVersion    string `json:"from_version"`
	ToVersion      string `json:"to_version"`
	Classification struct {
		Level     string `json:"level"`
		DecidedBy string `json:"decided_by"`
	} `json:"classification"`
	Rationale string `json:"rationale"`
	Migration struct {
		InstructionID     string            `json:"instruction_id"`
		InstructionSource string            `json:"instruction_source"`
		ReplacementRules  []replacementRule `json:"replacement_rules"`
		DivergentFiles    []replacementFile `json:"divergent_files"`
	} `json:"migration"`
	HistoricalArtifact struct {
		Path            string `json:"path"`
		ArtifactSHA256  string `json:"artifact_sha256"`
		AggregateSHA256 string `json:"aggregate_sha256"`
	} `json:"historical_artifact"`
	Selection struct {
		Unchanged    bool     `json:"unchanged"`
		SourceID     string   `json:"source_id"`
		BindingCount int      `json:"binding_count"`
		Bindings     []string `json:"bindings"`
	} `json:"selection"`
	ClaimsUpstreamProvenance bool `json:"claims_upstream_provenance"`
	ReplacesSourceLock       bool `json:"replaces_source_lock"`
}

type replacementRule struct {
	ID        string `json:"id"`
	Semantics string `json:"semantics"`
}

type replacementFile struct {
	Path             string   `json:"path"`
	ReplacementRules []string `json:"replacement_rules"`
}

type acceptedCompatibilityContract struct {
	PackID         string
	FromVersion    string
	ToVersion      string
	EvidenceSHA256 string
}

var acceptedCompatibilityContracts = []acceptedCompatibilityContract{
	{PackID: "matty", FromVersion: "1.0.0", ToVersion: "2.0.0", EvidenceSHA256: "fc15a7e2a3d14851356278d206b32cea5ea6b770cabc7a30267bd04e68b61bac"},
}

func compatibilityBlockers(repositoryRoot, snapshotRoot string, source SourceConfig, bindings []Binding, manifests map[string]packManifest) []string {
	validated := map[string]bool{}
	var blockers []string
	for _, contract := range acceptedCompatibilityContracts {
		current, ok := manifests[contract.PackID]
		if !ok || current.Version != contract.ToVersion {
			continue
		}
		historyRoot := filepath.Join(repositoryRoot, "bundle", "history", contract.PackID, contract.FromVersion)
		historical, err := readCompatibilityManifest(filepath.Join(historyRoot, "pack.json"))
		if err != nil || historical.ID != contract.PackID || historical.Version != contract.FromVersion {
			blockers = append(blockers, fmt.Sprintf("accepted compatibility history is missing or invalid for %s %s to %s", contract.PackID, contract.FromVersion, contract.ToVersion))
			continue
		}
		key := compatibilityKey(contract.PackID, contract.FromVersion, contract.ToVersion)
		validated[key] = true
		blockers = append(blockers, validateCompatibilityEvidence(repositoryRoot, snapshotRoot, source, bindings, current, historical, historyRoot, contract.EvidenceSHA256)...)
	}

	paths, err := filepath.Glob(filepath.Join(repositoryRoot, "bundle", "history", "*", "*", "pack.json"))
	if err != nil {
		return append(blockers, "inspect compatibility history: "+err.Error())
	}
	for _, historyPath := range paths {
		historical, err := readCompatibilityManifest(historyPath)
		if err != nil {
			blockers = append(blockers, "decode historical manifest for compatibility: "+err.Error())
			continue
		}
		current, ok := manifests[historical.ID]
		if !ok || current.Version == historical.Version {
			continue
		}
		if validated[compatibilityKey(historical.ID, historical.Version, current.Version)] {
			continue
		}
		blockers = append(blockers, validateCompatibilityEvidence(repositoryRoot, snapshotRoot, source, bindings, current, historical, filepath.Dir(historyPath), "")...)
	}
	return blockers
}

func validateCompatibilityEvidence(repositoryRoot, snapshotRoot string, source SourceConfig, bindings []Binding, current, historical packManifest, historyRoot, trustedDigest string) []string {
	evidencePath := filepath.Join(repositoryRoot, "bundle", "compatibility", historical.ID, fmt.Sprintf("%s-to-%s.json", historical.Version, current.Version))
	data, err := os.ReadFile(evidencePath)
	if err != nil {
		return []string{fmt.Sprintf("compatibility evidence is missing for %s %s to %s", historical.ID, historical.Version, current.Version)}
	}
	var evidence compatibilityEvidence
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return []string{"compatibility evidence is invalid: " + err.Error()}
	}
	if err := ensureCompatibilityEOF(decoder); err != nil {
		return []string{"compatibility evidence is invalid: " + err.Error()}
	}
	var failures []string
	if trustedDigest != "" && hashBytes(data) != trustedDigest {
		failures = append(failures, "accepted compatibility evidence does not match its trusted digest")
	}
	if evidence.SchemaVersion != 1 || evidence.PackID != historical.ID || evidence.FromVersion != historical.Version || evidence.ToVersion != current.Version {
		failures = append(failures, "identity and exact versions are incomplete")
	}
	if evidence.Classification.Level != "major" || evidence.Classification.DecidedBy != "human" || strings.TrimSpace(evidence.Rationale) == "" {
		failures = append(failures, "human major classification or rationale is incomplete")
	}
	if evidence.ClaimsUpstreamProvenance || evidence.ReplacesSourceLock {
		failures = append(failures, "migration evidence must not claim upstream provenance or replace the source lock")
	}

	rules := map[string]string{}
	for _, rule := range evidence.Migration.ReplacementRules {
		if rule.ID == "" || strings.TrimSpace(rule.Semantics) == "" || rules[rule.ID] != "" {
			failures = append(failures, "replacement semantics are missing or duplicated")
			continue
		}
		rules[rule.ID] = rule.Semantics
	}
	if len(rules) == 0 {
		failures = append(failures, "replacement semantics are missing")
	}
	instruction, found := manifestResourceByKey(current, "instruction", evidence.Migration.InstructionID)
	if !found || instruction.Source != evidence.Migration.InstructionSource || !safeSlashPath(instruction.Source) {
		failures = append(failures, "replacement instruction is absent from the current pack")
	} else {
		instructionBytes, err := os.ReadFile(filepath.Join(repositoryRoot, "bundle", filepath.FromSlash(instruction.Source)))
		if err != nil {
			failures = append(failures, "replacement instruction bytes are unavailable")
		} else {
			for _, semantics := range rules {
				if !strings.Contains(string(instructionBytes), semantics) {
					failures = append(failures, "replacement semantics are absent from the Packy-owned instruction")
					break
				}
			}
		}
	}

	selection, selectionFailures := compatibilitySelection(source, bindings, current, historical, evidence)
	failures = append(failures, selectionFailures...)
	failures = append(failures, validateManifestMigration(current, historical, evidence)...)
	expectedDivergence, err := historicalDivergence(snapshotRoot, historyRoot, selection)
	if err != nil {
		failures = append(failures, "inspect historical divergent files: "+err.Error())
	} else if mappingFailures := validateReplacementMapping(evidence.Migration.DivergentFiles, rules, expectedDivergence); len(mappingFailures) > 0 {
		failures = append(failures, mappingFailures...)
	}

	expectedArtifactPath := filepath.ToSlash(filepath.Join("history", historical.ID, historical.Version, "artifact.json"))
	if evidence.HistoricalArtifact.Path != expectedArtifactPath {
		failures = append(failures, "historical-artifact hash or path changed")
	} else {
		artifactPath := filepath.Join(repositoryRoot, "bundle", filepath.FromSlash(expectedArtifactPath))
		artifactBytes, err := os.ReadFile(artifactPath)
		if err != nil || hashBytes(artifactBytes) != evidence.HistoricalArtifact.ArtifactSHA256 {
			failures = append(failures, "historical-artifact hash or path changed")
			return prefixCompatibility(failures)
		}
		var artifact struct {
			AggregateSHA256 string `json:"aggregate_sha256"`
		}
		if err := json.Unmarshal(artifactBytes, &artifact); err != nil || artifact.AggregateSHA256 == "" || artifact.AggregateSHA256 != evidence.HistoricalArtifact.AggregateSHA256 {
			failures = append(failures, "historical aggregate hash is incomplete or changed")
		}
	}
	return prefixCompatibility(failures)
}

func readCompatibilityManifest(name string) (packManifest, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return packManifest{}, err
	}
	var manifest packManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return packManifest{}, err
	}
	return manifest, nil
}

func compatibilityKey(packID, fromVersion, toVersion string) string {
	return packID + "@" + fromVersion + "->" + toVersion
}

func compatibilitySelection(source SourceConfig, bindings []Binding, current, historical packManifest, evidence compatibilityEvidence) ([]Binding, []string) {
	var selected []Binding
	var keys []string
	for _, binding := range bindings {
		if binding.PackID != current.ID {
			continue
		}
		selected = append(selected, binding)
		keys = append(keys, bindingKey(binding))
		if _, ok := manifestResourceByKey(current, binding.Kind, binding.ResourceID); !ok {
			return selected, []string{"selection changed from the current manifest"}
		}
		if _, ok := manifestResourceByKey(historical, binding.Kind, binding.ResourceID); !ok {
			return selected, []string{"selection changed from the historical manifest"}
		}
	}
	sort.Strings(keys)
	if !evidence.Selection.Unchanged || evidence.Selection.SourceID != source.ID || evidence.Selection.BindingCount != len(keys) || !reflect.DeepEqual(evidence.Selection.Bindings, keys) {
		return selected, []string{"selection evidence changed or is incomplete"}
	}
	return selected, nil
}

func validateManifestMigration(current, historical packManifest, evidence compatibilityEvidence) []string {
	historicalResources := map[string]manifestResource{}
	for _, resource := range historical.Resources {
		historicalResources[resource.Kind+"/"+resource.ID] = resource
	}
	added := 0
	for _, resource := range current.Resources {
		key := resource.Kind + "/" + resource.ID
		before, existed := historicalResources[key]
		if existed {
			if before.Source != resource.Source {
				return []string{"pack resource selection or source changed during migration"}
			}
			delete(historicalResources, key)
			continue
		}
		if resource.Kind != "instruction" || resource.ID != evidence.Migration.InstructionID || resource.Source != evidence.Migration.InstructionSource {
			return []string{"pack resource selection changed beyond the replacement instruction"}
		}
		added++
	}
	if len(historicalResources) != 0 || added != 1 {
		return []string{"pack resource selection changed or replacement instruction is not unique"}
	}
	return nil
}

func historicalDivergence(snapshotRoot, historyRoot string, bindings []Binding) ([]string, error) {
	var paths []string
	manifestData, err := os.ReadFile(filepath.Join(historyRoot, "pack.json"))
	if err != nil {
		return nil, err
	}
	var historical packManifest
	if err := json.Unmarshal(manifestData, &historical); err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		resource, ok := manifestResourceByKey(historical, binding.Kind, binding.ResourceID)
		if !ok {
			continue
		}
		before, err := inventory(filepath.Join(historyRoot, filepath.FromSlash(resource.Source)))
		if err != nil {
			return nil, err
		}
		after, err := inventory(filepath.Join(snapshotRoot, filepath.FromSlash(binding.UpstreamPath)))
		if err != nil {
			return nil, err
		}
		for _, change := range diffFiles(binding, before, after) {
			paths = append(paths, strings.TrimPrefix(change.Path, "bundle/"))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func validateReplacementMapping(files []replacementFile, rules map[string]string, expected []string) []string {
	seen := map[string]bool{}
	usedRules := map[string]bool{}
	var actual []string
	for _, file := range files {
		if file.Path == "" || !safeSlashPath(file.Path) || seen[file.Path] || len(file.ReplacementRules) == 0 {
			return []string{"divergent-file mapping is incomplete, duplicated, or references missing replacement semantics"}
		}
		seen[file.Path] = true
		seenFileRules := map[string]bool{}
		for _, rule := range file.ReplacementRules {
			if rules[rule] == "" || seenFileRules[rule] {
				return []string{"divergent-file mapping is incomplete, duplicated, or references missing replacement semantics"}
			}
			seenFileRules[rule] = true
			usedRules[rule] = true
		}
		actual = append(actual, file.Path)
	}
	sort.Strings(actual)
	if !reflect.DeepEqual(actual, expected) {
		return []string{"divergent-file mapping does not exactly cover historical-to-upstream byte drift"}
	}
	if len(usedRules) != len(rules) {
		return []string{"divergent-file mapping does not use every replacement rule"}
	}
	return nil
}

func manifestResourceByKey(manifest packManifest, kind, id string) (manifestResource, bool) {
	for _, resource := range manifest.Resources {
		if resource.Kind == kind && resource.ID == id {
			return resource, true
		}
	}
	return manifestResource{}, false
}

func ensureCompatibilityEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func prefixCompatibility(failures []string) []string {
	for i := range failures {
		failures[i] = "compatibility evidence: " + failures[i]
	}
	return failures
}
